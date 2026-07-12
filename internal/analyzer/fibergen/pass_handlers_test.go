// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
)

func TestTypeNameFromGoType(t *testing.T) {
	_, info, file := parseAndCheck(t, `
package p

type User struct{}

func f() User { return User{} }
`)
	lit := firstCompositeLit(t, file, "f")
	userType := info.TypeOf(lit)
	assert.Equal(t, "User", typeNameFromGoType(userType))
	assert.Equal(t, "[]User", typeNameFromGoType(types.NewSlice(userType)))
}

func TestAnalyzeHandler_RequestResponseAndParams(t *testing.T) {
	_, info, file := parseAndCheck(t, `
package p

type User struct{}
type CreateUserRequest struct{}

type mockCtx struct{}

func (*mockCtx) BodyParser(any) error { return nil }
func (*mockCtx) QueryInt(string, int) int { return 0 }
func (*mockCtx) Params(string) string { return "" }
func (*mockCtx) JSON(any) error { return nil }

func listUsersHandler(c *mockCtx) error {
	var req CreateUserRequest
	c.BodyParser(&req)
	page := c.QueryInt("page", 1)
	id := c.Params("id")
	_ = page
	_ = id
	return c.JSON([]User{})
}
`)

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == "listUsersHandler" {
			fn = d
			break
		}
	}
	require.NotNil(t, fn)

	ctx := Context{Pass: &analysis.Pass{TypesInfo: info}}
	infoResult := analyzeHandler(ctx, fn, "listUsersHandler", "handler.go", 10)

	assert.Equal(t, "CreateUserRequest", infoResult.RequestBody)
	assert.Equal(t, "[]User", infoResult.ResponseType)
	require.Len(t, infoResult.QueryParams, 1)
	assert.Equal(t, "page", infoResult.QueryParams[0].Name)
	assert.Equal(t, "integer", infoResult.QueryParams[0].GoType)
	require.Len(t, infoResult.PathParams, 1)
	assert.Equal(t, "id", infoResult.PathParams[0].Name)
	assert.Equal(t, "string", infoResult.PathParams[0].GoType)
	assert.True(t, infoResult.PathParams[0].Required)
}

func TestResolveTypeName_CompositeLit(t *testing.T) {
	_, info, file := parseAndCheck(t, `
package p
type User struct{}
func f() User { return User{} }
`)
	var lit *ast.CompositeLit
	ast.Inspect(file, func(n ast.Node) bool {
		if cl, ok := n.(*ast.CompositeLit); ok {
			lit = cl
			return false
		}
		return true
	})
	require.NotNil(t, lit)

	name := resolveTypeName(&analysis.Pass{TypesInfo: info}, lit, nil)
	assert.Equal(t, "User", name)
}

func parseAndCheck(t *testing.T, src string) (*types.Package, *types.Info, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "handler.go", src, 0)
	require.NoError(t, err)

	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	cfg := &types.Config{Importer: importer.Default()}
	pkg, err := cfg.Check("p", fset, []*ast.File{file}, info)
	require.NoError(t, err)
	return pkg, info, file
}

func firstCompositeLit(t *testing.T, file *ast.File, funcName string) *ast.CompositeLit {
	t.Helper()
	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == funcName {
			fn = d
			break
		}
	}
	require.NotNil(t, fn)
	var lit *ast.CompositeLit
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if cl, ok := n.(*ast.CompositeLit); ok {
			lit = cl
			return false
		}
		return true
	})
	require.NotNil(t, lit)
	return lit
}
