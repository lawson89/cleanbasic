package codegen

import (
	"fmt"
	"path/filepath"
	"strings"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/sema"
)

func (e *emitter) emitZeroOrInitVarDecl(sb *strings.Builder, varGo string, d *ast.VarDecl, valStr string, indent string) {
	if d.Value != nil {
		if valStr == "" {
			valStr = e.emitExpr(d.Value)
		}
		typeStr := ""
		if d.Type != nil && !d.Type.IsVoid() {
			typeStr = " " + e.goSemaType(*d.Type)
		}
		sb.WriteString(fmt.Sprintf("%svar %s%s = %s\n", indent, varGo, typeStr, valStr))
		return
	}

	goType := e.goSemaType(*d.Type)
	switch d.Type.Kind {
	case ast.KindDict:
		sb.WriteString(fmt.Sprintf("%svar %s %s = %s{}\n", indent, varGo, goType, goType))
	case ast.KindMailbox:
		elemGo := e.goSemaType(d.Type.Elem())
		sb.WriteString(fmt.Sprintf("%svar %s %s = make(chan %s)\n", indent, varGo, goType, elemGo))
	case ast.KindStruct:
		if e.structNeedsFieldInit(e.lookupStruct(*d.Type)) {
			sb.WriteString(fmt.Sprintf("%svar %s %s = %s\n", indent, varGo, goType, e.emitZeroStruct(*d.Type)))
		} else {
			sb.WriteString(fmt.Sprintf("%svar %s %s\n", indent, varGo, goType))
		}
	default:
		sb.WriteString(fmt.Sprintf("%svar %s %s\n", indent, varGo, goType))
	}
}
func (e *emitter) emitStmtList(stmts []ast.Statement, indent string, sb *strings.Builder) {
	for _, st := range stmts {
		e.emitStmt(sb, st, indent)
	}
}

func (e *emitter) emitErrorJumpOrReturn(sb *strings.Builder, indent, errExpr string) {
	if e.currentTryID > 0 {
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
	if !ok {
		return "", false
	}
	ci := call.CallInfo
	if ci == nil {
		return "", false
	}
	if ci.Kind == sema.CallError {
		e.useImport("errors")
		errExpr := fmt.Sprintf("errors.New(%s)", e.emitExpr(call.Args[0]))
		e.emitErrorJumpOrReturn(sb, indent, errExpr)
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
		valStr := ""
		if s.Value != nil {
			valStr = e.emitRhs(sb, s.Value, indent)
		}
		varGo := s.Name
		e.emitZeroOrInitVarDecl(sb, varGo, s, valStr, indent)
		sb.WriteString(fmt.Sprintf("%s_ = %s\n", indent, varGo))

	case *ast.Assignment:
		valStr := e.emitRhs(sb, s.Value, indent)
		sb.WriteString(fmt.Sprintf("%s%s = %s\n", indent, e.emitExpr(s.Target), valStr))

	case *ast.ExprStmt:
		if valVar, ok := e.emitFallibleCallExpr(sb, s.Expr, indent); ok {
			if valVar != "" {
				sb.WriteString(fmt.Sprintf("%s_ = %s\n", indent, valVar))
			}
			return
		}
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, e.emitExpr(s.Expr)))

	case *ast.StartJobStmt:
		sb.WriteString(fmt.Sprintf("%sgo %s\n", indent, e.emitCallExprRaw(s.Call)))
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
			varGo := s.VarName
			sb.WriteString(fmt.Sprintf("%svar %s int64 = %s\n", indent, endVar, e.emitExpr(s.End)))
			sb.WriteString(fmt.Sprintf("%sfor %s := %s; %s <= %s; %s++ {\n",
				indent, varGo, e.emitExpr(s.Start), varGo, endVar, varGo))
			sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, varGo))
			e.emitStmtList(s.Body, indent+"\t", sb)
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		} else {
			iterType := s.Iterable.ExprType()
			iterExpr := e.emitExpr(s.Iterable)
			v0 := s.Vars[0]
			switch iterType.Kind {
			case sema.KindList:
				if len(s.Vars) == 1 {
					sb.WriteString(fmt.Sprintf("%sfor _, %s := range %s {\n", indent, v0, iterExpr))
				} else {
					e.loopCounter++
					idxTmp := fmt.Sprintf("_cbIdx_%d", e.loopCounter)
					v1 := s.Vars[1]
					sb.WriteString(fmt.Sprintf("%sfor %s, %s := range %s {\n", indent, idxTmp, v1, iterExpr))
					sb.WriteString(fmt.Sprintf("%s\tvar %s int64 = int64(%s)\n", indent, v0, idxTmp))
				}
			case sema.KindStr:
				e.loopCounter++
				runeTmp := fmt.Sprintf("_cbRune_%d", e.loopCounter)
				if len(s.Vars) == 1 {
					sb.WriteString(fmt.Sprintf("%sfor _, %s := range %s {\n", indent, runeTmp, iterExpr))
					sb.WriteString(fmt.Sprintf("%s\tvar %s string = string(%s)\n", indent, v0, runeTmp))
				} else {
					idxTmp := fmt.Sprintf("_cbIdx_%d", e.loopCounter)
					v1 := s.Vars[1]
					sb.WriteString(fmt.Sprintf("%sfor %s, %s := range %s {\n", indent, idxTmp, runeTmp, iterExpr))
					sb.WriteString(fmt.Sprintf("%s\tvar %s int64 = int64(%s)\n", indent, v0, idxTmp))
					sb.WriteString(fmt.Sprintf("%s\tvar %s string = string(%s)\n", indent, v1, runeTmp))
				}
			case sema.KindDict:
				if len(s.Vars) == 1 {
					sb.WriteString(fmt.Sprintf("%sfor %s := range %s {\n", indent, v0, iterExpr))
				} else {
					sb.WriteString(fmt.Sprintf("%sfor %s, %s := range %s {\n", indent, v0, s.Vars[1], iterExpr))
				}
			case sema.KindMailbox:
				sb.WriteString(fmt.Sprintf("%sfor %s := range %s {\n", indent, v0, iterExpr))
			}
			for _, v := range s.Vars {
				sb.WriteString(fmt.Sprintf("%s\t_ = %s\n", indent, v))
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
		sb.WriteString(fmt.Sprintf("%sif false {\n%s\tgoto _catch_%d\n%s}\n", indent, indent, tryID, indent))
		sb.WriteString(fmt.Sprintf("%sgoto _end_try_%d\n", indent, tryID))
		sb.WriteString(fmt.Sprintf("%s_catch_%d:\n", indent, tryID))
		sb.WriteString(fmt.Sprintf("%s{\n", indent))
		catchGo := s.CatchVar
		sb.WriteString(fmt.Sprintf("%s\tvar %s error = _tryErr_%d\n", indent, catchGo, tryID))
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
