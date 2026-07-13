// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package chi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/scanner"
)

func TestExtractSchemas_AnyBecomesEmptySchema(t *testing.T) {
	source := `package models

type ErrorInfo struct {
	Code    string ` + "`json:\"code\"`" + `
	Message string ` + "`json:\"message\"`" + `
	Details any    ` + "`json:\"details,omitempty\"`" + `
}

type Response struct {
	Success bool       ` + "`json:\"success\"`" + `
	Data    any        ` + "`json:\"data,omitempty\"`" + `
	Error   *ErrorInfo ` + "`json:\"error,omitempty\"`" + `
}
`

	p := New()
	schemas, err := p.ExtractSchemas([]scanner.SourceFile{{
		Path:     "models.go",
		Language: "go",
		Content:  []byte(source),
	}})
	require.NoError(t, err)

	byName := map[string]int{}
	for i, s := range schemas {
		byName[s.Title] = i
	}
	require.Contains(t, byName, "ErrorInfo")
	require.Contains(t, byName, "Response")

	errInfo := schemas[byName["ErrorInfo"]]
	require.NotNil(t, errInfo.Properties["details"])
	details := errInfo.Properties["details"]
	assert.Empty(t, details.Ref, "any must not become $ref to schemas/any")
	assert.True(t, details.Type == "" || details.Type == "object" || details.AdditionalProperties != nil || len(details.Properties) == 0)

	resp := schemas[byName["Response"]]
	require.NotNil(t, resp.Properties["data"])
	data := resp.Properties["data"]
	assert.Empty(t, data.Ref, "any must not become $ref to schemas/any")
}

func TestExtractRoutes_ErrorEnvelopePrefersResponseThenErrorResponse(t *testing.T) {
	withResponse := `package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Response struct {
	Success bool ` + "`json:\"success\"`" + `
	Data    any  ` + "`json:\"data,omitempty\"`" + `
}

func JWTAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
}

func List(w http.ResponseWriter, r *http.Request) {}

func Register(r chi.Router) {
	r.Use(JWTAuth)
	r.Get("/items", List)
}
`

	p := New()
	routes, err := p.ExtractRoutes([]scanner.SourceFile{{
		Path:     "routes.go",
		Language: "go",
		Content:  []byte(withResponse),
	}})
	require.NoError(t, err)
	require.Len(t, routes, 1)
	resp401 := routes[0].Responses["401"]
	require.NotNil(t, resp401.Content)
	schema := resp401.Content["application/json"].Schema
	require.NotNil(t, schema)
	assert.Equal(t, "Response", schema.Title)

	withErrorOnly := `package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type ErrorResponse struct {
	Error string ` + "`json:\"error\"`" + `
}

func JWTAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
}

func List(w http.ResponseWriter, r *http.Request) {}

func Register(r chi.Router) {
	r.Use(JWTAuth)
	r.Get("/items", List)
}
`

	p2 := New()
	routes2, err := p2.ExtractRoutes([]scanner.SourceFile{{
		Path:     "routes.go",
		Language: "go",
		Content:  []byte(withErrorOnly),
	}})
	require.NoError(t, err)
	require.Len(t, routes2, 1)
	resp401b := routes2[0].Responses["401"]
	require.NotNil(t, resp401b.Content)
	schema2 := resp401b.Content["application/json"].Schema
	require.NotNil(t, schema2)
	assert.Equal(t, "ErrorResponse", schema2.Title)
}
