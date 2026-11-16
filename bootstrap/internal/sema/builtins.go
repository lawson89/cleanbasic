package sema

import "cleanbasic/bootstrap/internal/ast"

type builtinTypeKind int

const (
	btConcrete builtinTypeKind = iota
	btElem
	btKey
	btVal
	btJobResult
)

type builtinTypeRef struct {
	kind     builtinTypeKind
	concrete Type
}

func bt(t Type) builtinTypeRef { return builtinTypeRef{kind: btConcrete, concrete: t} }

var (
	btListStr      = bt(Type{Kind: KindList, ElemType: &StrType})
	btListByte     = bt(Type{Kind: KindList, ElemType: &ByteType})
	btElemRef      = builtinTypeRef{kind: btElem}
	btKeyRef       = builtinTypeRef{kind: btKey}
	btValRef       = builtinTypeRef{kind: btVal}
	btJobResultRef = builtinTypeRef{kind: btJobResult}
)

func (r builtinTypeRef) resolve(recv Type) Type {
	switch r.kind {
	case btElem:
		return *recv.ElemType
	case btKey:
		return *recv.KeyType
	case btVal:
		return *recv.ValType
	case btJobResult:
		if recv.ElemType != nil {
			return *recv.ElemType
		}
		return VoidType
	default:
		return r.concrete
	}
}

type builtinMethodSpec struct {
	MethodName      string
	Params          []builtinTypeRef
	Return          builtinTypeRef
	MutatesReceiver bool
	FirstArgByRef   bool
	CanError        bool
}

var staticStrMethods = map[string]builtinMethodSpec{
	"join":         {MethodName: "str.join", Params: []builtinTypeRef{btListStr, bt(StrType)}, Return: bt(StrType)},
	"fromCharCode": {MethodName: "str.fromCharCode", Params: []builtinTypeRef{bt(IntType)}, Return: bt(StrType)},
	"quote":        {MethodName: "str.quote", Params: []builtinTypeRef{bt(StrType)}, Return: bt(StrType)},
	"fromBytes":    {MethodName: "str.fromBytes", Params: []builtinTypeRef{btListByte}, Return: bt(StrType)},
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
		"toBytes":    {MethodName: "str.toBytes", Return: btListByte},
	},
	KindInt: {
		"toByte": {MethodName: "int.toByte", Return: bt(ByteType)},
	},
	KindByte: {
		"toInt": {MethodName: "byte.toInt", Return: bt(IntType)},
	},
	KindMailbox: {
		"post":  {MethodName: "mailbox.post", Params: []builtinTypeRef{btElemRef}, Return: bt(VoidType), CanError: true},
		"check": {MethodName: "mailbox.check", Params: []builtinTypeRef{btElemRef}, Return: bt(BoolType), FirstArgByRef: true},
		"get":   {MethodName: "mailbox.get", Return: btElemRef, CanError: true},
		"close": {MethodName: "mailbox.close", Return: bt(VoidType)},
		"count": {MethodName: "mailbox.count", Return: bt(IntType)},
	},
	KindJob: {
		"wait":   {MethodName: "job.wait", Return: btJobResultRef, CanError: true},
		"isDone": {MethodName: "job.isDone", Return: bt(BoolType)},
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
	firstArgByRef := spec.FirstArgByRef

	if err := a.expectPositionalArgs(e, spec.MethodName, scope, params...); err != nil {
		return VoidType, err
	}
	if firstArgByRef {
		if err := a.checkByRefArg(e.Args[0], 1, "out", spec.MethodName, scope); err != nil {
			return VoidType, err
		}
	}
	if spec.CanError {
		if !a.inFallibleContext() {
			return VoidType, newSemaError(e.Pos, "call to 'can error' method '%s' must be inside a 'try' block or a 'can error' function", spec.MethodName)
		}
		if e != a.allowedFallibleExpr && e != a.startJobCall {
			return VoidType, newSemaError(e.Pos, "call to 'can error' method '%s' may only appear as a top-level statement expression", spec.MethodName)
		}
	}

	a.prog.CallInfos[e] = &CallInfo{
		Kind:       CallMethod,
		MethodName: spec.MethodName,
		Receiver:   recvExpr,
		CanError:   spec.CanError,
		HasValue:   !retType.IsVoid(),
	}
	return retType, nil
}
