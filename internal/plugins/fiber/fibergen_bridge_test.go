// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"testing"

	"github.com/whisskey/prism/internal/analyzer/fibergen"
	"github.com/whisskey/prism/pkg/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyFibergenHandlerDetail(t *testing.T) {
	p := New()
	p.usedSchemas = make(map[string]bool)
	p.fibergenCatalog = &fibergen.Catalog{
		Merged: &fibergen.MergedData{
			Handlers: map[string]fibergen.HandlerDetail{
				"GetUser": {
					Name:         "GetUser",
					RequestBody:  "CreateUserRequest",
					ResponseType: "User",
					PathParams: []fibergen.Param{
						{Name: "id", GoType: "string", Required: true},
					},
					QueryParams: []fibergen.Param{
						{Name: "page", GoType: "integer"},
					},
				},
			},
		},
	}

	route := &openapi.Route{
		Method:  "POST",
		Path:    "/users/{id}",
		Handler: "GetUser",
		HandlerRef: openapi.HandlerRef{
			Method: "GetUser",
		},
	}

	p.applyFibergenEnrichment(route)

	require.NotNil(t, route.RequestBody)
	assert.Contains(t, p.usedSchemas, "CreateUserRequest")
	assert.Contains(t, route.Responses, "200")
	require.Len(t, route.Parameters, 2)
}

func TestSupplementFibergenRoutes(t *testing.T) {
	p := New()
	p.fibergenCatalog = &fibergen.Catalog{
		Merged: &fibergen.MergedData{
			Routes: []fibergen.Route{
				{Method: "GET", Path: "/health", Handler: "Health"},
			},
		},
	}

	bundles := p.supplementFibergenRoutes(nil)
	require.Len(t, bundles, 1)
	assert.Equal(t, "GET", bundles[0].Route.Method)
	assert.Equal(t, "/health", bundles[0].Route.Path)
}
