package lexer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cleanbasic/bootstrap/internal/token"
)

func TestRemovedCptrIsIdent(t *testing.T) {
	if got := token.LookupIdent("cptr"); got != token.IDENT {
		t.Fatalf("LookupIdent(\"cptr\") = %s, want IDENT", got)
	}
}

func TestTwoWordKeywords(t *testing.T) {
	src := strings.Join([]string{
		"else if",
		"end fn",
		"end if",
		"end for",
		"end while",
		"end select",
		"end try",
		"end defer",
		"end type",
		"end enum",
		"select case",
		"case else",
		"can error",
		"start job",
		"start",
		"mailbox",
		"job",
	}, "\n")

	toks, err := Tokenize(src, "keywords.cb")
	if err != nil {
		t.Fatalf("unexpected lex error: %v", err)
	}

	wantKinds := []token.Kind{
		token.ELSE_IF, token.NEWLINE,
		token.END_FN, token.NEWLINE,
		token.END_IF, token.NEWLINE,
		token.END_FOR, token.NEWLINE,
		token.END_WHILE, token.NEWLINE,
		token.END_SELECT, token.NEWLINE,
		token.END_TRY, token.NEWLINE,
		token.END_DEFER, token.NEWLINE,
		token.END_TYPE, token.NEWLINE,
		token.END_ENUM, token.NEWLINE,
		token.SELECT_CASE, token.NEWLINE,
		token.CASE_ELSE, token.NEWLINE,
		token.CAN_ERROR, token.NEWLINE,
		token.START_JOB, token.NEWLINE,
		token.IDENT, token.NEWLINE,
		token.MAILBOX, token.NEWLINE,
		token.JOB, token.NEWLINE,
		token.EOF,
	}

	if len(toks) != len(wantKinds) {
		t.Fatalf("got %d tokens, want %d: %v", len(toks), len(wantKinds), toks)
	}
	for i, want := range wantKinds {
		if toks[i].Kind != want {
			t.Errorf("token[%d] kind = %s (%q), want %s", i, toks[i].Kind, toks[i].Lexeme, want)
		}
	}
}

func TestForbiddenCStyleLogicalOperators(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantSub string
	}{
		{
			name:    "logical and &&",
			src:     "dim ok = a && b\n",
			wantSub: "use 'and' instead",
		},
		{
			name:    "logical or ||",
			src:     "dim ok = a || b\n",
			wantSub: "use 'or' instead",
		},
		{
			name:    "logical not !",
			src:     "dim ok = !a\n",
			wantSub: "use 'not' instead",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Tokenize(tc.src, "bad.cb")
			if err == nil {
				t.Fatalf("expected lex error for %q, got nil", tc.src)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q did not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestCommentsAndNewlineCoalescing(t *testing.T) {
	src := "' leading comment\n# another comment\n\nconst x = 10 ' inline comment\n\n\n# middle comment\ndim y = 20\n"
	toks, err := Tokenize(src, "comments.cb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []token.Kind{
		token.CONST, token.IDENT, token.ASSIGN, token.INT_LIT, token.NEWLINE,
		token.DIM, token.IDENT, token.ASSIGN, token.INT_LIT, token.NEWLINE,
		token.EOF,
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(toks), len(want), toks)
	}
	for i, k := range want {
		if toks[i].Kind != k {
			t.Errorf("tok[%d] = %s, want %s", i, toks[i].Kind, k)
		}
	}
	if toks[0].Pos.Line != 4 || toks[0].Pos.Column != 1 {
		t.Errorf("first token pos = %+v, want line 4 col 1", toks[0].Pos)
	}
}

func TestInterpolatedStringDesugaring(t *testing.T) {
	src := `print($"Hello {user.name}, score={{100}} and {parts[2]}!")
`
	toks, err := Tokenize(src, "interp.cb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expected sequence:
	// IDENT("print") (
	//   STRING_LIT("Hello ") & ( IDENT("user") . IDENT("name") ) & STRING_LIT(", score={100} and ") & ( IDENT("parts") [ INT_LIT("2") ] ) & STRING_LIT("!")
	// ) NEWLINE EOF
	wantKinds := []token.Kind{
		token.IDENT, token.LPAREN,
		token.STRING_LIT,
		token.AMP, token.LPAREN, token.IDENT, token.DOT, token.IDENT, token.RPAREN,
		token.AMP, token.STRING_LIT,
		token.AMP, token.LPAREN, token.IDENT, token.LBRACKET, token.INT_LIT, token.RBRACKET, token.RPAREN,
		token.AMP, token.STRING_LIT,
		token.RPAREN, token.NEWLINE, token.EOF,
	}

	if len(toks) != len(wantKinds) {
		t.Fatalf("got %d tokens, want %d:\n%v", len(toks), len(wantKinds), toks)
	}
	for i, want := range wantKinds {
		if toks[i].Kind != want {
			t.Errorf("tok[%d] = %s (%q), want %s", i, toks[i].Kind, toks[i].Lexeme, want)
		}
	}
	if toks[10].Lexeme != ", score={100} and " {
		t.Errorf("escaped brace segment = %q, want %q", toks[10].Lexeme, ", score={100} and ")
	}
}

func TestInterpolatedStringFullExpressionsAndGoBlocks(t *testing.T) {
	goodInputs := []string{
		`print($"sum={a + b}")
`,
		`print($"call={foo(x)}")
`,
		`print($"bool={true}")
`,
		`print($"nested={str.quote("hi")}")
`,
		`print($"dict={{"k": 1}.len()}")
`,
	}
	for _, src := range goodInputs {
		if _, err := Tokenize(src, "good_interp.cb"); err != nil {
			t.Errorf("unexpected error for %q: %v", src, err)
		}
	}

	badInputs := []string{
		`print($"empty={}")
`,
		`print($"unterm={a + b")
`,
		"print($\"newline={a\n+ b}\")\n",
	}
	for _, src := range badInputs {
		if _, err := Tokenize(src, "bad_interp.cb"); err == nil {
			t.Errorf("expected error for invalid interpolation %q, got nil", src)
		}
	}

	goSrc := `go import "os"
go
func helper() bool { return true && !false }
end go
`
	toks, err := Tokenize(goSrc, "go_block.cb")
	if err != nil {
		t.Fatalf("unexpected error for go block: %v", err)
	}
	if toks[0].Kind != token.GO_IMPORT {
		t.Errorf("tok[0] = %s, want GO_IMPORT", toks[0].Kind)
	}
	foundBlock := false
	for _, tk := range toks {
		if tk.Kind == token.GO_BLOCK {
			foundBlock = true
			if !strings.Contains(tk.Lexeme, "true && !false") {
				t.Errorf("go block lexeme = %q, expected verbatim Go operators", tk.Lexeme)
			}
		}
	}
	if !foundBlock {
		t.Fatalf("expected GO_BLOCK token in %v", toks)
	}
}

func TestLexAllTestCases(t *testing.T) {
	casesDir := filepath.Join("..", "..", "..", "tests", "cases")
	count := 0
	err := filepath.WalkDir(casesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".cb") {
			return nil
		}
		count++
		rel, _ := filepath.Rel(casesDir, path)
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}
			toks, err := Tokenize(string(data), path)
			if err != nil {
				t.Fatalf("Tokenize(%s) failed: %v", rel, err)
			}
			if len(toks) == 0 || toks[len(toks)-1].Kind != token.EOF {
				t.Fatalf("expected non-empty token slice ending in EOF for %s", rel)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk %s: %v", casesDir, err)
	}
	if count < 6 {
		t.Fatalf("expected at least 6 .cb test cases, found %d", count)
	}
}
