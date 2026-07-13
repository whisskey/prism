// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"
	gotypes "go/types"
	"strings"

	"github.com/whisskey/prism/internal/analyzer/fibergen"
	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/pkg/openapi"
)

func (p *Plugin) splitMiddlewareArgs(args []ast.Expr, ctx *extractionContext) []ast.Expr {
	if len(args) <= 1 {
		return nil
	}
	handlerIdx := p.lastHandlerArgIndex(ctx.file, args)
	if handlerIdx <= 0 {
		return nil
	}
	return append([]ast.Expr(nil), args[:handlerIdx]...)
}

func (p *Plugin) lastHandlerArgIndex(pf *parser.ParsedFile, args []ast.Expr) int {
	if len(args) == 0 {
		return -1
	}
	pass := p.typeProgram.PassForFile(p.currentFilePath(pf))
	if pass == nil {
		return len(args) - 1
	}

	handlerIndices := make([]int, 0, len(args))
	for i, arg := range args {
		if isFiberHandlerType(pass, arg) {
			handlerIndices = append(handlerIndices, i)
		}
	}
	if len(handlerIndices) == 0 {
		return len(args) - 1
	}
	return handlerIndices[len(handlerIndices)-1]
}

func isFiberHandlerType(pass interface {
	TypeOf(expr ast.Expr) gotypes.Type
}, expr ast.Expr) bool {
	if pass == nil || expr == nil {
		return false
	}
	return isHandlerSignature(pass.TypeOf(expr))
}

func isHandlerSignature(typ gotypes.Type) bool {
	if typ == nil {
		return false
	}
	sig, ok := typ.Underlying().(*gotypes.Signature)
	if !ok {
		return false
	}
	if sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		return false
	}
	paramStr := gotypes.TypeString(sig.Params().At(0).Type(), nil)
	if !strings.Contains(paramStr, "fiber.Ctx") {
		return false
	}
	return gotypes.Identical(sig.Results().At(0).Type(), gotypes.Universe.Lookup("error").Type())
}

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
