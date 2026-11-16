package sema

import (
	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/token"
)

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
		_, err := a.inferExpr(s.Expr, scope)
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

func (a *analyzer) analyzeVarDecl(s *ast.VarDecl, scope *Scope) error {
	var finalType Type
	if s.Type != nil {
		declaredType, err := a.resolveTypeExpr(*s.Type)
		if err != nil {
			return err
		}
		if listLit, ok := s.Value.(*ast.ListLitExpr); ok && len(listLit.Elements) == 0 && declaredType.Kind == KindList {
			a.prog.ExprTypes[listLit] = declaredType
			finalType = declaredType
		} else if dictLit, ok := s.Value.(*ast.DictLitExpr); ok && len(dictLit.Entries) == 0 && declaredType.Kind == KindDict {
			a.prog.ExprTypes[dictLit] = declaredType
			finalType = declaredType
		} else {
			valType, err := a.inferExpr(s.Value, scope)
			if err != nil {
				return err
			}
			if valType.IsVoid() {
				return newSemaError(s.Pos, "cannot initialize variable %q with void expression", s.Name)
			}
			if !declaredType.Equals(valType) {
				return newSemaError(s.Pos, "type mismatch in declaration of %q: expected %s, got %s", s.Name, declaredType.String(), valType.String())
			}
			finalType = declaredType
		}
	} else {
		valType, err := a.inferExpr(s.Value, scope)
		if err != nil {
			return err
		}
		if valType.IsVoid() {
			return newSemaError(s.Pos, "cannot initialize variable %q with void expression", s.Name)
		}
		finalType = valType
	}

	sym := &VarSymbol{
		Name:    s.Name,
		Type:    finalType,
		IsConst: s.IsConst,
		Pos:     s.Pos,
	}
	if err := a.defineLocalVar(scope, sym); err != nil {
		return err
	}
	a.prog.VarTypes[s] = finalType
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
	valType, err := a.inferExpr(s.Value, scope)
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
		a.prog.ExprTypes[target] = sym.Type
		if !sym.Type.Equals(valType) {
			return newSemaError(s.Pos, "type mismatch in assignment to %q: expected %s, got %s", target.Name, sym.Type.String(), valType.String())
		}
		return nil

	case *ast.FieldAccessExpr:
		if err := a.checkMutableTarget(target.Target, scope); err != nil {
			return err
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
		if targetObjType, ok := a.prog.ExprTypes[target.Target]; ok && targetObjType.Kind == KindStr {
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
		switch iterType.Kind {
		case KindList:
			if len(s.Vars) < 1 || len(s.Vars) > 2 {
				return newSemaError(s.Pos, "for-in loop over list supports 1 or 2 loop variables")
			}
			if len(s.Vars) == 1 {
				if err := a.defineLocalVar(loopScope, &VarSymbol{
					Name: s.Vars[0],
					Type: *iterType.ElemType,
					Pos:  s.Pos,
				}); err != nil {
					return err
				}
			} else {
				if err := a.defineLocalVar(loopScope, &VarSymbol{
					Name: s.Vars[0],
					Type: IntType,
					Pos:  s.Pos,
				}); err != nil {
					return err
				}
				if err := a.defineLocalVar(loopScope, &VarSymbol{
					Name: s.Vars[1],
					Type: *iterType.ElemType,
					Pos:  s.Pos,
				}); err != nil {
					return err
				}
			}
		case KindStr:
			if len(s.Vars) < 1 || len(s.Vars) > 2 {
				return newSemaError(s.Pos, "for-in loop over str supports 1 or 2 loop variables")
			}
			if len(s.Vars) == 1 {
				if err := a.defineLocalVar(loopScope, &VarSymbol{
					Name: s.Vars[0],
					Type: StrType,
					Pos:  s.Pos,
				}); err != nil {
					return err
				}
			} else {
				if err := a.defineLocalVar(loopScope, &VarSymbol{
					Name: s.Vars[0],
					Type: IntType,
					Pos:  s.Pos,
				}); err != nil {
					return err
				}
				if err := a.defineLocalVar(loopScope, &VarSymbol{
					Name: s.Vars[1],
					Type: StrType,
					Pos:  s.Pos,
				}); err != nil {
					return err
				}
			}
		case KindDict:
			if len(s.Vars) < 1 || len(s.Vars) > 2 {
				return newSemaError(s.Pos, "for-in loop over dict supports 1 or 2 loop variables")
			}
			if err := a.defineLocalVar(loopScope, &VarSymbol{
				Name: s.Vars[0],
				Type: *iterType.KeyType,
				Pos:  s.Pos,
			}); err != nil {
				return err
			}
			if len(s.Vars) == 2 {
				if err := a.defineLocalVar(loopScope, &VarSymbol{
					Name: s.Vars[1],
					Type: *iterType.ValType,
					Pos:  s.Pos,
				}); err != nil {
					return err
				}
			}
		case KindMailbox:
			if len(s.Vars) != 1 {
				return newSemaError(s.Pos, "for-in loop over mailbox supports exactly 1 loop variable")
			}
			if err := a.defineLocalVar(loopScope, &VarSymbol{
				Name: s.Vars[0],
				Type: *iterType.ElemType,
				Pos:  s.Pos,
			}); err != nil {
				return err
			}
		default:
			return newSemaError(s.Iterable.ExprPos(), "for-in loop requires list, dict, str, or mailbox, got %s", iterType.String())
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

	if s.Step != nil {
		stepType, err := a.inferExpr(s.Step, scope)
		if err != nil {
			return err
		}
		if !stepType.Equals(IntType) {
			return newSemaError(s.Step.ExprPos(), "for loop step expression must be int, got %s", stepType.String())
		}
		if isZeroIntLiteral(s.Step) {
			return newSemaError(s.Step.ExprPos(), "for loop step cannot be 0")
		}
	}

	loopScope := NewScope(scope)
	if err := a.defineLocalVar(loopScope, &VarSymbol{
		Name:    s.VarName,
		Type:    IntType,
		IsConst: false,
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

	valType, err := a.inferExpr(s.Value, scope)
	if err != nil {
		return err
	}
	if !expected.Equals(valType) {
		return newSemaError(s.Pos, "return type mismatch in function %q: expected %s, got %s", a.currentFn.Name, expected.String(), valType.String())
	}
	return nil
}

func isZeroIntLiteral(expr ast.Expr) bool {
	switch ex := expr.(type) {
	case *ast.IntLitExpr:
		return ex.Value == 0
	case *ast.UnaryExpr:
		if ex.Op == token.MINUS || ex.Op == token.PLUS {
			return isZeroIntLiteral(ex.Operand)
		}
	}
	return false
}
