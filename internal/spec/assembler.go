// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package spec

import (
	"fmt"

	"github.com/whisskey/prism/internal/config"
	oas "github.com/whisskey/prism/pkg/openapi"
)

// Assembler is Layer 4: it turns resolved routes and schemas into a final OpenAPI document.
type Assembler struct {
	builder *Builder
	config  *config.Config
}

// NewAssembler creates a Layer 4 OpenAPI assembler.
func NewAssembler(cfg *config.Config) *Assembler {
	return &Assembler{
		builder: NewBuilder(cfg),
		config:  cfg,
	}
}

// Assemble serializes type graphs into a unified OpenAPI document.
func (a *Assembler) Assemble(routes []oas.Route, schemas []oas.Schema) (*oas.OpenAPI, error) {
	preparedRoutes := prepareRoutes(routes, a.config.Generation.PreserveRouteOrder)
	preparedSchemas := prepareComponentSchemas(schemas)

	doc, err := a.builder.buildDocument(preparedRoutes, preparedSchemas)
	if err != nil {
		return nil, fmt.Errorf("assemble openapi document: %w", err)
	}

	ensureComponentSchemas(doc, preparedSchemas)
	return doc, nil
}

func prepareRoutes(routes []oas.Route, preserveOrder bool) []oas.Route {
	if preserveOrder || len(routes) == 0 {
		return append([]oas.Route(nil), routes...)
	}
	return sortedRoutes(routes)
}

func prepareComponentSchemas(schemas []oas.Schema) []oas.Schema {
	if len(schemas) == 0 {
		return nil
	}

	byName := make(map[string]oas.Schema, len(schemas))
	for _, schema := range schemas {
		name := schema.Title
		if name == "" {
			continue
		}
		byName[name] = schema
	}

	deduped := make([]oas.Schema, 0, len(byName))
	for _, schema := range byName {
		deduped = append(deduped, schema)
	}

	return sortedSchemaList(deduped)
}

func ensureComponentSchemas(doc *oas.OpenAPI, schemas []oas.Schema) {
	if doc == nil || len(schemas) == 0 {
		return
	}
	if doc.Components == nil {
		doc.Components = &oas.Components{}
	}
	if doc.Components.Schemas == nil {
		doc.Components.Schemas = make(map[string]*oas.Schema)
	}
	for i := range schemas {
		schema := schemas[i]
		name := schema.Title
		if name == "" {
			continue
		}
		if _, exists := doc.Components.Schemas[name]; !exists {
			schemaCopy := schema
			doc.Components.Schemas[name] = &schemaCopy
		}
	}
}
