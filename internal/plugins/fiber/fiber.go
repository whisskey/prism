// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

// Package fiber provides a plugin for extracting routes from Fiber framework applications.
package fiber

import (
	"bufio"
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/whisskey/prism/internal/analyzer/fibergen"
	"github.com/whisskey/prism/internal/config"
	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/internal/plugins"
	"github.com/whisskey/prism/internal/scanner"
	"github.com/whisskey/prism/internal/schema"
	"github.com/whisskey/prism/internal/typecheck"
	"github.com/whisskey/prism/pkg/openapi"
)

// fiberImportPaths are the known import paths for Fiber framework.
var fiberImportPaths = []string{
	"github.com/gofiber/fiber",
	"github.com/gofiber/fiber/v2",
	"github.com/gofiber/fiber/v3",
}

// httpMethods are the HTTP methods supported by Fiber.
var httpMethods = map[string]bool{
	"Get":     true,
	"Post":    true,
	"Put":     true,
	"Delete":  true,
	"Patch":   true,
	"Head":    true,
	"Options": true,
	"Trace":   true,
	"Connect": true,
	"All":     true,
}

// Plugin implements the FrameworkPlugin interface for Fiber framework.
type Plugin struct {
	goParser          *parser.GoParser
	schemaExtractor   *schema.GoSchemaExtractor
	handlerIndex      map[string]*ast.FuncDecl
	routeFuncIndex    map[string]*ast.FuncDecl
	returnTypeIndex   map[string]string
	usedSchemas       map[string]bool
	syntheticSchemas  map[string]*openapi.Schema
	requestPathFields map[string]map[string]bool
	structIndex       map[string]parser.StructDefinition
	responseHelpers   map[*ast.FuncDecl]responseHelper
	inference         config.InferenceConfig
	routes            config.RoutesConfig
	globalMiddleware  []ast.Expr
	typeProgram       *typecheck.Program
	fibergenCatalog   *fibergen.Catalog
	parsedFileIndex   map[string]*parser.ParsedFile
	handlerFileIndex  map[*ast.FuncDecl]*parser.ParsedFile
	filePaths         []string
}

// New creates a new Fiber plugin instance.
func New() *Plugin {
	return &Plugin{
		goParser:        parser.NewGoParser(),
		schemaExtractor: schema.NewGoSchemaExtractor(),
	}
}

// Name returns the plugin identifier.
func (p *Plugin) Name() string {
	return "fiber"
}

// Extensions returns the file extensions this plugin handles.
func (p *Plugin) Extensions() []string {
	return []string{".go"}
}

// Info returns plugin metadata.
func (p *Plugin) Info() plugins.PluginInfo {
	return plugins.PluginInfo{
		Name:        "fiber",
		Version:     "1.0.0",
		Description: "Extracts routes from Fiber framework applications",
		SupportedFrameworks: []string{
			"github.com/gofiber/fiber/v2",
			"github.com/gofiber/fiber/v3",
		},
	}
}

// Configure applies optional static-analysis inference overrides.
func (p *Plugin) Configure(cfg *config.Config) {
	if cfg == nil {
		return
	}
	p.inference = cfg.Inference
	p.routes = cfg.Routes
}

// Detect checks if Fiber is used in the project by looking at go.mod.
func (p *Plugin) Detect(projectRoot string) (bool, error) {
	goModPath := filepath.Join(projectRoot, "go.mod")

	file, err := os.Open(goModPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to open go.mod: %w", err)
	}
	defer func() { _ = file.Close() }()

	scanr := bufio.NewScanner(file)
	for scanr.Scan() {
		line := scanr.Text()
		for _, importPath := range fiberImportPaths {
			if strings.Contains(line, importPath) {
				return true, nil
			}
		}
	}

	if err := scanr.Err(); err != nil {
		return false, fmt.Errorf("failed to read go.mod: %w", err)
	}

	return false, nil
}

// ExtractRoutes parses source files and extracts Fiber route definitions.
func (p *Plugin) ExtractRoutes(files []scanner.SourceFile) ([]openapi.Route, error) {
	bundles := p.discoverRouteBundles(files)
	bundles = p.supplementFibergenRoutes(bundles)
	bundles = filterRouteBundles(bundles, p.routes)
	p.enrichRouteBundles(bundles)

	return routeBundlesToRoutes(bundles), nil
}

// extractHandlerName extracts the handler function name from an expression.
func (p *Plugin) extractHandlerName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		// e.g., handlers.GetUser
		if ident, ok := e.X.(*ast.Ident); ok {
			return ident.Name + "." + e.Sel.Name
		}
		return e.Sel.Name
	case *ast.FuncLit:
		return "<anonymous>"
	case *ast.CallExpr:
		// e.g., middleware(handler)
		return "<wrapped>"
	default:
		return ""
	}
}

// indexFunctions records route setup and handler functions so routes can be
// enriched after the initial route call is found.
func (p *Plugin) indexFunctions(pf *parser.ParsedFile) {
	for _, decl := range pf.AST.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok {
			p.indexInterfaceReturns(genDecl)
			continue
		}

		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		name := funcDecl.Name.Name
		p.routeFuncIndex[name] = funcDecl
		p.indexFunctionReturn(name, funcDecl)
		if isFiberHandler(funcDecl) {
			p.handlerIndex[name] = funcDecl
		}

		if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
			recvType := cleanTypeName(astTypeString(funcDecl.Recv.List[0].Type))
			if recvType != "" {
				p.routeFuncIndex[recvType+"."+name] = funcDecl
				p.indexFunctionReturn(recvType+"."+name, funcDecl)
				if isFiberHandler(funcDecl) {
					p.handlerIndex[recvType+"."+name] = funcDecl
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

func (p *Plugin) forwardedReturnType(funcDecl *ast.FuncDecl) string {
	return p.resolveMethodReturnType(funcDecl)
}

func isFiberHandler(funcDecl *ast.FuncDecl) bool {
	if funcDecl.Type.Params == nil {
		return false
	}
	for _, field := range funcDecl.Type.Params.List {
		if strings.Contains(astTypeString(field.Type), "fiber.Ctx") {
			return true
		}
	}
	return false
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

func (p *Plugin) resolveRouteSetupFunc(callExpr *ast.CallExpr, ctx *extractionContext) *ast.FuncDecl {
	switch fn := callExpr.Fun.(type) {
	case *ast.Ident:
		return p.routeFuncIndex[fn.Name]
	case *ast.SelectorExpr:
		name := fn.Sel.Name
		if ident, ok := fn.X.(*ast.Ident); ok {
			if typeName := ctx.varTypes[ident.Name]; typeName != "" {
				if typed := p.routeFuncIndex[typeName+"."+name]; typed != nil {
					return typed
				}
			}
			if typed := p.routeFuncIndex[ident.Name+"."+name]; typed != nil {
				return typed
			}
		}
		return p.routeFuncIndex[name]
	default:
		return nil
	}
}

func (ctx *extractionContext) childForCall(callExpr *ast.CallExpr, callee *ast.FuncDecl) *extractionContext {
	groupVars := make(map[string]string, len(ctx.groupVars))
	for k, v := range ctx.groupVars {
		groupVars[k] = v
	}
	groupMiddleware := make(map[string][]ast.Expr, len(ctx.groupMiddleware))
	for k, v := range ctx.groupMiddleware {
		groupMiddleware[k] = append([]ast.Expr(nil), v...)
	}
	varTypes := make(map[string]string, len(ctx.varTypes))
	for k, v := range ctx.varTypes {
		varTypes[k] = v
	}

	if callee.Type.Params != nil {
		for i, field := range callee.Type.Params.List {
			if i >= len(callExpr.Args) {
				break
			}
			prefix := ctx.prefixForExpr(callExpr.Args[i])
			if prefix == "" {
				continue
			}
			for _, name := range field.Names {
				groupVars[name.Name] = prefix
			}
		}
	}

	return &extractionContext{
		file:            ctx.file,
		parser:          ctx.parser,
		prefixStack:     append([]string{}, ctx.prefixStack...),
		groupVars:       groupVars,
		groupMiddleware: groupMiddleware,
		varTypes:        varTypes,
		middleware:      append([]ast.Expr{}, ctx.middleware...),
		routerParams:    make(map[string]bool),
		visited:         ctx.visited,
		plugin:          ctx.plugin,
	}
}

func constructorTypeName(callExpr *ast.CallExpr) string {
	ident, ok := callExpr.Fun.(*ast.Ident)
	if !ok || !strings.HasPrefix(ident.Name, "New") || len(ident.Name) <= len("New") {
		return ""
	}
	return ident.Name[len("New"):]
}

func (ctx *extractionContext) prefixForExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return ctx.groupVars[e.Name]
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Group" && len(e.Args) > 0 {
			prefix, ok := parser.ExtractStringLiteral(e.Args[0])
			if !ok {
				return ""
			}
			return ctx.prefixForExpr(sel.X) + prefix
		}
	}
	return ""
}

func fiberRouterParams(funcDecl *ast.FuncDecl) map[string]bool {
	params := make(map[string]bool)
	if funcDecl.Type.Params == nil {
		return params
	}
	for _, field := range funcDecl.Type.Params.List {
		typeName := astTypeString(field.Type)
		if !strings.Contains(typeName, "fiber.Router") {
			continue
		}
		for _, name := range field.Names {
			params[name.Name] = true
		}
	}
	return params
}

func isRouteSetupFunc(funcDecl *ast.FuncDecl) bool {
	if funcDecl == nil {
		return false
	}
	if len(fiberRouterParams(funcDecl)) > 0 {
		return true
	}
	if funcDecl.Type.Params != nil {
		for _, field := range funcDecl.Type.Params.List {
			if strings.Contains(astTypeString(field.Type), "fiber.App") {
				return true
			}
		}
	}
	name := funcDecl.Name.Name
	return name == "SetupRoutes" || strings.HasSuffix(name, "Routes") || strings.HasPrefix(name, "Register")
}

func (p *Plugin) resolveRouteSetupCallee(callExpr *ast.CallExpr, ctx *extractionContext) *ast.FuncDecl {
	callee := p.resolveRouteSetupFunc(callExpr, ctx)
	if !isRouteSetupFunc(callee) {
		return nil
	}
	return callee
}

func appendQueryParameters(params []openapi.Parameter, queryParams map[string]*openapi.Schema, tags []string) []openapi.Parameter {
	if len(queryParams) == 0 {
		return params
	}
	existing := make(map[string]bool, len(params))
	for _, param := range params {
		existing[param.Name+"."+param.In] = true
	}
	for name, schemaType := range queryParams {
		if existing[name+".query"] {
			continue
		}
		param := openapi.Parameter{
			Name:     name,
			In:       "query",
			Schema:   schemaType,
			Required: false,
		}
		enrichQueryParameter(&param)
		enrichParameter(&param, tags)
		params = append(params, param)
	}
	return params
}

func (p *Plugin) resolveHandlerRef(ref openapi.HandlerRef) *ast.FuncDecl {
	if ref.Qualified != "" {
		if fn := p.handlerIndex[ref.Qualified]; fn != nil {
			return fn
		}
		if fn := p.routeFuncIndex[ref.Qualified]; fn != nil && isFiberHandler(fn) {
			return fn
		}
	}

	if ref.Method != "" {
		candidates := []string{ref.Method}
		if ref.Receiver != "" {
			candidates = append([]string{
				ref.Receiver + "." + ref.Method,
				strings.TrimPrefix(ref.Receiver, "*") + "." + ref.Method,
			}, candidates...)
		}
		for _, key := range candidates {
			if fn := p.handlerIndex[key]; fn != nil {
				return fn
			}
		}
	}

	return p.resolveHandler(ref.Qualified)
}

func (p *Plugin) resolveHandler(handler string) *ast.FuncDecl {
	if handler == "" || handler == "<anonymous>" || handler == "<wrapped>" {
		return nil
	}
	if fn := p.handlerIndex[handler]; fn != nil {
		return fn
	}
	parts := strings.Split(handler, ".")
	if len(parts) > 0 {
		return p.handlerIndex[parts[len(parts)-1]]
	}
	return nil
}

func statusCodeFromExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return strings.Trim(e.Value, `"`)
	case *ast.SelectorExpr:
		switch e.Sel.Name {
		case "StatusOK":
			return "200"
		case "StatusCreated":
			return "201"
		case "StatusNoContent":
			return "204"
		case "StatusBadRequest":
			return "400"
		case "StatusUnauthorized":
			return "401"
		case "StatusForbidden":
			return "403"
		case "StatusNotFound":
			return "404"
		case "StatusConflict":
			return "409"
		case "StatusTeapot":
			return "418"
		case "StatusUnprocessableEntity":
			return "422"
		case "StatusTooManyRequests":
			return "429"
		case "StatusInternalServerError":
			return "500"
		case "StatusServiceUnavailable":
			return "503"
		case "StatusGatewayTimeout":
			return "504"
		}
	}
	return "200"
}

func astTypeString(expr ast.Expr) string {
	switch e := expr.(type) {
	case nil:
		return ""
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + astTypeString(e.X)
	case *ast.SelectorExpr:
		return astTypeString(e.X) + "." + e.Sel.Name
	case *ast.ArrayType:
		return "[]" + astTypeString(e.Elt)
	case *ast.MapType:
		return "map[" + astTypeString(e.Key) + "]" + astTypeString(e.Value)
	case *ast.InterfaceType:
		return "interface{}"
	}
	return ""
}

func cleanTypeName(typeName string) string {
	for {
		original := typeName
		typeName = strings.TrimPrefix(typeName, "[]")
		typeName = strings.TrimPrefix(typeName, "*")
		if typeName == original {
			break
		}
	}
	if idx := strings.LastIndex(typeName, "."); idx >= 0 {
		typeName = typeName[idx+1:]
	}
	if strings.Contains(typeName, "[") || strings.Contains(typeName, "]") {
		return ""
	}
	if strings.Contains(typeName, ",") || strings.HasPrefix(strings.TrimSpace(typeName), "(") {
		return ""
	}
	switch typeName {
	case "", "any", "string", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64", "bool", "byte", "rune", "error", "interface{}":
		return ""
	}
	return typeName
}

func selectorName(expr ast.Expr) string {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

func schemaRef(schemaName string) *openapi.Schema {
	if isUntypedSchemaName(schemaName) {
		return &openapi.Schema{}
	}
	return &openapi.Schema{Ref: "#/components/schemas/" + schemaName}
}

func isUntypedSchemaName(name string) bool {
	switch name {
	case "", "any", "interface{}":
		return true
	default:
		return false
	}
}

func schemaRefWithExample(schemaName string, example interface{}) *openapi.Schema {
	return &openapi.Schema{
		Ref:     "#/components/schemas/" + schemaName,
		Example: example,
	}
}

func firstSentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if idx := strings.Index(text, "\n"); idx >= 0 {
		return strings.TrimSpace(text[:idx])
	}
	return text
}

func summaryFromHandler(handler string, tags []string) string {
	if handler == "" || handler == "<anonymous>" || handler == "<wrapped>" {
		return ""
	}
	parts := strings.Split(handler, ".")
	name := parts[len(parts)-1]
	words := splitIdentifier(name)
	if len(words) == 0 {
		return ""
	}
	subject := ""
	if len(tags) > 0 {
		subject = tags[0]
	}
	if subject != "" && !containsWord(words, subject) {
		words = append(words, subject)
	}
	for i, word := range words {
		if strings.EqualFold(word, "ID") {
			words[i] = "ID"
			continue
		}
		words[i] = strings.ToLower(word)
	}
	if len(words) >= 4 && words[0] == "get" && words[1] == "by" && words[2] == "ID" && subject != "" {
		return "Get " + strings.ToLower(subject) + " by ID"
	}
	words[0] = strings.ToUpper(words[0][:1]) + words[0][1:]
	return strings.Join(words, " ")
}

func requestBodyDescription(method string, tags []string) string {
	subject := routeSubject(tags)
	if subject == "" {
		return "Request body"
	}
	switch method {
	case "POST":
		return "Create " + subject + " request body"
	case "PUT", "PATCH":
		return "Update " + subject + " request body"
	default:
		return strings.ToUpper(method[:1]) + strings.ToLower(method[1:]) + " " + subject + " request body"
	}
}

func enrichParameter(param *openapi.Parameter, tags []string) {
	if param == nil {
		return
	}
	if param.Description == "" {
		param.Description = parameterDescription(param.Name, tags)
	}
	if param.Example == nil && param.Schema != nil {
		param.Example = parameterExample(param.Name, param.Schema)
	}
}

func parameterDescription(name string, tags []string) string {
	subject := routeSubject(tags)
	if subject == "" {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	if strings.EqualFold(name, "id") {
		return strings.ToUpper(subject[:1]) + subject[1:] + " ID"
	}
	return strings.ToUpper(subject[:1]) + subject[1:] + " " + name
}

func parameterExample(name string, schemaValue *openapi.Schema) interface{} {
	switch schemaValue.Type {
	case "integer":
		return uint(1)
	case "number":
		return 1
	case "boolean":
		return true
	case "string":
		if strings.EqualFold(name, "id") {
			return "example-id"
		}
		return "example"
	default:
		return nil
	}
}

func routeSubject(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return strings.ToLower(tags[0])
}

func splitIdentifier(name string) []string {
	runes := []rune(name)
	var words []string
	var current strings.Builder
	for i, r := range runes {
		if i > 0 && current.Len() > 0 {
			prev := runes[i-1]
			var next rune
			if i+1 < len(runes) {
				next = runes[i+1]
			}
			if isIdentifierBoundary(prev, r, next) {
				words = append(words, current.String())
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

func isIdentifierBoundary(prev, current, next rune) bool {
	if isLowerASCII(prev) && isUpperASCII(current) {
		return true
	}
	if isUpperASCII(prev) && isUpperASCII(current) && next != 0 && isLowerASCII(next) {
		return true
	}
	return false
}

func isUpperASCII(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

func isLowerASCII(r rune) bool {
	return r >= 'a' && r <= 'z'
}

func containsWord(words []string, word string) bool {
	for _, candidate := range words {
		if strings.EqualFold(candidate, word) {
			return true
		}
	}
	return false
}

func (p *Plugin) markSchemaUsed(schemaName string) {
	schemaName = cleanTypeName(schemaName)
	if schemaName != "" {
		p.usedSchemas[schemaName] = true
	}
}

func (p *Plugin) markSchemaRefs(schemaValue *openapi.Schema) {
	if schemaValue == nil {
		return
	}
	if schemaValue.Ref != "" {
		p.markSchemaUsed(strings.TrimPrefix(schemaValue.Ref, "#/components/schemas/"))
	}
	if schemaValue.Items != nil {
		p.markSchemaRefs(schemaValue.Items)
	}
	for _, prop := range schemaValue.Properties {
		p.markSchemaRefs(prop)
	}
	if schemaValue.AdditionalProperties != nil {
		p.markSchemaRefs(schemaValue.AdditionalProperties)
	}
	for _, item := range schemaValue.AllOf {
		p.markSchemaRefs(item)
	}
	for _, item := range schemaValue.OneOf {
		p.markSchemaRefs(item)
	}
	for _, item := range schemaValue.AnyOf {
		p.markSchemaRefs(item)
	}
}

// hasFiberImport checks if the file imports Fiber.
func (p *Plugin) hasFiberImport(pf *parser.ParsedFile) bool {
	for _, importPath := range fiberImportPaths {
		if p.goParser.HasImport(pf, importPath) {
			return true
		}
	}
	return false
}

// ExtractSchemas extracts schema definitions from Go structs.
func (p *Plugin) ExtractSchemas(files []scanner.SourceFile) ([]openapi.Schema, error) {
	p.ensureFibergenCatalog(files)
	p.schemaExtractor = schema.NewGoSchemaExtractor()

	for _, file := range files {
		if file.Language != "go" {
			continue
		}

		pf, err := p.goParser.ParseSource(file.Path, string(file.Content))
		if err != nil {
			continue
		}

		structs := p.goParser.ExtractStructs(pf)
		for _, def := range structs {
			p.schemaExtractor.ExtractFromStruct(def)
		}
		p.extractNamedPrimitiveSchemas(pf)
	}
	p.registerFibergenSchemas()
	p.registerSyntheticSchemas()

	if len(p.usedSchemas) > 0 {
		p.expandUsedSchemas()
		return p.filteredSchemas(), nil
	}

	return p.schemaExtractor.Registry().ToSlice(), nil
}

func (p *Plugin) registerSyntheticSchemas() {}

func (p *Plugin) extractNamedPrimitiveSchemas(pf *parser.ParsedFile) {
	for _, decl := range pf.AST.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			typeName := astTypeString(typeSpec.Type)
			schemaType := schemaForNamedPrimitive(typeName)
			if schemaType == nil {
				continue
			}
			schemaType.Title = typeSpec.Name.Name
			p.schemaExtractor.Registry().Add(typeSpec.Name.Name, schemaType)
		}
	}
}

func schemaForNamedPrimitive(typeName string) *openapi.Schema {
	switch typeName {
	case "string":
		return &openapi.Schema{Type: "string"}
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return &openapi.Schema{Type: "integer"}
	case "float32", "float64":
		return &openapi.Schema{Type: "number"}
	case "bool":
		return &openapi.Schema{Type: "boolean"}
	default:
		return nil
	}
}

func (p *Plugin) expandUsedSchemas() {
	registry := p.schemaExtractor.Registry()
	for {
		changed := false
		for name := range p.usedSchemas {
			schemaValue, ok := registry.Get(name)
			if !ok {
				continue
			}
			before := len(p.usedSchemas)
			p.markSchemaRefs(schemaValue)
			if len(p.usedSchemas) > before {
				changed = true
			}
		}
		if !changed {
			return
		}
	}
}

func (p *Plugin) filteredSchemas() []openapi.Schema {
	registry := p.schemaExtractor.Registry()
	schemas := make([]openapi.Schema, 0, len(p.usedSchemas))
	for name := range p.usedSchemas {
		schemaValue, ok := registry.Get(name)
		if !ok || schemaValue == nil {
			continue
		}
		p.stripPathFieldsFromRequestSchema(name, schemaValue)
		schemas = append(schemas, *schemaValue)
	}
	return schemas
}

func (p *Plugin) stripPathFieldsFromRequestSchema(name string, schemaValue *openapi.Schema) {
	fields := p.requestPathFields[name]
	if len(fields) == 0 || schemaValue == nil {
		return
	}
	for field := range fields {
		delete(schemaValue.Properties, field)
	}
	if len(schemaValue.Required) > 0 {
		filtered := schemaValue.Required[:0]
		for _, field := range schemaValue.Required {
			if !fields[field] {
				filtered = append(filtered, field)
			}
		}
		schemaValue.Required = filtered
	}
}

// --- Helper Functions ---

// normalizePath normalizes a route path.
func normalizePath(path string) string {
	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Remove double slashes
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}

	// Remove trailing slash (except for root)
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}

	return path
}

// colonParamRegex matches Fiber path parameters like :param.
var colonParamRegex = regexp.MustCompile(`:([a-zA-Z_][a-zA-Z0-9_]*)`)

// wildcardRegex matches Fiber wildcard like *.
var wildcardRegex = regexp.MustCompile(`\*`)

// convertFiberPathParams converts Fiber :param and * syntax to OpenAPI {param} syntax.
func convertFiberPathParams(path string) string {
	// Convert :param to {param}
	path = colonParamRegex.ReplaceAllString(path, "{$1}")

	// Convert * to {path} (wildcard)
	path = wildcardRegex.ReplaceAllString(path, "{path}")

	return path
}

// pathParamRegex matches OpenAPI path parameters like {id} or {userId}.
var pathParamRegex = regexp.MustCompile(`\{([^}]+)\}`)

// extractPathParams extracts path parameters from a route path.
func extractPathParams(path string) []openapi.Parameter {
	var params []openapi.Parameter

	matches := pathParamRegex.FindAllStringSubmatch(path, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		paramName := match[1]

		params = append(params, openapi.Parameter{
			Name:     paramName,
			In:       "path",
			Required: true,
			Schema: &openapi.Schema{
				Type: "string",
			},
		})
	}

	return params
}

// generateOperationID generates an operation ID from method, path, and handler.
func generateOperationID(method, path, handler string) string {
	// If we have a handler name, use it
	if handler != "" && handler != "<anonymous>" && handler != "<wrapped>" {
		// Remove package prefix and clean up
		parts := strings.Split(handler, ".")
		name := parts[len(parts)-1]
		return strings.ToLower(method) + name
	}

	// Generate from path
	// Remove parameter syntax and convert to camelCase
	path = pathParamRegex.ReplaceAllString(path, "By${1}")
	path = strings.ReplaceAll(path, "/", " ")
	path = strings.TrimSpace(path)

	words := strings.Fields(path)
	if len(words) == 0 {
		return strings.ToLower(method)
	}

	// Build camelCase operation ID
	var sb strings.Builder
	sb.WriteString(strings.ToLower(method))

	titleCaser := cases.Title(language.English)
	for _, word := range words {
		word = titleCaser.String(strings.ToLower(word))
		sb.WriteString(word)
	}

	return sb.String()
}

// inferTags infers tags from the route path.
func inferTags(path string) []string {
	// Remove leading slash and split
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		return nil
	}

	// Skip common prefixes like "api", "v1", etc.
	skipPrefixes := map[string]bool{
		"api": true,
		"v1":  true,
		"v2":  true,
		"v3":  true,
	}

	// Find the first meaningful segment
	var tagPart string
	for _, part := range parts {
		if part == "" {
			continue
		}
		// Skip version/api prefixes
		if skipPrefixes[part] {
			continue
		}
		// Skip if it's a parameter
		if strings.HasPrefix(part, "{") {
			continue
		}
		tagPart = part
		break
	}

	if tagPart == "" {
		return nil
	}

	return []string{tagPart}
}

func (p *Plugin) indexGlobalMiddleware(pf *parser.ParsedFile) {
	appVars := make(map[string]bool)
	ast.Inspect(pf.AST, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 {
			return true
		}
		callExpr, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok || !isFiberNewCall(callExpr) {
			return true
		}
		for _, lhs := range assign.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok {
				appVars[ident.Name] = true
			}
		}
		return true
	})

	ast.Inspect(pf.AST, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok || selExpr.Sel.Name != "Use" {
			return true
		}
		ident, ok := selExpr.X.(*ast.Ident)
		if !ok || !appVars[ident.Name] {
			return true
		}
		p.globalMiddleware = append(p.globalMiddleware, callExpr.Args...)
		return true
	})
}

func isFiberNewCall(callExpr *ast.CallExpr) bool {
	switch fn := callExpr.Fun.(type) {
	case *ast.Ident:
		return fn.Name == "New"
	case *ast.SelectorExpr:
		return fn.Sel.Name == "New" && strings.Contains(astTypeString(fn.X), "fiber")
	default:
		return false
	}
}

// Register registers the Fiber plugin with the global registry.
func Register() {
	plugins.MustRegister(New())
}

func init() {
	Register()
}
