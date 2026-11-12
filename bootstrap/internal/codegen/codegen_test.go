package codegen

import (
	"fmt"
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
