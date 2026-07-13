// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package echo

import (
	"strings"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/pkg/openapi"
)

type requestSchemaGraph struct {
	SchemaName string
	PathFields map[string]bool
	PathParams []ParameterResolution
	HasBody    bool
}

func (p *Plugin) resolveRequestSchemaGraph(schemaName string, route *openapi.Route) requestSchemaGraph {
	if p.structIndex == nil {
		p.structIndex = make(map[string]parser.StructDefinition)
	}

	graph := requestSchemaGraph{
		SchemaName: schemaName,
		PathFields: make(map[string]bool),
	}

	def, ok := p.structIndex[schemaName]
	if !ok {
		p.applyLegacyRequestPathHeuristics(schemaName, route, &graph)
		return graph
	}

	for _, field := range def.Fields {
		if field.URIName != "" {
			graph.PathParams = appendParameterResolution(graph.PathParams, ParameterResolution{
				Name:   field.URIName,
				In:     "path",
				Schema: p.schemaExtractor.FieldSchema(field),
			})
			p.markPathField(graph.PathFields, field.URIName)
			p.markPathField(graph.PathFields, field.JSONName)
			p.markPathField(graph.PathFields, field.Name)
			continue
		}
		if field.JSONName != "-" {
			graph.HasBody = true
		}
	}

	if len(graph.PathParams) == 0 {
		p.applyLegacyRequestPathHeuristics(schemaName, route, &graph)
	}

	return graph
}

func (p *Plugin) applyLegacyRequestPathHeuristics(schemaName string, route *openapi.Route, graph *requestSchemaGraph) {
	if !schemaLooksLikeIDParam(schemaName) && !(len(route.Parameters) > 0 && strings.Contains(schemaName, "Update")) {
		return
	}
	for _, param := range route.Parameters {
		if param.In != "path" {
			continue
		}
		graph.PathParams = appendParameterResolution(graph.PathParams, ParameterResolution{
			Name:   param.Name,
			In:     "path",
			Schema: p.defaultPathParamSchema(schemaName, param.Name),
		})
		p.markPathField(graph.PathFields, param.Name)
	}
}

func (p *Plugin) defaultPathParamSchema(schemaName, paramName string) *openapi.Schema {
	if def, ok := p.structIndex[schemaName]; ok {
		for _, field := range def.Fields {
			if field.URIName == paramName || strings.EqualFold(field.Name, paramName) {
				return p.schemaExtractor.FieldSchema(field)
			}
		}
	}
	return &openapi.Schema{Type: "integer"}
}

func (p *Plugin) markPathField(fields map[string]bool, name string) {
	if name == "" || name == "-" {
		return
	}
	fields[name] = true
	fields[strings.ToLower(name)] = true
	fields[strings.ToUpper(name)] = true
	if len(name) > 0 {
		fields[strings.ToUpper(name[:1])+name[1:]] = true
	}
}

func mergeParameterResolutions(existing, incoming []ParameterResolution) []ParameterResolution {
	for _, param := range incoming {
		existing = appendParameterResolution(existing, param)
	}
	return existing
}

func appendParameterResolution(existing []ParameterResolution, param ParameterResolution) []ParameterResolution {
	for _, current := range existing {
		if current.Name == param.Name && current.In == param.In {
			return existing
		}
	}
	return append(existing, param)
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
			source := "handler.body_decode"
			if hint.Kind == requestHintContextPayload {
				source = "handler.context_payload"
			}
			body = &RequestBodyResolution{
				SchemaName: hint.SchemaName,
				Source:     source,
			}
			requestSchemaName = hint.SchemaName
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
		if body != nil && len(graph.PathFields) > 0 {
			p.rememberRequestPathFields(requestSchemaName, pathParamsFromResolutions(graph.PathParams))
		} else if len(graph.PathFields) > 0 {
			p.storeRequestPathFields(requestSchemaName, graph.PathFields)
		}
	}

	return body, params
}

func pathParamsFromResolutions(params []ParameterResolution) []openapi.Parameter {
	result := make([]openapi.Parameter, 0, len(params))
	for _, param := range params {
		if param.In != "path" {
			continue
		}
		result = append(result, openapi.Parameter{
			Name:   param.Name,
			In:     param.In,
			Schema: param.Schema,
		})
	}
	return result
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
