package sema

import (
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
				Decl:     d,
				Pos:      d.Pos,
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
				Decl:       d,
				Pos:        d.Pos,
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

	// Pass 0.6: Reject direct and mutually recursive value struct fields.
	if err := a.checkRecursiveStructFields(); err != nil {
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

	// Pass 2: Analyze top-level constants in declaration order.
	for _, decl := range prog.Decls {
		constDecl, ok := decl.(*ast.ConstDecl)
		if !ok {
			continue
		}
		if err := a.analyzeTopLevelConst(constDecl); err != nil {
			return err
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
	for _, f := range d.Fields {
		if _, dup := sym.FieldMap[f.Name]; dup {
			return newSemaError(f.Pos, "duplicate field %q in type %q", f.Name, d.Name)
		}
		fType, err := a.resolveTypeExpr(f.Type)
		if err != nil {
			return err
		}
		fs := FieldSymbol{Name: f.Name, Type: fType, Pos: f.Pos}
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
		for _, f := range sym.Fields {
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

func (a *analyzer) structReaches(t Type, target *StructSymbol, visited map[string]bool) bool {
	switch t.Kind {
	case KindList, KindDict, KindMailbox:
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
		sym.Variants = append(sym.Variants, v)
		sym.VariantSet[v] = true
	}
	return nil
}

func (a *analyzer) registerFnDecl(fn *ast.FnDecl) error {
	if existing, exists := a.mod.Functions[fn.Name]; exists {
		return newSemaError(fn.Pos, "function %q is already declared (previous declaration at %s)", fn.Name, existing.Pos.String())
	}
	if existingVar, exists := a.mod.GlobalVars.Lookup(fn.Name); exists {
		return newSemaError(fn.Pos, "function name %q conflicts with constant declared at %s", fn.Name, existingVar.Pos.String())
	}

	var params []ParamSymbol
	seenParams := make(map[string]token.Position)
	for _, p := range fn.Params {
		if prevPos, dup := seenParams[p.Name]; dup {
			return newSemaError(p.Pos, "duplicate parameter %q in function %q (previous at %s)", p.Name, fn.Name, prevPos.String())
		}
		seenParams[p.Name] = p.Pos
		pType, err := a.resolveTypeExpr(p.Type)
		if err != nil {
			return err
		}
		params = append(params, ParamSymbol{
			Name:  p.Name,
			Type:  pType,
			ByRef: p.ByRef,
			Pos:   p.Pos,
		})
	}

	retType := VoidType
	if fn.ReturnType != nil {
		resolvedRet, err := a.resolveTypeExpr(*fn.ReturnType)
		if err != nil {
			return err
		}
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
		GoTarget:   fn.GoTarget,
		Decl:       fn,
		Pos:        fn.Pos,
	}
	return nil
}

func (a *analyzer) analyzeTopLevelConst(d *ast.ConstDecl) error {
	if existingFn, exists := a.mod.Functions[d.Name]; exists {
		return newSemaError(d.Pos, "constant %q conflicts with function declared at %s", d.Name, existingFn.Pos.String())
	}

	valType, err := a.inferExpr(d.Value, a.mod.GlobalVars)
	if err != nil {
		return err
	}
	if valType.IsVoid() {
		return newSemaError(d.Pos, "cannot initialize constant %q with void expression", d.Name)
	}

	finalType := valType
	if d.Type != nil {
		declaredType, err := a.resolveTypeExpr(*d.Type)
		if err != nil {
			return err
		}
		if !declaredType.Equals(valType) {
			return newSemaError(d.Pos, "type mismatch in constant %q declaration: expected %s, got %s", d.Name, declaredType.String(), valType.String())
		}
		finalType = declaredType
	}

	sym := &VarSymbol{
		Name:    d.Name,
		GoName:  a.mod.symbolGoName(d.Name),
		Type:    finalType,
		IsConst: true,
		Public:  d.Public,
		Pos:     d.Pos,
	}
	if err := a.mod.GlobalVars.Define(sym); err != nil {
		return err
	}
	d.SemaType = finalType
	return nil
}

func (a *analyzer) resolveTypeExpr(t ast.TypeExpr) (Type, error) {
	switch t.Kind {
	case ast.TypeBasic:
		switch t.Name {
		case "int":
			return IntType, nil
		case "float":
			return FloatType, nil
		case "str":
			return StrType, nil
		case "bool":
			return BoolType, nil
		case "byte":
			return ByteType, nil
		case "err":
			return ErrType, nil
		}
	case ast.TypeList:
		if len(t.SubTypes) != 1 {
			return VoidType, newSemaError(t.Pos, "list type requires an element type")
		}
		elem, err := a.resolveTypeExpr(t.SubTypes[0])
		if err != nil {
			return VoidType, err
		}
		return Type{Kind: KindList, SubTypes: []Type{elem}}, nil
	case ast.TypeMailbox:
		if len(t.SubTypes) != 1 {
			return VoidType, newSemaError(t.Pos, "mailbox type requires an element type")
		}
		elem, err := a.resolveTypeExpr(t.SubTypes[0])
		if err != nil {
			return VoidType, err
		}
		return Type{Kind: KindMailbox, SubTypes: []Type{elem}}, nil
	case ast.TypeDict:
		if len(t.SubTypes) != 2 {
			return VoidType, newSemaError(t.Pos, "dict type requires key and value types")
		}
		kt, err := a.resolveTypeExpr(t.SubTypes[0])
		if err != nil {
			return VoidType, err
		}
		if !a.isComparableType(kt) {
			return VoidType, newSemaError(t.SubTypes[0].Pos, "dict key type %s is not comparable", kt.String())
		}
		vt, err := a.resolveTypeExpr(t.SubTypes[1])
		if err != nil {
			return VoidType, err
		}
		return Type{Kind: KindDict, SubTypes: []Type{kt, vt}}, nil
	case ast.TypeQualified:
		if len(t.Parts) == 1 {
			name := t.Parts[0]
			if sym, ok := a.mod.Structs[name]; ok {
				return Type{Kind: KindStruct, Name: name, ModuleID: a.mod.ID, GoName: sym.GoName}, nil
			}
			if sym, ok := a.mod.Enums[name]; ok {
				return Type{Kind: KindEnum, Name: name, ModuleID: a.mod.ID, GoName: sym.GoName}, nil
			}
		} else if len(t.Parts) == 2 {
			modAlias, typeName := t.Parts[0], t.Parts[1]
			impMod, ok := a.mod.Imports[modAlias]
			if !ok {
				return VoidType, newSemaError(t.Pos, "unknown module %q in type %q", modAlias, t.String())
			}
			if sym, ok := impMod.Structs[typeName]; ok {
				if !sym.Public {
					return VoidType, newSemaError(t.Pos, "type %q in module %q is private", typeName, modAlias)
				}
				return Type{Kind: KindStruct, Name: typeName, ModuleID: impMod.ID, GoName: sym.GoName}, nil
			}
			if sym, ok := impMod.Enums[typeName]; ok {
				if !sym.Public {
					return VoidType, newSemaError(t.Pos, "enum %q in module %q is private", typeName, modAlias)
				}
				return Type{Kind: KindEnum, Name: typeName, ModuleID: impMod.ID, GoName: sym.GoName}, nil
			}
		}
	}
	return VoidType, newSemaError(t.Pos, "unknown or unsupported type %q", t.String())
}
