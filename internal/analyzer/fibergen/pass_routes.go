// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var fiberPathParamRE = regexp.MustCompile(`:([a-zA-Z_][a-zA-Z0-9_]*)`)

// routeCollector discovers Fiber HTTP routes with group prefix and middleware support.
type routeCollector struct {
	routes          []Route
	groupPrefix     map[string]string
	groupMiddleware map[string][]string
	constVals       map[string]string
}

func newRouteCollector() *routeCollector {
	return &routeCollector{
		groupPrefix:     make(map[string]string),
		groupMiddleware: make(map[string][]string),
		constVals:       make(map[string]string),
	}
}

// RouteCollectorPass converts Fiber route registration calls into Route entries (Pass 1).
func RouteCollectorPass(ctx Context) []Route {
	if ctx.Pass == nil {
		return nil
	}
	rc := newRouteCollector()
	rc.collect(ctx)
	return rc.routes
}

func (rc *routeCollector) collect(ctx Context) {
	pass := ctx.Pass
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	insp.Preorder([]ast.Node{(*ast.AssignStmt)(nil)}, func(n ast.Node) {
		assign := n.(*ast.AssignStmt)
		if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return
		}
		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return
		}
		lit, ok := assign.Rhs[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return
		}
		rc.constVals[ident.Name] = strings.Trim(lit.Value, `"`)
	})

	insp.Preorder([]ast.Node{(*ast.AssignStmt)(nil), (*ast.ExprStmt)(nil)}, func(n ast.Node) {
		var call *ast.CallExpr

		switch stmt := n.(type) {
		case *ast.AssignStmt:
			if len(stmt.Rhs) != 1 {
				return
			}
			c, ok := stmt.Rhs[0].(*ast.CallExpr)
			if !ok {
				return
			}
			call = c
			if len(stmt.Lhs) == 1 {
				if ident, ok := stmt.Lhs[0].(*ast.Ident); ok {
					rc.handleGroupAssign(pass, call, ident.Name)
				}
			}
		case *ast.ExprStmt:
			c, ok := stmt.X.(*ast.CallExpr)
			if !ok {
				return
			}
			call = c
		}

		if call == nil {
			return
		}

		rc.collectRoute(ctx, call)
	})
}

func (rc *routeCollector) handleGroupAssign(pass *analysis.Pass, call *ast.CallExpr, varName string) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Group" {
		return
	}
	if len(call.Args) == 0 {
		return
	}
	if !isFiberReceiver(pass, sel.X) {
		return
	}
	groupPath := rc.resolveStringArg(call.Args[0])
	if groupPath == "" {
		return
	}
	parentPrefix := rc.groupPrefix[identName(sel.X)]
	rc.groupPrefix[varName] = joinPathPrefix(parentPrefix, groupPath)
	if len(call.Args) > 1 {
		for _, arg := range call.Args[1:] {
			if name := exprName(arg); name != "" {
				rc.groupMiddleware[varName] = append(rc.groupMiddleware[varName], name)
			}
		}
	}
}

func (rc *routeCollector) collectRoute(ctx Context, call *ast.CallExpr) {
	pass := ctx.Pass
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	method := sel.Sel.Name
	if !isHTTPMethod(method) {
		return
	}
	if !isFiberReceiver(pass, sel.X) {
		return
	}
	if len(call.Args) < 2 {
		return
	}

	path := rc.resolveStringArg(call.Args[0])
	if path == "" {
		return // non-literal path — silent skip
	}

	receiverName := identName(sel.X)
	prefix := rc.groupPrefix[receiverName]
	fullPath := fiberPathToOpenAPI(joinPathPrefix(prefix, path))

	handlers, middlewares := rc.splitHandlersAndMiddlewares(pass, call.Args[1:])
	if inherited := rc.groupMiddleware[receiverName]; len(inherited) > 0 {
		middlewares = append(append([]string(nil), inherited...), middlewares...)
	}
	pos := pass.Fset.Position(call.Pos())
	recvType := pass.TypesInfo.TypeOf(sel.X)

	if len(handlers) == 0 {
		rc.routes = append(rc.routes, rc.newRoute(method, fullPath, "", middlewares, prefix, pos, recvType))
		return
	}

	for _, h := range handlers {
		rc.routes = append(rc.routes, rc.newRoute(method, fullPath, h, middlewares, prefix, pos, recvType))
	}
}

func (rc *routeCollector) newRoute(method, path, handler string, middlewares []string, prefix string, pos token.Position, recvType types.Type) Route {
	return Route{
		Method:       strings.ToUpper(method),
		Path:         path,
		Handler:      handler,
		HandlerRef:   handler,
		Middlewares:  middlewares,
		Prefix:       prefix,
		SourceFile:   pos.Filename,
		SourceLine:   pos.Line,
		ReceiverType: types.TypeString(recvType, nil),
	}
}

func (rc *routeCollector) resolveStringArg(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			return strings.Trim(e.Value, `"`)
		}
	case *ast.Ident:
		if val, ok := rc.constVals[e.Name]; ok {
			return val
		}
	}
	return ""
}

func (rc *routeCollector) splitHandlersAndMiddlewares(pass *analysis.Pass, args []ast.Expr) (handlers, middlewares []string) {
	if len(args) == 0 {
		return nil, nil
	}

	var handlerArgs, middlewareArgs []ast.Expr
	for _, arg := range args {
		if isFiberHandlerType(pass, arg) {
			handlerArgs = append(handlerArgs, arg)
		} else {
			middlewareArgs = append(middlewareArgs, arg)
		}
	}

	if len(handlerArgs) == 0 {
		// Fallback: last argument is the route handler (common Fiber convention).
		if len(args) > 1 {
			middlewareArgs = args[:len(args)-1]
		} else {
			middlewareArgs = nil
		}
		handlerArgs = args[len(args)-1:]
	}

	for _, arg := range middlewareArgs {
		if name := exprName(arg); name != "" {
			middlewares = append(middlewares, name)
		}
	}
	for _, arg := range handlerArgs {
		if name := exprName(arg); name != "" {
			handlers = append(handlers, name)
		}
	}
	return handlers, middlewares
}

func isFiberHandlerType(pass *analysis.Pass, expr ast.Expr) bool {
	if pass == nil || pass.TypesInfo == nil || expr == nil {
		return false
	}
	return isHandlerSignature(pass.TypesInfo.TypeOf(expr))
}

func isHandlerSignature(typ types.Type) bool {
	if typ == nil {
		return false
	}
	sig, ok := typ.Underlying().(*types.Signature)
	if !ok {
		return false
	}
	if sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		return false
	}
	paramStr := types.TypeString(sig.Params().At(0).Type(), nil)
	if !strings.Contains(paramStr, "fiber.Ctx") {
		return false
	}
	result := sig.Results().At(0).Type()
	return types.Identical(result, types.Universe.Lookup("error").Type())
}

func isHTTPMethod(name string) bool {
	return httpMethods[name]
}

func isFiberReceiver(pass *analysis.Pass, expr ast.Expr) bool {
	if pass == nil || pass.TypesInfo == nil {
		return identName(expr) == "app"
	}
	recvType := pass.TypesInfo.TypeOf(expr)
	if recvType == nil {
		return false
	}
	return isFiberRouterType(types.TypeString(recvType, nil))
}

func fiberPathToOpenAPI(path string) string {
	if path == "" {
		return path
	}
	path = normalizeRoutePath(path)
	return fiberPathParamRE.ReplaceAllString(path, `{$1}`)
}

func joinPathPrefix(parent, segment string) string {
	segment = normalizeRoutePath(segment)
	if parent == "" {
		return segment
	}
	parent = strings.TrimRight(normalizeRoutePath(parent), "/")
	if segment == "/" {
		return parent
	}
	return parent + segment
}

func normalizeRoutePath(path string) string {
	if path == "" {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func identName(expr ast.Expr) string {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

func exprName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		recv := exprName(e.X)
		if recv != "" {
			return recv + "." + e.Sel.Name
		}
		return e.Sel.Name
	case *ast.CallExpr:
		return exprName(e.Fun)
	default:
		return ""
	}
}

func handlerLookupKey(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		return name[idx+1:]
	}
	return name
}

func titleCaseMethod(method string) string {
	if method == "" {
		return method
	}
	runes := []rune(strings.ToLower(method))
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
