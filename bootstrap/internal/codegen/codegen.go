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
	var bodyBuf strings.Builder
	if len(prog.Modules) > 0 {
		for _, mod := range prog.Modules {
			if mod.IsStdlib || mod.Program == nil {
				continue
			}
			e.currentMod = mod
			modCode, err := e.emitProgram(mod.Program)
			if err != nil {
				return "", err
			}
			bodyBuf.WriteString(modCode)
		}
	} else {
		modCode, err := e.emitProgram(prog.Main)
		if err != nil {
			return "", err
		}
		bodyBuf.WriteString(modCode)
	}

	if mainSym, ok := prog.Functions["main"]; ok {
		if mainSym.ReturnType.Equals(sema.IntType) {
			e.usedOs = true
			bodyBuf.WriteString("func main() {\n\tif code := cb_main(); code != 0 {\n\t\tos.Exit(int(code))\n\t}\n}\n")
		} else {
			bodyBuf.WriteString("func main() {\n\tcb_main()\n}\n")
		}
	}
	bodyCode := bodyBuf.String()

	if e.usedStdFs {
		e.usedOs = true
		e.usedCbError = true
	}
	if e.usedStdOs {
		e.usedOs = true
	}
	if e.usedStdOsExec {
		e.usedBytes = true
		e.usedOsExec = true
		e.usedCbError = true
	}
	if e.usedStdJson {
		e.usedEncodingJson = true
		e.usedFmt = true
		e.usedStrconv = true
		e.usedCbError = true
	}
	if e.usedStdCli {
		e.usedFmt = true
		e.usedOs = true
		e.usedStrings = true
		e.usedCbError = true
	}

	var helpers strings.Builder
	if e.usedCbError {
		helpers.WriteString("type CbError struct {\n\tmsg  string\n\tcode int64\n\tfile string\n\tline int64\n}\n\nfunc (e CbError) Error() string { return e.msg }\n\n")
	}
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

	e.emitStdlibHelpers(&helpers)

	var imports []string
	if e.usedBytes {
		imports = append(imports, "bytes")
	}
	if e.usedEncodingJson {
		imports = append(imports, "encoding/json")
	}
	if e.usedFmt {
		imports = append(imports, "fmt")
	}
	if e.usedOs {
		imports = append(imports, "os")
	}
	if e.usedOsExec {
		imports = append(imports, "os/exec")
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
	currentMod       *sema.ModuleInfo
	currentFnSym     *sema.FnSymbol
	usedBytes        bool
	usedEncodingJson bool
	usedFmt          bool
	usedOs           bool
	usedOsExec       bool
	usedStrings      bool
	usedStrconv      bool
	usedListGet      bool
	usedDictGet      bool
	usedDictContains bool
	usedStrToInt     bool
	usedCbError      bool
	usedStdFs        bool
	usedStdOs        bool
	usedStdOsExec    bool
	usedStdJson      bool
	usedStdCli       bool
	byRefVars        map[string]bool
	currentFn        *ast.FnDecl
	currentTryID     int
	tryCounter       int
	errCounter       int
	tryUsedCatch     map[int]bool
	liftedExprs      map[ast.Expr]string
}

func (e *emitter) markStdlibUsage(goName string) {
	switch {
	case strings.HasPrefix(goName, "cbStdFs"):
		e.usedStdFs = true
	case goName == "cbStdOsExec":
		e.usedStdOs = true
		e.usedStdOsExec = true
	case strings.HasPrefix(goName, "cbStdOs"):
		e.usedStdOs = true
	case strings.HasPrefix(goName, "cbStdJson"):
		e.usedStdJson = true
	case strings.HasPrefix(goName, "cbStdCli"):
		e.usedStdCli = true
	}
}

func (e *emitter) emitProgram(prog *ast.Program) (string, error) {
	var sb strings.Builder

	for _, decl := range prog.Decls {
		switch d := decl.(type) {
		case *ast.TypeDecl:
			var structSym *sema.StructSymbol
			if e.currentMod != nil {
				structSym = e.currentMod.Structs[d.Name]
			}
			goName := d.Name
			if structSym != nil && structSym.GoName != "" {
				goName = structSym.GoName
			}
			sb.WriteString(fmt.Sprintf("type %s struct {\n", goName))
			for i, f := range d.Fields {
				fType := e.goType(f.Type)
				if structSym != nil && i < len(structSym.Fields) {
					fType = e.goSemaType(structSym.Fields[i].Type)
				}
				sb.WriteString(fmt.Sprintf("\t%s %s\n", f.Name, fType))
			}
			sb.WriteString("}\n\n")

		case *ast.EnumDecl:
			var enumSym *sema.EnumSymbol
			if e.currentMod != nil {
				enumSym = e.currentMod.Enums[d.Name]
			}
			goName := d.Name
			if enumSym != nil && enumSym.GoName != "" {
				goName = enumSym.GoName
			}
			sb.WriteString(fmt.Sprintf("type %s int64\n\nconst (\n", goName))
			for i, v := range d.Variants {
				if i == 0 {
					sb.WriteString(fmt.Sprintf("\t%s_%s %s = iota\n", goName, v, goName))
				} else {
					sb.WriteString(fmt.Sprintf("\t%s_%s\n", goName, v))
				}
			}
			sb.WriteString(")\n\n")

		case *ast.UnionDecl:
			var unionSym *sema.UnionSymbol
			if e.currentMod != nil {
				unionSym = e.currentMod.Unions[d.Name]
			}
			goName := d.Name
			if unionSym != nil && unionSym.GoName != "" {
				goName = unionSym.GoName
			}
			sb.WriteString(fmt.Sprintf("type %s interface {\n\tis%s()\n}\n\n", goName, goName))
			for i, v := range d.Variants {
				vStruct := fmt.Sprintf("%s_%s", goName, v.Name)
				vType := e.goType(v.Type)
				if unionSym != nil && i < len(unionSym.Variants) {
					vType = e.goSemaType(unionSym.Variants[i].Type)
				}
				sb.WriteString(fmt.Sprintf("type %s struct {\n\tValue %s\n}\n", vStruct, vType))
				sb.WriteString(fmt.Sprintf("func (%s) is%s() {}\n\n", vStruct, goName))
			}

		case *ast.ConstDecl:
			goName := d.Name
			if e.currentMod != nil {
				if sym, ok := e.currentMod.GlobalVars.Lookup(d.Name); ok && sym.GoName != "" {
					goName = sym.GoName
				}
			}
			typeStr := ""
			if cType, ok := e.prog.ConstTypes[d]; ok && !cType.IsVoid() {
				typeStr = " " + e.goSemaType(cType)
			} else if d.Type != nil {
				typeStr = " " + e.goType(*d.Type)
			}
			sb.WriteString(fmt.Sprintf("var %s%s = %s\n\n", goName, typeStr, e.emitExpr(d.Value)))

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

	return sb.String(), nil
}

func (e *emitter) emitFn(fn *ast.FnDecl) (string, error) {
	var sb strings.Builder
	var fnSym *sema.FnSymbol
	if e.currentMod != nil {
		fnSym = e.currentMod.Functions[fn.Name]
	}
	name := fn.Name
	if fnSym != nil && fnSym.GoName != "" {
		name = fnSym.GoName
	}
	if name == "main" && (e.currentMod == nil || e.currentMod.IsEntry) {
		name = "cb_main"
	}

	prevByRef := e.byRefVars
	prevFn := e.currentFn
	prevFnSym := e.currentFnSym
	prevTryID := e.currentTryID
	e.byRefVars = make(map[string]bool)
	e.currentFn = fn
	e.currentFnSym = fnSym
	e.currentTryID = 0
	defer func() {
		e.byRefVars = prevByRef
		e.currentFn = prevFn
		e.currentFnSym = prevFnSym
		e.currentTryID = prevTryID
	}()

	var params []string
	for i, p := range fn.Params {
		paramGoType := e.goType(p.Type)
		if fnSym != nil && i < len(fnSym.Params) {
			paramGoType = e.goSemaType(fnSym.Params[i].Type)
		}
		if p.ByRef {
			e.byRefVars[p.Name] = true
			paramGoType = "*" + paramGoType
		}
		params = append(params, fmt.Sprintf("%s %s", p.Name, paramGoType))
	}

	retType := ""
	if fn.CanError {
		e.usedCbError = true
		if fnSym != nil && !fnSym.ReturnType.IsVoid() {
			retType = fmt.Sprintf(" (%s, error)", e.goSemaType(fnSym.ReturnType))
		} else if fn.ReturnType != nil {
			retType = fmt.Sprintf(" (%s, error)", e.goType(*fn.ReturnType))
		} else {
			retType = " error"
		}
	} else if fnSym != nil && !fnSym.ReturnType.IsVoid() {
		retType = " " + e.goSemaType(fnSym.ReturnType)
	} else if fn.ReturnType != nil {
		retType = " " + e.goType(*fn.ReturnType)
	}

	sb.WriteString(fmt.Sprintf("func %s(%s)%s {\n", name, strings.Join(params, ", "), retType))
	for _, st := range fn.Body {
		e.emitStmt(&sb, st, "\t")
	}
	if fn.ReturnType != nil {
		if len(fn.Body) > 0 {
			if _, isRet := fn.Body[len(fn.Body)-1].(*ast.ReturnStmt); !isRet {
				zeroVal := e.goZeroValue(fn.ReturnType)
				if fnSym != nil && !fnSym.ReturnType.IsVoid() {
					zeroVal = e.goSemaZeroValue(fnSym.ReturnType)
				}
				if fn.CanError {
					sb.WriteString(fmt.Sprintf("\treturn %s, nil\n", zeroVal))
				} else {
					sb.WriteString(fmt.Sprintf("\treturn %s\n", zeroVal))
				}
			}
		}
	} else if fn.CanError {
		if len(fn.Body) == 0 {
			sb.WriteString("\treturn nil\n")
		} else if _, isRet := fn.Body[len(fn.Body)-1].(*ast.ReturnStmt); !isRet {
			sb.WriteString("\treturn nil\n")
		}
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
	case sema.KindErr:
		e.usedCbError = true
		return "CbError"
	case sema.KindStruct, sema.KindEnum, sema.KindUnion:
		switch t.ModuleID {
		case "std/os":
			e.usedStdOs = true
		case "std/json":
			e.usedStdJson = true
		case "std/cli":
			e.usedStdCli = true
		}
		if t.GoName != "" {
			return t.GoName
		}
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
		case "err":
			e.usedCbError = true
			return "CbError"
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

func (e *emitter) goZeroValue(t *ast.TypeExpr) string {
	if t == nil {
		return ""
	}
	switch t.Kind {
	case ast.TypeBasic:
		switch t.Name {
		case "int":
			return "int64(0)"
		case "float":
			return "float64(0)"
		case "str":
			return `""`
		case "bool":
			return "false"
		case "err":
			e.usedCbError = true
			return "CbError{}"
		}
	case ast.TypeList, ast.TypeDict:
		return "nil"
	case ast.TypeQualified:
		if len(t.Parts) == 1 {
			name := t.Parts[0]
			if e.prog != nil {
				if e.prog.Unions[name] != nil {
					return "nil"
				}
				if e.prog.Enums[name] != nil {
					return name + "(0)"
				}
				if e.prog.Structs[name] != nil {
					return name + "{}"
				}
			}
		}
	}
	return fmt.Sprintf("*new(%s)", e.goType(*t))
}

func (e *emitter) goSemaZeroValue(t sema.Type) string {
	switch t.Kind {
	case sema.KindInt:
		return "int64(0)"
	case sema.KindFloat:
		return "float64(0)"
	case sema.KindStr:
		return `""`
	case sema.KindBool:
		return "false"
	case sema.KindErr:
		e.usedCbError = true
		return "CbError{}"
	case sema.KindList, sema.KindDict, sema.KindUnion:
		return "nil"
	case sema.KindEnum:
		return e.goSemaType(t) + "(0)"
	case sema.KindStruct:
		if strings.HasPrefix(t.GoName, "*") {
			return "nil"
		}
		return e.goSemaType(t) + "{}"
	default:
		return "nil"
	}
}

func (e *emitter) emitErrorJumpOrReturn(sb *strings.Builder, indent, errExpr string) {
	if e.currentTryID > 0 {
		if e.tryUsedCatch == nil {
			e.tryUsedCatch = make(map[int]bool)
		}
		e.tryUsedCatch[e.currentTryID] = true
		sb.WriteString(fmt.Sprintf("%s_tryErr_%d = %s\n", indent, e.currentTryID, errExpr))
		sb.WriteString(fmt.Sprintf("%sgoto _catch_%d\n", indent, e.currentTryID))
		return
	}
	if e.currentFnSym != nil && !e.currentFnSym.ReturnType.IsVoid() {
		sb.WriteString(fmt.Sprintf("%sreturn %s, %s\n", indent, e.goSemaZeroValue(e.currentFnSym.ReturnType), errExpr))
		return
	}
	if e.currentFn != nil && e.currentFn.ReturnType != nil {
		sb.WriteString(fmt.Sprintf("%sreturn %s, %s\n", indent, e.goZeroValue(e.currentFn.ReturnType), errExpr))
		return
	}
	sb.WriteString(fmt.Sprintf("%sreturn %s\n", indent, errExpr))
}

func (e *emitter) liftFallibleCalls(sb *strings.Builder, expr ast.Expr, indent string) bool {
	if expr == nil {
		return false
	}
	switch ex := expr.(type) {
	case *ast.UnaryExpr:
		return e.liftFallibleCalls(sb, ex.Operand, indent)
	case *ast.BinaryExpr:
		l := e.liftFallibleCalls(sb, ex.Left, indent)
		r := e.liftFallibleCalls(sb, ex.Right, indent)
		return l || r
	case *ast.ListLitExpr:
		any := false
		for _, el := range ex.Elements {
			if e.liftFallibleCalls(sb, el, indent) {
				any = true
			}
		}
		return any
	case *ast.DictLitExpr:
		any := false
		for _, ent := range ex.Entries {
			if e.liftFallibleCalls(sb, ent.Key, indent) {
				any = true
			}
			if e.liftFallibleCalls(sb, ent.Value, indent) {
				any = true
			}
		}
		return any
	case *ast.FieldAccessExpr:
		return e.liftFallibleCalls(sb, ex.Target, indent)
	case *ast.IndexExpr:
		t := e.liftFallibleCalls(sb, ex.Target, indent)
		i := e.liftFallibleCalls(sb, ex.Index, indent)
		return t || i
	case *ast.CallExpr:
		if e.prog == nil {
			return false
		}
		ci, hasCI := e.prog.CallInfos[ex]
		if hasCI && ci.Receiver != nil {
			e.liftFallibleCalls(sb, ci.Receiver, indent)
		}
		for _, a := range ex.Args {
			e.liftFallibleCalls(sb, a.Value, indent)
		}
		if !hasCI {
			return false
		}
		if ci.Kind == sema.CallError {
			e.usedCbError = true
			msgCode := e.emitExpr(ex.Args[0].Value)
			codeExpr := "int64(0)"
			if len(ex.Args) == 2 {
				codeExpr = e.emitExpr(ex.Args[1].Value)
			}
			cbErr := fmt.Sprintf("CbError{msg: %s, code: %s, file: %s, line: int64(%d)}",
				msgCode, codeExpr, strconv.Quote(ex.Pos.File), ex.Pos.Line)
			e.emitErrorJumpOrReturn(sb, indent, cbErr)
			return true
		}
		isFallibleMethod := ci.Kind == sema.CallMethod && (ci.MethodName == "std/cli.parse" || ci.MethodName == "std/cli.parseArgs")
		if isFallibleMethod || ((ci.Kind == sema.CallFunc || ci.Kind == sema.CallUFCS) && ci.Fn != nil && ci.Fn.CanError) {
			callStr := e.emitCallExprRaw(ex)
			e.errCounter++
			id := e.errCounter
			errVar := fmt.Sprintf("_err_%d", id)
			hasRet := isFallibleMethod || (ci.Fn != nil && !ci.Fn.ReturnType.IsVoid())
			if hasRet {
				valVar := fmt.Sprintf("_val_%d", id)
				if e.liftedExprs == nil {
					e.liftedExprs = make(map[ast.Expr]string)
				}
				e.liftedExprs[ex] = valVar
				sb.WriteString(fmt.Sprintf("%s%s, %s := %s\n", indent, valVar, errVar, callStr))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s := %s\n", indent, errVar, callStr))
			}
			sb.WriteString(fmt.Sprintf("%sif %s != nil {\n", indent, errVar))
			e.emitErrorJumpOrReturn(sb, indent+"\t", errVar)
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
			return true
		}
	}
	return false
}

func (e *emitter) emitStmt(sb *strings.Builder, stmt ast.Statement, indent string) {
	switch s := stmt.(type) {
	case *ast.VarDecl:
		e.liftFallibleCalls(sb, s.Value, indent)
		typeStr := ""
		if vType, ok := e.prog.VarTypes[s]; ok && !vType.IsVoid() {
			typeStr = " " + e.goSemaType(vType)
		} else if s.Type != nil {
			typeStr = " " + e.goType(*s.Type)
		}
		sb.WriteString(fmt.Sprintf("%svar %s%s = %s\n", indent, s.Name, typeStr, e.emitExpr(s.Value)))
		sb.WriteString(fmt.Sprintf("%s_ = %s\n", indent, s.Name))

	case *ast.Assignment:
		e.liftFallibleCalls(sb, s.Value, indent)
		if idx, ok := s.Target.(*ast.IndexExpr); ok {
			e.liftFallibleCalls(sb, idx.Target, indent)
			e.liftFallibleCalls(sb, idx.Index, indent)
			sb.WriteString(fmt.Sprintf("%s%s[%s] = %s\n", indent, e.emitExpr(idx.Target), e.emitExpr(idx.Index), e.emitExpr(s.Value)))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s = %s\n", indent, e.emitExpr(s.Target), e.emitExpr(s.Value)))
		}

	case *ast.ExprStmt:
		if call, ok := s.Expr.(*ast.CallExpr); ok && e.prog != nil {
			if ci, hasCI := e.prog.CallInfos[call]; hasCI {
				if ci.Kind == sema.CallError {
					e.liftFallibleCalls(sb, s.Expr, indent)
					return
				}
				isFallibleMethod := ci.Kind == sema.CallMethod && (ci.MethodName == "std/cli.parse" || ci.MethodName == "std/cli.parseArgs")
				if isFallibleMethod || ((ci.Kind == sema.CallFunc || ci.Kind == sema.CallUFCS) && ci.Fn != nil && ci.Fn.CanError) {
					e.liftFallibleCalls(sb, s.Expr, indent)
					if valVar, lifted := e.liftedExprs[call]; lifted {
						sb.WriteString(fmt.Sprintf("%s_ = %s\n", indent, valVar))
					}
					return
				}
			}
		}
		e.liftFallibleCalls(sb, s.Expr, indent)
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, e.emitExpr(s.Expr)))

	case *ast.IfStmt:
		e.liftFallibleCalls(sb, s.Cond, indent)
		sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, e.emitExpr(s.Cond)))
		for _, st := range s.ThenBody {
			e.emitStmt(sb, st, indent+"\t")
		}
		elseIndent := indent
		extraCloses := 0
		for _, elif := range s.ElseIfs {
			var liftBuf strings.Builder
			if e.liftFallibleCalls(&liftBuf, elif.Cond, elseIndent+"\t") {
				sb.WriteString(fmt.Sprintf("%s} else {\n", elseIndent))
				sb.WriteString(liftBuf.String())
				elseIndent += "\t"
				extraCloses++
				sb.WriteString(fmt.Sprintf("%sif %s {\n", elseIndent, e.emitExpr(elif.Cond)))
			} else {
				sb.WriteString(fmt.Sprintf("%s} else if %s {\n", elseIndent, e.emitExpr(elif.Cond)))
			}
			for _, st := range elif.Body {
				e.emitStmt(sb, st, elseIndent+"\t")
			}
		}
		if s.HasElse {
			sb.WriteString(fmt.Sprintf("%s} else {\n", elseIndent))
			for _, st := range s.ElseBody {
				e.emitStmt(sb, st, elseIndent+"\t")
			}
		}
		sb.WriteString(fmt.Sprintf("%s}\n", elseIndent))
		for i := 0; i < extraCloses; i++ {
			elseIndent = strings.TrimSuffix(elseIndent, "\t")
			sb.WriteString(fmt.Sprintf("%s}\n", elseIndent))
		}

	case *ast.ForStmt:
		if s.IsRange {
			e.liftFallibleCalls(sb, s.Start, indent)
			e.liftFallibleCalls(sb, s.End, indent)
			if s.Step != nil {
				e.liftFallibleCalls(sb, s.Step, indent)
			}
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
			e.liftFallibleCalls(sb, s.Iterable, indent)
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
		e.liftFallibleCalls(sb, s.Target, indent)
		if unionSym, isUnion := e.prog.SelectUnion[s]; isUnion {
			unionGoName := unionSym.Name
			if unionSym.GoName != "" {
				unionGoName = unionSym.GoName
			}
			hasBinding := false
			for _, cc := range s.Cases {
				if cc.Binding != "" {
					hasBinding = true
					break
				}
			}
			if hasBinding {
				sb.WriteString(fmt.Sprintf("%sswitch cbSwitch := (%s).(type) {\n", indent, e.emitExpr(s.Target)))
			} else {
				sb.WriteString(fmt.Sprintf("%sswitch (%s).(type) {\n", indent, e.emitExpr(s.Target)))
			}
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
					caseTypes = append(caseTypes, fmt.Sprintf("%s_%s", unionGoName, vName))
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
		var liftBuf strings.Builder
		if e.liftFallibleCalls(&liftBuf, s.Cond, indent+"\t") {
			sb.WriteString(fmt.Sprintf("%sfor {\n", indent))
			sb.WriteString(liftBuf.String())
			sb.WriteString(fmt.Sprintf("%s\tif !(%s) {\n%s\t\tbreak\n%s\t}\n", indent, e.emitExpr(s.Cond), indent, indent))
			for _, st := range s.Body {
				e.emitStmt(sb, st, indent+"\t")
			}
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		} else {
			sb.WriteString(fmt.Sprintf("%sfor %s {\n", indent, e.emitExpr(s.Cond)))
			for _, st := range s.Body {
				e.emitStmt(sb, st, indent+"\t")
			}
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		}

	case *ast.TryCatchStmt:
		e.usedCbError = true
		e.tryCounter++
		tryID := e.tryCounter
		prevTryID := e.currentTryID
		sb.WriteString(fmt.Sprintf("%svar _tryErr_%d error\n", indent, tryID))
		sb.WriteString(fmt.Sprintf("%s_ = _tryErr_%d\n", indent, tryID))
		sb.WriteString(fmt.Sprintf("%s{\n", indent))
		e.currentTryID = tryID
		for _, st := range s.TryBody {
			e.emitStmt(sb, st, indent+"\t")
		}
		e.currentTryID = prevTryID
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
		if e.tryUsedCatch == nil || !e.tryUsedCatch[tryID] {
			sb.WriteString(fmt.Sprintf("%sif false {\n%s\tgoto _catch_%d\n%s}\n", indent, indent, tryID, indent))
		}
		sb.WriteString(fmt.Sprintf("%sgoto _end_try_%d\n", indent, tryID))
		sb.WriteString(fmt.Sprintf("%s_catch_%d:\n", indent, tryID))
		sb.WriteString(fmt.Sprintf("%s{\n", indent))
		sb.WriteString(fmt.Sprintf("%s\tvar %s CbError\n", indent, s.CatchVar))
		sb.WriteString(fmt.Sprintf("%s\tif _cbErr, _ok := _tryErr_%d.(CbError); _ok {\n", indent, tryID))
		sb.WriteString(fmt.Sprintf("%s\t\t%s = _cbErr\n", indent, s.CatchVar))
		sb.WriteString(fmt.Sprintf("%s\t} else {\n", indent))
		sb.WriteString(fmt.Sprintf("%s\t\t%s = CbError{msg: _tryErr_%d.Error()}\n", indent, s.CatchVar, tryID))
		sb.WriteString(fmt.Sprintf("%s\t}\n", indent))
		sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, s.CatchVar))
		for _, st := range s.CatchBody {
			e.emitStmt(sb, st, indent+"\t")
		}
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
		sb.WriteString(fmt.Sprintf("%s_end_try_%d:\n", indent, tryID))

	case *ast.DeferStmt:
		prevTryID := e.currentTryID
		e.currentTryID = 0
		sb.WriteString(fmt.Sprintf("%sdefer func() {\n", indent))
		for _, st := range s.Body {
			e.emitStmt(sb, st, indent+"\t")
		}
		sb.WriteString(fmt.Sprintf("%s}()\n", indent))
		e.currentTryID = prevTryID

	case *ast.ReturnStmt:
		if s.Value != nil {
			e.liftFallibleCalls(sb, s.Value, indent)
			if e.currentFn != nil && e.currentFn.CanError {
				sb.WriteString(fmt.Sprintf("%sreturn %s, nil\n", indent, e.emitExpr(s.Value)))
			} else {
				sb.WriteString(fmt.Sprintf("%sreturn %s\n", indent, e.emitExpr(s.Value)))
			}
		} else {
			if e.currentFn != nil && e.currentFn.CanError {
				sb.WriteString(fmt.Sprintf("%sreturn nil\n", indent))
			} else {
				sb.WriteString(fmt.Sprintf("%sreturn\n", indent))
			}
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
		if e.prog != nil {
			if goName, ok := e.prog.IdentGoNames[ex]; ok {
				return goName
			}
		}
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
		if e.prog != nil {
			if goName, ok := e.prog.IdentGoNames[ex]; ok {
				return goName
			}
		}
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
		if e.liftedExprs != nil {
			if valVar, ok := e.liftedExprs[ex]; ok {
				return valVar
			}
		}
		return e.emitCallExprRaw(ex)

	default:
		return "nil"
	}
}

func (e *emitter) emitCallExprRaw(ex *ast.CallExpr) string {
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
				structGoName := ci.Struct.Name
				if ci.Struct.GoName != "" {
					structGoName = ci.Struct.GoName
				}
				return fmt.Sprintf("%s{%s}", structGoName, strings.Join(fields, ", "))

			case sema.CallUnionCtor:
				unionGoName := ci.Union.Name
				if ci.Union.GoName != "" {
					unionGoName = ci.Union.GoName
				}
				return fmt.Sprintf("%s_%s{Value: %s}", unionGoName, ci.VariantName, e.emitExpr(ex.Args[0].Value))

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
				case "std/json.stringify":
					e.usedStdJson = true
					return fmt.Sprintf("cbStdJsonStringify(%s)", e.emitExpr(ex.Args[0].Value))
				case "std/json.isNull", "std/json.asStr", "std/json.asInt", "std/json.asFloat", "std/json.asBool", "std/json.asList", "std/json.asDict":
					e.usedStdJson = true
					method := strings.TrimPrefix(ci.MethodName, "std/json.")
					return fmt.Sprintf("%s.%s()", e.emitExpr(ci.Receiver), method)
				case "std/json.get", "std/json.has":
					e.usedStdJson = true
					method := strings.TrimPrefix(ci.MethodName, "std/json.")
					return fmt.Sprintf("%s.%s(%s)", e.emitExpr(ci.Receiver), method, e.emitExpr(ex.Args[0].Value))
				case "std/cli.addFlag", "std/cli.addOption", "std/cli.addArg", "std/cli.parse", "std/cli.parseArgs", "std/cli.printUsage", "std/cli.hasFlag", "std/cli.getOption", "std/cli.getArg":
					e.usedStdCli = true
					method := strings.TrimPrefix(ci.MethodName, "std/cli.")
					var mArgs []string
					for _, a := range ex.Args {
						mArgs = append(mArgs, e.emitExpr(a.Value))
					}
					return fmt.Sprintf("%s.%s(%s)", e.emitExpr(ci.Receiver), method, strings.Join(mArgs, ", "))
				}

			case sema.CallFunc:
				if ci.Fn != nil {
					e.markStdlibUsage(ci.Fn.GoName)
					var args []string
					for i, a := range ex.Args {
						if i < len(ci.Fn.Params) && ci.Fn.Params[i].ByRef {
							if argId, ok := a.Value.(*ast.IdentExpr); ok && e.byRefVars != nil && e.byRefVars[argId.Name] {
								args = append(args, argId.Name)
							} else {
								args = append(args, "&"+e.emitExpr(a.Value))
							}
						} else {
							args = append(args, e.emitExpr(a.Value))
						}
					}
					fnGoName := ci.Fn.GoName
					if fnGoName == "" {
						fnGoName = ci.Fn.Name
					}
					if fnGoName == "main" {
						fnGoName = "cb_main"
					}
					return fmt.Sprintf("%s(%s)", fnGoName, strings.Join(args, ", "))
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
				fnGoName := ci.Fn.GoName
				if fnGoName == "" {
					fnGoName = ci.Fn.Name
				}
				return fmt.Sprintf("%s(%s)", fnGoName, strings.Join(args, ", "))
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
}
