package codegen

import (
	"fmt"
	"strconv"
	"strings"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/sema"
	"cleanbasic/bootstrap/internal/token"
)

// EmitGo emits a single-file procedural Go program from a semantically analyzed TypedProgram.
func EmitGo(prog *sema.TypedProgram) (string, error) {
	if prog == nil || prog.Main == nil {
		return "", fmt.Errorf("nil program")
	}

	e := &emitter{
		prog: prog,
	}
	bodyCode, err := e.emitProgram(prog.Main)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	out.WriteString("package main\n\n")
	if e.usedFmt && e.usedOs {
		out.WriteString(fmt.Sprintf("import (\n\t%q\n\t%q\n)\n\n", "fmt", "os"))
	} else if e.usedFmt {
		out.WriteString(fmt.Sprintf("import %q\n\n", "fmt"))
	} else if e.usedOs {
		out.WriteString(fmt.Sprintf("import %q\n\n", "os"))
	}
	out.WriteString(bodyCode)
	return out.String(), nil
}

type emitter struct {
	prog      *sema.TypedProgram
	usedFmt   bool
	usedOs    bool
	byRefVars map[string]bool
}

func (e *emitter) emitProgram(prog *ast.Program) (string, error) {
	var sb strings.Builder

	for _, decl := range prog.Decls {
		switch d := decl.(type) {
		case *ast.ConstDecl:
			typeStr := ""
			if cType, ok := e.prog.ConstTypes[d]; ok && !cType.IsVoid() {
				typeStr = " " + e.goSemaType(cType)
			} else if d.Type != nil {
				typeStr = " " + e.goType(*d.Type)
			}
			sb.WriteString(fmt.Sprintf("var %s%s = %s\n\n", d.Name, typeStr, e.emitExpr(d.Value)))

		case *ast.FnDecl:
			if d.Extern {
				continue
			}
			fnCode, err := e.emitFn(d)
			if err != nil {
				return "", err
			}
			sb.WriteString(fnCode)
			sb.WriteString("\n")
		}
	}

	if mainSym, ok := e.prog.Functions["main"]; ok {
		if mainSym.ReturnType.Equals(sema.IntType) {
			e.usedOs = true
			sb.WriteString("func main() {\n\tif code := cb_main(); code != 0 {\n\t\tos.Exit(int(code))\n\t}\n}\n")
		} else {
			sb.WriteString("func main() {\n\tcb_main()\n}\n")
		}
	}

	return sb.String(), nil
}

func (e *emitter) emitFn(fn *ast.FnDecl) (string, error) {
	var sb strings.Builder
	name := fn.Name
	if name == "main" {
		name = "cb_main"
	}

	prevByRef := e.byRefVars
	e.byRefVars = make(map[string]bool)
	defer func() {
		e.byRefVars = prevByRef
	}()

	var params []string
	for _, p := range fn.Params {
		paramGoType := e.goType(p.Type)
		if p.ByRef {
			e.byRefVars[p.Name] = true
			paramGoType = "*" + paramGoType
		}
		params = append(params, fmt.Sprintf("%s %s", p.Name, paramGoType))
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

func (e *emitter) goSemaType(t sema.Type) string {
	switch t.Kind {
	case sema.KindInt:
		return "int64"
	case sema.KindFloat:
		return "float64"
	case sema.KindStr:
		return "string"
	case sema.KindBool:
		return "bool"
	default:
		return "any"
	}
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
		typeStr := ""
		if vType, ok := e.prog.VarTypes[s]; ok && !vType.IsVoid() {
			typeStr = " " + e.goSemaType(vType)
		} else if s.Type != nil {
			typeStr = " " + e.goType(*s.Type)
		}
		sb.WriteString(fmt.Sprintf("%svar %s%s = %s\n", indent, s.Name, typeStr, e.emitExpr(s.Value)))
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
			if s.Step == nil {
				sb.WriteString(fmt.Sprintf("%sfor %s := %s; %s <= %s; %s += int64(1) {\n",
					indent, s.VarName, e.emitExpr(s.Start), s.VarName, e.emitExpr(s.End), s.VarName))
				sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, s.VarName))
				for _, st := range s.Body {
					e.emitStmt(sb, st, indent+"\t")
				}
				sb.WriteString(fmt.Sprintf("%s}\n", indent))
			} else {
				stepExpr := e.emitExpr(s.Step)
				sb.WriteString(fmt.Sprintf("%sfor %s := %s; (%s >= 0 && %s <= %s) || (%s < 0 && %s >= %s); %s += %s {\n",
					indent, s.VarName, e.emitExpr(s.Start),
					stepExpr, s.VarName, e.emitExpr(s.End),
					stepExpr, s.VarName, e.emitExpr(s.End),
					s.VarName, stepExpr))
				sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, s.VarName))
				for _, st := range s.Body {
					e.emitStmt(sb, st, indent+"\t")
				}
				sb.WriteString(fmt.Sprintf("%s}\n", indent))
			}
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
		if e.byRefVars != nil && e.byRefVars[ex.Name] {
			return fmt.Sprintf("(*%s)", ex.Name)
		}
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

		var fnSym *sema.FnSymbol
		if id, ok := ex.Callee.(*ast.IdentExpr); ok && e.prog != nil {
			fnSym = e.prog.Functions[id.Name]
		}

		var args []string
		for i, a := range ex.Args {
			if fnSym != nil && i < len(fnSym.Params) && fnSym.Params[i].ByRef {
				if argId, ok := a.Value.(*ast.IdentExpr); ok && e.byRefVars != nil && e.byRefVars[argId.Name] {
					args = append(args, argId.Name)
				} else {
					args = append(args, "&"+e.emitExpr(a.Value))
				}
			} else {
				args = append(args, e.emitExpr(a.Value))
			}
		}

		calleeName := e.emitExpr(ex.Callee)
		if calleeName == "main" {
			calleeName = "cb_main"
		}
		return fmt.Sprintf("%s(%s)", calleeName, strings.Join(args, ", "))

	default:
		return "nil"
	}
}
