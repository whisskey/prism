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

func TestBuildSchema_BasicFieldsAndValidate(t *testing.T) {
	pkg, info, _ := parseSchemaSource(t, `
package p

type CreateUserRequest struct {
	Email string ` + "`json:\"email\" validate:\"required,email\"`" + `
	Age   int    ` + "`json:\"age\" validate:\"min=18,max=120\"`" + `
	Note  string ` + "`json:\"note,omitempty\"`" + `
}
`)

	pass := &analysis.Pass{Pkg: pkg, TypesInfo: info}
	schema := buildSchema(pass, "CreateUserRequest")

	assert.Equal(t, "object", schema.Type)
	require.Contains(t, schema.Properties, "email")
	assert.Equal(t, "string", schema.Properties["email"].Type)
	assert.Equal(t, "email", schema.Properties["email"].Format)

	require.Contains(t, schema.Properties, "age")
	assert.Equal(t, "integer", schema.Properties["age"].Type)
	require.NotNil(t, schema.Properties["age"].Minimum)
	assert.Equal(t, float64(18), *schema.Properties["age"].Minimum)
	require.NotNil(t, schema.Properties["age"].Maximum)
	assert.Equal(t, float64(120), *schema.Properties["age"].Maximum)

	assert.Contains(t, schema.Required, "email")
	assert.Contains(t, schema.Required, "age")
	assert.NotContains(t, schema.Required, "note")
}

func TestGoTypeToOpenAPIProperty_NamedAndSlice(t *testing.T) {
	pkg, info, _ := parseSchemaSource(t, `
package p

type User struct {
	Name string ` + "`json:\"name\"`" + `
}

type ListResponse struct {
	Users []User ` + "`json:\"users\"`" + `
}
`)

	pass := &analysis.Pass{Pkg: pkg, TypesInfo: info}
	userObj := pkg.Scope().Lookup("User")
	require.NotNil(t, userObj)
	named, ok := userObj.Type().(*types.Named)
	require.True(t, ok)

	refProp := goTypeToOpenAPIProperty(pass, named)
	assert.Equal(t, "#/components/schemas/User", refProp.Ref)

	listObj := pkg.Scope().Lookup("ListResponse")
	strct := listObj.Type().Underlying().(*types.Struct)
	usersField := strct.Field(0)
	arrayProp := goTypeToOpenAPIProperty(pass, usersField.Type())
	assert.Equal(t, "array", arrayProp.Type)
	require.NotNil(t, arrayProp.Items)
	assert.Equal(t, "#/components/schemas/User", arrayProp.Items.Ref)
}

func TestBasicTypeToProperty(t *testing.T) {
	assert.Equal(t, Property{Type: "boolean"}, basicTypeToProperty(types.Typ[types.Bool]))
	assert.Equal(t, Property{Type: "integer"}, basicTypeToProperty(types.Typ[types.Int]))
	assert.Equal(t, Property{Type: "number", Format: "float"}, basicTypeToProperty(types.Typ[types.Float64]))
}

func TestApplyValidateConstraints_StringLength(t *testing.T) {
	prop := Property{Type: "string"}
	applyValidateConstraints(&prop, "min=3,max=50", types.Typ[types.String])
	require.NotNil(t, prop.MinLength)
	assert.Equal(t, 3, *prop.MinLength)
	require.NotNil(t, prop.MaxLength)
	assert.Equal(t, 50, *prop.MaxLength)
}

func TestSchemaBuilderPass_ReferencedTypes(t *testing.T) {
	pkg, info, _ := parseSchemaSource(t, `
package p

type User struct {
	ID string ` + "`json:\"id\"`" + `
}
`)

	pass := &analysis.Pass{Pkg: pkg, TypesInfo: info}
	ctx := Context{Pass: pass}
	handlers := map[string]HandlerDetail{
		"getUser": {ResponseType: "User"},
	}

	schemas := SchemaBuilderPass(ctx, handlers)
	require.Contains(t, schemas, "User")
	assert.Equal(t, "object", schemas["User"].Type)
}

func parseSchemaSource(t *testing.T, src string) (*types.Package, *types.Info, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "schema.go", src, 0)
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
