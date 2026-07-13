// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package echo

import (
	"go/ast"
	"go/token"
	"path/filepath"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/internal/typecheck"
	"github.com/whisskey/prism/pkg/openapi"
)

type ResponseResolution struct {
	StatusCode string
	Schema     *openapi.Schema
	Example    interface{}
	Source     string
}

type ParameterResolution struct {
	Name   string
	In     string
	Schema *openapi.Schema
}

type RequestBodyResolution struct {
	SchemaName string
	Source     string
}

type RouteTypeGraph struct {
	RequestBody *RequestBodyResolution
	Parameters  []ParameterResolution
	Responses   map[string]ResponseResolution
}

func (p *Plugin) resolveRouteTypeGraph(route *openapi.Route, analysis RouteAnalysis) RouteTypeGraph {
	graph := RouteTypeGraph{
		Responses: make(map[string]ResponseResolution),
	}
	graph.RequestBody, graph.Parameters = p.resolveRequestTypes(route, analysis)
	if analysis.Handler != nil {
		p.resolveHandlerResponseTypes(route, analysis.Handler, &graph)
		for _, status := range analysis.Handler.ErrorStatuses {
			if !isErrorHTTPStatus(status) {
				continue
			}
			if _, exists := graph.Responses[status]; exists {
				continue
			}
			response := p.inferredErrorResponse(status)
			graph.Responses[status] = ResponseResolution{
				StatusCode: status,
				Schema:     response.schema,
				Example:    response.example,
				Source:     "handler.error",
			}
		}
		for _, param := range analysis.Handler.QueryParams {
			graph.Parameters = append(graph.Parameters, param)
		}
	}
	for _, mw := range analysis.Middleware {
		for _, status := range mw.ErrorStatuses {
			if !isErrorHTTPStatus(status) {
				continue
			}
			if _, exists := graph.Responses[status]; exists {
				continue
			}
			response := p.errorResponseFromFunc(mw.FuncDecl, status)
			if response.schema == nil {
				response = p.inferredErrorResponse(status)
			}
			graph.Responses[status] = ResponseResolution{
				StatusCode: status,
				Schema:     response.schema,
				Example:    response.example,
				Source:     "middleware.error",
			}
		}
	}
	return graph
}

func (p *Plugin) resolveHandlerResponseTypes(route *openapi.Route, handler *HandlerAnalysis, graph *RouteTypeGraph) {
	if handler == nil || handler.FuncDecl == nil || handler.FuncDecl.Body == nil {
		return
	}
	pf := p.handlerFileIndex[handler.FuncDecl]
	localTypes := p.buildHandlerLocalTypes(handler.FuncDecl)
	ast.Inspect(handler.FuncDecl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if resp := p.responseFromHelperCall(node, localTypes, pf); resp != nil && resp.schema != nil {
				if _, exists := graph.Responses[resp.code]; !exists {
					graph.Responses[resp.code] = ResponseResolution{
						StatusCode: resp.code,
						Schema:     resp.schema,
						Example:    p.exampleFromSchema(resp.schema),
						Source:     "handler.helper",
					}
				}
			}
			if resp := p.responseFromJSONCall(node, localTypes, pf); resp != nil && resp.schema != nil {
				if _, exists := graph.Responses[resp.code]; !exists {
					graph.Responses[resp.code] = ResponseResolution{
						StatusCode: resp.code,
						Schema:     resp.schema,
						Example:    p.exampleFromSchema(resp.schema),
						Source:     "handler.json",
					}
				}
			}
		}
		return true
	})
}

func (p *Plugin) buildHandlerLocalTypes(funcDecl *ast.FuncDecl) map[string]string {
	localTypes := make(map[string]string)
	if funcDecl == nil || funcDecl.Body == nil {
		return localTypes
	}
	pf := p.handlerFileIndex[funcDecl]
	ast.Inspect(funcDecl.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.DeclStmt:
			p.collectDeclTypes(n, localTypes)
		case *ast.AssignStmt:
			p.collectAssignTypes(n, localTypes, pf)
			p.enrichLocalTypesFromServiceCalls(n, pf, localTypes)
		}
		return true
	})
	return localTypes
}

func (p *Plugin) enrichLocalTypesFromServiceCalls(stmt *ast.AssignStmt, pf *parser.ParsedFile, localTypes map[string]string) {
	for i, lhs := range stmt.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok || ident.Name == "_" {
			continue
		}
		rhsIndex := i
		if len(stmt.Rhs) == 1 {
			rhsIndex = 0
		}
		if rhsIndex >= len(stmt.Rhs) {
			continue
		}
		callExpr, ok := stmt.Rhs[rhsIndex].(*ast.CallExpr)
		if !ok {
			continue
		}
		returnType := p.inferReturnTypeFromCall(callExpr)
		if returnType == "" && pf != nil {
			returnType = p.typedCallResultSchema(pf, callExpr)
		}
		if returnType == "" {
			continue
		}
		localTypes[ident.Name] = returnType
	}
}

func (p *Plugin) collectAssignTypes(assign *ast.AssignStmt, localTypes map[string]string, pf *parser.ParsedFile) {
	if len(assign.Lhs) == 0 || len(assign.Rhs) == 0 {
		return
	}
	ident, ok := assign.Lhs[0].(*ast.Ident)
	if !ok {
		return
	}
	switch rhs := assign.Rhs[0].(type) {
	case *ast.CompositeLit:
		if t, ok := rhs.Type.(*ast.ArrayType); ok {
			localTypes[ident.Name] = "[]" + astTypeString(t.Elt)
			return
		}
		if name := typeBaseName(rhs.Type); name != "" {
			localTypes[ident.Name] = name
		}
	case *ast.Ident:
		if schemaName := localTypes[rhs.Name]; schemaName != "" {
			localTypes[ident.Name] = schemaName
		}
	case *ast.CallExpr:
		if returnType := p.inferReturnTypeFromCall(rhs); returnType != "" {
			localTypes[ident.Name] = cleanTypeName(returnType)
		}
	}
	if pf != nil {
		line := positionLine(pf, ident.Pos())
		if schemaName := p.typedIdentSchema(pf, line, ident.Name); schemaName != "" {
			localTypes[ident.Name] = schemaName
		}
	}
}

func (p *Plugin) collectDeclTypes(decl *ast.DeclStmt, localTypes map[string]string) {
	gen, ok := decl.Decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.VAR {
		return
	}
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		typeName := astTypeString(vs.Type)
		if typeName == "" {
			continue
		}
		for _, name := range vs.Names {
			localTypes[name.Name] = typeName
		}
	}
}

func (p *Plugin) applyRouteTypeGraph(route *openapi.Route, graph RouteTypeGraph) {
	if route == nil {
		return
	}
	if route.Responses == nil {
		route.Responses = make(map[string]openapi.Response)
	}
	for code, resp := range graph.Responses {
		if _, exists := route.Responses[code]; exists {
			continue
		}
		route.Responses[code] = jsonResponseWithExample(responseDescription(code), resp.Schema, resp.Example)
		p.markSchemaRefs(resp.Schema)
	}
	existing := make(map[string]bool, len(route.Parameters))
	for i, param := range route.Parameters {
		existing[param.Name+"."+param.In] = true
		_ = i
	}
	for _, param := range graph.Parameters {
		key := param.Name + "." + param.In
		if param.In == "path" {
			upgraded := false
			for i := range route.Parameters {
				if route.Parameters[i].Name == param.Name && route.Parameters[i].In == "path" {
					if param.Schema != nil {
						route.Parameters[i].Schema = param.Schema
					}
					existing[key] = true
					upgraded = true
					break
				}
			}
			if upgraded {
				continue
			}
		}
		if existing[key] {
			continue
		}
		routeParam := openapi.Parameter{
			Name:   param.Name,
			In:     param.In,
			Schema: param.Schema,
		}
		enrichQueryParameter(&routeParam)
		route.Parameters = append(route.Parameters, routeParam)
		existing[key] = true
	}
	if graph.RequestBody != nil && route.RequestBody == nil && methodAllowsRequestBody(route.Method) {
		p.markSchemaUsed(graph.RequestBody.SchemaName)
		route.RequestBody = jsonRequestBody(graph.RequestBody.SchemaName)
	}
}

func (p *Plugin) typedIdentSchema(pf *parser.ParsedFile, line int, name string) string {
	if p.typeProgram == nil || pf == nil || name == "" {
		return ""
	}
	path, _ := filepath.Abs(pf.Path)
	t := p.typeProgram.TypeOfIdent(path, line, name)
	if t == nil {
		return ""
	}
	return typecheck.SchemaName(t)
}

func positionLine(pf *parser.ParsedFile, pos token.Pos) int {
	if pf == nil || pf.FileSet == nil {
		return 0
	}
	return pf.FileSet.Position(pos).Line
}

func isErrorHTTPStatus(code string) bool {
	switch code {
	case "400", "401", "403", "404", "409", "422", "429", "500", "502", "503":
		return true
	default:
		return false
	}
}
