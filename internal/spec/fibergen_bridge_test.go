// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/analyzer/fibergen"
	"github.com/whisskey/prism/internal/config"
)

func TestAssembler_AssembleFromFibergenCatalog(t *testing.T) {
	cfg := config.Default()
	catalog := &fibergen.Catalog{
		Merged: &fibergen.MergedData{
			Title:   "Users API",
			Version: "2.0.0",
			Routes: []fibergen.Route{
				{
					Method:  "POST",
					Path:    "/users",
					Handler: "CreateUser",
				},
			},
			Handlers: map[string]fibergen.HandlerDetail{
				"CreateUser": {
					Name:         "CreateUser",
					RequestBody:  "CreateUserRequest",
					ResponseType: "User",
				},
			},
			Schemas: map[string]fibergen.Schema{
				"CreateUserRequest": {
					Type: "object",
					Properties: map[string]fibergen.Property{
						"email": {Type: "string"},
					},
					Required: []string{"email"},
				},
				"User": {
					Type: "object",
					Properties: map[string]fibergen.Property{
						"id": {Type: "integer"},
					},
				},
			},
			Security: map[string][]fibergen.SecurityRequirement{
				"POST /users": {{Scheme: "BearerAuth"}},
			},
			SecuritySchemes: map[string]fibergen.SecurityScheme{
				"BearerAuth": {Type: "http", Scheme: "bearer"},
			},
		},
	}

	assembler := NewAssembler(cfg)
	doc, err := assembler.AssembleFromFibergenCatalog(catalog)
	require.NoError(t, err)
	require.NotNil(t, doc)

	assert.Equal(t, "Users API", doc.Info.Title)
	assert.Equal(t, "2.0.0", doc.Info.Version)
	require.Contains(t, doc.Paths, "/users")
	op := doc.Paths["/users"].Post
	require.NotNil(t, op)
	require.NotNil(t, op.RequestBody)
	require.Contains(t, op.Responses, "200")
	require.Len(t, op.Security, 1)
	assert.Contains(t, doc.Components.SecuritySchemes, "BearerAuth")
	require.Contains(t, doc.Components.Schemas, "CreateUserRequest")
	require.Contains(t, doc.Components.Schemas, "User")
}

func TestRoutesFromFibergenCatalog_NilCatalog(t *testing.T) {
	assert.Nil(t, RoutesFromFibergenCatalog(nil))
}

func TestEnrichDocumentFromFibergen_PreservesConfiguredTitle(t *testing.T) {
	cfg := config.Default()
	cfg.OpenAPI.Info.Title = "Fiber Golang Boilerplate API"
	cfg.OpenAPI.Info.Version = "1.0.0"

	assembler := NewAssembler(cfg)
	doc, err := assembler.Assemble(nil, nil)
	require.NoError(t, err)

	catalog := &fibergen.Catalog{
		Merged: &fibergen.MergedData{
			Title:   "dto API",
			Version: "9.9.9",
		},
	}

	EnrichDocumentFromFibergen(doc, catalog, cfg)
	assert.Equal(t, "Fiber Golang Boilerplate API", doc.Info.Title)
	assert.Equal(t, "1.0.0", doc.Info.Version)
}
