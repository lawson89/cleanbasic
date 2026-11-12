package codegen

import (
	"fmt"
	"strconv"
	"strings"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/sema"
	"cleanbasic/bootstrap/internal/token"
)

// EmitGo emits a single-file procedural Go program from an AnalyzedProgram.
// Phase 1 provides basic procedural emission sufficient for the unskipped
// bootstrap runner tests (01_hello_and_math.cb and 02_fibonacci.cb),
// while full semantic/codegen support is completed in Phases 2-3.
func EmitGo(prog *sema.AnalyzedProgram) (string, error) {
	if prog == nil || prog.Main == nil {
		return "", fmt.Errorf("nil program")
	}

	e := &emitter{
		usedFmt: false,
	}
	bodyCode, err := e.emitProgram(prog.Main)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	out.WriteString("package main\n\n")
	if e.usedFmt {
		out.WriteString("import \"fmt\"\n")
	}
	out.WriteString("import \"os\"\n\n")
	out.WriteString(bodyCode)
	return out.String(), nil
}

type emitter struct {
	usedFmt bool
}

func (e *emitter) emitProgram(prog *ast.Program) (string, error) {
	var sb strings.Builder

	for _, decl := range prog.Decls {
		switch d := decl.(type) {
		case *ast.ConstDecl:
			sb.WriteString(fmt.Sprintf("const %s = %s\n\n", d.Name, e.emitExpr(d.Value)))
		case *ast.FnDecl:
			fnCode, err := e.emitFn(d)
			if err != nil {
				return "", err
			}
			sb.WriteString(fnCode)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("func main() {\n\tos.Exit(int(cb_main()))\n}\n")
	return sb.String(), nil
}

func (e *emitter) emitFn(fn *ast.FnDecl) (string, error) {
	var sb strings.Builder
	name := fn.Name
	if name == "main" {
		name = "cb_main"
	}

	var params []string
	for _, p := range fn.Params {
		params = append(params, fmt.Sprintf("%s %s", p.Name, e.goType(p.Type)))
	}

	retType := ""
	if fn.ReturnType != nil {
		retType = " " + e.goType(*fn.ReturnType)
	}

	sb.WriteString(fmt.Sprintf("func %s(%s)%s {\n", name, strings.Join(params, ", "), retType))
	for _, st := range fn.Body {
		e.emitStmt(&sb, st, "\t")
	}
	sb.WriteString("}\n")
	return sb.String(), nil
}

func (e *emitter) goType(t ast.TypeExpr) string {
	switch t.Kind {
	case ast.TypeBasic:
		switch t.Name {
		case "int":
			return "int64"
		case "float":
			return "float64"
		case "str":
			return "string"
		case "bool":
			return "bool"
		}
	}
	return "any"
}

func (e *emitter) emitStmt(sb *strings.Builder, stmt ast.Statement, indent string) {
	switch s := stmt.(type) {
	case *ast.VarDecl:
		sb.WriteString(fmt.Sprintf("%svar %s = %s\n", indent, s.Name, e.emitExpr(s.Value)))
		sb.WriteString(fmt.Sprintf("%s_ = %s\n", indent, s.Name))
	case *ast.Assignment:
		sb.WriteString(fmt.Sprintf("%s%s = %s\n", indent, e.emitExpr(s.Target), e.emitExpr(s.Value)))
	case *ast.ExprStmt:
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, e.emitExpr(s.Expr)))
	case *ast.IfStmt:
		sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, e.emitExpr(s.Cond)))
		for _, st := range s.ThenBody {
			e.emitStmt(sb, st, indent+"\t")
		}
		for _, elif := range s.ElseIfs {
			sb.WriteString(fmt.Sprintf("%s} else if %s {\n", indent, e.emitExpr(elif.Cond)))
			for _, st := range elif.Body {
				e.emitStmt(sb, st, indent+"\t")
			}
		}
		if s.HasElse {
			sb.WriteString(fmt.Sprintf("%s} else {\n", indent))
			for _, st := range s.ElseBody {
				e.emitStmt(sb, st, indent+"\t")
			}
		}
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	case *ast.ForStmt:
		if s.IsRange {
			step := "int64(1)"
			if s.Step != nil {
				step = e.emitExpr(s.Step)
			}
			sb.WriteString(fmt.Sprintf("%sfor %s := %s; %s <= %s; %s += %s {\n",
				indent, s.VarName, e.emitExpr(s.Start), s.VarName, e.emitExpr(s.End), s.VarName, step))
			for _, st := range s.Body {
				e.emitStmt(sb, st, indent+"\t")
			}
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		}
	case *ast.WhileStmt:
		sb.WriteString(fmt.Sprintf("%sfor %s {\n", indent, e.emitExpr(s.Cond)))
		for _, st := range s.Body {
			e.emitStmt(sb, st, indent+"\t")
		}
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	case *ast.ReturnStmt:
		if s.Value != nil {
			sb.WriteString(fmt.Sprintf("%sreturn %s\n", indent, e.emitExpr(s.Value)))
		} else {
			sb.WriteString(fmt.Sprintf("%sreturn\n", indent))
		}
	case *ast.BreakStmt:
		sb.WriteString(fmt.Sprintf("%sbreak\n", indent))
	case *ast.ContinueStmt:
		sb.WriteString(fmt.Sprintf("%scontinue\n", indent))
	}
}

func (e *emitter) emitExpr(expr ast.Expr) string {
	switch ex := expr.(type) {
	case *ast.IdentExpr:
		return ex.Name
	case *ast.IntLitExpr:
		return fmt.Sprintf("int64(%s)", ex.Raw)
	case *ast.FloatLitExpr:
		return fmt.Sprintf("float64(%s)", ex.Raw)
	case *ast.StringLitExpr:
		return strconv.Quote(ex.Value)
	case *ast.BoolLitExpr:
		if ex.Value {
			return "true"
		}
		return "false"
	case *ast.UnaryExpr:
		op := string(ex.Op)
		if ex.Op == token.NOT {
			op = "!"
		}
		return fmt.Sprintf("(%s%s)", op, e.emitExpr(ex.Operand))
	case *ast.BinaryExpr:
		if ex.Op == token.AMP {
			e.usedFmt = true
			return fmt.Sprintf("(fmt.Sprint(%s) + fmt.Sprint(%s))", e.emitExpr(ex.Left), e.emitExpr(ex.Right))
		}
		op := string(ex.Op)
		if ex.Op == token.AND {
			op = "&&"
		} else if ex.Op == token.OR {
			op = "||"
		}
		return fmt.Sprintf("(%s %s %s)", e.emitExpr(ex.Left), op, e.emitExpr(ex.Right))
	case *ast.CallExpr:
		if id, ok := ex.Callee.(*ast.IdentExpr); ok && id.Name == "print" {
			e.usedFmt = true
			var parts []string
			for _, a := range ex.Args {
				parts = append(parts, fmt.Sprintf("fmt.Sprint(%s)", e.emitExpr(a.Value)))
			}
			if len(parts) == 0 {
				return "fmt.Println()"
			}
			return fmt.Sprintf("fmt.Println(%s)", strings.Join(parts, " + "))
		}
		var args []string
		for _, a := range ex.Args {
			args = append(args, e.emitExpr(a.Value))
		}
		return fmt.Sprintf("%s(%s)", e.emitExpr(ex.Callee), strings.Join(args, ", "))
	default:
		return "nil"
	}
}
