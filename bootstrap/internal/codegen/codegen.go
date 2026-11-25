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
	if prog == nil || prog.EntryModule == nil {
		return "", fmt.Errorf("nil program")
	}

	e := &emitter{
		emitLineDirectives: opts.EmitLineDirectives,
		prog:               prog,
	}
	var bodyBuf strings.Builder
	for _, mod := range prog.Modules {
		if mod.Program == nil {
			continue
		}
		e.currentMod = mod
		bodyBuf.WriteString(e.emitProgram(mod.Program))
	}

	if mainSym, ok := prog.EntryModule.Functions["main"]; ok {
		if mainSym.ReturnType.Equals(sema.IntType) {
			e.useGoImport("os", "_cb_os")
			bodyBuf.WriteString("func main() {\n\tif code := cb_main(); code != 0 {\n\t\t_cb_os.Exit(int(code))\n\t}\n}\n")
		} else {
			bodyBuf.WriteString("func main() {\n\tcb_main()\n}\n")
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
	emitLineDirectives bool
	prog               *sema.TypedProgram
	currentMod         *sema.ModuleInfo
	currentFnSym       *sema.FnSymbol
	imports            map[string]bool
	goImports          map[string]string
	usedDictGet        bool
	usedDictContains   bool
	usedJob            bool
	usedVoidJob        bool
	usedJobTimeout     bool
	usedVoidJobTimeout bool
	usedCancelToken    bool
	currentFn          *ast.FnDecl
	currentTryID       int
	tryCounter         int
	errCounter         int
	loopCounter        int
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

func (e *emitter) emitProgram(prog *ast.Program) string {
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
			sb.WriteString(fmt.Sprintf("type %s struct {\n", goName))
			for _, f := range d.Fields {
				fType := e.goSemaType(f.Type)
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

		case *ast.ConstDecl:
			goName := d.Name
			if e.currentMod != nil {
				if sym, ok := e.currentMod.GlobalVars.Lookup(d.Name); ok && sym.GoName != "" {
					goName = sym.GoName
				}
			}
			typeStr := ""
			if d.Type != nil && !d.Type.IsVoid() {
				typeStr = " " + e.goSemaType(*d.Type)
			}
			sb.WriteString(fmt.Sprintf("var %s%s = %s\n\n", goName, typeStr, e.emitExpr(d.Value)))

		case *ast.VarDecl:
			goName := d.Name
			if e.currentMod != nil {
				if sym, ok := e.currentMod.GlobalVars.Lookup(d.Name); ok && sym.GoName != "" {
					goName = sym.GoName
				}
			}
			e.emitZeroOrInitVarDecl(&sb, goName, d, "", "")
			sb.WriteString("\n")

		case *ast.FnDecl:
			if d.GoTarget != "" {
				sb.WriteString(e.emitGoBoundFn(d))
				sb.WriteString("\n")
				continue
			}
			sb.WriteString(e.emitFn(d))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (e *emitter) emitFnHeader(fn *ast.FnDecl) (string, *sema.FnSymbol, []string) {
	var fnSym *sema.FnSymbol
	if e.currentMod != nil {
		fnSym = e.currentMod.Functions[fn.Name]
	}
	name := fn.Name
	if fnSym != nil {
		name = e.fnGoName(fnSym)
	}

	var params []string
	var callArgs []string
	for i, p := range fn.Params {
		paramType := p.Type
		if fnSym != nil && i < len(fnSym.Params) {
			paramType = fnSym.Params[i].Type
		}
		paramGoType := e.goSemaType(paramType)
		if p.ByRef {
			paramGoType = "*" + paramGoType
		}
		pGo := p.Name
		params = append(params, fmt.Sprintf("%s %s", pGo, paramGoType))
		callArgs = append(callArgs, pGo)
	}

	retType := ""
	hasRet := fnSym != nil && !fnSym.ReturnType.IsVoid()
	if fn.CanError {
		if hasRet {
			retType = fmt.Sprintf(" (%s, error)", e.goSemaType(fnSym.ReturnType))
		} else {
			retType = " error"
		}
	} else if hasRet {
		retType = " " + e.goSemaType(fnSym.ReturnType)
	}

	header := fmt.Sprintf("func %s(%s)%s {", name, strings.Join(params, ", "), retType)
	return header, fnSym, callArgs
}

func (e *emitter) emitGoBoundFn(fn *ast.FnDecl) string {
	var sb strings.Builder
	header, fnSym, callArgs := e.emitFnHeader(fn)
	hasRet := fnSym != nil && !fnSym.ReturnType.IsVoid()

	sb.WriteString(header + "\n")
	callExpr := fmt.Sprintf("%s(%s)", fn.GoTarget, strings.Join(callArgs, ", "))
	if fn.CanError || hasRet {
		sb.WriteString(fmt.Sprintf("\treturn %s\n", callExpr))
	} else {
		sb.WriteString(fmt.Sprintf("\t%s\n", callExpr))
	}
	sb.WriteString("}\n")
	return sb.String()
}

func (e *emitter) emitFn(fn *ast.FnDecl) string {
	var sb strings.Builder
	header, fnSym, _ := e.emitFnHeader(fn)

	prevFn := e.currentFn
	prevFnSym := e.currentFnSym
	prevTryID := e.currentTryID
	e.currentFn = fn
	e.currentFnSym = fnSym
	e.currentTryID = 0
	defer func() {
		e.currentFn = prevFn
		e.currentFnSym = prevFnSym
		e.currentTryID = prevTryID
	}()

	sb.WriteString(header + "\n")
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
	return sb.String()
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

	case sema.KindErr:
		return "error"
	case sema.KindStruct, sema.KindEnum:
		name := t.Name
		if t.GoName != "" {
			name = t.GoName
		}
		return name
	case sema.KindList:
		if len(t.SubTypes) == 1 {
			return "[]" + e.goSemaType(t.SubTypes[0])
		}
		return "[]any"
	case sema.KindDict:
		if len(t.SubTypes) == 2 {
			return fmt.Sprintf("map[%s]%s", e.goSemaType(t.SubTypes[0]), e.goSemaType(t.SubTypes[1]))
		}
		return "map[any]any"
	case sema.KindJob:
		if t.Elem().IsVoid() {
			e.usedVoidJob = true
			return "*cbVoidJob"
		}
		e.usedJob = true
		return fmt.Sprintf("*cbJob[%s]", e.goSemaType(t.Elem()))
	case sema.KindCancelToken:
		e.usedCancelToken = true
		return "*cbCancelToken"
	default:
		return "any"
	}
}

func (e *emitter) lookupStruct(t sema.Type) *sema.StructSymbol {
	if e.prog == nil {
		return nil
	}
	return e.prog.LookupStruct(t, e.currentMod)
}

func (e *emitter) structNeedsFieldInit(s *sema.StructSymbol) bool {
	if s == nil || s.IsGoType || strings.HasPrefix(s.GoName, "*") {
		return false
	}
	for _, f := range s.Fields {
		switch f.Type.Kind {
		case sema.KindDict, sema.KindCancelToken:
			return true
		case sema.KindStruct:
			if e.structNeedsFieldInit(e.lookupStruct(f.Type)) {
				return true
			}
		}
	}
	return false
}

func (e *emitter) emitZeroStruct(t sema.Type) string {
	if strings.HasPrefix(t.GoName, "*") {
		return "nil"
	}
	goType := e.goSemaType(t)
	sym := e.lookupStruct(t)
	if sym != nil && sym.IsGoType {
		return fmt.Sprintf("*new(%s)", goType)
	}
	if !e.structNeedsFieldInit(sym) {
		return goType + "{}"
	}
	var inits []string
	for _, f := range sym.Fields {
		switch f.Type.Kind {
		case sema.KindDict:
			inits = append(inits, fmt.Sprintf("%s: %s{}", f.Name, e.goSemaType(f.Type)))
		case sema.KindCancelToken:
			e.usedCancelToken = true
			inits = append(inits, fmt.Sprintf("%s: cbNewCancelToken()", f.Name))
		case sema.KindStruct:
			if e.structNeedsFieldInit(e.lookupStruct(f.Type)) {
				inits = append(inits, fmt.Sprintf("%s: %s", f.Name, e.emitZeroStruct(f.Type)))
			}
		}
	}
	return fmt.Sprintf("%s{%s}", goType, strings.Join(inits, ", "))
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
		return "nil"
	case sema.KindList, sema.KindDict, sema.KindJob:
		return "nil"
	case sema.KindCancelToken:
		e.usedCancelToken = true
		return "cbNewCancelToken()"
	case sema.KindEnum:
		return e.goSemaType(t) + "(0)"
	case sema.KindStruct:
		return e.emitZeroStruct(t)
	default:
		return "nil"
	}
}
