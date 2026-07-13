// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"
	"strings"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/pkg/openapi"
)

func (p *Plugin) querySchemaFromHelperCall(callExpr *ast.CallExpr, pf *parser.ParsedFile) string {
	if callExpr == nil {
		return ""
	}
	helperDecl := p.resolveHelperFunc(callExpr)
	if helperDecl == nil {
		return ""
	}
	return p.querySchemaFromFuncDecl(helperDecl, pf)
}

func (p *Plugin) querySchemaFromFuncDecl(funcDecl *ast.FuncDecl, pf *parser.ParsedFile) string {
	if funcDecl == nil || funcDecl.Body == nil {
		return ""
	}

	localTypes := make(map[string]string)
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.DeclStmt:
			p.collectDeclTypes(node, localTypes)
		case *ast.AssignStmt:
			p.collectAssignTypes(node, localTypes)
		}
		return true
	})

	var schemaName string
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		if schemaName != "" {
			return false
		}
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if name := p.requestSchemaFromQueryBind(callExpr, pf, localTypes); name != "" {
			schemaName = name
			return false
		}
		return true
	})
	return schemaName
}

func (p *Plugin) requestSchemaFromQueryBind(callExpr *ast.CallExpr, pf *parser.ParsedFile, localTypes map[string]string) string {
	if !isQueryBindCall(callExpr) || len(callExpr.Args) == 0 {
		return ""
	}
	arg := callExpr.Args[0]
	if typed := p.typedSchemaName(pf, arg); typed != "" {
		return cleanTypeName(typed)
	}
	return cleanTypeName(p.schemaNameFromExpr(arg, localTypes))
}

func (p *Plugin) queryParamsFromSchemaName(schemaName string) []ParameterResolution {
	schemaName = cleanTypeName(schemaName)
	if schemaName == "" || p.structIndex == nil {
		return nil
	}
	def, ok := p.structIndex[schemaName]
	if !ok {
		return nil
	}

	var params []ParameterResolution
	for _, field := range def.Fields {
		name := queryParamName(field)
		if name == "" {
			continue
		}
		params = appendParameterResolution(params, ParameterResolution{
			Name:   name,
			In:     "query",
			Schema: p.schemaExtractor.FieldSchema(field),
		})
	}
	return params
}

func queryParamName(field parser.StructField) string {
	if field.QueryName != "" && field.QueryName != "-" {
		return field.QueryName
	}
	return ""
}

func appendUniqueSchemaNames(existing []string, name string) []string {
	name = cleanTypeName(name)
	if name == "" {
		return existing
	}
	for _, current := range existing {
		if current == name {
			return existing
		}
	}
	return append(existing, name)
}

func firstCallExprFromAssign(stmt *ast.AssignStmt) *ast.CallExpr {
	if stmt == nil || len(stmt.Rhs) == 0 {
		return nil
	}
	callExpr, ok := stmt.Rhs[0].(*ast.CallExpr)
	if !ok {
		return nil
	}
	return callExpr
}

func querySchemaFromAssign(stmt *ast.AssignStmt, p *Plugin, pf *parser.ParsedFile) string {
	callExpr := firstCallExprFromAssign(stmt)
	if callExpr == nil {
		return ""
	}
	return p.querySchemaFromHelperCall(callExpr, pf)
}

func enrichQueryParameter(param *openapi.Parameter) {
	if param == nil {
		return
	}
	switch strings.ToLower(param.Name) {
	case "page":
		if param.Description == "" {
			param.Description = "Page number"
		}
	case "per_page", "perpage":
		if param.Description == "" {
			param.Description = "Items per page"
		}
	case "limit":
		if param.Description == "" {
			param.Description = "Maximum number of items to return"
		}
	case "offset":
		if param.Description == "" {
			param.Description = "Number of items to skip"
		}
	}
	if param.Schema == nil {
		return
	}
	switch strings.ToLower(param.Name) {
	case "page":
		if param.Schema.Type == "integer" && param.Schema.Default == nil {
			param.Schema.Default = 1
			param.Example = 1
		}
	case "per_page":
		if param.Schema.Type == "integer" && param.Schema.Default == nil {
			param.Schema.Default = 10
			param.Example = 10
		}
	}
}
