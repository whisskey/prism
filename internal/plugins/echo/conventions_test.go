// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package echo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/scanner"
	"github.com/whisskey/prism/pkg/openapi"
)

func TestConventions_ValidateJSONMiddleware(t *testing.T) {
	source := `package main

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
)

func SetupRoutes(e *echo.Echo) {
	e.POST("/items", CreateItem, ValidateJSON[CreateItemRequest]())
}

func CreateItem(c echo.Context) error { return nil }

type CreateItemRequest struct {
	Name string ` + "`json:\"name\" validate:\"required,min=2\"`" + `
}

func ValidateJSON[T any]() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			var body T
			if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
				return c.NoContent(http.StatusBadRequest)
			}
			return next(c)
		}
	}
}
`

	p := New()
	routes, err := p.ExtractRoutes([]scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}})
	require.NoError(t, err)
	require.Len(t, routes, 1)
	require.NotNil(t, routes[0].RequestBody)
	assert.Equal(t, "#/components/schemas/CreateItemRequest", routes[0].RequestBody.Content["application/json"].Schema.Ref)
}

func TestConventions_HandlerBind(t *testing.T) {
	source := `package main

import "github.com/labstack/echo/v4"

func CreateItem(c echo.Context) error {
	var req CreateItemRequest
	_ = c.Bind(&req)
	return nil
}

type CreateItemRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

func SetupRoutes(e *echo.Echo) {
	e.POST("/items", CreateItem)
}
`

	p := New()
	routes, err := p.ExtractRoutes([]scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}})
	require.NoError(t, err)
	require.Len(t, routes, 1)
	require.NotNil(t, routes[0].RequestBody)
	assert.Equal(t, "#/components/schemas/CreateItemRequest", routes[0].RequestBody.Content["application/json"].Schema.Ref)
}

func TestConventions_ContextPayloadKey(t *testing.T) {
	source := `package main

import "github.com/labstack/echo/v4"

func Create(c echo.Context) error {
	req := c.Get("request").(*CreateItemRequest)
	return Success(c, req)
}

type CreateItemRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

func Success(c echo.Context, data any) error { return nil }
`

	p := New()
	p.discoverRouteBundles([]scanner.SourceFile{{Path: "handler.go", Language: "go", Content: []byte(source)}})
	fn := p.handlerIndex["Create"]
	require.NotNil(t, fn)

	analysis := p.collectHandlerAnalysis(fn)
	require.Len(t, analysis.RequestHints, 1)
	assert.Equal(t, "CreateItemRequest", analysis.RequestHints[0].SchemaName)

	graph := RouteTypeGraph{Responses: make(map[string]ResponseResolution)}
	p.resolveHandlerResponseTypes(&openapi.Route{Method: "POST"}, &analysis, &graph)
}
