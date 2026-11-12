package tests

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cleanbasic/bootstrap/internal/codegen"
	"cleanbasic/bootstrap/internal/sema"
)

// Optional -compiler flag so the exact same test suite can later validate
// the self-hosted Stage 1 compiler binary (e.g., `go test ./... -args -compiler=../../build/cb1`).
var compilerBin = flag.String("compiler", "", "Path to external CleanBasic compiler binary (uses in-process Go bootstrap if empty)")

type testExpectations struct {
	expectedOut []string
	expectedErr []string
	skip        bool
}

func parseTestDirectives(src string) testExpectations {
	var exp testExpectations
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "' SKIP_GO") {
			exp.skip = true
		} else if after, ok := strings.CutPrefix(trimmed, "' EXPECT_OUT:"); ok {
			exp.expectedOut = append(exp.expectedOut, strings.TrimSpace(after))
		} else if after, ok := strings.CutPrefix(trimmed, "' EXPECT_ERR:"); ok {
			exp.expectedErr = append(exp.expectedErr, strings.TrimSpace(after))
		}
	}
	return exp
}

func TestCleanBasicCases(t *testing.T) {
	casesDir := filepath.Join("..", "..", "tests", "cases")
	entries, err := os.ReadDir(casesDir)
	if err != nil {
		t.Fatalf("failed to read %s: %v", casesDir, err)
	}

	for _, entry := range entries {
		name := entry.Name()
		var entryFile string

		if entry.IsDir() {
			// Multi-file test case directory (expects main.cb inside)
			candidate := filepath.Join(casesDir, name, "main.cb")
			if _, err := os.Stat(candidate); err == nil {
				entryFile = candidate
			} else {
				continue
			}
		} else if strings.HasSuffix(name, ".cb") {
			entryFile = filepath.Join(casesDir, name)
		} else {
			continue
		}

		t.Run(name, func(t *testing.T) {
			rawBytes, err := os.ReadFile(entryFile)
			if err != nil {
				t.Fatalf("failed to read %s: %v", entryFile, err)
			}
			src := string(rawBytes)
			exp := parseTestDirectives(src)

			if exp.skip {
				t.Skip("skipped via SKIP_GO directive")
			}

			tmpDir := t.TempDir()
			outGoPath := filepath.Join(tmpDir, "main.go")

			// 1. Transpile CleanBasic -> Go
			goCode, compileErr := transpileFile(entryFile, outGoPath)

			// Handle expected compile-time error tests
			if len(exp.expectedErr) > 0 {
				if compileErr == nil {
					t.Fatalf("expected compile error containing %v, but compilation succeeded", exp.expectedErr)
				}
				errMsg := compileErr.Error()
				for _, wantErr := range exp.expectedErr {
					if !strings.Contains(errMsg, wantErr) {
						t.Errorf("compile error %q did not contain expected substring %q", errMsg, wantErr)
					}
				}
				return
			}

			if compileErr != nil {
				t.Fatalf("unexpected compile error:\n%v", compileErr)
			}

			// 2. Run the generated main.go via `go run`
			cmd := exec.Command("go", "run", outGoPath)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			if runErr := cmd.Run(); runErr != nil {
				t.Fatalf("generated Go code failed to run: %v\n--- STDERR ---\n%s\n--- GENERATED GO ---\n%s",
					runErr, stderr.String(), goCode)
			}

			// 3. Verify STDOUT lines against EXPECT_OUT directives
			actualOut := strings.TrimRight(strings.ReplaceAll(stdout.String(), "\r\n", "\n"), "\n")
			wantOut := strings.Join(exp.expectedOut, "\n")

			if actualOut != wantOut {
				t.Errorf("stdout mismatch:\n--- WANT ---\n%s\n--- GOT ---\n%s\n--- GENERATED GO ---\n%s",
					wantOut, actualOut, goCode)
			}
		})
	}
}

func transpileFile(entryPath, outGoPath string) (string, error) {
	// If -compiler=<bin> is provided, shell out to the compiled binary (e.g., Stage 1 self-hosted cb1)
	if *compilerBin != "" {
		cmd := exec.Command(*compilerBin, "emit", entryPath, "-o", outGoPath)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return "", &externalCompileError{msg: stderr.String()}
		}
		b, err := os.ReadFile(outGoPath)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	// Default: In-process Go bootstrap compiler (Stage 0)
	prog, err := sema.LoadAndAnalyze(entryPath)
	if err != nil {
		return "", err
	}
	goCode, err := codegen.EmitGo(prog)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(outGoPath, []byte(goCode), 0644); err != nil {
		return "", err
	}
	return goCode, nil
}

type externalCompileError struct {
	msg string
}

func (e *externalCompileError) Error() string {
	return e.msg
}
