// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package gin

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

	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine) {
	r.POST("/items", ValidateJSON[CreateItemRequest](), CreateItem)
}

func CreateItem(c *gin.Context) {}

type CreateItemRequest struct {
	Name string ` + "`json:\"name\" validate:\"required,min=2\"`" + `
}

func ValidateJSON[T any]() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body T
		if err := json.NewDecoder(c.Request.Body).Decode(&body); err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		c.Next()
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

import "github.com/gin-gonic/gin"

func CreateItem(c *gin.Context) {
	var req CreateItemRequest
	_ = c.ShouldBindJSON(&req)
}

type CreateItemRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

func SetupRoutes(r *gin.Engine) {
	r.POST("/items", CreateItem)
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

import "github.com/gin-gonic/gin"

func Create(c *gin.Context) {
	req := c.MustGet("request").(*CreateItemRequest)
	Success(c, req)
}

type CreateItemRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

func Success(c *gin.Context, data any) {}
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
