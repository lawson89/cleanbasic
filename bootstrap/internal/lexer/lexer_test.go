package lexer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cleanbasic/bootstrap/internal/token"
)

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
		"end union",
		"select case",
		"case else",
		"can error",
		"import header",
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
		token.END_UNION, token.NEWLINE,
		token.SELECT_CASE, token.NEWLINE,
		token.CASE_ELSE, token.NEWLINE,
		token.CAN_ERROR, token.NEWLINE,
		token.IMPORT_HEADER, token.NEWLINE,
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
	src := "print($\"Hello {user.name}, score={{100}} and {parts[2]}!\")\n"
	toks, err := Tokenize(src, "interp.cb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expected sequence:
	// IDENT("print") (
	//   STRING_LIT("Hello ") & IDENT("user") . IDENT("name") & STRING_LIT(", score={100} and ") & IDENT("parts") [ INT_LIT("2") ] & STRING_LIT("!")
	// ) NEWLINE EOF
	wantKinds := []token.Kind{
		token.IDENT, token.LPAREN,
		token.STRING_LIT,
		token.AMP, token.IDENT, token.DOT, token.IDENT,
		token.AMP, token.STRING_LIT,
		token.AMP, token.IDENT, token.LBRACKET, token.INT_LIT, token.RBRACKET,
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
	if toks[8].Lexeme != ", score={100} and " {
		t.Errorf("escaped brace segment = %q, want %q", toks[8].Lexeme, ", score={100} and ")
	}
}

func TestInterpolatedStringRejectsComplexExpressions(t *testing.T) {
	badInputs := []string{
		"print($\"sum={a + b}\")\n",
		"print($\"call={foo(x)}\")\n",
		"print($\"empty={}\")\n",
	}
	for _, src := range badInputs {
		_, err := Tokenize(src, "bad_interp.cb")
		if err == nil {
			t.Errorf("expected error for complex interpolation %q, got nil", src)
		}
	}
}

func TestLexAllTestCases(t *testing.T) {
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
			toks, err := Tokenize(string(data), path)
			if err != nil {
				t.Fatalf("Tokenize(%s) failed: %v", entry.Name(), err)
			}
			if len(toks) == 0 || toks[len(toks)-1].Kind != token.EOF {
				t.Fatalf("expected non-empty token slice ending in EOF for %s", entry.Name())
			}
		})
	}
	if count < 6 {
		t.Fatalf("expected at least 6 .cb test cases, found %d", count)
	}
}
