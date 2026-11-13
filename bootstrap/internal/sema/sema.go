package sema

import (
	"fmt"
	"os"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/parser"
	"cleanbasic/bootstrap/internal/token"
)

// TypeKind distinguishes primitive, container, user-defined, and void semantic types.
type TypeKind string

const (
	KindVoid   TypeKind = "void"
	KindInt    TypeKind = "int"
	KindFloat  TypeKind = "float"
	KindStr    TypeKind = "str"
	KindBool   TypeKind = "bool"
	KindStruct TypeKind = "struct"
	KindEnum   TypeKind = "enum"
	KindUnion  TypeKind = "union"
	KindList   TypeKind = "list"
	KindDict   TypeKind = "dict"
)

// Type represents a resolved semantic type in CleanBasic.
type Type struct {
	Kind     TypeKind
	Name     string
	ElemType *Type
	KeyType  *Type
	ValType  *Type
}

var (
	VoidType  = Type{Kind: KindVoid}
	IntType   = Type{Kind: KindInt}
	FloatType = Type{Kind: KindFloat}
	StrType   = Type{Kind: KindStr}
	BoolType  = Type{Kind: KindBool}
)

func (t Type) String() string {
	switch t.Kind {
	case "":
		return "void"
	case KindStruct, KindEnum, KindUnion:
		return t.Name
	case KindList:
		if t.ElemType != nil {
			return fmt.Sprintf("list[%s]", t.ElemType.String())
		}
		return "list[?]"
	case KindDict:
		if t.KeyType != nil && t.ValType != nil {
			return fmt.Sprintf("dict[%s, %s]", t.KeyType.String(), t.ValType.String())
		}
		return "dict[?, ?]"
	default:
		return string(t.Kind)
	}
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

// FieldSymbol represents a field in a user-defined struct type.
type FieldSymbol struct {
	Name string
	Type Type
	Pos  token.Position
}

// StructSymbol represents a user-defined struct type.
type StructSymbol struct {
	Name     string
	Fields   []FieldSymbol
	FieldMap map[string]FieldSymbol
	Public   bool
	Decl     *ast.TypeDecl
	Pos      token.Position
}

// EnumSymbol represents a user-defined enum type.
type EnumSymbol struct {
	Name       string
	Variants   []string
	VariantSet map[string]bool
	Public     bool
	Decl       *ast.EnumDecl
	Pos        token.Position
}

// UnionVariantSymbol represents a variant in a user-defined union type.
type UnionVariantSymbol struct {
	Name string
	Type Type
	Pos  token.Position
}

// UnionSymbol represents a user-defined discriminated union type.
type UnionSymbol struct {
	Name       string
	Variants   []UnionVariantSymbol
	VariantMap map[string]UnionVariantSymbol
	Public     bool
	Decl       *ast.UnionDecl
	Pos        token.Position
}

// CallKind identifies how a CallExpr was resolved during semantic analysis.
type CallKind int

const (
	CallFunc CallKind = iota
	CallStructCtor
	CallUnionCtor
	CallMethod
	CallUFCS
)

// CallInfo records the resolved target of a CallExpr for code generation.
type CallInfo struct {
	Kind        CallKind
	Fn          *FnSymbol
	Struct      *StructSymbol
	NamedArgs   bool
	Union       *UnionSymbol
	VariantName string
	MethodName  string
	Receiver    ast.Expr
}

// TypedProgram holds the AST and semantic type information for an analyzed CleanBasic program.
type TypedProgram struct {
	EntryFile   string
	Main        *ast.Program
	ExprTypes   map[ast.Expr]Type
	VarTypes    map[*ast.VarDecl]Type
	ConstTypes  map[*ast.ConstDecl]Type
	Functions   map[string]*FnSymbol
	Structs     map[string]*StructSymbol
	Enums       map[string]*EnumSymbol
	Unions      map[string]*UnionSymbol
	CallInfos   map[*ast.CallExpr]*CallInfo
	SelectUnion map[*ast.SelectStmt]*UnionSymbol
	GlobalVars  *Scope
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
		EntryFile:   prog.File,
		Main:        prog,
		ExprTypes:   make(map[ast.Expr]Type),
		VarTypes:    make(map[*ast.VarDecl]Type),
		ConstTypes:  make(map[*ast.ConstDecl]Type),
		Functions:   make(map[string]*FnSymbol),
		Structs:     make(map[string]*StructSymbol),
		Enums:       make(map[string]*EnumSymbol),
		Unions:      make(map[string]*UnionSymbol),
		CallInfos:   make(map[*ast.CallExpr]*CallInfo),
		SelectUnion: make(map[*ast.SelectStmt]*UnionSymbol),
		GlobalVars:  NewScope(nil),
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
	// Pass 0: Pre-declare all user-defined types (type, enum, union) so they can reference each other.
	for _, decl := range prog.Decls {
		switch d := decl.(type) {
		case *ast.TypeDecl:
			if err := a.predeclareType(d.Name, d.Pos); err != nil {
				return err
			}
			a.prog.Structs[d.Name] = &StructSymbol{
				Name:     d.Name,
				FieldMap: make(map[string]FieldSymbol),
				Public:   d.Public,
				Decl:     d,
				Pos:      d.Pos,
			}
		case *ast.EnumDecl:
			if err := a.predeclareType(d.Name, d.Pos); err != nil {
				return err
			}
			a.prog.Enums[d.Name] = &EnumSymbol{
				Name:       d.Name,
				VariantSet: make(map[string]bool),
				Public:     d.Public,
				Decl:       d,
				Pos:        d.Pos,
			}
		case *ast.UnionDecl:
			if err := a.predeclareType(d.Name, d.Pos); err != nil {
				return err
			}
			a.prog.Unions[d.Name] = &UnionSymbol{
				Name:       d.Name,
				VariantMap: make(map[string]UnionVariantSymbol),
				Public:     d.Public,
				Decl:       d,
				Pos:        d.Pos,
			}
		}
	}

	// Pass 0.5: Resolve struct fields, enum variants, and union variants.
	for _, decl := range prog.Decls {
		switch d := decl.(type) {
		case *ast.TypeDecl:
			if err := a.resolveTypeDecl(d); err != nil {
				return err
			}
		case *ast.EnumDecl:
			if err := a.resolveEnumDecl(d); err != nil {
				return err
			}
		case *ast.UnionDecl:
			if err := a.resolveUnionDecl(d); err != nil {
				return err
			}
		}
	}

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

func (a *analyzer) predeclareType(name string, pos token.Position) error {
	if _, exists := a.prog.Structs[name]; exists {
		return newSemaError(pos, "type %q is already declared", name)
	}
	if _, exists := a.prog.Enums[name]; exists {
		return newSemaError(pos, "type %q is already declared", name)
	}
	if _, exists := a.prog.Unions[name]; exists {
		return newSemaError(pos, "type %q is already declared", name)
	}
	return nil
}

func (a *analyzer) resolveTypeDecl(d *ast.TypeDecl) error {
	sym := a.prog.Structs[d.Name]
	for _, f := range d.Fields {
		if _, dup := sym.FieldMap[f.Name]; dup {
			return newSemaError(f.Pos, "duplicate field %q in type %q", f.Name, d.Name)
		}
		fType, err := a.resolveTypeExpr(f.Type)
		if err != nil {
			return err
		}
		fs := FieldSymbol{Name: f.Name, Type: fType, Pos: f.Pos}
		sym.Fields = append(sym.Fields, fs)
		sym.FieldMap[f.Name] = fs
	}
	return nil
}

func (a *analyzer) resolveEnumDecl(d *ast.EnumDecl) error {
	sym := a.prog.Enums[d.Name]
	for _, v := range d.Variants {
		if sym.VariantSet[v] {
			return newSemaError(d.Pos, "duplicate variant %q in enum %q", v, d.Name)
		}
		sym.Variants = append(sym.Variants, v)
		sym.VariantSet[v] = true
	}
	return nil
}

func (a *analyzer) resolveUnionDecl(d *ast.UnionDecl) error {
	sym := a.prog.Unions[d.Name]
	for _, v := range d.Variants {
		if _, dup := sym.VariantMap[v.Name]; dup {
			return newSemaError(v.Pos, "duplicate variant %q in union %q", v.Name, d.Name)
		}
		vType, err := a.resolveTypeExpr(v.Type)
		if err != nil {
			return err
		}
		uv := UnionVariantSymbol{Name: v.Name, Type: vType, Pos: v.Pos}
		sym.Variants = append(sym.Variants, uv)
		sym.VariantMap[v.Name] = uv
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
		case *ast.SelectStmt:
			if !s.HasElse {
				continue
			}
			allCasesReturn := true
			for _, cc := range s.Cases {
				if !blockAlwaysReturns(cc.Body) {
					allCasesReturn = false
					break
				}
			}
			if allCasesReturn && blockAlwaysReturns(s.ElseBody) {
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
	case *ast.SelectStmt:
		return a.analyzeSelectStmt(s, scope)
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
	var finalType Type
	if s.Type != nil {
		declaredType, err := a.resolveTypeExpr(*s.Type)
		if err != nil {
			return err
		}
		if listLit, ok := s.Value.(*ast.ListLitExpr); ok && len(listLit.Elements) == 0 && declaredType.Kind == KindList {
			a.prog.ExprTypes[listLit] = declaredType
			finalType = declaredType
		} else if dictLit, ok := s.Value.(*ast.DictLitExpr); ok && len(dictLit.Entries) == 0 && declaredType.Kind == KindDict {
			a.prog.ExprTypes[dictLit] = declaredType
			finalType = declaredType
		} else {
			valType, err := a.inferExpr(s.Value, scope)
			if err != nil {
				return err
			}
			if valType.IsVoid() {
				return newSemaError(s.Pos, "cannot initialize variable %q with void expression", s.Name)
			}
			if !declaredType.Equals(valType) {
				return newSemaError(s.Pos, "type mismatch in declaration of %q: expected %s, got %s", s.Name, declaredType.String(), valType.String())
			}
			finalType = declaredType
		}
	} else {
		valType, err := a.inferExpr(s.Value, scope)
		if err != nil {
			return err
		}
		if valType.IsVoid() {
			return newSemaError(s.Pos, "cannot initialize variable %q with void expression", s.Name)
		}
		finalType = valType
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

func (a *analyzer) checkMutableTarget(expr ast.Expr, scope *Scope) error {
	switch t := expr.(type) {
	case *ast.IdentExpr:
		sym, found := scope.Lookup(t.Name)
		if !found {
			return newSemaError(t.Pos, "undefined variable %q", t.Name)
		}
		if sym.IsConst {
			return newSemaError(t.Pos, "cannot mutate constant %q (declared at %s)", t.Name, sym.Pos.String())
		}
		return nil
	case *ast.FieldAccessExpr:
		return a.checkMutableTarget(t.Target, scope)
	case *ast.IndexExpr:
		return a.checkMutableTarget(t.Target, scope)
	default:
		return newSemaError(expr.ExprPos(), "target expression is not mutable")
	}
}

func (a *analyzer) analyzeAssignment(s *ast.Assignment, scope *Scope) error {
	valType, err := a.inferExpr(s.Value, scope)
	if err != nil {
		return err
	}

	switch target := s.Target.(type) {
	case *ast.IdentExpr:
		sym, found := scope.Lookup(target.Name)
		if !found {
			return newSemaError(target.Pos, "undefined variable %q", target.Name)
		}
		if sym.IsConst {
			return newSemaError(s.Pos, "cannot reassign constant %q (declared at %s)", target.Name, sym.Pos.String())
		}
		a.prog.ExprTypes[target] = sym.Type
		if !sym.Type.Equals(valType) {
			return newSemaError(s.Pos, "type mismatch in assignment to %q: expected %s, got %s", target.Name, sym.Type.String(), valType.String())
		}
		return nil

	case *ast.FieldAccessExpr:
		if err := a.checkMutableTarget(target.Target, scope); err != nil {
			return err
		}
		fieldType, err := a.inferExpr(target, scope)
		if err != nil {
			return err
		}
		if !fieldType.Equals(valType) {
			return newSemaError(s.Pos, "type mismatch in assignment to field %q: expected %s, got %s", target.Field, fieldType.String(), valType.String())
		}
		return nil

	case *ast.IndexExpr:
		if err := a.checkMutableTarget(target.Target, scope); err != nil {
			return err
		}
		elemType, err := a.inferExpr(target, scope)
		if err != nil {
			return err
		}
		if !elemType.Equals(valType) {
			return newSemaError(s.Pos, "type mismatch in index assignment: expected %s, got %s", elemType.String(), valType.String())
		}
		return nil

	default:
		return newSemaError(s.Pos, "invalid assignment target")
	}
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
		iterType, err := a.inferExpr(s.Iterable, scope)
		if err != nil {
			return err
		}
		loopScope := NewScope(scope)
		switch iterType.Kind {
		case KindList:
			if len(s.Vars) != 1 {
				return newSemaError(s.Pos, "for-in loop over list only supports a single loop variable")
			}
			if err := loopScope.Define(&VarSymbol{
				Name: s.Vars[0],
				Type: *iterType.ElemType,
				Pos:  s.Pos,
			}); err != nil {
				return err
			}
		case KindDict:
			if len(s.Vars) < 1 || len(s.Vars) > 2 {
				return newSemaError(s.Pos, "for-in loop over dict supports 1 or 2 loop variables")
			}
			if err := loopScope.Define(&VarSymbol{
				Name: s.Vars[0],
				Type: *iterType.KeyType,
				Pos:  s.Pos,
			}); err != nil {
				return err
			}
			if len(s.Vars) == 2 {
				if err := loopScope.Define(&VarSymbol{
					Name: s.Vars[1],
					Type: *iterType.ValType,
					Pos:  s.Pos,
				}); err != nil {
					return err
				}
			}
		default:
			return newSemaError(s.Iterable.ExprPos(), "for-in loop requires list or dict, got %s", iterType.String())
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

func (a *analyzer) analyzeSelectStmt(s *ast.SelectStmt, scope *Scope) error {
	exprType, err := a.inferExpr(s.Target, scope)
	if err != nil {
		return err
	}

	if exprType.Kind == KindUnion {
		unionSym := a.prog.Unions[exprType.Name]
		a.prog.SelectUnion[s] = unionSym
		for _, cc := range s.Cases {
			if cc.Binding != "" && len(cc.Values) != 1 {
				return newSemaError(cc.Pos, "cannot use 'as' binding with multiple patterns in select case")
			}
			var boundType Type
			for _, pat := range cc.Values {
				var variantName string
				switch p := pat.(type) {
				case *ast.IdentExpr:
					variantName = p.Name
				case *ast.FieldAccessExpr:
					objIdent, ok := p.Target.(*ast.IdentExpr)
					if !ok || objIdent.Name != unionSym.Name {
						return newSemaError(pat.ExprPos(), "invalid union case pattern in select case for %s", unionSym.Name)
					}
					variantName = p.Field
				default:
					return newSemaError(pat.ExprPos(), "invalid union case pattern in select case for %s", unionSym.Name)
				}
				uv, exists := unionSym.VariantMap[variantName]
				if !exists {
					return newSemaError(pat.ExprPos(), "union %q has no variant %q", unionSym.Name, variantName)
				}
				boundType = uv.Type
			}
			caseScope := NewScope(scope)
			if cc.Binding != "" {
				if err := caseScope.Define(&VarSymbol{
					Name: cc.Binding,
					Type: boundType,
					Pos:  cc.Pos,
				}); err != nil {
					return err
				}
			}
			for _, st := range cc.Body {
				if err := a.analyzeStmt(st, caseScope); err != nil {
					return err
				}
			}
		}
	} else {
		for _, cc := range s.Cases {
			if cc.Binding != "" {
				return newSemaError(cc.Pos, "'as' binding in select case is only supported for union types")
			}
			for _, pat := range cc.Values {
				patType, err := a.inferExpr(pat, scope)
				if err != nil {
					return err
				}
				if !patType.Equals(exprType) {
					return newSemaError(pat.ExprPos(), "case pattern type %s does not match select expression type %s", patType.String(), exprType.String())
				}
			}
			caseScope := NewScope(scope)
			for _, st := range cc.Body {
				if err := a.analyzeStmt(st, caseScope); err != nil {
					return err
				}
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
	case *ast.ListLitExpr:
		resolved, err = a.inferListLitExpr(e, scope)
	case *ast.DictLitExpr:
		resolved, err = a.inferDictLitExpr(e, scope)
	case *ast.TypedEmptyCtorExpr:
		resolved, err = a.inferTypedEmptyCtorExpr(e)
	case *ast.FieldAccessExpr:
		resolved, err = a.inferFieldAccessExpr(e, scope)
	case *ast.IndexExpr:
		resolved, err = a.inferIndexExpr(e, scope)
	case *ast.CallExpr:
		resolved, err = a.inferCallExpr(e, scope)
	default:
		return VoidType, newSemaError(expr.ExprPos(), "unsupported expression")
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

func (a *analyzer) inferListLitExpr(e *ast.ListLitExpr, scope *Scope) (Type, error) {
	if len(e.Elements) == 0 {
		return VoidType, newSemaError(e.Pos, "cannot infer type of empty list literal []; use list[T]() or an explicit type annotation")
	}
	firstType, err := a.inferExpr(e.Elements[0], scope)
	if err != nil {
		return VoidType, err
	}
	for i := 1; i < len(e.Elements); i++ {
		elemType, err := a.inferExpr(e.Elements[i], scope)
		if err != nil {
			return VoidType, err
		}
		if !firstType.Equals(elemType) {
			return VoidType, newSemaError(e.Elements[i].ExprPos(), "list literal element %d has type %s, expected %s", i, elemType.String(), firstType.String())
		}
	}
	return Type{Kind: KindList, ElemType: &firstType}, nil
}

func (a *analyzer) inferDictLitExpr(e *ast.DictLitExpr, scope *Scope) (Type, error) {
	if len(e.Entries) == 0 {
		return VoidType, newSemaError(e.Pos, "cannot infer type of empty dict literal {}; use dict[K, V]() or an explicit type annotation")
	}
	firstKey, err := a.inferExpr(e.Entries[0].Key, scope)
	if err != nil {
		return VoidType, err
	}
	firstVal, err := a.inferExpr(e.Entries[0].Value, scope)
	if err != nil {
		return VoidType, err
	}
	for i := 1; i < len(e.Entries); i++ {
		kt, err := a.inferExpr(e.Entries[i].Key, scope)
		if err != nil {
			return VoidType, err
		}
		if !firstKey.Equals(kt) {
			return VoidType, newSemaError(e.Entries[i].Key.ExprPos(), "dict literal key %d has type %s, expected %s", i, kt.String(), firstKey.String())
		}
		vt, err := a.inferExpr(e.Entries[i].Value, scope)
		if err != nil {
			return VoidType, err
		}
		if !firstVal.Equals(vt) {
			return VoidType, newSemaError(e.Entries[i].Value.ExprPos(), "dict literal value %d has type %s, expected %s", i, vt.String(), firstVal.String())
		}
	}
	return Type{Kind: KindDict, KeyType: &firstKey, ValType: &firstVal}, nil
}

func (a *analyzer) inferTypedEmptyCtorExpr(e *ast.TypedEmptyCtorExpr) (Type, error) {
	t, err := a.resolveTypeExpr(e.Type)
	if err != nil {
		return VoidType, err
	}
	if t.Kind != KindList && t.Kind != KindDict {
		return VoidType, newSemaError(e.Pos, "typed empty constructor is only supported for list and dict, got %s", t.String())
	}
	return t, nil
}

func (a *analyzer) inferFieldAccessExpr(e *ast.FieldAccessExpr, scope *Scope) (Type, error) {
	if objIdent, ok := e.Target.(*ast.IdentExpr); ok {
		if enumSym, isEnum := a.prog.Enums[objIdent.Name]; isEnum {
			if !enumSym.VariantSet[e.Field] {
				return VoidType, newSemaError(e.Pos, "enum %q has no variant %q", enumSym.Name, e.Field)
			}
			return Type{Kind: KindEnum, Name: enumSym.Name}, nil
		}
	}
	objType, err := a.inferExpr(e.Target, scope)
	if err != nil {
		return VoidType, err
	}
	if objType.Kind != KindStruct {
		return VoidType, newSemaError(e.Pos, "cannot access field %q on non-struct type %s", e.Field, objType.String())
	}
	structSym := a.prog.Structs[objType.Name]
	fs, exists := structSym.FieldMap[e.Field]
	if !exists {
		return VoidType, newSemaError(e.Pos, "struct %q has no field %q", structSym.Name, e.Field)
	}
	return fs.Type, nil
}

func (a *analyzer) inferIndexExpr(e *ast.IndexExpr, scope *Scope) (Type, error) {
	objType, err := a.inferExpr(e.Target, scope)
	if err != nil {
		return VoidType, err
	}
	idxType, err := a.inferExpr(e.Index, scope)
	if err != nil {
		return VoidType, err
	}
	switch objType.Kind {
	case KindList:
		if !idxType.Equals(IntType) {
			return VoidType, newSemaError(e.Index.ExprPos(), "list index must be int, got %s", idxType.String())
		}
		return *objType.ElemType, nil
	case KindDict:
		if !idxType.Equals(*objType.KeyType) {
			return VoidType, newSemaError(e.Index.ExprPos(), "dict key must be %s, got %s", objType.KeyType.String(), idxType.String())
		}
		return *objType.ValType, nil
	default:
		return VoidType, newSemaError(e.Pos, "cannot index into type %s", objType.String())
	}
}

func (a *analyzer) inferCallExpr(e *ast.CallExpr, scope *Scope) (Type, error) {
	if fa, ok := e.Callee.(*ast.FieldAccessExpr); ok {
		return a.inferMethodOrUFCSCall(e, fa, scope)
	}
	calleeIdent, ok := e.Callee.(*ast.IdentExpr)
	if !ok {
		return VoidType, newSemaError(e.Pos, "unsupported call target")
	}

	if structSym, isStruct := a.prog.Structs[calleeIdent.Name]; isStruct {
		return a.inferStructCtorCall(e, structSym, scope)
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

	a.prog.CallInfos[e] = &CallInfo{
		Kind: CallFunc,
		Fn:   fnSym,
	}
	return fnSym.ReturnType, nil
}

func (a *analyzer) inferStructCtorCall(e *ast.CallExpr, s *StructSymbol, scope *Scope) (Type, error) {
	retType := Type{Kind: KindStruct, Name: s.Name}
	if len(e.Args) == 0 {
		a.prog.CallInfos[e] = &CallInfo{
			Kind:   CallStructCtor,
			Struct: s,
		}
		return retType, nil
	}

	hasNamed := e.Args[0].Name != ""
	if hasNamed {
		seen := make(map[string]bool)
		for _, arg := range e.Args {
			if arg.Name == "" {
				return VoidType, newSemaError(arg.Pos, "cannot mix positional and named arguments in constructor for %q", s.Name)
			}
			if seen[arg.Name] {
				return VoidType, newSemaError(arg.Pos, "duplicate named argument %q in constructor for %q", arg.Name, s.Name)
			}
			seen[arg.Name] = true
			fs, exists := s.FieldMap[arg.Name]
			if !exists {
				return VoidType, newSemaError(arg.Pos, "struct %q has no field %q", s.Name, arg.Name)
			}
			argType, err := a.inferExpr(arg.Value, scope)
			if err != nil {
				return VoidType, err
			}
			if !fs.Type.Equals(argType) {
				return VoidType, newSemaError(arg.Pos, "field %q of %q expects %s, got %s", arg.Name, s.Name, fs.Type.String(), argType.String())
			}
		}
		a.prog.CallInfos[e] = &CallInfo{
			Kind:      CallStructCtor,
			Struct:    s,
			NamedArgs: true,
		}
		return retType, nil
	}

	if len(e.Args) != len(s.Fields) {
		return VoidType, newSemaError(e.Pos, "struct %q constructor expects %d argument(s), got %d", s.Name, len(s.Fields), len(e.Args))
	}
	for i, arg := range e.Args {
		if arg.Name != "" {
			return VoidType, newSemaError(arg.Pos, "cannot mix positional and named arguments in constructor for %q", s.Name)
		}
		argType, err := a.inferExpr(arg.Value, scope)
		if err != nil {
			return VoidType, err
		}
		if !s.Fields[i].Type.Equals(argType) {
			return VoidType, newSemaError(arg.Pos, "argument %d (%q) to %q constructor expects %s, got %s", i+1, s.Fields[i].Name, s.Name, s.Fields[i].Type.String(), argType.String())
		}
	}
	a.prog.CallInfos[e] = &CallInfo{
		Kind:   CallStructCtor,
		Struct: s,
	}
	return retType, nil
}

func (a *analyzer) inferMethodOrUFCSCall(e *ast.CallExpr, fa *ast.FieldAccessExpr, scope *Scope) (Type, error) {
	if objIdent, ok := fa.Target.(*ast.IdentExpr); ok {
		if unionSym, isUnion := a.prog.Unions[objIdent.Name]; isUnion {
			uv, exists := unionSym.VariantMap[fa.Field]
			if !exists {
				return VoidType, newSemaError(e.Pos, "union %q has no variant %q", unionSym.Name, fa.Field)
			}
			if len(e.Args) != 1 || e.Args[0].Name != "" {
				return VoidType, newSemaError(e.Pos, "union constructor %s.%s expects 1 positional argument", unionSym.Name, fa.Field)
			}
			argType, err := a.inferExpr(e.Args[0].Value, scope)
			if err != nil {
				return VoidType, err
			}
			if !uv.Type.Equals(argType) {
				return VoidType, newSemaError(e.Args[0].Pos, "union constructor %s.%s expects %s, got %s", unionSym.Name, fa.Field, uv.Type.String(), argType.String())
			}
			a.prog.CallInfos[e] = &CallInfo{
				Kind:        CallUnionCtor,
				Union:       unionSym,
				VariantName: fa.Field,
			}
			return Type{Kind: KindUnion, Name: unionSym.Name}, nil
		}

		if objIdent.Name == "str" && fa.Field == "join" {
			if len(e.Args) != 2 {
				return VoidType, newSemaError(e.Pos, "str.join expects 2 arguments (list[str], str), got %d", len(e.Args))
			}
			t0, err := a.inferExpr(e.Args[0].Value, scope)
			if err != nil {
				return VoidType, err
			}
			listStr := Type{Kind: KindList, ElemType: &StrType}
			if !t0.Equals(listStr) {
				return VoidType, newSemaError(e.Args[0].Pos, "str.join first argument must be list[str], got %s", t0.String())
			}
			t1, err := a.inferExpr(e.Args[1].Value, scope)
			if err != nil {
				return VoidType, err
			}
			if !t1.Equals(StrType) {
				return VoidType, newSemaError(e.Args[1].Pos, "str.join second argument must be str, got %s", t1.String())
			}
			a.prog.CallInfos[e] = &CallInfo{
				Kind:       CallMethod,
				MethodName: "str.join",
			}
			return StrType, nil
		}
	}

	recvType, err := a.inferExpr(fa.Target, scope)
	if err != nil {
		return VoidType, err
	}

	switch recvType.Kind {
	case KindList:
		switch fa.Field {
		case "len":
			if len(e.Args) != 0 {
				return VoidType, newSemaError(e.Pos, "list.len() expects 0 arguments, got %d", len(e.Args))
			}
			a.prog.CallInfos[e] = &CallInfo{Kind: CallMethod, MethodName: "list.len", Receiver: fa.Target}
			return IntType, nil
		case "push":
			if len(e.Args) != 1 {
				return VoidType, newSemaError(e.Pos, "list.push() expects 1 argument, got %d", len(e.Args))
			}
			if err := a.checkMutableTarget(fa.Target, scope); err != nil {
				return VoidType, err
			}
			argType, err := a.inferExpr(e.Args[0].Value, scope)
			if err != nil {
				return VoidType, err
			}
			if !recvType.ElemType.Equals(argType) {
				return VoidType, newSemaError(e.Args[0].Pos, "list.push() expects %s, got %s", recvType.ElemType.String(), argType.String())
			}
			a.prog.CallInfos[e] = &CallInfo{Kind: CallMethod, MethodName: "list.push", Receiver: fa.Target}
			return VoidType, nil
		}

	case KindDict:
		switch fa.Field {
		case "len":
			if len(e.Args) != 0 {
				return VoidType, newSemaError(e.Pos, "dict.len() expects 0 arguments, got %d", len(e.Args))
			}
			a.prog.CallInfos[e] = &CallInfo{Kind: CallMethod, MethodName: "dict.len", Receiver: fa.Target}
			return IntType, nil
		case "get":
			if len(e.Args) != 2 {
				return VoidType, newSemaError(e.Pos, "dict.get() expects 2 arguments (key, default), got %d", len(e.Args))
			}
			kt, err := a.inferExpr(e.Args[0].Value, scope)
			if err != nil {
				return VoidType, err
			}
			if !recvType.KeyType.Equals(kt) {
				return VoidType, newSemaError(e.Args[0].Pos, "dict.get() key must be %s, got %s", recvType.KeyType.String(), kt.String())
			}
			dt, err := a.inferExpr(e.Args[1].Value, scope)
			if err != nil {
				return VoidType, err
			}
			if !recvType.ValType.Equals(dt) {
				return VoidType, newSemaError(e.Args[1].Pos, "dict.get() default value must be %s, got %s", recvType.ValType.String(), dt.String())
			}
			a.prog.CallInfos[e] = &CallInfo{Kind: CallMethod, MethodName: "dict.get", Receiver: fa.Target}
			return *recvType.ValType, nil
		case "contains":
			if len(e.Args) != 1 {
				return VoidType, newSemaError(e.Pos, "dict.contains() expects 1 argument, got %d", len(e.Args))
			}
			kt, err := a.inferExpr(e.Args[0].Value, scope)
			if err != nil {
				return VoidType, err
			}
			if !recvType.KeyType.Equals(kt) {
				return VoidType, newSemaError(e.Args[0].Pos, "dict.contains() key must be %s, got %s", recvType.KeyType.String(), kt.String())
			}
			a.prog.CallInfos[e] = &CallInfo{Kind: CallMethod, MethodName: "dict.contains", Receiver: fa.Target}
			return BoolType, nil
		}

	case KindStr:
		switch fa.Field {
		case "len":
			if len(e.Args) != 0 {
				return VoidType, newSemaError(e.Pos, "str.len() expects 0 arguments, got %d", len(e.Args))
			}
			a.prog.CallInfos[e] = &CallInfo{Kind: CallMethod, MethodName: "str.len", Receiver: fa.Target}
			return IntType, nil
		case "trim", "toUpper", "toLower", "toStr":
			if len(e.Args) != 0 {
				return VoidType, newSemaError(e.Pos, "str.%s() expects 0 arguments, got %d", fa.Field, len(e.Args))
			}
			a.prog.CallInfos[e] = &CallInfo{Kind: CallMethod, MethodName: "str." + fa.Field, Receiver: fa.Target}
			return StrType, nil
		case "toInt":
			if len(e.Args) != 0 {
				return VoidType, newSemaError(e.Pos, "str.toInt() expects 0 arguments, got %d", len(e.Args))
			}
			a.prog.CallInfos[e] = &CallInfo{Kind: CallMethod, MethodName: "str.toInt", Receiver: fa.Target}
			return IntType, nil
		case "split":
			if len(e.Args) != 1 {
				return VoidType, newSemaError(e.Pos, "str.split() expects 1 argument, got %d", len(e.Args))
			}
			st, err := a.inferExpr(e.Args[0].Value, scope)
			if err != nil {
				return VoidType, err
			}
			if !st.Equals(StrType) {
				return VoidType, newSemaError(e.Args[0].Pos, "str.split() separator must be str, got %s", st.String())
			}
			a.prog.CallInfos[e] = &CallInfo{Kind: CallMethod, MethodName: "str.split", Receiver: fa.Target}
			return Type{Kind: KindList, ElemType: &StrType}, nil
		case "contains":
			if len(e.Args) != 1 {
				return VoidType, newSemaError(e.Pos, "str.contains() expects 1 argument, got %d", len(e.Args))
			}
			st, err := a.inferExpr(e.Args[0].Value, scope)
			if err != nil {
				return VoidType, err
			}
			if !st.Equals(StrType) {
				return VoidType, newSemaError(e.Args[0].Pos, "str.contains() substring must be str, got %s", st.String())
			}
			a.prog.CallInfos[e] = &CallInfo{Kind: CallMethod, MethodName: "str.contains", Receiver: fa.Target}
			return BoolType, nil
		}
	}

	// Same-module UFCS fallback
	fnSym, exists := a.prog.Functions[fa.Field]
	if !exists || len(fnSym.Params) == 0 || !fnSym.Params[0].Type.Equals(recvType) {
		return VoidType, newSemaError(e.Pos, "no method or UFCS function %q for receiver type %s", fa.Field, recvType.String())
	}

	if fnSym.Params[0].ByRef {
		if err := a.checkMutableTarget(fa.Target, scope); err != nil {
			return VoidType, err
		}
		if _, isIdent := fa.Target.(*ast.IdentExpr); !isIdent {
			return VoidType, newSemaError(fa.Target.ExprPos(), "receiver for byref UFCS function %q must be a mutable variable", fnSym.Name)
		}
	}

	expectedArgs := len(fnSym.Params) - 1
	if len(e.Args) != expectedArgs {
		return VoidType, newSemaError(e.Pos, "UFCS call %q expects %d argument(s), got %d", fnSym.Name, expectedArgs, len(e.Args))
	}

	for i, arg := range e.Args {
		param := fnSym.Params[i+1]
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

	a.prog.CallInfos[e] = &CallInfo{
		Kind:     CallUFCS,
		Fn:       fnSym,
		Receiver: fa.Target,
	}
	return fnSym.ReturnType, nil
}

func (a *analyzer) resolveTypeExpr(t ast.TypeExpr) (Type, error) {
	switch t.Kind {
	case ast.TypeBasic:
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
	case ast.TypeList:
		if t.ElemType == nil {
			return VoidType, newSemaError(t.Pos, "list type requires an element type")
		}
		elem, err := a.resolveTypeExpr(*t.ElemType)
		if err != nil {
			return VoidType, err
		}
		return Type{Kind: KindList, ElemType: &elem}, nil
	case ast.TypeDict:
		if t.KeyType == nil || t.ValType == nil {
			return VoidType, newSemaError(t.Pos, "dict type requires key and value types")
		}
		kt, err := a.resolveTypeExpr(*t.KeyType)
		if err != nil {
			return VoidType, err
		}
		vt, err := a.resolveTypeExpr(*t.ValType)
		if err != nil {
			return VoidType, err
		}
		return Type{Kind: KindDict, KeyType: &kt, ValType: &vt}, nil
	case ast.TypeQualified:
		if len(t.Parts) == 1 {
			name := t.Parts[0]
			if _, ok := a.prog.Structs[name]; ok {
				return Type{Kind: KindStruct, Name: name}, nil
			}
			if _, ok := a.prog.Enums[name]; ok {
				return Type{Kind: KindEnum, Name: name}, nil
			}
			if _, ok := a.prog.Unions[name]; ok {
				return Type{Kind: KindUnion, Name: name}, nil
			}
		}
	}
	return VoidType, newSemaError(t.Pos, "unknown or unsupported type %q", t.String())
}
