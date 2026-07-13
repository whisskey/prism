// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package chi

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/scanner"
	"github.com/whisskey/prism/pkg/openapi"
)

func TestApplyAuthFromMiddlewareNames_JWTAuth(t *testing.T) {
	p := New()
	route := &openapi.Route{Method: "GET", Path: "/api/v1/users/me"}
	p.applyAuthFromMiddlewareNames(route, []string{"middleware.JWTAuth", "relaxedLimiter"})
	requireSecurityScheme(t, route, "BearerAuth")
}

func TestApplyAuthSecurity_401Heuristic(t *testing.T) {
	source := `package main

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

func GetMe(w http.ResponseWriter, r *http.Request) {}

func main() {
	r := chi.NewRouter()
	r.Use(JWTAuth)
	r.Get("/me", GetMe)
}
`

	p := New()
	bundles := p.discoverRouteBundles([]scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}})
	require.Len(t, bundles, 1)
	p.enrichRouteBundles(bundles)
	require.Len(t, bundles[0].Route.Security, 1)
	assert.Contains(t, bundles[0].Route.Security[0], "BearerAuth")
}

func requireSecurityScheme(t *testing.T, route *openapi.Route, scheme string) {
	t.Helper()
	for _, req := range route.Security {
		if _, ok := req[scheme]; ok {
			return
		}
	}
	t.Fatalf("expected security scheme %q on route", scheme)
}

func TestMiddlewareNameFromExpr(t *testing.T) {
	name := middlewareNameFromExpr(astIdent("JWTAuth"))
	assert.Equal(t, "JWTAuth", name)
	name = middlewareNameFromExpr(astCallSelector("middleware", "JWTAuth"))
	assert.Equal(t, "middleware.JWTAuth", name)
}

func astIdent(name string) ast.Expr {
	return &ast.Ident{Name: name}
}

func astCallSelector(pkg, name string) ast.Expr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.Ident{Name: pkg},
			Sel: &ast.Ident{Name: name},
		},
	}
}
