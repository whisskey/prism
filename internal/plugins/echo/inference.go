// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package echo

import (
	"go/ast"
	"path"
)

type responseHelperOverride struct {
	kind   responseHelperKind
	schema string
}

func (p *Plugin) configuredResponseHelper(callExpr *ast.CallExpr) (responseHelperOverride, bool) {
	callName := callName(callExpr)
	if callName == "" {
		return responseHelperOverride{}, false
	}
	for _, rule := range p.inference.ResponseHelpers {
		if !matchesInferencePattern(rule.Pattern, callName) {
			continue
		}
		switch rule.Kind {
		case string(responseHelperSuccess):
			return responseHelperOverride{kind: responseHelperSuccess, schema: rule.Schema}, true
		case string(responseHelperError):
			return responseHelperOverride{kind: responseHelperError, schema: rule.Schema}, true
		}
	}
	return responseHelperOverride{}, false
}

func (p *Plugin) configuredMiddlewareStatuses(expr ast.Expr) []string {
	callExpr, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil
	}
	callName := callName(callExpr)
	if callName == "" {
		return nil
	}
	var statuses []string
	for _, rule := range p.inference.Middleware {
		if matchesInferencePattern(rule.Pattern, callName) {
			statuses = append(statuses, rule.Responses...)
		}
	}
	return statuses
}

func (p *Plugin) defaultErrorSchemaName() string {
	if p.inference.Errors.DefaultSchema != "" {
		return p.inference.Errors.DefaultSchema
	}
	return "ErrorResponse"
}

func callName(callExpr *ast.CallExpr) string {
	switch fn := callExpr.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return selectorName(fn.X) + "." + fn.Sel.Name
	default:
		return ""
	}
}

func selectorName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return selectorName(e.X) + "." + e.Sel.Name
	default:
		return ""
	}
}

func matchesInferencePattern(patternValue, callName string) bool {
	if patternValue == "" || callName == "" {
		return false
	}
	if ok, _ := path.Match(patternValue, callName); ok {
		return true
	}
	if ok, _ := path.Match(patternValue, shortCallName(callName)); ok {
		return true
	}
	return false
}

func shortCallName(callName string) string {
	for i := len(callName) - 1; i >= 0; i-- {
		if callName[i] == '.' {
			return callName[i+1:]
		}
	}
	return callName
}
