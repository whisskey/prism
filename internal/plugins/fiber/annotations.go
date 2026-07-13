// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"strings"

	"github.com/whisskey/prism/internal/analyzer/fibergen"
	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/pkg/openapi"
)

// applyAnnotationEscapeHatch applies godoc/swag annotations after static inference.
// Annotated values win over inferred request bodies, responses, and metadata.
func (p *Plugin) applyAnnotationEscapeHatch(route *openapi.Route, handler *HandlerAnalysis) {
	if route == nil {
		return
	}
	doc := p.handlerDocFromAnalysis(route, handler)
	if doc == nil {
		return
	}

	p.applyDocAnnotationOverrides(route, doc)
	p.mergeParametersWithAnnotations(route, doc)
	p.mergeRequestBodyWithAnnotations(route, doc)
	p.mergeResponsesWithAnnotations(route, doc)
}

func (p *Plugin) handlerDocFromAnalysis(route *openapi.Route, handler *HandlerAnalysis) *parser.DocAnnotations {
	if handler != nil && handler.Doc != nil {
		return handler.Doc
	}
	return p.handlerDocForRoute(route)
}

func (p *Plugin) handlerDocForRoute(route *openapi.Route) *parser.DocAnnotations {
	if route == nil {
		return nil
	}
	candidates := []string{
		route.HandlerRef.Method,
		route.Handler,
		route.HandlerRef.Qualified,
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if doc := p.findHandlerDoc(candidate); doc != nil {
			return doc
		}
	}
	return nil
}

func (p *Plugin) findHandlerDoc(handlerName string) *parser.DocAnnotations {
	name := handlerName
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	if fn, ok := p.routeFuncIndex[name]; ok {
		if pf := p.handlerFileIndex[fn]; pf != nil {
			return p.goParser.FindHandlerDoc(pf, name)
		}
	}
	return nil
}

func (p *Plugin) mergeRequestBodyWithAnnotations(route *openapi.Route, doc *parser.DocAnnotations) {
	if route == nil || doc == nil || !methodAllowsRequestBody(route.Method) {
		return
	}
	schemaName := fibergen.PreferAnnotation(inferredRequestBodyName(route), annotatedRequestBodyName(doc))
	if schemaName == "" {
		return
	}
	existingDesc := ""
	if route.RequestBody != nil {
		existingDesc = route.RequestBody.Description
	}
	p.markSchemaUsed(schemaName)
	route.RequestBody = jsonRequestBody(schemaName)
	if existingDesc != "" {
		route.RequestBody.Description = existingDesc
	} else if route.RequestBody.Description == "" {
		route.RequestBody.Description = requestBodyDescription(route.Method, route.Tags)
	}
}

func (p *Plugin) mergeResponsesWithAnnotations(route *openapi.Route, doc *parser.DocAnnotations) {
	if route == nil || doc == nil {
		return
	}
	merged := fibergen.PreferAnnotationMap(inferredResponseSchemaNames(route), annotatedResponseSchemaNames(doc))
	if route.Responses == nil {
		route.Responses = make(map[string]openapi.Response)
	}
	for _, resp := range doc.Responses {
		if resp.Code == "" {
			continue
		}
		if _, ok := merged[resp.Code]; !ok {
			if name := docResponseSchemaName(resp.Type); name != "" {
				merged[resp.Code] = name
			} else if strings.EqualFold(resp.Type, "array") && resp.ItemsType != "" {
				merged[resp.Code] = "array:" + cleanTypeName(resp.ItemsType)
			}
		}
		if existing, ok := route.Responses[resp.Code]; ok && resp.Description != "" {
			existing.Description = resp.Description
			route.Responses[resp.Code] = existing
		}
	}
	if len(merged) == 0 {
		return
	}
	for code, schemaName := range merged {
		resp := route.Responses[code]
		if resp.Description == "" {
			for _, annotated := range doc.Responses {
				if annotated.Code == code && annotated.Description != "" {
					resp.Description = annotated.Description
					break
				}
			}
		}
		if resp.Description == "" {
			resp.Description = responseDescription(code)
		}
		schema := annotatedResponseSchema(doc, code, schemaName)
		if schema != nil {
			p.markSchemaUsed(schemaName)
			p.markSchemaRefs(schema)
			resp.Content = map[string]openapi.MediaType{
				"application/json": {Schema: schema},
			}
		} else if schemaName != "" {
			p.markSchemaUsed(schemaName)
			resp.Content = map[string]openapi.MediaType{
				"application/json": {Schema: schemaRef(schemaName)},
			}
		}
		route.Responses[code] = resp
	}
}

func (p *Plugin) mergeParametersWithAnnotations(route *openapi.Route, doc *parser.DocAnnotations) {
	if route == nil || doc == nil || len(doc.Parameters) == 0 {
		return
	}
	existing := make(map[string]bool, len(route.Parameters))
	for _, param := range route.Parameters {
		existing[param.Name+"."+param.In] = true
	}
	for _, param := range doc.Parameters {
		in := strings.ToLower(strings.TrimSpace(param.In))
		if isBodyParamIn(in) {
			continue
		}
		if existing[param.Name+"."+in] {
			continue
		}
		schema := paramSchemaFromAnnotation(param.Type)
		if schema == nil {
			continue
		}
		routeParam := openapi.Parameter{
			Name:        param.Name,
			In:          in,
			Required:    param.Required,
			Description: param.Description,
			Schema:      schema,
		}
		enrichParameter(&routeParam, route.Tags)
		route.Parameters = append(route.Parameters, routeParam)
	}
}

func paramSchemaFromAnnotation(typeName string) *openapi.Schema {
	switch strings.ToLower(strings.TrimSpace(typeName)) {
	case "int", "integer", "int64", "int32":
		return &openapi.Schema{Type: "integer"}
	case "number", "float", "float64":
		return &openapi.Schema{Type: "number"}
	case "bool", "boolean":
		return &openapi.Schema{Type: "boolean"}
	default:
		if name := cleanTypeName(typeName); name != "" {
			return schemaRef(name)
		}
		return &openapi.Schema{Type: "string"}
	}
}

func annotatedResponseSchema(doc *parser.DocAnnotations, code, fallbackName string) *openapi.Schema {
	if doc == nil {
		return nil
	}
	for _, resp := range doc.Responses {
		if resp.Code != code {
			continue
		}
		if schema := schemaFromResponseAnnotation(resp); schema != nil {
			return schema
		}
		break
	}
	if strings.HasPrefix(fallbackName, "array:") {
		itemName := strings.TrimPrefix(fallbackName, "array:")
		if itemName != "" {
			return &openapi.Schema{Type: "array", Items: schemaRef(itemName)}
		}
	}
	if fallbackName == "" {
		return nil
	}
	return schemaRef(cleanTypeName(fallbackName))
}

func schemaFromResponseAnnotation(resp parser.ResponseAnnotation) *openapi.Schema {
	switch strings.ToLower(strings.TrimSpace(resp.Type)) {
	case "array":
		itemName := cleanTypeName(resp.ItemsType)
		if itemName == "" {
			return &openapi.Schema{Type: "array"}
		}
		return &openapi.Schema{Type: "array", Items: schemaRef(itemName)}
	case "object", "string", "integer", "number", "boolean":
		return nil
	default:
		if name := docResponseSchemaName(resp.Type); name != "" {
			return schemaRef(name)
		}
	}
	return nil
}

func inferredResponseSchemaNames(route *openapi.Route) map[string]string {
	if route == nil || len(route.Responses) == 0 {
		return nil
	}
	out := make(map[string]string, len(route.Responses))
	for code, resp := range route.Responses {
		if resp.Content == nil {
			continue
		}
		media, ok := resp.Content["application/json"]
		if !ok || media.Schema == nil || media.Schema.Ref == "" {
			continue
		}
		ref := strings.TrimPrefix(media.Schema.Ref, "#/components/schemas/")
		if ref != "" {
			out[code] = ref
		}
	}
	return out
}

func annotatedResponseSchemaNames(doc *parser.DocAnnotations) map[string]string {
	if doc == nil {
		return nil
	}
	out := make(map[string]string)
	for _, resp := range doc.Responses {
		if name := docResponseSchemaName(resp.Type); name != "" {
			out[resp.Code] = name
			continue
		}
		if strings.EqualFold(resp.Type, "array") && resp.ItemsType != "" {
			out[resp.Code] = "array:" + cleanTypeName(resp.ItemsType)
		}
	}
	return out
}

func annotatedRequestBodyName(doc *parser.DocAnnotations) string {
	if doc == nil {
		return ""
	}
	for _, param := range doc.Parameters {
		if !isBodyParamIn(param.In) {
			continue
		}
		return docResponseSchemaName(param.Type)
	}
	return ""
}

func inferredRequestBodyName(route *openapi.Route) string {
	if route == nil || route.RequestBody == nil || route.RequestBody.Content == nil {
		return ""
	}
	media, ok := route.RequestBody.Content["application/json"]
	if !ok || media.Schema == nil || media.Schema.Ref == "" {
		return ""
	}
	return strings.TrimPrefix(media.Schema.Ref, "#/components/schemas/")
}

func isBodyParamIn(in string) bool {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case "body", "formdata", "form":
		return true
	default:
		return false
	}
}

// FibergenCatalog exposes fibergen output for CLI enrichment.
func (p *Plugin) FibergenCatalog() *fibergen.Catalog {
	return p.fibergenCatalog
}
