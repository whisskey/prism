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

func TestAnalyzeNamedMiddleware_BindAndValidate(t *testing.T) {
	source := `package middleware

import "github.com/gofiber/fiber/v3"

type CreateUserRequest struct {
	Email string ` + "`json:\"email\"`" + `
}

func ValidateCreateUser(c fiber.Ctx) error {
	var req CreateUserRequest
	if err := bindAndValidate(c, &req); err != nil {
		return c.Status(400).JSON(map[string]string{"error": err.Error()})
	}
	return c.Next()
}

func bindAndValidate(c fiber.Ctx, req any) error {
	return c.Bind().Body(req)
}
`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "middleware.go", source, parser.ParseComments)
	require.NoError(t, err)

	p := New()
	p.routeFuncIndex = map[string]*ast.FuncDecl{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		p.routeFuncIndex[fn.Name.Name] = fn
	}

	analysis := p.analyzeNamedMiddleware("ValidateCreateUser")
	assert.Equal(t, "ValidateCreateUser", analysis.Name)
	assert.Equal(t, "CreateUserRequest", analysis.ValidatorSchema)
	assert.Contains(t, analysis.ErrorStatuses, "400")
}

func TestValidatorSchemaFromFuncBody_BindBody(t *testing.T) {
	source := `package mw

import "github.com/gofiber/fiber/v3"

type Payload struct {
	Name string ` + "`json:\"name\"`" + `
}

func bindBody(c fiber.Ctx, req any) error {
	return c.Bind().Body(req)
}
`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "mw.go", source, 0)
	require.NoError(t, err)

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == "bindBody" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	assert.Equal(t, "", validatorSchemaFromFuncBody(fn))
}

func TestMergeMiddlewareAnalyses_Deduplicates(t *testing.T) {
	merged := mergeMiddlewareAnalyses(
		[]MiddlewareAnalysis{{Name: "auth", ValidatorSchema: "Token"}},
		[]MiddlewareAnalysis{{Name: "auth", ErrorStatuses: []string{"401"}}},
	)
	require.Len(t, merged, 1)
	assert.Equal(t, "auth", merged[0].Name)
	assert.Equal(t, "Token", merged[0].ValidatorSchema)
	assert.Contains(t, merged[0].ErrorStatuses, "401")
}
