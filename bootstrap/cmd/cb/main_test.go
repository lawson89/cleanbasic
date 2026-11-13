package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIParseCommand(t *testing.T) {
	file := filepath.Join("..", "..", "..", "tests", "cases", "01_hello_and_math.cb")
	if code := run([]string{"parse", file}); code != 0 {
		t.Fatalf("cb parse %s returned exit code %d, want 0", file, code)
	}
	if code := run([]string{"--dump-ast", file}); code != 0 {
		t.Fatalf("cb --dump-ast %s returned exit code %d, want 0", file, code)
	}
	if code := run([]string{}); code != 2 {
		t.Fatalf("cb with no args returned %d, want 2", code)
	}
}

func TestCLICheckEmitBuildAndRun(t *testing.T) {
	file01 := filepath.Join("..", "..", "..", "tests", "cases", "01_hello_and_math.cb")
	file02 := filepath.Join("..", "..", "..", "tests", "cases", "02_fibonacci.cb")

	// 1. cb check
	if code := run([]string{"check", file01}); code != 0 {
		t.Fatalf("cb check %s returned %d, want 0", file01, code)
	}
	if code := run([]string{"check", file02}); code != 0 {
		t.Fatalf("cb check %s returned %d, want 0", file02, code)
	}

	// 2. cb emit -o <out.go>
	tmpDir := t.TempDir()
	outGo := filepath.Join(tmpDir, "out.go")
	if code := run([]string{"emit", file01, "-o", outGo}); code != 0 {
		t.Fatalf("cb emit -o returned %d, want 0", code)
	}
	emittedBytes, err := os.ReadFile(outGo)
	if err != nil {
		t.Fatalf("failed reading emitted Go file: %v", err)
	}
	if !strings.Contains(string(emittedBytes), "package main") {
		t.Errorf("emitted Go file missing package main:\n%s", string(emittedBytes))
	}

	// 3. cb build -o <bin> and execute compiled binary
	binPath := filepath.Join(tmpDir, "fib_bin")
	if code := run([]string{"build", file02, "-o", binPath}); code != 0 {
		t.Fatalf("cb build returned %d, want 0", code)
	}
	outBytes, err := exec.Command(binPath).CombinedOutput()
	if err != nil {
		t.Fatalf("running built binary failed: %v\nOutput: %s", err, string(outBytes))
	}
	gotOut := strings.TrimSpace(strings.ReplaceAll(string(outBytes), "\r\n", "\n"))
	wantOut := strings.Join([]string{
		"Iterative Fib(10) = 55",
		"Recursive Fib(10) = 55",
		"While sum = 15",
	}, "\n")
	if gotOut != wantOut {
		t.Errorf("binary output mismatch:\ngot:\n%s\nwant:\n%s", gotOut, wantOut)
	}

	// 4. cb run <file.cb>
	if code := run([]string{"run", file01}); code != 0 {
		t.Fatalf("cb run %s returned %d, want 0", file01, code)
	}

	// 5. cb check on invalid program should return exit code 1
	badFile := filepath.Join(tmpDir, "bad.cb")
	badSrc := "fn main() as int\n    const x = 1\n    x = 2\n    return 0\nend fn\n"
	if err := os.WriteFile(badFile, []byte(badSrc), 0644); err != nil {
		t.Fatalf("failed to write bad.cb: %v", err)
	}
	if code := run([]string{"check", badFile}); code != 1 {
		t.Fatalf("cb check on invalid file returned %d, want 1", code)
	}
}

func TestCLIRunForwardsArgsAndStdin(t *testing.T) {
	tmpDir := t.TempDir()
	cbFile := filepath.Join(tmpDir, "echo_args.cb")
	src := `import "std/os"

fn main() as int
    print("args=" & str.join(os.args(), ","))
    try
        dim input = os.readStdin()
        print("stdin=" & input.trim())
    catch e
        print("err=" & e.msg)
        return 1
    end try
    return 0
end fn
`
	if err := os.WriteFile(cbFile, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write echo_args.cb: %v", err)
	}

	cbBin := filepath.Join(tmpDir, "cb")
	buildCmd := exec.Command("go", "build", "-o", cbBin, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build cb binary: %v\n%s", err, string(out))
	}

	// Test direct trailing args with stdin
	cmd1 := exec.Command(cbBin, "run", cbFile, "alpha", "-n", "beta")
	cmd1.Stdin = strings.NewReader("hello from stdin\n")
	out1, err := cmd1.CombinedOutput()
	if err != nil {
		t.Fatalf("cb run failed: %v\n%s", err, string(out1))
	}
	got1 := strings.TrimSpace(strings.ReplaceAll(string(out1), "\r\n", "\n"))
	want1 := "args=alpha,-n,beta\nstdin=hello from stdin"
	if got1 != want1 {
		t.Errorf("cb run output mismatch:\ngot:\n%s\nwant:\n%s", got1, want1)
	}

	// Test optional -- separator
	cmd2 := exec.Command(cbBin, "run", cbFile, "--", "--flag", "val")
	cmd2.Stdin = strings.NewReader("piped")
	out2, err := cmd2.CombinedOutput()
	if err != nil {
		t.Fatalf("cb run with -- failed: %v\n%s", err, string(out2))
	}
	got2 := strings.TrimSpace(strings.ReplaceAll(string(out2), "\r\n", "\n"))
	want2 := "args=--flag,val\nstdin=piped"
	if got2 != want2 {
		t.Errorf("cb run with -- output mismatch:\ngot:\n%s\nwant:\n%s", got2, want2)
	}
}

func TestCBGrepExample(t *testing.T) {
	cbgrepFile := filepath.Join("..", "..", "..", "examples", "cbgrep", "main.cb")

	// Verify cb check passes on examples/cbgrep/main.cb
	if code := run([]string{"check", cbgrepFile}); code != 0 {
		t.Fatalf("cb check %s returned %d, want 0", cbgrepFile, code)
	}

	tmpDir := t.TempDir()
	cbgrepBin := filepath.Join(tmpDir, "cbgrep")
	if code := run([]string{"build", cbgrepFile, "-o", cbgrepBin}); code != 0 {
		t.Fatalf("cb build %s returned %d, want 0", cbgrepFile, code)
	}

	cbBin := filepath.Join(tmpDir, "cb")
	if out, err := exec.Command("go", "build", "-o", cbBin, ".").CombinedOutput(); err != nil {
		t.Fatalf("failed to build cb binary: %v\n%s", err, string(out))
	}

	// Prepare fixture files and directories
	searchDir := filepath.Join(tmpDir, "fixtures")
	subDir := filepath.Join(searchDir, "sub")
	gitDir := filepath.Join(searchDir, ".git")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("mkdir subDir: %v", err)
	}
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("mkdir gitDir: %v", err)
	}

	alphaPath := filepath.Join(searchDir, "alpha.txt")
	betaPath := filepath.Join(subDir, "beta.txt")
	ignoredGitPath := filepath.Join(gitDir, "config")
	if err := os.WriteFile(alphaPath, []byte("First line\nHello CleanBasic 42\nAnother line\nHELLO world\n"), 0644); err != nil {
		t.Fatalf("write alpha.txt: %v", err)
	}
	if err := os.WriteFile(betaPath, []byte("nested match 99\nplain line\n"), 0644); err != nil {
		t.Fatalf("write beta.txt: %v", err)
	}
	if err := os.WriteFile(ignoredGitPath, []byte("hello from git\n"), 0644); err != nil {
		t.Fatalf("write .git/config: %v", err)
	}

	// (a) -i -n regex matching with 1-based line numbers
	outA, err := exec.Command(cbgrepBin, "-i", "-n", "hello.*[0-9]+", alphaPath).CombinedOutput()
	if err != nil {
		t.Fatalf("cbgrep -i -n failed: %v\n%s", err, string(outA))
	}
	gotA := strings.TrimSpace(strings.ReplaceAll(string(outA), "\r\n", "\n"))
	if gotA != "2:Hello CleanBasic 42" {
		t.Errorf("-i -n mismatch: got %q, want %q", gotA, "2:Hello CleanBasic 42")
	}

	// (b) -F -v -c fixed-string inverted match count (regex metacharacters treated literally)
	outB, err := exec.Command(cbgrepBin, "-F", "-v", "-c", "Hello CleanBasic 42", alphaPath).CombinedOutput()
	if err != nil {
		t.Fatalf("cbgrep -F -v -c failed: %v\n%s", err, string(outB))
	}
	gotB := strings.TrimSpace(strings.ReplaceAll(string(outB), "\r\n", "\n"))
	if gotB != "3" {
		t.Errorf("-F -v -c mismatch: got %q, want %q", gotB, "3")
	}

	// (c) -r -n recursive directory traversal (must skip .git)
	outC, err := exec.Command(cbgrepBin, "-r", "-n", "-i", "hello|nested", searchDir).CombinedOutput()
	if err != nil {
		t.Fatalf("cbgrep -r -n failed: %v\n%s", err, string(outC))
	}
	gotC := strings.TrimSpace(strings.ReplaceAll(string(outC), "\r\n", "\n"))
	if !strings.Contains(gotC, alphaPath+":2:Hello CleanBasic 42") ||
		!strings.Contains(gotC, alphaPath+":4:HELLO world") ||
		!strings.Contains(gotC, betaPath+":1:nested match 99") {
		t.Errorf("-r -n missing expected matches in output:\n%s", gotC)
	}
	if strings.Contains(gotC, ".git") {
		t.Errorf("-r -n should skip .git directory, got:\n%s", gotC)
	}

	// (d) stdin input piping via cb run examples/cbgrep/main.cb
	cmdD := exec.Command(cbBin, "run", cbgrepFile, "-i", "-n", "cleanbasic")
	cmdD.Stdin = strings.NewReader("one\ntwo CleanBasic\nthree\n")
	outD, err := cmdD.CombinedOutput()
	if err != nil {
		t.Fatalf("cb run cbgrep stdin failed: %v\n%s", err, string(outD))
	}
	gotD := strings.TrimSpace(strings.ReplaceAll(string(outD), "\r\n", "\n"))
	if gotD != "2:two CleanBasic" {
		t.Errorf("stdin piping mismatch: got %q, want %q", gotD, "2:two CleanBasic")
	}

	// (e) exit codes: 0 (--help), 1 (no match), 2 (invalid regex / missing file)
	if err := exec.Command(cbgrepBin, "--help").Run(); err != nil {
		t.Errorf("--help returned non-zero: %v", err)
	}

	noMatchCmd := exec.Command(cbgrepBin, "does_not_exist_anywhere", alphaPath)
	if err := noMatchCmd.Run(); err == nil || noMatchCmd.ProcessState.ExitCode() != 1 {
		t.Errorf("no-match exit code = %v, want 1", err)
	}

	badRegexCmd := exec.Command(cbgrepBin, "[unclosed", alphaPath)
	if err := badRegexCmd.Run(); err == nil || badRegexCmd.ProcessState.ExitCode() != 2 {
		t.Errorf("invalid regex exit code = %v, want 2", err)
	}

	missingFileCmd := exec.Command(cbgrepBin, "hello", filepath.Join(tmpDir, "missing.txt"))
	if err := missingFileCmd.Run(); err == nil || missingFileCmd.ProcessState.ExitCode() != 2 {
		t.Errorf("missing file exit code = %v, want 2", err)
	}
}
