package sema

import (
	"fmt"
	"os"
	"path/filepath"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/parser"
	"cleanbasic/bootstrap/internal/token"
)

// TypeKind distinguishes primitive, container, user-defined, and void semantic types.
type TypeKind string

const (
	KindVoid    TypeKind = "void"
	KindInt     TypeKind = "int"
	KindFloat   TypeKind = "float"
	KindStr     TypeKind = "str"
	KindBool    TypeKind = "bool"
	KindByte    TypeKind = "byte"
	KindAny     TypeKind = "any"
	KindStruct  TypeKind = "struct"
	KindEnum    TypeKind = "enum"
	KindList    TypeKind = "list"
	KindDict    TypeKind = "dict"
	KindMailbox TypeKind = "mailbox"
	KindJob     TypeKind = "job"
	KindErr     TypeKind = "err"
)

// Type represents a resolved semantic type in CleanBasic.
type Type struct {
	Kind     TypeKind
	Name     string
	ModuleID string
	GoName   string
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
	ByteType  = Type{Kind: KindByte}
	AnyType   = Type{Kind: KindAny}
	ErrType   = Type{Kind: KindErr}
)

func (t Type) String() string {
	switch t.Kind {
	case "":
		return "void"
	case KindStruct, KindEnum:
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
	case KindMailbox:
		if t.ElemType != nil {
			return fmt.Sprintf("mailbox[%s]", t.ElemType.String())
		}
		return "mailbox[?]"
	case KindJob:
		if t.ElemType != nil {
			return fmt.Sprintf("job[%s]", t.ElemType.String())
		}
		return "job"
	default:
		return string(t.Kind)
	}
}

func (t Type) IsVoid() bool {
	return t.Kind == "" || t.Kind == KindVoid
}

func (t Type) Equals(other Type) bool {
	if t.Kind != other.Kind {
		return false
	}
	switch t.Kind {
	case KindStruct, KindEnum:
		return t.Name == other.Name && t.ModuleID == other.ModuleID
	case KindList, KindMailbox, KindJob:
		if t.ElemType == nil || other.ElemType == nil {
			return t.ElemType == other.ElemType
		}
		return t.ElemType.Equals(*other.ElemType)
	case KindDict:
		if t.KeyType == nil || other.KeyType == nil || t.ValType == nil || other.ValType == nil {
			return false
		}
		return t.KeyType.Equals(*other.KeyType) && t.ValType.Equals(*other.ValType)
	default:
		return true
	}
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
	GoName  string
	Type    Type
	IsConst bool
	Public  bool
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
	GoName     string
	Params     []ParamSymbol
	ReturnType Type
	CanError   bool
	Public     bool
	GoTarget   string
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
	Name  string
	Type  Type
	Pos   token.Position
	Boxed bool
}

// StructSymbol represents a user-defined struct type.
type StructSymbol struct {
	Name     string
	GoName   string
	ModuleID string
	Fields   []FieldSymbol
	FieldMap map[string]FieldSymbol
	Public   bool
	IsGoType bool
	Decl     *ast.TypeDecl
	Pos      token.Position
}

// EnumSymbol represents a user-defined enum type.
type EnumSymbol struct {
	Name       string
	GoName     string
	ModuleID   string
	Variants   []string
	VariantSet map[string]bool
	Public     bool
	Decl       *ast.EnumDecl
	Pos        token.Position
}

// CallKind identifies how a CallExpr was resolved during semantic analysis.
type CallKind int

const (
	CallFunc CallKind = iota
	CallStructCtor
	CallMethod
	CallUFCS
	CallError
)

// CallInfo records the resolved target of a CallExpr for code generation.
type CallInfo struct {
	Kind       CallKind
	Fn         *FnSymbol
	Struct     *StructSymbol
	NamedArgs  bool
	MethodName string
	Receiver   ast.Expr
	CanError   bool
	HasValue   bool
}

// TypedProgram holds the AST and semantic type information for an analyzed CleanBasic program.
type TypedProgram struct {
	EntryFile    string
	Main         *ast.Program
	Modules      []*ModuleInfo
	EntryModule  *ModuleInfo
	ModuleByID   map[string]*ModuleInfo
	ExprTypes    map[ast.Expr]Type
	VarTypes     map[*ast.VarDecl]Type
	ConstTypes   map[*ast.ConstDecl]Type
	IdentGoNames map[ast.Expr]string
	Functions    map[string]*FnSymbol
	CallInfos    map[*ast.CallExpr]*CallInfo
}

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

// Analyze performs semantic analysis on a parsed *ast.Program (and any imported modules).
func Analyze(prog *ast.Program) (*TypedProgram, error) {
	if prog == nil {
		return nil, fmt.Errorf("nil AST program")
	}

	rootDir := ""
	if prog.File != "" {
		if abs, err := filepath.Abs(prog.File); err == nil {
			rootDir = filepath.Dir(abs)
		}
	}
	loader := newModuleLoader(rootDir)
	topoOrder, entryMod, byID, err := loader.loadEntry(prog.File, prog)
	if err != nil {
		return nil, err
	}

	tp := &TypedProgram{
		EntryFile:    prog.File,
		Main:         prog,
		Modules:      topoOrder,
		EntryModule:  entryMod,
		ModuleByID:   byID,
		ExprTypes:    make(map[ast.Expr]Type),
		VarTypes:     make(map[*ast.VarDecl]Type),
		ConstTypes:   make(map[*ast.ConstDecl]Type),
		IdentGoNames: make(map[ast.Expr]string),
		Functions:    entryMod.Functions,
		CallInfos:    make(map[*ast.CallExpr]*CallInfo),
	}

	a := &analyzer{
		prog: tp,
	}

	for _, mod := range topoOrder {
		if mod.Program == nil {
			continue
		}
		a.mod = mod
		// Register built-in variadic print function in each module
		mod.Functions["print"] = &FnSymbol{
			Name:       "print",
			GoName:     "print",
			ReturnType: VoidType,
			IsVariadic: true,
		}
		if err := a.analyzeProgram(mod.Program); err != nil {
			return nil, err
		}
	}

	return tp, nil
}

type analyzer struct {
	prog         *TypedProgram
	mod          *ModuleInfo
	currentFn    *FnSymbol
	loopDepth    int
	tryDepth     int
	deferDepth   int
	startJobCall *ast.CallExpr
}

func (a *analyzer) lookupStruct(t Type) *StructSymbol {
	if t.ModuleID != "" {
		if m, ok := a.prog.ModuleByID[t.ModuleID]; ok {
			return m.Structs[t.Name]
		}
	}
	return a.mod.Structs[t.Name]
}
func (a *analyzer) isComparableType(t Type) bool {
	return a.isComparableTypeVisited(t, make(map[string]bool))
}

func (a *analyzer) isComparableTypeVisited(t Type, visited map[string]bool) bool {
	switch t.Kind {
	case KindInt, KindFloat, KindStr, KindByte, KindBool, KindEnum:
		return true
	case KindVoid, KindList, KindDict, KindErr, KindAny:
		return false
	case KindStruct:
		key := "struct:" + t.ModuleID + "." + t.Name
		if visited[key] {
			return true
		}
		visited[key] = true
		sym := a.lookupStruct(t)
		if sym == nil || sym.IsGoType {
			return false
		}
		for _, f := range sym.Fields {
			if f.Boxed || !a.isComparableTypeVisited(f.Type, visited) {
				return false
			}
		}
		return true

	default:
		return false
	}
}
