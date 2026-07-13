// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package typecheck

import (
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyAndStructFields(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	root := FindModuleRoot([]string{filepath.Join(wd, "loader.go")})
	require.NotEmpty(t, root)

	prog, err := Load([]string{filepath.Join(root, "internal/typecheck/types.go")})
	require.NoError(t, err)
	require.NotNil(t, prog)

	pass := prog.PassForFile(filepath.Join(root, "internal/typecheck/types.go"))
	require.NotNil(t, pass)
	require.NotNil(t, pass.Info)

	obj := pass.Pkg.Scope().Lookup("StructField")
	require.NotNil(t, obj)

	typ := obj.Type()
	assert.Equal(t, KindNamed, Classify(typ))

	fields := StructFields(typ)
	require.NotEmpty(t, fields)

	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name
		assert.NotNil(t, f.Type)
	}
	assert.Contains(t, names, "Name")
	assert.Contains(t, names, "Type")
	assert.Contains(t, names, "Tag")
}

func TestPass_ReceiverType(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	root := FindModuleRoot([]string{filepath.Join(wd, "loader.go")})
	require.NotEmpty(t, root)

	prog, err := Load([]string{filepath.Join(root, "internal/plugins/fiber/fiber.go")})
	require.NoError(t, err)
	require.NotNil(t, prog)

	pass := prog.PassForFile(filepath.Join(root, "internal/plugins/fiber/fiber.go"))
	require.NotNil(t, pass)

	var checked bool
	// Walk method bodies with *Plugin receiver for p.Method calls.
	for _, decl := range pass.File.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Body == nil {
			continue
		}
		recvType := pass.Info.TypeOf(fn.Recv.List[0].Type)
		if recvType == nil || !strings.Contains(TypeName(recvType), "Plugin") {
			continue
		}
		astWalkBlock(fn.Body, func(call *ast.CallExpr) {
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return
			}
			recvIdent, ok := sel.X.(*ast.Ident)
			if !ok || recvIdent.Name != "p" {
				return
			}
			recvType := pass.ReceiverType(sel)
			require.NotNil(t, recvType)
			checked = true
			assert.Contains(t, TypeName(recvType), "Plugin")
		})
	}
	assert.True(t, checked, "expected at least one typed p.Method call")
}

func TestIsInterface(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	root := FindModuleRoot([]string{filepath.Join(wd, "loader.go")})
	prog, err := Load([]string{filepath.Join(root, "internal/plugins/plugin.go")})
	require.NoError(t, err)
	require.NotNil(t, prog)

	pass := prog.PassForFile(filepath.Join(root, "internal/plugins/plugin.go"))
	require.NotNil(t, pass)

	obj := pass.Pkg.Scope().Lookup("FrameworkPlugin")
	require.NotNil(t, obj)
	assert.True(t, IsInterface(obj.Type()))
}

func astWalkBlock(block *ast.BlockStmt, fn func(*ast.CallExpr)) {
	if block == nil {
		return
	}
	for _, stmt := range block.List {
		astWalkStmt(stmt, fn)
	}
}

func astWalkStmt(stmt ast.Stmt, fn func(*ast.CallExpr)) {
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		if call, ok := s.X.(*ast.CallExpr); ok {
			fn(call)
		}
	case *ast.BlockStmt:
		astWalkBlock(s, fn)
	case *ast.IfStmt:
		astWalkBlock(s.Body, fn)
		if s.Else != nil {
			astWalkStmt(s.Else, fn)
		}
	case *ast.ReturnStmt:
		for _, r := range s.Results {
			if call, ok := r.(*ast.CallExpr); ok {
				fn(call)
			}
		}
	}
}
