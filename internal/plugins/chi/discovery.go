// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package chi

import (
	"go/ast"
	"strings"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/internal/scanner"
	"github.com/whisskey/prism/internal/schema"
	"github.com/whisskey/prism/pkg/openapi"
)

type discoveredRouteBundle struct {
	Route           openapi.Route
	middlewareExprs []ast.Expr
}

type extractionContext struct {
	file        *parser.ParsedFile
	parser      *parser.GoParser
	prefixStack []string
	middleware  []ast.Expr
	visited     map[*ast.FuncDecl]bool
	plugin      *Plugin
}

func (ctx *extractionContext) currentPrefix() string {
	return strings.Join(ctx.prefixStack, "")
}

func (p *Plugin) discoverRouteBundles(files []scanner.SourceFile) []discoveredRouteBundle {
	p.handlerIndex = make(map[string]*ast.FuncDecl)
	p.routeFuncIndex = make(map[string]*ast.FuncDecl)
	p.returnTypeIndex = make(map[string]string)
	p.usedSchemas = make(map[string]bool)
	p.requestPathFields = make(map[string]map[string]bool)
	p.structIndex = make(map[string]parser.StructDefinition)
	p.responseHelpers = make(map[*ast.FuncDecl]responseHelper)
	p.schemaExtractor = schema.NewGoSchemaExtractor()
	p.filePaths = make([]string, 0, len(files))

	parsedFiles := make(map[string]*parser.ParsedFile)
	for _, file := range files {
		if file.Language != "go" {
			continue
		}
		pf, err := p.goParser.ParseSource(file.Path, string(file.Content))
		if err != nil {
			continue
		}
		parsedFiles[file.Path] = pf
		p.filePaths = append(p.filePaths, file.Path)
		p.indexFunctions(pf)
		for _, def := range p.goParser.ExtractStructs(pf) {
			p.structIndex[def.Name] = def
			p.schemaExtractor.ExtractFromStruct(def)
		}
	}

	p.handlerFileIndex = make(map[*ast.FuncDecl]*parser.ParsedFile)
	for path, pf := range parsedFiles {
		for _, decl := range pf.AST.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				p.handlerFileIndex[fn] = parsedFiles[path]
			}
		}
	}
	p.parsedFileIndex = parsedFiles
	p.loadTypeProgram(p.filePaths)

	var bundles []discoveredRouteBundle
	for _, file := range files {
		if file.Language != "go" {
			continue
		}
		pf := parsedFiles[file.Path]
		if pf == nil || !p.hasChiImport(pf) {
			continue
		}
		bundles = append(bundles, p.extractRouteBundlesFromParsedFile(file, pf)...)
	}
	return bundles
}

func (p *Plugin) indexFunctions(pf *parser.ParsedFile) {
	if pf == nil || pf.AST == nil {
		return
	}
	for _, decl := range pf.AST.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok {
			p.indexInterfaceReturns(genDecl)
			continue
		}
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		name := fn.Name.Name
		p.routeFuncIndex[name] = fn
		p.indexFunctionReturn(name, fn)
		if isChiHTTPHandler(fn) {
			p.handlerIndex[name] = fn
		}
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			recvType := cleanTypeName(astTypeString(fn.Recv.List[0].Type))
			if recvType != "" {
				qualified := recvType + "." + name
				p.routeFuncIndex[qualified] = fn
				p.indexFunctionReturn(qualified, fn)
				if isChiHTTPHandler(fn) {
					p.handlerIndex[qualified] = fn
				}
			}
		}
	}
}

func (p *Plugin) indexFunctionReturn(name string, funcDecl *ast.FuncDecl) {
	if resolved := p.resolveMethodReturnType(funcDecl); cleanTypeName(resolved) != "" {
		p.returnTypeIndex[name] = resolved
	}
}

func (p *Plugin) indexInterfaceReturns(genDecl *ast.GenDecl) {
	for _, spec := range genDecl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		iface, ok := typeSpec.Type.(*ast.InterfaceType)
		if !ok || iface.Methods == nil {
			continue
		}
		for _, method := range iface.Methods.List {
			if len(method.Names) == 0 {
				continue
			}
			fnType, ok := method.Type.(*ast.FuncType)
			if !ok || fnType.Results == nil || len(fnType.Results.List) == 0 {
				continue
			}
			returnType := astTypeString(fnType.Results.List[0].Type)
			if cleanTypeName(returnType) == "" {
				continue
			}
			p.returnTypeIndex[method.Names[0].Name] = returnType
		}
	}
}

func isChiHTTPHandler(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Type == nil || fn.Type.Params == nil {
		return false
	}
	params := fn.Type.Params.List
	if len(params) < 2 {
		return false
	}
	// Prefer classic net/http signature: (ResponseWriter, *Request)
	first := astTypeString(params[0].Type)
	second := astTypeString(params[1].Type)
	return strings.Contains(first, "ResponseWriter") && strings.Contains(second, "Request")
}

func (p *Plugin) extractRouteBundlesFromParsedFile(file scanner.SourceFile, pf *parser.ParsedFile) []discoveredRouteBundle {
	var bundles []discoveredRouteBundle
	ctx := &extractionContext{
		file:        pf,
		parser:      p.goParser,
		prefixStack: []string{},
		visited:     make(map[*ast.FuncDecl]bool),
		plugin:      p,
	}

	ast.Inspect(pf.AST, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}
		bundles = append(bundles, p.extractRouteBundlesFromFunc(funcDecl, ctx)...)
		return true
	})

	for i := range bundles {
		bundles[i].Route.SourceFile = file.Path
	}
	return bundles
}

func (p *Plugin) extractRouteBundlesFromFunc(funcDecl *ast.FuncDecl, ctx *extractionContext) []discoveredRouteBundle {
	var bundles []discoveredRouteBundle
	if funcDecl.Body == nil || ctx.visited[funcDecl] {
		return bundles
	}
	ctx.visited[funcDecl] = true
	defer delete(ctx.visited, funcDecl)

	previousMiddleware := ctx.middleware
	ctx.middleware = append([]ast.Expr(nil), previousMiddleware...)
	defer func() { ctx.middleware = previousMiddleware }()

	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		methodName := selExpr.Sel.Name

		switch methodName {
		case "Use":
			ctx.middleware = append(ctx.middleware, callExpr.Args...)
			return false
		case "Route", "Group":
			bundles = append(bundles, p.handleRouteGroupBundles(callExpr, methodName, ctx)...)
			return false
		case "With":
			// Handled when chained: r.With(...).Get(...)
			return true
		}

		if httpMethods[methodName] {
			if bundle := p.extractRouteBundleFromCall(callExpr, methodName, selExpr.X, ctx); bundle != nil {
				bundles = append(bundles, *bundle)
			}
			return false
		}
		return true
	})

	return bundles
}

func (p *Plugin) handleRouteGroupBundles(callExpr *ast.CallExpr, methodName string, ctx *extractionContext) []discoveredRouteBundle {
	var bundles []discoveredRouteBundle
	var prefix string
	funcLitIdx := 0
	if methodName == "Route" {
		if len(callExpr.Args) < 2 {
			return bundles
		}
		if path, ok := parser.ExtractStringLiteral(callExpr.Args[0]); ok {
			prefix = path
		}
		funcLitIdx = 1
	} else if len(callExpr.Args) < 1 {
		return bundles
	}

	funcLit, ok := callExpr.Args[funcLitIdx].(*ast.FuncLit)
	if !ok {
		return bundles
	}

	ctx.prefixStack = append(ctx.prefixStack, prefix)
	previousMiddleware := ctx.middleware
	ctx.middleware = append([]ast.Expr(nil), previousMiddleware...)
	defer func() {
		ctx.prefixStack = ctx.prefixStack[:len(ctx.prefixStack)-1]
		ctx.middleware = previousMiddleware
	}()

	ast.Inspect(funcLit.Body, func(n ast.Node) bool {
		nestedCall, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selExpr, ok := nestedCall.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		nestedMethod := selExpr.Sel.Name
		switch nestedMethod {
		case "Use":
			ctx.middleware = append(ctx.middleware, nestedCall.Args...)
			return false
		case "Route", "Group":
			bundles = append(bundles, p.handleRouteGroupBundles(nestedCall, nestedMethod, ctx)...)
			return false
		}
		if httpMethods[nestedMethod] {
			if bundle := p.extractRouteBundleFromCall(nestedCall, nestedMethod, selExpr.X, ctx); bundle != nil {
				bundles = append(bundles, *bundle)
			}
			return false
		}
		return true
	})

	return bundles
}

func (p *Plugin) extractRouteBundleFromCall(callExpr *ast.CallExpr, method string, receiver ast.Expr, ctx *extractionContext) *discoveredRouteBundle {
	if len(callExpr.Args) < 1 {
		return nil
	}
	path, ok := parser.ExtractStringLiteral(callExpr.Args[0])
	if !ok {
		return nil
	}
	// Chi route paths always start with "/"; ignore Header.Get / Query().Get etc.
	if !strings.HasPrefix(path, "/") {
		return nil
	}

	fullPath := normalizePath(ctx.currentPrefix() + path)
	sourceLine := ctx.file.FileSet.Position(callExpr.Pos()).Line

	routeArgs := callExpr.Args[1:]
	middlewareArgs := append([]ast.Expr(nil), ctx.middleware...)
	middlewareArgs = append(middlewareArgs, withMiddlewareFromReceiver(receiver)...)

	var handlerExpr ast.Expr
	if len(routeArgs) > 0 {
		handlerExpr = routeArgs[len(routeArgs)-1]
		if len(routeArgs) > 1 {
			middlewareArgs = append(middlewareArgs, routeArgs[:len(routeArgs)-1]...)
		}
	}

	handlerRef := p.buildHandlerRef(handlerExpr, ctx.file.Path, sourceLine)
	route := openapi.Route{
		Method:              strings.ToUpper(method),
		Path:                fullPath,
		Handler:             handlerRef.Qualified,
		HandlerRef:          handlerRef,
		Middleware:          buildMiddlewareRefs(middlewareArgs, ctx.file),
		Parameters:          extractPathParams(fullPath),
		OperationID:         generateOperationID(method, fullPath, handlerRef.Qualified),
		Tags:                inferTags(fullPath),
		SourceFile:          ctx.file.Path,
		SourceLine:          sourceLine,
		DiscoveryConfidence: discoveryConfidence(handlerRef),
	}

	return &discoveredRouteBundle{
		Route:           route,
		middlewareExprs: middlewareArgs,
	}
}

func withMiddlewareFromReceiver(receiver ast.Expr) []ast.Expr {
	call, ok := receiver.(*ast.CallExpr)
	if !ok {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "With" {
		return nil
	}
	return call.Args
}

func (p *Plugin) buildHandlerRef(expr ast.Expr, sourceFile string, sourceLine int) openapi.HandlerRef {
	ref := openapi.HandlerRef{
		SourceFile: sourceFile,
		SourceLine: sourceLine,
	}
	if expr == nil {
		ref.UnresolvedReason = "missing-handler"
		return ref
	}

	switch e := expr.(type) {
	case *ast.Ident:
		ref.Method = e.Name
		ref.Qualified = e.Name
	case *ast.SelectorExpr:
		ref.Method = e.Sel.Name
		ref.Qualified = selectorQualifiedName(e)
		if ident, ok := e.X.(*ast.Ident); ok {
			ref.Receiver = ident.Name
		}
	case *ast.FuncLit:
		ref.Qualified = "<anonymous>"
		ref.UnresolvedReason = "anonymous-handler"
	case *ast.CallExpr:
		ref.Qualified = "<wrapped>"
		ref.UnresolvedReason = "wrapped-handler"
	default:
		ref.Qualified = p.extractHandlerName(expr)
	}

	if ref.Qualified == "" {
		ref.UnresolvedReason = "missing-handler"
	}
	if ref.UnresolvedReason == "" {
		ref.Resolved = p.handlerIndex[ref.Method] != nil || p.routeFuncIndex[ref.Method] != nil ||
			p.handlerIndex[ref.Qualified] != nil || p.routeFuncIndex[ref.Qualified] != nil
		if !ref.Resolved {
			if idx := strings.LastIndex(ref.Qualified, "."); idx >= 0 {
				short := ref.Qualified[idx+1:]
				ref.Resolved = p.handlerIndex[short] != nil || p.routeFuncIndex[short] != nil
			}
		}
	}
	return ref
}

func buildMiddlewareRefs(exprs []ast.Expr, pf *parser.ParsedFile) []openapi.MiddlewareRef {
	if len(exprs) == 0 {
		return nil
	}
	refs := make([]openapi.MiddlewareRef, 0, len(exprs))
	for _, expr := range exprs {
		name := middlewareNameFromExpr(expr)
		if name == "" {
			continue
		}
		ref := openapi.MiddlewareRef{Name: name}
		if pf != nil {
			ref.SourceFile = pf.Path
			if call, ok := expr.(*ast.CallExpr); ok {
				ref.SourceLine = pf.FileSet.Position(call.Pos()).Line
			}
		}
		refs = append(refs, ref)
	}
	return refs
}

func middlewareNameFromExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return selectorQualifiedName(e)
	case *ast.CallExpr:
		return middlewareNameFromExpr(e.Fun)
	default:
		return ""
	}
}

func selectorQualifiedName(expr *ast.SelectorExpr) string {
	if ident, ok := expr.X.(*ast.Ident); ok {
		return ident.Name + "." + expr.Sel.Name
	}
	if sel, ok := expr.X.(*ast.SelectorExpr); ok {
		return selectorQualifiedName(sel) + "." + expr.Sel.Name
	}
	return expr.Sel.Name
}

func discoveryConfidence(ref openapi.HandlerRef) string {
	if ref.UnresolvedReason != "" || !ref.Resolved {
		return "low"
	}
	return "high"
}

func routeBundlesToRoutes(bundles []discoveredRouteBundle) []openapi.Route {
	routes := make([]openapi.Route, len(bundles))
	for i, bundle := range bundles {
		routes[i] = bundle.Route
	}
	return routes
}

func (p *Plugin) enrichRouteBundles(bundles []discoveredRouteBundle) {
	for i := range bundles {
		analysis := p.analyzeRouteBundle(&bundles[i])
		p.applyRouteAnalysis(&bundles[i].Route, analysis, bundles[i].middlewareExprs)
		p.applyAnnotationEscapeHatch(&bundles[i].Route, analysis.Handler)
	}
}

func astTypeString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + astTypeString(e.X)
	case *ast.SelectorExpr:
		return astTypeString(e.X) + "." + e.Sel.Name
	default:
		return ""
	}
}
