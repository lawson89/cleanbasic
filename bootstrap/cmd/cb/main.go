package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"cleanbasic/bootstrap/internal/codegen"
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
	case "check":
		return runCheck(args[1:])
	case "emit":
		return runEmit(args[1:])
	case "build":
		return runBuild(args[1:])
	case "run":
		return runExec(args[1:])
	default:
		if strings.HasPrefix(first, "-") {
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", first)
		} else {
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", first)
		}
		printUsage()
		return 2
	}
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

func transpileProgram(filePath string) (string, int) {
	prog, err := sema.LoadAndAnalyze(filePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return "", 1
	}

	goCode, err := codegen.EmitGo(prog)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return "", 1
	}

	return goCode, 0
}

func syncFileIfChanged(path string, content []byte) error {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, content) {
		return nil
	}

	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".sync-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmpFile.Write(content); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		// On Windows, concurrent os.Rename onto an existing file can fail;
		// if another writer already wrote identical content, treat as success.
		if current, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(current, content) {
			return nil
		}
		return err
	}
	cleanupTmp = false
	return nil
}

func prepareBuildJob(goCode string) (string, func(), error) {
	goModBytes := []byte("module cbmain\n\ngo 1.23\n")

	cacheRoot, err := os.UserCacheDir()
	if err == nil && cacheRoot != "" {
		workspaceDir := filepath.Join(cacheRoot, "cleanbasic", "work-default")
		if err := os.MkdirAll(workspaceDir, 0700); err != nil {
			return "", nil, err
		}
		if err := syncFileIfChanged(filepath.Join(workspaceDir, "go.mod"), goModBytes); err != nil {
			return "", nil, err
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

	jobDir, err := os.MkdirTemp("", "cleanbasic-work-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(jobDir)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "go.mod"), goModBytes, 0644); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := os.WriteFile(filepath.Join(jobDir, "main.go"), []byte(goCode), 0644); err != nil {
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

	goCode, code := transpileProgram(filePath)
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

func defaultBuildOutputPath(filePath, goos string) string {
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	outPath := strings.TrimSuffix(base, ext)
	if outPath == "" {
		if goos == "windows" {
			return "a.exe"
		}
		return "a.out"
	}
	if goos == "windows" && !strings.HasSuffix(strings.ToLower(outPath), ".exe") {
		outPath += ".exe"
	}
	return outPath
}

func runBuild(args []string) int {
	filePath, outPath, code := parseFileAndOutArgs(args, true)
	if code != 0 {
		return code
	}

	goCode, code := transpileProgram(filePath)
	if code != 0 {
		return code
	}

	if outPath == "" {
		outPath = defaultBuildOutputPath(filePath, runtime.GOOS)
	}

	absOut, err := filepath.Abs(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error resolving output path %s: %v\n", outPath, err)
		return 1
	}

	jobDir, cleanup, err := prepareBuildJob(goCode)
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

	goCode, code := transpileProgram(filePath)
	if code != 0 {
		return code
	}

	jobDir, cleanup, err := prepareBuildJob(goCode)
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
	fmt.Fprintln(os.Stderr, "  cb check <file.cb>")
	fmt.Fprintln(os.Stderr, "  cb emit <file.cb> [-o <out.go>]")
	fmt.Fprintln(os.Stderr, "  cb build <file.cb> [-o <bin>]")
	fmt.Fprintln(os.Stderr, "  cb run <file.cb> [args...]")
}
