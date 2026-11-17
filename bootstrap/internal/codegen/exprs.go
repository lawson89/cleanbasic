package codegen

import (
	"fmt"
	"strconv"
	"strings"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/sema"
	"cleanbasic/bootstrap/internal/token"
)

func (e *emitter) emitBuiltinCall(ex *ast.CallExpr, ci *sema.CallInfo) string {
	switch ci.MethodName {
	case "list.join":
		e.useImport("strings")
		return fmt.Sprintf("strings.Join(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
	case "int.toChar":
		return fmt.Sprintf("string(rune(%s))", e.emitExpr(ci.Receiver))
	case "str.quote":
		e.useImport("strconv")
		return fmt.Sprintf("strconv.Quote(%s)", e.emitExpr(ci.Receiver))
	case "list.len", "dict.len", "str.len", "mailbox.len":
		return fmt.Sprintf("int64(len(%s))", e.emitExpr(ci.Receiver))
	case "list.push":
		recv := e.emitExpr(ci.Receiver)
		return fmt.Sprintf("%s = append(%s, %s)", recv, recv, e.emitExpr(ex.Args[0]))
	case "dict.get":
		e.usedDictGet = true
		return fmt.Sprintf("cbDictGet(%s, %s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]), e.emitExpr(ex.Args[1]))
	case "dict.contains":
		e.usedDictContains = true
		return fmt.Sprintf("cbDictContains(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
	case "dict.remove":
		return fmt.Sprintf("delete(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
	case "str.trim":
		e.useImport("strings")
		return fmt.Sprintf("strings.TrimSpace(%s)", e.emitExpr(ci.Receiver))
	case "str.toUpper":
		e.useImport("strings")
		return fmt.Sprintf("strings.ToUpper(%s)", e.emitExpr(ci.Receiver))
	case "str.toLower":
		e.useImport("strings")
		return fmt.Sprintf("strings.ToLower(%s)", e.emitExpr(ci.Receiver))
	case "str.toStr":
		return e.emitExpr(ci.Receiver)
	case "str.toInt":
		e.useImport("strconv")
		return fmt.Sprintf("strconv.ParseInt(%s, 10, 64)", e.emitExpr(ci.Receiver))
	case "str.toFloat":
		e.useImport("strconv")
		return fmt.Sprintf("strconv.ParseFloat(%s, 64)", e.emitExpr(ci.Receiver))
	case "str.split":
		e.useImport("strings")
		return fmt.Sprintf("strings.Split(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
	case "str.contains":
		e.useImport("strings")
		return fmt.Sprintf("strings.Contains(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
	case "str.startsWith":
		e.useImport("strings")
		return fmt.Sprintf("strings.HasPrefix(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
	case "str.endsWith":
		e.useImport("strings")
		return fmt.Sprintf("strings.HasSuffix(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
	case "str.indexOf":
		e.useImport("strings")
		return fmt.Sprintf("int64(strings.Index(%s, %s))", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
	case "str.replace":
		e.useImport("strings")
		return fmt.Sprintf("strings.ReplaceAll(%s, %s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]), e.emitExpr(ex.Args[1]))
	case "int.toByte":
		return fmt.Sprintf("byte(%s)", e.emitExpr(ci.Receiver))
	case "byte.toInt":
		return fmt.Sprintf("int64(%s)", e.emitExpr(ci.Receiver))
	case "mailbox.post":
		return fmt.Sprintf("%s <- %s", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
	case "mailbox.get":
		return fmt.Sprintf("(<-%s)", e.emitExpr(ci.Receiver))
	case "mailbox.close":
		return fmt.Sprintf("close(%s)", e.emitExpr(ci.Receiver))
	case "int.toStr":
		e.useImport("strconv")
		return fmt.Sprintf("strconv.FormatInt(%s, 10)", e.emitExpr(ci.Receiver))
	case "float.toStr":
		e.useImport("strconv")
		return fmt.Sprintf("strconv.FormatFloat(%s, 'g', -1, 64)", e.emitExpr(ci.Receiver))
	case "bool.toStr":
		e.useImport("strconv")
		return fmt.Sprintf("strconv.FormatBool(%s)", e.emitExpr(ci.Receiver))
	case "byte.toStr":
		e.useImport("strconv")
		return fmt.Sprintf("strconv.FormatInt(int64(%s), 10)", e.emitExpr(ci.Receiver))
	default:
		return ""
	}
}

func (e *emitter) emitExpr(expr ast.Expr) string {
	switch ex := expr.(type) {
	case *ast.IdentExpr:
		if ex.GoName != "" {
			return goIdent(ex.GoName)
		}
		if e.byRefVars != nil && e.byRefVars[ex.Name] {
			return fmt.Sprintf("(*%s)", goIdent(ex.Name))
		}
		return goIdent(ex.Name)

	case *ast.IntLitExpr:
		return fmt.Sprintf("int64(%s)", ex.Raw)

	case *ast.FloatLitExpr:
		return fmt.Sprintf("float64(%s)", ex.Raw)

	case *ast.StringLitExpr:
		return strconv.Quote(ex.Value)

	case *ast.ByteLitExpr:
		return fmt.Sprintf("byte(%d)", ex.Value)

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
		op := string(ex.Op)
		if ex.Op == token.AND {
			op = "&&"
		} else if ex.Op == token.OR {
			op = "||"
		}
		return fmt.Sprintf("(%s %s %s)", e.emitExpr(ex.Left), op, e.emitExpr(ex.Right))

	case *ast.ListLitExpr:
		t := ex.ExprType()
		goT := e.goSemaType(t)
		var elems []string
		for _, el := range ex.Elements {
			elems = append(elems, e.emitExpr(el))
		}
		return fmt.Sprintf("%s{%s}", goT, strings.Join(elems, ", "))

	case *ast.DictLitExpr:
		t := ex.ExprType()
		goT := e.goSemaType(t)
		var entries []string
		for _, ent := range ex.Entries {
			entries = append(entries, fmt.Sprintf("%s: %s", e.emitExpr(ent.Key), e.emitExpr(ent.Value)))
		}
		return fmt.Sprintf("%s{%s}", goT, strings.Join(entries, ", "))

	case *ast.TypedEmptyCtorExpr:
		t := ex.ExprType()
		if t.Kind == sema.KindMailbox {
			elemGo := e.goSemaType(t.Elem())
			if len(ex.Args) == 1 {
				return fmt.Sprintf("make(chan %s, %s)", elemGo, e.emitExpr(ex.Args[0]))
			}
			return fmt.Sprintf("make(chan %s)", elemGo)
		}
		return fmt.Sprintf("%s{}", e.goSemaType(t))

	case *ast.FieldAccessExpr:
		if ex.GoName != "" {
			return goIdent(ex.GoName)
		}
		targetType := ex.Target.ExprType()
		if targetType.Kind == sema.KindErr && ex.Field == "msg" {
			return fmt.Sprintf("%s.Error()", e.emitExpr(ex.Target))
		}
		if targetType.Kind == sema.KindStruct {
			return fmt.Sprintf("%s.%s", e.emitExpr(ex.Target), goStructFieldName(ex.Field))
		}
		return fmt.Sprintf("%s.%s", e.emitExpr(ex.Target), ex.Field)

	case *ast.IndexExpr:
		return fmt.Sprintf("%s[%s]", e.emitExpr(ex.Target), e.emitExpr(ex.Index))

	case *ast.SliceExpr:
		startStr := ""
		if ex.Start != nil {
			startStr = e.emitExpr(ex.Start)
		}
		endStr := ""
		if ex.End != nil {
			endStr = e.emitExpr(ex.End)
		}
		return fmt.Sprintf("%s[%s:%s]", e.emitExpr(ex.Target), startStr, endStr)

	case *ast.CallExpr:
		return e.emitCallExprRaw(ex)

	default:
		return "nil"
	}
}

func (e *emitter) emitArgExpr(expr ast.Expr, byRef bool) string {
	if byRef {
		if id, ok := expr.(*ast.IdentExpr); ok {
			if e.byRefVars != nil && e.byRefVars[id.Name] {
				return goIdent(id.Name)
			}
			return "&" + goIdent(id.Name)
		}
		return "&" + e.emitExpr(expr)
	}
	return e.emitExpr(expr)
}

func (e *emitter) fnGoName(fn *sema.FnSymbol) string {
	name := fn.GoName
	if name == "" {
		name = fn.Name
	}
	if name == "main" {
		return "cb_main"
	}
	return goIdent(name)
}

func (e *emitter) effectiveCallArgs(ex *ast.CallExpr, ci *sema.CallInfo) []ast.Expr {
	if ci.Kind == sema.CallUFCS && ci.Receiver != nil {
		return append([]ast.Expr{ci.Receiver}, ex.Args...)
	}
	return ex.Args
}

func (e *emitter) emitCallExprRaw(ex *ast.CallExpr) string {
	if id, ok := ex.Callee.(*ast.IdentExpr); ok && id.Name == "print" {
		e.useImport("fmt")
		return fmt.Sprintf("fmt.Println(%s)", e.emitExpr(ex.Args[0]))
	}

	if ci := ex.CallInfo; ci != nil {
		switch ci.Kind {
		case sema.CallStructCtor:
			var fields []string
			for _, a := range ex.Args {
				fields = append(fields, e.emitExpr(a))
			}
			structGoName := ci.Struct.Name
			if ci.Struct.GoName != "" {
				structGoName = ci.Struct.GoName
			}
			if !ci.Struct.IsGoType {
				structGoName = goIdent(structGoName)
			}
			return fmt.Sprintf("%s{%s}", structGoName, strings.Join(fields, ", "))

		case sema.CallMethod:
			return e.emitBuiltinCall(ex, ci)

		case sema.CallFunc, sema.CallUFCS:
			if ci.Fn != nil {
				rawArgs := e.effectiveCallArgs(ex, ci)
				args := make([]string, 0, len(rawArgs))
				for i, argExpr := range rawArgs {
					byRef := i < len(ci.Fn.Params) && ci.Fn.Params[i].ByRef
					args = append(args, e.emitArgExpr(argExpr, byRef))
				}
				return fmt.Sprintf("%s(%s)", e.fnGoName(ci.Fn), strings.Join(args, ", "))
			}
		}
	}

	return ""
}
