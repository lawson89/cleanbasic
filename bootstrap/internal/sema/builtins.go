package sema

import "cleanbasic/bootstrap/internal/ast"

type builtinTypeKind int

const (
	btConcrete builtinTypeKind = iota
	btElem
	btKey
	btVal
)

type builtinTypeRef struct {
	kind     builtinTypeKind
	concrete Type
}

func bt(t Type) builtinTypeRef { return builtinTypeRef{kind: btConcrete, concrete: t} }

var (
	btListStr = bt(Type{Kind: KindList, SubTypes: []Type{StrType}})
	btElemRef = builtinTypeRef{kind: btElem}
	btKeyRef  = builtinTypeRef{kind: btKey}
	btValRef  = builtinTypeRef{kind: btVal}
)

func (r builtinTypeRef) resolve(recv Type) Type {
	switch r.kind {
	case btElem:
		return recv.Elem()
	case btKey:
		return recv.Key()
	case btVal:
		return recv.Val()
	default:
		return r.concrete
	}
}

type builtinMethodSpec struct {
	MethodName      string
	Params          []builtinTypeRef
	Return          builtinTypeRef
	MutatesReceiver bool
}

var staticStrMethods = map[string]builtinMethodSpec{
	"join":         {MethodName: "str.join", Params: []builtinTypeRef{btListStr, bt(StrType)}, Return: bt(StrType)},
	"fromCharCode": {MethodName: "str.fromCharCode", Params: []builtinTypeRef{bt(IntType)}, Return: bt(StrType)},
	"quote":        {MethodName: "str.quote", Params: []builtinTypeRef{bt(StrType)}, Return: bt(StrType)},
}

var instanceBuiltinMethods = map[TypeKind]map[string]builtinMethodSpec{
	KindList: {
		"len":  {MethodName: "list.len", Return: bt(IntType)},
		"push": {MethodName: "list.push", Params: []builtinTypeRef{btElemRef}, Return: bt(VoidType), MutatesReceiver: true},
	},
	KindDict: {
		"len":      {MethodName: "dict.len", Return: bt(IntType)},
		"get":      {MethodName: "dict.get", Params: []builtinTypeRef{btKeyRef, btValRef}, Return: btValRef},
		"contains": {MethodName: "dict.contains", Params: []builtinTypeRef{btKeyRef}, Return: bt(BoolType)},
		"remove":   {MethodName: "dict.remove", Params: []builtinTypeRef{btKeyRef}, Return: bt(VoidType), MutatesReceiver: true},
	},
	KindStr: {
		"len":        {MethodName: "str.len", Return: bt(IntType)},
		"trim":       {MethodName: "str.trim", Return: bt(StrType)},
		"toUpper":    {MethodName: "str.toUpper", Return: bt(StrType)},
		"toLower":    {MethodName: "str.toLower", Return: bt(StrType)},
		"toStr":      {MethodName: "str.toStr", Return: bt(StrType)},
		"toInt":      {MethodName: "str.toInt", Return: bt(IntType)},
		"split":      {MethodName: "str.split", Params: []builtinTypeRef{bt(StrType)}, Return: btListStr},
		"contains":   {MethodName: "str.contains", Params: []builtinTypeRef{bt(StrType)}, Return: bt(BoolType)},
		"startsWith": {MethodName: "str.startsWith", Params: []builtinTypeRef{bt(StrType)}, Return: bt(BoolType)},
		"endsWith":   {MethodName: "str.endsWith", Params: []builtinTypeRef{bt(StrType)}, Return: bt(BoolType)},
		"indexOf":    {MethodName: "str.indexOf", Params: []builtinTypeRef{bt(StrType)}, Return: bt(IntType)},
		"replace":    {MethodName: "str.replace", Params: []builtinTypeRef{bt(StrType), bt(StrType)}, Return: bt(StrType)},
	},
	KindInt: {
		"toByte": {MethodName: "int.toByte", Return: bt(ByteType)},
		"toStr":  {MethodName: "int.toStr", Return: bt(StrType)},
	},
	KindFloat: {
		"toStr": {MethodName: "float.toStr", Return: bt(StrType)},
	},
	KindBool: {
		"toStr": {MethodName: "bool.toStr", Return: bt(StrType)},
	},
	KindByte: {
		"toInt": {MethodName: "byte.toInt", Return: bt(IntType)},
		"toStr": {MethodName: "byte.toStr", Return: bt(StrType)},
	},
	KindMailbox: {
		"post":  {MethodName: "mailbox.post", Params: []builtinTypeRef{btElemRef}, Return: bt(VoidType)},
		"get":   {MethodName: "mailbox.get", Return: btElemRef},
		"close": {MethodName: "mailbox.close", Return: bt(VoidType)},
		"count": {MethodName: "mailbox.count", Return: bt(IntType)},
	},
}

func (a *analyzer) applyBuiltinMethod(e *ast.CallExpr, recvExpr ast.Expr, recvType Type, spec builtinMethodSpec, scope *Scope) (Type, error) {
	if spec.MutatesReceiver {
		if err := a.checkMutableTarget(recvExpr, scope); err != nil {
			return VoidType, err
		}
	}

	var params []Type
	if len(spec.Params) > 0 {
		params = make([]Type, len(spec.Params))
		for i, p := range spec.Params {
			params[i] = p.resolve(recvType)
		}
	}
	retType := spec.Return.resolve(recvType)

	if err := a.expectPositionalArgs(e, spec.MethodName, scope, params...); err != nil {
		return VoidType, err
	}

	e.CallInfo = &CallInfo{
		Kind:       CallMethod,
		MethodName: spec.MethodName,
		Receiver:   recvExpr,
		HasValue:   !retType.IsVoid(),
	}
	return retType, nil
}
