package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	var filePath string
	for _, arg := range args {
		if arg == "--dump-ast" || arg == "parse" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", arg)
			printUsage()
			return 2
		}
		if filePath == "" {
			filePath = arg
		} else {
			fmt.Fprintf(os.Stderr, "unexpected argument: %s\n", arg)
			printUsage()
			return 2
		}
	}

	if filePath == "" {
		fmt.Fprintln(os.Stderr, "error: missing input .cb file path")
		printUsage()
		return 2
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
	filePath, outPath, code := parseFileAndOutArgs(args, false)
	if code != 0 {
		return code
	}
	_ = outPath

	if _, err := sema.LoadAndAnalyze(filePath); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}

func runEmit(args []string) int {
	filePath, outPath, code := parseFileAndOutArgs(args, true)
	if code != 0 {
		return code
	}

	prog, err := sema.LoadAndAnalyze(filePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	goCode, err := codegen.EmitGo(prog)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
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

	prog, err := sema.LoadAndAnalyze(filePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	goCode, err := codegen.EmitGo(prog)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
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

	tmpDir, err := os.MkdirTemp("", "cb-build-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating temp dir: %v\n", err)
		return 1
	}
	defer os.RemoveAll(tmpDir)

	mainGoPath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainGoPath, []byte(goCode), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing temp main.go: %v\n", err)
		return 1
	}

	cmd := exec.Command("go", "build", "-o", absOut, mainGoPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "go build failed: %v\n", err)
		return 1
	}

	return 0
}

func runExec(args []string) int {
	filePath, _, code := parseFileAndOutArgs(args, false)
	if code != 0 {
		return code
	}

	prog, err := sema.LoadAndAnalyze(filePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	goCode, err := codegen.EmitGo(prog)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	tmpDir, err := os.MkdirTemp("", "cb-run-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating temp dir: %v\n", err)
		return 1
	}
	defer os.RemoveAll(tmpDir)

	mainGoPath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainGoPath, []byte(goCode), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing temp main.go: %v\n", err)
		return 1
	}

	cmd := exec.Command("go", "run", mainGoPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "go run failed: %v\n", err)
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
	fmt.Fprintln(os.Stderr, "  cb run <file.cb>")
}
