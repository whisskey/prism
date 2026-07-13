// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package gin

import (
	"strings"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/pkg/openapi"
)

// applyAnnotationEscapeHatch applies godoc/@openapi annotations after static inference.
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
	for _, candidate := range []string{route.HandlerRef.Method, route.Handler, route.HandlerRef.Qualified} {
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

func (p *Plugin) applyDocAnnotationOverrides(route *openapi.Route, doc *parser.DocAnnotations) {
	if route == nil || doc == nil {
		return
	}
	if doc.Summary != "" {
		route.Summary = doc.Summary
	}
	if doc.Description != "" {
		route.Description = doc.Description
	}
	if doc.Deprecated {
		route.Deprecated = true
	}
	if len(doc.Tags) > 0 {
		route.Tags = append([]string(nil), doc.Tags...)
	}
}

func (p *Plugin) mergeRequestBodyWithAnnotations(route *openapi.Route, doc *parser.DocAnnotations) {
	if route == nil || doc == nil || !methodAllowsRequestBody(route.Method) {
		return
	}
	schemaName := annotatedRequestBodyName(doc)
	if schemaName == "" {
		return
	}
	p.markSchemaUsed(schemaName)
	route.RequestBody = jsonRequestBody(schemaName)
}

func (p *Plugin) mergeResponsesWithAnnotations(route *openapi.Route, doc *parser.DocAnnotations) {
	if route == nil || doc == nil || len(doc.Responses) == 0 {
		return
	}
	if route.Responses == nil {
		route.Responses = make(map[string]openapi.Response)
	}
	for _, resp := range doc.Responses {
		if resp.Code == "" {
			continue
		}
		out := route.Responses[resp.Code]
		if resp.Description != "" {
			out.Description = resp.Description
		}
		if out.Description == "" {
			out.Description = responseDescription(resp.Code)
		}
		schema := schemaFromResponseAnnotation(resp)
		if schema == nil {
			if name := docResponseSchemaName(resp.Type); name != "" {
				schema = schemaRef(name)
			}
		}
		if schema != nil {
			p.markSchemaRefs(schema)
			out.Content = map[string]openapi.MediaType{
				"application/json": {Schema: schema},
			}
		}
		route.Responses[resp.Code] = out
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
		route.Parameters = append(route.Parameters, openapi.Parameter{
			Name:        param.Name,
			In:          in,
			Required:    param.Required,
			Description: param.Description,
			Schema:      schema,
		})
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

func schemaFromResponseAnnotation(resp parser.ResponseAnnotation) *openapi.Schema {
	switch strings.ToLower(strings.TrimSpace(resp.Type)) {
	case "array":
		itemName := cleanTypeName(resp.ItemsType)
		if itemName == "" {
			return &openapi.Schema{Type: "array"}
		}
		return &openapi.Schema{Type: "array", Items: schemaRef(itemName)}
	case "object", "string", "integer", "number", "boolean":
		if name := cleanTypeName(resp.Type); name != "" && name != "object" {
			return schemaRef(name)
		}
		// {object} User style is stored with Type=User via parser; handled below.
		return nil
	default:
		if name := docResponseSchemaName(resp.Type); name != "" {
			return schemaRef(name)
		}
	}
	return nil
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

func docResponseSchemaName(typeName string) string {
	return cleanTypeName(typeName)
}

func isBodyParamIn(in string) bool {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case "body", "formdata", "form":
		return true
	default:
		return false
	}
}

func methodAllowsRequestBody(method string) bool {
	switch strings.ToUpper(method) {
	case "POST", "PUT", "PATCH":
		return true
	default:
		return false
	}
}

func responseDescription(code string) string {
	switch code {
	case "200":
		return "OK"
	case "201":
		return "Created"
	case "400":
		return "Bad request"
	case "401":
		return "Unauthorized"
	case "403":
		return "Forbidden"
	case "404":
		return "Not found"
	case "422":
		return "Unprocessable entity"
	case "500":
		return "Internal server error"
	default:
		return "Response"
	}
}

func jsonRequestBody(schemaName string) *openapi.RequestBody {
	return &openapi.RequestBody{
		Required: true,
		Content: map[string]openapi.MediaType{
			"application/json": {Schema: schemaRef(schemaName)},
		},
	}
}

func schemaRef(schemaName string) *openapi.Schema {
	if isUntypedSchemaName(schemaName) {
		return &openapi.Schema{}
	}
	return &openapi.Schema{Ref: "#/components/schemas/" + schemaName}
}

func isUntypedSchemaName(name string) bool {
	switch name {
	case "", "any", "interface{}":
		return true
	default:
		return false
	}
}

func cleanTypeName(typeName string) string {
	for {
		original := typeName
		typeName = strings.TrimPrefix(typeName, "[]")
		typeName = strings.TrimPrefix(typeName, "*")
		if typeName == original {
			break
		}
	}
	if idx := strings.LastIndex(typeName, "."); idx >= 0 {
		typeName = typeName[idx+1:]
	}
	if strings.Contains(typeName, "[") || strings.Contains(typeName, "]") {
		return ""
	}
	switch typeName {
	case "", "any", "string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64",
		"bool", "byte", "rune", "error", "interface{}":
		return ""
	}
	return typeName
}

func (p *Plugin) markSchemaUsed(name string) {
	name = cleanTypeName(name)
	if name == "" {
		return
	}
	if p.usedSchemas == nil {
		p.usedSchemas = make(map[string]bool)
	}
	p.usedSchemas[name] = true
}

func (p *Plugin) markSchemaRefs(schema *openapi.Schema) {
	if schema == nil {
		return
	}
	if schema.Ref != "" {
		p.markSchemaUsed(strings.TrimPrefix(schema.Ref, "#/components/schemas/"))
	}
	for _, prop := range schema.Properties {
		p.markSchemaRefs(prop)
	}
	if schema.Items != nil {
		p.markSchemaRefs(schema.Items)
	}
}
