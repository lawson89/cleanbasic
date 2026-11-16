package codegen

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/sema"
	"cleanbasic/bootstrap/internal/token"
)

func (e *emitter) emitStmtList(stmts []ast.Statement, indent string, sb *strings.Builder) {
	for _, st := range stmts {
		e.emitStmt(sb, st, indent)
	}
}

func (e *emitter) emitErrorJumpOrReturn(sb *strings.Builder, indent, errExpr string) {
	if e.currentTryID > 0 {
		if e.tryUsedCatch == nil {
			e.tryUsedCatch = make(map[int]bool)
		}
		e.tryUsedCatch[e.currentTryID] = true
		sb.WriteString(fmt.Sprintf("%s_tryErr_%d = %s\n", indent, e.currentTryID, errExpr))
		sb.WriteString(fmt.Sprintf("%sgoto _catch_%d\n", indent, e.currentTryID))
		return
	}
	if e.currentFnSym != nil && !e.currentFnSym.ReturnType.IsVoid() {
		sb.WriteString(fmt.Sprintf("%sreturn %s, %s\n", indent, e.goSemaZeroValue(e.currentFnSym.ReturnType), errExpr))
		return
	}
	sb.WriteString(fmt.Sprintf("%sreturn %s\n", indent, errExpr))
}

func (e *emitter) liftFallibleCalls(sb *strings.Builder, expr ast.Expr, indent string) bool {
	if expr == nil {
		return false
	}
	switch ex := expr.(type) {
	case *ast.UnaryExpr:
		return e.liftFallibleCalls(sb, ex.Operand, indent)
	case *ast.BinaryExpr:
		if ex.Op == token.AND || ex.Op == token.OR {
			var rightBuf strings.Builder
			e.liftFallibleCalls(&rightBuf, ex.Right, indent+"\t")
			if rightBuf.Len() > 0 {
				e.liftFallibleCalls(sb, ex.Left, indent)
				e.condCounter++
				condVar := fmt.Sprintf("_cbCond_%d", e.condCounter)
				sb.WriteString(fmt.Sprintf("%svar %s bool = %s\n", indent, condVar, e.emitExpr(ex.Left)))
				if ex.Op == token.AND {
					sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, condVar))
				} else {
					sb.WriteString(fmt.Sprintf("%sif !%s {\n", indent, condVar))
				}
				sb.WriteString(rightBuf.String())
				sb.WriteString(fmt.Sprintf("%s\t%s = %s\n", indent, condVar, e.emitExpr(ex.Right)))
				sb.WriteString(fmt.Sprintf("%s}\n", indent))
				if e.liftedExprs == nil {
					e.liftedExprs = make(map[ast.Expr]string)
				}
				e.liftedExprs[ex] = condVar
				return true
			}
			return e.liftFallibleCalls(sb, ex.Left, indent)
		}
		l := e.liftFallibleCalls(sb, ex.Left, indent)
		r := e.liftFallibleCalls(sb, ex.Right, indent)
		return l || r
	case *ast.ListLitExpr:
		any := false
		for _, el := range ex.Elements {
			if e.liftFallibleCalls(sb, el, indent) {
				any = true
			}
		}
		return any
	case *ast.DictLitExpr:
		any := false
		for _, ent := range ex.Entries {
			if e.liftFallibleCalls(sb, ent.Key, indent) {
				any = true
			}
			if e.liftFallibleCalls(sb, ent.Value, indent) {
				any = true
			}
		}
		return any
	case *ast.FieldAccessExpr:
		return e.liftFallibleCalls(sb, ex.Target, indent)
	case *ast.IndexExpr:
		t := e.liftFallibleCalls(sb, ex.Target, indent)
		i := e.liftFallibleCalls(sb, ex.Index, indent)
		return t || i
	case *ast.SliceExpr:
		t := e.liftFallibleCalls(sb, ex.Target, indent)
		s := e.liftFallibleCalls(sb, ex.Start, indent)
		en := e.liftFallibleCalls(sb, ex.End, indent)
		return t || s || en
	case *ast.TypedEmptyCtorExpr:
		any := false
		for _, a := range ex.Args {
			if e.liftFallibleCalls(sb, a, indent) {
				any = true
			}
		}
		return any
	case *ast.StartJobExpr:
		any := false
		if e.prog != nil {
			if ci, hasCI := e.prog.CallInfos[ex.Call]; hasCI && ci.Receiver != nil {
				if e.liftFallibleCalls(sb, ci.Receiver, indent) {
					any = true
				}
			}
		}
		for _, a := range ex.Call.Args {
			if e.liftFallibleCalls(sb, a.Value, indent) {
				any = true
			}
		}
		return any
	case *ast.CallExpr:
		if e.prog == nil {
			return false
		}
		ci, hasCI := e.prog.CallInfos[ex]
		if hasCI && ci.Receiver != nil {
			e.liftFallibleCalls(sb, ci.Receiver, indent)
		}
		for _, a := range ex.Args {
			e.liftFallibleCalls(sb, a.Value, indent)
		}
		if !hasCI {
			return false
		}
		if ci.Kind == sema.CallError {
			e.usedCbError = true
			msgCode := e.emitExpr(ex.Args[0].Value)
			codeExpr := "int64(0)"
			if len(ex.Args) == 2 {
				codeExpr = e.emitExpr(ex.Args[1].Value)
			}
			cbErr := fmt.Sprintf("CbError{msg: %s, code: %s, file: %s, line: int64(%d)}",
				msgCode, codeExpr, strconv.Quote(ex.Pos.File), ex.Pos.Line)
			e.emitErrorJumpOrReturn(sb, indent, cbErr)
			return true
		}
		if ci.CanError {
			callStr := e.emitCallExprRaw(ex)
			e.errCounter++
			id := e.errCounter
			errVar := fmt.Sprintf("_err_%d", id)
			if ci.HasValue {
				valVar := fmt.Sprintf("_val_%d", id)
				if e.liftedExprs == nil {
					e.liftedExprs = make(map[ast.Expr]string)
				}
				e.liftedExprs[ex] = valVar
				sb.WriteString(fmt.Sprintf("%s%s, %s := %s\n", indent, valVar, errVar, callStr))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s := %s\n", indent, errVar, callStr))
			}
			sb.WriteString(fmt.Sprintf("%sif %s != nil {\n", indent, errVar))
			e.emitErrorJumpOrReturn(sb, indent+"\t", errVar)
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
			return true
		}
	}
	return false
}

func (e *emitter) emitStmt(sb *strings.Builder, stmt ast.Statement, indent string) {
	if e.emitLineDirectives && stmt.StmtPos().File != "" && stmt.StmtPos().File != "<memory>" && stmt.StmtPos().Line > 0 {
		sb.WriteString(fmt.Sprintf("//line %s:%d\n", filepath.ToSlash(stmt.StmtPos().File), stmt.StmtPos().Line))
	}

	switch s := stmt.(type) {
	case *ast.VarDecl:
		e.liftFallibleCalls(sb, s.Value, indent)
		typeStr := ""
		if vType, ok := e.prog.VarTypes[s]; ok && !vType.IsVoid() {
			typeStr = " " + e.goSemaType(vType)
		}
		valStr := e.emitExpr(s.Value)
		varGo := goIdent(s.Name)
		sb.WriteString(fmt.Sprintf("%svar %s%s = %s\n", indent, varGo, typeStr, valStr))
		sb.WriteString(fmt.Sprintf("%s_ = %s\n", indent, varGo))

	case *ast.Assignment:
		e.liftFallibleCalls(sb, s.Value, indent)
		if idx, ok := s.Target.(*ast.IndexExpr); ok {
			e.liftFallibleCalls(sb, idx.Target, indent)
			e.liftFallibleCalls(sb, idx.Index, indent)
			sb.WriteString(fmt.Sprintf("%s%s[%s] = %s\n", indent, e.emitPlaceExpr(idx.Target), e.emitExpr(idx.Index), e.emitExpr(s.Value)))
		} else if fa, ok := s.Target.(*ast.FieldAccessExpr); ok {
			if _, boxed := e.isBoxedFieldAccess(fa); boxed {
				e.usedBoxHelpers = true
				sb.WriteString(fmt.Sprintf("%s%s.%s = cbBox(%s)\n", indent, e.emitPlaceExpr(fa.Target), goStructFieldName(fa.Field), e.emitExpr(s.Value)))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s = %s\n", indent, e.emitPlaceExpr(s.Target), e.emitExpr(s.Value)))
			}
		} else {
			sb.WriteString(fmt.Sprintf("%s%s = %s\n", indent, e.emitExpr(s.Target), e.emitExpr(s.Value)))
		}

	case *ast.ExprStmt:
		if call, ok := s.Expr.(*ast.CallExpr); ok && e.prog != nil {
			if ci, hasCI := e.prog.CallInfos[call]; hasCI {
				if ci.Kind == sema.CallError {
					e.liftFallibleCalls(sb, s.Expr, indent)
					return
				}
				if ci.CanError {
					e.liftFallibleCalls(sb, s.Expr, indent)
					if valVar, lifted := e.liftedExprs[call]; lifted {
						sb.WriteString(fmt.Sprintf("%s_ = %s\n", indent, valVar))
					}
					return
				}
			}
		}
		e.liftFallibleCalls(sb, s.Expr, indent)
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, e.emitExpr(s.Expr)))

	case *ast.IfStmt:
		e.liftFallibleCalls(sb, s.Cond, indent)
		sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, e.emitExpr(s.Cond)))
		e.emitStmtList(s.ThenBody, indent+"\t", sb)
		elseIndent := indent
		extraCloses := 0
		for _, elif := range s.ElseIfs {
			var liftBuf strings.Builder
			if e.liftFallibleCalls(&liftBuf, elif.Cond, elseIndent+"\t") {
				sb.WriteString(fmt.Sprintf("%s} else {\n", elseIndent))
				sb.WriteString(liftBuf.String())
				elseIndent += "\t"
				extraCloses++
				sb.WriteString(fmt.Sprintf("%sif %s {\n", elseIndent, e.emitExpr(elif.Cond)))
			} else {
				sb.WriteString(fmt.Sprintf("%s} else if %s {\n", elseIndent, e.emitExpr(elif.Cond)))
			}
			e.emitStmtList(elif.Body, elseIndent+"\t", sb)
		}
		if s.HasElse {
			sb.WriteString(fmt.Sprintf("%s} else {\n", elseIndent))
			e.emitStmtList(s.ElseBody, elseIndent+"\t", sb)
		}
		sb.WriteString(fmt.Sprintf("%s}\n", elseIndent))
		for i := 0; i < extraCloses; i++ {
			elseIndent = strings.TrimSuffix(elseIndent, "\t")
			sb.WriteString(fmt.Sprintf("%s}\n", elseIndent))
		}

	case *ast.ForStmt:
		if s.IsRange {
			e.liftFallibleCalls(sb, s.Start, indent)
			e.liftFallibleCalls(sb, s.End, indent)
			if s.Step != nil {
				e.liftFallibleCalls(sb, s.Step, indent)
			}
			e.loopCounter++
			lid := e.loopCounter
			endVar := fmt.Sprintf("_cbEnd_%d", lid)
			varGo := goIdent(s.VarName)
			sb.WriteString(fmt.Sprintf("%svar %s int64 = %s\n", indent, endVar, e.emitExpr(s.End)))
			if s.Step == nil {
				sb.WriteString(fmt.Sprintf("%sfor %s := %s; %s <= %s; %s += int64(1) {\n",
					indent, varGo, e.emitExpr(s.Start), varGo, endVar, varGo))
				sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, varGo))
				e.emitStmtList(s.Body, indent+"\t", sb)
				sb.WriteString(fmt.Sprintf("%s}\n", indent))
			} else {
				stepVar := fmt.Sprintf("_cbStep_%d", lid)
				sb.WriteString(fmt.Sprintf("%svar %s int64 = %s\n", indent, stepVar, e.emitExpr(s.Step)))
				sb.WriteString(fmt.Sprintf("%sfor %s := %s; (%s > 0 && %s <= %s) || (%s < 0 && %s >= %s); %s += %s {\n",
					indent, varGo, e.emitExpr(s.Start),
					stepVar, varGo, endVar,
					stepVar, varGo, endVar,
					varGo, stepVar))
				sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, varGo))
				e.emitStmtList(s.Body, indent+"\t", sb)
				sb.WriteString(fmt.Sprintf("%s}\n", indent))
			}
		} else {
			e.liftFallibleCalls(sb, s.Iterable, indent)
			iterType := e.prog.ExprTypes[s.Iterable]
			switch iterType.Kind {
			case sema.KindList:
				if len(s.Vars) == 1 {
					v0 := goIdent(s.Vars[0])
					sb.WriteString(fmt.Sprintf("%sfor _, %s := range %s {\n", indent, v0, e.emitExpr(s.Iterable)))
					sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, v0))
				} else {
					e.loopCounter++
					idxTmp := fmt.Sprintf("_cbIdx_%d", e.loopCounter)
					v0 := goIdent(s.Vars[0])
					v1 := goIdent(s.Vars[1])
					sb.WriteString(fmt.Sprintf("%sfor %s, %s := range %s {\n", indent, idxTmp, v1, e.emitExpr(s.Iterable)))
					sb.WriteString(fmt.Sprintf("%s\tvar %s int64 = int64(%s)\n", indent, v0, idxTmp))
					sb.WriteString(fmt.Sprintf("%s\t_ = %s\n%s\t_ = %s\n", indent, v0, indent, v1))
				}
			case sema.KindStr:
				e.loopCounter++
				runeTmp := fmt.Sprintf("_cbRune_%d", e.loopCounter)
				if len(s.Vars) == 1 {
					v0 := goIdent(s.Vars[0])
					sb.WriteString(fmt.Sprintf("%sfor _, %s := range %s {\n", indent, runeTmp, e.emitExpr(s.Iterable)))
					sb.WriteString(fmt.Sprintf("%s\tvar %s string = string(%s)\n", indent, v0, runeTmp))
					sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, v0))
				} else {
					idxTmp := fmt.Sprintf("_cbIdx_%d", e.loopCounter)
					v0 := goIdent(s.Vars[0])
					v1 := goIdent(s.Vars[1])
					sb.WriteString(fmt.Sprintf("%sfor %s, %s := range %s {\n", indent, idxTmp, runeTmp, e.emitExpr(s.Iterable)))
					sb.WriteString(fmt.Sprintf("%s\tvar %s int64 = int64(%s)\n", indent, v0, idxTmp))
					sb.WriteString(fmt.Sprintf("%s\tvar %s string = string(%s)\n", indent, v1, runeTmp))
					sb.WriteString(fmt.Sprintf("%s\t_ = %s\n%s\t_ = %s\n", indent, v0, indent, v1))
				}
			case sema.KindDict:
				if len(s.Vars) == 1 {
					v0 := goIdent(s.Vars[0])
					sb.WriteString(fmt.Sprintf("%sfor %s := range %s {\n", indent, v0, e.emitExpr(s.Iterable)))
					sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, v0))
				} else {
					v0 := goIdent(s.Vars[0])
					v1 := goIdent(s.Vars[1])
					sb.WriteString(fmt.Sprintf("%sfor %s, %s := range %s {\n", indent, v0, v1, e.emitExpr(s.Iterable)))
					sb.WriteString(fmt.Sprintf("%s\t_ = %s\n%s\t_ = %s\n", indent, v0, indent, v1))
				}
			case sema.KindMailbox:
				e.loopCounter++
				lid := e.loopCounter
				msgTmp := fmt.Sprintf("_cbMsg_%d", lid)
				errTmp := fmt.Sprintf("_cbErr_%d", lid)
				boxTmp := fmt.Sprintf("_cbBox_%d", lid)
				v0 := goIdent(s.Vars[0])
				sb.WriteString(fmt.Sprintf("%s%s := %s\n", indent, boxTmp, e.emitExpr(s.Iterable)))
				sb.WriteString(fmt.Sprintf("%sfor {\n", indent))
				sb.WriteString(fmt.Sprintf("%s\t%s, %s := %s.get()\n", indent, msgTmp, errTmp, boxTmp))
				sb.WriteString(fmt.Sprintf("%s\tif %s != nil {\n%s\t\tbreak\n%s\t}\n", indent, errTmp, indent, indent))
				sb.WriteString(fmt.Sprintf("%s\tvar %s = %s\n", indent, v0, msgTmp))
				sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, v0))
			}
			e.emitStmtList(s.Body, indent+"\t", sb)
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		}

	case *ast.SelectStmt:
		e.liftFallibleCalls(sb, s.Target, indent)
		sb.WriteString(fmt.Sprintf("%sswitch %s {\n", indent, e.emitExpr(s.Target)))
		for _, cc := range s.Cases {
			var vals []string
			for _, v := range cc.Values {
				vals = append(vals, e.emitExpr(v))
			}
			sb.WriteString(fmt.Sprintf("%scase %s:\n", indent, strings.Join(vals, ", ")))
			e.emitStmtList(cc.Body, indent+"\t", sb)
		}
		if s.HasElse {
			sb.WriteString(fmt.Sprintf("%sdefault:\n", indent))
			e.emitStmtList(s.ElseBody, indent+"\t", sb)
		}
		sb.WriteString(fmt.Sprintf("%s}\n", indent))

	case *ast.WhileStmt:
		var liftBuf strings.Builder
		if e.liftFallibleCalls(&liftBuf, s.Cond, indent+"\t") {
			sb.WriteString(fmt.Sprintf("%sfor {\n", indent))
			sb.WriteString(liftBuf.String())
			sb.WriteString(fmt.Sprintf("%s\tif !(%s) {\n%s\t\tbreak\n%s\t}\n", indent, e.emitExpr(s.Cond), indent, indent))
			e.emitStmtList(s.Body, indent+"\t", sb)
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		} else {
			sb.WriteString(fmt.Sprintf("%sfor %s {\n", indent, e.emitExpr(s.Cond)))
			e.emitStmtList(s.Body, indent+"\t", sb)
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		}

	case *ast.TryCatchStmt:
		e.usedCbError = true
		e.tryCounter++
		tryID := e.tryCounter
		prevTryID := e.currentTryID
		sb.WriteString(fmt.Sprintf("%svar _tryErr_%d error\n", indent, tryID))
		sb.WriteString(fmt.Sprintf("%s_ = _tryErr_%d\n", indent, tryID))
		sb.WriteString(fmt.Sprintf("%s{\n", indent))
		e.currentTryID = tryID
		e.emitStmtList(s.TryBody, indent+"\t", sb)
		e.currentTryID = prevTryID
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
		if e.tryUsedCatch == nil || !e.tryUsedCatch[tryID] {
			sb.WriteString(fmt.Sprintf("%sif false {\n%s\tgoto _catch_%d\n%s}\n", indent, indent, tryID, indent))
		}
		sb.WriteString(fmt.Sprintf("%sgoto _end_try_%d\n", indent, tryID))
		sb.WriteString(fmt.Sprintf("%s_catch_%d:\n", indent, tryID))
		sb.WriteString(fmt.Sprintf("%s{\n", indent))
		catchGo := goIdent(s.CatchVar)
		sb.WriteString(fmt.Sprintf("%s\tvar %s CbError\n", indent, catchGo))
		sb.WriteString(fmt.Sprintf("%s\tif _cbErr, _ok := _tryErr_%d.(CbError); _ok {\n", indent, tryID))
		sb.WriteString(fmt.Sprintf("%s\t\t%s = _cbErr\n", indent, catchGo))
		sb.WriteString(fmt.Sprintf("%s\t} else {\n", indent))
		sb.WriteString(fmt.Sprintf("%s\t\t%s = CbError{msg: _tryErr_%d.Error()}\n", indent, catchGo, tryID))
		sb.WriteString(fmt.Sprintf("%s\t}\n", indent))
		sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, catchGo))
		e.emitStmtList(s.CatchBody, indent+"\t", sb)
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
		sb.WriteString(fmt.Sprintf("%s_end_try_%d:\n", indent, tryID))

	case *ast.DeferStmt:
		prevTryID := e.currentTryID
		e.currentTryID = 0
		sb.WriteString(fmt.Sprintf("%sdefer func() {\n", indent))
		e.emitStmtList(s.Body, indent+"\t", sb)
		sb.WriteString(fmt.Sprintf("%s}()\n", indent))
		e.currentTryID = prevTryID

	case *ast.ReturnStmt:
		if s.Value != nil {
			e.liftFallibleCalls(sb, s.Value, indent)
			if e.currentFn != nil && e.currentFn.CanError {
				sb.WriteString(fmt.Sprintf("%sreturn %s, nil\n", indent, e.emitExpr(s.Value)))
			} else {
				sb.WriteString(fmt.Sprintf("%sreturn %s\n", indent, e.emitExpr(s.Value)))
			}
		} else {
			if e.currentFn != nil && e.currentFn.CanError {
				sb.WriteString(fmt.Sprintf("%sreturn nil\n", indent))
			} else {
				sb.WriteString(fmt.Sprintf("%sreturn\n", indent))
			}
		}

	case *ast.BreakStmt:
		sb.WriteString(fmt.Sprintf("%sbreak\n", indent))

	case *ast.ContinueStmt:
		sb.WriteString(fmt.Sprintf("%scontinue\n", indent))
	}
}
