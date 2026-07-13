// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"

	"github.com/whisskey/prism/pkg/openapi"
)

func (p *Plugin) applyRouteAnalysis(route *openapi.Route, analysis RouteAnalysis, middlewareExprs []ast.Expr) {
	graph := p.resolveRouteTypeGraph(route, analysis)
	p.applyRouteTypeGraph(route, graph)
	p.applyHandlerMetadata(route, analysis.Handler)
	p.applyAuthSecurity(route, middlewareExprs)
}

func astInspectDeclTypes(body *ast.BlockStmt, fn func(ast.Node) bool) {
	if body == nil {
		return
	}
	ast.Inspect(body, fn)
}
