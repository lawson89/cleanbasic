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

func (a *analyzer) inferRootExpr(expr ast.Expr, scope *Scope) (Type, error) {
	prev := a.allowedFallibleExpr
	a.allowedFallibleExpr = expr
	defer func() { a.allowedFallibleExpr = prev }()
	return a.inferExpr(expr, scope)
}

func (a *analyzer) analyzeVarDecl(s *ast.VarDecl, scope *Scope) error {
	finalType, err := a.resolveVarDeclType("variable", s.Name, s.Pos, s.Type, s.Value, true, scope)
	if err != nil {
		return err
	}

	sym := &VarSymbol{
		Name:    s.Name,
		Type:    finalType,
		IsConst: false,
		Pos:     s.Pos,
	}
	if err := scope.Define(sym); err != nil {
		return err
	}
	s.Type = &finalType
	return nil
}

func unwrapMutableDictTarget(expr ast.Expr) ast.Expr {
	for {
		idxExpr, ok := expr.(*ast.IndexExpr)
		if ok && expr.ExprType().Kind == KindDict && idxExpr.Target.ExprType().Kind == KindDict {
			expr = idxExpr.Target
			continue
		}
		break
	}
	return expr
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
		if objIdent, ok := t.Target.(*ast.IdentExpr); ok {
			if impMod, isMod := a.mod.Imports[objIdent.Name]; isMod {
				sym, found := impMod.GlobalVars.Lookup(t.Field)
				if !found {
					return newSemaError(t.Pos, "module %q has no variable %q", objIdent.Name, t.Field)
				}
				if !sym.Public {
					return newSemaError(t.Pos, "variable %q in module %q is private", t.Field, objIdent.Name)
				}
				if sym.IsConst {
					return newSemaError(t.Pos, "cannot mutate constant %q (declared at %s)", t.Field, sym.Pos.String())
				}
				return nil
			}
		}
		if t.Target.ExprType().Kind != KindStruct {
			return newSemaError(t.Pos, "field access target is not an addressable struct")
		}
		return a.checkMutableTarget(t.Target, scope)
	case *ast.IndexExpr:
		if t.Target.ExprType().Kind == KindDict {
			return newSemaError(t.Pos, "cannot mutate %s index expression in-place (only list elements are addressable; assign a modified value back to dict[key] instead)", t.Target.ExprType().String())
		}
		if t.Target.ExprType().Kind != KindList {
			return newSemaError(t.Pos, "cannot mutate %s index expression (only list elements are addressable)", t.Target.ExprType().String())
		}
		return a.checkMutableTarget(t.Target, scope)
	default:
		return newSemaError(expr.ExprPos(), "expression is not an addressable mutable variable, struct field, or list element")
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
		if a.currentFn != nil && sym.IsGlobal {
			a.recordGlobalAccess(a.currentFn, target.Pos, target.Name)
		}
		target.SetExprType(sym.Type)
		if sym.GoName != "" {
			target.GoName = sym.GoName
		}
		target.ByRef = sym.ByRef
		if !sym.Type.Equals(valType) {
			return newSemaError(s.Pos, "type mismatch in assignment to %q: expected %s, got %s", target.Name, sym.Type.String(), valType.String())
		}
		return nil

	case *ast.FieldAccessExpr:
		if targetType, err := a.inferExpr(target.Target, scope); err == nil && targetType.Kind == KindErr {
			if err := a.checkMutableTarget(target.Target, scope); err != nil {
				return err
			}
			return newSemaError(s.Pos, "cannot assign to read-only field %q on err", target.Field)
		}
		fieldType, err := a.inferExpr(target, scope)
		if err != nil {
			return err
		}
		if err := a.checkMutableTarget(target, scope); err != nil {
			return err
		}
		if !fieldType.Equals(valType) {
			return newSemaError(s.Pos, "type mismatch in assignment to field %q: expected %s, got %s", target.Field, fieldType.String(), valType.String())
		}
		return nil

	case *ast.IndexExpr:
		elemType, err := a.inferExpr(target, scope)
		if err != nil {
			return err
		}
		if targetObjType := target.Target.ExprType(); targetObjType.Kind == KindStr {
			return newSemaError(s.Pos, "cannot assign to string index (strings are immutable)")
		}
		mutableBase := unwrapMutableDictTarget(target.Target)
		if err := a.checkMutableTarget(mutableBase, scope); err != nil {
			return err
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
		default:
			return newSemaError(s.Iterable.ExprPos(), "for-in loop requires list, dict, or str, got %s", iterType.String())
		}

		for i, name := range s.Vars {
			if err := loopScope.Define(&VarSymbol{
				Name:    name,
				Type:    varTypes[i],
				IsConst: true,
				Pos:     s.Pos,
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
	if err := loopScope.Define(&VarSymbol{
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
	if err := catchScope.Define(&VarSymbol{
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
