package parser

import (
	"fmt"
	"slices"
	"strconv"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/lexer"
	"cleanbasic/bootstrap/internal/token"
)

// ParseError represents a syntax error with an exact file, line, and column position.
type ParseError struct {
	Pos     token.Position
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: syntax error: %s", e.Pos.String(), e.Message)
}

// Parser implements a recursive-descent parser for CleanBasic.
type Parser struct {
	file   string
	tokens []token.Token
	pos    int
}

// New creates a Parser from a slice of tokens.
func New(tokens []token.Token, filename string) *Parser {
	return &Parser{
		file:   filename,
		tokens: tokens,
		pos:    0,
	}
}

// Parse lexes and parses CleanBasic source code into an *ast.Program.
func Parse(src string, filename string) (*ast.Program, error) {
	tokens, err := lexer.Tokenize(src, filename)
	if err != nil {
		return nil, err
	}
	p := New(tokens, filename)
	return p.ParseProgram()
}

// ParseTokens parses a pre-tokenized stream into an *ast.Program.
func ParseTokens(tokens []token.Token, filename string) (*ast.Program, error) {
	p := New(tokens, filename)
	return p.ParseProgram()
}

func (p *Parser) cur() token.Token {
	if p.pos >= len(p.tokens) {
		if len(p.tokens) > 0 {
			last := p.tokens[len(p.tokens)-1]
			return token.Token{Kind: token.EOF, Pos: last.Pos}
		}
		return token.Token{Kind: token.EOF, Pos: token.Position{File: p.file, Line: 1, Column: 1}}
	}
	return p.tokens[p.pos]
}

func (p *Parser) peek(offset int) token.Token {
	idx := p.pos + offset
	if idx >= len(p.tokens) {
		if len(p.tokens) > 0 {
			last := p.tokens[len(p.tokens)-1]
			return token.Token{Kind: token.EOF, Pos: last.Pos}
		}
		return token.Token{Kind: token.EOF, Pos: token.Position{File: p.file, Line: 1, Column: 1}}
	}
	return p.tokens[idx]
}

func (p *Parser) advance() token.Token {
	tok := p.cur()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

func (p *Parser) match(kind token.Kind) bool {
	if p.cur().Kind == kind {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) expect(kind token.Kind, context string) (token.Token, error) {
	tok := p.cur()
	if tok.Kind != kind {
		return token.Token{}, &ParseError{
			Pos:     tok.Pos,
			Message: fmt.Sprintf("expected %s %s, got %s (%q)", kind, context, tok.Kind, tok.Lexeme),
		}
	}
	p.advance()
	return tok, nil
}

func (p *Parser) expectIdent(context string) (token.Token, error) {
	tok := p.cur()
	if tok.Kind == token.IDENT {
		p.advance()
		return tok, nil
	}
	return token.Token{}, &ParseError{
		Pos:     tok.Pos,
		Message: fmt.Sprintf("expected identifier %s, got %s (%q)", context, tok.Kind, tok.Lexeme),
	}
}

// expectFieldOrMethodIdent allows ordinary identifiers as well as contextual words
// that may appear after '.' (e.g., str.join, d.get, etc.).
func (p *Parser) expectFieldOrMethodIdent(context string) (token.Token, error) {
	tok := p.cur()
	if slices.Contains([]token.Kind{token.IDENT, token.STR, token.LIST, token.DICT, token.ERR}, tok.Kind) {
		p.advance()
		return tok, nil
	}
	return token.Token{}, &ParseError{
		Pos:     tok.Pos,
		Message: fmt.Sprintf("expected identifier %s, got %s (%q)", context, tok.Kind, tok.Lexeme),
	}
}

func (p *Parser) skipNewlines() {
	for p.cur().Kind == token.NEWLINE {
		p.advance()
	}
}

func (p *Parser) expectNewline(context string) error {
	tok := p.cur()
	if tok.Kind == token.NEWLINE {
		p.skipNewlines()
		return nil
	}
	if tok.Kind == token.EOF {
		return nil
	}
	return &ParseError{
		Pos:     tok.Pos,
		Message: fmt.Sprintf("expected newline after %s, got %s (%q)", context, tok.Kind, tok.Lexeme),
	}
}

func (p *Parser) parseStmtBlock(stopKinds ...token.Kind) ([]ast.Statement, error) {
	var body []ast.Statement
	for p.cur().Kind != token.EOF && !slices.Contains(stopKinds, p.cur().Kind) {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		body = append(body, stmt)
		p.skipNewlines()
	}
	return body, nil
}

// ParseProgram parses: Program ::= TopLevelDecl*
func (p *Parser) ParseProgram() (*ast.Program, error) {
	prog := &ast.Program{
		File: p.file,
	}

	p.skipNewlines()
	for p.cur().Kind != token.EOF {
		decl, err := p.parseTopLevelDecl()
		if err != nil {
			return nil, err
		}
		prog.Decls = append(prog.Decls, decl)
		p.skipNewlines()
	}

	return prog, nil
}

func (p *Parser) parseTopLevelDecl() (ast.TopLevelDecl, error) {
	p.skipNewlines()
	startTok := p.cur()

	if startTok.Kind == token.IMPORT || startTok.Kind == token.IMPORT_HEADER {
		return p.parseImportDecl()
	}

	isPublic := false
	if p.cur().Kind == token.PUBLIC {
		isPublic = true
		p.advance()
	}

	switch p.cur().Kind {
	case token.CONST:
		return p.parseConstDecl(startTok.Pos, isPublic)
	case token.TYPE:
		return p.parseTypeDecl(startTok.Pos, isPublic)
	case token.ENUM:
		return p.parseEnumDecl(startTok.Pos, isPublic)
	case token.UNION:
		return p.parseUnionDecl(startTok.Pos, isPublic)
	case token.FN, token.EXTERN:
		return p.parseFnDecl(startTok.Pos, isPublic)
	default:
		tok := p.cur()
		return nil, &ParseError{
			Pos:     tok.Pos,
			Message: fmt.Sprintf("unexpected token %s (%q) at top level; expected import, const, type, enum, union, or fn", tok.Kind, tok.Lexeme),
		}
	}
}

// ImportDecl ::= ("import" | "import header") StringLiteral ["as" Identifier] NEWLINE
func (p *Parser) parseImportDecl() (*ast.ImportDecl, error) {
	startTok := p.advance() // consume IMPORT or IMPORT_HEADER
	isHeader := startTok.Kind == token.IMPORT_HEADER

	pathTok, err := p.expect(token.STRING_LIT, "in import declaration")
	if err != nil {
		return nil, err
	}

	alias := ""
	if p.match(token.AS) {
		aliasTok, err := p.expectIdent("after 'as' in import declaration")
		if err != nil {
			return nil, err
		}
		alias = aliasTok.Lexeme
	}

	if err := p.expectNewline("import declaration"); err != nil {
		return nil, err
	}

	return &ast.ImportDecl{
		Pos:      startTok.Pos,
		IsHeader: isHeader,
		Path:     pathTok.Lexeme,
		Alias:    alias,
	}, nil
}

// ConstDecl ::= ["public"] "const" Identifier ["as" Type] "=" Expr NEWLINE
func (p *Parser) parseConstDecl(pos token.Position, isPublic bool) (*ast.ConstDecl, error) {
	if _, err := p.expect(token.CONST, "in const declaration"); err != nil {
		return nil, err
	}

	nameTok, err := p.expectIdent("in const declaration")
	if err != nil {
		return nil, err
	}

	var explicitType *ast.TypeExpr
	if p.match(token.AS) {
		t, err := p.parseType()
		if err != nil {
			return nil, err
		}
		explicitType = &t
	}

	if _, err := p.expect(token.ASSIGN, "in const declaration"); err != nil {
		return nil, err
	}

	val, err := p.parseExpr()
	if err != nil {
		return nil, err
	}

	if err := p.expectNewline("const declaration"); err != nil {
		return nil, err
	}

	return &ast.ConstDecl{
		Pos:    pos,
		Public: isPublic,
		Name:   nameTok.Lexeme,
		Type:   explicitType,
		Value:  val,
	}, nil
}

// TypeDecl ::= ["public"] "type" Identifier NEWLINE FieldDecl* "end type" NEWLINE
// FieldDecl ::= Identifier "as" Type NEWLINE
func (p *Parser) parseTypeDecl(pos token.Position, isPublic bool) (*ast.TypeDecl, error) {
	if _, err := p.expect(token.TYPE, "in type declaration"); err != nil {
		return nil, err
	}

	nameTok, err := p.expectIdent("for type name")
	if err != nil {
		return nil, err
	}

	if err := p.expectNewline("type header"); err != nil {
		return nil, err
	}

	var fields []ast.FieldDecl
	for p.cur().Kind != token.END_TYPE && p.cur().Kind != token.EOF {
		fieldTok, err := p.expectIdent("for struct field name")
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(token.AS, "after field name"); err != nil {
			return nil, err
		}
		fieldType, err := p.parseType()
		if err != nil {
			return nil, err
		}
		if err := p.expectNewline("field declaration"); err != nil {
			return nil, err
		}
		fields = append(fields, ast.FieldDecl{
			Pos:  fieldTok.Pos,
			Name: fieldTok.Lexeme,
			Type: fieldType,
		})
	}

	if _, err := p.expect(token.END_TYPE, "to close type declaration"); err != nil {
		return nil, err
	}
	if err := p.expectNewline("end type"); err != nil {
		return nil, err
	}

	return &ast.TypeDecl{
		Pos:    pos,
		Public: isPublic,
		Name:   nameTok.Lexeme,
		Fields: fields,
	}, nil
}

// EnumDecl ::= ["public"] "enum" Identifier NEWLINE Identifier* "end enum" NEWLINE
func (p *Parser) parseEnumDecl(pos token.Position, isPublic bool) (*ast.EnumDecl, error) {
	if _, err := p.expect(token.ENUM, "in enum declaration"); err != nil {
		return nil, err
	}

	nameTok, err := p.expectIdent("for enum name")
	if err != nil {
		return nil, err
	}

	if err := p.expectNewline("enum header"); err != nil {
		return nil, err
	}

	var variants []string
	for p.cur().Kind != token.END_ENUM && p.cur().Kind != token.EOF {
		varTok, err := p.expectIdent("for enum variant")
		if err != nil {
			return nil, err
		}
		variants = append(variants, varTok.Lexeme)
		// Allow optional comma or newline between enum variants
		if p.match(token.COMMA) {
			p.skipNewlines()
		} else {
			if err := p.expectNewline("enum variant"); err != nil {
				return nil, err
			}
		}
	}

	if _, err := p.expect(token.END_ENUM, "to close enum declaration"); err != nil {
		return nil, err
	}
	if err := p.expectNewline("end enum"); err != nil {
		return nil, err
	}

	return &ast.EnumDecl{
		Pos:      pos,
		Public:   isPublic,
		Name:     nameTok.Lexeme,
		Variants: variants,
	}, nil
}

// UnionDecl ::= ["public"] "union" Identifier NEWLINE UnionVariant+ "end union" NEWLINE
// UnionVariant ::= Identifier "as" Type NEWLINE
func (p *Parser) parseUnionDecl(pos token.Position, isPublic bool) (*ast.UnionDecl, error) {
	if _, err := p.expect(token.UNION, "in union declaration"); err != nil {
		return nil, err
	}

	nameTok, err := p.expectIdent("for union name")
	if err != nil {
		return nil, err
	}

	if err := p.expectNewline("union header"); err != nil {
		return nil, err
	}

	var variants []ast.UnionVariant
	for p.cur().Kind != token.END_UNION && p.cur().Kind != token.EOF {
		varTok, err := p.expectIdent("for union variant name")
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(token.AS, "after union variant name"); err != nil {
			return nil, err
		}
		varType, err := p.parseType()
		if err != nil {
			return nil, err
		}
		if err := p.expectNewline("union variant"); err != nil {
			return nil, err
		}
		variants = append(variants, ast.UnionVariant{
			Pos:  varTok.Pos,
			Name: varTok.Lexeme,
			Type: varType,
		})
	}

	if len(variants) == 0 {
		return nil, &ParseError{
			Pos:     pos,
			Message: fmt.Sprintf("union %q must declare at least one variant", nameTok.Lexeme),
		}
	}

	if _, err := p.expect(token.END_UNION, "to close union declaration"); err != nil {
		return nil, err
	}
	if err := p.expectNewline("end union"); err != nil {
		return nil, err
	}

	return &ast.UnionDecl{
		Pos:      pos,
		Public:   isPublic,
		Name:     nameTok.Lexeme,
		Variants: variants,
	}, nil
}

// FnDecl ::= ["public"] ["extern"] "fn" Identifier "(" [ParamList] ")" ["as" Type] ["can error"] NEWLINE ...
func (p *Parser) parseFnDecl(pos token.Position, isPublic bool) (*ast.FnDecl, error) {
	isExtern := false
	if p.match(token.EXTERN) {
		isExtern = true
	}

	if _, err := p.expect(token.FN, "in function declaration"); err != nil {
		return nil, err
	}

	nameTok, err := p.expectIdent("for function name")
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(token.LPAREN, "after function name"); err != nil {
		return nil, err
	}

	params, err := p.parseParamList()
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(token.RPAREN, "after parameter list"); err != nil {
		return nil, err
	}

	var retType *ast.TypeExpr
	if p.match(token.AS) {
		t, err := p.parseType()
		if err != nil {
			return nil, err
		}
		retType = &t
	}

	canError := false
	if p.match(token.CAN_ERROR) {
		canError = true
	}

	if err := p.expectNewline("function signature"); err != nil {
		return nil, err
	}

	if isExtern {
		return &ast.FnDecl{
			Pos:        pos,
			Public:     isPublic,
			Extern:     true,
			Name:       nameTok.Lexeme,
			Params:     params,
			ReturnType: retType,
			CanError:   canError,
		}, nil
	}

	body, err := p.parseStmtBlock(token.END_FN)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(token.END_FN, "to close function declaration"); err != nil {
		return nil, err
	}
	if err := p.expectNewline("end fn"); err != nil {
		return nil, err
	}

	return &ast.FnDecl{
		Pos:        pos,
		Public:     isPublic,
		Extern:     false,
		Name:       nameTok.Lexeme,
		Params:     params,
		ReturnType: retType,
		CanError:   canError,
		Body:       body,
	}, nil
}

// ParamList ::= Param ("," Param)* [","]
// Param ::= ["byref"] Identifier "as" Type
func (p *Parser) parseParamList() ([]ast.Param, error) {
	var params []ast.Param
	p.skipNewlines()
	if p.cur().Kind == token.RPAREN {
		return params, nil
	}

	for {
		p.skipNewlines()
		if p.cur().Kind == token.RPAREN {
			break
		}
		paramPos := p.cur().Pos
		byRef := p.match(token.BYREF)

		nameTok, err := p.expectIdent("for parameter name")
		if err != nil {
			return nil, err
		}

		if _, err := p.expect(token.AS, "after parameter name"); err != nil {
			return nil, err
		}

		paramType, err := p.parseType()
		if err != nil {
			return nil, err
		}

		params = append(params, ast.Param{
			Pos:   paramPos,
			ByRef: byRef,
			Name:  nameTok.Lexeme,
			Type:  paramType,
		})

		p.skipNewlines()
		if p.match(token.COMMA) {
			p.skipNewlines()
			// Optional trailing comma before ')'
			if p.cur().Kind == token.RPAREN {
				break
			}
			continue
		}
		break
	}

	return params, nil
}

// Type ::= "int" | "float" | "str" | "bool" | "err" | "cptr"
//
//	| "list" "[" Type "]"
//	| "dict" "[" Type "," Type "]"
//	| QualifiedIdent
func (p *Parser) parseType() (ast.TypeExpr, error) {
	tok := p.cur()
	switch tok.Kind {
	case token.INT, token.FLOAT, token.STR, token.BOOL, token.ERR, token.CPTR:
		p.advance()
		return ast.TypeExpr{
			Pos:  tok.Pos,
			Kind: ast.TypeBasic,
			Name: tok.Lexeme,
		}, nil

	case token.LIST:
		p.advance()
		if _, err := p.expect(token.LBRACKET, "after 'list'"); err != nil {
			return ast.TypeExpr{}, err
		}
		elemType, err := p.parseType()
		if err != nil {
			return ast.TypeExpr{}, err
		}
		if _, err := p.expect(token.RBRACKET, "after list element type"); err != nil {
			return ast.TypeExpr{}, err
		}
		return ast.TypeExpr{
			Pos:      tok.Pos,
			Kind:     ast.TypeList,
			ElemType: &elemType,
		}, nil

	case token.DICT:
		p.advance()
		if _, err := p.expect(token.LBRACKET, "after 'dict'"); err != nil {
			return ast.TypeExpr{}, err
		}
		keyType, err := p.parseType()
		if err != nil {
			return ast.TypeExpr{}, err
		}
		if _, err := p.expect(token.COMMA, "between dict key and value types"); err != nil {
			return ast.TypeExpr{}, err
		}
		valType, err := p.parseType()
		if err != nil {
			return ast.TypeExpr{}, err
		}
		if _, err := p.expect(token.RBRACKET, "after dict value type"); err != nil {
			return ast.TypeExpr{}, err
		}
		return ast.TypeExpr{
			Pos:     tok.Pos,
			Kind:    ast.TypeDict,
			KeyType: &keyType,
			ValType: &valType,
		}, nil

	case token.IDENT:
		p.advance()
		parts := []string{tok.Lexeme}
		for p.match(token.DOT) {
			partTok, err := p.expectIdent("after '.' in qualified type")
			if err != nil {
				return ast.TypeExpr{}, err
			}
			parts = append(parts, partTok.Lexeme)
		}
		return ast.TypeExpr{
			Pos:   tok.Pos,
			Kind:  ast.TypeQualified,
			Parts: parts,
		}, nil

	default:
		return ast.TypeExpr{}, &ParseError{
			Pos:     tok.Pos,
			Message: fmt.Sprintf("expected type name, got %s (%q)", tok.Kind, tok.Lexeme),
		}
	}
}

// --- Statements ---

func (p *Parser) parseStatement() (ast.Statement, error) {
	p.skipNewlines()
	tok := p.cur()

	switch tok.Kind {
	case token.DIM, token.CONST:
		return p.parseVarDecl()
	case token.IF:
		return p.parseIfStmt()
	case token.FOR:
		return p.parseForStmt()
	case token.WHILE:
		return p.parseWhileStmt()
	case token.SELECT_CASE:
		return p.parseSelectStmt()
	case token.TRY:
		return p.parseTryCatchStmt()
	case token.DEFER:
		return p.parseDeferStmt()
	case token.RETURN:
		return p.parseReturnStmt()
	case token.BREAK:
		p.advance()
		if err := p.expectNewline("break statement"); err != nil {
			return nil, err
		}
		return &ast.BreakStmt{Pos: tok.Pos}, nil
	case token.CONTINUE:
		p.advance()
		if err := p.expectNewline("continue statement"); err != nil {
			return nil, err
		}
		return &ast.ContinueStmt{Pos: tok.Pos}, nil
	default:
		return p.parseAssignOrExprStmt()
	}
}

// VarDecl ::= ("dim" | "const") Identifier ["as" Type] "=" Expr NEWLINE
func (p *Parser) parseVarDecl() (*ast.VarDecl, error) {
	kwTok := p.advance()
	isConst := kwTok.Kind == token.CONST

	nameTok, err := p.expectIdent("in variable declaration")
	if err != nil {
		return nil, err
	}

	var explicitType *ast.TypeExpr
	if p.match(token.AS) {
		t, err := p.parseType()
		if err != nil {
			return nil, err
		}
		explicitType = &t
	}

	if _, err := p.expect(token.ASSIGN, "in variable declaration"); err != nil {
		return nil, err
	}

	val, err := p.parseExpr()
	if err != nil {
		return nil, err
	}

	if err := p.expectNewline("variable declaration"); err != nil {
		return nil, err
	}

	return &ast.VarDecl{
		Pos:     kwTok.Pos,
		IsConst: isConst,
		Name:    nameTok.Lexeme,
		Type:    explicitType,
		Value:   val,
	}, nil
}

// Assignment ::= Expr "=" Expr NEWLINE
// ExprStmt   ::= Expr NEWLINE
func (p *Parser) parseAssignOrExprStmt() (ast.Statement, error) {
	startPos := p.cur().Pos
	lhs, err := p.parseExpr()
	if err != nil {
		return nil, err
	}

	if p.match(token.ASSIGN) {
		rhs, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if err := p.expectNewline("assignment statement"); err != nil {
			return nil, err
		}
		return &ast.Assignment{
			Pos:    startPos,
			Target: lhs,
			Value:  rhs,
		}, nil
	}

	if err := p.expectNewline("expression statement"); err != nil {
		return nil, err
	}
	return &ast.ExprStmt{
		Pos:  startPos,
		Expr: lhs,
	}, nil
}

// IfStmt ::= "if" Expr NEWLINE Statement* ("else if" Expr NEWLINE Statement*)* ["else" NEWLINE Statement*] "end if" NEWLINE
func (p *Parser) parseIfStmt() (*ast.IfStmt, error) {
	ifTok, err := p.expect(token.IF, "at start of if statement")
	if err != nil {
		return nil, err
	}

	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if err := p.expectNewline("if condition"); err != nil {
		return nil, err
	}

	thenBody, err := p.parseStmtBlock(token.ELSE_IF, token.ELSE, token.END_IF)
	if err != nil {
		return nil, err
	}

	var elseIfs []ast.ElseIfBranch
	for p.cur().Kind == token.ELSE_IF {
		elifTok := p.advance()
		elifCond, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if err := p.expectNewline("else if condition"); err != nil {
			return nil, err
		}

		elifBody, err := p.parseStmtBlock(token.ELSE_IF, token.ELSE, token.END_IF)
		if err != nil {
			return nil, err
		}
		elseIfs = append(elseIfs, ast.ElseIfBranch{
			Pos:  elifTok.Pos,
			Cond: elifCond,
			Body: elifBody,
		})
	}

	hasElse := false
	var elseBody []ast.Statement
	if p.match(token.ELSE) {
		hasElse = true
		if err := p.expectNewline("else"); err != nil {
			return nil, err
		}
		elseBody, err = p.parseStmtBlock(token.END_IF)
		if err != nil {
			return nil, err
		}
	}

	if _, err := p.expect(token.END_IF, "to close if statement"); err != nil {
		return nil, err
	}
	if err := p.expectNewline("end if"); err != nil {
		return nil, err
	}

	return &ast.IfStmt{
		Pos:      ifTok.Pos,
		Cond:     cond,
		ThenBody: thenBody,
		ElseIfs:  elseIfs,
		ElseBody: elseBody,
		HasElse:  hasElse,
	}, nil
}

// ForStmt ::= "for" Identifier "=" Expr "to" Expr ["step" Expr] NEWLINE Statement* "end for" NEWLINE
//
//	| "for" IdentList "in" Expr NEWLINE Statement* "end for" NEWLINE
func (p *Parser) parseForStmt() (*ast.ForStmt, error) {
	forTok, err := p.expect(token.FOR, "at start of for statement")
	if err != nil {
		return nil, err
	}

	firstIdent, err := p.expectIdent("after 'for'")
	if err != nil {
		return nil, err
	}

	// Range loop: for i = start to end [step s]
	if p.match(token.ASSIGN) {
		startExpr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(token.TO, "in numeric for loop"); err != nil {
			return nil, err
		}
		endExpr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		var stepExpr ast.Expr
		if p.match(token.STEP) {
			stepExpr, err = p.parseExpr()
			if err != nil {
				return nil, err
			}
		}
		return p.finishForStmt(&ast.ForStmt{
			Pos:     forTok.Pos,
			IsRange: true,
			VarName: firstIdent.Lexeme,
			Start:   startExpr,
			End:     endExpr,
			Step:    stepExpr,
		}, "for loop header")
	}

	// Collection loop: for x [, y] in iterable
	vars := []string{firstIdent.Lexeme}
	for p.match(token.COMMA) {
		nextIdent, err := p.expectIdent("in for-in variable list")
		if err != nil {
			return nil, err
		}
		vars = append(vars, nextIdent.Lexeme)
	}

	if _, err := p.expect(token.IN, "in for-in loop"); err != nil {
		return nil, err
	}

	iterable, err := p.parseExpr()
	if err != nil {
		return nil, err
	}

	return p.finishForStmt(&ast.ForStmt{
		Pos:      forTok.Pos,
		IsRange:  false,
		Vars:     vars,
		Iterable: iterable,
	}, "for-in loop header")
}

func (p *Parser) finishForStmt(stmt *ast.ForStmt, headerCtx string) (*ast.ForStmt, error) {
	if err := p.expectNewline(headerCtx); err != nil {
		return nil, err
	}
	body, err := p.parseStmtBlock(token.END_FOR)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(token.END_FOR, "to close for loop"); err != nil {
		return nil, err
	}
	if err := p.expectNewline("end for"); err != nil {
		return nil, err
	}
	stmt.Body = body
	return stmt, nil
}

// WhileStmt ::= "while" Expr NEWLINE Statement* "end while" NEWLINE
func (p *Parser) parseWhileStmt() (*ast.WhileStmt, error) {
	whileTok, err := p.expect(token.WHILE, "at start of while statement")
	if err != nil {
		return nil, err
	}

	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if err := p.expectNewline("while condition"); err != nil {
		return nil, err
	}

	body, err := p.parseStmtBlock(token.END_WHILE)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(token.END_WHILE, "to close while loop"); err != nil {
		return nil, err
	}
	if err := p.expectNewline("end while"); err != nil {
		return nil, err
	}

	return &ast.WhileStmt{
		Pos:  whileTok.Pos,
		Cond: cond,
		Body: body,
	}, nil
}

// SelectStmt ::= "select case" Expr NEWLINE ("case" ExprList ["as" Identifier] NEWLINE Statement*)* ["case else" NEWLINE Statement*] "end select" NEWLINE
func (p *Parser) parseSelectStmt() (*ast.SelectStmt, error) {
	selTok, err := p.expect(token.SELECT_CASE, "at start of select case statement")
	if err != nil {
		return nil, err
	}

	target, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if err := p.expectNewline("select case expression"); err != nil {
		return nil, err
	}

	var cases []ast.CaseClause
	for p.cur().Kind == token.CASE {
		caseTok := p.advance()
		var values []ast.Expr
		firstVal, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		values = append(values, firstVal)
		for p.match(token.COMMA) {
			nextVal, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			values = append(values, nextVal)
		}

		binding := ""
		if p.match(token.AS) {
			bindTok, err := p.expectIdent("after 'as' in case clause")
			if err != nil {
				return nil, err
			}
			binding = bindTok.Lexeme
		}

		if err := p.expectNewline("case clause"); err != nil {
			return nil, err
		}

		body, err := p.parseStmtBlock(token.CASE, token.CASE_ELSE, token.END_SELECT)
		if err != nil {
			return nil, err
		}

		cases = append(cases, ast.CaseClause{
			Pos:     caseTok.Pos,
			Values:  values,
			Binding: binding,
			Body:    body,
		})
	}

	hasElse := false
	var elseBody []ast.Statement
	if p.match(token.CASE_ELSE) {
		hasElse = true
		if err := p.expectNewline("case else"); err != nil {
			return nil, err
		}
		elseBody, err = p.parseStmtBlock(token.END_SELECT)
		if err != nil {
			return nil, err
		}
	}

	if _, err := p.expect(token.END_SELECT, "to close select case statement"); err != nil {
		return nil, err
	}
	if err := p.expectNewline("end select"); err != nil {
		return nil, err
	}

	return &ast.SelectStmt{
		Pos:      selTok.Pos,
		Target:   target,
		Cases:    cases,
		ElseBody: elseBody,
		HasElse:  hasElse,
	}, nil
}

// TryCatchStmt ::= "try" NEWLINE Statement* "catch" Identifier NEWLINE Statement* "end try" NEWLINE
func (p *Parser) parseTryCatchStmt() (*ast.TryCatchStmt, error) {
	tryTok, err := p.expect(token.TRY, "at start of try statement")
	if err != nil {
		return nil, err
	}
	if err := p.expectNewline("try"); err != nil {
		return nil, err
	}

	tryBody, err := p.parseStmtBlock(token.CATCH)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(token.CATCH, "in try/catch statement"); err != nil {
		return nil, err
	}
	catchVarTok, err := p.expectIdent("after 'catch'")
	if err != nil {
		return nil, err
	}
	if err := p.expectNewline("catch variable"); err != nil {
		return nil, err
	}

	catchBody, err := p.parseStmtBlock(token.END_TRY)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(token.END_TRY, "to close try/catch statement"); err != nil {
		return nil, err
	}
	if err := p.expectNewline("end try"); err != nil {
		return nil, err
	}

	return &ast.TryCatchStmt{
		Pos:       tryTok.Pos,
		TryBody:   tryBody,
		CatchVar:  catchVarTok.Lexeme,
		CatchBody: catchBody,
	}, nil
}

// DeferStmt ::= "defer" NEWLINE Statement* "end defer" NEWLINE
func (p *Parser) parseDeferStmt() (*ast.DeferStmt, error) {
	deferTok, err := p.expect(token.DEFER, "at start of defer statement")
	if err != nil {
		return nil, err
	}
	if err := p.expectNewline("defer"); err != nil {
		return nil, err
	}

	body, err := p.parseStmtBlock(token.END_DEFER)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(token.END_DEFER, "to close defer statement"); err != nil {
		return nil, err
	}
	if err := p.expectNewline("end defer"); err != nil {
		return nil, err
	}

	return &ast.DeferStmt{
		Pos:  deferTok.Pos,
		Body: body,
	}, nil
}

// ReturnStmt ::= "return" [Expr] NEWLINE
func (p *Parser) parseReturnStmt() (*ast.ReturnStmt, error) {
	retTok, err := p.expect(token.RETURN, "at start of return statement")
	if err != nil {
		return nil, err
	}

	if p.cur().Kind == token.NEWLINE || p.cur().Kind == token.EOF {
		p.skipNewlines()
		return &ast.ReturnStmt{Pos: retTok.Pos, Value: nil}, nil
	}

	val, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if err := p.expectNewline("return statement"); err != nil {
		return nil, err
	}

	return &ast.ReturnStmt{
		Pos:   retTok.Pos,
		Value: val,
	}, nil
}

// --- Expressions (Precedence Ladder) ---

// Expr ::= OrExpr
func (p *Parser) parseExpr() (ast.Expr, error) {
	return p.parseOrExpr()
}

func (p *Parser) parseBinaryLeft(next func() (ast.Expr, error), ops ...token.Kind) (ast.Expr, error) {
	left, err := next()
	if err != nil {
		return nil, err
	}
	for slices.Contains(ops, p.cur().Kind) {
		opTok := p.advance()
		right, err := next()
		if err != nil {
			return nil, err
		}
		left = &ast.BinaryExpr{
			Pos:   opTok.Pos,
			Left:  left,
			Op:    opTok.Kind,
			Right: right,
		}
	}
	return left, nil
}

// OrExpr ::= AndExpr ("or" AndExpr)*
func (p *Parser) parseOrExpr() (ast.Expr, error) {
	return p.parseBinaryLeft(p.parseAndExpr, token.OR)
}

// AndExpr ::= NotExpr ("and" NotExpr)*
func (p *Parser) parseAndExpr() (ast.Expr, error) {
	return p.parseBinaryLeft(p.parseNotExpr, token.AND)
}

// NotExpr ::= "not" NotExpr | CmpExpr
func (p *Parser) parseNotExpr() (ast.Expr, error) {
	if p.cur().Kind == token.NOT {
		opTok := p.advance()
		operand, err := p.parseNotExpr()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{
			Pos:     opTok.Pos,
			Op:      opTok.Kind,
			Operand: operand,
		}, nil
	}
	return p.parseCmpExpr()
}

// CmpExpr ::= AddExpr (("==" | "!=" | "<" | ">" | "<=" | ">=") AddExpr)*
func (p *Parser) parseCmpExpr() (ast.Expr, error) {
	return p.parseBinaryLeft(p.parseAddExpr, token.EQ, token.NEQ, token.LT, token.GT, token.LTE, token.GTE)
}

// AddExpr ::= MulExpr (("+" | "-" | "&") MulExpr)*
func (p *Parser) parseAddExpr() (ast.Expr, error) {
	return p.parseBinaryLeft(p.parseMulExpr, token.PLUS, token.MINUS, token.AMP)
}

// MulExpr ::= UnaryExpr (("*" | "/" | "%") UnaryExpr)*
func (p *Parser) parseMulExpr() (ast.Expr, error) {
	return p.parseBinaryLeft(p.parseUnaryExpr, token.STAR, token.SLASH, token.PERCENT)
}

// UnaryExpr ::= ("+" | "-") UnaryExpr | PostfixExpr
func (p *Parser) parseUnaryExpr() (ast.Expr, error) {
	if p.cur().Kind == token.PLUS || p.cur().Kind == token.MINUS {
		opTok := p.advance()
		operand, err := p.parseUnaryExpr()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{
			Pos:     opTok.Pos,
			Op:      opTok.Kind,
			Operand: operand,
		}, nil
	}
	return p.parsePostfixExpr()
}

// PostfixExpr ::= PrimaryExpr ( "." Identifier | "[" Expr "]" | "(" [CallArgList] ")" )*
func (p *Parser) parsePostfixExpr() (ast.Expr, error) {
	expr, err := p.parsePrimaryExpr()
	if err != nil {
		return nil, err
	}

	for {
		switch p.cur().Kind {
		case token.DOT:
			dotTok := p.advance()
			fieldTok, err := p.expectFieldOrMethodIdent("after '.'")
			if err != nil {
				return nil, err
			}
			expr = &ast.FieldAccessExpr{
				Pos:    dotTok.Pos,
				Target: expr,
				Field:  fieldTok.Lexeme,
			}

		case token.LBRACKET:
			lbrTok := p.advance()
			idxExpr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(token.RBRACKET, "after index expression"); err != nil {
				return nil, err
			}
			expr = &ast.IndexExpr{
				Pos:    lbrTok.Pos,
				Target: expr,
				Index:  idxExpr,
			}

		case token.LPAREN:
			lpTok := p.advance()
			args, err := p.parseCallArgList()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(token.RPAREN, "after call arguments"); err != nil {
				return nil, err
			}
			expr = &ast.CallExpr{
				Pos:    lpTok.Pos,
				Callee: expr,
				Args:   args,
			}

		default:
			return expr, nil
		}
	}
}

// CallArgList ::= CallArg ("," CallArg)* [","]
// Supports positional arguments as well as named arguments (e.g. error("msg", code=408)).
func (p *Parser) parseCallArgList() ([]ast.CallArg, error) {
	var args []ast.CallArg
	p.skipNewlines()
	if p.cur().Kind == token.RPAREN {
		return args, nil
	}

	for {
		p.skipNewlines()
		if p.cur().Kind == token.RPAREN {
			break
		}

		argPos := p.cur().Pos
		argName := ""
		if p.cur().Kind == token.IDENT && p.peek(1).Kind == token.ASSIGN {
			nameTok := p.advance()
			p.advance() // consume '='
			argName = nameTok.Lexeme
		}

		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, ast.CallArg{
			Pos:   argPos,
			Name:  argName,
			Value: val,
		})

		p.skipNewlines()
		if p.match(token.COMMA) {
			p.skipNewlines()
			if p.cur().Kind == token.RPAREN {
				break
			}
			continue
		}
		break
	}

	return args, nil
}

// PrimaryExpr ::= Literal | QualifiedIdent | TypedEmptyCtor | "(" Expr ")"
func (p *Parser) parsePrimaryExpr() (ast.Expr, error) {
	tok := p.cur()

	switch tok.Kind {
	case token.INT_LIT:
		p.advance()
		val, err := strconv.ParseInt(tok.Lexeme, 10, 64)
		if err != nil {
			return nil, &ParseError{
				Pos:     tok.Pos,
				Message: fmt.Sprintf("invalid integer literal %q: %v", tok.Lexeme, err),
			}
		}
		return &ast.IntLitExpr{
			Pos:   tok.Pos,
			Raw:   tok.Lexeme,
			Value: val,
		}, nil

	case token.FLOAT_LIT:
		p.advance()
		val, err := strconv.ParseFloat(tok.Lexeme, 64)
		if err != nil {
			return nil, &ParseError{
				Pos:     tok.Pos,
				Message: fmt.Sprintf("invalid float literal %q: %v", tok.Lexeme, err),
			}
		}
		return &ast.FloatLitExpr{
			Pos:   tok.Pos,
			Raw:   tok.Lexeme,
			Value: val,
		}, nil

	case token.STRING_LIT:
		p.advance()
		return &ast.StringLitExpr{
			Pos:   tok.Pos,
			Value: tok.Lexeme,
		}, nil

	case token.TRUE:
		p.advance()
		return &ast.BoolLitExpr{
			Pos:   tok.Pos,
			Value: true,
		}, nil

	case token.FALSE:
		p.advance()
		return &ast.BoolLitExpr{
			Pos:   tok.Pos,
			Value: false,
		}, nil

	case token.IDENT:
		p.advance()
		return &ast.IdentExpr{
			Pos:  tok.Pos,
			Name: tok.Lexeme,
		}, nil

	// Built-in prelude module names ("str", "list", "dict") or typed empty constructors:
	// e.g. str.join(...), list[int](), dict[str, int]()
	case token.STR:
		p.advance()
		return &ast.IdentExpr{
			Pos:  tok.Pos,
			Name: tok.Lexeme,
		}, nil

	case token.LIST, token.DICT:
		if p.peek(1).Kind == token.LBRACKET {
			return p.parseTypedEmptyCtor()
		}
		p.advance()
		return &ast.IdentExpr{
			Pos:  tok.Pos,
			Name: tok.Lexeme,
		}, nil

	case token.LPAREN:
		p.advance()
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(token.RPAREN, "to close '('"); err != nil {
			return nil, err
		}
		return inner, nil

	case token.LBRACKET:
		return p.parseListLiteral()

	case token.LBRACE:
		return p.parseDictLiteral()

	default:
		return nil, &ParseError{
			Pos:     tok.Pos,
			Message: fmt.Sprintf("unexpected token %s (%q) in expression", tok.Kind, tok.Lexeme),
		}
	}
}

// TypedEmptyCtor ::= ("list" "[" Type "]" | "dict" "[" Type "," Type "]") "(" ")"
func (p *Parser) parseTypedEmptyCtor() (ast.Expr, error) {
	startPos := p.cur().Pos
	ctorType, err := p.parseType()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(token.LPAREN, "after typed container constructor"); err != nil {
		return nil, err
	}
	if _, err := p.expect(token.RPAREN, "to close typed container constructor '()'"); err != nil {
		return nil, err
	}
	return &ast.TypedEmptyCtorExpr{
		Pos:  startPos,
		Type: ctorType,
	}, nil
}

// ListLiteral ::= "[" [Expr ("," Expr)* [","]] "]"
func (p *Parser) parseListLiteral() (ast.Expr, error) {
	startTok, err := p.expect(token.LBRACKET, "at start of list literal")
	if err != nil {
		return nil, err
	}

	var elems []ast.Expr
	p.skipNewlines()
	if p.cur().Kind != token.RBRACKET {
		for {
			p.skipNewlines()
			if p.cur().Kind == token.RBRACKET {
				break
			}
			el, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			elems = append(elems, el)

			p.skipNewlines()
			if p.match(token.COMMA) {
				p.skipNewlines()
				if p.cur().Kind == token.RBRACKET {
					break
				}
				continue
			}
			break
		}
	}

	if _, err := p.expect(token.RBRACKET, "to close list literal"); err != nil {
		return nil, err
	}

	return &ast.ListLitExpr{
		Pos:      startTok.Pos,
		Elements: elems,
	}, nil
}

// DictLiteral ::= "{" [DictEntry ("," DictEntry)* [","]] "}"
// DictEntry   ::= Expr ":" Expr
func (p *Parser) parseDictLiteral() (ast.Expr, error) {
	startTok, err := p.expect(token.LBRACE, "at start of dict literal")
	if err != nil {
		return nil, err
	}

	var entries []ast.DictEntryExpr
	p.skipNewlines()
	if p.cur().Kind != token.RBRACE {
		for {
			p.skipNewlines()
			if p.cur().Kind == token.RBRACE {
				break
			}
			entryPos := p.cur().Pos
			keyExpr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(token.COLON, "after dictionary key"); err != nil {
				return nil, err
			}
			valExpr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			entries = append(entries, ast.DictEntryExpr{
				Pos:   entryPos,
				Key:   keyExpr,
				Value: valExpr,
			})

			p.skipNewlines()
			if p.match(token.COMMA) {
				p.skipNewlines()
				if p.cur().Kind == token.RBRACE {
					break
				}
				continue
			}
			break
		}
	}

	if _, err := p.expect(token.RBRACE, "to close dict literal"); err != nil {
		return nil, err
	}

	return &ast.DictLitExpr{
		Pos:     startTok.Pos,
		Entries: entries,
	}, nil
}
