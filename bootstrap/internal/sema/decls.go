package sema

import (
	"strings"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/token"
)

func (a *analyzer) analyzeProgram(prog *ast.Program) error {
	// Pass 0: Pre-declare all user-defined types (type, enum) so they can reference each other.
	for _, decl := range prog.Decls {
		switch d := decl.(type) {
		case *ast.TypeDecl:
			if err := a.predeclareType(d.Name, d.Pos); err != nil {
				return err
			}
			goName := a.mod.symbolGoName(d.Name)
			isGoType := d.GoName != ""
			if isGoType {
				goName = d.GoName
			}
			a.mod.Structs[d.Name] = &StructSymbol{
				Name:     d.Name,
				GoName:   goName,
				ModuleID: a.mod.ID,
				FieldMap: make(map[string]FieldSymbol),
				Public:   d.Public,
				IsGoType: isGoType,
			}
		case *ast.EnumDecl:
			if err := a.predeclareType(d.Name, d.Pos); err != nil {
				return err
			}
			a.mod.Enums[d.Name] = &EnumSymbol{
				Name:       d.Name,
				GoName:     a.mod.symbolGoName(d.Name),
				ModuleID:   a.mod.ID,
				VariantSet: make(map[string]bool),
				Public:     d.Public,
			}
		}
	}

	// Pass 0.5: Resolve struct fields and enum variants.
	for _, decl := range prog.Decls {
		switch d := decl.(type) {
		case *ast.TypeDecl:
			if err := a.resolveTypeDecl(d); err != nil {
				return err
			}
		case *ast.EnumDecl:
			if err := a.resolveEnumDecl(d); err != nil {
				return err
			}
		}
	}

	// Pass 0.6: Reject direct and mutually recursive value struct fields, then validate dict key comparability.
	if err := a.checkRecursiveStructFields(); err != nil {
		return err
	}
	if err := a.validateStructFieldTypes(); err != nil {
		return err
	}

	// Pass 1: Register all top-level function declarations so functions can call each other regardless of order.
	for _, decl := range prog.Decls {
		fnDecl, ok := decl.(*ast.FnDecl)
		if !ok {
			continue
		}
		if err := a.registerFnDecl(fnDecl); err != nil {
			return err
		}
	}

	// Pass 2: Analyze top-level constants and variables in declaration order.
	for _, decl := range prog.Decls {
		switch d := decl.(type) {
		case *ast.ConstDecl:
			if err := a.analyzeTopLevelConst(d); err != nil {
				return err
			}
		case *ast.VarDecl:
			if err := a.analyzeTopLevelVar(d); err != nil {
				return err
			}
		}
	}

	// Pass 3: Analyze all function bodies.
	for _, decl := range prog.Decls {
		fnDecl, ok := decl.(*ast.FnDecl)
		if !ok {
			continue
		}
		if err := a.analyzeFnBody(fnDecl); err != nil {
			return err
		}
	}

	return nil
}

func (a *analyzer) predeclareType(name string, pos token.Position) error {
	if _, exists := a.mod.Structs[name]; exists {
		return newSemaError(pos, "type %q is already declared", name)
	}
	if _, exists := a.mod.Enums[name]; exists {
		return newSemaError(pos, "type %q is already declared", name)
	}
	return nil
}

func (a *analyzer) resolveTypeDecl(d *ast.TypeDecl) error {
	sym := a.mod.Structs[d.Name]
	for i, f := range d.Fields {
		if _, dup := sym.FieldMap[f.Name]; dup {
			return newSemaError(f.Pos, "duplicate field %q in type %q", f.Name, d.Name)
		}
		fType, err := a.resolveTypeNodes(f.Type)
		if err != nil {
			return err
		}
		d.Fields[i].Type = fType
		fs := FieldSymbol{Name: f.Name, Type: fType}
		sym.Fields = append(sym.Fields, fs)
		sym.FieldMap[f.Name] = fs
	}
	return nil
}

func (a *analyzer) checkRecursiveStructFields() error {
	for _, decl := range a.mod.Program.Decls {
		td, ok := decl.(*ast.TypeDecl)
		if !ok || td.GoName != "" {
			continue
		}
		sym := a.mod.Structs[td.Name]
		if sym == nil || sym.IsGoType {
			continue
		}
		for _, f := range td.Fields {
			if a.structReaches(f.Type, sym, make(map[string]bool)) {
				return newSemaError(
					f.Pos,
					"recursive struct field '%s.%s' cannot contain '%s' directly by value; use 'list[%s]' for recursive child nodes",
					sym.Name,
					f.Name,
					f.Type.Name,
					f.Type.Name,
				)
			}
		}
	}
	return nil
}

func (a *analyzer) validateStructFieldTypes() error {
	for _, decl := range a.mod.Program.Decls {
		td, ok := decl.(*ast.TypeDecl)
		if !ok || td.GoName != "" {
			continue
		}
		for _, f := range td.Fields {
			if err := a.validateType(f.Type); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *analyzer) structReaches(t Type, target *StructSymbol, visited map[string]bool) bool {
	switch t.Kind {
	case KindList, KindDict, KindJob, KindCancelToken:
		return false
	case KindStruct:
		s := a.lookupStruct(t)
		if s == nil || s.IsGoType {
			return false
		}
		if s == target {
			return true
		}
		key := s.ModuleID + "." + s.Name
		if visited[key] {
			return false
		}
		visited[key] = true
		for _, f := range s.Fields {
			if a.structReaches(f.Type, target, visited) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (a *analyzer) resolveEnumDecl(d *ast.EnumDecl) error {
	sym := a.mod.Enums[d.Name]
	for _, v := range d.Variants {
		if sym.VariantSet[v] {
			return newSemaError(d.Pos, "duplicate variant %q in enum %q", v, d.Name)
		}
		sym.VariantSet[v] = true
	}
	return nil
}

func (a *analyzer) registerFnDecl(fn *ast.FnDecl) error {
	if existing, exists := a.mod.Functions[fn.Name]; exists {
		return newSemaError(fn.Pos, "function %q is already declared (previous declaration at %s)", fn.Name, existing.Pos.String())
	}

	var params []ParamSymbol
	seenParams := make(map[string]token.Position)
	for i, p := range fn.Params {
		if prevPos, dup := seenParams[p.Name]; dup {
			return newSemaError(p.Pos, "duplicate parameter %q in function %q (previous at %s)", p.Name, fn.Name, prevPos.String())
		}
		seenParams[p.Name] = p.Pos
		pType, err := a.resolveType(p.Type)
		if err != nil {
			return err
		}
		fn.Params[i].Type = pType
		params = append(params, ParamSymbol{
			Name:  p.Name,
			Type:  pType,
			ByRef: p.ByRef,
			Pos:   p.Pos,
		})
	}

	retType := VoidType
	if fn.ReturnType != nil {
		resolvedRet, err := a.resolveType(*fn.ReturnType)
		if err != nil {
			return err
		}
		*fn.ReturnType = resolvedRet
		retType = resolvedRet
	}

	if a.mod.IsEntry && fn.Name == "main" {
		if len(params) != 0 {
			return newSemaError(fn.Pos, "function %q cannot have parameters", fn.Name)
		}
		if !retType.IsVoid() && !retType.Equals(IntType) {
			return newSemaError(fn.Pos, "function %q must return int (or void), got %s", fn.Name, retType.String())
		}
		if fn.CanError {
			return newSemaError(fn.Pos, "function %q cannot be declared 'can error'", fn.Name)
		}
	}

	a.mod.Functions[fn.Name] = &FnSymbol{
		Name:       fn.Name,
		GoName:     a.mod.symbolGoName(fn.Name),
		Params:     params,
		ReturnType: retType,
		CanError:   fn.CanError,
		Public:     fn.Public,
		Pos:        fn.Pos,
	}
	return nil
}

func (a *analyzer) resolveVarDeclType(label string, name string, pos token.Position, declType *Type, val ast.Expr, allowFallible bool, scope *Scope) (Type, error) {
	if val != nil {
		var valType Type
		var err error
		if allowFallible {
			valType, err = a.inferRootExpr(val, scope)
		} else {
			valType, err = a.inferExpr(val, scope)
		}
		if err != nil {
			return VoidType, err
		}
		if valType.IsVoid() {
			return VoidType, newSemaError(pos, "%s %q initializer has void type", label, name)
		}
		finalType := valType
		if declType != nil {
			declaredType, err := a.resolveType(*declType)
			if err != nil {
				return VoidType, err
			}
			if !declaredType.Equals(valType) {
				return VoidType, newSemaError(pos, "%s %q declared as %s, got %s", label, name, declaredType.String(), valType.String())
			}
			finalType = declaredType
		}
		return finalType, nil
	}

	if declType == nil {
		return VoidType, newSemaError(pos, "%s %q requires either 'as Type' or '= Expr'", label, name)
	}
	declaredType, err := a.resolveType(*declType)
	if err != nil {
		return VoidType, err
	}
	if declaredType.IsVoid() {
		return VoidType, newSemaError(pos, "%s %q cannot have void type", label, name)
	}
	return declaredType, nil
}

func (a *analyzer) registerTopLevelVar(name string, pos token.Position, isPublic bool, isConst bool, declType *Type, val ast.Expr) (Type, error) {
	label := "variable"
	if isConst {
		label = "constant"
	}
	if existingFn, exists := a.mod.Functions[name]; exists {
		return VoidType, newSemaError(pos, "%s %q conflicts with function declared at %s", label, name, existingFn.Pos.String())
	}

	finalType, err := a.resolveVarDeclType(label, name, pos, declType, val, false, a.mod.GlobalVars)
	if err != nil {
		return VoidType, err
	}

	sym := &VarSymbol{
		Name:     name,
		GoName:   a.mod.symbolGoName(name),
		Type:     finalType,
		IsConst:  isConst,
		IsGlobal: true,
		Public:   isPublic,
		Pos:      pos,
	}
	if err := a.mod.GlobalVars.Define(sym); err != nil {
		return VoidType, err
	}
	return finalType, nil
}

func (a *analyzer) analyzeTopLevelConst(d *ast.ConstDecl) error {
	finalType, err := a.registerTopLevelVar(d.Name, d.Pos, d.Public, true, d.Type, d.Value)
	if err != nil {
		return err
	}
	d.Type = &finalType
	return nil
}

func (a *analyzer) analyzeTopLevelVar(d *ast.VarDecl) error {
	finalType, err := a.registerTopLevelVar(d.Name, d.Pos, d.Public, false, d.Type, d.Value)
	if err != nil {
		return err
	}
	d.Type = &finalType
	return nil
}

func (a *analyzer) resolveType(t Type) (Type, error) {
	resolved, err := a.resolveTypeNodes(t)
	if err != nil {
		return VoidType, err
	}
	if err := a.validateType(resolved); err != nil {
		return VoidType, err
	}
	return resolved, nil
}

func (a *analyzer) validateType(t Type) error {
	switch t.Kind {
	case KindList:
		return a.validateType(t.SubTypes[0])
	case KindJob:
		if len(t.SubTypes) == 1 && !t.SubTypes[0].IsVoid() {
			return a.validateType(t.SubTypes[0])
		}
		return nil
	case KindDict:
		if err := a.validateType(t.SubTypes[0]); err != nil {
			return err
		}
		if !a.isComparableType(t.SubTypes[0]) {
			return newSemaError(t.SubTypes[0].Pos, "dict key type %s is not comparable", t.SubTypes[0].String())
		}
		return a.validateType(t.SubTypes[1])
	}
	return nil
}

func (a *analyzer) resolveTypeNodes(t Type) (Type, error) {
	switch t.Kind {
	case KindInt, KindFloat, KindStr, KindBool, KindByte, KindErr, KindCancelToken, KindStruct, KindEnum:
		return t, nil
	case KindList:
		elem, err := a.resolveTypeNodes(t.SubTypes[0])
		if err != nil {
			return VoidType, err
		}
		return Type{Pos: t.Pos, Kind: KindList, SubTypes: []Type{elem}}, nil
	case KindJob:
		if len(t.SubTypes) == 1 && !t.SubTypes[0].IsVoid() {
			elem, err := a.resolveTypeNodes(t.SubTypes[0])
			if err != nil {
				return VoidType, err
			}
			return Type{Pos: t.Pos, Kind: KindJob, SubTypes: []Type{elem}}, nil
		}
		return Type{Pos: t.Pos, Kind: KindJob, SubTypes: []Type{VoidType}}, nil
	case KindDict:
		kt, err := a.resolveTypeNodes(t.SubTypes[0])
		if err != nil {
			return VoidType, err
		}
		vt, err := a.resolveTypeNodes(t.SubTypes[1])
		if err != nil {
			return VoidType, err
		}
		return Type{Pos: t.Pos, Kind: KindDict, SubTypes: []Type{kt, vt}}, nil
	case "":
		parts := strings.Split(t.Name, ".")
		if len(parts) == 1 {
			name := parts[0]
			if sym, ok := a.mod.Structs[name]; ok {
				return Type{Pos: t.Pos, Kind: KindStruct, Name: name, ModuleID: a.mod.ID, GoName: sym.GoName}, nil
			}
			if sym, ok := a.mod.Enums[name]; ok {
				return Type{Pos: t.Pos, Kind: KindEnum, Name: name, ModuleID: a.mod.ID, GoName: sym.GoName}, nil
			}
		} else if len(parts) == 2 {
			modAlias, typeName := parts[0], parts[1]
			impMod, ok := a.mod.Imports[modAlias]
			if !ok {
				return VoidType, newSemaError(t.Pos, "unknown module %q in type %q", modAlias, t.String())
			}
			if sym, ok := impMod.Structs[typeName]; ok {
				if !sym.Public {
					return VoidType, newSemaError(t.Pos, "type %q in module %q is private", typeName, modAlias)
				}
				return Type{Pos: t.Pos, Kind: KindStruct, Name: typeName, ModuleID: impMod.ID, GoName: sym.GoName}, nil
			}
			if sym, ok := impMod.Enums[typeName]; ok {
				if !sym.Public {
					return VoidType, newSemaError(t.Pos, "enum %q in module %q is private", typeName, modAlias)
				}
				return Type{Pos: t.Pos, Kind: KindEnum, Name: typeName, ModuleID: impMod.ID, GoName: sym.GoName}, nil
			}
		}
	}
	return VoidType, newSemaError(t.Pos, "unknown or unsupported type %q", t.String())
}
