package sema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cleanbasic/bootstrap/internal/ast"
)

func TestAnalyzeCoreCases(t *testing.T) {
	cases := []string{
		filepath.Join("..", "..", "..", "tests", "cases", "01_hello_and_math.cb"),
		filepath.Join("..", "..", "..", "tests", "cases", "02_fibonacci.cb"),
		filepath.Join("..", "..", "..", "tests", "cases", "03_structs_and_ufcs.cb"),
		filepath.Join("..", "..", "..", "tests", "cases", "04_containers_and_strings.cb"),
		filepath.Join("..", "..", "..", "tests", "cases", "05_enums_and_select.cb"),
		filepath.Join("..", "..", "..", "tests", "cases", "06_errors_and_defer.cb"),
	}

	for _, tc := range cases {
		t.Run(filepath.Base(tc), func(t *testing.T) {
			prog, err := LoadAndAnalyze(tc)
			if err != nil {
				t.Fatalf("LoadAndAnalyze(%s) failed: %v", tc, err)
			}
			if prog == nil || prog.EntryModule == nil {
				t.Fatalf("LoadAndAnalyze(%s) returned nil program", tc)
			}
			if _, ok := prog.EntryModule.Functions["main"]; !ok {
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
        return "sum=" + sum.toStr() + ", scaled=" + scaled.toStr()
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
	var fnDecl *ast.FnDecl
	for _, decl := range prog.EntryModule.Program.Decls {
		if fn, ok := decl.(*ast.FnDecl); ok && fn.Name == "compute" {
			fnDecl = fn
			break
		}
	}
	if fnDecl == nil {
		t.Fatal("missing compute function declaration")
	}
	expectedVars := map[string]Type{
		"sum":    IntType,
		"scaled": FloatType,
		"ok":     BoolType,
	}

	for _, st := range fnDecl.Body {
		if vd, ok := st.(*ast.VarDecl); ok {
			if vd.Type == nil {
				t.Errorf("missing inferred type for variable %q", vd.Name)
				continue
			}
			gotType := *vd.Type
			if gotType.IsVoid() {
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
		{"reassign top-level const", "\nconst MAX_LIMIT = 50\nfn main() as int\n    MAX_LIMIT = 60\n    return 0\nend fn\n", `test.cb:4:5: semantic error: cannot reassign constant "MAX_LIMIT"`},
		{"call undefined function", "\nfn main() as int\n    dim x = unknownFunc(1, 2)\n    return x\nend fn\n", `test.cb:3:13: semantic error: undefined function "unknownFunc"`},
		{"return wrong type", "\nfn getCount() as int\n    return \"not an int\"\nend fn\nfn main() as int\n    return getCount()\nend fn\n", `test.cb:3:5: semantic error: return type mismatch in function "getCount": expected int, got str`},
		{"missing return statement", "fn getCount(x as int) as int\n    if x > 0\n        return x\n    end if\nend fn\n", "missing a return statement on all control paths"},
		{"binary operator type mismatch", "fn main() as int\n    dim x = 10 + 2.5\n    return 0\nend fn\n", `type mismatch for operator "+": int and float`},
		{"if condition not bool", "fn main() as int\n    if 123\n        return 1\n    end if\n    return 0\nend fn\n", "if condition must be bool, got int"},
		{"function argument type mismatch", "fn add(a as int, b as int) as int\n    return a + b\nend fn\nfn main() as int\n    return add(1, \"two\")\nend fn\n", `argument 2 ("b") to function "add" has type str, expected int`},
		{"break outside loop", "fn main() as int\n    break\n    return 0\nend fn\n", "break statement is not inside a loop"},
		{"pass const to byref parameter", "const c = 10\nfn inc(byref x as int)\n    x = x + 1\nend fn\nfn main() as int\n    inc(c)\n    return 0\nend fn\n", `cannot mutate constant "c"`},
		{"pass literal to byref UFCS method", "type Box\n    v as int\nend type\nfn bump(byref b as Box)\n    b.v = b.v + 1\nend fn\nfn main() as int\n    Box(1).bump()\n    return 0\nend fn\n", "is not an addressable mutable variable, struct field, or list element"},
		{"mutate field of const struct", "type Box\n    v as int\nend type\nconst b = Box(1)\nfn main() as int\n    b.v = 2\n    return 0\nend fn\n", `cannot mutate constant "b"`},
		{"nested fallible call rejected", "fn f() as int can error\n    return 1\nend fn\nfn g() as int can error\n    dim x = 1 + f()\n    return x\nend fn\n", "may only appear as a top-level statement expression"},
		{"dict index byref argument rejected", "fn bump(byref x as int)\n    x = x + 1\nend fn\nfn main() as int\n    dim d = {\"a\": 1}\n    bump(d[\"a\"])\n    return 0\nend fn\n", "only list elements are addressable"},
		{"str index byref argument rejected", "fn bump(byref b as byte)\nend fn\nfn main() as int\n    dim s = \"abc\"\n    bump(s[0])\n    return 0\nend fn\n", "only list elements are addressable"},
		{"dict element field byref argument rejected", "type Box\n    v as int\nend type\nfn bump(byref x as int)\n    x = x + 1\nend fn\nfn main() as int\n    dim d = {\"a\": Box(1)}\n    bump(d[\"a\"].v)\n    return 0\nend fn\n", "only list elements are addressable"},
		{"non-existent enum variant", "enum Color\n    Red\n    Blue\nend enum\nfn main() as int\n    dim c = Color.Green\n    return 0\nend fn\n", `enum "Color" has no variant "Green"`},
		{"invalid select case pattern type", "fn main() as int\n    dim x = 1\n    select case x\n        case \"one\"\n            print(1)\n    end select\n    return 0\nend fn\n", "case pattern type str does not match select expression type int"},
		{"call can error function outside try in non-fallible function", "fn risky() as int can error\n    return 1\nend fn\nfn main() as int\n    dim x = risky()\n    return x\nend fn\n", `call to 'can error' function "risky" must be inside a 'try' block or a 'can error' function`},
		{"call error() outside try in non-fallible function", "fn main() as int\n    error(\"boom\")\n    return 0\nend fn\n", "error() can only be called inside a 'can error' function or a 'try' block"},
		{"return inside defer block", "fn main() as int\n    defer\n        return 1\n    end defer\n    return 0\nend fn\n", "return statement is not allowed inside a defer block"},
		{"unhandled fallible call inside defer block", "fn cleanup() can error\n    error(\"cleanup failed\")\nend fn\nfn helper() can error\n    defer\n        cleanup()\n    end defer\nend fn\nfn main() as int\n    try\n        helper()\n    catch e\n        print(e.msg)\n    end try\n    return 0\nend fn\n", `call to 'can error' function "cleanup" must be inside a 'try' block or a 'can error' function`},
		{"main cannot be declared can error", "fn main() as int can error\n    return 0\nend fn\n", `function "main" cannot be declared 'can error'`},
		{"mutate catch error binding", "fn risky() can error\n    error(\"fail\")\nend fn\nfn main() as int\n    try\n        risky()\n    catch e\n        e.msg = \"mutated\"\n    end try\n    return 0\nend fn\n", `cannot mutate constant "e"`},
		{"access invalid field on err", "fn risky() can error\n    error(\"fail\")\nend fn\nfn main() as int\n    try\n        risky()\n    catch e\n        print(e.code)\n    end try\n    return 0\nend fn\n", `err type has no field "code"`},
		{"error() rejects 2 arguments", "fn risky() can error\n    error(\"fail\", 1)\nend fn\nfn main() as int\n    return 0\nend fn\n", `error() expects 1 argument, got 2`},
		{"reassign numeric for counter", "fn main() as int\n    for i = 1 to 5\n        i = 10\n    end for\n    return 0\nend fn\n", `cannot reassign constant "i"`},
		{"mailbox postTimeout removed", "fn main() as int\n    dim ch = mailbox[int](1)\n    ch.postTimeout(1, 10)\n    return 0\nend fn\n", `no method or UFCS function "postTimeout" for receiver type mailbox[int]`},
		{"mailbox checkTimeout removed", "fn main() as int\n    dim ch = mailbox[int](1)\n    dim v = 0\n    ch.checkTimeout(v, 10)\n    return 0\nend fn\n", `no method or UFCS function "checkTimeout" for receiver type mailbox[int]`},
		{"mailbox isClosed removed", "fn main() as int\n    dim ch = mailbox[int](1)\n    dim c = ch.isClosed()\n    return 0\nend fn\n", `no method or UFCS function "isClosed" for receiver type mailbox[int]`},
		{"start job rejects fallible fn inside try", "fn work() can error\n    error(\"boom\")\nend fn\nfn main() as int\n    try\n        start job work()\n    catch e\n    end try\n    return 0\nend fn\n", "cannot use 'start job' with fallible function"},
		{"start job rejects fallible fn outside try", "fn work() can error\n    error(\"boom\")\nend fn\nfn main() as int\n    start job work()\n    return 0\nend fn\n", "cannot use 'start job' with fallible function"},
		{"start job rejects fallible argument", "fn failArg() as int can error\n    error(\"arg\")\n    return 1\nend fn\nfn worker(x as int)\nend fn\nfn main() as int\n    try\n        start job worker(failArg())\n    catch e\n    end try\n    return 0\nend fn\n", "may only appear as a top-level statement expression"},
		{"str plus int rejected", "fn main() as int\n    dim s = \"a\" + 1\n    return 0\nend fn\n", `type mismatch for operator "+": str and int`},
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
			name: "private defining-module UFCS rejected",
			files: map[string]string{
				"main.cb": `import "helper.cb"
fn main() as int
    dim b = helper.Box(1)
    b.secretBump()
    return 0
end fn
`,
				"helper.cb": `public type Box
    v as int
end type
fn secretBump(byref b as Box)
    b.v = b.v + 1
end fn
`,
			},
			wantErr: `no method or UFCS function "secretBump" for receiver type Box`,
		},
		{
			name: "unrelated-module UFCS rejected",
			files: map[string]string{
				"main.cb": `import "helper.cb"
import "other.cb"
fn main() as int
    dim b = helper.Box(1)
    b.bump()
    return 0
end fn
`,
				"helper.cb": `public type Box
    v as int
end type
`,
				"other.cb": `import "helper.cb"
public fn bump(byref b as helper.Box)
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
        dim exePath as str = os.executable()
        dim relPath as str = fs.rel("a", joined)
        dim stdinText as str = os.readStdin()
        dim re as regex.Regex = regex.compile("[0-9]+")
        dim m1 as bool = re.isMatch("abc123")
        dim m2 as bool = regex.match("[a-z]+", "abc")
        dim f1 as str = re.find("a12b34")
        dim fAll as list[str] = re.findAll("a12b34")
        dim rAll as str = re.replaceAll("a12b34", "#")
        if cwd == "" and exePath == "" and relPath == "" and stdinText == "" and m1 and m2 and f1 == "" and fAll.len() == 0 and rAll == ""
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

func TestSemaMutabilityReturnAndCycleRules(t *testing.T) {
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

	t.Run("mod.CONST.field, mod.CONST.method(), and non-entry fn main allowed", func(t *testing.T) {
		entry := writeFiles(t, map[string]string{
			"helper.cb": "public type Point\n    x as int\n    y as int\nend type\npublic const ORIGIN = Point(3, 4)\npublic const GREETING = \"  hello  \"\npublic fn main(x as int) as int\n    return x + 1\nend fn\n",
			"main.cb":   "import \"helper.cb\"\nfn main() as int\n    dim ox = helper.ORIGIN.x\n    dim trimmed = helper.GREETING.trim()\n    dim m = helper.main(ox)\n    if trimmed == \"hello\" and m == 4\n        return 0\n    end if\n    return 1\nend fn\n",
		})
		if _, err := LoadAndAnalyze(entry); err != nil {
			t.Fatalf("LoadAndAnalyze failed: %v", err)
		}
	})

	t.Run("valid dict and list mutations", func(t *testing.T) {
		src := `type Box
    val as int
end type

fn main() as int
    dim d = dict[str, int]()
    d["a"] = 1
    dim nested = dict[str, dict[str, int]]()
    nested["outer"] = {"inner": 2}
    nested["outer"]["inner"] = 3
    nested["outer"].remove("inner")

    dim boxes = [Box(1)]
    boxes[0].val = 2

    dim lists = [[1, 2]]
    lists[0].push(3)
    return 0
end fn
`
		if _, err := AnalyzeSource(src, "valid_mutations.cb"); err != nil {
			t.Fatalf("AnalyzeSource failed: %v", err)
		}
	})

	errCases := []struct {
		name    string
		src     string
		wantErr string
	}{
		{
			name: "dict index struct field mutation rejected",
			src: `type Box
    val as int
end type
fn main() as int
    dim d = {"a": Box(1)}
    d["a"].val = 2
    return 0
end fn
`,
			wantErr: "cannot mutate dict[str, Box] index expression in-place",
		},
		{
			name: "dict index list element mutation rejected",
			src: `fn main() as int
    dim d = {"a": [1, 2]}
    d["a"][0] = 3
    return 0
end fn
`,
			wantErr: "cannot mutate dict[str, list[int]] index expression in-place",
		},
		{
			name: "dict index list push rejected",
			src: `fn main() as int
    dim d = {"a": [1, 2]}
    d["a"].push(3)
    return 0
end fn
`,
			wantErr: "cannot mutate dict[str, list[int]] index expression in-place",
		},
		{
			name: "dict index struct list field push rejected",
			src: `type Holder
    items as list[int]
end type
fn main() as int
    dim d = {"a": Holder([1])}
    d["a"].items.push(2)
    return 0
end fn
`,
			wantErr: "cannot mutate dict[str, Holder] index expression in-place",
		},
		{
			name: "dict index struct list field element assignment rejected",
			src: `type Holder
    items as list[int]
end type
fn main() as int
    dim d = {"a": Holder([1])}
    d["a"].items[0] = 2
    return 0
end fn
`,
			wantErr: "cannot mutate dict[str, Holder] index expression in-place",
		},
		{
			name: "byref on err.msg rejected",
			src: `fn takeRef(byref s as str)
    s = "changed"
end fn
fn main() as int
    dim e as err
    takeRef(e.msg)
    return 0
end fn
`,
			wantErr: "field access target is not an addressable struct",
		},
		{
			name: "byref on dict index rejected",
			src: `fn takeRef(byref n as int)
    n = 2
end fn
fn main() as int
    dim d = {"a": 1}
    takeRef(d["a"])
    return 0
end fn
`,
			wantErr: "cannot mutate dict[str, int] index expression in-place",
		},
		{
			name:    "equality on list rejected",
			src:     "fn main() as int\n    dim ok = [1, 2] == [1, 2]\n    return 0\nend fn\n",
			wantErr: "cannot be compared",
		},
		{
			name:    "equality on dict rejected",
			src:     "fn main() as int\n    dim ok = {\"a\": 1} != {\"a\": 2}\n    return 0\nend fn\n",
			wantErr: "cannot be compared",
		},
		{
			name:    "uncomparable dict key rejected",
			src:     "fn main() as int\n    dim d = dict[list[int], str]()\n    return 0\nend fn\n",
			wantErr: "is not comparable",
		},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := AnalyzeSource(tc.src, "test.cb")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestSemaBytesSlicesAndBuiltins(t *testing.T) {
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

	t.Run("defining-module public UFCS allowed", func(t *testing.T) {
		entry := writeFiles(t, map[string]string{
			"helper.cb": `public type Box
    v as int
end type
public fn bump(byref b as Box)
    b.v = b.v + 1
end fn
`,
			"main.cb": `import "helper.cb"
fn main() as int
    dim b = helper.Box(1)
    b.bump()
    return 0
end fn
`,
		})
		if _, err := LoadAndAnalyze(entry); err != nil {
			t.Fatalf("LoadAndAnalyze failed: %v", err)
		}
	})

	t.Run("byref shadowing allowed in inner scopes", func(t *testing.T) {
		src := strings.Join([]string{
			"fn f(byref x as int)",
			"    x = x + 1",
			"    if true",
			"        dim x = 10",
			"        x = x + 1",
			"    end if",
			"    for x = 1 to 2",
			"        print(x.toStr())",
			"    end for",
			"    try",
			"        print(x.toStr())",
			"    catch x",
			"        print(x.msg)",
			"    end try",
			"end fn",
		}, "\n")
		if _, err := AnalyzeSource(src, "byref_shadow_ok.cb"); err != nil {
			t.Fatalf("expected byref shadowing to succeed, got: %v", err)
		}
	})

	errCases := []struct {
		name    string
		src     string
		wantErr string
	}{
		{
			name: "string index assignment rejected",
			src: `fn main()
    dim s = "abc"
    s[0] = 65.toByte()
end fn
`,
			wantErr: "cannot assign to string index",
		},
		{
			name: "slice non-int bound rejected",
			src: `fn main()
    dim s = "abc"
    dim t = s["a":2]
end fn
`,
			wantErr: "slice start index must be int",
		},
		{
			name:    "slice non-sliceable type rejected",
			src:     "fn main()\n    dim n = 10\n    dim t = n[0:1]\nend fn\n",
			wantErr: "cannot slice type int",
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
func TestConcurrencyAndRecursiveStructsSema(t *testing.T) {
	t.Run("start_job_allows_byref", func(t *testing.T) {
		src := "fn mut(byref x as int)\n    x = x + 1\nend fn\nfn main()\n    dim a = 1\n    start job mut(a)\nend fn\n"
		if _, err := AnalyzeSource(src, "byref_job.cb"); err != nil {
			t.Fatalf("expected start job with byref to succeed, got: %v", err)
		}
	})

	t.Run("direct_recursive_value_struct_rejected", func(t *testing.T) {
		src := strings.Join([]string{
			"type Node",
			"    val as int",
			"    next as Node",
			"end type",
			"fn main()",
			"    dim a = Node()",
			"end fn",
		}, "\n")
		_, err := AnalyzeSource(src, "rec_direct.cb")
		want := "recursive struct field 'Node.next' cannot contain 'Node' directly by value; use 'list[Node]' for recursive child nodes"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error %q, got: %v", want, err)
		}
	})

	t.Run("mutual_recursive_value_struct_rejected", func(t *testing.T) {
		src := strings.Join([]string{
			"type Expr",
			"    val as int",
			"    blk as Block",
			"end type",
			"type Block",
			"    expr as Expr",
			"end type",
			"fn main()",
			"    dim e = Expr()",
			"end fn",
		}, "\n")
		_, err := AnalyzeSource(src, "mutual.cb")
		want := "recursive struct field 'Expr.blk' cannot contain 'Block' directly by value; use 'list[Block]' for recursive child nodes"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error %q, got: %v", want, err)
		}
	})

	t.Run("recursive_struct_with_list_and_dict_succeeds", func(t *testing.T) {
		src := strings.Join([]string{
			"type TreeNode",
			"    val as int",
			"    children as list[TreeNode]",
			"    named as dict[str, TreeNode]",
			"end type",
			"fn main()",
			"    dim a = TreeNode()",
			"    dim b = TreeNode()",
			"    dim eq = (a == b)",
			"end fn",
		}, "\n")
		_, err := AnalyzeSource(src, "rec_list.cb")
		if err == nil || !strings.Contains(err.Error(), "cannot be compared with") {
			t.Fatalf("expected non-comparable error for struct containing list/dict, got: %v", err)
		}
	})

	t.Run("recursive_struct_with_list_valid", func(t *testing.T) {
		src := strings.Join([]string{
			"type Expr",
			"    val as int",
			"    blocks as list[Block]",
			"end type",
			"type Block",
			"    exprs as list[Expr]",
			"end type",
			"fn main()",
			"    dim e = Expr()",
			"    dim b = Block()",
			"    e.blocks.push(b)",
			"end fn",
		}, "\n")
		if _, err := AnalyzeSource(src, "rec_list_valid.cb"); err != nil {
			t.Fatalf("unexpected sema error: %v", err)
		}
	})

	t.Run("start_job_rejects_fallible_arg_outside_try", func(t *testing.T) {
		src := strings.Join([]string{
			"fn failArg() as int can error",
			"    return 1",
			"end fn",
			"fn worker(x as int)",
			"    print(x.toStr())",
			"end fn",
			"fn main()",
			"    start job worker(failArg())",
			"end fn",
		}, "\n")
		_, err := AnalyzeSource(src, "fallible_arg_job.cb")
		if err == nil || !strings.Contains(err.Error(), "must be inside a 'try' block or a 'can error' function") {
			t.Fatalf("expected fallible argument error in start job, got: %v", err)
		}
	})
}

func TestAddressableByRefSema(t *testing.T) {
	src := strings.Join([]string{
		"type Node",
		"    val as int",
		"    children as list[Node]",
		"end type",
		"fn inc(byref x as int)",
		"    x = x + 1",
		"end fn",
		"fn bumpNode(byref n as Node)",
		"    inc(n.val)",
		"    inc(n.children[0].val)",
		"end fn",
		"fn main() as int",
		"    dim root = Node(10, [Node(20, list[Node]())])",
		"    inc(root.val)",
		"    inc(root.children[0].val)",
		"    root.children[0].bumpNode()",
		"    return 0",
		"end fn",
	}, "\n")
	if _, err := AnalyzeSource(src, "addressable_byref.cb"); err != nil {
		t.Fatalf("AnalyzeSource failed for addressable byref args: %v", err)
	}
}

func TestByteLiteralSema(t *testing.T) {
	src := strings.Join([]string{
		"/// Byte constant doc comment",
		"const LETTER_A = 'A'",
		"fn classify(b as byte) as int",
		"    select case b",
		"        case 'A'",
		"            return 1",
		"        case '\\n'",
		"            return 2",
		"        case else",
		"            return 0",
		"    end select",
		"end fn",
		"fn main() as int",
		"    dim b = 'a'",
		"    return classify(b) + classify(LETTER_A)",
		"end fn",
	}, "\n")
	if _, err := AnalyzeSource(src, "byte_sema.cb"); err != nil {
		t.Fatalf("AnalyzeSource failed for byte literals: %v", err)
	}

	badSrc := "fn main() as int\n    dim x as int = 'a'\n    return x\nend fn\n"
	if _, err := AnalyzeSource(badSrc, "bad_byte_sema.cb"); err == nil {
		t.Fatalf("expected type mismatch error assigning byte literal to int")
	}
}

func TestPreSelfHostingSimplificationsSema(t *testing.T) {
	errCases := []struct {
		name    string
		src     string
		wantErr string
	}{
		{
			name:    "print rejects non-str arg",
			src:     "fn main()\n    print(42)\nend fn\n",
			wantErr: "expected str",
		},
		{
			name: "print rejects multiple args",
			src: `fn main()
    print("a", "b")
end fn
`,
			wantErr: "expects 1 argument(s), got 2",
		},
		{
			name: "list[int].join rejected",
			src: `fn main()
    dim xs = [1, 2]
    print(xs.join(","))
end fn
`,
			wantErr: `no method or UFCS function "join"`,
		},
		{
			name: "str.toInt outside try/can error rejected",
			src: `fn main()
    dim n = "42".toInt()
end fn
`,
			wantErr: "must be inside a 'try' block or a 'can error' function",
		},
		{
			name: "str.toFloat nested in expr rejected",
			src: `fn main()
    try
        dim x = "1.5".toFloat() + 1.0
    catch e
        print(e.msg)
    end try
end fn
`,
			wantErr: "may only appear as a top-level statement expression",
		},
		{
			name:    "for-in single var is immutable",
			src:     "fn main()\n    for x in [1, 2]\n        x = 3\n    end for\nend fn\n",
			wantErr: `cannot reassign constant "x"`,
		},
		{
			name: "for-in index and value vars are immutable",
			src: `fn main()
    for i, v in ["a", "b"]
        v = "c"
    end for
end fn
`,
			wantErr: `cannot reassign constant "v"`,
		},
	}

	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := AnalyzeSource(tc.src, tc.name+".cb")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestTopLevelAndZeroInitVarDeclSema(t *testing.T) {
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

	t.Run("cross-module public dim read and mutate", func(t *testing.T) {
		entry := writeFiles(t, map[string]string{
			"state.cb": strings.Join([]string{
				"public dim counter = 0",
				"public dim tags as list[str]",
				"public const LIMIT = 10",
				"dim secret = 42",
			}, "\n"),
			"main.cb": strings.Join([]string{
				`import "state.cb"`,
				"fn bump(byref n as int)",
				"    n = n + 1",
				"end fn",
				"fn main() as int",
				"    state.counter = state.counter + 1",
				"    bump(state.counter)",
				`    state.tags.push("ok")`,
				"    dim localInt as int",
				"    dim localDict as dict[str, int]",
				`    localDict["a"] = localInt`,
				"    return state.counter",
				"end fn",
			}, "\n"),
		})
		if _, err := LoadAndAnalyze(entry); err != nil {
			t.Fatalf("LoadAndAnalyze failed: %v", err)
		}
	})

	t.Run("private module dim and public const mutation rejected", func(t *testing.T) {
		entryPrivate := writeFiles(t, map[string]string{
			"state.cb": "dim secret = 42\n",
			"main.cb":  `import "state.cb"` + "\nfn main() as int\n    return state.secret\nend fn\n",
		})
		if _, err := LoadAndAnalyze(entryPrivate); err == nil || !strings.Contains(err.Error(), "is private") {
			t.Fatalf("expected private variable error, got: %v", err)
		}

		entryConst := writeFiles(t, map[string]string{
			"state.cb": "public const LIMIT = 10\n",
			"main.cb":  `import "state.cb"` + "\nfn main()\n    state.LIMIT = 20\nend fn\n",
		})
		if _, err := LoadAndAnalyze(entryConst); err == nil || !strings.Contains(err.Error(), "cannot mutate constant") {
			t.Fatalf("expected constant mutation error, got: %v", err)
		}
	})
}
