package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cleanbasic/bootstrap/internal/ast"
)

func TestParseAllTestCases(t *testing.T) {
	casesDir := filepath.Join("..", "..", "..", "tests", "cases")
	entries, err := os.ReadDir(casesDir)
	if err != nil {
		t.Fatalf("failed to read %s: %v", casesDir, err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".cb") {
			continue
		}
		count++
		path := filepath.Join(casesDir, entry.Name())
		t.Run(entry.Name(), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}
			// Ignore ' SKIP_GO directive during parse tests so all 6 test files are verified.
			prog, err := Parse(string(data), path)
			if err != nil {
				t.Fatalf("Parse(%s) failed: %v", entry.Name(), err)
			}
			if prog == nil || len(prog.Decls) == 0 {
				t.Fatalf("expected non-empty AST declarations for %s", entry.Name())
			}
			dump := ast.Dump(prog)
			if !strings.Contains(dump, "FnDecl(main()") {
				t.Errorf("expected AST dump for %s to contain FnDecl(main()), got:\n%s", entry.Name(), dump)
			}
		})
	}
	if count < 6 {
		t.Fatalf("expected at least 6 .cb test cases, found %d", count)
	}
}

func TestOperatorPrecedenceAndTrailingCommas(t *testing.T) {
	src := strings.Join([]string{
		"import \"entities/hero.cb\" as hero",
		"",
		"public fn compute(",
		"    byref xs as list[int],",
		"    d as dict[str, int],",
		") as bool",
		"    dim a = 1 + 2 * 3 == 7 and not false or true",
		"    dim nums = [1, 2, 3,]",
		"    dim scores = {\"k1\": 10, \"k2\": 20,}",
		"    dim emptyList = list[str]()",
		"    dim emptyDict = dict[str, int]()",
		"    print(a, nums[0], scores[\"k1\"],)",
		"    return a",
		"end fn",
	}, "\n")
	prog, err := Parse(src, "prec.cb")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(prog.Decls) != 2 {
		t.Fatalf("expected 2 top-level decls, got %d", len(prog.Decls))
	}

	fnDecl, ok := prog.Decls[1].(*ast.FnDecl)
	if !ok {
		t.Fatalf("expected FnDecl, got %T", prog.Decls[1])
	}
	if len(fnDecl.Params) != 2 {
		t.Fatalf("expected 2 params with trailing comma, got %d", len(fnDecl.Params))
	}

	varDecl, ok := fnDecl.Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", fnDecl.Body[0])
	}
	gotExpr := ast.FormatExpr(varDecl.Value)
	wantExpr := "((((1 + (2 * 3)) == 7) and (not false)) or true)"
	if gotExpr != wantExpr {
		t.Errorf("precedence mismatch:\ngot:  %s\nwant: %s", gotExpr, wantExpr)
	}
}

func TestSyntaxErrorReporting(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantSub string
	}{
		{
			name:    "missing end fn",
			src:     "fn main() as int\n    return 0\n",
			wantSub: "expected end fn",
		},
		{
			name:    "invalid top level token",
			src:     "return 0\n",
			wantSub: "unexpected token return",
		},
		{
			name:    "empty union",
			src:     "union Empty\nend union\n",
			wantSub: "must declare at least one variant",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.src, "err.cb")
			if err == nil {
				t.Fatalf("expected parse error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q did not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}
