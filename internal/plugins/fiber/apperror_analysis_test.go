// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApperrorStatusFromCall(t *testing.T) {
	call := parseCallExpr(t, `apperror.NewUnauthorized("missing token")`)
	assert.Equal(t, "401", apperrorStatusFromCall(call))

	call = parseCallExpr(t, `apperror.NewBadRequest("invalid")`)
	assert.Equal(t, "400", apperrorStatusFromCall(call))
}

func TestBindQueryErrorStatusesFromFunc(t *testing.T) {
	source := `package handler

func paginationQuery(c interface {
	Bind() interface{ Query(any) error }
}) (int, int, error) {
	var q struct{}
	if err := c.Bind().Query(&q); err != nil {
		return 0, 0, err
	}
	return 1, 10, nil
}
`
	_, fn := parseFuncFromSource(t, source, "paginationQuery")
	statuses := bindQueryErrorStatusesFromFunc(fn)
	assert.Equal(t, []string{"400"}, statuses)
}

func parseCallExpr(t *testing.T, src string) *ast.CallExpr {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "call.go", "package main\nvar _ = "+src, parser.AllErrors)
	require.NoError(t, err)
	decl := file.Decls[0].(*ast.GenDecl)
	assign := decl.Specs[0].(*ast.ValueSpec)
	return assign.Values[0].(*ast.CallExpr)
}

func parseFuncFromSource(t *testing.T, source, name string) (*ast.File, *ast.FuncDecl) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", source, parser.AllErrors)
	require.NoError(t, err)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name {
			return file, fn
		}
	}
	t.Fatalf("function %s not found", name)
	return nil, nil
}
