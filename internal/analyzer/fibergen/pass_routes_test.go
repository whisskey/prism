// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"go/ast"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFiberPathToOpenAPI(t *testing.T) {
	assert.Equal(t, "/users/{id}", fiberPathToOpenAPI("/users/:id"))
	assert.Equal(t, "/users/{user_id}/posts/{post_id}", fiberPathToOpenAPI("users/:user_id/posts/:post_id"))
}

func TestJoinPathPrefix(t *testing.T) {
	assert.Equal(t, "/v1", joinPathPrefix("", "/v1"))
	assert.Equal(t, "/v1/users", joinPathPrefix("/v1", "/users"))
	assert.Equal(t, "/v1/users", joinPathPrefix("/v1/", "users"))
	assert.Equal(t, "/api/v1", joinPathPrefix("/api", "/v1"))
}

func TestRouteCollector_resolveStringArg(t *testing.T) {
	rc := newRouteCollector()
	rc.constVals["basePath"] = "/api/v1"

	assert.Equal(t, "/users", rc.resolveStringArg(&ast.BasicLit{Kind: token.STRING, Value: `"/users"`}))
	assert.Equal(t, "/api/v1", rc.resolveStringArg(&ast.Ident{Name: "basePath"}))
	assert.Equal(t, "", rc.resolveStringArg(&ast.Ident{Name: "unknown"}))
}

func TestRouteCollector_splitHandlersAndMiddlewares(t *testing.T) {
	rc := newRouteCollector()
	handlers, middlewares := rc.splitHandlersAndMiddlewares(nil, []ast.Expr{
		&ast.Ident{Name: "authMiddleware"},
		&ast.SelectorExpr{
			X:   &ast.Ident{Name: "userCtrl"},
			Sel: &ast.Ident{Name: "GetAll"},
		},
	})
	assert.Equal(t, []string{"userCtrl.GetAll"}, handlers)
	assert.Equal(t, []string{"authMiddleware"}, middlewares)
}

func TestRouteCollector_splitHandlersAndMiddlewares_SingleHandler(t *testing.T) {
	rc := newRouteCollector()
	handlers, middlewares := rc.splitHandlersAndMiddlewares(nil, []ast.Expr{
		&ast.Ident{Name: "getUserHandler"},
	})
	assert.Equal(t, []string{"getUserHandler"}, handlers)
	assert.Nil(t, middlewares)
}
