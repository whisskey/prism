// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

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
