// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"
	"strings"

	"github.com/whisskey/prism/internal/config"
	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/internal/scanner"
	"github.com/whisskey/prism/internal/schema"
	"github.com/whisskey/prism/pkg/openapi"
)

type extractionContext struct {
	file            *parser.ParsedFile
	parser          *parser.GoParser
	prefixStack     []string
	groupVars       map[string]string
	groupMiddleware map[string][]ast.Expr
	varTypes        map[string]string
	middleware      []ast.Expr
	routerParams    map[string]bool
	visited         map[*ast.FuncDecl]bool
	plugin          *Plugin
}

func (ctx *extractionContext) currentPrefix() string {
	return strings.Join(ctx.prefixStack, "")
}

type discoveredRouteBundle struct {
	Route           openapi.Route
	middlewareExprs []ast.Expr
}

func (p *Plugin) discoverRouteBundles(files []scanner.SourceFile) []discoveredRouteBundle {
	p.handlerIndex = make(map[string]*ast.FuncDecl)
	p.routeFuncIndex = make(map[string]*ast.FuncDecl)
	p.returnTypeIndex = make(map[string]string)
	p.usedSchemas = make(map[string]bool)
	p.syntheticSchemas = make(map[string]*openapi.Schema)
	p.requestPathFields = make(map[string]map[string]bool)
	p.structIndex = make(map[string]parser.StructDefinition)
	p.responseHelpers = make(map[*ast.FuncDecl]responseHelper)
	p.globalMiddleware = nil
	p.schemaExtractor = schema.NewGoSchemaExtractor()

	parsedFiles := make(map[string]*parser.ParsedFile)
	p.filePaths = make([]string, 0, len(files))
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
		p.indexGlobalMiddleware(pf)
		structs := p.goParser.ExtractStructs(pf)
		for _, def := range structs {
			p.structIndex[def.Name] = def
			p.schemaExtractor.ExtractFromStruct(def)
		}
		p.extractNamedPrimitiveSchemas(pf)
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
	p.fibergenCatalog = fibergenCatalogFromProgram(p.typeProgram)

	var bundles []discoveredRouteBundle
	for _, file := range files {
		if file.Language != "go" {
			continue
		}

		pf := parsedFiles[file.Path]
		if pf == nil {
			continue
		}

		fileBundles := p.extractRouteBundlesFromParsedFile(file, pf)
		bundles = append(bundles, fileBundles...)
	}

	return bundles
}

func filterRouteBundles(bundles []discoveredRouteBundle, cfg config.RoutesConfig) []discoveredRouteBundle {
	if len(cfg.IncludePrefixes) == 0 && len(cfg.ExcludePaths) == 0 {
		return bundles
	}

	filtered := make([]discoveredRouteBundle, 0, len(bundles))
	for _, bundle := range bundles {
		if cfg.AllowsPath(bundle.Route.Path) {
			filtered = append(filtered, bundle)
		}
	}
	return filtered
}

func (p *Plugin) enrichRouteBundles(bundles []discoveredRouteBundle) {
	for i := range bundles {
		middlewareExprs := append([]ast.Expr{}, bundles[i].middlewareExprs...)
		analysis := p.analyzeRouteBundle(&bundles[i])
		p.applyRouteAnalysis(&bundles[i].Route, analysis, middlewareExprs)
		p.applyFibergenEnrichment(&bundles[i].Route)
		p.applyAnnotationEscapeHatch(&bundles[i].Route, analysis.Handler)
	}
}

func routeBundlesToRoutes(bundles []discoveredRouteBundle) []openapi.Route {
	routes := make([]openapi.Route, len(bundles))
	for i, bundle := range bundles {
		routes[i] = bundle.Route
	}
	return routes
}

func (p *Plugin) extractRouteBundlesFromParsedFile(file scanner.SourceFile, pf *parser.ParsedFile) []discoveredRouteBundle {
	if !p.hasFiberImport(pf) {
		return nil
	}

	var bundles []discoveredRouteBundle
	ctx := &extractionContext{
		file:            pf,
		parser:          p.goParser,
		prefixStack:     []string{},
		groupVars:       make(map[string]string),
		groupMiddleware: make(map[string][]ast.Expr),
		varTypes:        make(map[string]string),
		visited:         make(map[*ast.FuncDecl]bool),
		plugin:          p,
	}

	ast.Inspect(pf.AST, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}
		if len(fiberRouterParams(funcDecl)) > 0 {
			return false
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

	if funcDecl.Body == nil {
		return bundles
	}
	if ctx.visited[funcDecl] {
		return bundles
	}
	ctx.visited[funcDecl] = true
	defer delete(ctx.visited, funcDecl)

	previousRouterParams := ctx.routerParams
	ctx.routerParams = fiberRouterParams(funcDecl)
	previousMiddleware := ctx.middleware
	ctx.middleware = nil
	defer func() {
		ctx.routerParams = previousRouterParams
		ctx.middleware = previousMiddleware
	}()

	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		assignStmt, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		if len(assignStmt.Lhs) != 1 || len(assignStmt.Rhs) != 1 {
			return true
		}

		callExpr, ok := assignStmt.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := assignStmt.Lhs[0].(*ast.Ident); ok {
			if typeName := constructorTypeName(callExpr); typeName != "" {
				ctx.varTypes[ident.Name] = typeName
			}
		}

		selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok || selExpr.Sel.Name != "Group" {
			return true
		}

		ident, ok := assignStmt.Lhs[0].(*ast.Ident)
		if !ok || len(callExpr.Args) < 1 {
			return true
		}

		prefix, ok := parser.ExtractStringLiteral(callExpr.Args[0])
		if !ok {
			return true
		}

		parentPrefix := ""
		if parentIdent, ok := selExpr.X.(*ast.Ident); ok {
			parentPrefix = ctx.groupVars[parentIdent.Name]
		}
		ctx.groupVars[ident.Name] = parentPrefix + prefix
		if len(callExpr.Args) > 1 {
			ctx.groupMiddleware[ident.Name] = append(ctx.groupMiddleware[ident.Name], callExpr.Args[1:]...)
		}
		return true
	})

	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
			methodName := selExpr.Sel.Name

			if methodName == "Route" {
				bundles = append(bundles, p.handleRouteCallbackBundles(callExpr, ctx)...)
				return false
			}
			if methodName == "Use" {
				ctx.middleware = append(ctx.middleware, callExpr.Args...)
				return true
			}

			if httpMethods[methodName] {
				if bundle := p.extractRouteBundleFromCall(callExpr, methodName, selExpr.X, ctx); bundle != nil {
					bundles = append(bundles, *bundle)
				}
				return true
			}
		}

		if callee := ctx.plugin.resolveRouteSetupCallee(callExpr, ctx); callee != nil {
			childCtx := ctx.childForCall(callExpr, callee)
			bundles = append(bundles, ctx.plugin.extractRouteBundlesFromFunc(callee, childCtx)...)
			return false
		}

		return true
	})

	return bundles
}

func (p *Plugin) handleRouteCallbackBundles(callExpr *ast.CallExpr, ctx *extractionContext) []discoveredRouteBundle {
	var bundles []discoveredRouteBundle

	if len(callExpr.Args) < 2 {
		return bundles
	}

	prefix, ok := parser.ExtractStringLiteral(callExpr.Args[0])
	if !ok {
		return bundles
	}

	objectPrefix := ""
	if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
		if ident, ok := selExpr.X.(*ast.Ident); ok {
			objectPrefix = ctx.groupVars[ident.Name]
		}
	}

	fullPrefix := objectPrefix + prefix
	funcLit, ok := callExpr.Args[1].(*ast.FuncLit)
	if !ok {
		return bundles
	}

	routerParamName := ""
	if funcLit.Type.Params != nil && len(funcLit.Type.Params.List) > 0 {
		if len(funcLit.Type.Params.List[0].Names) > 0 {
			routerParamName = funcLit.Type.Params.List[0].Names[0].Name
		}
	}

	ctx.prefixStack = append(ctx.prefixStack, fullPrefix)
	if routerParamName != "" {
		ctx.groupVars[routerParamName] = fullPrefix
	}

	ast.Inspect(funcLit.Body, func(n ast.Node) bool {
		nestedCall, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selExpr, ok := nestedCall.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if httpMethods[selExpr.Sel.Name] {
			if bundle := p.extractRouteBundleFromCall(nestedCall, selExpr.Sel.Name, selExpr.X, ctx); bundle != nil {
				bundles = append(bundles, *bundle)
			}
		}
		return true
	})

	ctx.prefixStack = ctx.prefixStack[:len(ctx.prefixStack)-1]
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
	if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, ":") {
		return nil
	}

	prefix := ""
	var groupMiddleware []ast.Expr
	if ident, ok := receiver.(*ast.Ident); ok {
		prefix = ctx.groupVars[ident.Name]
		if prefix == "" && ctx.currentPrefix() == "" && ctx.routerParams[ident.Name] {
			return nil
		}
		groupMiddleware = ctx.groupMiddleware[ident.Name]
	}
	if prefix == "" {
		prefix = ctx.currentPrefix()
	}

	fullPath := normalizePath(convertFiberPathParams(prefix + path))
	sourceLine := ctx.file.FileSet.Position(callExpr.Pos()).Line

	routeArgs := callExpr.Args[1:]
	var handlerExpr ast.Expr
	if len(routeArgs) > 0 {
		handlerIdx := len(routeArgs) - 1
		if len(routeArgs) > 1 {
			if idx := p.lastHandlerArgIndex(ctx.file, routeArgs); idx >= 0 {
				handlerIdx = idx
			}
		}
		handlerExpr = routeArgs[handlerIdx]
	}

	handlerRef := p.buildHandlerRef(handlerExpr, ctx.file.Path, sourceLine)
	middlewareArgs := make([]ast.Expr, 0, len(ctx.middleware)+len(routeArgs)+len(groupMiddleware))
	middlewareArgs = append(middlewareArgs, ctx.middleware...)
	middlewareArgs = append(middlewareArgs, groupMiddleware...)
	if len(routeArgs) > 0 {
		middlewareArgs = append(middlewareArgs, p.splitMiddlewareArgs(routeArgs, ctx)...)
	}

	httpMethod := strings.ToUpper(method)
	if method == "All" {
		httpMethod = "ALL"
	}

	route := openapi.Route{
		Method:              httpMethod,
		Path:                fullPath,
		Handler:             handlerRef.Qualified,
		HandlerRef:          handlerRef,
		Middleware:          buildMiddlewareRefs(middlewareArgs, ctx.file),
		AppVar:              routerVarName(receiver),
		Parameters:          extractPathParams(fullPath),
		OperationID:         generateOperationID(httpMethod, fullPath, handlerRef.Qualified),
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

func (p *Plugin) buildHandlerRef(expr ast.Expr, sourceFile string, sourceLine int) openapi.HandlerRef {
	ref := openapi.HandlerRef{
		SourceFile: sourceFile,
		SourceLine: sourceLine,
	}

	switch e := expr.(type) {
	case *ast.Ident:
		ref.Method = e.Name
		ref.Qualified = e.Name
	case *ast.SelectorExpr:
		ref.Method = e.Sel.Name
		ref.Qualified = selectorQualifiedName(e)
		ref.Receiver = selectorReceiverName(e)
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

	ref.Resolved = p.resolveHandler(ref.Qualified) != nil
	return ref
}

func buildMiddlewareRefs(exprs []ast.Expr, pf *parser.ParsedFile) []openapi.MiddlewareRef {
	if len(exprs) == 0 {
		return nil
	}

	refs := make([]openapi.MiddlewareRef, 0, len(exprs))
	for _, expr := range exprs {
		ref := openapi.MiddlewareRef{Name: middlewareNameFromExpr(expr)}
		if callExpr, ok := expr.(*ast.CallExpr); ok && pf != nil {
			ref.SourceLine = pf.FileSet.Position(callExpr.Pos()).Line
			ref.SourceFile = pf.Path
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
	}

	callExpr, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}

	switch fn := callExpr.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return selectorQualifiedName(fn)
	case *ast.IndexExpr:
		baseName := middlewareNameFromExpr(&ast.CallExpr{Fun: fn.X})
		if baseName == "" {
			return ""
		}
		if indexType := astTypeString(fn.Index); indexType != "" {
			return baseName + "[" + cleanTypeName(indexType) + "]"
		}
		return baseName
	case *ast.IndexListExpr:
		baseName := middlewareNameFromExpr(&ast.CallExpr{Fun: fn.X})
		if baseName == "" || len(fn.Indices) == 0 {
			return baseName
		}
		if indexType := astTypeString(fn.Indices[0]); indexType != "" {
			return baseName + "[" + cleanTypeName(indexType) + "]"
		}
		return baseName
	}

	return ""
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

func selectorReceiverName(expr *ast.SelectorExpr) string {
	switch x := expr.X.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	default:
		return ""
	}
}

func routerVarName(receiver ast.Expr) string {
	ident, ok := receiver.(*ast.Ident)
	if !ok {
		return ""
	}
	return ident.Name
}

func discoveryConfidence(ref openapi.HandlerRef) string {
	if ref.UnresolvedReason != "" || !ref.Resolved {
		return "low"
	}
	return "high"
}
