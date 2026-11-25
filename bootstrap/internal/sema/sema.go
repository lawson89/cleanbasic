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
type TypeKind = ast.TypeKind

const (
	KindVoid        = ast.KindVoid
	KindInt         = ast.KindInt
	KindFloat       = ast.KindFloat
	KindStr         = ast.KindStr
	KindBool        = ast.KindBool
	KindByte        = ast.KindByte
	KindStruct      = ast.KindStruct
	KindEnum        = ast.KindEnum
	KindList        = ast.KindList
	KindDict        = ast.KindDict
	KindJob         = ast.KindJob
	KindCancelToken = ast.KindCancelToken
	KindErr         = ast.KindErr
)

// Type represents a resolved semantic type in CleanBasic.
type Type = ast.Type

var (
	VoidType        = ast.VoidType
	IntType         = ast.IntType
	FloatType       = ast.FloatType
	StrType         = ast.StrType
	BoolType        = ast.BoolType
	ByteType        = ast.ByteType
	CancelTokenType = ast.CancelTokenType
	ErrType         = ast.ErrType
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
	Name     string
	GoName   string
	Type     Type
	IsConst  bool
	IsGlobal bool
	Public   bool
	ByRef    bool
	Pos      token.Position
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
	Modules     []*ModuleInfo
	EntryModule *ModuleInfo
	ModuleByID  map[string]*ModuleInfo
}

// LookupStruct resolves a struct symbol by its semantic Type, falling back to fallbackMod when ModuleID is empty.
func (p *TypedProgram) LookupStruct(t Type, fallbackMod *ModuleInfo) *StructSymbol {
	if t.ModuleID != "" && p != nil {
		if m, ok := p.ModuleByID[t.ModuleID]; ok {
			return m.Structs[t.Name]
		}
	}
	if fallbackMod != nil {
		return fallbackMod.Structs[t.Name]
	}
	return nil
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
		Modules:     topoOrder,
		EntryModule: entryMod,
		ModuleByID:  byID,
	}

	a := &analyzer{
		prog:           tp,
		fnGlobalAccess: make(map[*FnSymbol]globalAccessInfo),
		fnCallees:      make(map[*FnSymbol][]*FnSymbol),
	}

	for _, mod := range topoOrder {
		if mod.Program == nil {
			continue
		}
		a.mod = mod
		// Register built-in single-argument print(msg as str) function in each module
		mod.Functions["print"] = &FnSymbol{
			Name:       "print",
			Params:     []ParamSymbol{{Name: "msg", Type: StrType}},
			ReturnType: VoidType,
		}
		if err := a.analyzeProgram(mod.Program); err != nil {
			return nil, err
		}
	}

	if err := a.validateStartJobIsolation(); err != nil {
		return nil, err
	}

	return tp, nil
}

type globalAccessInfo struct {
	pos     token.Position
	varName string
}

type startSite struct {
	pos token.Position
	fn  *FnSymbol
}

type analyzer struct {
	prog                *TypedProgram
	mod                 *ModuleInfo
	currentFn           *FnSymbol
	loopDepth           int
	tryDepth            int
	deferDepth          int
	allowedFallibleExpr ast.Expr
	startCall           *ast.CallExpr
	fnGlobalAccess      map[*FnSymbol]globalAccessInfo
	fnCallees           map[*FnSymbol][]*FnSymbol
	startSites          []startSite
}

func (a *analyzer) recordGlobalAccess(fn *FnSymbol, pos token.Position, varName string) {
	if fn == nil {
		return
	}
	if _, exists := a.fnGlobalAccess[fn]; !exists {
		a.fnGlobalAccess[fn] = globalAccessInfo{pos: pos, varName: varName}
	}
}

func (a *analyzer) recordCallee(caller, callee *FnSymbol) {
	if caller == nil || callee == nil {
		return
	}
	a.fnCallees[caller] = append(a.fnCallees[caller], callee)
}

func (a *analyzer) validateStartJobIsolation() error {
	for _, site := range a.startSites {
		visited := map[*FnSymbol]bool{site.fn: true}
		queue := []*FnSymbol{site.fn}
		for len(queue) > 0 {
			fn := queue[0]
			queue = queue[1:]
			if acc, hasGlobal := a.fnGlobalAccess[fn]; hasGlobal {
				if fn == site.fn {
					return newSemaError(acc.pos, "function %q started with 'start' cannot access mutable module-level variable %q (pass values as arguments or use 'const')", site.fn.Name, acc.varName)
				}
				return newSemaError(site.pos, "function %q started with 'start' transitively accesses mutable module-level variable %q via %q at %s", site.fn.Name, acc.varName, fn.Name, acc.pos.String())
			}
			for _, callee := range a.fnCallees[fn] {
				if !visited[callee] {
					visited[callee] = true
					queue = append(queue, callee)
				}
			}
		}
	}
	return nil
}

func (a *analyzer) isSendableType(t Type, visited map[string]bool) bool {
	switch t.Kind {
	case KindInt, KindFloat, KindStr, KindBool, KindByte, KindErr, KindEnum, KindCancelToken:
		return true
	case KindStruct:
		sym := a.lookupStruct(t)
		if sym == nil || sym.IsGoType {
			return false
		}
		key := sym.ModuleID + "." + sym.Name
		if visited[key] {
			return true
		}
		visited[key] = true
		for _, f := range sym.Fields {
			if !a.isSendableType(f.Type, visited) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (a *analyzer) lookupStruct(t Type) *StructSymbol {
	return a.prog.LookupStruct(t, a.mod)
}
func (a *analyzer) isComparableType(t Type) bool {
	switch t.Kind {
	case KindInt, KindFloat, KindStr, KindByte, KindBool, KindEnum:
		return true
	case KindVoid, KindList, KindDict, KindJob, KindCancelToken, KindErr:
		return false
	case KindStruct:
		sym := a.lookupStruct(t)
		if sym == nil || sym.IsGoType {
			return false
		}
		for _, f := range sym.Fields {
			if !a.isComparableType(f.Type) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
