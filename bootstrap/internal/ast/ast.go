package ast

import (
	"fmt"

	"cleanbasic/bootstrap/internal/token"
)

// Program represents a parsed CleanBasic source file (.cb module).
type Program struct {
	File  string
	Decls []TopLevelDecl
}

// TopLevelDecl is implemented by all top-level declarations in a CleanBasic module.
type TopLevelDecl interface {
	DeclPos() token.Position
	declNode()
}

// Statement is implemented by all statements inside functions and control blocks.
type Statement interface {
	StmtPos() token.Position
	stmtNode()
}

// Expr is implemented by all expressions in CleanBasic.
type Expr interface {
	ExprPos() token.Position
	ExprType() Type
	SetExprType(Type)
	exprNode()
}

// TypeKind distinguishes primitive, container, user-defined, and void types.
type TypeKind string

const (
	KindVoid    TypeKind = "void"
	KindInt     TypeKind = "int"
	KindFloat   TypeKind = "float"
	KindStr     TypeKind = "str"
	KindBool    TypeKind = "bool"
	KindByte    TypeKind = "byte"
	KindStruct  TypeKind = "struct"
	KindEnum    TypeKind = "enum"
	KindList   TypeKind = "list"
	KindDict   TypeKind = "dict"
	KindJob    TypeKind = "job"
	KindErr    TypeKind = "err"
)

// Type represents a syntactic or resolved semantic type in CleanBasic.
type Type struct {
	Pos      token.Position
	Kind     TypeKind
	Name     string
	ModuleID string
	GoName   string
	SubTypes []Type // [elem] for list/job; [key, val] for dict
}

func (t Type) Elem() Type {
	if len(t.SubTypes) > 0 {
		return t.SubTypes[0]
	}
	return VoidType
}

func (t Type) Key() Type {
	if len(t.SubTypes) > 0 {
		return t.SubTypes[0]
	}
	return VoidType
}

func (t Type) Val() Type {
	if len(t.SubTypes) > 1 {
		return t.SubTypes[1]
	}
	return VoidType
}

var (
	VoidType  = Type{Kind: KindVoid}
	IntType   = Type{Kind: KindInt}
	FloatType = Type{Kind: KindFloat}
	StrType   = Type{Kind: KindStr}
	BoolType  = Type{Kind: KindBool}
	ByteType  = Type{Kind: KindByte}
	ErrType   = Type{Kind: KindErr}
)

func (t Type) String() string {
	switch t.Kind {
	case "":
		if t.Name != "" {
			return t.Name
		}
		return "void"
	case KindStruct, KindEnum:
		return t.Name
	case KindList:
		if len(t.SubTypes) == 1 {
			return fmt.Sprintf("list[%s]", t.SubTypes[0].String())
		}
		return "list[?]"
	case KindJob:
		if len(t.SubTypes) == 0 || t.SubTypes[0].IsVoid() {
			return "job"
		}
		return fmt.Sprintf("job[%s]", t.SubTypes[0].String())
	case KindDict:
		if len(t.SubTypes) == 2 {
			return fmt.Sprintf("dict[%s, %s]", t.SubTypes[0].String(), t.SubTypes[1].String())
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
	if t.Kind != other.Kind {
		return false
	}
	switch t.Kind {
	case KindStruct, KindEnum:
		return t.Name == other.Name && t.ModuleID == other.ModuleID
	case KindList, KindDict:
		if len(t.SubTypes) != len(other.SubTypes) {
			return false
		}
		for i := range t.SubTypes {
			if !t.SubTypes[i].Equals(other.SubTypes[i]) {
				return false
			}
		}
		return true
	case KindJob:
		return t.Elem().Equals(other.Elem())
	default:
		return true
	}
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
	Pos        token.Position
}

// FieldSymbol represents a field in a user-defined struct type.
type FieldSymbol struct {
	Name string
	Type Type
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
}

// EnumSymbol represents a user-defined enum type.
type EnumSymbol struct {
	Name       string
	GoName     string
	ModuleID   string
	VariantSet map[string]bool
	Public     bool
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
	MethodName string
	Receiver   Expr
	CanError   bool
	HasValue   bool
}

// --- Top-Level Declarations ---

// ImportDecl represents: import "path.cb" [as alias]
type ImportDecl struct {
	Pos   token.Position
	Path  string
	Alias string
}

func (d *ImportDecl) DeclPos() token.Position { return d.Pos }
func (d *ImportDecl) declNode()               {}

// GoImportDecl represents: go import "pkg" [as alias]
type GoImportDecl struct {
	Pos   token.Position
	Path  string
	Alias string
}

func (d *GoImportDecl) DeclPos() token.Position { return d.Pos }
func (d *GoImportDecl) declNode()               {}

// GoBlockDecl represents a top-level raw Go code block: go ... end go
type GoBlockDecl struct {
	Pos  token.Position
	Code string
}

func (d *GoBlockDecl) DeclPos() token.Position { return d.Pos }
func (d *GoBlockDecl) declNode()               {}

// ConstDecl represents: [public] const Name [as Type] = Value
type ConstDecl struct {
	Pos    token.Position
	Public bool
	Name   string
	Type   *Type // Optional explicit type; resolved in place by sema
	Value  Expr
}

func (d *ConstDecl) DeclPos() token.Position { return d.Pos }
func (d *ConstDecl) declNode()               {}

// FieldDecl represents a field inside a TypeDecl: Name as Type
type FieldDecl struct {
	Pos  token.Position
	Name string
	Type Type
}

// TypeDecl represents: [public] type Name ... end type or [public] type Name = go "GoType"
type TypeDecl struct {
	Pos    token.Position
	Public bool
	Name   string
	GoName string
	Fields []FieldDecl
}

func (d *TypeDecl) DeclPos() token.Position { return d.Pos }
func (d *TypeDecl) declNode()               {}

// EnumDecl represents: [public] enum Name ... end enum
type EnumDecl struct {
	Pos      token.Position
	Public   bool
	Name     string
	Variants []string
}

func (d *EnumDecl) DeclPos() token.Position { return d.Pos }
func (d *EnumDecl) declNode()               {}

// Param represents a function parameter: [byref] Name as Type
type Param struct {
	Pos   token.Position
	ByRef bool
	Name  string
	Type  Type
}

// FnDecl represents a function or Go-bound function declaration.
type FnDecl struct {
	Pos        token.Position
	Public     bool
	Name       string
	Params     []Param
	ReturnType *Type // nil if void
	CanError   bool
	GoTarget   string
	Body       []Statement
}

func (d *FnDecl) DeclPos() token.Position { return d.Pos }
func (d *FnDecl) declNode()               {}

// --- Statements ---

// VarDecl represents: [public] dim Name [as Type] [= Value]
type VarDecl struct {
	Pos    token.Position
	Public bool
	Name   string
	Type   *Type // nil if inferred; resolved in place by sema
	Value  Expr  // nil if zero-initialized via 'dim Name as Type'
}

func (s *VarDecl) DeclPos() token.Position { return s.Pos }
func (s *VarDecl) declNode()               {}
func (s *VarDecl) StmtPos() token.Position { return s.Pos }
func (s *VarDecl) stmtNode()               {}

// Assignment represents: Target = Value
type Assignment struct {
	Pos    token.Position
	Target Expr
	Value  Expr
}

func (s *Assignment) StmtPos() token.Position { return s.Pos }
func (s *Assignment) stmtNode()               {}

// ExprStmt represents a standalone expression statement (e.g., function call).
type ExprStmt struct {
	Pos  token.Position
	Expr Expr
}

func (s *ExprStmt) StmtPos() token.Position { return s.Pos }
func (s *ExprStmt) stmtNode()               {}

// ElseIfBranch represents an "else if Cond" branch in an IfStmt.
type ElseIfBranch struct {
	Pos  token.Position
	Cond Expr
	Body []Statement
}

// IfStmt represents: if Cond ... (else if Cond ...)* [else ...] end if
type IfStmt struct {
	Pos      token.Position
	Cond     Expr
	ThenBody []Statement
	ElseIfs  []ElseIfBranch
	ElseBody []Statement
	HasElse  bool
}

func (s *IfStmt) StmtPos() token.Position { return s.Pos }
func (s *IfStmt) stmtNode()               {}

// ForStmt represents either a numeric range loop ("for i = start to end")
// or a collection loop ("for x [, y] in iterable").
type ForStmt struct {
	Pos     token.Position
	IsRange bool

	// Range loop fields: for VarName = Start to End
	VarName string
	Start   Expr
	End     Expr

	// For-in loop fields: for Vars in Iterable
	Vars     []string
	Iterable Expr

	Body []Statement
}

func (s *ForStmt) StmtPos() token.Position { return s.Pos }
func (s *ForStmt) stmtNode()               {}

// WhileStmt represents: while Cond ... end while
type WhileStmt struct {
	Pos  token.Position
	Cond Expr
	Body []Statement
}

func (s *WhileStmt) StmtPos() token.Position { return s.Pos }
func (s *WhileStmt) stmtNode()               {}

// CaseClause represents: case ExprList ...
type CaseClause struct {
	Pos    token.Position
	Values []Expr
	Body   []Statement
}

// SelectStmt represents: select case Target ... [case else ...] end select
type SelectStmt struct {
	Pos      token.Position
	Target   Expr
	Cases    []CaseClause
	ElseBody []Statement
	HasElse  bool
}

func (s *SelectStmt) StmtPos() token.Position { return s.Pos }
func (s *SelectStmt) stmtNode()               {}

// TryCatchStmt represents: try ... catch CatchVar ... end try
type TryCatchStmt struct {
	Pos       token.Position
	TryBody   []Statement
	CatchVar  string
	CatchBody []Statement
}

func (s *TryCatchStmt) StmtPos() token.Position { return s.Pos }
func (s *TryCatchStmt) stmtNode()               {}

// DeferStmt represents: defer ... end defer
type DeferStmt struct {
	Pos  token.Position
	Body []Statement
}

func (s *DeferStmt) StmtPos() token.Position { return s.Pos }
func (s *DeferStmt) stmtNode()               {}

// ReturnStmt represents: return [Value]
type ReturnStmt struct {
	Pos   token.Position
	Value Expr // nil for void return
}

func (s *ReturnStmt) StmtPos() token.Position { return s.Pos }
func (s *ReturnStmt) stmtNode()               {}

// BreakStmt represents: break
type BreakStmt struct {
	Pos token.Position
}

func (s *BreakStmt) StmtPos() token.Position { return s.Pos }
func (s *BreakStmt) stmtNode()               {}

// ContinueStmt represents: continue
type ContinueStmt struct {
	Pos token.Position
}

func (s *ContinueStmt) StmtPos() token.Position { return s.Pos }
func (s *ContinueStmt) stmtNode()               {}

// --- Expressions ---

// IdentExpr represents a simple identifier reference.
type IdentExpr struct {
	Pos      token.Position
	Name     string
	GoName   string
	ByRef    bool
	SemaType Type
}

func (e *IdentExpr) ExprPos() token.Position { return e.Pos }
func (e *IdentExpr) ExprType() Type          { return e.SemaType }
func (e *IdentExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *IdentExpr) exprNode()               {}

// IntLitExpr represents an integer literal.
type IntLitExpr struct {
	Pos      token.Position
	Raw      string
	SemaType Type
}

func (e *IntLitExpr) ExprPos() token.Position { return e.Pos }
func (e *IntLitExpr) ExprType() Type          { return e.SemaType }
func (e *IntLitExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *IntLitExpr) exprNode()               {}

// FloatLitExpr represents a floating-point literal.
type FloatLitExpr struct {
	Pos      token.Position
	Raw      string
	SemaType Type
}

func (e *FloatLitExpr) ExprPos() token.Position { return e.Pos }
func (e *FloatLitExpr) ExprType() Type          { return e.SemaType }
func (e *FloatLitExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *FloatLitExpr) exprNode()               {}

// StringLitExpr represents a string literal.
type StringLitExpr struct {
	Pos      token.Position
	Value    string
	SemaType Type
}

func (e *StringLitExpr) ExprPos() token.Position { return e.Pos }
func (e *StringLitExpr) ExprType() Type          { return e.SemaType }
func (e *StringLitExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *StringLitExpr) exprNode()               {}

// ByteLitExpr represents a single-quoted ASCII byte literal.
type ByteLitExpr struct {
	Pos      token.Position
	Value    byte
	SemaType Type
}

func (e *ByteLitExpr) ExprPos() token.Position { return e.Pos }
func (e *ByteLitExpr) ExprType() Type          { return e.SemaType }
func (e *ByteLitExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *ByteLitExpr) exprNode()               {}

// BoolLitExpr represents a boolean literal (true or false).
type BoolLitExpr struct {
	Pos      token.Position
	Value    bool
	SemaType Type
}

func (e *BoolLitExpr) ExprPos() token.Position { return e.Pos }
func (e *BoolLitExpr) ExprType() Type          { return e.SemaType }
func (e *BoolLitExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *BoolLitExpr) exprNode()               {}

// ListLitExpr represents a list literal: [e1, e2, ...].
type ListLitExpr struct {
	Pos      token.Position
	Elements []Expr
	SemaType Type
}

func (e *ListLitExpr) ExprPos() token.Position { return e.Pos }
func (e *ListLitExpr) ExprType() Type          { return e.SemaType }
func (e *ListLitExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *ListLitExpr) exprNode()               {}

// DictEntryExpr represents a key: value pair inside a DictLitExpr.
type DictEntryExpr struct {
	Pos   token.Position
	Key   Expr
	Value Expr
}

// DictLitExpr represents a dictionary literal: {k1: v1, ...}.
type DictLitExpr struct {
	Pos      token.Position
	Entries  []DictEntryExpr
	SemaType Type
}

func (e *DictLitExpr) ExprPos() token.Position { return e.Pos }
func (e *DictLitExpr) ExprType() Type          { return e.SemaType }
func (e *DictLitExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *DictLitExpr) exprNode()               {}

// TypedEmptyCtorExpr represents a typed container constructor:
// list[T]() or dict[K, V]()
type TypedEmptyCtorExpr struct {
	Pos  token.Position
	Type Type
	Args []Expr
}

func (e *TypedEmptyCtorExpr) ExprPos() token.Position { return e.Pos }
func (e *TypedEmptyCtorExpr) ExprType() Type          { return e.Type }
func (e *TypedEmptyCtorExpr) SetExprType(t Type)      { e.Type = t }
func (e *TypedEmptyCtorExpr) exprNode()               {}

// UnaryExpr represents a prefix unary operation: not, +, or -
type UnaryExpr struct {
	Pos      token.Position
	Op       token.Kind
	Operand  Expr
	SemaType Type
}

func (e *UnaryExpr) ExprPos() token.Position { return e.Pos }
func (e *UnaryExpr) ExprType() Type          { return e.SemaType }
func (e *UnaryExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *UnaryExpr) exprNode()               {}

// BinaryExpr represents an infix binary operation:
// or, and, ==, !=, <, >, <=, >=, +, -, *, /, %
type BinaryExpr struct {
	Pos      token.Position
	Left     Expr
	Op       token.Kind
	Right    Expr
	SemaType Type
}

func (e *BinaryExpr) ExprPos() token.Position { return e.Pos }
func (e *BinaryExpr) ExprType() Type          { return e.SemaType }
func (e *BinaryExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *BinaryExpr) exprNode()               {}

// FieldAccessExpr represents postfix dot access: Target.Field
type FieldAccessExpr struct {
	Pos      token.Position
	Target   Expr
	Field    string
	GoName   string
	SemaType Type
}

func (e *FieldAccessExpr) ExprPos() token.Position { return e.Pos }
func (e *FieldAccessExpr) ExprType() Type          { return e.SemaType }
func (e *FieldAccessExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *FieldAccessExpr) exprNode()               {}

// IndexExpr represents postfix indexing: Target[Index]
type IndexExpr struct {
	Pos      token.Position
	Target   Expr
	Index    Expr
	SemaType Type
}

func (e *IndexExpr) ExprPos() token.Position { return e.Pos }
func (e *IndexExpr) ExprType() Type          { return e.SemaType }
func (e *IndexExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *IndexExpr) exprNode()               {}

// SliceExpr represents postfix slicing: Target[Start:End]
type SliceExpr struct {
	Pos      token.Position
	Target   Expr
	Start    Expr // nil if omitted (e.g. x[:end])
	End      Expr // nil if omitted (e.g. x[start:])
	SemaType Type
}

func (e *SliceExpr) ExprPos() token.Position { return e.Pos }
func (e *SliceExpr) ExprType() Type          { return e.SemaType }
func (e *SliceExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *SliceExpr) exprNode()               {}

// CallExpr represents a function, constructor, or UFCS method call: Callee(Args...)
type CallExpr struct {
	Pos      token.Position
	Callee   Expr
	Args     []Expr
	CallInfo *CallInfo
	SemaType Type
}

func (e *CallExpr) ExprPos() token.Position { return e.Pos }
func (e *CallExpr) ExprType() Type          { return e.SemaType }
func (e *CallExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *CallExpr) exprNode()               {}

// StartExpr represents launching a concurrent background job: start fnCall(...)
type StartExpr struct {
	Pos      token.Position
	Call     *CallExpr
	SemaType Type
}

func (e *StartExpr) ExprPos() token.Position { return e.Pos }
func (e *StartExpr) ExprType() Type          { return e.SemaType }
func (e *StartExpr) SetExprType(t Type)      { e.SemaType = t }
func (e *StartExpr) exprNode()               {}
