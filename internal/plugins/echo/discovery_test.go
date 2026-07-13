// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package echo

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

	"github.com/labstack/echo/v4"
)

func JWTAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		return next(c)
	}
}

func RequireRole(role string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return next(c)
		}
	}
}

func ListUsers(c echo.Context) error { return nil }
func GetMe(c echo.Context) error { return nil }

func Register(e *echo.Echo) {
	e.Use(JWTAuth)
	g := e.Group("/users")
	g.GET("/me", GetMe)
	g.GET("/", ListUsers, RequireRole("admin"))
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
	assert.True(t, me.HandlerRef.Resolved)
	require.NotEmpty(t, me.Middleware)
	assert.Equal(t, "JWTAuth", me.Middleware[0].Name)

	listIdx, ok := byPath["GET /users"]
	require.True(t, ok, "expected GET /users")
	list := routes[listIdx]
	assert.Equal(t, "ListUsers", list.Handler)
	require.GreaterOrEqual(t, len(list.Middleware), 2)
	names := make([]string, 0, len(list.Middleware))
	for _, mw := range list.Middleware {
		names = append(names, mw.Name)
	}
	assert.Contains(t, names, "JWTAuth")
	assert.Contains(t, names, "RequireRole")
}

func TestDiscoverRouteBundles_GroupPrefix(t *testing.T) {
	source := `package routes

import "github.com/labstack/echo/v4"

func ListUsers(c echo.Context) error { return nil }

func Register(e *echo.Echo) {
	api := e.Group("/api/v1")
	api.GET("/users", ListUsers)
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
	assert.Equal(t, "/api/v1/users", routes[0].Path)
}
