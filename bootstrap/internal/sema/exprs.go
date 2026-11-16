package sema

import (
	"fmt"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/token"
)

func (a *analyzer) inferExpr(expr ast.Expr, scope *Scope) (Type, error) {
	var resolved Type
	var err error

	switch e := expr.(type) {
	case *ast.IntLitExpr:
		resolved = IntType
	case *ast.FloatLitExpr:
		resolved = FloatType
	case *ast.StringLitExpr:
		resolved = StrType
	case *ast.BoolLitExpr:
		resolved = BoolType
	case *ast.IdentExpr:
		sym, ok := scope.Lookup(e.Name)
		if !ok {
			return VoidType, newSemaError(e.Pos, "undefined identifier %q", e.Name)
		}
		if sym.GoName != "" {
			a.prog.IdentGoNames[e] = sym.GoName
		}
		resolved = sym.Type
	case *ast.UnaryExpr:
		resolved, err = a.inferUnaryExpr(e, scope)
	case *ast.BinaryExpr:
		resolved, err = a.inferBinaryExpr(e, scope)
	case *ast.ListLitExpr:
		resolved, err = a.inferListLitExpr(e, scope)
	case *ast.DictLitExpr:
		resolved, err = a.inferDictLitExpr(e, scope)
	case *ast.TypedEmptyCtorExpr:
		resolved, err = a.inferTypedEmptyCtorExpr(e, scope)
	case *ast.FieldAccessExpr:
		resolved, err = a.inferFieldAccessExpr(e, scope)
	case *ast.IndexExpr:
		resolved, err = a.inferIndexExpr(e, scope)
	case *ast.SliceExpr:
		resolved, err = a.inferSliceExpr(e, scope)
	case *ast.CallExpr:
		resolved, err = a.inferCallExpr(e, scope)
	default:
		return VoidType, newSemaError(expr.ExprPos(), "unsupported expression")
	}

	if err != nil {
		return VoidType, err
	}
	a.prog.ExprTypes[expr] = resolved
	return resolved, nil
}

func (a *analyzer) inferUnaryExpr(e *ast.UnaryExpr, scope *Scope) (Type, error) {
	opType, err := a.inferExpr(e.Operand, scope)
	if err != nil {
		return VoidType, err
	}

	switch e.Op {
	case token.NOT:
		if !opType.Equals(BoolType) {
			return VoidType, newSemaError(e.Pos, "unary operator %q requires bool operand, got %s", e.Op, opType.String())
		}
		return BoolType, nil
	case token.PLUS, token.MINUS:
		if !opType.Equals(IntType) && !opType.Equals(FloatType) {
			return VoidType, newSemaError(e.Pos, "unary operator %q requires numeric (int or float) operand, got %s", e.Op, opType.String())
		}
		return opType, nil
	default:
		return VoidType, newSemaError(e.Pos, "unknown unary operator %q", e.Op)
	}
}

func (a *analyzer) inferBinaryExpr(e *ast.BinaryExpr, scope *Scope) (Type, error) {
	leftType, err := a.inferExpr(e.Left, scope)
	if err != nil {
		return VoidType, err
	}
	rightType, err := a.inferExpr(e.Right, scope)
	if err != nil {
		return VoidType, err
	}

	switch e.Op {
	case token.PLUS, token.MINUS, token.STAR, token.SLASH:
		if !leftType.Equals(rightType) {
			return VoidType, newSemaError(e.Pos, "type mismatch for operator %q: %s and %s", e.Op, leftType.String(), rightType.String())
		}
		if e.Op == token.PLUS && leftType.Equals(StrType) {
			return StrType, nil
		}
		if !leftType.Equals(IntType) && !leftType.Equals(FloatType) && !leftType.Equals(ByteType) {
			if e.Op == token.PLUS {
				return VoidType, newSemaError(e.Pos, "operator %q requires int, float, byte, or str operands, got %s", e.Op, leftType.String())
			}
			return VoidType, newSemaError(e.Pos, "operator %q requires int, float, or byte operands, got %s", e.Op, leftType.String())
		}
		return leftType, nil

	case token.PERCENT:
		if !leftType.Equals(rightType) || (!leftType.Equals(IntType) && !leftType.Equals(ByteType)) {
			return VoidType, newSemaError(e.Pos, "modulo operator %q requires int or byte operands, got %s and %s", e.Op, leftType.String(), rightType.String())
		}
		return leftType, nil

	case token.EQ, token.NEQ:
		if !leftType.Equals(rightType) || leftType.IsVoid() {
			return VoidType, newSemaError(e.Pos, "cannot compare %s and %s with %q", leftType.String(), rightType.String(), e.Op)
		}
		if !a.isComparableType(leftType) {
			return VoidType, newSemaError(e.Pos, "type %s cannot be compared with %q", leftType.String(), e.Op)
		}
		return BoolType, nil

	case token.LT, token.GT, token.LTE, token.GTE:
		if !leftType.Equals(rightType) {
			return VoidType, newSemaError(e.Pos, "cannot compare %s and %s with %q", leftType.String(), rightType.String(), e.Op)
		}
		if !leftType.Equals(IntType) && !leftType.Equals(FloatType) && !leftType.Equals(StrType) && !leftType.Equals(ByteType) {
			return VoidType, newSemaError(e.Pos, "relational operator %q is not supported for type %s", e.Op, leftType.String())
		}
		return BoolType, nil

	case token.AND, token.OR:
		if !leftType.Equals(BoolType) || !rightType.Equals(BoolType) {
			return VoidType, newSemaError(e.Pos, "logical operator %q requires bool operands, got %s and %s", e.Op, leftType.String(), rightType.String())
		}
		return BoolType, nil

	default:
		return VoidType, newSemaError(e.Pos, "unsupported binary operator %q", e.Op)
	}
}

func (a *analyzer) inferListLitExpr(e *ast.ListLitExpr, scope *Scope) (Type, error) {
	firstType, err := a.inferExpr(e.Elements[0], scope)
	if err != nil {
		return VoidType, err
	}
	for i := 1; i < len(e.Elements); i++ {
		elemType, err := a.inferExpr(e.Elements[i], scope)
		if err != nil {
			return VoidType, err
		}
		if !firstType.Equals(elemType) {
			return VoidType, newSemaError(e.Elements[i].ExprPos(), "list literal element %d has type %s, expected %s", i, elemType.String(), firstType.String())
		}
	}
	return Type{Kind: KindList, SubTypes: []Type{firstType}}, nil
}

func (a *analyzer) inferDictLitExpr(e *ast.DictLitExpr, scope *Scope) (Type, error) {
	firstKey, err := a.inferExpr(e.Entries[0].Key, scope)
	if err != nil {
		return VoidType, err
	}
	if !a.isComparableType(firstKey) {
		return VoidType, newSemaError(e.Entries[0].Key.ExprPos(), "dict key type %s is not comparable", firstKey.String())
	}
	firstVal, err := a.inferExpr(e.Entries[0].Value, scope)
	if err != nil {
		return VoidType, err
	}
	for i := 1; i < len(e.Entries); i++ {
		kt, err := a.inferExpr(e.Entries[i].Key, scope)
		if err != nil {
			return VoidType, err
		}
		if !firstKey.Equals(kt) {
			return VoidType, newSemaError(e.Entries[i].Key.ExprPos(), "dict literal key %d has type %s, expected %s", i, kt.String(), firstKey.String())
		}
		vt, err := a.inferExpr(e.Entries[i].Value, scope)
		if err != nil {
			return VoidType, err
		}
		if !firstVal.Equals(vt) {
			return VoidType, newSemaError(e.Entries[i].Value.ExprPos(), "dict literal value %d has type %s, expected %s", i, vt.String(), firstVal.String())
		}
	}
	return Type{Kind: KindDict, SubTypes: []Type{firstKey, firstVal}}, nil
}

func (a *analyzer) inferTypedEmptyCtorExpr(e *ast.TypedEmptyCtorExpr, scope *Scope) (Type, error) {
	t, err := a.resolveTypeExpr(e.Type)
	if err != nil {
		return VoidType, err
	}
	switch t.Kind {
	case KindList, KindDict:
		if len(e.Args) > 0 {
			return VoidType, newSemaError(e.Pos, "%s() takes no arguments", t.String())
		}
		return t, nil
	case KindMailbox:
		if len(e.Args) > 1 {
			return VoidType, newSemaError(e.Pos, "mailbox constructor takes at most 1 capacity argument, got %d", len(e.Args))
		}
		if len(e.Args) == 1 {
			capType, err := a.inferExpr(e.Args[0], scope)
			if err != nil {
				return VoidType, err
			}
			if !capType.Equals(IntType) {
				return VoidType, newSemaError(e.Args[0].ExprPos(), "mailbox capacity must be int, got %s", capType.String())
			}
		}
		return t, nil
	default:
		return VoidType, newSemaError(e.Pos, "typed container constructor is only supported for list, dict, and mailbox, got %s", t.String())
	}
}

func (a *analyzer) inferFieldAccessExpr(e *ast.FieldAccessExpr, scope *Scope) (Type, error) {
	if objIdent, ok := e.Target.(*ast.IdentExpr); ok {
		if enumSym, isEnum := a.mod.Enums[objIdent.Name]; isEnum {
			if !enumSym.VariantSet[e.Field] {
				return VoidType, newSemaError(e.Pos, "enum %q has no variant %q", enumSym.Name, e.Field)
			}
			a.prog.IdentGoNames[e] = fmt.Sprintf("%s_%s", enumSym.GoName, e.Field)
			return Type{Kind: KindEnum, Name: enumSym.Name, ModuleID: enumSym.ModuleID, GoName: enumSym.GoName}, nil
		}
		if impMod, isMod := a.mod.Imports[objIdent.Name]; isMod {
			sym, found := impMod.GlobalVars.Lookup(e.Field)
			if !found {
				return VoidType, newSemaError(e.Pos, "module %q has no constant %q", objIdent.Name, e.Field)
			}
			if !sym.Public {
				return VoidType, newSemaError(e.Pos, "constant %q in module %q is private", e.Field, objIdent.Name)
			}
			a.prog.IdentGoNames[e] = sym.GoName
			return sym.Type, nil
		}
	}
	if innerFA, ok := e.Target.(*ast.FieldAccessExpr); ok {
		if modIdent, ok2 := innerFA.Target.(*ast.IdentExpr); ok2 {
			if impMod, isMod := a.mod.Imports[modIdent.Name]; isMod {
				if enumSym, isEnum := impMod.Enums[innerFA.Field]; isEnum {
					if !enumSym.Public {
						return VoidType, newSemaError(e.Pos, "enum %q in module %q is private", innerFA.Field, modIdent.Name)
					}
					if !enumSym.VariantSet[e.Field] {
						return VoidType, newSemaError(e.Pos, "enum %q has no variant %q", enumSym.Name, e.Field)
					}
					a.prog.IdentGoNames[e] = fmt.Sprintf("%s_%s", enumSym.GoName, e.Field)
					return Type{Kind: KindEnum, Name: enumSym.Name, ModuleID: enumSym.ModuleID, GoName: enumSym.GoName}, nil
				}
				if _, isGlobal := impMod.GlobalVars.Lookup(innerFA.Field); !isGlobal {
					return VoidType, newSemaError(e.Pos, "module %q has no enum %q", modIdent.Name, innerFA.Field)
				}
			}
		}
	}
	objType, err := a.inferExpr(e.Target, scope)
	if err != nil {
		return VoidType, err
	}
	if objType.Kind == KindErr {
		if e.Field == "msg" {
			return StrType, nil
		}
		return VoidType, newSemaError(e.Pos, "err type has no field %q", e.Field)
	}
	if objType.Kind != KindStruct {
		return VoidType, newSemaError(e.Pos, "cannot access field %q on non-struct type %s", e.Field, objType.String())
	}
	structSym := a.lookupStruct(objType)
	if structSym == nil {
		return VoidType, newSemaError(e.Pos, "unknown struct type %s", objType.String())
	}
	fs, exists := structSym.FieldMap[e.Field]
	if !exists {
		return VoidType, newSemaError(e.Pos, "struct %q has no field %q", structSym.Name, e.Field)
	}
	return fs.Type, nil
}

func (a *analyzer) inferIndexExpr(e *ast.IndexExpr, scope *Scope) (Type, error) {
	objType, err := a.inferExpr(e.Target, scope)
	if err != nil {
		return VoidType, err
	}
	idxType, err := a.inferExpr(e.Index, scope)
	if err != nil {
		return VoidType, err
	}
	switch objType.Kind {
	case KindList:
		if !idxType.Equals(IntType) {
			return VoidType, newSemaError(e.Index.ExprPos(), "list index must be int, got %s", idxType.String())
		}
		return objType.Elem(), nil
	case KindDict:
		if !idxType.Equals(objType.Key()) {
			return VoidType, newSemaError(e.Index.ExprPos(), "dict key must be %s, got %s", objType.Key().String(), idxType.String())
		}
		return objType.Val(), nil
	case KindStr:
		if !idxType.Equals(IntType) {
			return VoidType, newSemaError(e.Index.ExprPos(), "str index must be int, got %s", idxType.String())
		}
		return ByteType, nil
	default:
		return VoidType, newSemaError(e.Pos, "cannot index into type %s", objType.String())
	}
}

func (a *analyzer) inferSliceExpr(e *ast.SliceExpr, scope *Scope) (Type, error) {
	objType, err := a.inferExpr(e.Target, scope)
	if err != nil {
		return VoidType, err
	}
	if objType.Kind != KindStr && objType.Kind != KindList {
		return VoidType, newSemaError(e.Pos, "cannot slice type %s (expected str or list)", objType.String())
	}
	if e.Start != nil {
		stType, err := a.inferExpr(e.Start, scope)
		if err != nil {
			return VoidType, err
		}
		if !stType.Equals(IntType) {
			return VoidType, newSemaError(e.Start.ExprPos(), "slice start index must be int, got %s", stType.String())
		}
	}
	if e.End != nil {
		enType, err := a.inferExpr(e.End, scope)
		if err != nil {
			return VoidType, err
		}
		if !enType.Equals(IntType) {
			return VoidType, newSemaError(e.End.ExprPos(), "slice end index must be int, got %s", enType.String())
		}
	}
	return objType, nil
}

func (a *analyzer) inferCallExpr(e *ast.CallExpr, scope *Scope) (Type, error) {
	if fa, ok := e.Callee.(*ast.FieldAccessExpr); ok {
		return a.inferMethodOrUFCSCall(e, fa, scope)
	}
	calleeIdent, ok := e.Callee.(*ast.IdentExpr)
	if !ok {
		return VoidType, newSemaError(e.Pos, "unsupported call target")
	}

	if calleeIdent.Name == "error" {
		return a.inferErrorCall(e, scope)
	}

	if structSym, isStruct := a.mod.Structs[calleeIdent.Name]; isStruct {
		return a.inferStructCtorCall(e, structSym, scope)
	}

	fnSym, exists := a.mod.Functions[calleeIdent.Name]
	if !exists {
		return VoidType, newSemaError(calleeIdent.Pos, "undefined function %q", calleeIdent.Name)
	}

	if fnSym.IsVariadic {
		for _, arg := range e.Args {
			argType, err := a.inferExpr(arg, scope)
			if err != nil {
				return VoidType, err
			}
			if argType.IsVoid() {
				return VoidType, newSemaError(arg.ExprPos(), "argument to %q cannot be void", fnSym.Name)
			}
		}
		return fnSym.ReturnType, nil
	}

	if err := a.validateCallArgs(e, e.Args, fnSym.Params, fnSym.Name, fnSym.CanError, "function", scope); err != nil {
		return VoidType, err
	}

	a.prog.CallInfos[e] = &CallInfo{
		Kind:     CallFunc,
		Fn:       fnSym,
		CanError: fnSym.CanError,
		HasValue: !fnSym.ReturnType.IsVoid(),
	}
	return fnSym.ReturnType, nil
}

func (a *analyzer) inFallibleContext() bool {
	return a.tryDepth > 0 || (a.deferDepth == 0 && a.currentFn != nil && a.currentFn.CanError)
}

func (a *analyzer) inferErrorCall(e *ast.CallExpr, scope *Scope) (Type, error) {
	if e != a.startJobCall {
		if !a.inFallibleContext() {
			return VoidType, newSemaError(e.Pos, "error() can only be called inside a 'can error' function or a 'try' block")
		}
		if e != a.allowedFallibleExpr {
			return VoidType, newSemaError(e.Pos, "error() may only appear as a top-level statement expression")
		}
	}
	if len(e.Args) != 1 {
		return VoidType, newSemaError(e.Pos, "error() expects 1 argument, got %d", len(e.Args))
	}
	msgType, err := a.inferExpr(e.Args[0], scope)
	if err != nil {
		return VoidType, err
	}
	if !msgType.Equals(StrType) {
		return VoidType, newSemaError(e.Args[0].ExprPos(), "error() message must be str, got %s", msgType.String())
	}
	a.prog.CallInfos[e] = &CallInfo{Kind: CallError}
	return VoidType, nil
}

func (a *analyzer) inferStructCtorCall(e *ast.CallExpr, s *StructSymbol, scope *Scope) (Type, error) {
	retType := Type{Kind: KindStruct, Name: s.Name, ModuleID: s.ModuleID, GoName: s.GoName}
	if len(e.Args) != 0 && len(e.Args) != len(s.Fields) {
		return VoidType, newSemaError(e.Pos, "struct %q constructor expects 0 or %d argument(s), got %d", s.Name, len(s.Fields), len(e.Args))
	}
	for i, arg := range e.Args {
		argType, err := a.inferExpr(arg, scope)
		if err != nil {
			return VoidType, err
		}
		if !s.Fields[i].Type.Equals(argType) {
			return VoidType, newSemaError(arg.ExprPos(), "argument %d (%q) to %q constructor expects %s, got %s", i+1, s.Fields[i].Name, s.Name, s.Fields[i].Type.String(), argType.String())
		}
	}
	a.prog.CallInfos[e] = &CallInfo{Kind: CallStructCtor, Struct: s}
	return retType, nil
}

func (a *analyzer) checkByRefArg(val ast.Expr, argIdx int, paramName, fnName string, scope *Scope) error {
	id, ok := val.(*ast.IdentExpr)
	if !ok {
		return newSemaError(val.ExprPos(), "argument %d (%q) to byref parameter of %q must be a mutable variable", argIdx, paramName, fnName)
	}
	return a.checkMutableTarget(id, scope)
}

func (a *analyzer) validateCallArgs(callExpr *ast.CallExpr, args []ast.Expr, params []ParamSymbol, fnName string, canError bool, callKindDesc string, scope *Scope) error {
	if len(args) != len(params) {
		return newSemaError(callExpr.Pos, "%s %q expects %d argument(s), got %d", callKindDesc, fnName, len(params), len(args))
	}
	for i, arg := range args {
		param := params[i]
		argType, err := a.inferExpr(arg, scope)
		if err != nil {
			return err
		}
		if param.Type.Kind == KindAny {
			if argType.IsVoid() {
				return newSemaError(arg.ExprPos(), "argument %d (%q) to function %q cannot be void", i+1, param.Name, fnName)
			}
		} else if !param.Type.Equals(argType) {
			return newSemaError(arg.ExprPos(), "argument %d (%q) to function %q has type %s, expected %s",
				i+1, param.Name, fnName, argType.String(), param.Type.String())
		}
		if param.ByRef {
			if err := a.checkByRefArg(arg, i+1, param.Name, fnName, scope); err != nil {
				return err
			}
		}
	}
	if canError && callExpr != a.startJobCall {
		if !a.inFallibleContext() {
			return newSemaError(callExpr.Pos, "call to 'can error' function %q must be inside a 'try' block or a 'can error' function", fnName)
		}
		if callExpr != a.allowedFallibleExpr {
			return newSemaError(callExpr.Pos, "call to 'can error' function %q may only appear as a top-level statement expression", fnName)
		}
	}
	return nil
}

func (a *analyzer) expectPositionalArgs(e *ast.CallExpr, methodName string, scope *Scope, expected ...Type) error {
	if len(e.Args) != len(expected) {
		return newSemaError(e.Pos, "%s() expects %d argument(s), got %d", methodName, len(expected), len(e.Args))
	}
	for i, expType := range expected {
		at, err := a.inferExpr(e.Args[i], scope)
		if err != nil {
			return err
		}
		if !at.Equals(expType) {
			return newSemaError(e.Args[i].ExprPos(), "%s() argument %d must be %s, got %s", methodName, i+1, expType.String(), at.String())
		}
	}
	return nil
}

func (a *analyzer) inferMethodOrUFCSCall(e *ast.CallExpr, fa *ast.FieldAccessExpr, scope *Scope) (Type, error) {
	if objIdent, ok := fa.Target.(*ast.IdentExpr); ok {
		// Cross-module function or struct constructor: modAlias.Symbol(...)
		if impMod, isMod := a.mod.Imports[objIdent.Name]; isMod {
			if structSym, isStruct := impMod.Structs[fa.Field]; isStruct {
				if !structSym.Public {
					return VoidType, newSemaError(e.Pos, "type %q in module %q is private", fa.Field, objIdent.Name)
				}
				return a.inferStructCtorCall(e, structSym, scope)
			}
			if fnSym, isFn := impMod.Functions[fa.Field]; isFn {
				if !fnSym.Public {
					return VoidType, newSemaError(e.Pos, "function %q in module %q is private", fa.Field, objIdent.Name)
				}
				if err := a.validateCallArgs(e, e.Args, fnSym.Params, fnSym.Name, fnSym.CanError, "function", scope); err != nil {
					return VoidType, err
				}
				a.prog.CallInfos[e] = &CallInfo{
					Kind:     CallFunc,
					Fn:       fnSym,
					CanError: fnSym.CanError,
					HasValue: !fnSym.ReturnType.IsVoid(),
				}
				return fnSym.ReturnType, nil
			}
			return VoidType, newSemaError(e.Pos, "module %q has no symbol %q", objIdent.Name, fa.Field)
		}

		if objIdent.Name == "str" {
			if spec, ok := staticStrMethods[fa.Field]; ok {
				return a.applyBuiltinMethod(e, nil, VoidType, spec, scope)
			}
		}
	}

	recvType, err := a.inferExpr(fa.Target, scope)
	if err != nil {
		return VoidType, err
	}

	if methods, ok := instanceBuiltinMethods[recvType.Kind]; ok {
		if spec, ok := methods[fa.Field]; ok {
			return a.applyBuiltinMethod(e, fa.Target, recvType, spec, scope)
		}
	}

	// UFCS fallback: check current module first, then public functions in receiver type's defining module
	var fnSym *FnSymbol
	if cand, ok := a.mod.Functions[fa.Field]; ok && len(cand.Params) > 0 && cand.Params[0].Type.Equals(recvType) {
		fnSym = cand
	} else if recvType.ModuleID != "" && recvType.ModuleID != a.mod.ID {
		if defMod, ok := a.prog.ModuleByID[recvType.ModuleID]; ok {
			if cand, ok := defMod.Functions[fa.Field]; ok && cand.Public && len(cand.Params) > 0 && cand.Params[0].Type.Equals(recvType) {
				fnSym = cand
			}
		}
	}
	if fnSym == nil {
		return VoidType, newSemaError(e.Pos, "no method or UFCS function %q for receiver type %s", fa.Field, recvType.String())
	}

	if fnSym.Params[0].ByRef {
		if err := a.checkMutableTarget(fa.Target, scope); err != nil {
			return VoidType, err
		}
		if _, isIdent := fa.Target.(*ast.IdentExpr); !isIdent {
			return VoidType, newSemaError(fa.Target.ExprPos(), "receiver for byref UFCS function %q must be a mutable variable", fnSym.Name)
		}
	}

	if err := a.validateCallArgs(e, e.Args, fnSym.Params[1:], fnSym.Name, fnSym.CanError, "UFCS call", scope); err != nil {
		return VoidType, err
	}

	a.prog.CallInfos[e] = &CallInfo{
		Kind:     CallUFCS,
		Fn:       fnSym,
		Receiver: fa.Target,
		CanError: fnSym.CanError,
		HasValue: !fnSym.ReturnType.IsVoid(),
	}
	return fnSym.ReturnType, nil
}
