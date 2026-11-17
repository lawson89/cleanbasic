package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cleanbasic/bootstrap/internal/ast"
)

func TestParseAllCaseFixtures(t *testing.T) {
	casesDir := filepath.Join("..", "..", "..", "tests", "cases")
	count := 0
	err := filepath.WalkDir(casesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "errors" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".cb") {
			return nil
		}
		count++
		rel, _ := filepath.Rel(casesDir, path)
		relSlash := filepath.ToSlash(rel)
		t.Run(relSlash, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}
			prog, err := Parse(string(data), path)
			if err != nil {
				t.Fatalf("Parse(%s) failed: %v", relSlash, err)
			}
			if prog == nil || len(prog.Decls) == 0 {
				t.Fatalf("expected non-empty AST declarations for %s", relSlash)
			}
			if !strings.Contains(relSlash, "/") || d.Name() == "main.cb" {
				hasMain := false
				for _, decl := range prog.Decls {
					if fn, ok := decl.(*ast.FnDecl); ok && fn.Name == "main" {
						hasMain = true
						break
					}
				}
				if !hasMain {
					t.Errorf("expected AST for %s to contain FnDecl main", relSlash)
				}
			}
		})
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk %s: %v", casesDir, err)
	}
	if count < 11 {
		t.Fatalf("expected at least 11 .cb test cases (01 through 11), found %d", count)
	}
}

func TestParseRejectsImportHeader(t *testing.T) {
	if _, err := Parse("import header \"foo.h\"\n", "header.cb"); err == nil {
		t.Fatalf("expected parse error for removed import header, got nil")
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
	gotExpr := formatTestExpr(varDecl.Value)
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
			name:    "as binding in select case",
			src:     "fn main()\n    select case x\n        case 1 as v\n            print(v)\n    end select\nend fn\n",
			wantSub: "expected newline after case clause",
		},
		{
			name:    "empty list literal rejected",
			src:     "fn main()\n    dim xs = []\nend fn\n",
			wantSub: "empty list literal '[]' is not allowed",
		},
		{
			name:    "empty dict literal rejected",
			src:     "fn main()\n    dim d = {}\nend fn\n",
			wantSub: "empty dict literal '{}' is not allowed",
		},
		{
			name:    "local const rejected",
			src:     "fn main()\n    const x = 1\nend fn\n",
			wantSub: "unexpected token const",
		},
		{
			name:    "for step rejected",
			src:     "fn main()\n    for i = 1 to 10 step 2\n    end for\nend fn\n",
			wantSub: "expected newline after for loop header",
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

func TestParserTier1GoInteropByteAndSlicing(t *testing.T) {
	src := `go import "os" as stdos
go
func helper() int64 { return 1 }
end go
public type Opaque = go "CbOpaque"
public fn readIt(p as str) as list[byte] can error = go "stdos.ReadFile"
fn main() as int
    dim a = s[1:3]
    dim b = s[:2]
    dim c = s[2:]
    dim d = s[:]
    return 0
end fn
`

	prog, err := Parse(src, "tier1.cb")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(prog.Decls) != 5 {
		t.Fatalf("expected 5 decls, got %d", len(prog.Decls))
	}
	gi, ok := prog.Decls[0].(*ast.GoImportDecl)
	if !ok || gi.Path != "os" || gi.Alias != "stdos" {
		t.Fatalf("unexpected GoImportDecl: %#v", prog.Decls[0])
	}
	if _, ok := prog.Decls[1].(*ast.GoBlockDecl); !ok {
		t.Fatalf("expected GoBlockDecl, got %T", prog.Decls[1])
	}
	td, ok := prog.Decls[2].(*ast.TypeDecl)
	if !ok || !td.Public || td.Name != "Opaque" || td.GoName != "CbOpaque" {
		t.Fatalf("unexpected TypeDecl: %#v", prog.Decls[2])
	}
	fnRead, ok := prog.Decls[3].(*ast.FnDecl)
	if !ok || !fnRead.Public || fnRead.Name != "readIt" || !fnRead.CanError || fnRead.GoTarget != "stdos.ReadFile" || fnRead.ReturnType == nil || fnRead.ReturnType.String() != "list[byte]" {
		t.Fatalf("unexpected FnDecl readIt: %#v", prog.Decls[3])
	}
	fnMain, ok := prog.Decls[4].(*ast.FnDecl)
	if !ok || len(fnMain.Body) != 5 {
		t.Fatalf("unexpected main FnDecl: %#v", prog.Decls[4])
	}
	wantSlices := [][2]bool{{true, true}, {false, true}, {true, false}, {false, false}}
	for i, want := range wantSlices {
		vd, ok := fnMain.Body[i].(*ast.VarDecl)
		if !ok {
			t.Fatalf("stmt %d: expected VarDecl, got %T", i, fnMain.Body[i])
		}
		sl, ok := vd.Value.(*ast.SliceExpr)
		if !ok || (sl.Start != nil) != want[0] || (sl.End != nil) != want[1] {
			t.Fatalf("stmt %d: unexpected SliceExpr: %#v", i, vd.Value)
		}
	}
}

func TestParseMailboxAndStartJob(t *testing.T) {
	src := strings.Join([]string{
		"fn worker(box as mailbox[int]) as int",
		"    return 1",
		"end fn",
		"",
		"fn main()",
		"    dim box1 = mailbox[int]()",
		"    dim box2 = mailbox[str](16)",
		"    start job worker(box1)",
		"end fn",
	}, "\n")
	prog, err := Parse(src, "concurrency.cb")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(prog.Decls) != 2 {
		t.Fatalf("got %d decls, want 2", len(prog.Decls))
	}
	mainFn, ok := prog.Decls[1].(*ast.FnDecl)
	if !ok {
		t.Fatalf("expected *ast.FnDecl, got %T", prog.Decls[1])
	}
	if len(mainFn.Body) != 3 {
		t.Fatalf("got %d statements in main, want 3", len(mainFn.Body))
	}
	if _, ok := mainFn.Body[2].(*ast.StartJobStmt); !ok {
		t.Fatalf("expected *ast.StartJobStmt, got %T", mainFn.Body[2])
	}

	for _, bad := range []string{
		"fn main()\n    dim j = start job worker(box1)\nend fn\n",
		"fn main()\n    start job 42\nend fn\n",
	} {
		if _, err := Parse(bad, "bad_start_job.cb"); err == nil {
			t.Errorf("expected parse error for %q, got nil", bad)
		}
	}
}

func formatTestExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.IntLitExpr:
		return e.Raw
	case *ast.BoolLitExpr:
		if e.Value {
			return "true"
		}
		return "false"
	case *ast.UnaryExpr:
		return "(" + string(e.Op) + " " + formatTestExpr(e.Operand) + ")"
	case *ast.BinaryExpr:
		return "(" + formatTestExpr(e.Left) + " " + string(e.Op) + " " + formatTestExpr(e.Right) + ")"
	default:
		return "<expr>"
	}
}

func TestParseByteLiterals(t *testing.T) {
	src := "const CR = '\\r'\nfn main() as int\n    dim ch = 'a'\n    dim esc = '\\''\n    return 0\nend fn\n"
	prog, err := Parse(src, "byte_lit.cb")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(prog.Decls) != 2 {
		t.Fatalf("got %d decls, want 2", len(prog.Decls))
	}
	cDecl, ok := prog.Decls[0].(*ast.ConstDecl)
	if !ok {
		t.Fatalf("expected ConstDecl, got %T", prog.Decls[0])
	}
	bl, ok := cDecl.Value.(*ast.ByteLitExpr)
	if !ok || bl.Value != '\r' {
		t.Fatalf("expected ByteLitExpr('\r'), got %#v", cDecl.Value)
	}
}

func TestParseRejectsTypeKeywordsAsExprIdents(t *testing.T) {
	badSources := []string{
		`fn main()
    dim s = str.join(["a"], ",")
end fn
`,
		"fn main()\n    dim x = list\nend fn\n",
		"fn main()\n    dim x = dict\nend fn\n",
		"fn main()\n    dim x = mailbox\nend fn\n",
		"fn main()\n    dim x = obj.str\nend fn\n",
	}
	for _, src := range badSources {
		if _, err := Parse(src, "bad_kw_expr.cb"); err == nil {
			t.Errorf("expected syntax error for %q, got nil", src)
		}
	}
}

func TestParseTopLevelAndZeroInitVarDecl(t *testing.T) {
	src := strings.Join([]string{
		"dim gCount = 1",
		"public dim gName as str = \"cb\"",
		"dim gItems as list[str]",
		"public dim gCache as dict[str, int]",
		"fn main() as int",
		"    dim x as int",
		"    dim ch as mailbox[int]",
		"    return x",
		"end fn",
	}, "\n")
	prog, err := Parse(src, "top_var.cb")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(prog.Decls) != 5 {
		t.Fatalf("got %d decls, want 5", len(prog.Decls))
	}
	v0, ok := prog.Decls[0].(*ast.VarDecl)
	if !ok || v0.Public || v0.Name != "gCount" || v0.Value == nil {
		t.Fatalf("unexpected decl[0]: %#v", prog.Decls[0])
	}
	v1, ok := prog.Decls[1].(*ast.VarDecl)
	if !ok || !v1.Public || v1.Name != "gName" || v1.Type == nil || v1.Value == nil {
		t.Fatalf("unexpected decl[1]: %#v", prog.Decls[1])
	}
	v2, ok := prog.Decls[2].(*ast.VarDecl)
	if !ok || v2.Public || v2.Type == nil || v2.Value != nil {
		t.Fatalf("unexpected decl[2]: %#v", prog.Decls[2])
	}
	v3, ok := prog.Decls[3].(*ast.VarDecl)
	if !ok || !v3.Public || v3.Type == nil || v3.Value != nil {
		t.Fatalf("unexpected decl[3]: %#v", prog.Decls[3])
	}

	if _, err := Parse("fn main()\n    dim bad\nend fn\n", "bad_var.cb"); err == nil {
		t.Fatalf("expected parse error for 'dim bad' without type or initializer")
	}
}
