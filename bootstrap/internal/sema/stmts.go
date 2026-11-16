package sema

import "cleanbasic/bootstrap/internal/ast"

func (a *analyzer) analyzeFnBody(fn *ast.FnDecl) error {
	if fn.GoTarget != "" {
		return nil
	}

	fnSym := a.mod.Functions[fn.Name]
	prevFn := a.currentFn
	a.currentFn = fnSym
	defer func() {
		a.currentFn = prevFn
	}()

	fnScope := NewScope(a.mod.GlobalVars)
	for _, p := range fnSym.Params {
		err := fnScope.Define(&VarSymbol{
			Name:    p.Name,
			Type:    p.Type,
			IsConst: false,
			ByRef:   p.ByRef,
			Pos:     p.Pos,
		})
		if err != nil {
			return err
		}
	}

	for _, stmt := range fn.Body {
		if err := a.analyzeStmt(stmt, fnScope); err != nil {
			return err
		}
	}

	if !fnSym.ReturnType.IsVoid() && !blockAlwaysReturns(fn.Body) {
		return newSemaError(fn.Pos, "function %q with return type %s is missing a return statement on all control paths", fn.Name, fnSym.ReturnType.String())
	}

	return nil
}

func blockAlwaysReturns(stmts []ast.Statement) bool {
	for _, st := range stmts {
		switch s := st.(type) {
		case *ast.ReturnStmt:
			return true
		case *ast.IfStmt:
			if !s.HasElse {
				continue
			}
			if !blockAlwaysReturns(s.ThenBody) {
				continue
			}
			allElseIfsReturn := true
			for _, elif := range s.ElseIfs {
				if !blockAlwaysReturns(elif.Body) {
					allElseIfsReturn = false
					break
				}
			}
			if allElseIfsReturn && blockAlwaysReturns(s.ElseBody) {
				return true
			}
		case *ast.SelectStmt:
			if !s.HasElse {
				continue
			}
			allCasesReturn := true
			for _, cc := range s.Cases {
				if !blockAlwaysReturns(cc.Body) {
					allCasesReturn = false
					break
				}
			}
			if allCasesReturn && blockAlwaysReturns(s.ElseBody) {
				return true
			}
		case *ast.TryCatchStmt:
			if blockAlwaysReturns(s.TryBody) && blockAlwaysReturns(s.CatchBody) {
				return true
			}
		case *ast.ExprStmt:
			if call, ok := s.Expr.(*ast.CallExpr); ok {
				if id, ok := call.Callee.(*ast.IdentExpr); ok && id.Name == "error" {
					return true
				}
			}
		}
	}
	return false
}

func (a *analyzer) analyzeStmt(stmt ast.Statement, scope *Scope) error {
	switch s := stmt.(type) {
	case *ast.VarDecl:
		return a.analyzeVarDecl(s, scope)
	case *ast.Assignment:
		return a.analyzeAssignment(s, scope)
	case *ast.ExprStmt:
		_, err := a.inferRootExpr(s.Expr, scope)
		return err
	case *ast.IfStmt:
		return a.analyzeIfStmt(s, scope)
	case *ast.WhileStmt:
		return a.analyzeWhileStmt(s, scope)
	case *ast.ForStmt:
		return a.analyzeForStmt(s, scope)
	case *ast.SelectStmt:
		return a.analyzeSelectStmt(s, scope)
	case *ast.TryCatchStmt:
		return a.analyzeTryCatchStmt(s, scope)
	case *ast.DeferStmt:
		return a.analyzeDeferStmt(s, scope)
	case *ast.StartJobStmt:
		return a.analyzeStartJobStmt(s, scope)
	case *ast.ReturnStmt:
		return a.analyzeReturnStmt(s, scope)
	case *ast.BreakStmt:
		if a.loopDepth <= 0 {
			return newSemaError(s.Pos, "break statement is not inside a loop")
		}
		return nil
	case *ast.ContinueStmt:
		if a.loopDepth <= 0 {
			return newSemaError(s.Pos, "continue statement is not inside a loop")
		}
		return nil
	default:
		return newSemaError(stmt.StmtPos(), "unsupported statement")
	}
}

func (a *analyzer) analyzeStartJobStmt(s *ast.StartJobStmt, scope *Scope) error {
	prevStartJobCall := a.startJobCall
	a.startJobCall = s.Call
	_, err := a.inferExpr(s.Call, scope)
	a.startJobCall = prevStartJobCall
	if err != nil {
		return err
	}
	ci := s.Call.CallInfo
	if ci == nil || (ci.Kind != CallFunc && ci.Kind != CallUFCS) || ci.Fn == nil {
		return newSemaError(s.Pos, "start job target must be a user or module function call")
	}
	if ci.Fn.CanError {
		return newSemaError(s.Pos, "cannot use 'start job' with fallible function %q (handle errors inside the worker function)", ci.Fn.Name)
	}
	for _, p := range ci.Fn.Params {
		if p.ByRef {
			return newSemaError(s.Pos, "cannot use 'start job' with function %q that takes 'byref' parameters", ci.Fn.Name)
		}
	}
	return nil
}

func (a *analyzer) inferRootExpr(expr ast.Expr, scope *Scope) (Type, error) {
	prev := a.allowedFallibleExpr
	a.allowedFallibleExpr = expr
	defer func() { a.allowedFallibleExpr = prev }()
	return a.inferExpr(expr, scope)
}

func (a *analyzer) analyzeVarDecl(s *ast.VarDecl, scope *Scope) error {
	valType, err := a.inferRootExpr(s.Value, scope)
	if err != nil {
		return err
	}
	if valType.IsVoid() {
		return newSemaError(s.Pos, "cannot initialize variable %q with void expression", s.Name)
	}
	finalType := valType
	if s.Type != nil {
		declaredType, err := a.resolveTypeExpr(*s.Type)
		if err != nil {
			return err
		}
		if !declaredType.Equals(valType) {
			return newSemaError(s.Pos, "type mismatch in declaration of %q: expected %s, got %s", s.Name, declaredType.String(), valType.String())
		}
		finalType = declaredType
	}

	sym := &VarSymbol{
		Name:    s.Name,
		Type:    finalType,
		IsConst: false,
		Pos:     s.Pos,
	}
	if err := a.defineLocalVar(scope, sym); err != nil {
		return err
	}
	s.SemaType = finalType
	return nil
}

func (a *analyzer) defineLocalVar(scope *Scope, sym *VarSymbol) error {
	if a.currentFn != nil {
		for _, p := range a.currentFn.Params {
			if p.ByRef && p.Name == sym.Name {
				return newSemaError(sym.Pos, "cannot shadow byref parameter %q", sym.Name)
			}
		}
	}
	return scope.Define(sym)
}

func (a *analyzer) checkMutableTarget(expr ast.Expr, scope *Scope) error {
	switch t := expr.(type) {
	case *ast.IdentExpr:
		sym, found := scope.Lookup(t.Name)
		if !found {
			return newSemaError(t.Pos, "undefined variable %q", t.Name)
		}
		if sym.IsConst {
			return newSemaError(t.Pos, "cannot mutate constant %q (declared at %s)", t.Name, sym.Pos.String())
		}
		return nil
	case *ast.FieldAccessExpr:
		return a.checkMutableTarget(t.Target, scope)
	case *ast.IndexExpr:
		return a.checkMutableTarget(t.Target, scope)
	default:
		return newSemaError(expr.ExprPos(), "target expression is not mutable")
	}
}

func (a *analyzer) analyzeAssignment(s *ast.Assignment, scope *Scope) error {
	valType, err := a.inferRootExpr(s.Value, scope)
	if err != nil {
		return err
	}

	switch target := s.Target.(type) {
	case *ast.IdentExpr:
		sym, found := scope.Lookup(target.Name)
		if !found {
			return newSemaError(target.Pos, "undefined variable %q", target.Name)
		}
		if sym.IsConst {
			return newSemaError(s.Pos, "cannot reassign constant %q (declared at %s)", target.Name, sym.Pos.String())
		}
		target.SetExprType(sym.Type)
		if !sym.Type.Equals(valType) {
			return newSemaError(s.Pos, "type mismatch in assignment to %q: expected %s, got %s", target.Name, sym.Type.String(), valType.String())
		}
		return nil

	case *ast.FieldAccessExpr:
		if err := a.checkMutableTarget(target.Target, scope); err != nil {
			return err
		}
		if targetType, err := a.inferExpr(target.Target, scope); err == nil && targetType.Kind == KindErr {
			return newSemaError(s.Pos, "cannot assign to read-only field %q on err", target.Field)
		}
		fieldType, err := a.inferExpr(target, scope)
		if err != nil {
			return err
		}
		if !fieldType.Equals(valType) {
			return newSemaError(s.Pos, "type mismatch in assignment to field %q: expected %s, got %s", target.Field, fieldType.String(), valType.String())
		}
		return nil

	case *ast.IndexExpr:
		if err := a.checkMutableTarget(target.Target, scope); err != nil {
			return err
		}
		elemType, err := a.inferExpr(target, scope)
		if err != nil {
			return err
		}
		if targetObjType := target.Target.ExprType(); targetObjType.Kind == KindStr {
			return newSemaError(s.Pos, "cannot assign to string index (strings are immutable)")
		}
		if !elemType.Equals(valType) {
			return newSemaError(s.Pos, "type mismatch in index assignment: expected %s, got %s", elemType.String(), valType.String())
		}
		return nil

	default:
		return newSemaError(s.Pos, "invalid assignment target")
	}
}

func (a *analyzer) analyzeIfStmt(s *ast.IfStmt, scope *Scope) error {
	condType, err := a.inferExpr(s.Cond, scope)
	if err != nil {
		return err
	}
	if !condType.Equals(BoolType) {
		return newSemaError(s.Cond.ExprPos(), "if condition must be bool, got %s", condType.String())
	}

	thenScope := NewScope(scope)
	for _, st := range s.ThenBody {
		if err := a.analyzeStmt(st, thenScope); err != nil {
			return err
		}
	}

	for _, elif := range s.ElseIfs {
		elifCondType, err := a.inferExpr(elif.Cond, scope)
		if err != nil {
			return err
		}
		if !elifCondType.Equals(BoolType) {
			return newSemaError(elif.Cond.ExprPos(), "else if condition must be bool, got %s", elifCondType.String())
		}
		elifScope := NewScope(scope)
		for _, st := range elif.Body {
			if err := a.analyzeStmt(st, elifScope); err != nil {
				return err
			}
		}
	}

	if s.HasElse {
		elseScope := NewScope(scope)
		for _, st := range s.ElseBody {
			if err := a.analyzeStmt(st, elseScope); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *analyzer) analyzeWhileStmt(s *ast.WhileStmt, scope *Scope) error {
	condType, err := a.inferExpr(s.Cond, scope)
	if err != nil {
		return err
	}
	if !condType.Equals(BoolType) {
		return newSemaError(s.Cond.ExprPos(), "while condition must be bool, got %s", condType.String())
	}

	bodyScope := NewScope(scope)
	a.loopDepth++
	defer func() {
		a.loopDepth--
	}()
	for _, st := range s.Body {
		if err := a.analyzeStmt(st, bodyScope); err != nil {
			return err
		}
	}
	return nil
}

func (a *analyzer) analyzeForStmt(s *ast.ForStmt, scope *Scope) error {
	if !s.IsRange {
		iterType, err := a.inferExpr(s.Iterable, scope)
		if err != nil {
			return err
		}
		loopScope := NewScope(scope)
		var varTypes []Type
		switch iterType.Kind {
		case KindList, KindStr:
			if len(s.Vars) < 1 || len(s.Vars) > 2 {
				return newSemaError(s.Pos, "for-in loop over %s supports 1 or 2 loop variables", iterType.Kind)
			}
			elemType := StrType
			if iterType.Kind == KindList {
				elemType = iterType.Elem()
			}
			if len(s.Vars) == 1 {
				varTypes = []Type{elemType}
			} else {
				varTypes = []Type{IntType, elemType}
			}
		case KindDict:
			if len(s.Vars) < 1 || len(s.Vars) > 2 {
				return newSemaError(s.Pos, "for-in loop over dict supports 1 or 2 loop variables")
			}
			if len(s.Vars) == 1 {
				varTypes = []Type{iterType.Key()}
			} else {
				varTypes = []Type{iterType.Key(), iterType.Val()}
			}
		case KindMailbox:
			if len(s.Vars) != 1 {
				return newSemaError(s.Pos, "for-in loop over mailbox supports exactly 1 loop variable")
			}
			varTypes = []Type{iterType.Elem()}
		default:
			return newSemaError(s.Iterable.ExprPos(), "for-in loop requires list, dict, str, or mailbox, got %s", iterType.String())
		}

		for i, name := range s.Vars {
			if err := a.defineLocalVar(loopScope, &VarSymbol{
				Name: name,
				Type: varTypes[i],
				Pos:  s.Pos,
			}); err != nil {
				return err
			}
		}

		return a.analyzeLoopBody(s.Body, loopScope)
	}

	startType, err := a.inferExpr(s.Start, scope)
	if err != nil {
		return err
	}
	if !startType.Equals(IntType) {
		return newSemaError(s.Start.ExprPos(), "for loop start expression must be int, got %s", startType.String())
	}

	endType, err := a.inferExpr(s.End, scope)
	if err != nil {
		return err
	}
	if !endType.Equals(IntType) {
		return newSemaError(s.End.ExprPos(), "for loop end expression must be int, got %s", endType.String())
	}

	loopScope := NewScope(scope)
	if err := a.defineLocalVar(loopScope, &VarSymbol{
		Name:    s.VarName,
		Type:    IntType,
		IsConst: true,
		Pos:     s.Pos,
	}); err != nil {
		return err
	}

	return a.analyzeLoopBody(s.Body, loopScope)
}

func (a *analyzer) analyzeLoopBody(stmts []ast.Statement, loopScope *Scope) error {
	a.loopDepth++
	defer func() {
		a.loopDepth--
	}()
	for _, st := range stmts {
		if err := a.analyzeStmt(st, loopScope); err != nil {
			return err
		}
	}
	return nil
}

func (a *analyzer) analyzeSelectStmt(s *ast.SelectStmt, scope *Scope) error {
	exprType, err := a.inferExpr(s.Target, scope)
	if err != nil {
		return err
	}
	if !a.isComparableType(exprType) {
		return newSemaError(s.Target.ExprPos(), "select case expression of type %s is not comparable", exprType.String())
	}

	for _, cc := range s.Cases {
		for _, pat := range cc.Values {
			patType, err := a.inferExpr(pat, scope)
			if err != nil {
				return err
			}
			if !patType.Equals(exprType) {
				return newSemaError(pat.ExprPos(), "case pattern type %s does not match select expression type %s", patType.String(), exprType.String())
			}
		}
		caseScope := NewScope(scope)
		for _, st := range cc.Body {
			if err := a.analyzeStmt(st, caseScope); err != nil {
				return err
			}
		}
	}

	if s.HasElse {
		elseScope := NewScope(scope)
		for _, st := range s.ElseBody {
			if err := a.analyzeStmt(st, elseScope); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *analyzer) analyzeTryCatchStmt(s *ast.TryCatchStmt, scope *Scope) error {
	tryScope := NewScope(scope)
	a.tryDepth++
	for _, st := range s.TryBody {
		if err := a.analyzeStmt(st, tryScope); err != nil {
			a.tryDepth--
			return err
		}
	}
	a.tryDepth--

	catchScope := NewScope(scope)
	if err := a.defineLocalVar(catchScope, &VarSymbol{
		Name:    s.CatchVar,
		Type:    ErrType,
		IsConst: true,
		Pos:     s.Pos,
	}); err != nil {
		return err
	}
	for _, st := range s.CatchBody {
		if err := a.analyzeStmt(st, catchScope); err != nil {
			return err
		}
	}
	return nil
}

func (a *analyzer) analyzeDeferStmt(s *ast.DeferStmt, scope *Scope) error {
	prevTryDepth := a.tryDepth
	prevLoopDepth := a.loopDepth
	a.tryDepth = 0
	a.loopDepth = 0
	a.deferDepth++
	defer func() {
		a.tryDepth = prevTryDepth
		a.loopDepth = prevLoopDepth
		a.deferDepth--
	}()

	deferScope := NewScope(scope)
	for _, st := range s.Body {
		if err := a.analyzeStmt(st, deferScope); err != nil {
			return err
		}
	}
	return nil
}

func (a *analyzer) analyzeReturnStmt(s *ast.ReturnStmt, scope *Scope) error {
	if a.deferDepth > 0 {
		return newSemaError(s.Pos, "return statement is not allowed inside a defer block")
	}
	if a.currentFn == nil {
		return newSemaError(s.Pos, "return statement outside of function")
	}

	expected := a.currentFn.ReturnType
	if expected.IsVoid() {
		if s.Value != nil {
			valType, err := a.inferExpr(s.Value, scope)
			if err != nil {
				return err
			}
			return newSemaError(s.Pos, "function %q has void return type, cannot return %s", a.currentFn.Name, valType.String())
		}
		return nil
	}

	if s.Value == nil {
		return newSemaError(s.Pos, "function %q expects return value of type %s, got empty return", a.currentFn.Name, expected.String())
	}

	valType, err := a.inferRootExpr(s.Value, scope)
	if err != nil {
		return err
	}
	if !expected.Equals(valType) {
		return newSemaError(s.Pos, "return type mismatch in function %q: expected %s, got %s", a.currentFn.Name, expected.String(), valType.String())
	}
	return nil
}
