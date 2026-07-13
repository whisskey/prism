// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package chi

import (
	"go/ast"
	"strings"

	"github.com/whisskey/prism/pkg/openapi"
)

func (p *Plugin) collectQueryParamsFromHandler(funcDecl *ast.FuncDecl) []ParameterResolution {
	if funcDecl == nil || funcDecl.Body == nil {
		return nil
	}
	seen := make(map[string]bool)
	var params []ParameterResolution
	add := func(param ParameterResolution) {
		key := param.Name + "." + param.In
		if param.Name == "" || seen[key] {
			return
		}
		seen[key] = true
		params = append(params, param)
	}

	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if name, schema := queryParamFromCall(callExpr); name != "" {
			add(ParameterResolution{Name: name, In: "query", Schema: schema})
		}
		if schemaName := p.querySchemaFromHelperCall(callExpr); schemaName != "" {
			for _, param := range p.parametersFromQuerySchema(schemaName) {
				add(param)
			}
		}
		return true
	})
	return params
}

func (p *Plugin) querySchemaFromHelperCall(callExpr *ast.CallExpr) string {
	name := callNameFromExpr(callExpr)
	lower := strings.ToLower(name)
	if !strings.Contains(lower, "pagination") && !strings.Contains(lower, "query") {
		return ""
	}
	helper := p.resolveHelperFunc(callExpr)
	if helper == nil || helper.Body == nil {
		return ""
	}
	var schemaName string
	ast.Inspect(helper.Body, func(n ast.Node) bool {
		if schemaName != "" {
			return false
		}
		switch node := n.(type) {
		case *ast.DeclStmt:
			gen, ok := node.Decl.(*ast.GenDecl)
			if !ok {
				return true
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || vs.Type == nil {
					continue
				}
				if name := cleanTypeName(astTypeString(vs.Type)); name != "" {
					schemaName = name
					return false
				}
			}
		case *ast.AssignStmt:
			for _, rhs := range node.Rhs {
				if lit, ok := rhs.(*ast.CompositeLit); ok {
					if name := typeBaseName(lit.Type); name != "" {
						schemaName = name
						return false
					}
				}
			}
		}
		return true
	})
	return schemaName
}

func enrichQueryParameter(param *openapi.Parameter) {
	if param == nil || param.Schema == nil {
		return
	}
	switch param.Name {
	case "page":
		param.Schema.Type = "integer"
		if param.Description == "" {
			param.Description = "Page number"
		}
	case "per_page":
		param.Schema.Type = "integer"
		if param.Description == "" {
			param.Description = "Items per page"
		}
	}
}
