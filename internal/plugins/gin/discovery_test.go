// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package gin

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

	"github.com/gin-gonic/gin"
)

func JWTAuth(c *gin.Context) {
	if c.GetHeader("Authorization") == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}

func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-Role") != role {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}

func ListUsers(c *gin.Context) {}
func GetMe(c *gin.Context) {}

func Register(r *gin.Engine) {
	r.Use(JWTAuth)
	g := r.Group("/users")
	g.GET("/me", GetMe)
	g.GET("/", RequireRole("admin"), ListUsers)
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

import "github.com/gin-gonic/gin"

func ListUsers(c *gin.Context) {}

func Register(r *gin.Engine) {
	api := r.Group("/api/v1")
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
