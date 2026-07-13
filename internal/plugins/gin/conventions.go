// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package gin

import (
	"go/ast"
	"strings"
)

var (
	genericValidatorNames = []string{
		"RequestValidator",
		"ValidateJSON",
		"ValidateBody",
		"ValidatePayload",
		"ValidateRequest",
		"BodyValidator",
	}

	contextPayloadKeys = []string{
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

func selectorNameFromExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	default:
		return ""
	}
}

func isContextPayloadKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, candidate := range contextPayloadKeys {
		if key == candidate {
			return true
		}
	}
	return false
}

func schemaLooksLikeIDParam(schemaName string) bool {
	return schemaName == "IDParamRequest" || strings.Contains(schemaName, "IDParam")
}
