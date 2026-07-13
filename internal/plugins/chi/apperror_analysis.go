// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package chi

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/whisskey/prism/pkg/openapi"
)

func (p *Plugin) collectAppErrorStatusesFromFunc(funcDecl *ast.FuncDecl, visited map[*ast.FuncDecl]bool) []string {
	if funcDecl == nil || funcDecl.Body == nil || visited[funcDecl] {
		return nil
	}
	visited[funcDecl] = true
	seen := make(map[string]struct{})
	var statuses []string
	add := func(code string) {
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
		add(apperrorStatusFromCall(callExpr))
		return true
	})
	return statuses
}

func apperrorStatusFromCall(callExpr *ast.CallExpr) string {
	if callExpr == nil {
		return ""
	}
	sel, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg := ""
	if ident, ok := sel.X.(*ast.Ident); ok {
		pkg = ident.Name
	}
	if pkg != "apperror" {
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

func writeHeaderErrorStatusesFromFunc(funcDecl *ast.FuncDecl) []string {
	if funcDecl == nil || funcDecl.Body == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var statuses []string
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "WriteHeader" || len(callExpr.Args) == 0 {
			return true
		}
		code := statusCodeFromExpr(callExpr.Args[0])
		if !isErrorHTTPStatus(code) {
			return true
		}
		if _, ok := seen[code]; ok {
			return true
		}
		seen[code] = struct{}{}
		statuses = append(statuses, code)
		return true
	})
	return statuses
}

func statusCodeFromExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.INT {
			return e.Value
		}
	case *ast.SelectorExpr:
		name := e.Sel.Name
		switch name {
		case "StatusBadRequest":
			return "400"
		case "StatusUnauthorized":
			return "401"
		case "StatusForbidden":
			return "403"
		case "StatusNotFound":
			return "404"
		case "StatusUnprocessableEntity":
			return "422"
		case "StatusInternalServerError":
			return "500"
		}
	case *ast.Ident:
		switch e.Name {
		case "StatusBadRequest":
			return "400"
		case "StatusUnauthorized":
			return "401"
		case "StatusForbidden":
			return "403"
		case "StatusNotFound":
			return "404"
		}
	}
	return ""
}

func (p *Plugin) standardEnvelopeErrorSchema() *openapi.Schema {
	if schema, ok := p.registeredResponseSchema("Response"); ok {
		return schema
	}
	if schema, ok := p.registeredResponseSchema("ErrorResponse"); ok {
		return schema
	}
	return p.responseSchemaFromType("ErrorResponse")
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
	return &inferredResponse{code: status, schema: schema}
}

func (p *Plugin) errorResponseFromFunc(funcDecl *ast.FuncDecl, status string) *inferredResponse {
	if funcDecl == nil {
		return &inferredResponse{code: status}
	}
	return p.inferredErrorResponse(status)
}

func queryParamFromCall(callExpr *ast.CallExpr) (string, *openapi.Schema) {
	sel, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Get" || len(callExpr.Args) == 0 {
		return "", nil
	}
	// r.URL.Query().Get("page")
	queryCall, ok := sel.X.(*ast.CallExpr)
	if !ok {
		return "", nil
	}
	querySel, ok := queryCall.Fun.(*ast.SelectorExpr)
	if !ok || querySel.Sel.Name != "Query" {
		return "", nil
	}
	name, ok := parserExtractString(callExpr.Args[0])
	if !ok || name == "" {
		return "", nil
	}
	return name, &openapi.Schema{Type: "string"}
}

func parserExtractString(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return v, true
}

func (p *Plugin) parametersFromQuerySchema(schemaName string) []ParameterResolution {
	def, ok := p.structIndex[schemaName]
	if !ok {
		return nil
	}
	var params []ParameterResolution
	for _, field := range def.Fields {
		name := field.QueryName
		if name == "" {
			name = field.JSONName
		}
		if name == "" || name == "-" {
			continue
		}
		schema := &openapi.Schema{Type: "string"}
		switch field.Type {
		case "int", "int64", "int32":
			schema.Type = "integer"
		case "bool":
			schema.Type = "boolean"
		}
		params = append(params, ParameterResolution{Name: name, In: "query", Schema: schema})
	}
	return params
}

func callNameFromExpr(callExpr *ast.CallExpr) string {
	switch fn := callExpr.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	default:
		return ""
	}
}
