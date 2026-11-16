package codegen

import (
	"fmt"
	"strconv"
	"strings"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/sema"
	"cleanbasic/bootstrap/internal/token"
)

type builtinCodegenSpec struct {
	Imports []string
	Setup   func(e *emitter)
	Emit    func(e *emitter, ex *ast.CallExpr, ci *sema.CallInfo) string
}

var builtinCodegenTable map[string]builtinCodegenSpec

func init() {
	builtinCodegenTable = map[string]builtinCodegenSpec{
		"str.join": {
			Imports: []string{"strings"},
			Emit: func(e *emitter, ex *ast.CallExpr, _ *sema.CallInfo) string {
				return fmt.Sprintf("strings.Join(%s, %s)", e.emitExpr(ex.Args[0]), e.emitExpr(ex.Args[1]))
			},
		},
		"str.fromCharCode": {
			Emit: func(e *emitter, ex *ast.CallExpr, _ *sema.CallInfo) string {
				return fmt.Sprintf("string(rune(%s))", e.emitExpr(ex.Args[0]))
			},
		},
		"str.quote": {
			Imports: []string{"strconv"},
			Emit: func(e *emitter, ex *ast.CallExpr, _ *sema.CallInfo) string {
				return fmt.Sprintf("strconv.Quote(%s)", e.emitExpr(ex.Args[0]))
			},
		},
		"str.fromBytes": {
			Emit: func(e *emitter, ex *ast.CallExpr, _ *sema.CallInfo) string {
				return fmt.Sprintf("string(%s)", e.emitExpr(ex.Args[0]))
			},
		},
		"list.len": {Emit: emitBuiltinLen},
		"dict.len": {Emit: emitBuiltinLen},
		"str.len":  {Emit: emitBuiltinLen},
		"list.push": {
			Emit: func(e *emitter, ex *ast.CallExpr, ci *sema.CallInfo) string {
				recv := e.emitExpr(ci.Receiver)
				return fmt.Sprintf("%s = append(%s, %s)", recv, recv, e.emitExpr(ex.Args[0]))
			},
		},
		"dict.get": {
			Setup: func(e *emitter) { e.usedDictGet = true },
			Emit: func(e *emitter, ex *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("cbDictGet(%s, %s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]), e.emitExpr(ex.Args[1]))
			},
		},
		"dict.contains": {
			Setup: func(e *emitter) { e.usedDictContains = true },
			Emit: func(e *emitter, ex *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("cbDictContains(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
			},
		},
		"dict.remove": {
			Emit: func(e *emitter, ex *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("delete(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
			},
		},
		"str.trim": {
			Imports: []string{"strings"},
			Emit: func(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("strings.TrimSpace(%s)", e.emitExpr(ci.Receiver))
			},
		},
		"str.toUpper": {
			Imports: []string{"strings"},
			Emit: func(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("strings.ToUpper(%s)", e.emitExpr(ci.Receiver))
			},
		},
		"str.toLower": {
			Imports: []string{"strings"},
			Emit: func(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("strings.ToLower(%s)", e.emitExpr(ci.Receiver))
			},
		},
		"str.toStr": {
			Emit: func(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
				return e.emitExpr(ci.Receiver)
			},
		},
		"str.toInt": {
			Setup: func(e *emitter) { e.usedStrToInt = true },
			Emit: func(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("cbStrToInt(%s)", e.emitExpr(ci.Receiver))
			},
		},
		"str.toBytes": {
			Emit: func(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("[]byte(%s)", e.emitExpr(ci.Receiver))
			},
		},
		"str.split": {
			Imports: []string{"strings"},
			Emit: func(e *emitter, ex *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("strings.Split(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
			},
		},
		"str.contains": {
			Imports: []string{"strings"},
			Emit: func(e *emitter, ex *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("strings.Contains(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
			},
		},
		"str.startsWith": {
			Imports: []string{"strings"},
			Emit: func(e *emitter, ex *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("strings.HasPrefix(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
			},
		},
		"str.endsWith": {
			Imports: []string{"strings"},
			Emit: func(e *emitter, ex *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("strings.HasSuffix(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
			},
		},
		"str.indexOf": {
			Imports: []string{"strings"},
			Emit: func(e *emitter, ex *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("int64(strings.Index(%s, %s))", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
			},
		},
		"str.replace": {
			Imports: []string{"strings"},
			Emit: func(e *emitter, ex *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("strings.ReplaceAll(%s, %s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]), e.emitExpr(ex.Args[1]))
			},
		},
		"int.toByte": {
			Emit: func(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("byte(%s)", e.emitExpr(ci.Receiver))
			},
		},
		"byte.toInt": {
			Emit: func(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("int64(%s)", e.emitExpr(ci.Receiver))
			},
		},
		"mailbox.post": {
			Emit: func(e *emitter, ex *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("%s <- %s", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0]))
			},
		},
		"mailbox.get": {
			Emit: func(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("(<-%s)", e.emitExpr(ci.Receiver))
			},
		},
		"mailbox.close": {
			Emit: func(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("close(%s)", e.emitExpr(ci.Receiver))
			},
		},
		"mailbox.count": {Emit: emitBuiltinLen},
		"int.toStr": {
			Imports: []string{"strconv"},
			Emit: func(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("strconv.FormatInt(%s, 10)", e.emitExpr(ci.Receiver))
			},
		},
		"float.toStr": {
			Imports: []string{"strconv"},
			Emit: func(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("strconv.FormatFloat(%s, 'g', -1, 64)", e.emitExpr(ci.Receiver))
			},
		},
		"bool.toStr": {
			Imports: []string{"strconv"},
			Emit: func(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("strconv.FormatBool(%s)", e.emitExpr(ci.Receiver))
			},
		},
		"byte.toStr": {
			Imports: []string{"strconv"},
			Emit: func(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
				return fmt.Sprintf("strconv.FormatInt(int64(%s), 10)", e.emitExpr(ci.Receiver))
			},
		},
	}
}

func emitBuiltinLen(e *emitter, _ *ast.CallExpr, ci *sema.CallInfo) string {
	return fmt.Sprintf("int64(len(%s))", e.emitExpr(ci.Receiver))
}

func (e *emitter) emitExpr(expr ast.Expr) string {
	switch ex := expr.(type) {
	case *ast.IdentExpr:
		if e.prog != nil {
			if goName, ok := e.prog.IdentGoNames[ex]; ok {
				return goIdent(goName)
			}
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
		t := e.prog.ExprTypes[ex]
		goT := e.goSemaType(t)
		var elems []string
		for _, el := range ex.Elements {
			elems = append(elems, e.emitExpr(el))
		}
		return fmt.Sprintf("%s{%s}", goT, strings.Join(elems, ", "))

	case *ast.DictLitExpr:
		t := e.prog.ExprTypes[ex]
		goT := e.goSemaType(t)
		var entries []string
		for _, ent := range ex.Entries {
			entries = append(entries, fmt.Sprintf("%s: %s", e.emitExpr(ent.Key), e.emitExpr(ent.Value)))
		}
		return fmt.Sprintf("%s{%s}", goT, strings.Join(entries, ", "))

	case *ast.TypedEmptyCtorExpr:
		t := e.prog.ExprTypes[ex]
		if t.Kind == sema.KindMailbox {
			elemGo := e.goSemaType(t.Elem())
			if len(ex.Args) == 1 {
				return fmt.Sprintf("make(chan %s, %s)", elemGo, e.emitExpr(ex.Args[0]))
			}
			return fmt.Sprintf("make(chan %s)", elemGo)
		}
		return fmt.Sprintf("%s{}", e.goSemaType(t))

	case *ast.FieldAccessExpr:
		if e.prog != nil {
			if goName, ok := e.prog.IdentGoNames[ex]; ok {
				return goIdent(goName)
			}
			if targetType, ok := e.prog.ExprTypes[ex.Target]; ok {
				if targetType.Kind == sema.KindErr && ex.Field == "msg" {
					return fmt.Sprintf("%s.Error()", e.emitExpr(ex.Target))
				}
				if targetType.Kind == sema.KindStruct {
					return fmt.Sprintf("%s.%s", e.emitExpr(ex.Target), goStructFieldName(ex.Field))
				}
			}
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
		id := expr.(*ast.IdentExpr)
		if e.byRefVars != nil && e.byRefVars[id.Name] {
			return goIdent(id.Name)
		}
		return "&" + goIdent(id.Name)
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
		var parts []string
		for _, a := range ex.Args {
			parts = append(parts, fmt.Sprintf("fmt.Sprint(%s)", e.emitExpr(a)))
		}
		if len(parts) == 0 {
			return "fmt.Println()"
		}
		return fmt.Sprintf("fmt.Println(%s)", strings.Join(parts, " + "))
	}

	if e.prog != nil {
		if ci, ok := e.prog.CallInfos[ex]; ok {
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
				if spec, found := builtinCodegenTable[ci.MethodName]; found {
					for _, imp := range spec.Imports {
						e.useImport(imp)
					}
					if spec.Setup != nil {
						spec.Setup(e)
					}
					return spec.Emit(e, ex, ci)
				}

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
	}

	return ""
}
