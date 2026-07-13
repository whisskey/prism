// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

// Package spec builds, merges, diffs, and writes OpenAPI documents.
package spec

import (
	"fmt"
	"sort"
	"strings"

	"github.com/whisskey/prism/internal/config"
	oas "github.com/whisskey/prism/pkg/openapi"
)

// Builder constructs OpenAPI specifications from routes and schemas.
type Builder struct {
	config *config.Config
}

// NewBuilder creates a new OpenAPI builder with the given configuration.
func NewBuilder(cfg *config.Config) *Builder {
	return &Builder{
		config: cfg,
	}
}

// Build creates an OpenAPI document from routes and schemas.
func (b *Builder) Build(routes []oas.Route, schemas []oas.Schema) (*oas.OpenAPI, error) {
	preparedRoutes := prepareRoutes(routes, b.config.Generation.PreserveRouteOrder)
	preparedSchemas := prepareComponentSchemas(schemas)
	return b.buildDocument(preparedRoutes, preparedSchemas)
}

func (b *Builder) buildDocument(routes []oas.Route, schemas []oas.Schema) (*oas.OpenAPI, error) {
	schemas = sortedSchemaList(schemas)

	doc := &oas.OpenAPI{
		OpenAPI: b.config.OpenAPI.Version,
		Info:    b.buildInfo(),
		Servers: b.buildServers(),
		Paths:   make(map[string]oas.PathItem),
		Tags:    b.buildTags(),
	}

	// Build paths from routes
	if err := b.buildPaths(doc, routes); err != nil {
		return nil, fmt.Errorf("failed to build paths: %w", err)
	}

	// Build components from schemas
	if len(schemas) > 0 {
		doc.Components = b.buildComponents(schemas)
	}

	// Add security if configured
	if len(b.config.OpenAPI.Security.Schemes) > 0 {
		doc.Security = b.buildSecurity()
		if doc.Components == nil {
			doc.Components = &oas.Components{}
		}
		doc.Components.SecuritySchemes = b.buildSecuritySchemes()
	}

	b.ensureRouteSecuritySchemes(doc, routes)
	ensureComponentSchemas(doc, schemas)

	return doc, nil
}

func sortedRoutes(routes []oas.Route) []oas.Route {
	sorted := append([]oas.Route(nil), routes...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Path != sorted[j].Path {
			return sorted[i].Path < sorted[j].Path
		}
		if sorted[i].Method != sorted[j].Method {
			return sorted[i].Method < sorted[j].Method
		}
		return sorted[i].OperationID < sorted[j].OperationID
	})
	return sorted
}

func sortedSchemaList(schemas []oas.Schema) []oas.Schema {
	sorted := append([]oas.Schema(nil), schemas...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Title < sorted[j].Title
	})
	return sorted
}

// buildInfo constructs the Info object from configuration.
func (b *Builder) buildInfo() oas.Info {
	info := oas.Info{
		Title:          b.config.OpenAPI.Info.Title,
		Description:    b.config.OpenAPI.Info.Description,
		TermsOfService: b.config.OpenAPI.Info.TermsOfService,
		Version:        b.config.OpenAPI.Info.Version,
	}

	if b.config.OpenAPI.Info.Contact.Name != "" ||
		b.config.OpenAPI.Info.Contact.Email != "" ||
		b.config.OpenAPI.Info.Contact.URL != "" {
		info.Contact = &oas.Contact{
			Name:  b.config.OpenAPI.Info.Contact.Name,
			URL:   b.config.OpenAPI.Info.Contact.URL,
			Email: b.config.OpenAPI.Info.Contact.Email,
		}
	}

	if b.config.OpenAPI.Info.License.Name != "" {
		info.License = &oas.License{
			Name: b.config.OpenAPI.Info.License.Name,
			URL:  b.config.OpenAPI.Info.License.URL,
		}
	}

	return info
}

// buildServers constructs the servers list from configuration.
func (b *Builder) buildServers() []oas.Server {
	servers := make([]oas.Server, 0, len(b.config.OpenAPI.Servers))
	for _, s := range b.config.OpenAPI.Servers {
		servers = append(servers, oas.Server{
			URL:         s.URL,
			Description: s.Description,
		})
	}
	return servers
}

// buildTags constructs the tags list from configuration.
func (b *Builder) buildTags() []oas.Tag {
	tags := make([]oas.Tag, 0, len(b.config.OpenAPI.Tags))
	for _, t := range b.config.OpenAPI.Tags {
		tags = append(tags, oas.Tag{
			Name:        t.Name,
			Description: t.Description,
		})
	}
	return tags
}

// buildPaths constructs paths from routes.
func (b *Builder) buildPaths(doc *oas.OpenAPI, routes []oas.Route) error {
	for _, route := range routes {
		pathItem, exists := doc.Paths[route.Path]
		if !exists {
			pathItem = oas.PathItem{}
		}

		operation := b.routeToOperation(route)

		switch strings.ToUpper(route.Method) {
		case "GET":
			pathItem.Get = operation
		case "POST":
			pathItem.Post = operation
		case "PUT":
			pathItem.Put = operation
		case "DELETE":
			pathItem.Delete = operation
		case "PATCH":
			pathItem.Patch = operation
		case "OPTIONS":
			pathItem.Options = operation
		case "HEAD":
			pathItem.Head = operation
		case "TRACE":
			pathItem.Trace = operation
		default:
			return fmt.Errorf("unsupported HTTP method: %s", route.Method)
		}

		doc.Paths[route.Path] = pathItem
	}

	return nil
}

// routeToOperation converts a Route to an OpenAPI Operation.
func (b *Builder) routeToOperation(route oas.Route) *oas.Operation {
	op := &oas.Operation{
		Tags:        route.Tags,
		Summary:     route.Summary,
		Description: route.Description,
		OperationID: route.OperationID,
		Deprecated:  route.Deprecated,
	}

	// Copy parameters
	if len(route.Parameters) > 0 {
		op.Parameters = make([]oas.Parameter, len(route.Parameters))
		copy(op.Parameters, route.Parameters)
	}

	// Copy request body
	if route.RequestBody != nil {
		op.RequestBody = route.RequestBody
	}

	// Copy responses
	if len(route.Responses) > 0 {
		op.Responses = make(map[string]oas.Response)
		for code, resp := range route.Responses {
			op.Responses[code] = resp
		}
	} else {
		// Add default responses based on config
		op.Responses = b.buildDefaultResponses()
	}

	// Copy security
	if len(route.Security) > 0 {
		op.Security = route.Security
	}

	return op
}

// buildDefaultResponses creates default responses based on configuration.
func (b *Builder) buildDefaultResponses() map[string]oas.Response {
	responses := make(map[string]oas.Response)

	for _, code := range b.config.Generation.DefaultResponses {
		switch code {
		case "200":
			responses["200"] = oas.Response{
				Description: "Successful response",
			}
		case "201":
			responses["201"] = oas.Response{
				Description: "Created",
			}
		case "204":
			responses["204"] = oas.Response{
				Description: "No content",
			}
		case "400":
			responses["400"] = oas.Response{
				Description: "Bad request",
			}
		case "401":
			responses["401"] = oas.Response{
				Description: "Unauthorized",
			}
		case "403":
			responses["403"] = oas.Response{
				Description: "Forbidden",
			}
		case "404":
			responses["404"] = oas.Response{
				Description: "Not found",
			}
		case "500":
			responses["500"] = oas.Response{
				Description: "Internal server error",
			}
		default:
			responses[code] = oas.Response{
				Description: fmt.Sprintf("Response %s", code),
			}
		}
	}

	// Ensure at least one response exists
	if len(responses) == 0 {
		responses["200"] = oas.Response{
			Description: "Successful response",
		}
	}

	return responses
}

// buildComponents constructs the Components object from schemas.
func (b *Builder) buildComponents(schemas []oas.Schema) *oas.Components {
	components := &oas.Components{
		Schemas: make(map[string]*oas.Schema),
	}

	for i := range schemas {
		schema := schemas[i]
		name := schema.Title
		if name == "" {
			// Generate a name if not provided
			name = fmt.Sprintf("Schema%d", i+1)
		}
		components.Schemas[name] = &schema
	}

	return components
}

// buildSecurity constructs the global security requirements.
func (b *Builder) buildSecurity() []map[string][]string {
	if len(b.config.OpenAPI.Security.Default) == 0 {
		return nil
	}

	security := make([]map[string][]string, 0, len(b.config.OpenAPI.Security.Default))
	for _, name := range b.config.OpenAPI.Security.Default {
		security = append(security, map[string][]string{
			name: {},
		})
	}

	return security
}

// buildSecuritySchemes constructs security scheme definitions.
func (b *Builder) buildSecuritySchemes() map[string]oas.SecurityScheme {
	schemes := make(map[string]oas.SecurityScheme)

	for name, cfg := range b.config.OpenAPI.Security.Schemes {
		scheme := oas.SecurityScheme{
			Type:         cfg.Type,
			Description:  cfg.Description,
			Name:         cfg.Name,
			In:           cfg.In,
			Scheme:       cfg.Scheme,
			BearerFormat: cfg.BearerFormat,
		}
		schemes[name] = scheme
	}

	return schemes
}

func (b *Builder) ensureRouteSecuritySchemes(doc *oas.OpenAPI, routes []oas.Route) {
	required := make(map[string]bool)
	for _, route := range routes {
		for _, requirement := range route.Security {
			for name := range requirement {
				required[name] = true
			}
		}
	}
	if len(required) == 0 {
		return
	}
	if doc.Components == nil {
		doc.Components = &oas.Components{}
	}
	if doc.Components.SecuritySchemes == nil {
		doc.Components.SecuritySchemes = make(map[string]oas.SecurityScheme)
	}
	for name := range required {
		if _, exists := doc.Components.SecuritySchemes[name]; exists {
			continue
		}
		if name == "BearerAuth" {
			doc.Components.SecuritySchemes[name] = oas.SecurityScheme{
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "JWT",
				Description:  "Bearer token authentication",
			}
			continue
		}
		doc.Components.SecuritySchemes[name] = oas.SecurityScheme{
			Type:        "apiKey",
			Name:        name,
			In:          "header",
			Description: "Inferred security scheme",
		}
	}
}

// SchemaRef creates a reference to a schema in components.
func SchemaRef(schemaName string) *oas.Schema {
	return &oas.Schema{
		Ref: "#/components/schemas/" + schemaName,
	}
}

// SortedPaths returns a sorted list of path keys for deterministic output.
func SortedPaths(paths map[string]oas.PathItem) []string {
	keys := make([]string, 0, len(paths))
	for k := range paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// SortedSchemas returns a sorted list of schema keys for deterministic output.
func SortedSchemas(schemas map[string]*oas.Schema) []string {
	keys := make([]string, 0, len(schemas))
	for k := range schemas {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
