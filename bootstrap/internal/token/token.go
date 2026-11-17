package token

import "fmt"

// Position represents a 1-based source location in a CleanBasic file.
type Position struct {
	File   string
	Line   int
	Column int
}

// String formats the position as file:line:col (or line:col if File is empty).
func (p Position) String() string {
	if p.File != "" {
		return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Column)
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// Kind identifies the category of a lexical token.
type Kind string

const (
	// Special tokens
	EOF     Kind = "EOF"
	NEWLINE Kind = "NEWLINE"

	// Identifiers and literals
	IDENT      Kind = "IDENT"
	INT_LIT    Kind = "INT_LIT"
	FLOAT_LIT  Kind = "FLOAT_LIT"
	STRING_LIT Kind = "STRING_LIT"
	BYTE_LIT   Kind = "BYTE_LIT"
	TRUE       Kind = "true"
	FALSE      Kind = "false"

	// Single-word keywords
	IMPORT   Kind = "import"
	AS       Kind = "as"
	PUBLIC   Kind = "public"
	CONST    Kind = "const"
	DIM      Kind = "dim"
	TYPE     Kind = "type"
	ENUM     Kind = "enum"
	FN       Kind = "fn"
	GO       Kind = "go"
	BYREF    Kind = "byref"
	IF       Kind = "if"
	ELSE     Kind = "else"
	FOR      Kind = "for"
	TO       Kind = "to"
	IN       Kind = "in"
	WHILE    Kind = "while"
	CASE     Kind = "case"
	TRY      Kind = "try"
	CATCH    Kind = "catch"
	DEFER    Kind = "defer"
	RETURN   Kind = "return"
	BREAK    Kind = "break"
	CONTINUE Kind = "continue"
	AND      Kind = "and"
	OR       Kind = "or"
	NOT      Kind = "not"

	// Built-in type / container keywords
	INT     Kind = "int"
	FLOAT   Kind = "float"
	STR     Kind = "str"
	BOOL    Kind = "bool"
	BYTE    Kind = "byte"
	ERR     Kind = "err"
	LIST    Kind = "list"
	DICT    Kind = "dict"
	MAILBOX Kind = "mailbox"

	// Helper / transitional single words used before two-word coalescing
	END    Kind = "end"
	SELECT Kind = "select"
	CAN    Kind = "can"

	// Two-word coalesced keywords
	ELSE_IF     Kind = "else if"
	END_FN      Kind = "end fn"
	END_IF      Kind = "end if"
	END_FOR     Kind = "end for"
	END_WHILE   Kind = "end while"
	END_SELECT  Kind = "end select"
	END_TRY     Kind = "end try"
	END_DEFER   Kind = "end defer"
	END_TYPE    Kind = "end type"
	END_ENUM    Kind = "end enum"
	GO_IMPORT   Kind = "go import"
	SELECT_CASE Kind = "select case"
	CASE_ELSE   Kind = "case else"
	CAN_ERROR   Kind = "can error"
	START_JOB   Kind = "start job"

	// Verbatim Go block token
	GO_BLOCK Kind = "GO_BLOCK"

	// Operators and punctuation
	ASSIGN   Kind = "="
	EQ       Kind = "=="
	NEQ      Kind = "!="
	LT       Kind = "<"
	GT       Kind = ">"
	LTE      Kind = "<="
	GTE      Kind = ">="
	PLUS     Kind = "+"
	MINUS    Kind = "-"
	STAR     Kind = "*"
	SLASH    Kind = "/"
	PERCENT  Kind = "%"
	DOT      Kind = "."
	COMMA    Kind = ","
	COLON    Kind = ":"
	LPAREN   Kind = "("
	RPAREN   Kind = ")"
	LBRACKET Kind = "["
	RBRACKET Kind = "]"
	LBRACE   Kind = "{"
	RBRACE   Kind = "}"
)

// Token represents a single lexical unit with its source position.
type Token struct {
	Kind   Kind
	Lexeme string
	Pos    Position
}

var singleWordKeywords = map[string]Kind{
	"true":     TRUE,
	"false":    FALSE,
	"import":   IMPORT,
	"as":       AS,
	"public":   PUBLIC,
	"const":    CONST,
	"dim":      DIM,
	"type":     TYPE,
	"enum":     ENUM,
	"fn":       FN,
	"go":       GO,
	"byref":    BYREF,
	"if":       IF,
	"else":     ELSE,
	"for":      FOR,
	"to":       TO,
	"in":       IN,
	"while":    WHILE,
	"case":     CASE,
	"try":      TRY,
	"catch":    CATCH,
	"defer":    DEFER,
	"return":   RETURN,
	"break":    BREAK,
	"continue": CONTINUE,
	"and":      AND,
	"or":       OR,
	"not":      NOT,
	"int":      INT,
	"float":    FLOAT,
	"str":      STR,
	"bool":     BOOL,
	"byte":     BYTE,
	"err":      ERR,
	"list":     LIST,
	"dict":     DICT,
	"mailbox":  MAILBOX,
	"end":      END,
	"select":   SELECT,
	"can":      CAN,
}

type twoWordKey struct {
	first  string
	second string
}

var twoWordKeywords = map[twoWordKey]Kind{
	{first: "else", second: "if"}:     ELSE_IF,
	{first: "end", second: "fn"}:      END_FN,
	{first: "end", second: "if"}:      END_IF,
	{first: "end", second: "for"}:     END_FOR,
	{first: "end", second: "while"}:   END_WHILE,
	{first: "end", second: "select"}:  END_SELECT,
	{first: "end", second: "try"}:     END_TRY,
	{first: "end", second: "defer"}:   END_DEFER,
	{first: "end", second: "type"}:    END_TYPE,
	{first: "end", second: "enum"}:    END_ENUM,
	{first: "go", second: "import"}:   GO_IMPORT,
	{first: "select", second: "case"}: SELECT_CASE,
	{first: "case", second: "else"}:   CASE_ELSE,
	{first: "can", second: "error"}:   CAN_ERROR,
	{first: "start", second: "job"}:   START_JOB,
}

// LookupIdent returns the keyword Kind for word, or IDENT if it is an ordinary identifier.
func LookupIdent(word string) Kind {
	if k, ok := singleWordKeywords[word]; ok {
		return k
	}
	return IDENT
}

// CoalesceTwoWordKeyword checks whether two adjacent word lexemes on the same line
// form a two-word CleanBasic keyword (e.g., "else" + "if" -> ELSE_IF).
func CoalesceTwoWordKeyword(first, second string) (Kind, string, bool) {
	if k, ok := twoWordKeywords[twoWordKey{first: first, second: second}]; ok {
		return k, first + " " + second, true
	}
	return "", "", false
}
