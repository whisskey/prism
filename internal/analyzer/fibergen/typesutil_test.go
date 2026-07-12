// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
)

func TestTypeBaseName_GenericUnwrap(t *testing.T) {
	_, info, file := parseSchemaSource(t, `
package p

type User struct{}
type Response[T any] struct {
	Data T
}

func f() Response[User] {
	return Response[User]{}
}
`)

	var ret *ast.CompositeLit
	ast.Inspect(file, func(n ast.Node) bool {
		if cl, ok := n.(*ast.CompositeLit); ok {
			ret = cl
			return false
		}
		return true
	})
	require.NotNil(t, ret)

	name := resolveTypeName(&analysis.Pass{TypesInfo: info}, ret, nil)
	assert.Equal(t, "User", name)
}

func TestGenericTypeFromASTExpr(t *testing.T) {
	name := genericTypeFromASTExpr(&ast.IndexExpr{
		Index: &ast.Ident{Name: "CreateUserRequest"},
	})
	assert.Equal(t, "CreateUserRequest", name)
}
