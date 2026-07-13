// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package chi

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

	"github.com/go-chi/chi/v5"
)

func SetupRoutes(r chi.Router) {
	r.Post("/items", ValidateJSON[CreateItemRequest](), CreateItem)
}

func CreateItem(w http.ResponseWriter, r *http.Request) {}

type CreateItemRequest struct {
	Name string ` + "`json:\"name\" validate:\"required,min=2\"`" + `
}

func ValidateJSON[T any]() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
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

func TestConventions_InlineDecodeMiddleware(t *testing.T) {
	source := `package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func SetupRoutes(r chi.Router) {
	r.Post("/items", ParseBody[CreateItemRequest](), CreateItem)
}

func CreateItem(w http.ResponseWriter, r *http.Request) {}

type CreateItemRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

func ParseBody[T any]() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body T
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			next.ServeHTTP(w, r)
		})
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

func TestConventions_ContextPayloadKey(t *testing.T) {
	source := `package main

import (
	"net/http"
)

func Create(w http.ResponseWriter, r *http.Request) {
	req := r.Context().Value("request").(*CreateItemRequest)
	Success(w, req)
}

type CreateItemRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

type Response struct {
	Success bool ` + "`json:\"success\"`" + `
	Data    any  ` + "`json:\"data,omitempty\"`" + `
}

func Success(w http.ResponseWriter, data any) {}
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

func TestConventions_HandlerBodyDecode(t *testing.T) {
	source := `package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func CreateItem(w http.ResponseWriter, r *http.Request) {
	var req CreateItemRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
}

type CreateItemRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

func SetupRoutes(r chi.Router) {
	r.Post("/items", CreateItem)
}
`

	p := New()
	routes, err := p.ExtractRoutes([]scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}})
	require.NoError(t, err)
	require.Len(t, routes, 1)
	require.NotNil(t, routes[0].RequestBody)
	assert.Equal(t, "#/components/schemas/CreateItemRequest", routes[0].RequestBody.Content["application/json"].Schema.Ref)
}
