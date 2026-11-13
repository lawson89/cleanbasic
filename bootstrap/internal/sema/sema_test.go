package sema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cleanbasic/bootstrap/internal/ast"
)

func TestAnalyzeCase01AndCase02(t *testing.T) {
	cases := []string{
		filepath.Join("..", "..", "..", "tests", "cases", "01_hello_and_math.cb"),
		filepath.Join("..", "..", "..", "tests", "cases", "02_fibonacci.cb"),
		filepath.Join("..", "..", "..", "tests", "cases", "03_structs_and_ufcs.cb"),
		filepath.Join("..", "..", "..", "tests", "cases", "04_containers_and_strings.cb"),
		filepath.Join("..", "..", "..", "tests", "cases", "05_enums_and_unions.cb"),
		filepath.Join("..", "..", "..", "tests", "cases", "06_errors_and_defer.cb"),
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
		{
			name: "pass const to byref parameter",
			src: `
fn inc(byref x as int)
    x = x + 1
end fn
fn main() as int
    const c = 10
    inc(c)
    return 0
end fn
`,
			wantErr: `cannot pass constant "c" to byref parameter`,
		},
		{
			name: "pass literal to byref UFCS method",
			src: `
type Box
    v as int
end type
fn bump(byref b as Box)
    b.v = b.v + 1
end fn
fn main() as int
    Box(1).bump()
    return 0
end fn
`,
			wantErr: `target expression is not mutable`,
		},
		{
			name: "mutate field of const struct",
			src: `
type Box
    v as int
end type
fn main() as int
    const b = Box(1)
    b.v = 2
    return 0
end fn
`,
			wantErr: `cannot mutate constant "b"`,
		},
		{
			name: "untyped empty list literal",
			src: `
fn main() as int
    dim xs = []
    return 0
end fn
`,
			wantErr: `cannot infer type of empty list literal []`,
		},
		{
			name: "untyped empty dict literal",
			src: `
fn main() as int
    dim d = {}
    return 0
end fn
`,
			wantErr: `cannot infer type of empty dict literal {}`,
		},
		{
			name: "unknown named field in struct constructor",
			src: `
type Box
    v as int
end type
fn main() as int
    dim b = Box(nope=1)
    return 0
end fn
`,
			wantErr: `struct "Box" has no field "nope"`,
		},
		{
			name: "non-existent enum variant",
			src: `
enum Color
    Red
    Blue
end enum
fn main() as int
    dim c = Color.Green
    return 0
end fn
`,
			wantErr: `enum "Color" has no variant "Green"`,
		},
		{
			name: "non-existent union variant",
			src: `
union Val
    Num as int
end union
fn main() as int
    dim v = Val.Text("hi")
    return 0
end fn
`,
			wantErr: `union "Val" has no variant "Text"`,
		},
		{
			name: "invalid select case binding on non-union",
			src: `
fn main() as int
    dim x = 1
    select case x
        case 1 as y
            print(y)
    end select
    return 0
end fn
`,
			wantErr: `'as' binding in select case is only supported for union types`,
		},
		{
			name: "invalid select case pattern type",
			src: `
fn main() as int
    dim x = 1
    select case x
        case "one"
            print(1)
    end select
    return 0
end fn
`,
			wantErr: `case pattern type str does not match select expression type int`,
		},
		{
			name: "call can error function outside try in non-fallible function",
			src: `
fn risky() as int can error
    return 1
end fn
fn main() as int
    dim x = risky()
    return x
end fn
`,
			wantErr: `call to 'can error' function "risky" must be inside a 'try' block or a 'can error' function`,
		},
		{
			name: "call error() outside try in non-fallible function",
			src: `
fn main() as int
    error("boom")
    return 0
end fn
`,
			wantErr: `error() can only be called inside a 'can error' function or a 'try' block`,
		},
		{
			name: "return inside defer block",
			src: `
fn main() as int
    defer
        return 1
    end defer
    return 0
end fn
`,
			wantErr: `return statement is not allowed inside a defer block`,
		},
		{
			name: "unhandled fallible call inside defer block",
			src: `
fn cleanup() can error
    error("cleanup failed")
end fn
fn helper() can error
    defer
        cleanup()
    end defer
end fn
fn main() as int
    try
        helper()
    catch e
        print(e.msg)
    end try
    return 0
end fn
`,
			wantErr: `call to 'can error' function "cleanup" must be inside a 'try' block or a 'can error' function`,
		},
		{
			name: "main cannot be declared can error",
			src: `
fn main() as int can error
    return 0
end fn
`,
			wantErr: `function "main" cannot be declared 'can error'`,
		},
		{
			name: "mutate catch error binding",
			src: `
fn risky() can error
    error("fail")
end fn
fn main() as int
    try
        risky()
    catch e
        e.msg = "mutated"
    end try
    return 0
end fn
`,
			wantErr: `cannot mutate constant "e"`,
		},
		{
			name: "access invalid field on err",
			src: `
fn risky() can error
    error("fail")
end fn
fn main() as int
    try
        risky()
    catch e
        print(e.nope)
    end try
    return 0
end fn
`,
			wantErr: `err type has no field "nope"`,
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
func TestMultiFileImportErrors(t *testing.T) {
	writeFiles := func(t *testing.T, files map[string]string) string {
		t.Helper()
		dir := t.TempDir()
		for rel, content := range files {
			full := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
				t.Fatalf("mkdir failed: %v", err)
			}
			if err := os.WriteFile(full, []byte(content), 0644); err != nil {
				t.Fatalf("write failed: %v", err)
			}
		}
		return filepath.Join(dir, "main.cb")
	}

	tests := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			name: "circular import",
			files: map[string]string{
				"main.cb": `import "a.cb"
fn main() as int
    return 0
end fn
`,
				"a.cb": `import "b.cb"
`,
				"b.cb": `import "a.cb"
`,
			},
			wantErr: "circular import detected",
		},
		{
			name: "missing imported file",
			files: map[string]string{
				"main.cb": `import "missing.cb"
fn main() as int
    return 0
end fn
`,
			},
			wantErr: `cannot import "missing.cb"`,
		},
		{
			name: "duplicate module alias",
			files: map[string]string{
				"main.cb": `import "a.cb" as mod
import "b.cb" as mod
fn main() as int
    return 0
end fn
`,
				"a.cb": "public const X = 1\n",
				"b.cb": "public const Y = 2\n",
			},
			wantErr: `duplicate import alias "mod"`,
		},
		{
			name: "access private function from imported module",
			files: map[string]string{
				"main.cb": `import "helper.cb"
fn main() as int
    helper.secret()
    return 0
end fn
`,
				"helper.cb": "fn secret()\nend fn\n",
			},
			wantErr: `function "secret" in module "helper" is private`,
		},
		{
			name: "access private struct from imported module",
			files: map[string]string{
				"main.cb": `import "helper.cb"
fn main() as int
    dim x = helper.Hidden(1)
    return 0
end fn
`,
				"helper.cb": "type Hidden\n    v as int\nend type\n",
			},
			wantErr: `type "Hidden" in module "helper" is private`,
		},
		{
			name: "access private const from imported module",
			files: map[string]string{
				"main.cb": `import "helper.cb"
fn main() as int
    print(helper.SECRET)
    return 0
end fn
`,
				"helper.cb": "const SECRET = 42\n",
			},
			wantErr: `constant "SECRET" in module "helper" is private`,
		},
		{
			name: "cross-module UFCS rejected",
			files: map[string]string{
				"main.cb": `import "helper.cb"
fn main() as int
    dim b = helper.Box(1)
    b.bump()
    return 0
end fn
`,
				"helper.cb": `public type Box
    v as int
end type
public fn bump(byref b as Box)
    b.v = b.v + 1
end fn
`,
			},
			wantErr: `no method or UFCS function "bump" for receiver type Box`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := writeFiles(t, tc.files)
			_, err := LoadAndAnalyze(entry)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestStdRegexNewStdlibAndStrMethods(t *testing.T) {
	src := `import "std/fs"
import "std/os"
import "std/regex"

fn main() as int
    dim isD as bool = fs.isDir(".")
    dim joined as str = fs.join("a", "b.cb")
    dim baseName as str = fs.base(joined)
    dim dirName as str = fs.dir(joined)
    dim extName as str = fs.ext(joined)

    dim okStart as bool = baseName.startsWith("b")
    dim okEnd as bool = baseName.endsWith(".cb")
    dim idx as int = baseName.indexOf(".cb")
    dim rep as str = baseName.replace(".cb", ".go")

    if not isD or not okStart or not okEnd or idx < 0 or rep == "" or dirName == "" or extName == ""
        return 1
    end if

    try
        dim cwd as str = os.cwd()
        dim stdinText as str = os.readStdin()
        dim re as regex.Regex = regex.compile("[0-9]+")
        dim m1 as bool = re.isMatch("abc123")
        dim m2 as bool = regex.match("[a-z]+", "abc")
        dim f1 as str = re.find("a12b34")
        dim fAll as list[str] = re.findAll("a12b34")
        dim rAll as str = re.replaceAll("a12b34", "#")
        if cwd == "" and stdinText == "" and m1 and m2 and f1 == "" and fAll.len() == 0 and rAll == ""
            return 0
        end if
    catch e
        print(e.msg)
    end try
    return 0
end fn
`
	if _, err := AnalyzeSource(src, "valid_new_stdlib.cb"); err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}

	errCases := []struct {
		name    string
		src     string
		wantErr string
	}{
		{
			name: "regex.compile outside fallible context",
			src: `import "std/regex"
fn main()
    dim re = regex.compile("[0-9]+")
end fn
`,
			wantErr: "must be inside a 'try' block or a 'can error' function",
		},
		{
			name: "os.readStdin outside fallible context",
			src: `import "std/os"
fn main()
    dim s = os.readStdin()
end fn
`,
			wantErr: "must be inside a 'try' block or a 'can error' function",
		},
		{
			name: "str.replace wrong arg count",
			src: `fn main()
    dim s = "abc".replace("a")
end fn
`,
			wantErr: "str.replace() expects 2 argument(s), got 1",
		},
	}

	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := AnalyzeSource(tc.src, tc.name+".cb")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}
