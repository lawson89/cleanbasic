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
type TypeKind = ast.SemaTypeKind

const (
	KindVoid    = ast.KindVoid
	KindInt     = ast.KindInt
	KindFloat   = ast.KindFloat
	KindStr     = ast.KindStr
	KindBool    = ast.KindBool
	KindByte    = ast.KindByte
	KindStruct  = ast.KindStruct
	KindEnum    = ast.KindEnum
	KindList    = ast.KindList
	KindDict    = ast.KindDict
	KindMailbox = ast.KindMailbox
	KindErr     = ast.KindErr
)

// Type represents a resolved semantic type in CleanBasic.
type Type = ast.SemaType

var (
	VoidType  = ast.VoidType
	IntType   = ast.IntType
	FloatType = ast.FloatType
	StrType   = ast.StrType
	BoolType  = ast.BoolType
	ByteType  = ast.ByteType
	ErrType   = ast.ErrType
)

// ParamSymbol, FnSymbol, FieldSymbol, StructSymbol, EnumSymbol, CallKind, and CallInfo
// are defined in package ast so AST nodes can store resolved metadata directly.
type ParamSymbol = ast.ParamSymbol
type FnSymbol = ast.FnSymbol
type FieldSymbol = ast.FieldSymbol
type StructSymbol = ast.StructSymbol
type EnumSymbol = ast.EnumSymbol
type CallKind = ast.CallKind
type CallInfo = ast.CallInfo

const (
	CallFunc       = ast.CallFunc
	CallStructCtor = ast.CallStructCtor
	CallMethod     = ast.CallMethod
	CallUFCS       = ast.CallUFCS
	CallError      = ast.CallError
)

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

// TypedProgram holds the AST and module symbol tables for an analyzed CleanBasic program.
type TypedProgram struct {
	EntryFile   string
	Main        *ast.Program
	Modules     []*ModuleInfo
	EntryModule *ModuleInfo
	ModuleByID  map[string]*ModuleInfo
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
		EntryFile:   prog.File,
		Main:        prog,
		Modules:     topoOrder,
		EntryModule: entryMod,
		ModuleByID:  byID,
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
	prog                *TypedProgram
	mod                 *ModuleInfo
	currentFn           *FnSymbol
	loopDepth           int
	tryDepth            int
	deferDepth          int
	allowedFallibleExpr ast.Expr
	startJobCall        *ast.CallExpr
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
	case KindVoid, KindList, KindDict, KindErr:
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
			if !a.isComparableTypeVisited(f.Type, visited) {
				return false
			}
		}
		return true

	default:
		return false
	}
}
