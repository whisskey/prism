// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"
	"testing"

	"github.com/whisskey/prism/pkg/openapi"
	"github.com/stretchr/testify/assert"
)

func TestApplyAuthFromMiddlewareNames_JWTAuth(t *testing.T) {
	p := New()
	route := &openapi.Route{Method: "GET", Path: "/api/v1/users/me"}
	p.applyAuthFromMiddlewareNames(route, []string{"middleware.JWTAuth", "relaxedLimiter"})
	requireSecurityScheme(t, route, "BearerAuth")
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
	name := middlewareNameFromExpr(astIdent("authMiddleware"))
	assert.Equal(t, "authMiddleware", name)
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
