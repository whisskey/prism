// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package chi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/scanner"
)

func TestExtractRoutes_OpenAPIAnnotationEscapeHatch(t *testing.T) {
	source := `package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type User struct {
	ID   string ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}

// ListUsers returns users.
// @openapi:success 200 {object} User
// @openapi:failure 401 {object} ErrorResponse
func ListUsers(w http.ResponseWriter, r *http.Request) {}

type ErrorResponse struct {
	Message string ` + "`json:\"message\"`" + `
}

func Register(r chi.Router) {
	r.Get("/users", ListUsers)
}
`

	p := New()
	routes, err := p.ExtractRoutes([]scanner.SourceFile{{
		Path:     "routes.go",
		Language: "go",
		Content:  []byte(source),
	}})
	require.NoError(t, err)
	require.Len(t, routes, 1)

	route := routes[0]
	require.NotNil(t, route.Responses)
	ok := route.Responses["200"]
	require.NotNil(t, ok.Content)
	assert.Equal(t, "#/components/schemas/User", ok.Content["application/json"].Schema.Ref)

	unauthorized := route.Responses["401"]
	require.NotNil(t, unauthorized.Content)
	assert.Equal(t, "#/components/schemas/ErrorResponse", unauthorized.Content["application/json"].Schema.Ref)
}
