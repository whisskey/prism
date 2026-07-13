// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package chi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/scanner"
)

func TestDiscoverRouteBundles_MiddlewareAndHandlerRef(t *testing.T) {
	source := `package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func JWTAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
}

func ListUsers(w http.ResponseWriter, r *http.Request) {}
func GetMe(w http.ResponseWriter, r *http.Request) {}

func Register(r chi.Router) {
	r.Use(JWTAuth)

	r.Route("/users", func(r chi.Router) {
		r.Get("/me", GetMe)
		r.With(RequireRole("admin")).Get("/", ListUsers)
	})
}
`

	p := New()
	routes, err := p.ExtractRoutes([]scanner.SourceFile{{
		Path:     "routes.go",
		Language: "go",
		Content:  []byte(source),
	}})
	require.NoError(t, err)

	byPath := map[string]int{}
	for i, route := range routes {
		byPath[route.Method+" "+route.Path] = i
	}

	meIdx, ok := byPath["GET /users/me"]
	require.True(t, ok, "expected GET /users/me")
	me := routes[meIdx]
	assert.Equal(t, "GetMe", me.Handler)
	assert.Equal(t, "GetMe", me.HandlerRef.Qualified)
	assert.True(t, me.HandlerRef.Resolved)
	require.NotEmpty(t, me.Middleware)
	assert.Equal(t, "JWTAuth", me.Middleware[0].Name)

	listIdx, ok := byPath["GET /users"]
	require.True(t, ok, "expected GET /users")
	list := routes[listIdx]
	assert.Equal(t, "ListUsers", list.Handler)
	assert.Equal(t, "ListUsers", list.HandlerRef.Qualified)
	assert.True(t, list.HandlerRef.Resolved)
	require.GreaterOrEqual(t, len(list.Middleware), 2)
	names := make([]string, 0, len(list.Middleware))
	for _, mw := range list.Middleware {
		names = append(names, mw.Name)
	}
	assert.Contains(t, names, "JWTAuth")
	assert.Contains(t, names, "RequireRole")
}
