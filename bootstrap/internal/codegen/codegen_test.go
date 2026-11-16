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
		"    dim b as byte = 7.toByte()",
		"    dim greeting = \"res=\" + res.toStr() + \":\" + ratio.toStr() + \":\" + b.toStr() + \":\" + \"hi\".toStr()",
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
		"strconv.FormatInt(res, 10)",
		"strconv.FormatFloat(ratio, 'g', -1, 64)",
		"strconv.FormatInt(int64(b), 10)",
		fmt.Sprintf("%q", "hi"),
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

	got := runGeneratedGo(t, goCode)
	want := "body\nthird\nsecond\nfirst"
	if got != want {
		t.Errorf("defer LIFO output = %q, want %q", got, want)
	}
}

func TestEmitGoPhase4EdgeCases(t *testing.T) {
	src := strings.Join([]string{
		"fn mightFail(n as int) as int can error",
		"    if n < 0",
		fmt.Sprintf("        error(%q)", "negative"),
		"    end if",
		"    return n * 10",
		"end fn",
		"",
		"fn nonVoidEndingInTry(n as int) as int",
		"    try",
		"        return mightFail(n)",
		"    catch e",
		"        return -1",
		"    end try",
		"end fn",
		"",
		"fn checkElseIf(n as int) as int can error",
		"    dim v = mightFail(n)",
		"    if n == 0",
		"        return 0",
		"    else if v > 20",
		"        return 1",
		"    else",
		"        return 2",
		"    end if",
		"end fn",
		"",
		"fn loopAndSelect(limit as int) as int can error",
		"    dim sum = 0",
		"    dim i = 1",
		"    while i <= limit",
		"        sum = sum + i",
		"        i = i + 1",
		"    end while",
		"    dim lim = mightFail(1)",
		"    for j = 1 to lim / 5",
		"        sum = sum + j",
		"    end for",
		"    dim sel = mightFail(2)",
		"    select case sel",
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
		"        dim c0 = checkElseIf(0)",
		"        print(c0)",
		"        dim c3 = checkElseIf(3)",
		"        print(c3)",
		"        dim c1 = checkElseIf(1)",
		"        print(c1)",
		"        dim ls = loopAndSelect(2)",
		"        print(ls)",
		"        dim cNeg = checkElseIf(-1)",
		"        print(cNeg)",
		"    catch e",
		"        print(e.msg)",
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

	got := runGeneratedGo(t, goCode)
	want := "30\n-1\n0\n1\n2\n106\nnegative"
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

			"cb_std_cli__newParser",
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
		got := runGeneratedGo(t, goCode)
		want := "forced failure\nparsed-ok"
		if got != want {
			t.Errorf("output = %q, want %q\ngo:\n%s", got, want, goCode)
		}
	})

	t.Run("std/regex, fs path helpers, os.cwd, and str methods", func(t *testing.T) {
		src := `import "std/fs"
import "std/os"
import "std/regex"

fn main()
	dim p = fs.join("dir", "hello.cb")
	print(fs.base(p) + ":" + fs.dir(p) + ":" + fs.ext(p))
	print(fs.isDir("."))

	dim s = "hello_world.cb"
	print(s.startsWith("hello").toStr() + ":" + s.endsWith(".cb").toStr() + ":" + s.indexOf("world").toStr() + ":" + s.indexOf("nope").toStr() + ":" + s.replace("_", "-"))

	try
		dim cwd = os.cwd()
		print(cwd.len() > 0)

		dim re = regex.compile("[0-9]+")
		print(re.isMatch("ab12cd").toStr() + ":" + re.find("ab12cd34") + ":" + str.join(re.findAll("ab12cd34"), ",") + ":" + re.replaceAll("ab12cd34", "X"))
		dim m = regex.match("^foo", "foobar")
		print(m)
	catch e
		print("err:" + e.msg)
	end try

	try
		dim bad = regex.compile("[unclosed")
		print(bad.find("x"))
	catch e
		print("bad-regex-caught")
	end try
end fn
`
		prog, err := sema.AnalyzeSource(src, "new_stdlib_runtime.cb")
		if err != nil {
			t.Fatalf("AnalyzeSource failed: %v", err)
		}
		goCode, err := EmitGo(prog)
		if err != nil {
			t.Fatalf("EmitGo failed: %v", err)
		}
		got := runGeneratedGo(t, goCode)
		want := strings.Join([]string{
			"hello.cb:dir:.cb",
			"true",
			"true:true:6:-1:hello-world.cb",
			"true",
			"true:12:12,34:abXcdX",
			"true",
			"bad-regex-caught",
		}, "\n")
		if got != want {
			t.Errorf("output = %q, want %q\ngo:\n%s", got, want, goCode)
		}
	})
}

func TestEmitGoByteLiterals(t *testing.T) {
	src := strings.Join([]string{
		"/// Top-level byte constant",
		"const LETTER_A = 'A'",
		"fn classify(ch as byte) as str",
		"    select case ch",
		"        case LETTER_A",
		"            return \"upper-a\"",
		"        case '\\n'",
		"            return \"newline\"",
		"        case '\\''",
		"            return \"quote\"",
		"        case '\\0'",
		"            return \"nul\"",
		"        case else",
		"            return \"other\"",
		"    end select",
		"end fn",
		"fn main() as int",
		"    // Test byte literals",
		"    dim b as byte = 'A'",
		"    print(classify(b) + \",\" + classify('\\n') + \",\" + classify('\\'') + \",\" + classify('\\0') + \",\" + classify('z'))",
		"    return 0",
		"end fn",
	}, "\n")
	prog, err := sema.AnalyzeSource(src, "byte_codegen.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	goCode, err := EmitGo(prog)
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}
	got := runGeneratedGo(t, goCode)
	want := "upper-a,newline,quote,nul,other"
	if got != want {
		t.Errorf("byte literal output = %q, want %q\ngo:\n%s", got, want, goCode)
	}
}
func runGeneratedGo(t *testing.T, goCode string) string {
	t.Helper()
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
	return strings.TrimRight(strings.ReplaceAll(stdout.String(), "\r\n", "\n"), "\n")
}

func TestEmitGoReviewRegressions(t *testing.T) {
	src := strings.Join([]string{

		`import "std/regex"`,
		`import "std/cli"`,
		"",
		"type User",
		"    name as str",
		"    func as int",
		"end type",
		"",
		"fn boom() as bool can error",
		`    error("boom")`,
		"    return true",
		"end fn",
		"",
		"fn countCall(byref calls as int, v as int) as int",
		"    calls = calls + 1",
		"    return v",
		"end fn",
		"",
		"fn updateByRef(byref x as int)",
		"    if true",
		"        dim y = 99",
		`        print("inner=" + y.toStr())`,
		"    end if",
		"    x = x + 1",
		"end fn",
		"",
		"fn main() as int",
		"    dim scCalls = 0",
		"    if false and (countCall(scCalls, 1) > 0)",
		`        print("bad1")`,
		"    else",
		`        print("sc-and-ok")`,
		"    end if",
		"    if true or (countCall(scCalls, 1) > 0)",
		`        print("sc-or-ok")`,
		"    end if",

		"    dim endCalls = 0",
		"    dim sum = 0",
		"    for i = 1 to countCall(endCalls, 5)",
		"        sum = sum + i",
		"    end for",
		`    print(sum.toStr() + ":" + endCalls.toStr())`,
		"",
		`    dim u = User("Ada", 7)`,
		`    print(u.name + ":" + u.func.toStr())`,
		"    dim fmt = 10",
		"    dim map = 20",
		"    updateByRef(fmt)",
		`    print("vars=" + fmt.toStr() + ":" + map.toStr())`,
		"",
		"    dim r = regex.Regex()",
		`    print("re=" + r.isMatch("abc").toStr())`,
		"    dim p = cli.Parser()",
		`    p.addFlag("ignore-case", "i", "ignore case")`,
		`    p.addFlag("line-number", "n", "line numbers")`,
		"    try",
		`        dim args = p.parseArgs(["-in", "--", "-v", "file.txt"])`,
		`        print("cli=" + args.hasFlag("ignore-case").toStr() + ":" + args.hasFlag("line-number").toStr() + ":" + str.join(args.positionals, ","))`,
		"    catch e",
		`        print("cli-err:" + e.msg)`,
		"    end try",
		"    return 0",
		"end fn",
	}, "\n")
	prog, err := sema.AnalyzeSource(src, "review_regressions.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	goCode, err := EmitGo(prog)
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}
	got := runGeneratedGo(t, goCode)
	want := strings.Join([]string{
		"sc-and-ok",
		"sc-or-ok",
		"15:1",
		"Ada:7",
		"inner=99",
		"vars=11:20",
		"re=false",
		"cli=true:true:-v,file.txt",
	}, "\n")
	if got != want {
		t.Errorf("output mismatch:\ngot:\n%s\nwant:\n%s\ngo:\n%s", got, want, goCode)
	}
}

func TestEmitGoLineDirectives(t *testing.T) {
	src := "fn main() as int\n    dim x = 42\n    return 0\nend fn\n"
	prog, err := sema.AnalyzeSource(src, "dir/sample.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}

	withLines, err := EmitGo(prog)
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}
	if !strings.Contains(withLines, "\n//line dir/sample.cb:2\n") {
		t.Errorf("expected column-1 //line directive in generated Go:\n%s", withLines)
	}

	withoutLines, err := EmitGoWithOptions(prog, Options{EmitLineDirectives: false})
	if err != nil {
		t.Fatalf("EmitGoWithOptions failed: %v", err)
	}
	if strings.Contains(withoutLines, "//line ") {
		t.Errorf("did not expect //line directive when EmitLineDirectives is false:\n%s", withoutLines)
	}
}

func TestEmitConcurrencyAndValueStructs(t *testing.T) {
	src := strings.Join([]string{
		"type TreeNode",
		"    val as int",
		"    children as list[TreeNode]",
		"end type",
		"",
		"fn worker(box as mailbox[int]) as int",
		"    return box.get()",
		"end fn",
		"",
		"fn main()",
		"    dim n = TreeNode()",
		"    n.val = 10",
		"    dim child = TreeNode()",
		"    child.val = 20",
		"    n.children.push(child)",
		"    dim box = mailbox[int](4)",
		"    start job worker(box)",
		"    box.post(42)",
		"    print(n.children[0].val)",
		"end fn",
	}, "\n")
	prog, err := sema.AnalyzeSource(src, "cg_test.cb")
	if err != nil {
		t.Fatalf("sema failed: %v", err)
	}
	goCode, err := EmitGoWithOptions(prog, Options{EmitLineDirectives: false})
	if err != nil {
		t.Fatalf("codegen failed: %v", err)
	}
	for _, want := range []string{"Cb_children []TreeNode", "make(chan int64, int64(4))", "go worker("} {
		if !strings.Contains(goCode, want) {
			t.Errorf("expected generated Go code to contain %q, got:\n%s", want, goCode)
		}
	}
	for _, unwanted := range []string{"cbBox", "cbUnbox", "cbEnsureBox", "*TreeNode", "CbJob", "cbStartJob"} {
		if strings.Contains(goCode, unwanted) {
			t.Errorf("did not expect generated Go code to contain %q, got:\n%s", unwanted, goCode)
		}
	}
}

func TestEmitFallibleMailboxCapAndStartJobArg(t *testing.T) {
	src := strings.Join([]string{
		"fn getCap() as int can error",
		"    return 4",
		"end fn",
		"fn worker(x as int) as int",
		"    return x",
		"end fn",
		"fn runCase() can error",
		"    dim cap = getCap()",
		"    dim box = mailbox[int](cap)",
		"    start job worker(cap)",
		"    print(box.count())",
		"end fn",
		"fn main() as int",
		"    try",
		"        runCase()",
		"    catch e",
		"        print(e)",
		"    end try",
		"    return 0",
		"end fn",
	}, "\n")
	prog, err := sema.AnalyzeSource(src, "fallible_lift.cb")
	if err != nil {
		t.Fatalf("sema failed: %v", err)
	}
	goCode, err := EmitGoWithOptions(prog, Options{EmitLineDirectives: false})
	if err != nil {
		t.Fatalf("codegen failed: %v", err)
	}
	if !strings.Contains(goCode, "make(chan int64, cap") {
		t.Errorf("expected cap variable in make(chan int64, ...), got:\n%s", goCode)
	}
}

func TestEmitAddressableByRefArgs(t *testing.T) {
	src := strings.Join([]string{
		"type Node",
		"    val as int",
		"    children as list[Node]",
		"end type",
		"",
		"fn inc(byref x as int)",
		"    x = x + 1",
		"end fn",
		"",
		"fn bumpChild(byref n as Node)",
		"    inc(n.val)",
		"    inc(n.children[0].val)",
		"end fn",
		"",
		"fn main() as int",
		"    dim items = [10, 20]",
		"    inc(items[1])",
		"    dim root = Node(1, [Node(5, [Node(9, list[Node]())])])",
		"    inc(root.val)",
		"    bumpChild(root.children[0])",
		"    print(items[1].toStr() + \":\" + root.val.toStr() + \":\" + root.children[0].val.toStr() + \":\" + root.children[0].children[0].val.toStr())",
		"    return 0",
		"end fn",
	}, "\n")
	prog, err := sema.AnalyzeSource(src, "byref_addr.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	goCode, err := EmitGoWithOptions(prog, Options{EmitLineDirectives: false})
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}
	got := runGeneratedGo(t, goCode)
	want := "21:2:6:10"
	if got != want {
		t.Errorf("output = %q, want %q\ngo:\n%s", got, want, goCode)
	}
}
