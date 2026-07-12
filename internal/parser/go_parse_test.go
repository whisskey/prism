// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package parser

import (
	"go/ast"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGoSource_UserStruct(t *testing.T) {
	src := `package models

type User struct {
	ID   int    ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}
`

	file, err := parseGoSource("models.go", src, defaultParseOptions())
	require.NoError(t, err)
	require.NotNil(t, file.AST)

	typeSpec := TypeSpecDecl(file.AST, "User")
	require.NotNil(t, typeSpec)
	assert.Equal(t, "TypeSpec", NodeKind(typeSpec))

	structType, ok := typeSpec.Type.(*ast.StructType)
	require.True(t, ok)
	assert.Equal(t, "StructType", NodeKind(structType))

	fields := StructFieldDecls(file.AST, "User")
	require.Len(t, fields, 2)

	assert.Equal(t, "ID", fields[0].Names[0].Name)
	idType, ok := fields[0].Type.(*ast.Ident)
	require.True(t, ok)
	assert.Equal(t, "int", idType.Name)

	assert.Equal(t, "Name", fields[1].Names[0].Name)
	nameType, ok := fields[1].Type.(*ast.Ident)
	require.True(t, ok)
	assert.Equal(t, "string", nameType.Name)

	tag := fields[0].Tag
	require.NotNil(t, tag)
	assert.Equal(t, token.STRING, tag.Kind)
	assert.Contains(t, tag.Value, "json")
}

func TestParseGoSource_AllErrors(t *testing.T) {
	src := `package main
func main() {
	return
}
func broken() {`
	_, err := parseGoSource("main.go", src, defaultParseOptions())
	assert.Error(t, err)
}

func TestParseGoSource_ParseComments(t *testing.T) {
	src := `package main
// handler docs
func Handler() {}
`
	file, err := parseGoSource("main.go", src, defaultParseOptions())
	require.NoError(t, err)

	fn, ok := file.AST.Decls[0].(*ast.FuncDecl)
	require.True(t, ok)
	require.NotNil(t, fn.Doc)
	assert.Contains(t, fn.Doc.Text(), "handler docs")
}

func TestParseGoFileSet_SharesFileSetWithLexer(t *testing.T) {
	fset := token.NewFileSet()
	src := "package main\n"
	file, err := parseGoFileSet(fset, "main.go", src, defaultParseOptions())
	require.NoError(t, err)
	assert.Same(t, fset, file.FileSet)
}

func TestDefaultParseOptions(t *testing.T) {
	opts := defaultParseOptions()
	assert.True(t, opts.AllErrors)
	assert.True(t, opts.ParseComments)
}
