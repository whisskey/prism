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

	"github.com/gin-gonic/gin"
)

func JWTAuth(c *gin.Context) {
	if c.GetHeader("Authorization") == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}

func GetMe(c *gin.Context) {}

func main() {
	r := gin.New()
	r.Use(JWTAuth)
	r.GET("/me", GetMe)
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
