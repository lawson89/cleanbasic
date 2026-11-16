package ast

import (
	"fmt"
	"strings"

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
	ExprType() SemaType
	SetExprType(SemaType)
	exprNode()
}

// SemaTypeKind distinguishes primitive, container, user-defined, and void semantic types.
type SemaTypeKind string

const (
	KindVoid    SemaTypeKind = "void"
	KindInt     SemaTypeKind = "int"
	KindFloat   SemaTypeKind = "float"
	KindStr     SemaTypeKind = "str"
	KindBool    SemaTypeKind = "bool"
	KindByte    SemaTypeKind = "byte"
	KindStruct  SemaTypeKind = "struct"
	KindEnum    SemaTypeKind = "enum"
	KindList    SemaTypeKind = "list"
	KindDict    SemaTypeKind = "dict"
	KindMailbox SemaTypeKind = "mailbox"
	KindErr     SemaTypeKind = "err"
)

// SemaType represents a resolved semantic type in CleanBasic.
type SemaType struct {
	Kind     SemaTypeKind
	Name     string
	ModuleID string
	GoName   string
	SubTypes []SemaType // [elem] for list/mailbox; [key, val] for dict
}

func (t SemaType) Elem() SemaType {
	if len(t.SubTypes) > 0 {
		return t.SubTypes[0]
	}
	return VoidType
}

func (t SemaType) Key() SemaType {
	if len(t.SubTypes) > 0 {
		return t.SubTypes[0]
	}
	return VoidType
}

func (t SemaType) Val() SemaType {
	if len(t.SubTypes) > 1 {
		return t.SubTypes[1]
	}
	return VoidType
}

var (
	VoidType  = SemaType{Kind: KindVoid}
	IntType   = SemaType{Kind: KindInt}
	FloatType = SemaType{Kind: KindFloat}
	StrType   = SemaType{Kind: KindStr}
	BoolType  = SemaType{Kind: KindBool}
	ByteType  = SemaType{Kind: KindByte}
	ErrType   = SemaType{Kind: KindErr}
)

func (t SemaType) String() string {
	switch t.Kind {
	case "":
		return "void"
	case KindStruct, KindEnum:
		return t.Name
	case KindList, KindMailbox:
		if len(t.SubTypes) == 1 {
			return fmt.Sprintf("%s[%s]", t.Kind, t.SubTypes[0].String())
		}
		return fmt.Sprintf("%s[?]", t.Kind)
	case KindDict:
		if len(t.SubTypes) == 2 {
			return fmt.Sprintf("dict[%s, %s]", t.SubTypes[0].String(), t.SubTypes[1].String())
		}
		return "dict[?, ?]"
	default:
		return string(t.Kind)
	}
}

func (t SemaType) IsVoid() bool {
	return t.Kind == "" || t.Kind == KindVoid
}

func (t SemaType) Equals(other SemaType) bool {
	if t.Kind != other.Kind {
		return false
	}
	switch t.Kind {
	case KindStruct, KindEnum:
		return t.Name == other.Name && t.ModuleID == other.ModuleID
	case KindList, KindMailbox, KindDict:
		if len(t.SubTypes) != len(other.SubTypes) {
			return false
		}
		for i := range t.SubTypes {
			if !t.SubTypes[i].Equals(other.SubTypes[i]) {
				return false
			}
		}
		return true
	default:
		return true
	}
}

// ParamSymbol represents a typed function parameter.
type ParamSymbol struct {
	Name  string
	Type  SemaType
	ByRef bool
	Pos   token.Position
}

// FnSymbol represents a declared or built-in function signature.
type FnSymbol struct {
	Name       string
	GoName     string
	Params     []ParamSymbol
	ReturnType SemaType
	CanError   bool
	Public     bool
	GoTarget   string
	IsVariadic bool
	Decl       *FnDecl
	Pos        token.Position
}

// FieldSymbol represents a field in a user-defined struct type.
type FieldSymbol struct {
	Name string
	Type SemaType
	Pos  token.Position
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
	Decl     *TypeDecl
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
	Decl       *EnumDecl
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
	MethodName string
	Receiver   Expr
	CanError   bool
	HasValue   bool
}

// TypeKind distinguishes the category of a TypeExpr.
type TypeKind string

const (
	TypeBasic     TypeKind = "basic"     // int, float, str, bool, byte, err
	TypeList      TypeKind = "list"      // list[ElemType]
	TypeDict      TypeKind = "dict"      // dict[KeyType, ValType]
	TypeMailbox   TypeKind = "mailbox"   // mailbox[ElemType]
	TypeQualified TypeKind = "qualified" // Player or mod.Player
)

// TypeExpr represents a static type annotation in CleanBasic.
type TypeExpr struct {
	Pos      token.Position
	Kind     TypeKind
	Name     string     // For TypeBasic ("int", "str", ...)
	Parts    []string   // For TypeQualified (e.g., ["Player"] or ["player", "Player"])
	SubTypes []TypeExpr // [elem] for list/mailbox/job; [key, val] for dict
}

func (t TypeExpr) String() string {
	switch t.Kind {
	case TypeBasic:
		return t.Name
	case TypeList, TypeMailbox:
		if len(t.SubTypes) == 1 {
			return fmt.Sprintf("%s[%s]", t.Kind, t.SubTypes[0].String())
		}
		return string(t.Kind) + "[]"
	case TypeDict:
		if len(t.SubTypes) == 2 {
			return fmt.Sprintf("dict[%s, %s]", t.SubTypes[0].String(), t.SubTypes[1].String())
		}
		return "dict[]"
	case TypeQualified:
		return strings.Join(t.Parts, ".")
	default:
		return "<unknown-type>"
	}
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
	Pos      token.Position
	Public   bool
	Name     string
	Type     *TypeExpr // Optional explicit type
	SemaType SemaType
	Value    Expr
}

func (d *ConstDecl) DeclPos() token.Position { return d.Pos }
func (d *ConstDecl) declNode()               {}

// FieldDecl represents a field inside a TypeDecl: Name as Type
type FieldDecl struct {
	Pos  token.Position
	Name string
	Type TypeExpr
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
	Type  TypeExpr
}

// FnDecl represents a function or Go-bound function declaration.
type FnDecl struct {
	Pos        token.Position
	Public     bool
	Name       string
	Params     []Param
	ReturnType *TypeExpr // nil if void
	CanError   bool
	GoTarget   string
	Body       []Statement
}

func (d *FnDecl) DeclPos() token.Position { return d.Pos }
func (d *FnDecl) declNode()               {}

// --- Statements ---

// VarDecl represents: dim Name [as Type] = Value
type VarDecl struct {
	Pos      token.Position
	Name     string
	Type     *TypeExpr // nil if inferred
	SemaType SemaType
	Value    Expr
}

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
	SemaType SemaType
}

func (e *IdentExpr) ExprPos() token.Position { return e.Pos }
func (e *IdentExpr) ExprType() SemaType      { return e.SemaType }
func (e *IdentExpr) SetExprType(t SemaType)  { e.SemaType = t }
func (e *IdentExpr) exprNode()               {}

// IntLitExpr represents an integer literal.
type IntLitExpr struct {
	Pos      token.Position
	Raw      string
	SemaType SemaType
}

func (e *IntLitExpr) ExprPos() token.Position { return e.Pos }
func (e *IntLitExpr) ExprType() SemaType      { return e.SemaType }
func (e *IntLitExpr) SetExprType(t SemaType)  { e.SemaType = t }
func (e *IntLitExpr) exprNode()               {}

// FloatLitExpr represents a floating-point literal.
type FloatLitExpr struct {
	Pos      token.Position
	Raw      string
	SemaType SemaType
}

func (e *FloatLitExpr) ExprPos() token.Position { return e.Pos }
func (e *FloatLitExpr) ExprType() SemaType      { return e.SemaType }
func (e *FloatLitExpr) SetExprType(t SemaType)  { e.SemaType = t }
func (e *FloatLitExpr) exprNode()               {}

// StringLitExpr represents a string literal.
type StringLitExpr struct {
	Pos      token.Position
	Value    string
	SemaType SemaType
}

func (e *StringLitExpr) ExprPos() token.Position { return e.Pos }
func (e *StringLitExpr) ExprType() SemaType      { return e.SemaType }
func (e *StringLitExpr) SetExprType(t SemaType)  { e.SemaType = t }
func (e *StringLitExpr) exprNode()               {}

// BoolLitExpr represents a boolean literal (true or false).
type BoolLitExpr struct {
	Pos      token.Position
	Value    bool
	SemaType SemaType
}

func (e *BoolLitExpr) ExprPos() token.Position { return e.Pos }
func (e *BoolLitExpr) ExprType() SemaType      { return e.SemaType }
func (e *BoolLitExpr) SetExprType(t SemaType)  { e.SemaType = t }
func (e *BoolLitExpr) exprNode()               {}

// ListLitExpr represents a list literal: [e1, e2, ...] (or [] in explicitly typed contexts).
type ListLitExpr struct {
	Pos      token.Position
	Elements []Expr
	SemaType SemaType
}

func (e *ListLitExpr) ExprPos() token.Position { return e.Pos }
func (e *ListLitExpr) ExprType() SemaType      { return e.SemaType }
func (e *ListLitExpr) SetExprType(t SemaType)  { e.SemaType = t }
func (e *ListLitExpr) exprNode()               {}

// DictEntryExpr represents a key: value pair inside a DictLitExpr.
type DictEntryExpr struct {
	Pos   token.Position
	Key   Expr
	Value Expr
}

// DictLitExpr represents a dictionary literal: {k1: v1, ...} (or {} in explicitly typed contexts).
type DictLitExpr struct {
	Pos      token.Position
	Entries  []DictEntryExpr
	SemaType SemaType
}

func (e *DictLitExpr) ExprPos() token.Position { return e.Pos }
func (e *DictLitExpr) ExprType() SemaType      { return e.SemaType }
func (e *DictLitExpr) SetExprType(t SemaType)  { e.SemaType = t }
func (e *DictLitExpr) exprNode()               {}

// TypedEmptyCtorExpr represents a typed container constructor:
// list[T](), dict[K, V](), mailbox[T](), or mailbox[T](cap)
type TypedEmptyCtorExpr struct {
	Pos      token.Position
	Type     TypeExpr
	Args     []Expr
	SemaType SemaType
}

func (e *TypedEmptyCtorExpr) ExprPos() token.Position { return e.Pos }
func (e *TypedEmptyCtorExpr) ExprType() SemaType      { return e.SemaType }
func (e *TypedEmptyCtorExpr) SetExprType(t SemaType)  { e.SemaType = t }
func (e *TypedEmptyCtorExpr) exprNode()               {}

// UnaryExpr represents a prefix unary operation: not, +, or -
type UnaryExpr struct {
	Pos      token.Position
	Op       token.Kind
	Operand  Expr
	SemaType SemaType
}

func (e *UnaryExpr) ExprPos() token.Position { return e.Pos }
func (e *UnaryExpr) ExprType() SemaType      { return e.SemaType }
func (e *UnaryExpr) SetExprType(t SemaType)  { e.SemaType = t }
func (e *UnaryExpr) exprNode()               {}

// BinaryExpr represents an infix binary operation:
// or, and, ==, !=, <, >, <=, >=, +, -, *, /, %
type BinaryExpr struct {
	Pos      token.Position
	Left     Expr
	Op       token.Kind
	Right    Expr
	SemaType SemaType
}

func (e *BinaryExpr) ExprPos() token.Position { return e.Pos }
func (e *BinaryExpr) ExprType() SemaType      { return e.SemaType }
func (e *BinaryExpr) SetExprType(t SemaType)  { e.SemaType = t }
func (e *BinaryExpr) exprNode()               {}

// FieldAccessExpr represents postfix dot access: Target.Field
type FieldAccessExpr struct {
	Pos      token.Position
	Target   Expr
	Field    string
	GoName   string
	SemaType SemaType
}

func (e *FieldAccessExpr) ExprPos() token.Position { return e.Pos }
func (e *FieldAccessExpr) ExprType() SemaType      { return e.SemaType }
func (e *FieldAccessExpr) SetExprType(t SemaType)  { e.SemaType = t }
func (e *FieldAccessExpr) exprNode()               {}

// IndexExpr represents postfix indexing: Target[Index]
type IndexExpr struct {
	Pos      token.Position
	Target   Expr
	Index    Expr
	SemaType SemaType
}

func (e *IndexExpr) ExprPos() token.Position { return e.Pos }
func (e *IndexExpr) ExprType() SemaType      { return e.SemaType }
func (e *IndexExpr) SetExprType(t SemaType)  { e.SemaType = t }
func (e *IndexExpr) exprNode()               {}

// SliceExpr represents postfix slicing: Target[Start:End]
type SliceExpr struct {
	Pos      token.Position
	Target   Expr
	Start    Expr // nil if omitted (e.g. x[:end])
	End      Expr // nil if omitted (e.g. x[start:])
	SemaType SemaType
}

func (e *SliceExpr) ExprPos() token.Position { return e.Pos }
func (e *SliceExpr) ExprType() SemaType      { return e.SemaType }
func (e *SliceExpr) SetExprType(t SemaType)  { e.SemaType = t }
func (e *SliceExpr) exprNode()               {}

// CallExpr represents a function, constructor, or UFCS method call: Callee(Args...)
type CallExpr struct {
	Pos      token.Position
	Callee   Expr
	Args     []Expr
	CallInfo *CallInfo
	SemaType SemaType
}

func (e *CallExpr) ExprPos() token.Position { return e.Pos }
func (e *CallExpr) ExprType() SemaType      { return e.SemaType }
func (e *CallExpr) SetExprType(t SemaType)  { e.SemaType = t }
func (e *CallExpr) exprNode()               {}

// StartJobStmt represents launching a concurrent background goroutine: start job fnCall(...)
type StartJobStmt struct {
	Pos  token.Position
	Call *CallExpr
}

func (s *StartJobStmt) StmtPos() token.Position { return s.Pos }
func (s *StartJobStmt) stmtNode()               {}
