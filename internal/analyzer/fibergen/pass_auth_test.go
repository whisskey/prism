// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSchemeForMiddleware(t *testing.T) {
	scheme, ok := SchemeForMiddleware("authMiddleware")
	assert.True(t, ok)
	assert.Equal(t, "BearerAuth", scheme)

	scheme, ok = SchemeForMiddleware("middleware.JWTAuth")
	assert.True(t, ok)
	assert.Equal(t, "BearerAuth", scheme)

	_, ok = SchemeForMiddleware("normalLimiter")
	assert.False(t, ok)
}

func TestAuthMapperPass_GroupJWTAuth(t *testing.T) {
	result := AuthMapperPass([]Route{{
		Method:      "GET",
		Path:        "/api/v1/users/me",
		Middlewares: []string{"middleware.JWTAuth", "relaxedLimiter"},
	}})
	reqs := result.Requirements["GET /api/v1/users/me"]
	assert.Len(t, reqs, 1)
	assert.Equal(t, "BearerAuth", reqs[0].Scheme)
	assert.Contains(t, result.Schemes, "BearerAuth")
}

func TestRouteCollector_GroupMiddleware(t *testing.T) {
	rc := newRouteCollector()
	rc.groupMiddleware["users"] = []string{"middleware.JWTAuth"}
	handlers, middlewares := rc.splitHandlersAndMiddlewares(nil, []ast.Expr{
		&ast.Ident{Name: "relaxedLimiter"},
		&ast.SelectorExpr{
			X:   &ast.Ident{Name: "deps"},
			Sel: &ast.Ident{Name: "GetMe"},
		},
	})
	inherited := rc.groupMiddleware["users"]
	middlewares = append(append([]string(nil), inherited...), middlewares...)
	assert.Equal(t, []string{"middleware.JWTAuth", "relaxedLimiter"}, middlewares)
	assert.Equal(t, []string{"deps.GetMe"}, handlers)
}
