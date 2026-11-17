package lexer

import (
	"fmt"
	"strings"

	"cleanbasic/bootstrap/internal/token"
)

// LexError represents a lexical error with an exact file, line, and column position.
type LexError struct {
	Pos     token.Position
	Message string
}

func (e *LexError) Error() string {
	return fmt.Sprintf("%s: syntax error: %s", e.Pos.String(), e.Message)
}

var singleCharTokens = map[byte]token.Kind{
	'+': token.PLUS, '-': token.MINUS, '*': token.STAR, '/': token.SLASH,
	'%': token.PERCENT, '.': token.DOT, ',': token.COMMA, ':': token.COLON,
	'(': token.LPAREN, ')': token.RPAREN, '[': token.LBRACKET, ']': token.RBRACKET,
	'{': token.LBRACE, '}': token.RBRACE,
}

// Lexer scans CleanBasic source text into a stream of tokens.
type Lexer struct {
	file string
	src  string
	pos  int
	line int
	col  int
}

// New creates a new Lexer for the given source string and filename.
func New(src string, filename string) *Lexer {
	return &Lexer{
		file: filename,
		src:  src,
		pos:  0,
		line: 1,
		col:  1,
	}
}

// Tokenize lexes the entire CleanBasic source string into a slice of tokens
// ending with token.EOF. It performs comment skipping, newline coalescing,
// C-style logical operator rejection, and two-word keyword coalescing.
func Tokenize(src string, filename string) ([]token.Token, error) {
	l := New(src, filename)
	raw, err := l.scanAll()
	if err != nil {
		return nil, err
	}
	coalesced := coalesceKeywords(raw)
	return coalesced, nil
}

func (l *Lexer) currentPos() token.Position {
	return token.Position{
		File:   l.file,
		Line:   l.line,
		Column: l.col,
	}
}

func (l *Lexer) peek() byte {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peekNext() byte {
	if l.pos+1 >= len(l.src) {
		return 0
	}
	return l.src[l.pos+1]
}

func (l *Lexer) advance() byte {
	if l.pos >= len(l.src) {
		return 0
	}
	ch := l.src[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func (l *Lexer) scanAll() ([]token.Token, error) {
	var tokens []token.Token

	emitNewline := func(pos token.Position) {
		// Suppress leading blank lines and coalesce consecutive NEWLINE tokens.
		if len(tokens) == 0 {
			return
		}
		if tokens[len(tokens)-1].Kind == token.NEWLINE {
			return
		}
		tokens = append(tokens, token.Token{
			Kind:   token.NEWLINE,
			Lexeme: "\n",
			Pos:    pos,
		})
	}

	for l.pos < len(l.src) {
		ch := l.peek()

		// Handle horizontal whitespace
		if ch == ' ' || ch == '\t' || ch == '\r' {
			l.advance()
			continue
		}

		// Handle newline
		if ch == '\n' {
			pos := l.currentPos()
			l.advance()
			emitNewline(pos)
			continue
		}

		// Handle comments starting with //
		if ch == '/' && l.peekNext() == '/' {
			for l.pos < len(l.src) && l.peek() != '\n' {
				l.advance()
			}
			continue
		}

		// Detect standalone top-level 'go' block at start of line
		if isIdentStart(ch) && (len(tokens) == 0 || tokens[len(tokens)-1].Kind == token.NEWLINE) {
			tok := l.scanIdentifier()
			if tok.Kind == token.GO && l.isRestOfLineEmptyOrComment() {
				blockTok, err := l.scanGoBlock(tok.Pos)
				if err != nil {
					return nil, err
				}
				tokens = append(tokens, blockTok)
				emitNewline(l.currentPos())
				continue
			}
			tokens = append(tokens, tok)
			continue
		}

		tok, err := l.scanNextToken()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
	}

	// Ensure a trailing NEWLINE before EOF if any tokens were emitted and the last token wasn't NEWLINE.
	eofPos := l.currentPos()
	if len(tokens) > 0 && tokens[len(tokens)-1].Kind != token.NEWLINE {
		tokens = append(tokens, token.Token{
			Kind:   token.NEWLINE,
			Lexeme: "\n",
			Pos:    eofPos,
		})
	}
	tokens = append(tokens, token.Token{
		Kind:   token.EOF,
		Lexeme: "",
		Pos:    eofPos,
	})

	return tokens, nil
}

func (l *Lexer) scanIdentifier() token.Token {
	startPos := l.currentPos()
	var sb strings.Builder
	for l.pos < len(l.src) && isIdentPart(l.peek()) {
		sb.WriteByte(l.advance())
	}
	word := sb.String()
	kind := token.LookupIdent(word)
	return token.Token{
		Kind:   kind,
		Lexeme: word,
		Pos:    startPos,
	}
}

func (l *Lexer) scanNumber() (token.Token, error) {
	startPos := l.currentPos()
	var sb strings.Builder
	isFloat := false

	for l.pos < len(l.src) && isDigit(l.peek()) {
		sb.WriteByte(l.advance())
	}

	if l.pos < len(l.src) && l.peek() == '.' && isDigit(l.peekNext()) {
		isFloat = true
		sb.WriteByte(l.advance()) // consume '.'
		for l.pos < len(l.src) && isDigit(l.peek()) {
			sb.WriteByte(l.advance())
		}
	}

	kind := token.INT_LIT
	if isFloat {
		kind = token.FLOAT_LIT
	}
	return token.Token{
		Kind:   kind,
		Lexeme: sb.String(),
		Pos:    startPos,
	}, nil
}

func (l *Lexer) scanString() (token.Token, error) {
	startPos := l.currentPos()
	l.advance() // consume opening '"'

	var sb strings.Builder
	for l.pos < len(l.src) {
		ch := l.peek()
		if ch == '"' {
			l.advance() // consume closing '"'
			return token.Token{
				Kind:   token.STRING_LIT,
				Lexeme: sb.String(),
				Pos:    startPos,
			}, nil
		}
		if ch == '\n' || ch == '\r' {
			return token.Token{}, &LexError{
				Pos:     startPos,
				Message: "unterminated string literal",
			}
		}
		if ch == '\\' {
			esc, err := l.scanEscapeSequence()
			if err != nil {
				return token.Token{}, err
			}
			sb.WriteByte(esc)
			continue
		}
		sb.WriteByte(l.advance())
	}

	return token.Token{}, &LexError{
		Pos:     startPos,
		Message: "unterminated string literal",
	}
}

func (l *Lexer) scanEscapeSequence() (byte, error) {
	escPos := l.currentPos()
	l.advance() // consume '\'
	if l.pos >= len(l.src) {
		return 0, &LexError{
			Pos:     escPos,
			Message: "unterminated escape sequence",
		}
	}
	ch := l.advance()
	switch ch {
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case '\\':
		return '\\', nil
	case '\'':
		return '\'', nil
	case '"':
		return '"', nil
	case '0':
		return 0, nil
	default:
		return 0, &LexError{
			Pos:     escPos,
			Message: fmt.Sprintf("unknown escape sequence \\%c", ch),
		}
	}
}

func (l *Lexer) isRestOfLineEmptyOrComment() bool {
	i := l.pos
	for i < len(l.src) && (l.src[i] == ' ' || l.src[i] == '\t') {
		i++
	}
	if i >= len(l.src) {
		return true
	}
	ch := l.src[i]
	return ch == '\n' || ch == '\r' || (ch == '/' && i+1 < len(l.src) && l.src[i+1] == '/')
}

func (l *Lexer) scanGoBlock(startPos token.Position) (token.Token, error) {
	for l.pos < len(l.src) && l.peek() != '\n' {
		l.advance()
	}
	if l.pos < len(l.src) && l.peek() == '\n' {
		l.advance()
	}

	var code strings.Builder
	for l.pos < len(l.src) {
		lineStart := l.pos
		for l.pos < len(l.src) && l.peek() != '\n' {
			l.advance()
		}
		lineText := l.src[lineStart:l.pos]
		if l.pos < len(l.src) && l.peek() == '\n' {
			l.advance()
		}

		trimmed := strings.TrimSpace(strings.TrimSuffix(lineText, "\r"))
		if isEndGoDirective(trimmed) {
			return token.Token{
				Kind:   token.GO_BLOCK,
				Lexeme: code.String(),
				Pos:    startPos,
			}, nil
		}
		code.WriteString(strings.TrimSuffix(lineText, "\r"))
		code.WriteByte('\n')
	}

	return token.Token{}, &LexError{
		Pos:     startPos,
		Message: "unterminated 'go' block (expected 'end go')",
	}
}

func isEndGoDirective(trimmed string) bool {
	if !strings.HasPrefix(trimmed, "end") {
		return false
	}
	rest := trimmed[len("end"):]
	if len(rest) == 0 || (rest[0] != ' ' && rest[0] != '\t') {
		return false
	}
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, "go") {
		return false
	}
	afterGo := rest[len("go"):]
	if len(afterGo) == 0 {
		return true
	}
	if afterGo[0] != ' ' && afterGo[0] != '\t' && !strings.HasPrefix(afterGo, "//") {
		return false
	}
	afterGo = strings.TrimLeft(afterGo, " \t")
	return len(afterGo) == 0 || strings.HasPrefix(afterGo, "//")
}

func (l *Lexer) scanByteLiteral() (token.Token, error) {
	startPos := l.currentPos()
	l.advance() // consume opening single quote
	if l.pos >= len(l.src) || l.peek() == '\n' || l.peek() == '\r' {
		return token.Token{}, &LexError{
			Pos:     startPos,
			Message: "unterminated byte literal",
		}
	}
	if l.peek() == '\'' {
		return token.Token{}, &LexError{
			Pos:     startPos,
			Message: "empty byte literal",
		}
	}

	var b byte
	if l.peek() == '\\' {
		esc, err := l.scanEscapeSequence()
		if err != nil {
			return token.Token{}, err
		}
		b = esc
	} else {
		ch := l.advance()
		if ch >= 0x80 {
			return token.Token{}, &LexError{
				Pos:     startPos,
				Message: "byte literal must be a single ASCII character",
			}
		}
		b = ch
	}

	if l.pos >= len(l.src) || l.peek() == '\n' || l.peek() == '\r' {
		return token.Token{}, &LexError{
			Pos:     startPos,
			Message: "unterminated byte literal",
		}
	}
	if l.peek() != '\'' {
		return token.Token{}, &LexError{
			Pos:     startPos,
			Message: "byte literal may only contain a single character",
		}
	}
	l.advance() // consume closing single quote
	return token.Token{Kind: token.BYTE_LIT, Lexeme: string([]byte{b}), Pos: startPos}, nil
}

func (l *Lexer) scanNextToken() (token.Token, error) {
	ch := l.peek()
	if ch == '"' {
		return l.scanString()
	}
	if ch == '\'' {
		return l.scanByteLiteral()
	}
	if isDigit(ch) {
		return l.scanNumber()
	}
	if isIdentStart(ch) {
		return l.scanIdentifier(), nil
	}

	startPos := l.currentPos()
	switch ch {
	case '&':
		if l.peekNext() == '&' {
			return token.Token{}, &LexError{Pos: startPos, Message: "C-style logical operator '&&' is forbidden in CleanBasic; use 'and' instead"}
		}
		return token.Token{}, &LexError{Pos: startPos, Message: fmt.Sprintf("unexpected character %q", ch)}
	case '|':
		if l.peekNext() == '|' {
			return token.Token{}, &LexError{Pos: startPos, Message: "C-style logical operator '||' is forbidden in CleanBasic; use 'or' instead"}
		}
		return token.Token{}, &LexError{Pos: startPos, Message: fmt.Sprintf("unexpected character %q", ch)}
	case '!':
		if l.peekNext() == '=' {
			l.advance()
			l.advance()
			return token.Token{Kind: token.NEQ, Lexeme: "!=", Pos: startPos}, nil
		}
		return token.Token{}, &LexError{Pos: startPos, Message: "C-style logical operator '!' is forbidden in CleanBasic; use 'not' instead"}
	case '=':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return token.Token{Kind: token.EQ, Lexeme: "==", Pos: startPos}, nil
		}
		return token.Token{Kind: token.ASSIGN, Lexeme: "=", Pos: startPos}, nil
	case '<':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return token.Token{Kind: token.LTE, Lexeme: "<=", Pos: startPos}, nil
		}
		return token.Token{Kind: token.LT, Lexeme: "<", Pos: startPos}, nil
	case '>':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return token.Token{Kind: token.GTE, Lexeme: ">=", Pos: startPos}, nil
		}
		return token.Token{Kind: token.GT, Lexeme: ">", Pos: startPos}, nil
	default:
		if kind, ok := singleCharTokens[ch]; ok {
			l.advance()
			return token.Token{Kind: kind, Lexeme: string(ch), Pos: startPos}, nil
		}
		return token.Token{}, &LexError{Pos: startPos, Message: fmt.Sprintf("unexpected character %q", ch)}
	}
}

func coalesceKeywords(raw []token.Token) []token.Token {
	out := make([]token.Token, 0, len(raw))
	i := 0
	for i < len(raw) {
		if i+1 < len(raw) && raw[i].Pos.Line == raw[i+1].Pos.Line {
			if kind, lexeme, ok := token.CoalesceTwoWordKeyword(raw[i].Lexeme, raw[i+1].Lexeme); ok {
				out = append(out, token.Token{
					Kind:   kind,
					Lexeme: lexeme,
					Pos:    raw[i].Pos,
				})
				i += 2
				continue
			}
		}
		out = append(out, raw[i])
		i++
	}
	return out
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || isDigit(ch)
}
