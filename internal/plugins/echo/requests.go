// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package echo

import (
	"go/ast"
	"strings"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/pkg/openapi"
)

type requestHintKind string

const (
	requestHintBodyDecode      requestHintKind = "body_decode"
	requestHintContextPayload  requestHintKind = "context_payload"
)

type requestHint struct {
	Kind       requestHintKind
	SchemaName string
}

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
				if _, exists := route.Responses[status]; exists {
					continue
				}
				response := p.errorResponseFromFunc(p.resolveMiddlewareFuncDecl(callExpr, nil), status)
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
				if route.Parameters[i].In == "path" {
					route.Parameters[i].Schema = &openapi.Schema{Type: "integer"}
				}
			}
		}
	}
}

func (p *Plugin) middlewareStatuses(expr ast.Expr) []string {
	callExpr, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil
	}
	funcDecl := p.resolveMiddlewareFuncDecl(callExpr, nil)
	if funcDecl == nil || funcDecl.Body == nil {
		return nil
	}
	return writeHeaderErrorStatusesFromFunc(funcDecl)
}

func (p *Plugin) middlewareBodySchemaFromFuncDecl(funcDecl *ast.FuncDecl) string {
	if funcDecl == nil || funcDecl.Body == nil {
		return ""
	}

	localTypes := make(map[string]string)
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.DeclStmt:
			p.collectDeclTypesForRequest(node, localTypes)
		case *ast.AssignStmt:
			p.collectAssignTypesForRequest(node, localTypes)
		}
		return true
	})

	var schemaName string
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		if schemaName != "" {
			return false
		}
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if name := p.schemaFromValidatorCall(callExpr, localTypes); name != "" {
			schemaName = name
			return false
		}
		return true
	})
	return schemaName
}

func (p *Plugin) schemaFromValidatorCall(callExpr *ast.CallExpr, localTypes map[string]string) string {
	if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
		switch selExpr.Sel.Name {
		case "Bind":
			if len(callExpr.Args) > 0 {
				return cleanTypeName(p.schemaNameFromExpr(callExpr.Args[0], localTypes))
			}
		case "Decode":
			if len(callExpr.Args) > 0 {
				return cleanTypeName(p.schemaNameFromExpr(callExpr.Args[0], localTypes))
			}
		case "Unmarshal":
			if len(callExpr.Args) >= 2 {
				return cleanTypeName(p.schemaNameFromExpr(callExpr.Args[1], localTypes))
			}
		case "DecodeJSON":
			if len(callExpr.Args) >= 2 {
				return cleanTypeName(p.schemaNameFromExpr(callExpr.Args[1], localTypes))
			}
		}
	}
	switch callNameFromExpr(callExpr) {
	case "bindAndValidate", "bindBody":
		if len(callExpr.Args) >= 2 {
			if name := cleanTypeName(p.schemaNameFromExpr(callExpr.Args[1], localTypes)); name != "" {
				return name
			}
		}
		if len(callExpr.Args) >= 1 {
			return cleanTypeName(p.schemaNameFromExpr(callExpr.Args[0], localTypes))
		}
	}
	return ""
}

func (p *Plugin) requestSchemaFromBind(callExpr *ast.CallExpr, localTypes map[string]string) string {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || selExpr.Sel.Name != "Bind" || len(callExpr.Args) == 0 {
		return ""
	}
	return cleanTypeName(p.schemaNameFromExpr(callExpr.Args[0], localTypes))
}

func (p *Plugin) requestSchemaFromBodyDecode(callExpr *ast.CallExpr, localTypes map[string]string) string {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || selExpr.Sel.Name != "Decode" || len(callExpr.Args) == 0 {
		return ""
	}
	return cleanTypeName(p.schemaNameFromExpr(callExpr.Args[0], localTypes))
}

func (p *Plugin) requestSchemaFromUnmarshal(callExpr *ast.CallExpr, localTypes map[string]string) string {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || selExpr.Sel.Name != "Unmarshal" || len(callExpr.Args) < 2 {
		return ""
	}
	return cleanTypeName(p.schemaNameFromExpr(callExpr.Args[1], localTypes))
}

func (p *Plugin) requestSchemaFromRenderDecode(callExpr *ast.CallExpr, localTypes map[string]string) string {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || selExpr.Sel.Name != "DecodeJSON" || len(callExpr.Args) < 2 {
		return ""
	}
	return cleanTypeName(p.schemaNameFromExpr(callExpr.Args[1], localTypes))
}

func requestHintFromAssign(stmt *ast.AssignStmt) requestHint {
	if len(stmt.Rhs) != 1 {
		return requestHint{}
	}

	typeAssert, ok := stmt.Rhs[0].(*ast.TypeAssertExpr)
	if !ok {
		return requestHint{}
	}

	contextCall, ok := typeAssert.X.(*ast.CallExpr)
	if !ok {
		return requestHint{}
	}

	selExpr, ok := contextCall.Fun.(*ast.SelectorExpr)
	if !ok || selExpr.Sel.Name != "Get" || len(contextCall.Args) == 0 {
		return requestHint{}
	}

	key, ok := parser.ExtractStringLiteral(contextCall.Args[0])
	if !ok || !isContextPayloadKey(key) {
		return requestHint{}
	}

	schemaName := cleanTypeName(astTypeString(typeAssert.Type))
	if schemaName == "" {
		return requestHint{}
	}

	return requestHint{
		Kind:       requestHintContextPayload,
		SchemaName: schemaName,
	}
}

func paramTypeFromCall(callExpr *ast.CallExpr) (string, *openapi.Schema) {
	if name, schema := urlParamFromCall(callExpr); name != "" {
		return name, schema
	}

	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", nil
	}

	if selExpr.Sel.Name == "ParseInt" && selectorName(selExpr.X) == "strconv" && len(callExpr.Args) > 0 {
		if name := paramsNameFromExpr(callExpr.Args[0]); name != "" {
			return name, &openapi.Schema{Type: "integer"}
		}
	}

	if selExpr.Sel.Name == "Atoi" && selectorName(selExpr.X) == "strconv" && len(callExpr.Args) > 0 {
		if name := paramsNameFromExpr(callExpr.Args[0]); name != "" {
			return name, &openapi.Schema{Type: "integer"}
		}
	}

	if selExpr.Sel.Name == "Parse" && selectorName(selExpr.X) == "uuid" && len(callExpr.Args) > 0 {
		if name := paramsNameFromExpr(callExpr.Args[0]); name != "" {
			return name, &openapi.Schema{Type: "string", Format: "uuid"}
		}
	}

	return "", nil
}

func urlParamFromCall(callExpr *ast.CallExpr) (string, *openapi.Schema) {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || len(callExpr.Args) == 0 {
		return "", nil
	}
	name, ok := parser.ExtractStringLiteral(callExpr.Args[0])
	if !ok || name == "" {
		return "", nil
	}
	switch selExpr.Sel.Name {
	case "Param":
		return name, &openapi.Schema{Type: "string"}
	case "ParamInt":
		return name, &openapi.Schema{Type: "integer"}
	default:
		return "", nil
	}
}

func paramsNameFromExpr(expr ast.Expr) string {
	return urlParamNameFromExpr(expr)
}

func urlParamNameFromExpr(expr ast.Expr) string {
	callExpr, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || len(callExpr.Args) == 0 {
		return ""
	}
	switch selExpr.Sel.Name {
	case "Param", "ParamInt":
		if len(callExpr.Args) < 1 {
			return ""
		}
		name, _ := parser.ExtractStringLiteral(callExpr.Args[0])
		return name
	default:
		return ""
	}
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

func (p *Plugin) collectDeclTypesForRequest(stmt *ast.DeclStmt, localTypes map[string]string) {
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

func (p *Plugin) collectAssignTypesForRequest(stmt *ast.AssignStmt, localTypes map[string]string) {
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

func (p *Plugin) rememberRequestPathFields(schemaName string, params []openapi.Parameter) {
	if schemaName == "" || len(params) == 0 {
		return
	}
	if p.requestPathFields == nil {
		p.requestPathFields = make(map[string]map[string]bool)
	}
	if p.requestPathFields[schemaName] == nil {
		p.requestPathFields[schemaName] = make(map[string]bool)
	}
	for _, param := range params {
		if param.In != "path" {
			continue
		}
		p.requestPathFields[schemaName][param.Name] = true
		p.requestPathFields[schemaName][strings.ToLower(param.Name)] = true
		p.requestPathFields[schemaName][strings.ToUpper(param.Name)] = true
		if len(param.Name) > 0 {
			p.requestPathFields[schemaName][strings.ToUpper(param.Name[:1])+param.Name[1:]] = true
		}
	}
}
