// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"strings"

	"github.com/whisskey/prism/internal/analyzer/fibergen"
	"github.com/whisskey/prism/pkg/openapi"
)

func (p *Plugin) supplementFibergenRoutes(bundles []discoveredRouteBundle) []discoveredRouteBundle {
	merged := p.fibergenMerged()
	if merged == nil {
		return bundles
	}

	existing := make(map[string]struct{}, len(bundles))
	for _, bundle := range bundles {
		existing[routeOperationKey(bundle.Route.Method, bundle.Route.Path)] = struct{}{}
	}

	for _, route := range merged.Routes {
		key := routeOperationKey(route.Method, route.Path)
		if _, ok := existing[key]; ok {
			continue
		}
		bundles = append(bundles, discoveredRouteBundle{
			Route: openapi.Route{
				Method:     strings.ToUpper(route.Method),
				Path:       route.Path,
				Handler:    route.Handler,
				SourceFile: route.SourceFile,
				SourceLine: route.SourceLine,
				HandlerRef: openapi.HandlerRef{
					Qualified: route.Handler,
					Method:    handlerMethodName(route.Handler),
					Resolved:  route.Handler != "",
				},
				Middleware: middlewareRefsFromNames(route.Middlewares, route.SourceFile, route.SourceLine),
			},
		})
	}
	return bundles
}

func (p *Plugin) applyFibergenEnrichment(route *openapi.Route) {
	merged := p.fibergenMerged()
	if merged == nil {
		return
	}

	p.applyFibergenSecurity(route, merged)
	p.applyFibergenHandlerDetail(route, merged)
	p.applyNamedMiddlewareInference(route, p.analyzeNamedMiddlewareChain(route.Middleware))
}

func (p *Plugin) applyFibergenSecurity(route *openapi.Route, merged *fibergen.MergedData) {
	if route == nil || merged == nil || routeHasBearerAuth(route) {
		return
	}
	key := routeOperationKey(route.Method, route.Path)
	reqs, ok := merged.Security[key]
	if !ok || len(reqs) == 0 {
		return
	}
	for _, req := range reqs {
		if req.Scheme == "" {
			continue
		}
		if routeHasSecurityScheme(route, req.Scheme) {
			continue
		}
		route.Security = append(route.Security, map[string][]string{req.Scheme: req.Scopes})
	}
}

func (p *Plugin) applyFibergenHandlerDetail(route *openapi.Route, merged *fibergen.MergedData) {
	if route == nil || merged == nil {
		return
	}

	detail, ok := merged.LookupHandler(
		route.HandlerRef.Qualified,
		route.Handler,
		route.HandlerRef.Method,
	)
	if !ok {
		return
	}

	if route.RequestBody == nil && detail.RequestBody != "" && methodAllowsRequestBody(route.Method) {
		schemaName := cleanTypeName(strings.TrimPrefix(detail.RequestBody, "[]"))
		if schemaName != "" {
			p.markSchemaUsed(schemaName)
			route.RequestBody = jsonRequestBody(schemaName)
		}
	}

	if route.Responses == nil {
		route.Responses = make(map[string]openapi.Response)
	}
	p.applyFibergenResponse(route, "200", detail.ResponseType)
	for code, schemaName := range detail.ResponseSchemas {
		p.applyFibergenResponse(route, code, schemaName)
	}

	route.Parameters = appendFibergenParams(route.Parameters, detail.PathParams, "path")
	route.Parameters = appendFibergenParams(route.Parameters, detail.QueryParams, "query")

	if !route.HandlerRef.Resolved && detail.Name != "" {
		route.HandlerRef.Resolved = true
		route.HandlerRef.UnresolvedReason = ""
		if route.HandlerRef.SourceFile == "" {
			route.HandlerRef.SourceFile = detail.SourceFile
			route.HandlerRef.SourceLine = detail.SourceLine
		}
	}
}

func (p *Plugin) applyFibergenResponse(route *openapi.Route, code, schemaName string) {
	if route == nil || code == "" || schemaName == "" {
		return
	}
	if _, exists := route.Responses[code]; exists {
		return
	}
	name := cleanTypeName(strings.TrimPrefix(schemaName, "[]"))
	if name == "" {
		return
	}
	p.markSchemaUsed(name)
	var schema *openapi.Schema
	if strings.HasPrefix(schemaName, "[]") {
		schema = &openapi.Schema{Type: "array", Items: schemaRef(name)}
	} else {
		schema = schemaRef(name)
	}
	route.Responses[code] = jsonResponseWithExample(responseDescription(code), schema, nil)
}

func (p *Plugin) registerFibergenSchemas() {
	merged := p.fibergenMerged()
	if merged == nil || p.schemaExtractor == nil {
		return
	}
	registry := p.schemaExtractor.Registry()
	for name, fgSchema := range merged.Schemas {
		if _, exists := registry.Get(name); exists {
			continue
		}
		if converted := convertFibergenSchema(name, fgSchema); converted != nil {
			registry.Add(name, converted)
		}
	}
}

func (p *Plugin) fibergenMerged() *fibergen.MergedData {
	if p.fibergenCatalog == nil || p.fibergenCatalog.Merged == nil {
		return nil
	}
	return p.fibergenCatalog.Merged
}

func convertFibergenSchema(name string, fg fibergen.Schema) *openapi.Schema {
	schema := &openapi.Schema{
		Title:  name,
		Type:   fg.Type,
		Format: fg.Format,
		Ref:    fg.Ref,
	}
	if len(fg.Required) > 0 {
		schema.Required = append([]string(nil), fg.Required...)
	}
	if len(fg.Properties) == 0 {
		if schema.Type == "" {
			schema.Type = "object"
		}
		return schema
	}
	schema.Properties = make(map[string]*openapi.Schema, len(fg.Properties))
	for propName, prop := range fg.Properties {
		schema.Properties[propName] = convertFibergenProperty(prop)
	}
	if schema.Type == "" {
		schema.Type = "object"
	}
	return schema
}

func convertFibergenProperty(prop fibergen.Property) *openapi.Schema {
	s := &openapi.Schema{
		Type:   prop.Type,
		Format: prop.Format,
		Ref:    prop.Ref,
	}
	if prop.Minimum != nil {
		s.Minimum = prop.Minimum
	}
	if prop.Maximum != nil {
		s.Maximum = prop.Maximum
	}
	if prop.MinLength != nil {
		s.MinLength = prop.MinLength
	}
	if prop.MaxLength != nil {
		s.MaxLength = prop.MaxLength
	}
	if len(prop.Enum) > 0 {
		s.Enum = make([]interface{}, len(prop.Enum))
		for i, v := range prop.Enum {
			s.Enum[i] = v
		}
	}
	if prop.Items != nil {
		s.Type = "array"
		s.Items = convertFibergenSchema("", *prop.Items)
	}
	return s
}

func appendFibergenParams(existing []openapi.Parameter, params []fibergen.Param, in string) []openapi.Parameter {
	if len(params) == 0 {
		return existing
	}
	seen := make(map[string]bool, len(existing))
	for _, param := range existing {
		seen[param.Name+"."+param.In] = true
	}
	for _, param := range params {
		location := in
		if location == "" {
			location = "query"
		}
		key := param.Name + "." + location
		if seen[key] {
			continue
		}
		existing = append(existing, openapi.Parameter{
			Name:     param.Name,
			In:       location,
			Required: param.Required || location == "path",
			Schema:   openAPISchemaForGoType(param.GoType),
		})
		seen[key] = true
	}
	return existing
}

func openAPISchemaForGoType(goType string) *openapi.Schema {
	switch goType {
	case "integer":
		return &openapi.Schema{Type: "integer"}
	case "number":
		return &openapi.Schema{Type: "number"}
	case "boolean":
		return &openapi.Schema{Type: "boolean"}
	default:
		return &openapi.Schema{Type: "string"}
	}
}

func middlewareRefsFromNames(names []string, sourceFile string, sourceLine int) []openapi.MiddlewareRef {
	if len(names) == 0 {
		return nil
	}
	refs := make([]openapi.MiddlewareRef, 0, len(names))
	for _, name := range names {
		refs = append(refs, openapi.MiddlewareRef{
			Name:       name,
			SourceFile: sourceFile,
			SourceLine: sourceLine,
		})
	}
	return refs
}

func routeOperationKey(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

func routeHasSecurityScheme(route *openapi.Route, scheme string) bool {
	for _, req := range route.Security {
		if _, ok := req[scheme]; ok {
			return true
		}
	}
	return false
}

func handlerMethodName(handler string) string {
	if idx := strings.LastIndex(handler, "."); idx >= 0 {
		return handler[idx+1:]
	}
	return handler
}
