// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package spec

import (
	"strings"

	"github.com/whisskey/prism/internal/analyzer/fibergen"
	"github.com/whisskey/prism/internal/config"
	oas "github.com/whisskey/prism/pkg/openapi"
)

// AssembleFromFibergenCatalog builds a full OpenAPI document from fibergen output.
func (a *Assembler) AssembleFromFibergenCatalog(catalog *fibergen.Catalog) (*oas.OpenAPI, error) {
	routes := RoutesFromFibergenCatalog(catalog)
	schemas := SchemasFromFibergenCatalog(catalog)

	doc, err := a.Assemble(routes, schemas)
	if err != nil {
		return nil, err
	}
	if catalog != nil && catalog.Merged != nil {
		applyFibergenSecuritySchemes(doc, catalog.Merged.SecuritySchemes)
		applyFibergenInfo(doc, catalog.Merged, a.config, true)
	}
	return doc, nil
}

// EnrichDocumentFromFibergen merges fibergen security schemes into an assembled document.
// Configured info.title and info.version from the assembler are preserved.
func EnrichDocumentFromFibergen(doc *oas.OpenAPI, catalog *fibergen.Catalog, cfg *config.Config) *oas.OpenAPI {
	if doc == nil || catalog == nil || catalog.Merged == nil {
		return doc
	}
	applyFibergenSecuritySchemes(doc, catalog.Merged.SecuritySchemes)
	_ = cfg
	return doc
}

func applyFibergenInfo(doc *oas.OpenAPI, merged *fibergen.MergedData, cfg *config.Config, allowMetadata bool) {
	if doc == nil || merged == nil || !allowMetadata {
		return
	}
	configuredTitle, configuredVersion := configuredInfoMetadata(cfg)
	if merged.Title != "" && shouldApplyFibergenMetadata(doc.Info.Title, configuredTitle, defaultInfoTitle) {
		doc.Info.Title = merged.Title
	}
	if merged.Version != "" && shouldApplyFibergenMetadata(doc.Info.Version, configuredVersion, defaultInfoVersion) {
		doc.Info.Version = merged.Version
	}
}

func configuredInfoMetadata(cfg *config.Config) (title, version string) {
	if cfg == nil {
		return "", ""
	}
	return cfg.OpenAPI.Info.Title, cfg.OpenAPI.Info.Version
}

func shouldApplyFibergenMetadata(current, configured, fallback string) bool {
	if configured != "" && current == configured && configured != fallback {
		return false
	}
	if current != "" && current != fallback {
		return false
	}
	return true
}

const (
	defaultInfoTitle   = "API"
	defaultInfoVersion = "1.0.0"
)

// RoutesFromFibergenCatalog converts fibergen routes into prism route models.
func RoutesFromFibergenCatalog(catalog *fibergen.Catalog) []oas.Route {
	if catalog == nil || catalog.Merged == nil {
		return nil
	}
	merged := catalog.Merged
	routes := make([]oas.Route, 0, len(merged.Routes))
	for _, fgRoute := range merged.Routes {
		route := oas.Route{
			Method:     strings.ToUpper(fgRoute.Method),
			Path:       fgRoute.Path,
			Handler:    fgRoute.Handler,
			SourceFile: fgRoute.SourceFile,
			SourceLine: fgRoute.SourceLine,
			HandlerRef: oas.HandlerRef{
				Qualified: fgRoute.Handler,
				Method:    fibergenHandlerMethod(fgRoute.Handler),
				Resolved:  fgRoute.Handler != "",
			},
			Middleware: fibergenMiddlewareRefs(fgRoute.Middlewares, fgRoute.SourceFile, fgRoute.SourceLine),
		}

		key := fibergenRouteKey(route.Method, route.Path)
		for _, req := range merged.Security[key] {
			if req.Scheme == "" {
				continue
			}
			route.Security = append(route.Security, map[string][]string{req.Scheme: req.Scopes})
		}

		if detail, ok := merged.LookupHandler(fgRoute.Handler); ok {
			enrichFibergenRoute(&route, detail)
		}
		routes = append(routes, route)
	}
	return routes
}

// SchemasFromFibergenCatalog converts fibergen schemas into prism schema models.
func SchemasFromFibergenCatalog(catalog *fibergen.Catalog) []oas.Schema {
	if catalog == nil || catalog.Merged == nil || len(catalog.Merged.Schemas) == 0 {
		return nil
	}
	schemas := make([]oas.Schema, 0, len(catalog.Merged.Schemas))
	for name, fgSchema := range catalog.Merged.Schemas {
		if converted := convertFibergenSchema(name, fgSchema); converted != nil {
			schemas = append(schemas, *converted)
		}
	}
	return schemas
}

func enrichFibergenRoute(route *oas.Route, detail fibergen.HandlerDetail) {
	if route == nil {
		return
	}
	if route.RequestBody == nil && detail.RequestBody != "" && fibergenMethodAllowsBody(route.Method) {
		schemaName := strings.TrimPrefix(detail.RequestBody, "[]")
		route.RequestBody = fibergenJSONRequestBody(schemaName)
	}
	if route.Responses == nil {
		route.Responses = make(map[string]oas.Response)
	}
	applyFibergenResponse(route, "200", detail.ResponseType)
	for code, schemaName := range detail.ResponseSchemas {
		applyFibergenResponse(route, code, schemaName)
	}
	route.Parameters = appendFibergenRouteParams(route.Parameters, detail.PathParams, "path")
	route.Parameters = appendFibergenRouteParams(route.Parameters, detail.QueryParams, "query")
}

func applyFibergenResponse(route *oas.Route, code, schemaName string) {
	if route == nil || code == "" || schemaName == "" {
		return
	}
	if _, exists := route.Responses[code]; exists {
		return
	}
	name := strings.TrimPrefix(schemaName, "[]")
	if name == "" {
		return
	}
	var schema *oas.Schema
	if strings.HasPrefix(schemaName, "[]") {
		schema = &oas.Schema{Type: "array", Items: fibergenSchemaRef(name)}
	} else {
		schema = fibergenSchemaRef(name)
	}
	route.Responses[code] = oas.Response{
		Description: fibergenResponseDescription(code),
		Content: map[string]oas.MediaType{
			"application/json": {Schema: schema},
		},
	}
}

func applyFibergenSecuritySchemes(doc *oas.OpenAPI, schemes map[string]fibergen.SecurityScheme) {
	if doc == nil || len(schemes) == 0 {
		return
	}
	if doc.Components == nil {
		doc.Components = &oas.Components{}
	}
	if doc.Components.SecuritySchemes == nil {
		doc.Components.SecuritySchemes = make(map[string]oas.SecurityScheme)
	}
	for name, fgScheme := range schemes {
		if _, exists := doc.Components.SecuritySchemes[name]; exists {
			continue
		}
		doc.Components.SecuritySchemes[name] = oas.SecurityScheme{
			Type:   fgScheme.Type,
			Scheme: fgScheme.Scheme,
			In:     fgScheme.In,
			Name:   fgScheme.Name,
		}
	}
}

func convertFibergenSchema(name string, fg fibergen.Schema) *oas.Schema {
	schema := &oas.Schema{
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
	schema.Properties = make(map[string]*oas.Schema, len(fg.Properties))
	for propName, prop := range fg.Properties {
		schema.Properties[propName] = convertFibergenProperty(prop)
	}
	if schema.Type == "" {
		schema.Type = "object"
	}
	return schema
}

func convertFibergenProperty(prop fibergen.Property) *oas.Schema {
	s := &oas.Schema{
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

func appendFibergenRouteParams(existing []oas.Parameter, params []fibergen.Param, in string) []oas.Parameter {
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
		existing = append(existing, oas.Parameter{
			Name:     param.Name,
			In:       location,
			Required: param.Required || location == "path",
			Schema:   fibergenOpenAPISchemaForGoType(param.GoType),
		})
		seen[key] = true
	}
	return existing
}

func fibergenMiddlewareRefs(names []string, sourceFile string, sourceLine int) []oas.MiddlewareRef {
	if len(names) == 0 {
		return nil
	}
	refs := make([]oas.MiddlewareRef, 0, len(names))
	for _, name := range names {
		refs = append(refs, oas.MiddlewareRef{
			Name:       name,
			SourceFile: sourceFile,
			SourceLine: sourceLine,
		})
	}
	return refs
}

func fibergenJSONRequestBody(schemaName string) *oas.RequestBody {
	return &oas.RequestBody{
		Required: true,
		Content: map[string]oas.MediaType{
			"application/json": {Schema: fibergenSchemaRef(schemaName)},
		},
	}
}

func fibergenSchemaRef(name string) *oas.Schema {
	if name == "" {
		return nil
	}
	return &oas.Schema{Ref: "#/components/schemas/" + name}
}

func fibergenOpenAPISchemaForGoType(goType string) *oas.Schema {
	switch goType {
	case "integer":
		return &oas.Schema{Type: "integer"}
	case "number":
		return &oas.Schema{Type: "number"}
	case "boolean":
		return &oas.Schema{Type: "boolean"}
	default:
		return &oas.Schema{Type: "string"}
	}
}

func fibergenMethodAllowsBody(method string) bool {
	switch strings.ToUpper(method) {
	case "POST", "PUT", "PATCH":
		return true
	default:
		return false
	}
}

func fibergenResponseDescription(code string) string {
	switch code {
	case "200":
		return "Successful response"
	case "201":
		return "Created"
	case "204":
		return "No content"
	case "400":
		return "Bad request"
	case "401":
		return "Unauthorized"
	case "403":
		return "Forbidden"
	case "404":
		return "Not found"
	case "500":
		return "Internal server error"
	default:
		return "Response"
	}
}

func fibergenRouteKey(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

func fibergenHandlerMethod(handler string) string {
	if idx := strings.LastIndex(handler, "."); idx >= 0 {
		return handler[idx+1:]
	}
	return handler
}
