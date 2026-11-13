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

	var helpers strings.Builder
	if e.usedListGet {
		helpers.WriteString("func cbListGet[T any](s []T, idx int64) T {\n\tvar zero T\n\tif idx < 0 || int(idx) >= len(s) {\n\t\treturn zero\n\t}\n\treturn s[idx]\n}\n\n")
	}
	if e.usedDictGet {
		helpers.WriteString("func cbDictGet[K comparable, V any](m map[K]V, k K, def V) V {\n\tif v, ok := m[k]; ok {\n\t\treturn v\n\t}\n\treturn def\n}\n\n")
	}
	if e.usedDictContains {
		helpers.WriteString("func cbDictContains[K comparable, V any](m map[K]V, k K) bool {\n\t_, ok := m[k]\n\treturn ok\n}\n\n")
	}
	if e.usedStrToInt {
		e.usedStrconv = true
		helpers.WriteString("func cbStrToInt(s string) int64 {\n\tv, _ := strconv.ParseInt(s, 10, 64)\n\treturn v\n}\n\n")
	}

	var imports []string
	if e.usedFmt {
		imports = append(imports, "fmt")
	}
	if e.usedOs {
		imports = append(imports, "os")
	}
	if e.usedStrconv {
		imports = append(imports, "strconv")
	}
	if e.usedStrings {
		imports = append(imports, "strings")
	}

	var out strings.Builder
	out.WriteString("package main\n\n")
	if len(imports) == 1 {
		out.WriteString(fmt.Sprintf("import %q\n\n", imports[0]))
	} else if len(imports) > 1 {
		out.WriteString("import (\n")
		for _, imp := range imports {
			out.WriteString(fmt.Sprintf("\t%q\n", imp))
		}
		out.WriteString(")\n\n")
	}
	out.WriteString(helpers.String())
	out.WriteString(bodyCode)
	return out.String(), nil
}

type emitter struct {
	prog             *sema.TypedProgram
	usedFmt          bool
	usedOs           bool
	usedStrings      bool
	usedStrconv      bool
	usedListGet      bool
	usedDictGet      bool
	usedDictContains bool
	usedStrToInt     bool
	byRefVars        map[string]bool
}

func (e *emitter) emitProgram(prog *ast.Program) (string, error) {
	var sb strings.Builder

	for _, decl := range prog.Decls {
		switch d := decl.(type) {
		case *ast.TypeDecl:
			sb.WriteString(fmt.Sprintf("type %s struct {\n", d.Name))
			for _, f := range d.Fields {
				sb.WriteString(fmt.Sprintf("\t%s %s\n", f.Name, e.goType(f.Type)))
			}
			sb.WriteString("}\n\n")

		case *ast.EnumDecl:
			sb.WriteString(fmt.Sprintf("type %s int64\n\nconst (\n", d.Name))
			for i, v := range d.Variants {
				if i == 0 {
					sb.WriteString(fmt.Sprintf("\t%s_%s %s = iota\n", d.Name, v, d.Name))
				} else {
					sb.WriteString(fmt.Sprintf("\t%s_%s\n", d.Name, v))
				}
			}
			sb.WriteString(")\n\n")

		case *ast.UnionDecl:
			sb.WriteString(fmt.Sprintf("type %s interface {\n\tis%s()\n}\n\n", d.Name, d.Name))
			for _, v := range d.Variants {
				vStruct := fmt.Sprintf("%s_%s", d.Name, v.Name)
				sb.WriteString(fmt.Sprintf("type %s struct {\n\tValue %s\n}\n", vStruct, e.goType(v.Type)))
				sb.WriteString(fmt.Sprintf("func (%s) is%s() {}\n\n", vStruct, d.Name))
			}

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
	case sema.KindStruct, sema.KindEnum, sema.KindUnion:
		return t.Name
	case sema.KindList:
		if t.ElemType != nil {
			return "[]" + e.goSemaType(*t.ElemType)
		}
		return "[]any"
	case sema.KindDict:
		if t.KeyType != nil && t.ValType != nil {
			return fmt.Sprintf("map[%s]%s", e.goSemaType(*t.KeyType), e.goSemaType(*t.ValType))
		}
		return "map[any]any"
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
	case ast.TypeList:
		if t.ElemType != nil {
			return "[]" + e.goType(*t.ElemType)
		}
		return "[]any"
	case ast.TypeDict:
		if t.KeyType != nil && t.ValType != nil {
			return fmt.Sprintf("map[%s]%s", e.goType(*t.KeyType), e.goType(*t.ValType))
		}
		return "map[any]any"
	case ast.TypeQualified:
		if len(t.Parts) == 1 {
			return t.Parts[0]
		}
		return strings.Join(t.Parts, ".")
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
		if idx, ok := s.Target.(*ast.IndexExpr); ok {
			sb.WriteString(fmt.Sprintf("%s%s[%s] = %s\n", indent, e.emitExpr(idx.Target), e.emitExpr(idx.Index), e.emitExpr(s.Value)))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s = %s\n", indent, e.emitExpr(s.Target), e.emitExpr(s.Value)))
		}

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
		} else {
			iterType := e.prog.ExprTypes[s.Iterable]
			if iterType.Kind == sema.KindList {
				sb.WriteString(fmt.Sprintf("%sfor _, %s := range %s {\n", indent, s.Vars[0], e.emitExpr(s.Iterable)))
				sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, s.Vars[0]))
			} else if iterType.Kind == sema.KindDict {
				if len(s.Vars) == 1 {
					sb.WriteString(fmt.Sprintf("%sfor %s := range %s {\n", indent, s.Vars[0], e.emitExpr(s.Iterable)))
					sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, s.Vars[0]))
				} else {
					sb.WriteString(fmt.Sprintf("%sfor %s, %s := range %s {\n", indent, s.Vars[0], s.Vars[1], e.emitExpr(s.Iterable)))
					sb.WriteString(fmt.Sprintf("%s\t_ = %s\n%s\t_ = %s\n", indent, s.Vars[0], indent, s.Vars[1]))
				}
			}
			for _, st := range s.Body {
				e.emitStmt(sb, st, indent+"\t")
			}
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		}

	case *ast.SelectStmt:
		if unionSym, isUnion := e.prog.SelectUnion[s]; isUnion {
			sb.WriteString(fmt.Sprintf("%sswitch cbSwitch := (%s).(type) {\n", indent, e.emitExpr(s.Target)))
			hasBinding := false
			for _, cc := range s.Cases {
				var caseTypes []string
				for _, val := range cc.Values {
					var vName string
					switch p := val.(type) {
					case *ast.IdentExpr:
						vName = p.Name
					case *ast.FieldAccessExpr:
						vName = p.Field
					}
					caseTypes = append(caseTypes, fmt.Sprintf("%s_%s", unionSym.Name, vName))
				}
				sb.WriteString(fmt.Sprintf("%scase %s:\n", indent, strings.Join(caseTypes, ", ")))
				if cc.Binding != "" {
					hasBinding = true
					sb.WriteString(fmt.Sprintf("%s\tvar %s = cbSwitch.Value\n", indent, cc.Binding))
					sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, cc.Binding))
				}
				for _, st := range cc.Body {
					e.emitStmt(sb, st, indent+"\t")
				}
			}
			if s.HasElse {
				sb.WriteString(fmt.Sprintf("%sdefault:\n", indent))
				for _, st := range s.ElseBody {
					e.emitStmt(sb, st, indent+"\t")
				}
			}
			if !hasBinding {
				// Avoid unused cbSwitch if no case binds a payload
				_ = hasBinding
			}
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		} else {
			sb.WriteString(fmt.Sprintf("%sswitch %s {\n", indent, e.emitExpr(s.Target)))
			for _, cc := range s.Cases {
				var vals []string
				for _, v := range cc.Values {
					vals = append(vals, e.emitExpr(v))
				}
				sb.WriteString(fmt.Sprintf("%scase %s:\n", indent, strings.Join(vals, ", ")))
				for _, st := range cc.Body {
					e.emitStmt(sb, st, indent+"\t")
				}
			}
			if s.HasElse {
				sb.WriteString(fmt.Sprintf("%sdefault:\n", indent))
				for _, st := range s.ElseBody {
					e.emitStmt(sb, st, indent+"\t")
				}
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
		return fmt.Sprintf("%s{}", e.goSemaType(t))

	case *ast.FieldAccessExpr:
		if id, ok := ex.Target.(*ast.IdentExpr); ok && e.prog != nil && e.prog.Enums[id.Name] != nil {
			return fmt.Sprintf("%s_%s", id.Name, ex.Field)
		}
		return fmt.Sprintf("%s.%s", e.emitExpr(ex.Target), ex.Field)

	case *ast.IndexExpr:
		targetType := e.prog.ExprTypes[ex.Target]
		if targetType.Kind == sema.KindList {
			e.usedListGet = true
			return fmt.Sprintf("cbListGet(%s, %s)", e.emitExpr(ex.Target), e.emitExpr(ex.Index))
		}
		return fmt.Sprintf("%s[%s]", e.emitExpr(ex.Target), e.emitExpr(ex.Index))

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

		if e.prog != nil {
			if ci, ok := e.prog.CallInfos[ex]; ok {
				switch ci.Kind {
				case sema.CallStructCtor:
					var fields []string
					if ci.NamedArgs {
						for _, a := range ex.Args {
							fields = append(fields, fmt.Sprintf("%s: %s", a.Name, e.emitExpr(a.Value)))
						}
					} else {
						for _, a := range ex.Args {
							fields = append(fields, e.emitExpr(a.Value))
						}
					}
					return fmt.Sprintf("%s{%s}", ci.Struct.Name, strings.Join(fields, ", "))

				case sema.CallUnionCtor:
					return fmt.Sprintf("%s_%s{Value: %s}", ci.Union.Name, ci.VariantName, e.emitExpr(ex.Args[0].Value))

				case sema.CallMethod:
					switch ci.MethodName {
					case "str.join":
						e.usedStrings = true
						return fmt.Sprintf("strings.Join(%s, %s)", e.emitExpr(ex.Args[0].Value), e.emitExpr(ex.Args[1].Value))
					case "list.len", "dict.len", "str.len":
						return fmt.Sprintf("int64(len(%s))", e.emitExpr(ci.Receiver))
					case "list.push":
						recv := e.emitExpr(ci.Receiver)
						return fmt.Sprintf("%s = append(%s, %s)", recv, recv, e.emitExpr(ex.Args[0].Value))
					case "dict.get":
						e.usedDictGet = true
						return fmt.Sprintf("cbDictGet(%s, %s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0].Value), e.emitExpr(ex.Args[1].Value))
					case "dict.contains":
						e.usedDictContains = true
						return fmt.Sprintf("cbDictContains(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0].Value))
					case "str.trim":
						e.usedStrings = true
						return fmt.Sprintf("strings.TrimSpace(%s)", e.emitExpr(ci.Receiver))
					case "str.toUpper":
						e.usedStrings = true
						return fmt.Sprintf("strings.ToUpper(%s)", e.emitExpr(ci.Receiver))
					case "str.toLower":
						e.usedStrings = true
						return fmt.Sprintf("strings.ToLower(%s)", e.emitExpr(ci.Receiver))
					case "str.toStr":
						return e.emitExpr(ci.Receiver)
					case "str.toInt":
						e.usedStrToInt = true
						return fmt.Sprintf("cbStrToInt(%s)", e.emitExpr(ci.Receiver))
					case "str.split":
						e.usedStrings = true
						return fmt.Sprintf("strings.Split(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0].Value))
					case "str.contains":
						e.usedStrings = true
						return fmt.Sprintf("strings.Contains(%s, %s)", e.emitExpr(ci.Receiver), e.emitExpr(ex.Args[0].Value))
					}

				case sema.CallUFCS:
					var args []string
					if ci.Fn.Params[0].ByRef {
						if recvId, ok := ci.Receiver.(*ast.IdentExpr); ok && e.byRefVars != nil && e.byRefVars[recvId.Name] {
							args = append(args, recvId.Name)
						} else {
							args = append(args, "&"+e.emitExpr(ci.Receiver))
						}
					} else {
						args = append(args, e.emitExpr(ci.Receiver))
					}
					for i, a := range ex.Args {
						if i+1 < len(ci.Fn.Params) && ci.Fn.Params[i+1].ByRef {
							if argId, ok := a.Value.(*ast.IdentExpr); ok && e.byRefVars != nil && e.byRefVars[argId.Name] {
								args = append(args, argId.Name)
							} else {
								args = append(args, "&"+e.emitExpr(a.Value))
							}
						} else {
							args = append(args, e.emitExpr(a.Value))
						}
					}
					return fmt.Sprintf("%s(%s)", ci.Fn.Name, strings.Join(args, ", "))
				}
			}
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
