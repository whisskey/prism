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

	"github.com/labstack/echo/v4"
)

func JWTAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if c.Request().Header.Get("Authorization") == "" {
			return c.NoContent(http.StatusUnauthorized)
		}
		return next(c)
	}
}

func GetMe(c echo.Context) error { return nil }

func main() {
	e := echo.New()
	e.Use(JWTAuth)
	e.GET("/me", GetMe)
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
