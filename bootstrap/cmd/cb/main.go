package main

import (
	"fmt"
	"os"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/parser"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 2
	}

	var filePath string
	dumpAST := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-h", "--help", "help":
			printUsage()
			return 0
		case "parse":
			dumpAST = true
		case "--dump-ast":
			dumpAST = true
		default:
			if len(arg) > 0 && arg[0] == '-' {
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

	if dumpAST {
		fmt.Print(ast.Dump(prog))
	}
	return 0
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  cb parse <file.cb>")
	fmt.Fprintln(os.Stderr, "  cb --dump-ast <file.cb>")
}
