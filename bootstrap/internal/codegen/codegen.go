package codegen

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/sema"
)

// Options controls Go code emission behavior.
type Options struct {
	EmitLineDirectives bool
}

// EmitGo emits a single-file procedural Go program from a semantically analyzed TypedProgram
// with //line source directives enabled.
func EmitGo(prog *sema.TypedProgram) (string, error) {
	return EmitGoWithOptions(prog, Options{EmitLineDirectives: true})
}

// EmitGoWithOptions emits a single-file procedural Go program using the specified Options.
func EmitGoWithOptions(prog *sema.TypedProgram, opts Options) (string, error) {
	if prog == nil || prog.Main == nil {
		return "", fmt.Errorf("nil program")
	}

	e := &emitter{
		prog:               prog,
		emitLineDirectives: opts.EmitLineDirectives,
	}
	var bodyBuf strings.Builder
	for _, mod := range prog.Modules {
		if mod.Program == nil {
			continue
		}
		e.currentMod = mod
		modCode, err := e.emitProgram(mod.Program)
		if err != nil {
			return "", err
		}
		bodyBuf.WriteString(modCode)
	}

	if prog.EntryModule != nil {
		if mainSym, ok := prog.EntryModule.Functions["main"]; ok {
			if mainSym.ReturnType.Equals(sema.IntType) {
				e.useImport("os")
				bodyBuf.WriteString("func main() {\n\tif code := cb_main(); code != 0 {\n\t\tos.Exit(int(code))\n\t}\n}\n")
			} else {
				bodyBuf.WriteString("func main() {\n\tcb_main()\n}\n")
			}
		}
	}
	bodyCode := bodyBuf.String()

	helpers := e.emitRuntimeHelpers()

	importLines := e.formatImportLines()
	var out strings.Builder
	out.WriteString("package main\n\n")
	if len(importLines) == 1 {
		out.WriteString(fmt.Sprintf("import %s\n\n", importLines[0]))
	} else if len(importLines) > 1 {
		out.WriteString("import (\n")
		for _, imp := range importLines {
			out.WriteString(fmt.Sprintf("\t%s\n", imp))
		}
		out.WriteString(")\n\n")
	}
	out.WriteString(helpers)
	out.WriteString(bodyCode)
	return out.String(), nil
}

type emitter struct {
	prog                *sema.TypedProgram
	emitLineDirectives  bool
	currentMod          *sema.ModuleInfo
	currentFnSym        *sema.FnSymbol
	imports             map[string]bool
	goImports           map[string]string
	usedListGet         bool
	usedStrGet          bool
	usedSliceStr        bool
	usedSliceList       bool
	usedDictGet         bool
	usedDictContains    bool
	usedStrToInt        bool
	usedStrFromCharCode bool
	usedCbError         bool
	usedBoxHelpers      bool
	usedMailbox         bool
	usedJob             bool
	byRefVars           map[string]bool
	currentFn           *ast.FnDecl
	currentTryID        int
	tryCounter          int
	errCounter          int
	loopCounter         int
	tryUsedCatch        map[int]bool
}

var goReservedIdents = map[string]bool{
	"func": true, "var": true, "map": true, "chan": true, "package": true,
	"interface": true, "go": true, "fallthrough": true, "range": true, "default": true,
	"switch": true, "struct": true, "defer": true, "goto": true, "select": true,
	"case": true, "type": true, "const": true, "return": true, "break": true,
	"continue": true, "for": true, "if": true, "else": true, "import": true,
	"fmt": true, "strings": true, "os": true, "io": true, "bytes": true,
	"strconv": true, "regexp": true, "filepath": true, "json": true, "exec": true,
	"any": true, "error": true, "string": true, "int64": true, "float64": true,
	"byte": true, "bool": true, "true": true, "false": true, "nil": true, "len": true,
	"append": true, "make": true, "new": true, "panic": true, "recover": true,
	"close": true, "delete": true, "copy": true, "print": true, "println": true,
}

func goIdent(name string) string {
	if goReservedIdents[name] {
		return "cb_id_" + name
	}
	return name
}

func goStructFieldName(name string) string {
	return "Cb_" + name
}

func (e *emitter) useImport(pkg string) {
	if e.imports == nil {
		e.imports = make(map[string]bool)
	}
	e.imports[pkg] = true
}

func (e *emitter) useGoImport(path, alias string) {
	if e.goImports == nil {
		e.goImports = make(map[string]string)
	}
	key := alias + "\x00" + path
	if alias != "" {
		e.goImports[key] = fmt.Sprintf("%s %q", alias, path)
	} else {
		e.goImports[key] = fmt.Sprintf("%q", path)
	}
}

func (e *emitter) formatImportLines() []string {
	seen := make(map[string]bool)
	for pkg := range e.imports {
		seen[fmt.Sprintf("%q", pkg)] = true
	}
	for _, spec := range e.goImports {
		seen[spec] = true
	}
	return slices.Sorted(maps.Keys(seen))
}

func (e *emitter) emitProgram(prog *ast.Program) (string, error) {
	var sb strings.Builder

	for _, decl := range prog.Decls {
		switch d := decl.(type) {
		case *ast.GoImportDecl:
			e.useGoImport(d.Path, d.Alias)

		case *ast.GoBlockDecl:
			sb.WriteString(d.Code)
			if !strings.HasSuffix(d.Code, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("\n")

		case *ast.TypeDecl:
			if d.GoName != "" {
				continue
			}
			var structSym *sema.StructSymbol
			if e.currentMod != nil {
				structSym = e.currentMod.Structs[d.Name]
			}
			goName := d.Name
			if structSym != nil && structSym.GoName != "" {
				goName = structSym.GoName
			}
			goName = goIdent(goName)
			sb.WriteString(fmt.Sprintf("type %s struct {\n", goName))
			for i, f := range d.Fields {
				fType := e.goSemaType(structSym.Fields[i].Type)
				if structSym != nil && i < len(structSym.Fields) && structSym.Fields[i].Boxed {
					fType = "*" + fType
				}
				sb.WriteString(fmt.Sprintf("\t%s %s \x60json:%q\x60\n", goStructFieldName(f.Name), fType, f.Name))
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
			goName = goIdent(goName)
			sb.WriteString(fmt.Sprintf("type %s int64\n\nconst (\n", goName))
			for i, v := range d.Variants {
				if i == 0 {
					sb.WriteString(fmt.Sprintf("\t%s_%s %s = iota\n", goName, v, goName))
				} else {
					sb.WriteString(fmt.Sprintf("\t%s_%s\n", goName, v))
				}
			}
			sb.WriteString(")\n\n")

		case *ast.ConstDecl:
			goName := d.Name
			if e.currentMod != nil {
				if sym, ok := e.currentMod.GlobalVars.Lookup(d.Name); ok && sym.GoName != "" {
					goName = sym.GoName
				}
			}
			goName = goIdent(goName)
			typeStr := ""
			if cType, ok := e.prog.ConstTypes[d]; ok && !cType.IsVoid() {
				typeStr = " " + e.goSemaType(cType)
			}
			sb.WriteString(fmt.Sprintf("var %s%s = %s\n\n", goName, typeStr, e.emitExpr(d.Value)))

		case *ast.FnDecl:
			if d.GoTarget != "" {
				sb.WriteString(e.emitGoBoundFn(d))
				sb.WriteString("\n")
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

func (e *emitter) emitGoBoundFn(fn *ast.FnDecl) string {
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
	} else {
		name = goIdent(name)
	}

	var params []string
	var callArgs []string
	for i, p := range fn.Params {
		paramGoType := e.goSemaType(fnSym.Params[i].Type)
		if p.ByRef {
			paramGoType = "*" + paramGoType
		}
		pGo := goIdent(p.Name)
		params = append(params, fmt.Sprintf("%s %s", pGo, paramGoType))
		callArgs = append(callArgs, pGo)
	}

	retType := ""
	hasRet := fnSym != nil && !fnSym.ReturnType.IsVoid()
	if fn.CanError {
		e.usedCbError = true
		if hasRet {
			retType = fmt.Sprintf(" (%s, error)", e.goSemaType(fnSym.ReturnType))
		} else {
			retType = " error"
		}
	} else if hasRet {
		retType = " " + e.goSemaType(fnSym.ReturnType)
	}

	sb.WriteString(fmt.Sprintf("func %s(%s)%s {\n", name, strings.Join(params, ", "), retType))
	callExpr := fmt.Sprintf("%s(%s)", fn.GoTarget, strings.Join(callArgs, ", "))
	if fn.CanError {
		if hasRet {
			sb.WriteString(fmt.Sprintf("\t_res, _err := %s\n", callExpr))
			sb.WriteString("\tif _err != nil {\n")
			sb.WriteString(fmt.Sprintf("\t\treturn %s, CbError{msg: _err.Error()}\n", e.goSemaZeroValue(fnSym.ReturnType)))
			sb.WriteString("\t}\n")
			sb.WriteString("\treturn _res, nil\n")
		} else {
			sb.WriteString(fmt.Sprintf("\t_err := %s\n", callExpr))
			sb.WriteString("\tif _err != nil {\n")
			sb.WriteString("\t\treturn CbError{msg: _err.Error()}\n")
			sb.WriteString("\t}\n")
			sb.WriteString("\treturn nil\n")
		}
	} else if hasRet {
		sb.WriteString(fmt.Sprintf("\treturn %s\n", callExpr))
	} else {
		sb.WriteString(fmt.Sprintf("\t%s\n", callExpr))
	}
	sb.WriteString("}\n")
	return sb.String()
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
	} else {
		name = goIdent(name)
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
		paramGoType := e.goSemaType(fnSym.Params[i].Type)
		if p.ByRef {
			e.byRefVars[p.Name] = true
			paramGoType = "*" + paramGoType
		}
		params = append(params, fmt.Sprintf("%s %s", goIdent(p.Name), paramGoType))
	}

	retType := ""
	if fn.CanError {
		e.usedCbError = true
		if fnSym != nil && !fnSym.ReturnType.IsVoid() {
			retType = fmt.Sprintf(" (%s, error)", e.goSemaType(fnSym.ReturnType))
		} else {
			retType = " error"
		}
	} else if fnSym != nil && !fnSym.ReturnType.IsVoid() {
		retType = " " + e.goSemaType(fnSym.ReturnType)
	}

	sb.WriteString(fmt.Sprintf("func %s(%s)%s {\n", name, strings.Join(params, ", "), retType))
	for _, st := range fn.Body {
		e.emitStmt(&sb, st, "\t")
	}
	if fnSym != nil && !fnSym.ReturnType.IsVoid() {
		if len(fn.Body) > 0 {
			if _, isRet := fn.Body[len(fn.Body)-1].(*ast.ReturnStmt); !isRet {
				zeroVal := e.goSemaZeroValue(fnSym.ReturnType)
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
	case sema.KindByte:
		return "byte"
	case sema.KindAny:
		return "any"
	case sema.KindErr:
		e.usedCbError = true
		return "CbError"
	case sema.KindStruct, sema.KindEnum:
		name := t.Name
		if t.GoName != "" {
			name = t.GoName
		}
		return goIdent(name)
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
	case sema.KindMailbox:
		e.usedMailbox = true
		if t.ElemType != nil {
			return fmt.Sprintf("*CbMailbox[%s]", e.goSemaType(*t.ElemType))
		}
		return "*CbMailbox[any]"
	case sema.KindJob:
		e.usedJob = true
		if t.ElemType != nil {
			return fmt.Sprintf("*CbJob[%s]", e.goSemaType(*t.ElemType))
		}
		return "*CbJob[struct{}]"
	default:
		return "any"
	}
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
	case sema.KindByte:
		return "byte(0)"
	case sema.KindErr:
		e.usedCbError = true
		return "CbError{}"
	case sema.KindList, sema.KindDict, sema.KindMailbox, sema.KindJob:
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
