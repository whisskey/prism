// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"
	"strings"
)

// Built-in Fiber conventions (Scramble-style defaults, no yaml required).
var (
	genericValidatorNames = []string{
		"RequestValidator",
		"ValidateJSON",
		"ValidateBody",
		"ValidatePayload",
		"ValidateRequest",
		"BodyValidator",
	}

	localsPayloadKeys = []string{
		"payload",
		"request",
		"body",
	}
)

func isGenericValidatorName(name string) bool {
	for _, candidate := range genericValidatorNames {
		if candidate == name {
			return true
		}
	}
	return false
}

func genericValidatorSchema(expr ast.Expr) string {
	callExpr, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}

	switch fun := callExpr.Fun.(type) {
	case *ast.IndexExpr:
		if isGenericValidatorName(selectorNameFromExpr(fun.X)) {
			return cleanTypeName(astTypeString(fun.Index))
		}
	case *ast.IndexListExpr:
		if isGenericValidatorName(selectorNameFromExpr(fun.X)) && len(fun.Indices) > 0 {
			return cleanTypeName(astTypeString(fun.Indices[0]))
		}
	}
	return ""
}

func (p *Plugin) middlewareRequestSchema(expr ast.Expr) string {
	if schema := genericValidatorSchema(expr); schema != "" {
		return schema
	}
	if schema := genericMiddlewareTypeArg(expr); schema != "" {
		return schema
	}
	callExpr, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}
	funcDecl := p.resolveHelperFunc(callExpr)
	if funcDecl == nil {
		return ""
	}
	return p.middlewareBodySchemaFromFuncDecl(funcDecl)
}

func genericMiddlewareTypeArg(expr ast.Expr) string {
	callExpr, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}
	switch fun := callExpr.Fun.(type) {
	case *ast.IndexExpr:
		return cleanTypeName(astTypeString(fun.Index))
	case *ast.IndexListExpr:
		if len(fun.Indices) > 0 {
			return cleanTypeName(astTypeString(fun.Indices[0]))
		}
	}
	return ""
}

func validatorSchemaFromFuncBody(funcDecl *ast.FuncDecl) string {
	if funcDecl == nil || funcDecl.Body == nil {
		return ""
	}

	var schemaName string
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		if schemaName != "" {
			return false
		}
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if name := schemaFromValidatorCall(callExpr); name != "" {
			schemaName = name
			return false
		}
		return true
	})
	return schemaName
}

func schemaFromValidatorCall(callExpr *ast.CallExpr) string {
	if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
		switch selExpr.Sel.Name {
		case "BodyParser":
			if len(callExpr.Args) > 0 {
				return schemaNameFromBodyParserArg(callExpr.Args[0])
			}
		case "Body":
			if len(callExpr.Args) > 0 {
				return schemaNameFromBodyParserArg(callExpr.Args[0])
			}
		case "Unmarshal":
			if len(callExpr.Args) >= 2 {
				return schemaNameFromBodyParserArg(callExpr.Args[1])
			}
		}
	}
	switch callNameFromExpr(callExpr) {
	case "bindAndValidate", "bindBody":
		if len(callExpr.Args) >= 2 {
			if name := schemaNameFromBodyParserArg(callExpr.Args[1]); name != "" {
				return name
			}
		}
		if len(callExpr.Args) >= 1 {
			return schemaNameFromBodyParserArg(callExpr.Args[0])
		}
	}
	return ""
}

func callNameFromExpr(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	default:
		return ""
	}
}

func schemaNameFromBodyParserArg(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		if e.Op.String() == "&" {
			return cleanTypeName(astTypeString(e.X))
		}
	case *ast.Ident:
		return ""
	case *ast.CallExpr:
		if ident, ok := e.Fun.(*ast.Ident); ok && ident.Name == "new" && len(e.Args) > 0 {
			return cleanTypeName(astTypeString(e.Args[0]))
		}
	}
	return ""
}

func isLocalsPayloadKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, candidate := range localsPayloadKeys {
		if key == candidate {
			return true
		}
	}
	return false
}
