package lexer

import (
	"fmt"
	"strings"
	"unicode"

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

// Lexer scans CleanBasic source text into a stream of tokens.
type Lexer struct {
	file string
	src  []rune
	pos  int
	line int
	col  int
}

// New creates a new Lexer for the given source string and filename.
func New(src string, filename string) *Lexer {
	return &Lexer{
		file: filename,
		src:  []rune(src),
		pos:  0,
		line: 1,
		col:  1,
	}
}

// Tokenize lexes the entire CleanBasic source string into a slice of tokens
// ending with token.EOF. It performs comment skipping, newline coalescing,
// C-style logical operator rejection, two-word keyword coalescing, and
// lexer-level desugaring of $"..." interpolated strings.
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

func (l *Lexer) peek() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peekNext() rune {
	if l.pos+1 >= len(l.src) {
		return 0
	}
	return l.src[l.pos+1]
}

func (l *Lexer) advance() rune {
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

		// Handle comments starting with ' or #
		if ch == 39 || ch == '#' {
			for l.pos < len(l.src) && l.peek() != '\n' {
				l.advance()
			}
			continue
		}

		// Handle interpolated string $"..."
		if ch == '$' && l.peekNext() == '"' {
			interpTokens, err := l.scanInterpString()
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, interpTokens...)
			continue
		}

		// Handle regular string literal "..."
		if ch == '"' {
			tok, err := l.scanString()
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			continue
		}

		// Handle numbers
		if isDigit(ch) {
			tok, err := l.scanNumber()
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			continue
		}

		// Handle identifiers and keywords
		if isIdentStart(ch) {
			tok := l.scanIdentifier()
			tokens = append(tokens, tok)
			continue
		}

		// Handle operators and punctuation
		startPos := l.currentPos()
		switch ch {
		case '&':
			if l.peekNext() == '&' {
				return nil, &LexError{
					Pos:     startPos,
					Message: "C-style logical operator '&&' is forbidden in CleanBasic; use 'and' instead",
				}
			}
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.AMP, Lexeme: "&", Pos: startPos})

		case '|':
			if l.peekNext() == '|' {
				return nil, &LexError{
					Pos:     startPos,
					Message: "C-style logical operator '||' is forbidden in CleanBasic; use 'or' instead",
				}
			}
			return nil, &LexError{
				Pos:     startPos,
				Message: fmt.Sprintf("unexpected character %q", ch),
			}

		case '!':
			if l.peekNext() == '=' {
				l.advance()
				l.advance()
				tokens = append(tokens, token.Token{Kind: token.NEQ, Lexeme: "!=", Pos: startPos})
			} else {
				return nil, &LexError{
					Pos:     startPos,
					Message: "C-style logical operator '!' is forbidden in CleanBasic; use 'not' instead",
				}
			}

		case '=':
			l.advance()
			if l.peek() == '=' {
				l.advance()
				tokens = append(tokens, token.Token{Kind: token.EQ, Lexeme: "==", Pos: startPos})
			} else {
				tokens = append(tokens, token.Token{Kind: token.ASSIGN, Lexeme: "=", Pos: startPos})
			}

		case '<':
			l.advance()
			if l.peek() == '=' {
				l.advance()
				tokens = append(tokens, token.Token{Kind: token.LTE, Lexeme: "<=", Pos: startPos})
			} else {
				tokens = append(tokens, token.Token{Kind: token.LT, Lexeme: "<", Pos: startPos})
			}

		case '>':
			l.advance()
			if l.peek() == '=' {
				l.advance()
				tokens = append(tokens, token.Token{Kind: token.GTE, Lexeme: ">=", Pos: startPos})
			} else {
				tokens = append(tokens, token.Token{Kind: token.GT, Lexeme: ">", Pos: startPos})
			}

		case '+':
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.PLUS, Lexeme: "+", Pos: startPos})
		case '-':
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.MINUS, Lexeme: "-", Pos: startPos})
		case '*':
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.STAR, Lexeme: "*", Pos: startPos})
		case '/':
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.SLASH, Lexeme: "/", Pos: startPos})
		case '%':
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.PERCENT, Lexeme: "%", Pos: startPos})
		case '.':
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.DOT, Lexeme: ".", Pos: startPos})
		case ',':
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.COMMA, Lexeme: ",", Pos: startPos})
		case ':':
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.COLON, Lexeme: ":", Pos: startPos})
		case '(':
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.LPAREN, Lexeme: "(", Pos: startPos})
		case ')':
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.RPAREN, Lexeme: ")", Pos: startPos})
		case '[':
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.LBRACKET, Lexeme: "[", Pos: startPos})
		case ']':
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.RBRACKET, Lexeme: "]", Pos: startPos})
		case '{':
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.LBRACE, Lexeme: "{", Pos: startPos})
		case '}':
			l.advance()
			tokens = append(tokens, token.Token{Kind: token.RBRACE, Lexeme: "}", Pos: startPos})

		default:
			return nil, &LexError{
				Pos:     startPos,
				Message: fmt.Sprintf("unexpected character %q", ch),
			}
		}
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
		sb.WriteRune(l.advance())
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
		sb.WriteRune(l.advance())
	}

	if l.pos < len(l.src) && l.peek() == '.' && isDigit(l.peekNext()) {
		isFloat = true
		sb.WriteRune(l.advance()) // consume '.'
		for l.pos < len(l.src) && isDigit(l.peek()) {
			sb.WriteRune(l.advance())
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
			sb.WriteRune(esc)
			continue
		}
		sb.WriteRune(l.advance())
	}

	return token.Token{}, &LexError{
		Pos:     startPos,
		Message: "unterminated string literal",
	}
}

func (l *Lexer) scanEscapeSequence() (rune, error) {
	escPos := l.currentPos()
	l.advance() // consume '\'
	if l.pos >= len(l.src) {
		return 0, &LexError{
			Pos:     escPos,
			Message: "unterminated escape sequence in string literal",
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

// scanInterpString lexes $"..." and desugars it directly into STRING_LIT, AMP (&),
// and identifier/dot/index path tokens.
func (l *Lexer) scanInterpString() ([]token.Token, error) {
	startPos := l.currentPos()
	l.advance() // consume '$'
	l.advance() // consume '"'

	type segment struct {
		isLiteral bool
		litText   string
		litPos    token.Position
		exprToks  []token.Token
	}

	var segments []segment
	var litBuilder strings.Builder
	litPos := startPos

	flushLiteral := func(force bool) {
		if litBuilder.Len() > 0 || force {
			segments = append(segments, segment{
				isLiteral: true,
				litText:   litBuilder.String(),
				litPos:    litPos,
			})
			litBuilder.Reset()
		}
	}

	for l.pos < len(l.src) {
		ch := l.peek()
		if ch == '"' {
			l.advance() // consume closing '"'
			flushLiteral(len(segments) == 0)

			var out []token.Token
			for i, seg := range segments {
				if i > 0 {
					ampPos := seg.litPos
					if !seg.isLiteral && len(seg.exprToks) > 0 {
						ampPos = seg.exprToks[0].Pos
					}
					out = append(out, token.Token{
						Kind:   token.AMP,
						Lexeme: "&",
						Pos:    ampPos,
					})
				} else if !seg.isLiteral {
					// Ensure an interpolated string starting with "{x}" begins with "" & x
					// so the & operator always coerces x to a string even when there's only 1 placeholder.
					out = append(out,
						token.Token{Kind: token.STRING_LIT, Lexeme: "", Pos: startPos},
						token.Token{Kind: token.AMP, Lexeme: "&", Pos: startPos},
					)
				}

				if seg.isLiteral {
					out = append(out, token.Token{
						Kind:   token.STRING_LIT,
						Lexeme: seg.litText,
						Pos:    seg.litPos,
					})
				} else {
					out = append(out, seg.exprToks...)
				}
			}
			return out, nil
		}

		if ch == '\n' || ch == '\r' {
			return nil, &LexError{
				Pos:     startPos,
				Message: "unterminated interpolated string literal",
			}
		}

		if ch == '\\' {
			esc, err := l.scanEscapeSequence()
			if err != nil {
				return nil, err
			}
			litBuilder.WriteRune(esc)
			continue
		}

		// Escaped "{{" -> literal "{"
		if ch == '{' && l.peekNext() == '{' {
			l.advance()
			l.advance()
			litBuilder.WriteRune('{')
			continue
		}

		// Escaped "}}" -> literal "}"
		if ch == '}' && l.peekNext() == '}' {
			l.advance()
			l.advance()
			litBuilder.WriteRune('}')
			continue
		}

		// Unescaped "}" without matching "{"
		if ch == '}' {
			return nil, &LexError{
				Pos:     l.currentPos(),
				Message: "unescaped '}' in interpolated string (use '}}' for a literal brace)",
			}
		}

		// Start of "{...}" placeholder
		if ch == '{' {
			bracePos := l.currentPos()
			l.advance() // consume '{'
			flushLiteral(false)

			exprToks, err := l.scanInterpPlaceholder(bracePos)
			if err != nil {
				return nil, err
			}
			segments = append(segments, segment{
				isLiteral: false,
				exprToks:  exprToks,
			})
			litPos = l.currentPos()
			continue
		}

		litBuilder.WriteRune(l.advance())
	}

	return nil, &LexError{
		Pos:     startPos,
		Message: "unterminated interpolated string literal",
	}
}

// scanInterpPlaceholder parses the inside of {...} in $"..." up to the matching '}'.
// CleanBasic restricts interpolated placeholders to simple paths: QualifiedIdent
// (e.g. {name}, {u.name}, {e.code}) and simple constant/identifier index lookups
// (e.g. {parts[2]} used in 04_containers_and_strings.cb). Arbitrary expressions,
// binary operators, and function calls inside {...} are rejected with a LexError.
func (l *Lexer) scanInterpPlaceholder(openBracePos token.Position) ([]token.Token, error) {
	l.skipHorizontalWhitespace()
	if l.pos >= len(l.src) {
		return nil, &LexError{
			Pos:     openBracePos,
			Message: "unterminated '{' in interpolated string",
		}
	}
	if l.peek() == '}' {
		return nil, &LexError{
			Pos:     openBracePos,
			Message: "empty '{}' placeholder in interpolated string",
		}
	}

	if !isIdentStart(l.peek()) {
		return nil, &LexError{
			Pos:     l.currentPos(),
			Message: "interpolated string placeholder must start with an identifier path (complex expressions are not allowed inside '{...}')",
		}
	}

	var toks []token.Token
	firstTok := l.scanIdentifier()
	switch firstTok.Kind {
	case token.TRUE, token.FALSE, token.AND, token.OR, token.NOT:
		return nil, &LexError{
			Pos:     firstTok.Pos,
			Message: fmt.Sprintf("keyword %q cannot be used as an identifier in interpolated string placeholder", firstTok.Lexeme),
		}
	}
	// Inside a placeholder path, allow contextual keywords (e.g. type names like str/err) as IDENT.
	if firstTok.Kind != token.IDENT {
		firstTok.Kind = token.IDENT
	}
	toks = append(toks, firstTok)

	for l.pos < len(l.src) {
		ch := l.peek()
		if ch == '.' {
			dotPos := l.currentPos()
			l.advance()
			if l.pos >= len(l.src) || !isIdentStart(l.peek()) {
				return nil, &LexError{
					Pos:     dotPos,
					Message: "expected field identifier after '.' in interpolated string placeholder",
				}
			}
			fieldTok := l.scanIdentifier()
			fieldTok.Kind = token.IDENT
			toks = append(toks,
				token.Token{Kind: token.DOT, Lexeme: ".", Pos: dotPos},
				fieldTok,
			)
			continue
		}

		if ch == '[' {
			lbracketPos := l.currentPos()
			l.advance()
			l.skipHorizontalWhitespace()
			var idxTok token.Token
			if isDigit(l.peek()) {
				numTok, err := l.scanNumber()
				if err != nil {
					return nil, err
				}
				if numTok.Kind != token.INT_LIT {
					return nil, &LexError{
						Pos:     numTok.Pos,
						Message: "only integer or identifier index access is allowed in interpolated string placeholder",
					}
				}
				idxTok = numTok
			} else if isIdentStart(l.peek()) {
				idTok := l.scanIdentifier()
				switch idTok.Kind {
				case token.TRUE, token.FALSE, token.AND, token.OR, token.NOT:
					return nil, &LexError{
						Pos:     idTok.Pos,
						Message: fmt.Sprintf("keyword %q cannot be used as an index identifier in interpolated string placeholder", idTok.Lexeme),
					}
				}
				idTok.Kind = token.IDENT
				idxTok = idTok
			} else {
				return nil, &LexError{
					Pos:     l.currentPos(),
					Message: "complex expressions are not allowed inside interpolated string '{...}' placeholder",
				}
			}
			l.skipHorizontalWhitespace()
			if l.peek() != ']' {
				return nil, &LexError{
					Pos:     l.currentPos(),
					Message: "expected ']' in interpolated string placeholder (complex expressions are not allowed inside '{...}')",
				}
			}
			rbracketPos := l.currentPos()
			l.advance()
			toks = append(toks,
				token.Token{Kind: token.LBRACKET, Lexeme: "[", Pos: lbracketPos},
				idxTok,
				token.Token{Kind: token.RBRACKET, Lexeme: "]", Pos: rbracketPos},
			)
			continue
		}

		break
	}

	l.skipHorizontalWhitespace()
	if l.pos >= len(l.src) || l.peek() == '\n' || l.peek() == '\r' || l.peek() == '"' {
		return nil, &LexError{
			Pos:     openBracePos,
			Message: "unterminated '{' in interpolated string",
		}
	}
	if l.peek() != '}' {
		return nil, &LexError{
			Pos:     l.currentPos(),
			Message: fmt.Sprintf("invalid token %q in interpolated string placeholder; only simple identifier/field paths are allowed inside '{...}'", string(l.peek())),
		}
	}
	l.advance() // consume '}'
	return toks, nil
}

func (l *Lexer) skipHorizontalWhitespace() {
	for l.pos < len(l.src) {
		ch := l.peek()
		if ch == ' ' || ch == '\t' {
			l.advance()
		} else {
			break
		}
	}
}

// coalesceKeywords merges adjacent two-word keywords on the same line
// (e.g., "else" + "if" -> ELSE_IF, "end" + "fn" -> END_FN, "can" + "error" -> CAN_ERROR).
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

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func isIdentStart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch)
}

func isIdentPart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch) || isDigit(ch)
}
