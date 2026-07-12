// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
)

func TestPreferAnnotation(t *testing.T) {
	assert.Equal(t, "Annotated", PreferAnnotation("Inferred", "Annotated"))
	assert.Equal(t, "Inferred", PreferAnnotation("Inferred", ""))
}

func TestEmptySchemaOnUnresolved(t *testing.T) {
	pkg, _, _ := parseSchemaSource(t, `package p`)
	ctx := Context{
		Pass: &analysis.Pass{Pkg: pkg},
		Warn: NewWarnLogger(false),
	}
	schemas := SchemaBuilderPass(ctx, map[string]HandlerDetail{
		"h": {ResponseType: "MissingType"},
	})
	require.Contains(t, schemas, "MissingType")
	assert.Equal(t, "object", schemas["MissingType"].Type)
	assert.Empty(t, schemas["MissingType"].Properties)
	require.Len(t, ctx.Warn.All(), 1)
	assert.Contains(t, ctx.Warn.All()[0].Message, "MissingType")
}

func TestWarnMissingResponseType(t *testing.T) {
	logger := NewWarnLogger(false)
	ctx := Context{Warn: logger}
	warnMissingResponseType(ctx, HandlerDetail{Name: "getUser", SourceFile: "h.go", SourceLine: 10})
	require.Len(t, logger.All(), 1)
	assert.Contains(t, logger.All()[0].Message, "getUser")
	assert.Contains(t, logger.All()[0].Message, "response type")
}

func TestRouteWithoutHandler(t *testing.T) {
	rc := newRouteCollector()
	rc.routes = append(rc.routes, rc.newRoute("Get", "/health", "", nil, "", token.Position{
		Filename: "routes.go",
		Line:     1,
	}, nil))
	assert.Equal(t, "", rc.routes[0].Handler)
	assert.Equal(t, "GET", rc.routes[0].Method)
	assert.Equal(t, "/health", rc.routes[0].Path)
}

func TestIsAnyType(t *testing.T) {
	assert.True(t, isAnyType(types.Universe.Lookup("any").Type()))
}
