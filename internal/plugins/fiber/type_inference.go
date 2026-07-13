// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"
	"strings"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/pkg/openapi"
)

type RequestBodyResolution struct {
	SchemaName string
	Source     string
	PathFields map[string]bool
}

type ParameterResolution struct {
	Name   string
	In     string
	Schema *openapi.Schema
}

type ResponseResolution struct {
	StatusCode string
	Schema     *openapi.Schema
	Example    interface{}
	Source     string
}

// RouteTypeGraph is the Layer 3 output: resolved request/response shapes before route binding.
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
			if !shouldUseEnvelopeErrorSchema(status) {
				response.schema = nil
				response.example = nil
			}
			graph.Responses[status] = ResponseResolution{
				StatusCode: status,
				Schema:     response.schema,
				Example:    response.example,
				Source:     "handler.error",
			}
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
			if response.schema == nil && shouldUseEnvelopeErrorSchema(status) {
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

func (p *Plugin) resolveRequestTypes(route *openapi.Route, analysis RouteAnalysis) (*RequestBodyResolution, []ParameterResolution) {
	var body *RequestBodyResolution
	var params []ParameterResolution
	requestSchemaName := ""

	for _, mw := range analysis.Middleware {
		if mw.ValidatorSchema == "" {
			continue
		}
		requestSchemaName = mw.ValidatorSchema
		if body == nil && methodAllowsRequestBody(route.Method) {
			body = &RequestBodyResolution{
				SchemaName: mw.ValidatorSchema,
				Source:     "middleware.request_validator",
			}
		}
	}

	if analysis.Handler != nil {
		for _, hint := range analysis.Handler.RequestHints {
			if hint.SchemaName == "" {
				continue
			}
			if requestSchemaName == "" {
				requestSchemaName = hint.SchemaName
			}
			if body != nil || !methodAllowsRequestBody(route.Method) {
				continue
			}
			source := "handler.body_parser"
			if hint.Kind == requestHintLocalsPayload {
				source = "handler.locals_payload"
			}
			body = &RequestBodyResolution{
				SchemaName: hint.SchemaName,
				Source:     source,
			}
			requestSchemaName = hint.SchemaName
		}

		for _, callExpr := range analysis.Handler.QueryCalls {
			name, schemaType := queryParamFromCall(callExpr)
			if name == "" {
				continue
			}
			params = appendParameterResolution(params, ParameterResolution{
				Name:   name,
				In:     "query",
				Schema: schemaType,
			})
		}

		for _, schemaName := range analysis.Handler.QueryBindSchemas {
			params = mergeParameterResolutions(params, p.queryParamsFromSchemaName(schemaName))
		}

		for _, callExpr := range analysis.Handler.ParamCalls {
			name, schemaType := paramTypeFromCall(callExpr)
			if name == "" {
				continue
			}
			params = appendParameterResolution(params, ParameterResolution{
				Name:   name,
				In:     "path",
				Schema: schemaType,
			})
		}
	}

	if requestSchemaName != "" {
		graph := p.resolveRequestSchemaGraph(requestSchemaName, route)
		params = mergeParameterResolutions(params, graph.PathParams)
		if body != nil {
			body.PathFields = graph.PathFields
		} else if graph.HasBody && methodAllowsRequestBody(route.Method) {
			body = &RequestBodyResolution{
				SchemaName: requestSchemaName,
				Source:     "middleware.request_validator",
				PathFields: graph.PathFields,
			}
		} else if len(graph.PathFields) > 0 {
			p.storeRequestPathFields(requestSchemaName, graph.PathFields)
		}
	}

	return body, params
}

func (p *Plugin) storeRequestPathFields(schemaName string, pathFields map[string]bool) {
	if schemaName == "" || len(pathFields) == 0 {
		return
	}
	if p.requestPathFields == nil {
		p.requestPathFields = make(map[string]map[string]bool)
	}
	if p.requestPathFields[schemaName] == nil {
		p.requestPathFields[schemaName] = make(map[string]bool)
	}
	for name := range pathFields {
		p.requestPathFields[schemaName][name] = true
	}
}

func appendParameterResolution(existing []ParameterResolution, param ParameterResolution) []ParameterResolution {
	for _, current := range existing {
		if current.Name == param.Name && current.In == param.In {
			return existing
		}
	}
	return append(existing, param)
}

func (p *Plugin) resolveHandlerResponseTypes(route *openapi.Route, analysis *HandlerAnalysis, graph *RouteTypeGraph) {
	if analysis == nil || analysis.FuncDecl == nil || analysis.FuncDecl.Body == nil {
		return
	}

	localTypes := p.buildHandlerLocalTypes(analysis.FuncDecl)

	pf := p.handlerFileIndex[analysis.FuncDecl]

	for _, callExpr := range analysis.ReturnCalls {
		if isExceptionCall(callExpr, "InternalServerError") {
			graph.Responses["500"] = ResponseResolution{
				StatusCode: "500",
				Source:     "handler.exception",
			}
		}
		if code := exceptionStatusFromCall(callExpr); code != "" {
			graph.Responses[code] = ResponseResolution{
				StatusCode: code,
				Source:     "handler.exception",
			}
			continue
		}

		if response := p.responseFromSendStatusCall(callExpr); response != nil && isErrorHTTPStatus(response.code) {
			graph.Responses[response.code] = ResponseResolution{
				StatusCode: response.code,
				Schema:     response.schema,
				Example:    response.example,
				Source:     "handler.send_status",
			}
			continue
		}

		if response := p.inferResponseFromReturnCall(callExpr, localTypes, pf); response != nil && response.schema != nil {
			graph.Responses[response.code] = ResponseResolution{
				StatusCode: response.code,
				Schema:     response.schema,
				Example:    response.example,
				Source:     response.source,
			}
		}
	}
}

type inferredReturnResponse struct {
	code    string
	schema  *openapi.Schema
	example interface{}
	source  string
}

func (p *Plugin) inferResponseFromReturnCall(callExpr *ast.CallExpr, localTypes map[string]string, pf *parser.ParsedFile) *inferredReturnResponse {
	if response := p.responseFromHelperCall(callExpr, localTypes, pf); response != nil {
		source := "handler.response_helper"
		if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
			source = "handler.response_helper." + sel.Sel.Name
		} else if ident, ok := callExpr.Fun.(*ast.Ident); ok {
			source = "handler.response_helper." + ident.Name
		}
		return &inferredReturnResponse{
			code:    response.code,
			schema:  response.schema,
			example: response.example,
			source:  source,
		}
	}

	if response := p.responseFromJSONCall(callExpr, localTypes, pf); response != nil {
		return &inferredReturnResponse{
			code:    response.code,
			schema:  response.schema,
			example: response.example,
			source:  "handler.direct_json",
		}
	}

	return nil
}

func (p *Plugin) buildHandlerLocalTypes(funcDecl *ast.FuncDecl) map[string]string {
	localTypes := make(map[string]string)
	if funcDecl == nil || funcDecl.Body == nil {
		return localTypes
	}
	pf := p.handlerFileIndex[funcDecl]

	astInspectDeclTypes(funcDecl.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.DeclStmt:
			p.collectDeclTypes(n, localTypes)
		case *ast.AssignStmt:
			p.collectAssignTypes(n, localTypes)
			p.enrichLocalTypesFromServiceCalls(n, pf, localTypes)
		}
		return true
	})

	return localTypes
}

func (p *Plugin) enrichLocalTypesFromServiceCalls(stmt *ast.AssignStmt, pf *parser.ParsedFile, localTypes map[string]string) {
	for i, lhs := range stmt.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok {
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

func (p *Plugin) inferReturnTypeFromCall(callExpr *ast.CallExpr) string {
	visited := make(map[*ast.FuncDecl]bool)
	return p.inferReturnTypeFromCallDepth(callExpr, visited, 0)
}

func (p *Plugin) resolveMiddlewareFunc(name string) *ast.FuncDecl {
	if funcDecl, ok := p.routeFuncIndex[name]; ok {
		return funcDecl
	}
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		if funcDecl, ok := p.routeFuncIndex[name[idx+1:]]; ok {
			return funcDecl
		}
	}
	return nil
}

func (p *Plugin) applyRouteTypeGraph(route *openapi.Route, graph RouteTypeGraph) {
	if graph.RequestBody != nil && route.RequestBody == nil && methodAllowsRequestBody(route.Method) {
		p.markSchemaUsed(graph.RequestBody.SchemaName)
		route.RequestBody = jsonRequestBody(graph.RequestBody.SchemaName)
		if len(graph.RequestBody.PathFields) > 0 {
			p.storeRequestPathFields(graph.RequestBody.SchemaName, graph.RequestBody.PathFields)
		} else if graph.RequestBody.Source == "middleware.request_validator" {
			p.rememberRequestPathFields(graph.RequestBody.SchemaName, route.Parameters)
		}
	}
	if route.RequestBody != nil && route.RequestBody.Description == "" {
		route.RequestBody.Description = requestBodyDescription(route.Method, route.Tags)
	}

	existingParams := make(map[string]bool, len(route.Parameters))
	for i := range route.Parameters {
		existingParams[route.Parameters[i].Name+"."+route.Parameters[i].In] = true
	}
	queryParams := make(map[string]*openapi.Schema)
	for _, param := range graph.Parameters {
		if param.In == "path" {
			for i := range route.Parameters {
				if route.Parameters[i].Name == param.Name {
					route.Parameters[i].Schema = param.Schema
				}
			}
			continue
		}
		if param.In == "query" {
			queryParams[param.Name] = param.Schema
		}
	}
	route.Parameters = appendQueryParameters(route.Parameters, queryParams, route.Tags)

	if len(graph.Responses) > 0 {
		if route.Responses == nil {
			route.Responses = make(map[string]openapi.Response)
		}
		for code, resolution := range graph.Responses {
			route.Responses[code] = jsonResponseWithExample(responseDescription(code), resolution.Schema, resolution.Example)
			p.markSchemaRefs(resolution.Schema)
		}
	}
}

func (p *Plugin) applyHandlerMetadata(route *openapi.Route, analysis *HandlerAnalysis) {
	if analysis == nil {
		return
	}

	if route.Summary == "" && analysis.Summary != "" {
		route.Summary = analysis.Summary
	}
	if route.Summary == "" {
		route.Summary = summaryFromHandler(route.Handler, route.Tags)
	}
	if route.Description == "" && analysis.Description != "" {
		route.Description = analysis.Description
	}
	for i := range route.Parameters {
		enrichParameter(&route.Parameters[i], route.Tags)
	}

	// Layer 4 overrides: godoc annotations win when present.
	// Final annotation merge runs in applyAnnotationEscapeHatch after fibergen enrichment.
	if doc := analysis.Doc; doc != nil {
		if doc.Summary != "" {
			route.Summary = doc.Summary
		}
		if doc.Description != "" {
			route.Description = doc.Description
		}
	}
}

func (p *Plugin) applyDocAnnotationOverrides(route *openapi.Route, doc *parser.DocAnnotations) {
	if doc == nil {
		return
	}
	if doc.Summary != "" {
		route.Summary = doc.Summary
	}
	if doc.Description != "" {
		route.Description = doc.Description
	}
	if len(doc.Tags) > 0 {
		route.Tags = append([]string(nil), doc.Tags...)
	}
	if doc.OperationID != "" {
		route.OperationID = doc.OperationID
	}
	if doc.Deprecated {
		route.Deprecated = true
	}
	if len(doc.Security) > 0 {
		route.Security = make([]map[string][]string, 0, len(doc.Security))
		for _, name := range doc.Security {
			route.Security = append(route.Security, map[string][]string{name: {}})
		}
	}
	for _, resp := range doc.Responses {
		if resp.Code == "" {
			continue
		}
		if route.Responses == nil {
			route.Responses = make(map[string]openapi.Response)
		}
		existing, ok := route.Responses[resp.Code]
		if !ok {
			route.Responses[resp.Code] = openapi.Response{Description: responseDescription(resp.Code)}
			existing = route.Responses[resp.Code]
		}
		if resp.Description != "" {
			existing.Description = resp.Description
		}
		if schemaName := docResponseSchemaName(resp.Type); schemaName != "" && existing.Content == nil {
			existing.Content = map[string]openapi.MediaType{
				"application/json": {
					Schema: schemaRef(schemaName),
				},
			}
		}
		route.Responses[resp.Code] = existing
	}
}

func docResponseSchemaName(typeName string) string {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return ""
	}
	switch strings.ToLower(typeName) {
	case "object", "array", "string", "integer", "number", "boolean":
		return ""
	}
	if idx := strings.Index(typeName, "{"); idx > 0 {
		typeName = typeName[:idx]
	}
	return cleanTypeName(typeName)
}
