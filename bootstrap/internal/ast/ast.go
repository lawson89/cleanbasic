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
	exprNode()
}

// TypeKind distinguishes the category of a TypeExpr.
type TypeKind string

const (
	TypeBasic     TypeKind = "basic"     // int, float, str, bool, err
	TypeList      TypeKind = "list"      // list[ElemType]
	TypeDict      TypeKind = "dict"      // dict[KeyType, ValType]
	TypeQualified TypeKind = "qualified" // Player or mod.Player
)

// TypeExpr represents a static type annotation in CleanBasic.
type TypeExpr struct {
	Pos      token.Position
	Kind     TypeKind
	Name     string    // For TypeBasic ("int", "str", ...)
	Parts    []string  // For TypeQualified (e.g., ["Player"] or ["player", "Player"])
	ElemType *TypeExpr // For TypeList
	KeyType  *TypeExpr // For TypeDict
	ValType  *TypeExpr // For TypeDict
}

func (t TypeExpr) String() string {
	switch t.Kind {
	case TypeBasic:
		return t.Name
	case TypeList:
		if t.ElemType != nil {
			return fmt.Sprintf("list[%s]", t.ElemType.String())
		}
		return "list[]"
	case TypeDict:
		if t.KeyType != nil && t.ValType != nil {
			return fmt.Sprintf("dict[%s, %s]", t.KeyType.String(), t.ValType.String())
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

// ConstDecl represents: [public] const Name [as Type] = Value
type ConstDecl struct {
	Pos    token.Position
	Public bool
	Name   string
	Type   *TypeExpr // Optional explicit type
	Value  Expr
}

func (d *ConstDecl) DeclPos() token.Position { return d.Pos }
func (d *ConstDecl) declNode()               {}

// FieldDecl represents a field inside a TypeDecl: Name as Type
type FieldDecl struct {
	Pos  token.Position
	Name string
	Type TypeExpr
}

// TypeDecl represents: [public] type Name ... end type
type TypeDecl struct {
	Pos    token.Position
	Public bool
	Name   string
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

// UnionVariant represents a single variant inside a UnionDecl: Name as Type
type UnionVariant struct {
	Pos  token.Position
	Name string
	Type TypeExpr
}

// UnionDecl represents: [public] union Name ... end union
type UnionDecl struct {
	Pos      token.Position
	Public   bool
	Name     string
	Variants []UnionVariant
}

func (d *UnionDecl) DeclPos() token.Position { return d.Pos }
func (d *UnionDecl) declNode()               {}

// Param represents a function parameter: [byref] Name as Type
type Param struct {
	Pos   token.Position
	ByRef bool
	Name  string
	Type  TypeExpr
}

// FnDecl represents a function or extern function declaration.
type FnDecl struct {
	Pos        token.Position
	Public     bool
	Extern     bool
	Name       string
	Params     []Param
	ReturnType *TypeExpr // nil if void
	CanError   bool
	Body       []Statement
}

func (d *FnDecl) DeclPos() token.Position { return d.Pos }
func (d *FnDecl) declNode()               {}

// --- Statements ---

// VarDecl represents: (dim | const) Name [as Type] = Value
type VarDecl struct {
	Pos     token.Position
	IsConst bool
	Name    string
	Type    *TypeExpr // nil if inferred
	Value   Expr
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

// ForStmt represents either a numeric range loop ("for i = start to end [step s]")
// or a collection loop ("for x [, y] in iterable").
type ForStmt struct {
	Pos     token.Position
	IsRange bool

	// Range loop fields: for VarName = Start to End [step Step]
	VarName string
	Start   Expr
	End     Expr
	Step    Expr // nil if omitted

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

// CaseClause represents: case ExprList [as Binding] ...
type CaseClause struct {
	Pos     token.Position
	Values  []Expr
	Binding string // optional union payload binding ("as r")
	Body    []Statement
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
	Pos  token.Position
	Name string
}

func (e *IdentExpr) ExprPos() token.Position { return e.Pos }
func (e *IdentExpr) exprNode()               {}

// IntLitExpr represents an integer literal.
type IntLitExpr struct {
	Pos   token.Position
	Raw   string
	Value int64
}

func (e *IntLitExpr) ExprPos() token.Position { return e.Pos }
func (e *IntLitExpr) exprNode()               {}

// FloatLitExpr represents a floating-point literal.
type FloatLitExpr struct {
	Pos   token.Position
	Raw   string
	Value float64
}

func (e *FloatLitExpr) ExprPos() token.Position { return e.Pos }
func (e *FloatLitExpr) exprNode()               {}

// StringLitExpr represents a string literal.
type StringLitExpr struct {
	Pos   token.Position
	Value string
}

func (e *StringLitExpr) ExprPos() token.Position { return e.Pos }
func (e *StringLitExpr) exprNode()               {}

// BoolLitExpr represents a boolean literal (true or false).
type BoolLitExpr struct {
	Pos   token.Position
	Value bool
}

func (e *BoolLitExpr) ExprPos() token.Position { return e.Pos }
func (e *BoolLitExpr) exprNode()               {}

// ListLitExpr represents a list literal: [e1, e2, ...] (or [] in explicitly typed contexts).
type ListLitExpr struct {
	Pos      token.Position
	Elements []Expr
}

func (e *ListLitExpr) ExprPos() token.Position { return e.Pos }
func (e *ListLitExpr) exprNode()               {}

// DictEntryExpr represents a key: value pair inside a DictLitExpr.
type DictEntryExpr struct {
	Pos   token.Position
	Key   Expr
	Value Expr
}

// DictLitExpr represents a dictionary literal: {k1: v1, ...} (or {} in explicitly typed contexts).
type DictLitExpr struct {
	Pos     token.Position
	Entries []DictEntryExpr
}

func (e *DictLitExpr) ExprPos() token.Position { return e.Pos }
func (e *DictLitExpr) exprNode()               {}

// TypedEmptyCtorExpr represents a typed empty container constructor:
// list[T]() or dict[K, V]()
type TypedEmptyCtorExpr struct {
	Pos  token.Position
	Type TypeExpr
}

func (e *TypedEmptyCtorExpr) ExprPos() token.Position { return e.Pos }
func (e *TypedEmptyCtorExpr) exprNode()               {}

// UnaryExpr represents a prefix unary operation: not, +, or -
type UnaryExpr struct {
	Pos     token.Position
	Op      token.Kind
	Operand Expr
}

func (e *UnaryExpr) ExprPos() token.Position { return e.Pos }
func (e *UnaryExpr) exprNode()               {}

// BinaryExpr represents an infix binary operation:
// or, and, ==, !=, <, >, <=, >=, +, -, &, *, /, %
type BinaryExpr struct {
	Pos   token.Position
	Left  Expr
	Op    token.Kind
	Right Expr
}

func (e *BinaryExpr) ExprPos() token.Position { return e.Pos }
func (e *BinaryExpr) exprNode()               {}

// FieldAccessExpr represents postfix dot access: Target.Field
type FieldAccessExpr struct {
	Pos    token.Position
	Target Expr
	Field  string
}

func (e *FieldAccessExpr) ExprPos() token.Position { return e.Pos }
func (e *FieldAccessExpr) exprNode()               {}

// IndexExpr represents postfix indexing: Target[Index]
type IndexExpr struct {
	Pos    token.Position
	Target Expr
	Index  Expr
}

func (e *IndexExpr) ExprPos() token.Position { return e.Pos }
func (e *IndexExpr) exprNode()               {}

// CallArg represents an argument in a function/method call, supporting optional named syntax (e.g. code=408).
type CallArg struct {
	Pos   token.Position
	Name  string // empty for positional arguments, non-empty for named arguments like code=408
	Value Expr
}

// CallExpr represents a function, constructor, or UFCS method call: Callee(Args...)
type CallExpr struct {
	Pos    token.Position
	Callee Expr
	Args   []CallArg
}

func (e *CallExpr) ExprPos() token.Position { return e.Pos }
func (e *CallExpr) exprNode()               {}

// --- Pretty Printer / AST Dump ---

// Dump returns a deterministic, human-readable multi-line string representation of the AST.
func Dump(prog *Program) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Program(%s)\n", prog.File)
	for _, decl := range prog.Decls {
		dumpDecl(&sb, decl, "  ")
	}
	return sb.String()
}

func dumpDecl(sb *strings.Builder, decl TopLevelDecl, indent string) {
	switch d := decl.(type) {
	case *ImportDecl:
		if d.Alias != "" {
			fmt.Fprintf(sb, "%sImport(%q as %s)\n", indent, d.Path, d.Alias)
		} else {
			fmt.Fprintf(sb, "%sImport(%q)\n", indent, d.Path)
		}

	case *ConstDecl:
		vis := ""
		if d.Public {
			vis = "public "
		}
		typeStr := ""
		if d.Type != nil {
			typeStr = " as " + d.Type.String()
		}
		fmt.Fprintf(sb, "%s%sConstDecl(%s%s = %s)\n", indent, vis, d.Name, typeStr, FormatExpr(d.Value))

	case *TypeDecl:
		vis := ""
		if d.Public {
			vis = "public "
		}
		fmt.Fprintf(sb, "%s%sTypeDecl(%s)\n", indent, vis, d.Name)
		for _, f := range d.Fields {
			fmt.Fprintf(sb, "%s  Field(%s as %s)\n", indent, f.Name, f.Type.String())
		}

	case *EnumDecl:
		vis := ""
		if d.Public {
			vis = "public "
		}
		fmt.Fprintf(sb, "%s%sEnumDecl(%s: [%s])\n", indent, vis, d.Name, strings.Join(d.Variants, ", "))

	case *UnionDecl:
		vis := ""
		if d.Public {
			vis = "public "
		}
		fmt.Fprintf(sb, "%s%sUnionDecl(%s)\n", indent, vis, d.Name)
		for _, v := range d.Variants {
			fmt.Fprintf(sb, "%s  Variant(%s as %s)\n", indent, v.Name, v.Type.String())
		}

	case *FnDecl:
		var mods []string
		if d.Public {
			mods = append(mods, "public")
		}
		if d.Extern {
			mods = append(mods, "extern")
		}
		modPrefix := ""
		if len(mods) > 0 {
			modPrefix = strings.Join(mods, " ") + " "
		}
		var params []string
		for _, p := range d.Params {
			prefix := ""
			if p.ByRef {
				prefix = "byref "
			}
			params = append(params, fmt.Sprintf("%s%s as %s", prefix, p.Name, p.Type.String()))
		}
		ret := ""
		if d.ReturnType != nil {
			ret = " as " + d.ReturnType.String()
		}
		errSuffix := ""
		if d.CanError {
			errSuffix = " can error"
		}
		fmt.Fprintf(sb, "%s%sFnDecl(%s(%s)%s%s)\n", indent, modPrefix, d.Name, strings.Join(params, ", "), ret, errSuffix)
		for _, st := range d.Body {
			dumpStmt(sb, st, indent+"  ")
		}
	}
}

func dumpStmt(sb *strings.Builder, stmt Statement, indent string) {
	switch s := stmt.(type) {
	case *VarDecl:
		kw := "dim"
		if s.IsConst {
			kw = "const"
		}
		typeStr := ""
		if s.Type != nil {
			typeStr = " as " + s.Type.String()
		}
		fmt.Fprintf(sb, "%sVarDecl(%s %s%s = %s)\n", indent, kw, s.Name, typeStr, FormatExpr(s.Value))

	case *Assignment:
		fmt.Fprintf(sb, "%sAssignment(%s = %s)\n", indent, FormatExpr(s.Target), FormatExpr(s.Value))

	case *ExprStmt:
		fmt.Fprintf(sb, "%sExprStmt(%s)\n", indent, FormatExpr(s.Expr))

	case *IfStmt:
		fmt.Fprintf(sb, "%sIf(%s)\n", indent, FormatExpr(s.Cond))
		for _, st := range s.ThenBody {
			dumpStmt(sb, st, indent+"  ")
		}
		for _, elif := range s.ElseIfs {
			fmt.Fprintf(sb, "%sElseIf(%s)\n", indent, FormatExpr(elif.Cond))
			for _, st := range elif.Body {
				dumpStmt(sb, st, indent+"  ")
			}
		}
		if s.HasElse {
			fmt.Fprintf(sb, "%sElse\n", indent)
			for _, st := range s.ElseBody {
				dumpStmt(sb, st, indent+"  ")
			}
		}

	case *ForStmt:
		if s.IsRange {
			stepStr := ""
			if s.Step != nil {
				stepStr = " step " + FormatExpr(s.Step)
			}
			fmt.Fprintf(sb, "%sForRange(%s = %s to %s%s)\n", indent, s.VarName, FormatExpr(s.Start), FormatExpr(s.End), stepStr)
		} else {
			fmt.Fprintf(sb, "%sForIn(%s in %s)\n", indent, strings.Join(s.Vars, ", "), FormatExpr(s.Iterable))
		}
		for _, st := range s.Body {
			dumpStmt(sb, st, indent+"  ")
		}

	case *WhileStmt:
		fmt.Fprintf(sb, "%sWhile(%s)\n", indent, FormatExpr(s.Cond))
		for _, st := range s.Body {
			dumpStmt(sb, st, indent+"  ")
		}

	case *SelectStmt:
		fmt.Fprintf(sb, "%sSelectCase(%s)\n", indent, FormatExpr(s.Target))
		for _, c := range s.Cases {
			var vals []string
			for _, v := range c.Values {
				vals = append(vals, FormatExpr(v))
			}
			bind := ""
			if c.Binding != "" {
				bind = " as " + c.Binding
			}
			fmt.Fprintf(sb, "%s  Case(%s%s)\n", indent, strings.Join(vals, ", "), bind)
			for _, st := range c.Body {
				dumpStmt(sb, st, indent+"    ")
			}
		}
		if s.HasElse {
			fmt.Fprintf(sb, "%s  CaseElse\n", indent)
			for _, st := range s.ElseBody {
				dumpStmt(sb, st, indent+"    ")
			}
		}

	case *TryCatchStmt:
		fmt.Fprintf(sb, "%sTry\n", indent)
		for _, st := range s.TryBody {
			dumpStmt(sb, st, indent+"  ")
		}
		fmt.Fprintf(sb, "%sCatch(%s)\n", indent, s.CatchVar)
		for _, st := range s.CatchBody {
			dumpStmt(sb, st, indent+"  ")
		}

	case *DeferStmt:
		fmt.Fprintf(sb, "%sDefer\n", indent)
		for _, st := range s.Body {
			dumpStmt(sb, st, indent+"  ")
		}

	case *ReturnStmt:
		if s.Value != nil {
			fmt.Fprintf(sb, "%sReturn(%s)\n", indent, FormatExpr(s.Value))
		} else {
			fmt.Fprintf(sb, "%sReturn()\n", indent)
		}

	case *BreakStmt:
		fmt.Fprintf(sb, "%sBreak\n", indent)

	case *ContinueStmt:
		fmt.Fprintf(sb, "%sContinue\n", indent)
	}
}

// FormatExpr returns a concise single-line string representation of an expression.
func FormatExpr(expr Expr) string {
	if expr == nil {
		return "<nil>"
	}
	switch e := expr.(type) {
	case *IdentExpr:
		return e.Name
	case *IntLitExpr:
		return e.Raw
	case *FloatLitExpr:
		return e.Raw
	case *StringLitExpr:
		return fmt.Sprintf("%q", e.Value)
	case *BoolLitExpr:
		if e.Value {
			return "true"
		}
		return "false"
	case *ListLitExpr:
		var parts []string
		for _, el := range e.Elements {
			parts = append(parts, FormatExpr(el))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *DictLitExpr:
		var parts []string
		for _, ent := range e.Entries {
			parts = append(parts, fmt.Sprintf("%s: %s", FormatExpr(ent.Key), FormatExpr(ent.Value)))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case *TypedEmptyCtorExpr:
		return e.Type.String() + "()"
	case *UnaryExpr:
		return fmt.Sprintf("(%s %s)", e.Op, FormatExpr(e.Operand))
	case *BinaryExpr:
		return fmt.Sprintf("(%s %s %s)", FormatExpr(e.Left), e.Op, FormatExpr(e.Right))
	case *FieldAccessExpr:
		return fmt.Sprintf("%s.%s", FormatExpr(e.Target), e.Field)
	case *IndexExpr:
		return fmt.Sprintf("%s[%s]", FormatExpr(e.Target), FormatExpr(e.Index))
	case *CallExpr:
		var args []string
		for _, a := range e.Args {
			if a.Name != "" {
				args = append(args, fmt.Sprintf("%s=%s", a.Name, FormatExpr(a.Value)))
			} else {
				args = append(args, FormatExpr(a.Value))
			}
		}
		return fmt.Sprintf("%s(%s)", FormatExpr(e.Callee), strings.Join(args, ", "))
	default:
		return "<unknown-expr>"
	}
}
