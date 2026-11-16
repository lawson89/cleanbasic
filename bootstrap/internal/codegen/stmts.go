package codegen

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/sema"
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

func (e *emitter) emitFallibleCallExpr(sb *strings.Builder, expr ast.Expr, indent string) (string, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || e.prog == nil {
		return "", false
	}
	ci, hasCI := e.prog.CallInfos[call]
	if !hasCI {
		return "", false
	}
	if ci.Kind == sema.CallError {
		e.usedCbError = true
		codeExpr := "int64(0)"
		if len(call.Args) == 2 {
			codeExpr = e.emitExpr(call.Args[1])
		}
		cbErr := fmt.Sprintf("CbError{msg: %s, code: %s, file: %s, line: int64(%d)}",
			e.emitExpr(call.Args[0]), codeExpr, strconv.Quote(call.Pos.File), call.Pos.Line)
		e.emitErrorJumpOrReturn(sb, indent, cbErr)
		return "", true
	}
	if !ci.CanError {
		return "", false
	}
	callStr := e.emitCallExprRaw(call)
	e.errCounter++
	errVar := fmt.Sprintf("_err_%d", e.errCounter)
	valVar := ""
	if ci.HasValue {
		valVar = fmt.Sprintf("_val_%d", e.errCounter)
		sb.WriteString(fmt.Sprintf("%s%s, %s := %s\n", indent, valVar, errVar, callStr))
	} else {
		sb.WriteString(fmt.Sprintf("%s%s := %s\n", indent, errVar, callStr))
	}
	sb.WriteString(fmt.Sprintf("%sif %s != nil {\n", indent, errVar))
	e.emitErrorJumpOrReturn(sb, indent+"\t", errVar)
	sb.WriteString(fmt.Sprintf("%s}\n", indent))
	return valVar, true
}

func (e *emitter) emitRhs(sb *strings.Builder, expr ast.Expr, indent string) string {
	if valVar, ok := e.emitFallibleCallExpr(sb, expr, indent); ok {
		return valVar
	}
	return e.emitExpr(expr)
}

func (e *emitter) emitStmt(sb *strings.Builder, stmt ast.Statement, indent string) {
	if e.emitLineDirectives && stmt.StmtPos().File != "" && stmt.StmtPos().File != "<memory>" && stmt.StmtPos().Line > 0 {
		sb.WriteString(fmt.Sprintf("//line %s:%d\n", filepath.ToSlash(stmt.StmtPos().File), stmt.StmtPos().Line))
	}

	switch s := stmt.(type) {
	case *ast.VarDecl:
		valStr := e.emitRhs(sb, s.Value, indent)
		typeStr := ""
		if vType, ok := e.prog.VarTypes[s]; ok && !vType.IsVoid() {
			typeStr = " " + e.goSemaType(vType)
		}
		varGo := goIdent(s.Name)
		sb.WriteString(fmt.Sprintf("%svar %s%s = %s\n", indent, varGo, typeStr, valStr))
		sb.WriteString(fmt.Sprintf("%s_ = %s\n", indent, varGo))

	case *ast.Assignment:
		valStr := e.emitRhs(sb, s.Value, indent)
		if idx, ok := s.Target.(*ast.IndexExpr); ok {
			sb.WriteString(fmt.Sprintf("%s%s[%s] = %s\n", indent, e.emitPlaceExpr(idx.Target), e.emitExpr(idx.Index), valStr))
		} else if fa, ok := s.Target.(*ast.FieldAccessExpr); ok {
			if _, boxed := e.isBoxedFieldAccess(fa); boxed {
				e.usedBoxHelpers = true
				sb.WriteString(fmt.Sprintf("%s%s.%s = cbBox(%s)\n", indent, e.emitPlaceExpr(fa.Target), goStructFieldName(fa.Field), valStr))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s = %s\n", indent, e.emitPlaceExpr(s.Target), valStr))
			}
		} else {
			sb.WriteString(fmt.Sprintf("%s%s = %s\n", indent, e.emitExpr(s.Target), valStr))
		}

	case *ast.ExprStmt:
		if valVar, ok := e.emitFallibleCallExpr(sb, s.Expr, indent); ok {
			if valVar != "" {
				sb.WriteString(fmt.Sprintf("%s_ = %s\n", indent, valVar))
			}
			return
		}
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, e.emitExpr(s.Expr)))

	case *ast.IfStmt:
		sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, e.emitExpr(s.Cond)))
		e.emitStmtList(s.ThenBody, indent+"\t", sb)
		for _, elif := range s.ElseIfs {
			sb.WriteString(fmt.Sprintf("%s} else if %s {\n", indent, e.emitExpr(elif.Cond)))
			e.emitStmtList(elif.Body, indent+"\t", sb)
		}
		if s.HasElse {
			sb.WriteString(fmt.Sprintf("%s} else {\n", indent))
			e.emitStmtList(s.ElseBody, indent+"\t", sb)
		}
		sb.WriteString(fmt.Sprintf("%s}\n", indent))

	case *ast.ForStmt:
		if s.IsRange {
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
		sb.WriteString(fmt.Sprintf("%sfor %s {\n", indent, e.emitExpr(s.Cond)))
		e.emitStmtList(s.Body, indent+"\t", sb)
		sb.WriteString(fmt.Sprintf("%s}\n", indent))

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
			valStr := e.emitRhs(sb, s.Value, indent)
			if e.currentFn != nil && e.currentFn.CanError {
				sb.WriteString(fmt.Sprintf("%sreturn %s, nil\n", indent, valStr))
			} else {
				sb.WriteString(fmt.Sprintf("%sreturn %s\n", indent, valStr))
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
