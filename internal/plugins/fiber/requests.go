// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"
	"strings"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/pkg/openapi"
)

func (p *Plugin) applyMiddlewareInference(route *openapi.Route, middlewareArgs []ast.Expr) {
	for _, arg := range middlewareArgs {
		statuses := p.configuredMiddlewareStatuses(arg)
		statuses = append(statuses, p.middlewareStatuses(arg)...)
		if callExpr, ok := arg.(*ast.CallExpr); ok {
			for _, status := range statuses {
				if !isErrorHTTPStatus(status) {
					continue
				}
				if route.Responses == nil {
					route.Responses = make(map[string]openapi.Response)
				}
				response := p.errorResponseFromFunc(p.resolveHelperFunc(callExpr), status)
				route.Responses[status] = jsonResponseWithExample(responseDescription(status), response.schema, response.example)
				p.markSchemaRefs(response.schema)
			}
		}

		schemaName := p.middlewareRequestSchema(arg)
		if schemaName == "" {
			continue
		}

		if methodAllowsRequestBody(route.Method) && route.RequestBody == nil {
			p.markSchemaUsed(schemaName)
			route.RequestBody = jsonRequestBody(schemaName)
			p.rememberRequestPathFields(schemaName, route.Parameters)
		}
		if schemaLooksLikeIDParam(schemaName) || (len(route.Parameters) > 0 && strings.Contains(schemaName, "Update")) {
			for i := range route.Parameters {
				route.Parameters[i].Schema = &openapi.Schema{Type: "integer"}
			}
		}
	}

	p.applyAuthSecurity(route, middlewareArgs)
}

func requestValidatorSchema(expr ast.Expr) string {
	return genericValidatorSchema(expr)
}

func selectorNameFromExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	default:
		return ""
	}
}

func schemaLooksLikeIDParam(schemaName string) bool {
	return schemaName == "IDParamRequest" || strings.Contains(schemaName, "IDParam")
}

func (p *Plugin) middlewareStatuses(expr ast.Expr) []string {
	callExpr, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil
	}
	funcDecl := p.resolveHelperFunc(callExpr)
	if funcDecl == nil || funcDecl.Body == nil {
		return nil
	}
	return p.collectErrorStatusesFromFunc(funcDecl, make(map[*ast.FuncDecl]bool))
}

func (p *Plugin) errorResponseFromFunc(funcDecl *ast.FuncDecl, status string) *inferredResponse {
	return p.errorResponseFromFuncDepth(funcDecl, status, make(map[*ast.FuncDecl]bool))
}

func (p *Plugin) errorResponseFromFuncDepth(funcDecl *ast.FuncDecl, status string, visited map[*ast.FuncDecl]bool) *inferredResponse {
	if funcDecl == nil || funcDecl.Body == nil || visited[funcDecl] {
		return &inferredResponse{code: status}
	}
	visited[funcDecl] = true

	var found *inferredResponse
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if resp := p.responseFromHelperCall(callExpr, nil, nil); resp != nil && resp.code == status {
			found = resp
			return false
		}
		if resp := p.responseFromJSONCall(callExpr, nil, nil); resp != nil && resp.code == status {
			found = resp
			return false
		}
		if resp := p.responseFromSendStatusCall(callExpr); resp != nil && resp.code == status {
			found = resp
			return false
		}
		if callStatusCode(callExpr) == status && (found == nil || found.schema == nil) {
			found = &inferredResponse{code: status}
		}
		helperDecl := p.resolveHelperFunc(callExpr)
		if helperDecl != nil && helperDecl != funcDecl {
			if nested := p.errorResponseFromFuncDepth(helperDecl, status, visited); nested != nil && nested.schema != nil {
				found = nested
				return false
			}
		}
		return true
	})
	if found != nil {
		return found
	}
	return &inferredResponse{code: status}
}

func (p *Plugin) collectErrorStatusesFromFunc(funcDecl *ast.FuncDecl, visited map[*ast.FuncDecl]bool) []string {
	if funcDecl == nil || funcDecl.Body == nil || visited[funcDecl] {
		return nil
	}
	visited[funcDecl] = true

	statuses := make(map[string]bool)
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if resp := p.responseFromHelperCall(callExpr, nil, nil); resp != nil && isErrorHTTPStatus(resp.code) {
			statuses[resp.code] = true
			return true
		}
		if status := callStatusCode(callExpr); isErrorHTTPStatus(status) {
			statuses[status] = true
			return true
		}
		helperDecl := p.resolveHelperFunc(callExpr)
		if helperDecl == nil || helperDecl == funcDecl {
			return true
		}
		if p.analyzeResponseHelper(helperDecl).kind != responseHelperUnknown {
			return true
		}
		for _, status := range p.collectErrorStatusesFromFunc(helperDecl, visited) {
			statuses[status] = true
		}
		return true
	})

	result := make([]string, 0, len(statuses))
	for status := range statuses {
		result = append(result, status)
	}
	return result
}

func isErrorHTTPStatus(code string) bool {
	return code != "" && !strings.HasPrefix(code, "2") && !strings.HasPrefix(code, "3")
}

func callStatusCode(callExpr *ast.CallExpr) string {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	switch selExpr.Sel.Name {
	case "Status", "SendStatus":
		if len(callExpr.Args) > 0 {
			return statusCodeFromExpr(callExpr.Args[0])
		}
	case "NewError":
		if selectorName(selExpr.X) == "fiber" && len(callExpr.Args) > 0 {
			return statusCodeFromExpr(callExpr.Args[0])
		}
	default:
		return exceptionStatusFromCall(callExpr)
	}
	return ""
}

func (p *Plugin) rememberRequestPathFields(schemaName string, params []openapi.Parameter) {
	if schemaName == "" || len(params) == 0 {
		return
	}
	if p.requestPathFields[schemaName] == nil {
		p.requestPathFields[schemaName] = make(map[string]bool)
	}
	for _, param := range params {
		p.requestPathFields[schemaName][param.Name] = true
		p.requestPathFields[schemaName][strings.ToLower(param.Name)] = true
		p.requestPathFields[schemaName][strings.ToUpper(param.Name)] = true
		p.requestPathFields[schemaName][strings.ToUpper(param.Name[:1])+param.Name[1:]] = true
	}
}

func (p *Plugin) collectDeclTypes(stmt *ast.DeclStmt, localTypes map[string]string) {
	genDecl, ok := stmt.Decl.(*ast.GenDecl)
	if !ok {
		return
	}
	for _, spec := range genDecl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		typeName := astTypeString(valueSpec.Type)
		for i, name := range valueSpec.Names {
			if typeName != "" {
				localTypes[name.Name] = typeName
				continue
			}
			if i < len(valueSpec.Values) {
				if inferred := p.schemaNameFromExpr(valueSpec.Values[i], localTypes); inferred != "" {
					localTypes[name.Name] = inferred
				}
			}
		}
	}
}

func (p *Plugin) collectAssignTypes(stmt *ast.AssignStmt, localTypes map[string]string) {
	for i, lhs := range stmt.Lhs {
		rhsIndex := i
		if len(stmt.Rhs) == 1 {
			rhsIndex = 0
		}
		if rhsIndex >= len(stmt.Rhs) {
			continue
		}
		ident, ok := lhs.(*ast.Ident)
		if !ok {
			continue
		}
		if inferred := p.schemaNameFromExpr(stmt.Rhs[rhsIndex], localTypes); inferred != "" {
			localTypes[ident.Name] = inferred
		}
	}
}

func (p *Plugin) requestSchemaFromBodyParser(callExpr *ast.CallExpr, localTypes map[string]string) string {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || selExpr.Sel.Name != "BodyParser" || len(callExpr.Args) == 0 {
		return ""
	}
	return cleanTypeName(p.schemaNameFromExpr(callExpr.Args[0], localTypes))
}

func (p *Plugin) requestSchemaFromBodyBind(callExpr *ast.CallExpr, pf *parser.ParsedFile, localTypes map[string]string) string {
	if schemaName := p.requestSchemaFromBindCall(callExpr, pf, localTypes); schemaName != "" {
		return schemaName
	}
	if schemaName := p.requestSchemaFromFiberBindBody(callExpr, pf, localTypes); schemaName != "" {
		return schemaName
	}
	return ""
}

func (p *Plugin) requestSchemaFromBindCall(callExpr *ast.CallExpr, pf *parser.ParsedFile, localTypes map[string]string) string {
	if callExpr == nil || len(callExpr.Args) < 2 {
		return ""
	}
	if !isBodyBindHelperCall(callExpr, p.resolveHelperFunc(callExpr)) {
		return ""
	}
	arg := callExpr.Args[1]
	if typed := p.typedSchemaName(pf, arg); typed != "" {
		return typed
	}
	return cleanTypeName(p.schemaNameFromExpr(arg, localTypes))
}

func (p *Plugin) requestSchemaFromFiberBindBody(callExpr *ast.CallExpr, pf *parser.ParsedFile, localTypes map[string]string) string {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || selExpr.Sel.Name != "Body" || len(callExpr.Args) == 0 {
		return ""
	}
	bindCall, ok := selExpr.X.(*ast.CallExpr)
	if !ok {
		return ""
	}
	bindSel, ok := bindCall.Fun.(*ast.SelectorExpr)
	if !ok || bindSel.Sel.Name != "Bind" {
		return ""
	}
	arg := callExpr.Args[0]
	if typed := p.typedSchemaName(pf, arg); typed != "" {
		return typed
	}
	return cleanTypeName(p.schemaNameFromExpr(arg, localTypes))
}

func isBodyBindHelperCall(callExpr *ast.CallExpr, helper *ast.FuncDecl) bool {
	if name := callName(callExpr); name == "bindAndValidate" || name == "bindBody" {
		return true
	}
	if helper == nil || helper.Body == nil {
		return false
	}
	found := false
	ast.Inspect(helper.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if p := requestSchemaFromFiberBindBodyStatic(call); p != "" {
			found = true
			return false
		}
		return true
	})
	return found
}

func requestSchemaFromFiberBindBodyStatic(callExpr *ast.CallExpr) string {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || selExpr.Sel.Name != "Body" || len(callExpr.Args) == 0 {
		return ""
	}
	bindCall, ok := selExpr.X.(*ast.CallExpr)
	if !ok {
		return ""
	}
	bindSel, ok := bindCall.Fun.(*ast.SelectorExpr)
	if !ok || bindSel.Sel.Name != "Bind" {
		return ""
	}
	return "body"
}

func paramTypeFromCall(callExpr *ast.CallExpr) (string, *openapi.Schema) {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if ok && selExpr.Sel.Name == "ParamsInt" && len(callExpr.Args) > 0 {
		if name, ok := parser.ExtractStringLiteral(callExpr.Args[0]); ok {
			return name, &openapi.Schema{Type: "integer"}
		}
	}

	if ok && selExpr.Sel.Name == "Parse" && selectorName(selExpr.X) == "uuid" && len(callExpr.Args) > 0 {
		if name := paramsNameFromExpr(callExpr.Args[0]); name != "" {
			return name, &openapi.Schema{Type: "string", Format: "uuid"}
		}
	}

	if ok && selExpr.Sel.Name == "Atoi" && selectorName(selExpr.X) == "strconv" && len(callExpr.Args) > 0 {
		if name := paramsNameFromExpr(callExpr.Args[0]); name != "" {
			return name, &openapi.Schema{Type: "integer"}
		}
	}

	return "", nil
}

func paramsNameFromExpr(expr ast.Expr) string {
	callExpr, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || selExpr.Sel.Name != "Params" || len(callExpr.Args) == 0 {
		return ""
	}
	name, _ := parser.ExtractStringLiteral(callExpr.Args[0])
	return name
}

func (p *Plugin) schemaNameFromExpr(expr ast.Expr, localTypes map[string]string) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return localTypes[e.Name]
	case *ast.UnaryExpr:
		return p.schemaNameFromExpr(e.X, localTypes)
	case *ast.CallExpr:
		if ident, ok := e.Fun.(*ast.Ident); ok && ident.Name == "new" && len(e.Args) > 0 {
			return astTypeString(e.Args[0])
		}
		if selExpr, ok := e.Fun.(*ast.SelectorExpr); ok {
			return p.returnTypeIndex[selExpr.Sel.Name]
		}
	case *ast.CompositeLit:
		return astTypeString(e.Type)
	case *ast.TypeAssertExpr:
		return astTypeString(e.Type)
	}
	return ""
}

func jsonRequestBody(schemaName string) *openapi.RequestBody {
	return &openapi.RequestBody{
		Required: true,
		Content: map[string]openapi.MediaType{
			"application/json": {
				Schema: schemaRef(schemaName),
			},
		},
	}
}

func methodAllowsRequestBody(method string) bool {
	switch method {
	case "POST", "PUT", "PATCH":
		return true
	default:
		return false
	}
}
