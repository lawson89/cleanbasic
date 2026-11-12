package sema

import (
	"fmt"
	"os"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/parser"
	"cleanbasic/bootstrap/internal/token"
)

// TypeKind distinguishes primitive and void semantic types.
type TypeKind string

const (
	KindVoid  TypeKind = "void"
	KindInt   TypeKind = "int"
	KindFloat TypeKind = "float"
	KindStr   TypeKind = "str"
	KindBool  TypeKind = "bool"
)

// Type represents a resolved semantic type in CleanBasic.
type Type struct {
	Kind TypeKind
}

var (
	VoidType  = Type{Kind: KindVoid}
	IntType   = Type{Kind: KindInt}
	FloatType = Type{Kind: KindFloat}
	StrType   = Type{Kind: KindStr}
	BoolType  = Type{Kind: KindBool}
)

func (t Type) String() string {
	if t.Kind == "" {
		return "void"
	}
	return string(t.Kind)
}

func (t Type) IsVoid() bool {
	return t.Kind == "" || t.Kind == KindVoid
}

func (t Type) Equals(other Type) bool {
	return t.String() == other.String()
}

// SemaError represents a semantic analysis error with File:Line:Column location.
type SemaError struct {
	Pos token.Position
	Msg string
}

func (e *SemaError) Error() string {
	return fmt.Sprintf("%s: semantic error: %s", e.Pos.String(), e.Msg)
}

func newSemaError(pos token.Position, format string, args ...any) error {
	return &SemaError{
		Pos: pos,
		Msg: fmt.Sprintf(format, args...),
	}
}

// VarSymbol represents a top-level constant, function parameter, local variable, or loop counter.
type VarSymbol struct {
	Name    string
	Type    Type
	IsConst bool
	ByRef   bool
	Pos     token.Position
}

// ParamSymbol represents a typed function parameter.
type ParamSymbol struct {
	Name  string
	Type  Type
	ByRef bool
	Pos   token.Position
}

// FnSymbol represents a declared or built-in function signature.
type FnSymbol struct {
	Name       string
	Params     []ParamSymbol
	ReturnType Type
	CanError   bool
	Public     bool
	Extern     bool
	IsVariadic bool
	Decl       *ast.FnDecl
	Pos        token.Position
}

// Scope represents a lexical variable scope with an optional parent scope.
type Scope struct {
	parent *Scope
	vars   map[string]*VarSymbol
}

// NewScope creates a new lexical scope enclosed by parent.
func NewScope(parent *Scope) *Scope {
	return &Scope{
		parent: parent,
		vars:   make(map[string]*VarSymbol),
	}
}

// Define inserts a variable symbol into the current scope, failing if already defined in this scope.
func (s *Scope) Define(sym *VarSymbol) error {
	if existing, exists := s.vars[sym.Name]; exists {
		return newSemaError(sym.Pos, "symbol %q is already defined in this scope (previous definition at %s)", sym.Name, existing.Pos.String())
	}
	s.vars[sym.Name] = sym
	return nil
}

// Lookup searches the current scope and enclosing scopes for a variable symbol.
func (s *Scope) Lookup(name string) (*VarSymbol, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if sym, ok := cur.vars[name]; ok {
			return sym, true
		}
	}
	return nil, false
}

// TypedProgram holds the AST and semantic type information for an analyzed CleanBasic program.
type TypedProgram struct {
	EntryFile  string
	Main       *ast.Program
	ExprTypes  map[ast.Expr]Type
	VarTypes   map[*ast.VarDecl]Type
	ConstTypes map[*ast.ConstDecl]Type
	Functions  map[string]*FnSymbol
	GlobalVars *Scope
}

// AnalyzedProgram is an alias for TypedProgram for backwards compatibility.
type AnalyzedProgram = TypedProgram

// LoadAndAnalyze reads, parses, and semantically analyzes the CleanBasic file at entryPath.
func LoadAndAnalyze(entryPath string) (*TypedProgram, error) {
	raw, err := os.ReadFile(entryPath)
	if err != nil {
		return nil, err
	}
	return AnalyzeSource(string(raw), entryPath)
}

// AnalyzeSource parses and semantically analyzes CleanBasic source text.
func AnalyzeSource(src string, filename string) (*TypedProgram, error) {
	prog, err := parser.Parse(src, filename)
	if err != nil {
		return nil, err
	}
	return Analyze(prog)
}

// Analyze performs semantic analysis on a parsed *ast.Program.
func Analyze(prog *ast.Program) (*TypedProgram, error) {
	if prog == nil {
		return nil, fmt.Errorf("nil AST program")
	}

	tp := &TypedProgram{
		EntryFile:  prog.File,
		Main:       prog,
		ExprTypes:  make(map[ast.Expr]Type),
		VarTypes:   make(map[*ast.VarDecl]Type),
		ConstTypes: make(map[*ast.ConstDecl]Type),
		Functions:  make(map[string]*FnSymbol),
		GlobalVars: NewScope(nil),
	}

	// Register built-in variadic print function
	tp.Functions["print"] = &FnSymbol{
		Name:       "print",
		ReturnType: VoidType,
		IsVariadic: true,
	}

	a := &analyzer{
		prog: tp,
	}

	if err := a.analyzeProgram(prog); err != nil {
		return nil, err
	}

	return tp, nil
}

type analyzer struct {
	prog      *TypedProgram
	currentFn *FnSymbol
	loopDepth int
}

func (a *analyzer) analyzeProgram(prog *ast.Program) error {
	// Pass 1: Register all top-level function declarations so functions can call each other regardless of order.
	for _, decl := range prog.Decls {
		fnDecl, ok := decl.(*ast.FnDecl)
		if !ok {
			continue
		}
		if err := a.registerFnDecl(fnDecl); err != nil {
			return err
		}
	}

	// Pass 2: Analyze top-level constants in declaration order.
	for _, decl := range prog.Decls {
		constDecl, ok := decl.(*ast.ConstDecl)
		if !ok {
			continue
		}
		if err := a.analyzeTopLevelConst(constDecl); err != nil {
			return err
		}
	}

	// Pass 3: Analyze all function bodies.
	for _, decl := range prog.Decls {
		fnDecl, ok := decl.(*ast.FnDecl)
		if !ok {
			continue
		}
		if err := a.analyzeFnBody(fnDecl); err != nil {
			return err
		}
	}

	return nil
}

func (a *analyzer) registerFnDecl(fn *ast.FnDecl) error {
	if existing, exists := a.prog.Functions[fn.Name]; exists {
		return newSemaError(fn.Pos, "function %q is already declared (previous declaration at %s)", fn.Name, existing.Pos.String())
	}
	if existingVar, exists := a.prog.GlobalVars.Lookup(fn.Name); exists {
		return newSemaError(fn.Pos, "function name %q conflicts with constant declared at %s", fn.Name, existingVar.Pos.String())
	}

	var params []ParamSymbol
	seenParams := make(map[string]token.Position)
	for _, p := range fn.Params {
		if prevPos, dup := seenParams[p.Name]; dup {
			return newSemaError(p.Pos, "duplicate parameter %q in function %q (previous at %s)", p.Name, fn.Name, prevPos.String())
		}
		seenParams[p.Name] = p.Pos
		pType, err := a.resolveTypeExpr(p.Type)
		if err != nil {
			return err
		}
		params = append(params, ParamSymbol{
			Name:  p.Name,
			Type:  pType,
			ByRef: p.ByRef,
			Pos:   p.Pos,
		})
	}

	retType := VoidType
	if fn.ReturnType != nil {
		resolvedRet, err := a.resolveTypeExpr(*fn.ReturnType)
		if err != nil {
			return err
		}
		retType = resolvedRet
	}

	if fn.Name == "main" {
		if len(params) != 0 {
			return newSemaError(fn.Pos, "function %q cannot have parameters", fn.Name)
		}
		if !retType.IsVoid() && !retType.Equals(IntType) {
			return newSemaError(fn.Pos, "function %q must return int (or void), got %s", fn.Name, retType.String())
		}
	}

	a.prog.Functions[fn.Name] = &FnSymbol{
		Name:       fn.Name,
		Params:     params,
		ReturnType: retType,
		CanError:   fn.CanError,
		Public:     fn.Public,
		Extern:     fn.Extern,
		Decl:       fn,
		Pos:        fn.Pos,
	}
	return nil
}

func (a *analyzer) analyzeTopLevelConst(d *ast.ConstDecl) error {
	if existingFn, exists := a.prog.Functions[d.Name]; exists {
		return newSemaError(d.Pos, "constant %q conflicts with function declared at %s", d.Name, existingFn.Pos.String())
	}

	valType, err := a.inferExpr(d.Value, a.prog.GlobalVars)
	if err != nil {
		return err
	}
	if valType.IsVoid() {
		return newSemaError(d.Pos, "cannot initialize constant %q with void expression", d.Name)
	}

	finalType := valType
	if d.Type != nil {
		declaredType, err := a.resolveTypeExpr(*d.Type)
		if err != nil {
			return err
		}
		if !declaredType.Equals(valType) {
			return newSemaError(d.Pos, "type mismatch in constant %q declaration: expected %s, got %s", d.Name, declaredType.String(), valType.String())
		}
		finalType = declaredType
	}

	sym := &VarSymbol{
		Name:    d.Name,
		Type:    finalType,
		IsConst: true,
		Pos:     d.Pos,
	}
	if err := a.prog.GlobalVars.Define(sym); err != nil {
		return err
	}
	a.prog.ConstTypes[d] = finalType
	return nil
}

func (a *analyzer) analyzeFnBody(fn *ast.FnDecl) error {
	if fn.Extern {
		return nil
	}

	fnSym := a.prog.Functions[fn.Name]
	prevFn := a.currentFn
	a.currentFn = fnSym
	defer func() {
		a.currentFn = prevFn
	}()

	fnScope := NewScope(a.prog.GlobalVars)
	for _, p := range fnSym.Params {
		err := fnScope.Define(&VarSymbol{
			Name:    p.Name,
			Type:    p.Type,
			IsConst: false,
			ByRef:   p.ByRef,
			Pos:     p.Pos,
		})
		if err != nil {
			return err
		}
	}

	for _, stmt := range fn.Body {
		if err := a.analyzeStmt(stmt, fnScope); err != nil {
			return err
		}
	}

	if !fnSym.ReturnType.IsVoid() && !blockAlwaysReturns(fn.Body) {
		return newSemaError(fn.Pos, "function %q with return type %s is missing a return statement on all control paths", fn.Name, fnSym.ReturnType.String())
	}

	return nil
}

func blockAlwaysReturns(stmts []ast.Statement) bool {
	for _, st := range stmts {
		switch s := st.(type) {
		case *ast.ReturnStmt:
			return true
		case *ast.IfStmt:
			if !s.HasElse {
				continue
			}
			if !blockAlwaysReturns(s.ThenBody) {
				continue
			}
			allElseIfsReturn := true
			for _, elif := range s.ElseIfs {
				if !blockAlwaysReturns(elif.Body) {
					allElseIfsReturn = false
					break
				}
			}
			if allElseIfsReturn && blockAlwaysReturns(s.ElseBody) {
				return true
			}
		}
	}
	return false
}

func (a *analyzer) analyzeStmt(stmt ast.Statement, scope *Scope) error {
	switch s := stmt.(type) {
	case *ast.VarDecl:
		return a.analyzeVarDecl(s, scope)
	case *ast.Assignment:
		return a.analyzeAssignment(s, scope)
	case *ast.ExprStmt:
		_, err := a.inferExpr(s.Expr, scope)
		return err
	case *ast.IfStmt:
		return a.analyzeIfStmt(s, scope)
	case *ast.WhileStmt:
		return a.analyzeWhileStmt(s, scope)
	case *ast.ForStmt:
		return a.analyzeForStmt(s, scope)
	case *ast.ReturnStmt:
		return a.analyzeReturnStmt(s, scope)
	case *ast.BreakStmt:
		if a.loopDepth <= 0 {
			return newSemaError(s.Pos, "break statement is not inside a loop")
		}
		return nil
	case *ast.ContinueStmt:
		if a.loopDepth <= 0 {
			return newSemaError(s.Pos, "continue statement is not inside a loop")
		}
		return nil
	default:
		return newSemaError(stmt.StmtPos(), "unsupported statement in Phase 2")
	}
}

func (a *analyzer) analyzeVarDecl(s *ast.VarDecl, scope *Scope) error {
	valType, err := a.inferExpr(s.Value, scope)
	if err != nil {
		return err
	}
	if valType.IsVoid() {
		return newSemaError(s.Pos, "cannot initialize variable %q with void expression", s.Name)
	}

	finalType := valType
	if s.Type != nil {
		declaredType, err := a.resolveTypeExpr(*s.Type)
		if err != nil {
			return err
		}
		if !declaredType.Equals(valType) {
			return newSemaError(s.Pos, "type mismatch in declaration of %q: expected %s, got %s", s.Name, declaredType.String(), valType.String())
		}
		finalType = declaredType
	}

	sym := &VarSymbol{
		Name:    s.Name,
		Type:    finalType,
		IsConst: s.IsConst,
		Pos:     s.Pos,
	}
	if err := scope.Define(sym); err != nil {
		return err
	}
	a.prog.VarTypes[s] = finalType
	return nil
}

func (a *analyzer) analyzeAssignment(s *ast.Assignment, scope *Scope) error {
	ident, ok := s.Target.(*ast.IdentExpr)
	if !ok {
		return newSemaError(s.Pos, "invalid assignment target")
	}

	sym, found := scope.Lookup(ident.Name)
	if !found {
		return newSemaError(ident.Pos, "undefined variable %q", ident.Name)
	}
	if sym.IsConst {
		return newSemaError(s.Pos, "cannot reassign constant %q (declared at %s)", ident.Name, sym.Pos.String())
	}
	a.prog.ExprTypes[ident] = sym.Type

	valType, err := a.inferExpr(s.Value, scope)
	if err != nil {
		return err
	}
	if !sym.Type.Equals(valType) {
		return newSemaError(s.Pos, "type mismatch in assignment to %q: expected %s, got %s", ident.Name, sym.Type.String(), valType.String())
	}
	return nil
}

func (a *analyzer) analyzeIfStmt(s *ast.IfStmt, scope *Scope) error {
	condType, err := a.inferExpr(s.Cond, scope)
	if err != nil {
		return err
	}
	if !condType.Equals(BoolType) {
		return newSemaError(s.Cond.ExprPos(), "if condition must be bool, got %s", condType.String())
	}

	thenScope := NewScope(scope)
	for _, st := range s.ThenBody {
		if err := a.analyzeStmt(st, thenScope); err != nil {
			return err
		}
	}

	for _, elif := range s.ElseIfs {
		elifCondType, err := a.inferExpr(elif.Cond, scope)
		if err != nil {
			return err
		}
		if !elifCondType.Equals(BoolType) {
			return newSemaError(elif.Cond.ExprPos(), "else if condition must be bool, got %s", elifCondType.String())
		}
		elifScope := NewScope(scope)
		for _, st := range elif.Body {
			if err := a.analyzeStmt(st, elifScope); err != nil {
				return err
			}
		}
	}

	if s.HasElse {
		elseScope := NewScope(scope)
		for _, st := range s.ElseBody {
			if err := a.analyzeStmt(st, elseScope); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *analyzer) analyzeWhileStmt(s *ast.WhileStmt, scope *Scope) error {
	condType, err := a.inferExpr(s.Cond, scope)
	if err != nil {
		return err
	}
	if !condType.Equals(BoolType) {
		return newSemaError(s.Cond.ExprPos(), "while condition must be bool, got %s", condType.String())
	}

	bodyScope := NewScope(scope)
	a.loopDepth++
	defer func() {
		a.loopDepth--
	}()
	for _, st := range s.Body {
		if err := a.analyzeStmt(st, bodyScope); err != nil {
			return err
		}
	}
	return nil
}

func (a *analyzer) analyzeForStmt(s *ast.ForStmt, scope *Scope) error {
	if !s.IsRange {
		return newSemaError(s.Pos, "for-in collection loops are not supported until Phase 3")
	}

	startType, err := a.inferExpr(s.Start, scope)
	if err != nil {
		return err
	}
	if !startType.Equals(IntType) {
		return newSemaError(s.Start.ExprPos(), "for loop start expression must be int, got %s", startType.String())
	}

	endType, err := a.inferExpr(s.End, scope)
	if err != nil {
		return err
	}
	if !endType.Equals(IntType) {
		return newSemaError(s.End.ExprPos(), "for loop end expression must be int, got %s", endType.String())
	}

	if s.Step != nil {
		stepType, err := a.inferExpr(s.Step, scope)
		if err != nil {
			return err
		}
		if !stepType.Equals(IntType) {
			return newSemaError(s.Step.ExprPos(), "for loop step expression must be int, got %s", stepType.String())
		}
	}

	loopScope := NewScope(scope)
	if err := loopScope.Define(&VarSymbol{
		Name:    s.VarName,
		Type:    IntType,
		IsConst: false,
		Pos:     s.Pos,
	}); err != nil {
		return err
	}

	a.loopDepth++
	defer func() {
		a.loopDepth--
	}()
	for _, st := range s.Body {
		if err := a.analyzeStmt(st, loopScope); err != nil {
			return err
		}
	}
	return nil
}

func (a *analyzer) analyzeReturnStmt(s *ast.ReturnStmt, scope *Scope) error {
	if a.currentFn == nil {
		return newSemaError(s.Pos, "return statement outside of function")
	}

	expected := a.currentFn.ReturnType
	if expected.IsVoid() {
		if s.Value != nil {
			valType, err := a.inferExpr(s.Value, scope)
			if err != nil {
				return err
			}
			return newSemaError(s.Pos, "function %q has void return type, cannot return %s", a.currentFn.Name, valType.String())
		}
		return nil
	}

	if s.Value == nil {
		return newSemaError(s.Pos, "function %q expects return value of type %s, got empty return", a.currentFn.Name, expected.String())
	}

	valType, err := a.inferExpr(s.Value, scope)
	if err != nil {
		return err
	}
	if !expected.Equals(valType) {
		return newSemaError(s.Pos, "return type mismatch in function %q: expected %s, got %s", a.currentFn.Name, expected.String(), valType.String())
	}
	return nil
}

func (a *analyzer) inferExpr(expr ast.Expr, scope *Scope) (Type, error) {
	var resolved Type
	var err error

	switch e := expr.(type) {
	case *ast.IntLitExpr:
		resolved = IntType
	case *ast.FloatLitExpr:
		resolved = FloatType
	case *ast.StringLitExpr:
		resolved = StrType
	case *ast.BoolLitExpr:
		resolved = BoolType
	case *ast.IdentExpr:
		sym, ok := scope.Lookup(e.Name)
		if !ok {
			return VoidType, newSemaError(e.Pos, "undefined identifier %q", e.Name)
		}
		resolved = sym.Type
	case *ast.UnaryExpr:
		resolved, err = a.inferUnaryExpr(e, scope)
	case *ast.BinaryExpr:
		resolved, err = a.inferBinaryExpr(e, scope)
	case *ast.CallExpr:
		resolved, err = a.inferCallExpr(e, scope)
	default:
		return VoidType, newSemaError(expr.ExprPos(), "unsupported expression in Phase 2")
	}

	if err != nil {
		return VoidType, err
	}
	a.prog.ExprTypes[expr] = resolved
	return resolved, nil
}

func (a *analyzer) inferUnaryExpr(e *ast.UnaryExpr, scope *Scope) (Type, error) {
	opType, err := a.inferExpr(e.Operand, scope)
	if err != nil {
		return VoidType, err
	}

	switch e.Op {
	case token.NOT:
		if !opType.Equals(BoolType) {
			return VoidType, newSemaError(e.Pos, "unary operator %q requires bool operand, got %s", e.Op, opType.String())
		}
		return BoolType, nil
	case token.PLUS, token.MINUS:
		if !opType.Equals(IntType) && !opType.Equals(FloatType) {
			return VoidType, newSemaError(e.Pos, "unary operator %q requires numeric (int or float) operand, got %s", e.Op, opType.String())
		}
		return opType, nil
	default:
		return VoidType, newSemaError(e.Pos, "unknown unary operator %q", e.Op)
	}
}

func (a *analyzer) inferBinaryExpr(e *ast.BinaryExpr, scope *Scope) (Type, error) {
	leftType, err := a.inferExpr(e.Left, scope)
	if err != nil {
		return VoidType, err
	}
	rightType, err := a.inferExpr(e.Right, scope)
	if err != nil {
		return VoidType, err
	}

	switch e.Op {
	case token.AMP:
		if leftType.IsVoid() || rightType.IsVoid() {
			return VoidType, newSemaError(e.Pos, "string concatenation operator %q cannot operate on void", e.Op)
		}
		return StrType, nil

	case token.PLUS, token.MINUS, token.STAR, token.SLASH:
		if !leftType.Equals(rightType) {
			return VoidType, newSemaError(e.Pos, "type mismatch for operator %q: %s and %s", e.Op, leftType.String(), rightType.String())
		}
		if !leftType.Equals(IntType) && !leftType.Equals(FloatType) {
			return VoidType, newSemaError(e.Pos, "operator %q requires int or float operands, got %s", e.Op, leftType.String())
		}
		return leftType, nil

	case token.PERCENT:
		if !leftType.Equals(IntType) || !rightType.Equals(IntType) {
			return VoidType, newSemaError(e.Pos, "modulo operator %q requires int operands, got %s and %s", e.Op, leftType.String(), rightType.String())
		}
		return IntType, nil

	case token.EQ, token.NEQ:
		if !leftType.Equals(rightType) || leftType.IsVoid() {
			return VoidType, newSemaError(e.Pos, "cannot compare %s and %s with %q", leftType.String(), rightType.String(), e.Op)
		}
		return BoolType, nil

	case token.LT, token.GT, token.LTE, token.GTE:
		if !leftType.Equals(rightType) {
			return VoidType, newSemaError(e.Pos, "cannot compare %s and %s with %q", leftType.String(), rightType.String(), e.Op)
		}
		if !leftType.Equals(IntType) && !leftType.Equals(FloatType) && !leftType.Equals(StrType) {
			return VoidType, newSemaError(e.Pos, "relational operator %q is not supported for type %s", e.Op, leftType.String())
		}
		return BoolType, nil

	case token.AND, token.OR:
		if !leftType.Equals(BoolType) || !rightType.Equals(BoolType) {
			return VoidType, newSemaError(e.Pos, "logical operator %q requires bool operands, got %s and %s", e.Op, leftType.String(), rightType.String())
		}
		return BoolType, nil

	default:
		return VoidType, newSemaError(e.Pos, "unsupported binary operator %q", e.Op)
	}
}

func (a *analyzer) inferCallExpr(e *ast.CallExpr, scope *Scope) (Type, error) {
	calleeIdent, ok := e.Callee.(*ast.IdentExpr)
	if !ok {
		return VoidType, newSemaError(e.Pos, "unsupported call target")
	}

	fnSym, exists := a.prog.Functions[calleeIdent.Name]
	if !exists {
		return VoidType, newSemaError(calleeIdent.Pos, "undefined function %q", calleeIdent.Name)
	}

	if fnSym.IsVariadic {
		for _, arg := range e.Args {
			argType, err := a.inferExpr(arg.Value, scope)
			if err != nil {
				return VoidType, err
			}
			if argType.IsVoid() {
				return VoidType, newSemaError(arg.Pos, "argument to %q cannot be void", fnSym.Name)
			}
		}
		return fnSym.ReturnType, nil
	}

	if len(e.Args) != len(fnSym.Params) {
		return VoidType, newSemaError(e.Pos, "function %q expects %d argument(s), got %d", fnSym.Name, len(fnSym.Params), len(e.Args))
	}

	for i, arg := range e.Args {
		param := fnSym.Params[i]
		if arg.Name != "" && arg.Name != param.Name {
			return VoidType, newSemaError(arg.Pos, "named argument %q does not match parameter %q of function %q", arg.Name, param.Name, fnSym.Name)
		}
		argType, err := a.inferExpr(arg.Value, scope)
		if err != nil {
			return VoidType, err
		}
		if !param.Type.Equals(argType) {
			return VoidType, newSemaError(arg.Pos, "argument %d (%q) to function %q has type %s, expected %s",
				i+1, param.Name, fnSym.Name, argType.String(), param.Type.String())
		}
		if param.ByRef {
			argIdent, isIdent := arg.Value.(*ast.IdentExpr)
			if !isIdent {
				return VoidType, newSemaError(arg.Pos, "argument %d (%q) to byref parameter of %q must be a mutable variable", i+1, param.Name, fnSym.Name)
			}
			sym, found := scope.Lookup(argIdent.Name)
			if !found || sym.IsConst {
				return VoidType, newSemaError(arg.Pos, "cannot pass constant %q to byref parameter %q of %q", argIdent.Name, param.Name, fnSym.Name)
			}
		}
	}

	return fnSym.ReturnType, nil
}

func (a *analyzer) resolveTypeExpr(t ast.TypeExpr) (Type, error) {
	if t.Kind == ast.TypeBasic {
		switch t.Name {
		case "int":
			return IntType, nil
		case "float":
			return FloatType, nil
		case "str":
			return StrType, nil
		case "bool":
			return BoolType, nil
		}
	}
	return VoidType, newSemaError(t.Pos, "unknown or unsupported type %q in Phase 2", t.String())
}
