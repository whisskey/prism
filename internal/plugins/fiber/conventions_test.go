// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/scanner"
	"github.com/whisskey/prism/pkg/openapi"
)

func TestConventions_ValidateJSONMiddleware(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Post("/items", middleware.ValidateJSON[CreateItemRequest](), CreateItem)
}

func CreateItem(c *fiber.Ctx) error {
	req := c.Locals("request").(*CreateItemRequest)
	return c.JSON(req)
}

type CreateItemRequest struct {
	Name string ` + "`json:\"name\" validate:\"required,min=2\"`" + `
}

func ValidateJSON[T any]() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.Next()
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

func TestConventions_InlineBodyParserMiddleware(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Post("/items", ParseBody[CreateItemRequest](), CreateItem)
}

func CreateItem(c *fiber.Ctx) error {
	return c.JSON(CreateItemRequest{})
}

type CreateItemRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

func ParseBody[T any]() fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body T
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("bad")
		}
		return c.Next()
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

func TestConventions_LocalsRequestKey(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func Create(c *fiber.Ctx) error {
	req := c.Locals("request").(*CreateItemRequest)
	return Success(c, req)
}

type CreateItemRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

type ApiResponse[T any] struct {
	Status  string ` + "`json:\"status\"`" + `
	Message string ` + "`json:\"message\"`" + `
	Payload T      ` + "`json:\"payload,omitempty\"`" + `
}

func Success(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusOK).JSON(ApiResponse[any]{
		Status:  "ok",
		Message: "OK",
		Payload: data,
	})
}
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
	success, ok := graph.Responses["200"]
	require.True(t, ok)
	require.NotNil(t, success.Schema.Properties["payload"])
	assert.Equal(t, "#/components/schemas/CreateItemRequest", success.Schema.Properties["payload"].Ref)
}

func TestGenericValidatorSchema_Names(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "ValidateBody",
			source: `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) { app.Post("/", middleware.ValidateBody[Foo](), h) }
func h(c *fiber.Ctx) error { return nil }
func ValidateBody[T any]() fiber.Handler { return nil }
type Foo struct{}`,
			want: "Foo",
		},
		{
			name: "BodyValidator",
			source: `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) { app.Post("/", BodyValidator[CreateReq](), h) }
func h(c *fiber.Ctx) error { return nil }
func BodyValidator[T any]() fiber.Handler { return nil }
type CreateReq struct{}`,
			want: "CreateReq",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			routes, err := p.ExtractRoutes([]scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(tt.source)}})
			require.NoError(t, err)
			require.Len(t, routes, 1)
			require.NotNil(t, routes[0].RequestBody)
			assert.Equal(t, "#/components/schemas/"+tt.want, routes[0].RequestBody.Content["application/json"].Schema.Ref)
		})
	}
}

func TestValidatorSchemaFromFuncBody(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func ParseBody[T any]() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := c.BodyParser(new(CreateItemRequest)); err != nil {
			return err
		}
		return c.Next()
	}
}

type CreateItemRequest struct {
	Name string
}
`

	p := New()
	p.discoverRouteBundles([]scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}})
	fn := p.routeFuncIndex["ParseBody"]
	require.NotNil(t, fn)
	assert.Equal(t, "CreateItemRequest", validatorSchemaFromFuncBody(fn))
}
