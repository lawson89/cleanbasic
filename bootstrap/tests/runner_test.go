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
		if strings.HasPrefix(trimmed, "// SKIP_GO") {
			exp.skip = true
		} else if after, ok := strings.CutPrefix(trimmed, "// EXPECT_OUT:"); ok {
			exp.expectedOut = append(exp.expectedOut, strings.TrimSpace(after))
		} else if after, ok := strings.CutPrefix(trimmed, "// EXPECT_ERR:"); ok {
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

		runCaseSubtest(t, name, entryFile)
	}

	errorsDir := filepath.Join(casesDir, "errors")
	if errEntries, err := os.ReadDir(errorsDir); err == nil {
		for _, errEntry := range errEntries {
			if errEntry.IsDir() || !strings.HasSuffix(errEntry.Name(), ".cb") {
				continue
			}
			errFile := filepath.Join(errorsDir, errEntry.Name())
			runCaseSubtest(t, "errors/"+errEntry.Name(), errFile)
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("failed to read %s: %v", errorsDir, err)
	}
}

func runCaseSubtest(t *testing.T, subtestName, entryFile string) {
	t.Run(subtestName, func(t *testing.T) {
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

		// 2. Run the generated main.go via go run
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

func TestCliPrintUsageLongOnly(t *testing.T) {
	tmpDir := t.TempDir()
	cbPath := filepath.Join(tmpDir, "cli_usage.cb")
	goPath := filepath.Join(tmpDir, "main.go")
	cbSrc := `import "std/cli"
fn main() as int
    dim p = cli.newParser("longonly", "1.0", "")
    p.addFlag("verbose", "", "Verbose output")
    p.addOption("config", "", "Config file", "cfg.toml")
    p.printUsage()
    return 0
end fn
`
	if err := os.WriteFile(cbPath, []byte(cbSrc), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if _, err := transpileFile(cbPath, goPath); err != nil {
		t.Fatalf("transpile failed: %v", err)
	}
	cmd := exec.Command("go", "run", goPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\n%s", err, stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "-, --") {
		t.Fatalf("unexpected empty short flag prefix in usage output:\n%s", out)
	}
	if !strings.Contains(out, "  --verbose  Verbose output") {
		t.Fatalf("missing long-only flag line in usage output:\n%s", out)
	}
	if !strings.Contains(out, "  --config <val>  Config file (default: cfg.toml)") {
		t.Fatalf("missing long-only option line in usage output:\n%s", out)
	}
}

func TestTutorialExercises(t *testing.T) {
	exercisesDir := filepath.Join("..", "..", "tutorial", "exercises")
	solutionsDir := filepath.Join("..", "..", "tutorial", "solutions")
	tutorialReadmePath := filepath.Join("..", "..", "tutorial", "README.md")

	readmeBytes, err := os.ReadFile(tutorialReadmePath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", tutorialReadmePath, err)
	}
	readmeText := strings.ReplaceAll(string(readmeBytes), "\r\n", "\n")

	cases := []struct {
		file    string
		wantOut string
	}{
		{
			file:    "ex01_variables_and_arithmetic.cb",
			wantOut: "0 C = 32 F\n25 C = 77 F\n100 C = 212 F",
		},
		{
			file:    "ex02_fizzbuzz.cb",
			wantOut: "1\n2\nFizz\n4\nBuzz\nFizz\n7\n8\nFizz\nBuzz\n11\nFizz\n13\n14\nFizzBuzz",
		},
		{
			file:    "ex03_functions_and_loops.cb",
			wantOut: "6! = 720\ndigitSum(4829) = 23",
		},
		{
			file:    "ex04_strings_and_lists.cb",
			wantOut: "items (4): apple | banana | cherry | date",
		},
		{
			file:    "ex05_dictionaries.cb",
			wantOut: "go: 3\nbasic: 2\nclean: 1\nrust: 0",
		},
		{
			file:    "ex06_structs_and_ufcs.cb",
			wantOut: "withdraw 200 ok: false\nwithdraw 40 ok: true\nAda: $110",
		},
		{
			file:    "ex07_enums_and_select.cb",
			wantOut: "Standard: $11\nExpress: $24\nOvernight: $43",
		},
		{
			file:    "ex08_error_handling.cb",
			wantOut: "valid sum: 40\nerror caught: value must be positive",
		},
		{
			file:    "ex09_defer_cleanup.cb",
			wantOut: "open: job-A\nwork: job-A\nclose: job-A\nopen: job-B\nclose: job-B\ncaught: job-B failed",
		},
		{
			file:    "ex10_concurrency_jobs.cb",
			wantOut: "primes in 1..400: 78",
		},
		{
			file:    "ex11_cancellation_and_timeouts.cb",
			wantOut: "fast sum: 55\nw1 done before cancel: false\ntimed out: true\nworker 1 canceled\nworker 2 canceled\nw1 done after wait: true",
		},
	}

	expectedSet := make(map[string]bool, len(cases))
	for _, tc := range cases {
		expectedSet[tc.file] = true
	}

	for _, dir := range []string{exercisesDir, solutionsDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("failed to read %s: %v", dir, err)
		}
		var cbCount int
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".cb") {
				continue
			}
			cbCount++
			if !expectedSet[entry.Name()] {
				t.Errorf("unexpected tutorial file %s in %s not listed in TestTutorialExercises", entry.Name(), dir)
			}
		}
		if cbCount != len(cases) {
			t.Errorf("expected %d .cb files in %s, found %d", len(cases), dir, cbCount)
		}
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			starterPath := filepath.Join(exercisesDir, tc.file)
			solutionPath := filepath.Join(solutionsDir, tc.file)

			// 1. Verify starter compiles cleanly
			starterTmp := t.TempDir()
			starterGo := filepath.Join(starterTmp, "main.go")
			if _, err := transpileFile(starterPath, starterGo); err != nil {
				t.Fatalf("starter %s failed to transpile: %v", starterPath, err)
			}
			starterBin := filepath.Join(starterTmp, "starter")
			buildCmd := exec.Command("go", "build", "-o", starterBin, starterGo)
			if out, err := buildCmd.CombinedOutput(); err != nil {
				t.Fatalf("starter %s generated Go failed to build: %v\n%s", starterPath, err, string(out))
			}

			// 2. Verify solution compiles, runs, and produces exact expected stdout
			solTmp := t.TempDir()
			solGo := filepath.Join(solTmp, "main.go")
			goCode, err := transpileFile(solutionPath, solGo)
			if err != nil {
				t.Fatalf("solution %s failed to transpile: %v", solutionPath, err)
			}
			runCmd := exec.Command("go", "run", solGo)
			var stdout, stderr bytes.Buffer
			runCmd.Stdout = &stdout
			runCmd.Stderr = &stderr
			if err := runCmd.Run(); err != nil {
				t.Fatalf("solution %s failed to run: %v\n--- STDERR ---\n%s\n--- GENERATED GO ---\n%s",
					solutionPath, err, stderr.String(), goCode)
			}
			actualOut := strings.TrimRight(strings.ReplaceAll(stdout.String(), "\r\n", "\n"), "\n")
			if actualOut != tc.wantOut {
				t.Errorf("solution %s stdout mismatch:\n--- WANT ---\n%s\n--- GOT ---\n%s",
					solutionPath, tc.wantOut, actualOut)
			}

			// 3. Verify tutorial/README.md contains the verbatim solution block
			solBytes, err := os.ReadFile(solutionPath)
			if err != nil {
				t.Fatalf("failed to read %s: %v", solutionPath, err)
			}
			solTrimmed := strings.TrimSpace(strings.ReplaceAll(string(solBytes), "\r\n", "\n"))
			fencedBlock := "```cleanbasic\n" + solTrimmed + "\n```"
			if !strings.Contains(readmeText, fencedBlock) {
				t.Errorf("tutorial/README.md does not contain verbatim fenced solution block for %s", tc.file)
			}
		})
	}
}
