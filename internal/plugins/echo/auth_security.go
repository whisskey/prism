// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package echo

import (
	"go/ast"

	"github.com/whisskey/prism/internal/analyzer/fibergen"
	"github.com/whisskey/prism/pkg/openapi"
)

func (p *Plugin) applyAuthSecurity(route *openapi.Route, middlewareArgs []ast.Expr) {
	if route == nil {
		return
	}
	names := middlewareNamesFromExprs(middlewareArgs)
	for _, ref := range route.Middleware {
		names = append(names, ref.Name)
	}
	p.applyAuthFromMiddlewareNames(route, names)

	if routeHasBearerAuth(route) {
		return
	}
	for _, arg := range middlewareArgs {
		statuses := p.configuredMiddlewareStatuses(arg)
		statuses = append(statuses, p.middlewareStatuses(arg)...)
		for _, status := range statuses {
			if status == "401" {
				route.Security = append(route.Security, map[string][]string{"BearerAuth": {}})
				return
			}
		}
	}
}

func (p *Plugin) applyAuthFromMiddlewareNames(route *openapi.Route, names []string) {
	if route == nil {
		return
	}
	for _, name := range names {
		scheme, ok := fibergen.SchemeForMiddleware(name)
		if !ok || scheme == "" {
			continue
		}
		if routeHasSecurityScheme(route, scheme) {
			continue
		}
		route.Security = append(route.Security, map[string][]string{scheme: {}})
	}
}

func middlewareNamesFromExprs(exprs []ast.Expr) []string {
	if len(exprs) == 0 {
		return nil
	}
	names := make([]string, 0, len(exprs))
	for _, expr := range exprs {
		if name := middlewareNameFromExpr(expr); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func routeHasBearerAuth(route *openapi.Route) bool {
	return routeHasSecurityScheme(route, "BearerAuth")
}

func routeHasSecurityScheme(route *openapi.Route, scheme string) bool {
	for _, req := range route.Security {
		if _, ok := req[scheme]; ok {
			return true
		}
	}
	return false
}
