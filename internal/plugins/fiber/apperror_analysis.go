// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"

	"github.com/whisskey/prism/pkg/openapi"
)

func apperrorStatusFromCall(callExpr *ast.CallExpr) string {
	if callExpr == nil {
		return ""
	}
	sel, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || !isAppErrorSelector(sel) {
		return ""
	}
	switch sel.Sel.Name {
	case "NewBadRequest":
		return "400"
	case "NewUnauthorized":
		return "401"
	case "NewForbidden":
		return "403"
	case "NewNotFound":
		return "404"
	case "NewValidation":
		return "422"
	case "NewInternal":
		return "500"
	default:
		return ""
	}
}

func isAppErrorSelector(sel *ast.SelectorExpr) bool {
	if sel == nil {
		return false
	}
	return qualifiedSelectorPackage(sel) == "apperror"
}

func qualifiedSelectorPackage(sel *ast.SelectorExpr) string {
	if sel == nil {
		return ""
	}
	switch x := sel.X.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	default:
		return ""
	}
}

func (p *Plugin) collectAppErrorStatusesFromFunc(funcDecl *ast.FuncDecl, visited map[*ast.FuncDecl]bool) []string {
	if funcDecl == nil || funcDecl.Body == nil || visited[funcDecl] {
		return nil
	}
	visited[funcDecl] = true

	seen := make(map[string]struct{})
	var statuses []string
	addStatus := func(code string) {
		if code == "" || !isErrorHTTPStatus(code) {
			return
		}
		if _, ok := seen[code]; ok {
			return
		}
		seen[code] = struct{}{}
		statuses = append(statuses, code)
	}

	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		addStatus(apperrorStatusFromCall(callExpr))
		if code := callStatusCode(callExpr); isErrorHTTPStatus(code) {
			addStatus(code)
		}
		if resp := responseErrorStatusFromCall(callExpr); resp != "" {
			addStatus(resp)
		}
		helperDecl := p.resolveHelperFunc(callExpr)
		if helperDecl != nil && helperDecl != funcDecl {
			for _, code := range p.collectAppErrorStatusesFromFunc(helperDecl, visited) {
				addStatus(code)
			}
		}
		return true
	})
	return statuses
}

func responseErrorStatusFromCall(callExpr *ast.CallExpr) string {
	sel, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || qualifiedSelectorPackage(sel) != "response" {
		return ""
	}
	switch sel.Sel.Name {
	case "Error", "ErrorWithDetails":
		if len(callExpr.Args) > 0 {
			return statusCodeFromExpr(callExpr.Args[0])
		}
	}
	return ""
}

func (p *Plugin) standardEnvelopeErrorSchema() *openapi.Schema {
	if schema, ok := p.registeredResponseSchema("Response"); ok {
		return schema
	}
	if schema, ok := p.registeredResponseSchema(p.defaultErrorSchemaName()); ok {
		return schema
	}
	return p.responseSchemaFromType(p.defaultErrorSchemaName())
}

func (p *Plugin) registeredResponseSchema(typeName string) (*openapi.Schema, bool) {
	typeName = cleanTypeName(typeName)
	if typeName == "" {
		return nil, false
	}
	schemaValue, ok := p.schemaExtractor.Registry().Get(typeName)
	if !ok || schemaValue == nil {
		return nil, false
	}
	p.markSchemaUsed(typeName)
	return cloneSchema(schemaValue), true
}

func (p *Plugin) inferredErrorResponse(status string) *inferredResponse {
	schema := p.standardEnvelopeErrorSchema()
	return &inferredResponse{
		code:    status,
		schema:  schema,
		example: p.exampleFromSchema(schema),
	}
}

func bindQueryErrorStatusesFromFunc(funcDecl *ast.FuncDecl) []string {
	if funcDecl == nil || funcDecl.Body == nil {
		return nil
	}
	hasQueryBind := false
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isQueryBindCall(callExpr) {
			hasQueryBind = true
			return false
		}
		return true
	})
	if !hasQueryBind {
		return nil
	}
	return []string{"400"}
}

func isQueryBindCall(callExpr *ast.CallExpr) bool {
	sel, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Query" || len(callExpr.Args) == 0 {
		return false
	}
	bindCall, ok := sel.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	bindSel, ok := bindCall.Fun.(*ast.SelectorExpr)
	return ok && bindSel.Sel.Name == "Bind"
}

func (p *Plugin) collectHandlerErrorStatuses(funcDecl *ast.FuncDecl) []string {
	if funcDecl == nil || funcDecl.Body == nil {
		return nil
	}

	seen := make(map[string]struct{})
	var statuses []string
	addStatus := func(code string) {
		if code == "" || !isErrorHTTPStatus(code) {
			return
		}
		if _, ok := seen[code]; ok {
			return
		}
		seen[code] = struct{}{}
		statuses = append(statuses, code)
	}

	visited := make(map[*ast.FuncDecl]bool)
	var walkStmts func(stmts []ast.Stmt, errSource ast.Expr)
	walkStmts = func(stmts []ast.Stmt, errSource ast.Expr) {
		for _, stmt := range stmts {
			switch node := stmt.(type) {
			case *ast.AssignStmt:
				errSource = latestErrRHS(node, errSource)
			case *ast.ReturnStmt:
				for _, result := range node.Results {
					addStatus(p.errorStatusFromReturnResult(result, errSource, visited))
				}
			case *ast.IfStmt:
				walkStmts(node.Body.List, errSource)
				if node.Else != nil {
					walkElseStmt(node.Else, errSource, walkStmts)
				}
			case *ast.BlockStmt:
				walkStmts(node.List, errSource)
			}
		}
	}
	walkStmts(funcDecl.Body.List, nil)

	for _, code := range p.collectAppErrorStatusesFromFunc(funcDecl, visited) {
		addStatus(code)
	}
	return statuses
}

func walkElseStmt(stmt ast.Stmt, errSource ast.Expr, walkStmts func([]ast.Stmt, ast.Expr)) {
	switch node := stmt.(type) {
	case *ast.BlockStmt:
		walkStmts(node.List, errSource)
	case *ast.IfStmt:
		walkStmts(node.Body.List, errSource)
		if node.Else != nil {
			walkElseStmt(node.Else, errSource, walkStmts)
		}
	}
}

func latestErrRHS(stmt *ast.AssignStmt, previous ast.Expr) ast.Expr {
	for i, lhs := range stmt.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok || ident.Name != "err" {
			continue
		}
		rhsIndex := i
		if len(stmt.Rhs) == 1 {
			rhsIndex = 0
		}
		if rhsIndex < len(stmt.Rhs) {
			return stmt.Rhs[rhsIndex]
		}
	}
	return previous
}

func (p *Plugin) errorStatusFromReturnResult(expr ast.Expr, errSource ast.Expr, visited map[*ast.FuncDecl]bool) string {
	switch e := expr.(type) {
	case *ast.Ident:
		if e.Name != "err" {
			return ""
		}
		return p.errorStatusFromErrSource(errSource, visited)
	case *ast.CallExpr:
		return apperrorStatusFromCall(e)
	default:
		return ""
	}
}

func (p *Plugin) errorStatusFromErrSource(source ast.Expr, visited map[*ast.FuncDecl]bool) string {
	callExpr, ok := source.(*ast.CallExpr)
	if !ok {
		return ""
	}
	if status := apperrorStatusFromCall(callExpr); status != "" {
		return status
	}
	helperDecl := p.resolveHelperFunc(callExpr)
	if helperDecl == nil {
		return ""
	}
	for _, code := range bindQueryErrorStatusesFromFunc(helperDecl) {
		return code
	}
	for _, code := range p.collectAppErrorStatusesFromFunc(helperDecl, visited) {
		return code
	}
	return ""
}

func mergeStatusCodes(existing []string, extra ...string) []string {
	if len(extra) == 0 {
		return existing
	}
	seen := make(map[string]struct{}, len(existing))
	merged := append([]string(nil), existing...)
	for _, code := range existing {
		seen[code] = struct{}{}
	}
	for _, code := range extra {
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		merged = append(merged, code)
	}
	return merged
}

func shouldUseEnvelopeErrorSchema(status string) bool {
	switch status {
	case "400", "401", "403", "404", "409", "422", "500":
		return true
	default:
		return false
	}
}

func errorStatusesFromMiddlewareCallExtended(call *ast.CallExpr) []string {
	codes := errorStatusesFromMiddlewareCall(call)
	if status := apperrorStatusFromCall(call); status != "" {
		codes = append(codes, status)
	}
	return codes
}
