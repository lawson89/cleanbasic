package codegen

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cleanbasic/bootstrap/internal/sema"
)

func TestEmitGoPrimitiveMappingAndLowering(t *testing.T) {
	src := strings.Join([]string{
		"const BASE = 10",
		"",
		"fn add(a as int, b as int) as int",
		"    dim unusedLocal = 42",
		"    return a + b + BASE",
		"end fn",
		"",
		"fn main() as int",
		"    dim res = add(3, 4)",
		"    dim ratio = 1.5",
		"    dim ok = true",
		fmt.Sprintf("    dim greeting = %q & res", "res="),
		fmt.Sprintf("    print(greeting, %q, ratio, %q, ok)", " ratio=", " ok="),
		"    return 0",
		"end fn",
	}, "\n")

	prog, err := sema.AnalyzeSource(src, "codegen_test.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}

	goCode, err := EmitGo(prog)
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}

	expectedSnippets := []string{
		"package main",
		fmt.Sprintf("%q", "fmt"),
		fmt.Sprintf("%q", "os"),
		"func add(a int64, b int64) int64",
		"_ = unusedLocal",
		"var ratio float64 = float64(1.5)",
		"var ok bool = true",
		fmt.Sprintf("fmt.Sprint(%q) + fmt.Sprint(res)", "res="),
		fmt.Sprintf("fmt.Println(fmt.Sprint(greeting) + fmt.Sprint(%q) + fmt.Sprint(ratio) + fmt.Sprint(%q) + fmt.Sprint(ok))", " ratio=", " ok="),
		"func cb_main() int64",
		"os.Exit(int(code))",
	}

	for _, want := range expectedSnippets {
		if !strings.Contains(goCode, want) {
			t.Errorf("generated Go code did not contain %q\n--- GENERATED GO ---\n%s", want, goCode)
		}
	}
}

func TestEmitGoOmitsUnusedFmtImport(t *testing.T) {
	src := strings.Join([]string{
		"fn main() as int",
		"    dim x = 1 + 2",
		"    return x - 3",
		"end fn",
	}, "\n")

	prog, err := sema.AnalyzeSource(src, "nofmt.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}

	goCode, err := EmitGo(prog)
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}

	if strings.Contains(goCode, fmt.Sprintf("%q", "fmt")) {
		t.Errorf("expected fmt import to be omitted when unused, got:\n%s", goCode)
	}
	if !strings.Contains(goCode, fmt.Sprintf("%q", "os")) {
		t.Errorf("expected os import for main() as int exit handling, got:\n%s", goCode)
	}
}

func TestEmitGoPhase3Cases(t *testing.T) {
	cases := []string{
		"../../../tests/cases/03_structs_and_ufcs.cb",
		"../../../tests/cases/04_containers_and_strings.cb",
		"../../../tests/cases/05_enums_and_unions.cb",
		"../../../tests/cases/06_errors_and_defer.cb",
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			prog, err := sema.LoadAndAnalyze(path)
			if err != nil {
				t.Fatalf("LoadAndAnalyze(%s) failed: %v", path, err)
			}
			goCode, err := EmitGo(prog)
			if err != nil {
				t.Fatalf("EmitGo(%s) failed: %v", path, err)
			}
			if !strings.Contains(goCode, "package main") {
				t.Errorf("expected package main in generated Go for %s", path)
			}
		})
	}
}

func TestEmitGoDeferLIFO(t *testing.T) {
	src := strings.Join([]string{
		"fn work()",
		"    defer",
		fmt.Sprintf("        print(%q)", "first"),
		"    end defer",
		"    defer",
		fmt.Sprintf("        print(%q)", "second"),
		"    end defer",
		"    defer",
		fmt.Sprintf("        print(%q)", "third"),
		"    end defer",
		fmt.Sprintf("    print(%q)", "body"),
		"end fn",
		"",
		"fn main() as int",
		"    work()",
		"    return 0",
		"end fn",
	}, "\n")

	prog, err := sema.AnalyzeSource(src, "defer_lifo.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	goCode, err := EmitGo(prog)
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}

	tmpDir := t.TempDir()
	outGoPath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(outGoPath, []byte(goCode), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cmd := exec.Command("go", "run", outGoPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go run failed: %v\nstderr:\n%s\ngo:\n%s", err, stderr.String(), goCode)
	}

	got := strings.TrimRight(strings.ReplaceAll(stdout.String(), "\r\n", "\n"), "\n")
	want := "body\nthird\nsecond\nfirst"
	if got != want {
		t.Errorf("defer LIFO output = %q, want %q", got, want)
	}
}

func TestEmitGoPhase4EdgeCases(t *testing.T) {
	src := strings.Join([]string{
		"fn mightFail(n as int) as int can error",
		"    if n < 0",
		fmt.Sprintf("        error(%q, code=n)", "negative"),
		"    end if",
		"    return n * 10",
		"end fn",
		"",
		"fn nonVoidEndingInTry(n as int) as int",
		"    try",
		"        return mightFail(n)",
		"    catch e",
		"        return e.code",
		"    end try",
		"end fn",
		"",
		"fn checkElseIf(n as int) as int can error",
		"    if n == 0",
		"        return 0",
		"    else if mightFail(n) > 20",
		"        return 1",
		"    else",
		"        return 2",
		"    end if",
		"end fn",
		"",
		"fn loopAndSelect(limit as int) as int can error",
		"    dim sum = 0",
		"    dim i = 1",
		"    while mightFail(i) <= limit * 10",
		"        sum = sum + i",
		"        i = i + 1",
		"    end while",
		"    for j = 1 to mightFail(1) / 5",
		"        sum = sum + j",
		"    end for",
		"    select case mightFail(2)",
		"        case 20",
		"            sum = sum + 100",
		"        case else",
		"            sum = sum + 1",
		"    end select",
		"    return sum",
		"end fn",
		"",
		"fn main() as int",
		"    print(nonVoidEndingInTry(3))",
		"    print(nonVoidEndingInTry(-7))",
		"    try",
		"        print(checkElseIf(0))",
		"        print(checkElseIf(3))",
		"        print(checkElseIf(1))",
		"        print(loopAndSelect(2))",
		"        print(checkElseIf(-1))",
		"    catch e",
		"        print(e.msg)",
		"        print(e.code)",
		"    end try",
		"    return 0",
		"end fn",
	}, "\n")

	prog, err := sema.AnalyzeSource(src, "phase4_edges.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	goCode, err := EmitGo(prog)
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}

	tmpDir := t.TempDir()
	outGoPath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(outGoPath, []byte(goCode), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cmd := exec.Command("go", "run", outGoPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go run failed: %v\nstderr:\n%s\ngo:\n%s", err, stderr.String(), goCode)
	}

	got := strings.TrimRight(strings.ReplaceAll(stdout.String(), "\r\n", "\n"), "\n")
	want := "30\n-7\n0\n1\n2\n106\nnegative\n-1"
	if got != want {
		t.Errorf("phase4 edge cases output = %q, want %q\ngo:\n%s", got, want, goCode)
	}
}

func TestEmitGoPhase5MultiFileAndStdlib(t *testing.T) {
	t.Run("multi-file mangling", func(t *testing.T) {
		prog, err := sema.LoadAndAnalyze("../../../tests/cases/07_multi_file_modules/main.cb")
		if err != nil {
			t.Fatalf("LoadAndAnalyze failed: %v", err)
		}
		goCode, err := EmitGo(prog)
		if err != nil {
			t.Fatalf("EmitGo failed: %v", err)
		}
		for _, want := range []string{
			"cb_entities_hero__Stats",
			"cb_combat_actions__Stats",
			"cb_entities_hero__heal",
			"cb_combat_actions__strike",
		} {
			if !strings.Contains(goCode, want) {
				t.Errorf("expected mangled symbol %q in generated Go:\n%s", want, goCode)
			}
		}
	})

	t.Run("stdlib helpers on demand", func(t *testing.T) {
		prog, err := sema.LoadAndAnalyze("../../../tests/cases/08_stdlib_fs_os_cli.cb")
		if err != nil {
			t.Fatalf("LoadAndAnalyze failed: %v", err)
		}
		goCode, err := EmitGo(prog)
		if err != nil {
			t.Fatalf("EmitGo failed: %v", err)
		}
		for _, want := range []string{
			"cbStdFsReadFile",
			"cbStdOsExec",
			"cbStdJsonParse",
			"cbStdCliNewParser",
		} {
			if !strings.Contains(goCode, want) {
				t.Errorf("expected stdlib helper %q in generated Go", want)
			}
		}
	})

	t.Run("can error returning str and standalone cli parseArgs", func(t *testing.T) {
		src := `import "std/fs"
import "std/cli"

fn readOrFail(path as str, failNow as bool) as str can error
	if failNow
		error("forced failure")
	end if
	return fs.readFile(path)
end fn

fn main()
	try
		dim s as str = readOrFail("/nonexistent/path/for/test.txt", true)
		print(s)
	catch errVal
		print(errVal.msg)
	end try

	dim p = cli.newParser("app", "1.0", "desc")
	p.addFlag("verbose", "v", "verbose")
	try
		p.parseArgs(["--verbose"])
		print("parsed-ok")
	catch e
		print(e.msg)
	end try
end fn
`
		prog, err := sema.AnalyzeSource(src, "fallible_str_cli.cb")
		if err != nil {
			t.Fatalf("AnalyzeSource failed: %v", err)
		}
		goCode, err := EmitGo(prog)
		if err != nil {
			t.Fatalf("EmitGo failed: %v", err)
		}
		tmpDir := t.TempDir()
		outGoPath := filepath.Join(tmpDir, "main.go")
		if err := os.WriteFile(outGoPath, []byte(goCode), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command("go", "run", outGoPath)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("go run failed: %v\nstderr:\n%s\ngo:\n%s", err, stderr.String(), goCode)
		}
		got := strings.TrimRight(strings.ReplaceAll(stdout.String(), "\r\n", "\n"), "\n")
		want := "forced failure\nparsed-ok"
		if got != want {
			t.Errorf("output = %q, want %q\ngo:\n%s", got, want, goCode)
		}
	})
}
