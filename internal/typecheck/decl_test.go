// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package typecheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFuncDeclForCall_AndName(t *testing.T) {
	dir := t.TempDir()
	mod := `module example.com/decltest

go 1.22
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644))

	mwPath := filepath.Join(dir, "middleware.go")
	mwSrc := `package main

func JWTAuth() {}
func RequireRole(role string) {}
`
	require.NoError(t, os.WriteFile(mwPath, []byte(mwSrc), 0o644))

	routerPath := filepath.Join(dir, "router.go")
	routerSrc := `package main

func Register() {
	_ = JWTAuth()
	_ = RequireRole("admin")
}
`
	require.NoError(t, os.WriteFile(routerPath, []byte(routerSrc), 0o644))

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, routerPath, routerSrc, 0)
	require.NoError(t, err)

	var jwtCall *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "JWTAuth" {
			return true
		}
		jwtCall = call
		return false
	})
	require.NotNil(t, jwtCall)

	pos := fset.Position(jwtCall.Pos())
	prog, err := Load([]string{routerPath, mwPath})
	require.NoError(t, err)
	require.NotNil(t, prog)

	fn := prog.FuncDeclForCall(routerPath, ExprPosition{Line: pos.Line, Column: pos.Column})
	require.NotNil(t, fn, "FuncDeclForCall at %d:%d", pos.Line, pos.Column)
	assert.Equal(t, "JWTAuth", fn.Name.Name)

	requireRole := prog.FuncDeclForName("RequireRole")
	require.NotNil(t, requireRole)
	assert.Equal(t, "RequireRole", requireRole.Name.Name)
}
