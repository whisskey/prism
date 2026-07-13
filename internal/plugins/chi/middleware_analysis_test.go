// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package chi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/scanner"
)

func TestExtractRoutes_MiddlewareErrorStatusesFromBody(t *testing.T) {
	source := `package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func JWTAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Role") != role {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ListUsers(w http.ResponseWriter, r *http.Request) {}

func Register(r chi.Router) {
	r.Use(JWTAuth)
	r.With(RequireRole("admin")).Get("/users", ListUsers)
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
	_, has401 := route.Responses["401"]
	_, has403 := route.Responses["403"]
	assert.True(t, has401, "expected 401 from JWTAuth body")
	assert.True(t, has403, "expected 403 from RequireRole body")
}
