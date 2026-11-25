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
		"    print(greeting + \" ratio=\" + ratio.toStr() + \" ok=\" + ok.toStr())",
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
		"fmt.Println(",
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

func TestEmitGoStructsContainersEnumsAndErrors(t *testing.T) {
	cases := []string{
		"../../../tests/cases/03_structs_and_ufcs.cb",
		"../../../tests/cases/04_containers_and_strings.cb",
		"../../../tests/cases/05_enums_and_select.cb",
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

func TestEmitGoDeferLIFOAndControlFlowEdges(t *testing.T) {
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
		"    print(nonVoidEndingInTry(3).toStr())",
		"    print(nonVoidEndingInTry(-7).toStr())",
		"    try",
		"        dim c0 = checkElseIf(0)",
		"        print(c0.toStr())",
		"        dim c3 = checkElseIf(3)",
		"        print(c3.toStr())",
		"        dim c1 = checkElseIf(1)",
		"        print(c1.toStr())",
		"        dim ls = loopAndSelect(2)",
		"        print(ls.toStr())",
		"        dim cNeg = checkElseIf(-1)",
		"        print(cNeg.toStr())",
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

func TestEmitGoModulesStdlibAndConcurrency(t *testing.T) {
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
			"entities_hero__Stats",
			"combat_actions__Stats",
			"entities_hero__heal",
			"combat_actions__strike",
		} {
			if !strings.Contains(goCode, want) {
				t.Errorf("expected mangled symbol %q in generated Go:\n%s", want, goCode)
			}
		}
	})

	t.Run("stdlib helpers on demand", func(t *testing.T) {
		prog, err := sema.LoadAndAnalyze("../../../tests/cases/08_stdlib.cb")
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

			"std_cli__newParser",
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
	print(fs.isDir(".").toStr())

	dim s = "hello_world.cb"
	print(s.startsWith("hello").toStr() + ":" + s.endsWith(".cb").toStr() + ":" + s.indexOf("world").toStr() + ":" + s.indexOf("nope").toStr() + ":" + s.replace("_", "-"))

	try
		dim cwd = os.cwd()
		print((cwd.len() > 0).toStr())

		dim re = regex.compile("[0-9]+")
		print(re.isMatch("ab12cd").toStr() + ":" + re.find("ab12cd34") + ":" + re.findAll("ab12cd34").join(",") + ":" + re.replaceAll("ab12cd34", "X"))
		dim m = regex.match("^foo", "foobar")
		print(m.toStr())
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

func TestEmitGoShortCircuitAndStdlibInterop(t *testing.T) {
	src := strings.Join([]string{

		`import "std/regex"`,
		`import "std/cli"`,
		"",
		"type User",
		"    name as str",
		"    level as int",
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
		`    print(u.name + ":" + u.level.toStr())`,
		"    dim score = 10",
		"    dim bonus = 20",
		"    updateByRef(score)",
		`    print("vars=" + score.toStr() + ":" + bonus.toStr())`,
		"",
		"    dim r = regex.Regex()",
		`    print("re=" + r.isMatch("abc").toStr())`,
		"    dim p = cli.Parser()",
		`    p.addFlag("ignore-case", "i", "ignore case")`,
		`    p.addFlag("line-number", "n", "line numbers")`,
		"    try",
		`        dim args = p.parseArgs(["-in", "--", "-v", "file.txt"])`,
		`        print("cli=" + args.hasFlag("ignore-case").toStr() + ":" + args.hasFlag("line-number").toStr() + ":" + args.positionals.join(","))`,
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
		"fn worker(x as int) as int",
		"    return x + 1",
		"end fn",
		"",
		"fn main()",
		"    dim n = TreeNode()",
		"    n.val = 10",
		"    dim child = TreeNode()",
		"    child.val = 20",
		"    n.children.push(child)",
		"    dim j = start worker(41)",
		"    print(n.children[0].val.toStr() + \":\" + j.wait().toStr())",
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
	for _, want := range []string{"children []TreeNode", "cbStartJob(func() int64", ".wait()"} {
		if !strings.Contains(goCode, want) {
			t.Errorf("expected generated Go code to contain %q, got:\n%s", want, goCode)
		}
	}
	for _, unwanted := range []string{"cbBox", "cbUnbox", "cbEnsureBox", "*TreeNode", "CbJob"} {
		if strings.Contains(goCode, unwanted) {
			t.Errorf("did not expect generated Go code to contain %q, got:\n%s", unwanted, goCode)
		}
	}
}

func TestEmitFallibleStartArg(t *testing.T) {
	src := strings.Join([]string{
		"fn getCap() as int can error",
		"    return 4",
		"end fn",
		"fn worker(x as int) as int",
		"    return x",
		"end fn",
		"fn runCase() can error",
		"    dim cap = getCap()",
		"    dim j = start worker(cap)",
		"    print(j.wait().toStr())",
		"end fn",
		"fn main() as int",
		"    try",
		"        runCase()",
		"    catch e",
		"        print(e.msg)",
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
	if !strings.Contains(goCode, "}(cap)") {
		t.Errorf("expected cap variable passed eagerly to start IIFE, got:\n%s", goCode)
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

func TestEmitFallibleNumericParsingAndReceiverMethods(t *testing.T) {
	src := strings.Join([]string{
		"fn main() as int",
		"    try",
		`        dim n = "42".toInt()`,
		`        dim f = "3.5".toFloat()`,
		`        dim xs = ["a", "b"]`,
		`        print(n.toStr() + ":" + f.toStr() + ":" + xs.join("-") + ":" + "hi".quote() + ":" + 65.toChar() + ":" + xs.len().toStr())`,
		"    catch e",
		`        print("unexpected:" + e.msg)`,
		"    end try",
		"    try",
		`        dim badInt = "nope".toInt()`,
		`        print(badInt.toStr())`,
		"    catch e",
		`        print("badInt-caught")`,
		"    end try",
		"    try",
		`        dim badFloat = "not-float".toFloat()`,
		`        print(badFloat.toStr())`,
		"    catch e",
		`        print("badFloat-caught")`,
		"    end try",
		"    return 0",
		"end fn",
	}, "\n")
	prog, err := sema.AnalyzeSource(src, "fallible_parse.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	goCode, err := EmitGoWithOptions(prog, Options{EmitLineDirectives: false})
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}
	if strings.Contains(goCode, "cbStrToInt") {
		t.Errorf("did not expect cbStrToInt helper in generated Go:\n%s", goCode)
	}
	got := runGeneratedGo(t, goCode)
	want := "42:3.5:a-b:\"hi\":A:2\nbadInt-caught\nbadFloat-caught"
	if got != want {
		t.Errorf("output = %q, want %q\ngo:\n%s", got, want, goCode)
	}
}

func TestEmitGoAlignedSimplifications(t *testing.T) {
	src := strings.Join([]string{
		"dim gTotal = 10",
		"dim gScores as dict[str, int]",
		"",
		"fn shadowAndBump(byref x as int)",
		"    x = x + 5",
		"    if true",
		"        dim x = 100",
		"        x = x + 1",
		"    end if",
		"    for x = 1 to 2",
		"    end for",
		"end fn",
		"",
		"fn workerAdd(n as int) as int",
		"    return n + 32",
		"end fn",
		"",
		"fn main() as int",
		"    dim localZero as int",
		"    dim localDict as dict[str, int]",
		`    localDict["k"] = 7`,
		`    gScores["a"] = 3`,
		"    shadowAndBump(gTotal)",
		"    dim j = start workerAdd(10)",
		"    dim sharedVal = j.wait()",
		`    print(gTotal.toStr() + ":" + gScores["a"].toStr() + ":" + localZero.toStr() + ":" + localDict["k"].toStr() + ":" + sharedVal.toStr())`,
		"    return 0",
		"end fn",
	}, "\n")
	prog, err := sema.AnalyzeSource(src, "simplifications.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	goCode, err := EmitGoWithOptions(prog, Options{EmitLineDirectives: false})
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}
	got := runGeneratedGo(t, goCode)
	want := "15:3:0:7:42"
	if got != want {
		t.Errorf("output = %q, want %q\ngo:\n%s", got, want, goCode)
	}
}

func TestEmitGoUFCSPrint(t *testing.T) {
	src := strings.Join([]string{
		"fn main() as int",
		`    "hello".print()`,
		"    return 0",
		"end fn",
	}, "\n")

	prog, err := sema.AnalyzeSource(src, "ufcs_print_test.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}

	goCode, err := EmitGoWithOptions(prog, Options{EmitLineDirectives: false})
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}

	if !strings.Contains(goCode, `fmt.Println("hello")`) {
		t.Errorf("expected fmt.Println call in generated Go:\n%s", goCode)
	}

	got := runGeneratedGo(t, goCode)
	if got != "hello" {
		t.Errorf("output = %q, want %q", got, "hello")
	}
}

func TestEmitGoListSort(t *testing.T) {
	src := `fn main() as int
    dim words = ["pear", "apple", "fig"]
    words.sort()
    dim nums = [30, 10, 20]
    nums.sort()
    print(words.join(",") + ":" + nums[0].toStr() + "," + nums[1].toStr() + "," + nums[2].toStr())
    return 0
end fn`

	prog, err := sema.AnalyzeSource(src, "list_sort.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	goCode, err := EmitGoWithOptions(prog, Options{EmitLineDirectives: false})
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}
	if !strings.Contains(goCode, `"slices"`) || !strings.Contains(goCode, "slices.Sort(words)") {
		t.Fatalf("expected slices import and slices.Sort in generated Go:\n%s", goCode)
	}
	got := runGeneratedGo(t, goCode)
	want := "apple,fig,pear:10,20,30"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
func TestEmitGoZeroStructFieldInit(t *testing.T) {
	src := `type Plain
    x as int
end type
type Inner
    meta as dict[str, int]
end type
type Outer
    counts as dict[str, int]
    inner as Inner
    plain as Plain
end type
fn makeVal(v as int) as int
    return v
end fn
fn makeOuter(fail as bool) as Outer can error
    if fail
        error("boom")
    end if
    dim o as Outer
    o.counts["ok"] = 1
    return o
end fn
fn main() as int
    dim p as Plain
    dim p2 = Plain()
    dim o1 as Outer
    o1.counts["a"] = 42
    o1.inner.meta["b"] = 7
    dim j = start makeVal(99)
    dim v = j.wait()
    dim o2 = Outer()
    o2.counts["c"] = 5
    dim errHit = false
    try
        dim unused = makeOuter(true)
    catch e
        errHit = true
    end try
    print(p.x.toStr() + ":" + p2.x.toStr() + ":" + o1.counts["a"].toStr() + ":" + o1.inner.meta["b"].toStr() + ":" + v.toStr() + ":" + o2.counts["c"].toStr() + ":" + errHit.toStr())
    return 0
end fn`

	prog, err := sema.AnalyzeSource(src, "zero_struct_init.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	goCode, err := EmitGoWithOptions(prog, Options{EmitLineDirectives: false})
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}
	if !strings.Contains(goCode, "var p Plain\n") || !strings.Contains(goCode, "var p2 Plain = Plain{}") {
		t.Errorf("expected plain structs to keep zero-value emission without composite init:\n%s", goCode)
	}
	if !strings.Contains(goCode, "return Outer{counts: map[string]int64{}, inner: Inner{meta: map[string]int64{}}}") {
		t.Errorf("expected error return path to use goSemaZeroValue composite struct init:\n%s", goCode)
	}
	got := runGeneratedGo(t, goCode)
	want := "0:0:42:7:99:5:true"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestEmitGoJobRaceSafetyAndRuntime(t *testing.T) {
	src := strings.Join([]string{
		"fn scale(x as int) as list[int]",
		"    return [x, x * 2]",
		"end fn",
		"",
		"fn noop()",
		"end fn",
		"",
		"fn main() as int",
		"    dim jobs = list[job[list[int]]]()",
		"    for i = 1 to 4",
		"        jobs.push(start scale(i))",
		"    end for",
		"    dim x = 10",
		"    dim j = start scale(x)",
		"    x = 999",
		"    dim sum = 0",
		"    for item in jobs",
		"        dim res = item.wait()",
		"        sum = sum + res[0] + res[1]",
		"    end for",
		"    dim single = j.wait()",
		"    dim singleAgain = j.wait()",
		"    dim nilJob as job[int]",
		"    dim vj = start noop()",
		"    vj.wait()",
		"    print(sum.toStr() + \":\" + single[0].toStr() + \":\" + singleAgain[1].toStr() + \":\" + nilJob.wait().toStr())",
		"    return 0",
		"end fn",
	}, "\n")

	prog, err := sema.AnalyzeSource(src, "job_race.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	goCode, err := EmitGoWithOptions(prog, Options{EmitLineDirectives: false})
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}

	tmpDir := t.TempDir()
	mainPath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainPath, []byte(goCode), 0o600); err != nil {
		t.Fatalf("write temp main.go: %v", err)
	}
	cmd := exec.Command("go", "run", "-race", mainPath)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("go run -race failed: %v\nstderr:\n%s\ngo:\n%s", err, errBuf.String(), goCode)
	}
	got := strings.TrimSpace(outBuf.String())
	want := "30:10:20:0"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
func TestEmitLineDirectiveRejectsControlCharsAndAliasesOsExit(t *testing.T) {
	src := "fn main() as int\n    return 0\nend fn\n"
	for _, badName := range []string{"evil\npackage main.cb", "evil\rfile.cb"} {
		prog, err := sema.AnalyzeSource(src, badName)
		if err != nil {
			t.Fatalf("AnalyzeSource(%q) failed: %v", badName, err)
		}
		goCode, err := EmitGoWithOptions(prog, Options{EmitLineDirectives: true})
		if err != nil {
			t.Fatalf("EmitGoWithOptions(%q) failed: %v", badName, err)
		}
		if strings.Contains(goCode, "//line ") {
			t.Fatalf("expected //line directive to be omitted for %q, got:\n%s", badName, goCode)
		}
		if !strings.Contains(goCode, "_cb_os \"os\"") || !strings.Contains(goCode, "_cb_os.Exit(int(code))") {
			t.Fatalf("expected aliased _cb_os.Exit in generated code, got:\n%s", goCode)
		}
	}
}

func TestEdgeCasesOpaqueGoTypePushEvalShadowAndMinInt64(t *testing.T) {
	src := strings.Join([]string{
		"import \"std/os\"",
		"go import \"errors\"",
		"go import \"fmt\"",
		"go import \"io\"",
		"go import \"os\" as go_os",
		"go import \"strings\"",
		"type Reader = go \"io.Reader\"",
		"type Mode = go \"go_os.FileMode\"",
		"fn newReader(flag as bool) as Reader can error = go \"cbNewReader\"",
		"fn modeStr(m as Mode) as str = go \"cbModeStr\"",
		"go",
		"func cbNewReader(flag bool) (io.Reader, error) {",
		"    if !flag {",
		"        return nil, errors.New(\"bad reader\")",
		"    }",
		"    return strings.NewReader(\"ok\"), nil",
		"}",
		"func cbModeStr(m go_os.FileMode) string {",
		"    return fmt.Sprintf(\"%d\", m)",
		"}",
		"end go",
		"fn makeReader(fail as bool) as Reader can error",
		"    if fail",
		"        error(\"fail reader\")",
		"    end if",
		"    return newReader(true)",
		"end fn",
		"fn makeMode(fail as bool) as Mode can error",
		"    if fail",
		"        error(\"fail mode\")",
		"    end if",
		"    return Mode()",
		"end fn",
		"const MIN_I64 = -9223372036854775808",
		"type Counter",
		"    calls as int",
		"end type",
		"fn nextIdx(byref c as Counter) as int",
		"    c.calls = c.calls + 1",
		"    return 0",
		"end fn",
		"fn _a0(x as int) as int",
		"    return x + 1",
		"end fn",
		"fn failIfNeg(v as int) as int can error",
		"    if v < 0",
		"        error(\"neg\")",
		"    end if",
		"    return v",
		"end fn",
		"fn main() as int",
		"    dim m = Mode()",
		"    dim gotReaderErr = false",
		"    try",
		"        dim r = makeReader(true)",
		"    catch e",
		"        gotReaderErr = e.msg == \"fail reader\"",
		"    end try",
		"    dim gotModeErr = false",
		"    try",
		"        dim m2 = makeMode(true)",
		"    catch e",
		"        gotModeErr = e.msg == \"fail mode\"",
		"    end try",
		"    dim c = Counter(0)",
		"    dim buckets = [[10]]",
		"    buckets[nextIdx(c)].push(42)",
		"    dim j = start _a0(7)",
		"    dim _err_1 = 0",
		"    dim _tryErr_1 = 0",
		"    try",
		"        _err_1 = failIfNeg(5)",
		"    catch ex",
		"        _tryErr_1 = 1",
		"    end try",
		"    dim a1 = os.args()",
		"    a1[0] = \"mutated\"",
		"    dim a2 = os.args()",
		"    print(gotReaderErr.toStr() + \":\" + gotModeErr.toStr() + \":\" + modeStr(m) + \":\" + c.calls.toStr() + \":\" + buckets[0][1].toStr() + \":\" + j.wait().toStr() + \":\" + _err_1.toStr() + \":\" + _tryErr_1.toStr() + \":\" + MIN_I64.toStr() + \":\" + a2[0])",
		"    return 0",
		"end fn",
	}, "\n")
	tmpDir := t.TempDir()
	cbPath := filepath.Join(tmpDir, "edge_cases.cb")
	if err := os.WriteFile(cbPath, []byte(src), 0o600); err != nil {
		t.Fatalf("write temp edge_cases.cb: %v", err)
	}
	prog, err := sema.LoadAndAnalyze(cbPath)
	if err != nil {
		t.Fatalf("LoadAndAnalyze failed: %v", err)
	}
	goCode, err := EmitGoWithOptions(prog, Options{EmitLineDirectives: false})
	if err != nil {
		t.Fatalf("EmitGo failed: %v", err)
	}

	mainPath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainPath, []byte(goCode), 0o600); err != nil {
		t.Fatalf("write temp main.go: %v", err)
	}
	cmd := exec.Command("go", "run", mainPath, "origArg")
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("go run failed: %v\nstderr:\n%s\ngo:\n%s", err, errBuf.String(), goCode)
	}
	got := strings.TrimSpace(outBuf.String())
	want := "true:true:0:1:42:8:5:0:-9223372036854775808:origArg"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestEmitGoCancelTokenAndJobTimeout(t *testing.T) {
	isDoneOnlySrc := strings.Join([]string{
		"fn work() as int",
		"    return 7",
		"end fn",
		"fn main() as int",
		"    dim j = start work()",
		"    dim v = j.wait()",
		"    if j.isDone()",
		"        return v - 7",
		"    end if",
		"    return 1",
		"end fn",
	}, "\n")
	prog1, err := sema.AnalyzeSource(isDoneOnlySrc, "isdone_only.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	go1, err := EmitGoWithOptions(prog1, Options{EmitLineDirectives: false})
	if err != nil {
		t.Fatalf("EmitGoWithOptions failed: %v", err)
	}
	for _, unwanted := range []string{`"time"`, `"sync"`, `"sync/atomic"`, "_cb_atomic", "cbCancelToken", "waitTimeout"} {
		if strings.Contains(go1, unwanted) {
			t.Errorf("did not expect %q when cancelToken and waitTimeout are unused, got:\n%s", unwanted, go1)
		}
	}

	fullSrc := strings.Join([]string{
		"const atomic = 42",
		"type Cfg",
		"    tok as cancelToken",
		"end type",
		"fn waitCancel(tok as cancelToken) as int",
		"    while not tok.isCanceled()",
		"    end while",
		"    return atomic",
		"end fn",
		"fn fastVoid()",
		"end fn",
		"fn main() as int",
		"    dim cfg = Cfg()",
		"    dim j = start waitCancel(cfg.tok)",
		"    dim runningDone = j.isDone()",
		"    dim timedOut = false",
		"    try",
		"        dim unused = j.waitTimeout(15)",
		"    catch e",
		"        timedOut = true",
		"        cfg.tok.cancel()",
		"        cfg.tok.cancel()",
		"    end try",
		"    dim res = j.wait()",
		"    dim vj = start fastVoid()",
		"    try",
		"        vj.waitTimeout(5000)",
		"    catch e",
		"        return 1",
		"    end try",
		`    print(runningDone.toStr() + ":" + timedOut.toStr() + ":" + res.toStr() + ":" + j.isDone().toStr() + ":" + cfg.tok.isCanceled().toStr())`,
		"    return 0",
		"end fn",
	}, "\n")
	prog2, err := sema.AnalyzeSource(fullSrc, "cancel_timeout_race.cb")
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	go2, err := EmitGoWithOptions(prog2, Options{EmitLineDirectives: false})
	if err != nil {
		t.Fatalf("EmitGoWithOptions failed: %v", err)
	}

	tmpDir := t.TempDir()
	mainPath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainPath, []byte(go2), 0o600); err != nil {
		t.Fatalf("write temp main.go: %v", err)
	}
	cmd := exec.Command("go", "run", "-race", mainPath)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("go run -race failed: %v\nstderr:\n%s\ngo:\n%s", err, errBuf.String(), go2)
	}
	got := strings.TrimSpace(outBuf.String())
	want := "false:true:42:true:true"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
