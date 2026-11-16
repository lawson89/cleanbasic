package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/codegen"
	"cleanbasic/bootstrap/internal/parser"
	"cleanbasic/bootstrap/internal/sema"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 2
	}

	first := args[0]
	switch first {
	case "-h", "--help", "help":
		printUsage()
		return 0
	case "parse":
		return runParse(args[1:])
	case "--dump-ast":
		return runParse(args[1:])
	case "check":
		return runCheck(args[1:])
	case "emit":
		return runEmit(args[1:])
	case "build":
		return runBuild(args[1:])
	case "run":
		return runExec(args[1:])
	default:
		// Support e.g. cb <file.cb> --dump-ast
		for _, a := range args {
			if a == "--dump-ast" {
				return runParse(args)
			}
		}
		if strings.HasPrefix(first, "-") {
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", first)
		} else {
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", first)
		}
		printUsage()
		return 2
	}
}

func runParse(args []string) int {
	var filtered []string
	for _, arg := range args {
		if arg == "--dump-ast" || arg == "parse" {
			continue
		}
		filtered = append(filtered, arg)
	}
	filePath, _, code := parseFileAndOutArgs(filtered, false)
	if code != 0 {
		return code
	}

	rawBytes, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", filePath, err)
		return 1
	}

	prog, err := parser.Parse(string(rawBytes), filePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	fmt.Print(ast.Dump(prog))
	return 0
}

func runCheck(args []string) int {
	filePath, _, code := parseFileAndOutArgs(args, false)
	if code != 0 {
		return code
	}

	if _, err := sema.LoadAndAnalyze(filePath); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}

func transpileProgram(filePath string) (*sema.TypedProgram, string, int) {
	prog, err := sema.LoadAndAnalyze(filePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return nil, "", 1
	}

	goCode, err := codegen.EmitGo(prog)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return nil, "", 1
	}

	return prog, goCode, 0
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isBootstrapGoMod(goModPath string) bool {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "module ") {
			modName := strings.TrimSpace(strings.TrimPrefix(trimmed, "module "))
			return modName == "cleanbasic/bootstrap"
		}
	}
	return false
}

func isSubpath(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func walkUpForGoMod(startDir, targetFile string) (string, string, string) {
	dir := startDir
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if fileExists(goModPath) {
			if isBootstrapGoMod(goModPath) && !isSubpath(dir, targetFile) {
				// Ignore bootstrap/go.mod unless compiling inside bootstrap/
			} else {
				goSumPath := filepath.Join(dir, "go.sum")
				if !fileExists(goSumPath) {
					goSumPath = ""
				}
				return dir, goModPath, goSumPath
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", "", ""
}

func discoverGoMod(entryFile string, prog *sema.TypedProgram) (string, string, string) {
	if absEntry, err := filepath.Abs(entryFile); err == nil {
		if dir, gm, gs := walkUpForGoMod(filepath.Dir(absEntry), absEntry); gm != "" {
			return dir, gm, gs
		}
	}
	if prog != nil {
		for _, m := range prog.Modules {
			if m.IsStdlib || m.FilePath == "" {
				continue
			}
			if modAbs, err := filepath.Abs(m.FilePath); err == nil {
				if dir, gm, gs := walkUpForGoMod(filepath.Dir(modAbs), modAbs); gm != "" {
					return dir, gm, gs
				}
			}
		}
	}
	return "", "", ""
}

func rewriteReplaceLine(line, projectDir string) string {
	comment := ""
	code := line
	if idx := strings.Index(line, "//"); idx >= 0 {
		code = line[:idx]
		comment = line[idx:]
	}
	arrowIdx := strings.Index(code, "=>")
	if arrowIdx < 0 {
		return line
	}
	left := code[:arrowIdx+2]
	right := strings.TrimSpace(code[arrowIdx+2:])
	if right == "" {
		return line
	}

	var targetToken, rest string
	wasQuoted := false
	if strings.HasPrefix(right, "\x22") {
		if unq, endIdx, err := extractQuotedPrefix(right); err == nil {
			targetToken = unq
			rest = right[endIdx:]
			wasQuoted = true
		}
	}
	if targetToken == "" && !wasQuoted {
		fields := strings.Fields(right)
		if len(fields) == 0 {
			return line
		}
		targetToken = fields[0]
		rest = right[len(targetToken):]
	}

	if targetToken == "." || targetToken == ".." || strings.HasPrefix(targetToken, "./") || strings.HasPrefix(targetToken, "../") || strings.HasPrefix(targetToken, ".\x5c") || strings.HasPrefix(targetToken, "..\x5c") {
		absTarget := filepath.Clean(filepath.Join(projectDir, targetToken))
		if wasQuoted || strings.ContainsAny(absTarget, " \t") {
			targetToken = strconv.Quote(absTarget)
		} else {
			targetToken = absTarget
		}
		return left + " " + targetToken + rest + comment
	}
	return line
}

func extractQuotedPrefix(s string) (string, int, error) {
	for i := 1; i < len(s); i++ {
		if s[i] == '\x5c' {
			i++
			continue
		}
		if s[i] == '"' {
			unq, err := strconv.Unquote(s[:i+1])
			if err != nil {
				return "", 0, err
			}
			return unq, i + 1, nil
		}
	}
	return "", 0, fmt.Errorf("unterminated quote")
}

func rewriteGoModReplacePaths(raw []byte, projectDir string) []byte {
	lines := strings.Split(string(raw), "\n")
	inReplaceBlock := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		noComment := trimmed
		if idx := strings.Index(noComment, "//"); idx >= 0 {
			noComment = strings.TrimSpace(noComment[:idx])
		}
		if inReplaceBlock {
			if noComment == ")" {
				inReplaceBlock = false
				continue
			}
			if strings.Contains(noComment, "=>") {
				lines[i] = rewriteReplaceLine(line, projectDir)
			}
			continue
		}
		if strings.HasPrefix(noComment, "replace ") || noComment == "replace(" {
			rest := strings.TrimSpace(strings.TrimPrefix(noComment, "replace"))
			if strings.HasPrefix(rest, "(") {
				inReplaceBlock = true
				continue
			}
			if strings.Contains(noComment, "=>") {
				lines[i] = rewriteReplaceLine(line, projectDir)
			}
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func syncFileIfChanged(path string, content []byte) error {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, content) {
		return nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

func prepareBuildJob(entryFile string, prog *sema.TypedProgram, goCode string) (string, func(), error) {
	projectDir, goModPath, goSumPath := discoverGoMod(entryFile, prog)

	cacheRoot, err := os.UserCacheDir()
	if err != nil || cacheRoot == "" {
		cacheRoot = os.TempDir()
	}

	workKey := "stdlib"
	if projectDir != "" {
		sum := sha256.Sum256([]byte(projectDir))
		workKey = hex.EncodeToString(sum[:8])
	}
	workspaceDir := filepath.Join(cacheRoot, "cleanbasic", "work-"+workKey)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return "", nil, err
	}

	var goModBytes []byte
	if goModPath != "" {
		rawMod, err := os.ReadFile(goModPath)
		if err != nil {
			return "", nil, err
		}
		goModBytes = rewriteGoModReplacePaths(rawMod, projectDir)
	} else {
		goModBytes = []byte("module cbmain\n\ngo 1.22\n")
	}
	if err := syncFileIfChanged(filepath.Join(workspaceDir, "go.mod"), goModBytes); err != nil {
		return "", nil, err
	}

	goSumDst := filepath.Join(workspaceDir, "go.sum")
	if goSumPath != "" {
		rawSum, err := os.ReadFile(goSumPath)
		if err != nil {
			return "", nil, err
		}
		if err := syncFileIfChanged(goSumDst, rawSum); err != nil {
			return "", nil, err
		}
	} else {
		_ = os.Remove(goSumDst)
	}

	jobDir, err := os.MkdirTemp(workspaceDir, "job-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(jobDir)
	}

	mainGoPath := filepath.Join(jobDir, "main.go")
	if err := os.WriteFile(mainGoPath, []byte(goCode), 0644); err != nil {
		cleanup()
		return "", nil, err
	}

	return jobDir, cleanup, nil
}

func runGoBuildInDir(jobDir, absOut string) int {
	cmd := exec.Command("go", "build", "-o", absOut, ".")
	cmd.Dir = jobDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "go build failed: %v\n", err)
		return 1
	}
	return 0
}

func runEmit(args []string) int {
	filePath, outPath, code := parseFileAndOutArgs(args, true)
	if code != 0 {
		return code
	}

	_, goCode, code := transpileProgram(filePath)
	if code != 0 {
		return code
	}

	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(goCode), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outPath, err)
			return 1
		}
		return 0
	}

	fmt.Print(goCode)
	return 0
}

func runBuild(args []string) int {
	filePath, outPath, code := parseFileAndOutArgs(args, true)
	if code != 0 {
		return code
	}

	prog, goCode, code := transpileProgram(filePath)
	if code != 0 {
		return code
	}

	if outPath == "" {
		base := filepath.Base(filePath)
		ext := filepath.Ext(base)
		outPath = strings.TrimSuffix(base, ext)
		if outPath == "" {
			outPath = "a.out"
		}
	}

	absOut, err := filepath.Abs(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error resolving output path %s: %v\n", outPath, err)
		return 1
	}

	jobDir, cleanup, err := prepareBuildJob(filePath, prog, goCode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error preparing build workspace: %v\n", err)
		return 1
	}
	defer cleanup()

	return runGoBuildInDir(jobDir, absOut)
}

func runExec(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: missing input .cb file path")
		printUsage()
		return 2
	}
	filePath := args[0]
	if strings.HasPrefix(filePath, "-") {
		fmt.Fprintf(os.Stderr, "unknown flag: %s\n", filePath)
		printUsage()
		return 2
	}

	progArgs := args[1:]
	if len(progArgs) > 0 && progArgs[0] == "--" {
		progArgs = progArgs[1:]
	}

	prog, goCode, code := transpileProgram(filePath)
	if code != 0 {
		return code
	}

	jobDir, cleanup, err := prepareBuildJob(filePath, prog, goCode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error preparing build workspace: %v\n", err)
		return 1
	}
	defer cleanup()

	binName := "cbprog"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(jobDir, binName)
	if buildCode := runGoBuildInDir(jobDir, binPath); buildCode != 0 {
		return buildCode
	}

	cmd := exec.Command(binPath, progArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "execution failed: %v\n", err)
		return 1
	}
	return 0
}
func parseFileAndOutArgs(args []string, allowOutputFlag bool) (string, string, int) {
	var filePath string
	var outPath string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if allowOutputFlag && arg == "-o" {
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: missing argument for -o")
				printUsage()
				return "", "", 2
			}
			i++
			outPath = args[i]
			continue
		}
		if strings.HasPrefix(arg, "-") {
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", arg)
			printUsage()
			return "", "", 2
		}
		if filePath == "" {
			filePath = arg
		} else {
			fmt.Fprintf(os.Stderr, "unexpected argument: %s\n", arg)
			printUsage()
			return "", "", 2
		}
	}

	if filePath == "" {
		fmt.Fprintln(os.Stderr, "error: missing input .cb file path")
		printUsage()
		return "", "", 2
	}
	return filePath, outPath, 0
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  cb parse <file.cb>")
	fmt.Fprintln(os.Stderr, "  cb --dump-ast <file.cb>")
	fmt.Fprintln(os.Stderr, "  cb check <file.cb>")
	fmt.Fprintln(os.Stderr, "  cb emit <file.cb> [-o <out.go>]")
	fmt.Fprintln(os.Stderr, "  cb build <file.cb> [-o <bin>]")
	fmt.Fprintln(os.Stderr, "  cb run <file.cb> [args...]")
}
