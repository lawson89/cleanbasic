package sema

import "cleanbasic/bootstrap/internal/ast"

func (a *analyzer) finishBuiltinCall(e *ast.CallExpr, recvExpr ast.Expr, methodName string, retType Type, scope *Scope, params ...Type) (Type, error) {
	if err := a.expectPositionalArgs(e, methodName, scope, params...); err != nil {
		return VoidType, err
	}
	e.CallInfo = &CallInfo{
		Kind:       CallMethod,
		MethodName: methodName,
		Receiver:   recvExpr,
		HasValue:   !retType.IsVoid(),
	}
	return retType, nil
}

func (a *analyzer) inferStaticStrCall(e *ast.CallExpr, method string, scope *Scope) (Type, bool, error) {
	listStr := Type{Kind: KindList, SubTypes: []Type{StrType}}
	switch method {
	case "join":
		t, err := a.finishBuiltinCall(e, nil, "str.join", StrType, scope, listStr, StrType)
		return t, true, err
	case "fromCharCode":
		t, err := a.finishBuiltinCall(e, nil, "str.fromCharCode", StrType, scope, IntType)
		return t, true, err
	case "quote":
		t, err := a.finishBuiltinCall(e, nil, "str.quote", StrType, scope, StrType)
		return t, true, err
	default:
		return VoidType, false, nil
	}
}

func (a *analyzer) inferBuiltinMethodCall(e *ast.CallExpr, recvExpr ast.Expr, recvType Type, method string, scope *Scope) (Type, bool, error) {
	switch recvType.Kind {
	case KindList:
		switch method {
		case "len":
			t, err := a.finishBuiltinCall(e, recvExpr, "list.len", IntType, scope)
			return t, true, err
		case "push":
			if err := a.checkMutableTarget(recvExpr, scope); err != nil {
				return VoidType, true, err
			}
			t, err := a.finishBuiltinCall(e, recvExpr, "list.push", VoidType, scope, recvType.Elem())
			return t, true, err
		}
	case KindDict:
		switch method {
		case "len":
			t, err := a.finishBuiltinCall(e, recvExpr, "dict.len", IntType, scope)
			return t, true, err
		case "get":
			t, err := a.finishBuiltinCall(e, recvExpr, "dict.get", recvType.Val(), scope, recvType.Key(), recvType.Val())
			return t, true, err
		case "contains":
			t, err := a.finishBuiltinCall(e, recvExpr, "dict.contains", BoolType, scope, recvType.Key())
			return t, true, err
		case "remove":
			if err := a.checkMutableTarget(recvExpr, scope); err != nil {
				return VoidType, true, err
			}
			t, err := a.finishBuiltinCall(e, recvExpr, "dict.remove", VoidType, scope, recvType.Key())
			return t, true, err
		}
	case KindStr:
		switch method {
		case "len":
			t, err := a.finishBuiltinCall(e, recvExpr, "str.len", IntType, scope)
			return t, true, err
		case "trim":
			t, err := a.finishBuiltinCall(e, recvExpr, "str.trim", StrType, scope)
			return t, true, err
		case "toUpper":
			t, err := a.finishBuiltinCall(e, recvExpr, "str.toUpper", StrType, scope)
			return t, true, err
		case "toLower":
			t, err := a.finishBuiltinCall(e, recvExpr, "str.toLower", StrType, scope)
			return t, true, err
		case "toStr":
			t, err := a.finishBuiltinCall(e, recvExpr, "str.toStr", StrType, scope)
			return t, true, err
		case "toInt":
			t, err := a.finishBuiltinCall(e, recvExpr, "str.toInt", IntType, scope)
			return t, true, err
		case "split":
			listStr := Type{Kind: KindList, SubTypes: []Type{StrType}}
			t, err := a.finishBuiltinCall(e, recvExpr, "str.split", listStr, scope, StrType)
			return t, true, err
		case "contains":
			t, err := a.finishBuiltinCall(e, recvExpr, "str.contains", BoolType, scope, StrType)
			return t, true, err
		case "startsWith":
			t, err := a.finishBuiltinCall(e, recvExpr, "str.startsWith", BoolType, scope, StrType)
			return t, true, err
		case "endsWith":
			t, err := a.finishBuiltinCall(e, recvExpr, "str.endsWith", BoolType, scope, StrType)
			return t, true, err
		case "indexOf":
			t, err := a.finishBuiltinCall(e, recvExpr, "str.indexOf", IntType, scope, StrType)
			return t, true, err
		case "replace":
			t, err := a.finishBuiltinCall(e, recvExpr, "str.replace", StrType, scope, StrType, StrType)
			return t, true, err
		}
	case KindInt:
		switch method {
		case "toByte":
			t, err := a.finishBuiltinCall(e, recvExpr, "int.toByte", ByteType, scope)
			return t, true, err
		case "toStr":
			t, err := a.finishBuiltinCall(e, recvExpr, "int.toStr", StrType, scope)
			return t, true, err
		}
	case KindFloat:
		if method == "toStr" {
			t, err := a.finishBuiltinCall(e, recvExpr, "float.toStr", StrType, scope)
			return t, true, err
		}
	case KindBool:
		if method == "toStr" {
			t, err := a.finishBuiltinCall(e, recvExpr, "bool.toStr", StrType, scope)
			return t, true, err
		}
	case KindByte:
		switch method {
		case "toInt":
			t, err := a.finishBuiltinCall(e, recvExpr, "byte.toInt", IntType, scope)
			return t, true, err
		case "toStr":
			t, err := a.finishBuiltinCall(e, recvExpr, "byte.toStr", StrType, scope)
			return t, true, err
		}
	case KindMailbox:
		switch method {
		case "post":
			t, err := a.finishBuiltinCall(e, recvExpr, "mailbox.post", VoidType, scope, recvType.Elem())
			return t, true, err
		case "get":
			t, err := a.finishBuiltinCall(e, recvExpr, "mailbox.get", recvType.Elem(), scope)
			return t, true, err
		case "close":
			t, err := a.finishBuiltinCall(e, recvExpr, "mailbox.close", VoidType, scope)
			return t, true, err
		case "count":
			t, err := a.finishBuiltinCall(e, recvExpr, "mailbox.count", IntType, scope)
			return t, true, err
		}
	}
	return VoidType, false, nil
}
