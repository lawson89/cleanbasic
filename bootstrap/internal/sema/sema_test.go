package sema

import (
	"path/filepath"
	"strings"
	"testing"

	"cleanbasic/bootstrap/internal/ast"
)

func TestAnalyzeCase01AndCase02(t *testing.T) {
	cases := []string{
		filepath.Join("..", "..", "..", "tests", "cases", "01_hello_and_math.cb"),
		filepath.Join("..", "..", "..", "tests", "cases", "02_fibonacci.cb"),
	}

	for _, tc := range cases {
		t.Run(filepath.Base(tc), func(t *testing.T) {
			prog, err := LoadAndAnalyze(tc)
			if err != nil {
				t.Fatalf("LoadAndAnalyze(%s) failed: %v", tc, err)
			}
			if prog == nil || prog.Main == nil {
				t.Fatalf("LoadAndAnalyze(%s) returned nil program", tc)
			}
			if _, ok := prog.Functions["main"]; !ok {
				t.Errorf("expected main function in Functions table for %s", tc)
			}
		})
	}
}

func TestBottomUpTypeInference(t *testing.T) {
	src := `
const GLOBAL_OFFSET = 100

fn compute(a as int, ratio as float) as str
    dim sum = a + GLOBAL_OFFSET
    dim scaled = ratio * 2.5
    dim ok = (sum > 10) and not false
    if ok
        return "sum=" & sum & ", scaled=" & scaled
    else
        return "none"
    end if
end fn

fn main() as int
    dim res = compute(5, 1.5)
    print(res)
    return 0
end fn
`
	prog, err := AnalyzeSource(src, "infer_test.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}

	// Verify inferred types for local VarDecls inside compute
	fnDecl := prog.Functions["compute"].Decl
	expectedVars := map[string]Type{
		"sum":    IntType,
		"scaled": FloatType,
		"ok":     BoolType,
	}

	for _, st := range fnDecl.Body {
		if vd, ok := st.(*ast.VarDecl); ok {
			gotType, exists := prog.VarTypes[vd]
			if !exists {
				t.Errorf("missing inferred type for variable %q", vd.Name)
				continue
			}
			if wantType, wantOk := expectedVars[vd.Name]; wantOk && !gotType.Equals(wantType) {
				t.Errorf("variable %q inferred type = %s, want %s", vd.Name, gotType, wantType)
			}
		}
	}
}

func TestSemanticErrors(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantErr string
	}{
		{
			name: "reassign local const",
			src: `
fn main() as int
    const x = 10
    x = 20
    return 0
end fn
`,
			wantErr: `test.cb:4:5: semantic error: cannot reassign constant "x"`,
		},
		{
			name: "reassign top-level const",
			src: `
const MAX_LIMIT = 50
fn main() as int
    MAX_LIMIT = 60
    return 0
end fn
`,
			wantErr: `test.cb:4:5: semantic error: cannot reassign constant "MAX_LIMIT"`,
		},
		{
			name: "call undefined function",
			src: `
fn main() as int
    dim x = unknownFunc(1, 2)
    return x
end fn
`,
			wantErr: `test.cb:3:13: semantic error: undefined function "unknownFunc"`,
		},
		{
			name: "return wrong type",
			src: `
fn getCount() as int
    return "not an int"
end fn
fn main() as int
    return getCount()
end fn
`,
			wantErr: `test.cb:3:5: semantic error: return type mismatch in function "getCount": expected int, got str`,
		},
		{
			name: "missing return statement",
			src: `
fn getCount(x as int) as int
    if x > 0
        return x
    end if
end fn
`,
			wantErr: `missing a return statement on all control paths`,
		},
		{
			name: "binary operator type mismatch",
			src: `
fn main() as int
    dim x = 10 + 2.5
    return 0
end fn
`,
			wantErr: `type mismatch for operator "+": int and float`,
		},
		{
			name: "if condition not bool",
			src: `
fn main() as int
    if 123
        return 1
    end if
    return 0
end fn
`,
			wantErr: `if condition must be bool, got int`,
		},
		{
			name: "function argument type mismatch",
			src: `
fn add(a as int, b as int) as int
    return a + b
end fn
fn main() as int
    return add(1, "two")
end fn
`,
			wantErr: `argument 2 ("b") to function "add" has type str, expected int`,
		},
		{
			name: "break outside loop",
			src: `
fn main() as int
    break
    return 0
end fn
`,
			wantErr: `break statement is not inside a loop`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := AnalyzeSource(tc.src, "test.cb")
			if err == nil {
				t.Fatalf("expected semantic error containing %q, but got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}
