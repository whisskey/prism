// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"
	"strings"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/pkg/openapi"
)

type handlerInfo struct {
	summary        string
	description    string
	requestSchema  string
	hasServerError bool
	paramTypes     map[string]*openapi.Schema
	queryParams    map[string]*openapi.Schema
	responses      map[string]openapi.Response
	errorStatuses  map[string]bool
}

type responseHelperKind string

const (
	responseHelperUnknown responseHelperKind = ""
	responseHelperSuccess responseHelperKind = "success"
	responseHelperError   responseHelperKind = "error"
)

type responseHelper struct {
	kind             responseHelperKind
	dataArgIndex     int
	responseTypeName string
	payloadField     string // JSON property name for the success payload field, when known
	statusCode       string
}

type inferredResponse struct {
	code    string
	schema  *openapi.Schema
	example interface{}
}

func (p *Plugin) analyzeHandler(funcDecl *ast.FuncDecl) handlerInfo {
	analysis := p.collectHandlerAnalysis(funcDecl)
	route := &openapi.Route{Method: "GET"}
	graph := p.resolveRouteTypeGraph(route, RouteAnalysis{Handler: &analysis})
	return handlerInfoFromTypeGraph(graph, analysis)
}

func (p *Plugin) enrichRoute(route *openapi.Route) {
	handler := p.resolveHandlerRef(route.HandlerRef)
	if handler == nil {
		handler = p.resolveHandler(route.Handler)
	}
	if handler == nil {
		return
	}

	analysis := p.collectHandlerAnalysis(handler)
	graph := p.resolveRouteTypeGraph(route, RouteAnalysis{Handler: &analysis})
	p.applyRouteTypeGraph(route, graph)
	p.applyHandlerMetadata(route, &analysis)
}

func handlerInfoFromTypeGraph(graph RouteTypeGraph, analysis HandlerAnalysis) handlerInfo {
	info := handlerInfo{
		paramTypes:    make(map[string]*openapi.Schema),
		queryParams:   make(map[string]*openapi.Schema),
		responses:     make(map[string]openapi.Response),
		errorStatuses: make(map[string]bool),
	}

	if graph.RequestBody != nil {
		info.requestSchema = graph.RequestBody.SchemaName
	}

	for _, param := range graph.Parameters {
		switch param.In {
		case "path":
			info.paramTypes[param.Name] = param.Schema
		case "query":
			info.queryParams[param.Name] = param.Schema
		}
	}

	for code, resolution := range graph.Responses {
		if resolution.Source == "handler.exception" {
			info.errorStatuses[code] = true
			if code == "500" {
				info.hasServerError = true
			}
		}
		info.responses[code] = jsonResponseWithExample(responseDescription(code), resolution.Schema, resolution.Example)
	}

	return info
}

func (p *Plugin) responseFromJSONCall(callExpr *ast.CallExpr, localTypes map[string]string, pf *parser.ParsedFile) *inferredResponse {
	if response := p.responseFromHelperCall(callExpr, localTypes, pf); response != nil {
		return response
	}

	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || selExpr.Sel.Name != "JSON" || len(callExpr.Args) == 0 {
		return nil
	}

	code := "200"
	if statusCall, ok := selExpr.X.(*ast.CallExpr); ok {
		if statusSel, ok := statusCall.Fun.(*ast.SelectorExpr); ok && statusSel.Sel.Name == "Status" && len(statusCall.Args) > 0 {
			code = statusCodeFromExpr(statusCall.Args[0])
		}
	}

	schemaValue := p.schemaFromJSONExprWithFile(callExpr.Args[0], localTypes, pf, 0)
	if schemaValue == nil {
		return nil
	}
	return &inferredResponse{
		code:    code,
		schema:  schemaValue,
		example: p.exampleFromSchema(schemaValue),
	}
}

func (p *Plugin) responseFromSendStatusCall(callExpr *ast.CallExpr) *inferredResponse {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || selExpr.Sel.Name != "SendStatus" || len(callExpr.Args) == 0 {
		return nil
	}
	code := statusCodeFromExpr(callExpr.Args[0])
	if code == "" || strings.HasPrefix(code, "2") || strings.HasPrefix(code, "3") {
		return nil
	}
	return &inferredResponse{
		code: code,
	}
}

func (p *Plugin) responseFromHelperCall(callExpr *ast.CallExpr, localTypes map[string]string, pf *parser.ParsedFile) *inferredResponse {
	if override, ok := p.configuredResponseHelper(callExpr); ok {
		return p.responseFromConfiguredHelper(callExpr, localTypes, override)
	}

	helperDecl := p.resolveHelperFunc(callExpr)
	if helperDecl == nil {
		return nil
	}

	helper := p.analyzeResponseHelper(helperDecl)
	switch helper.kind {
	case responseHelperSuccess:
		if helper.dataArgIndex < 0 || helper.dataArgIndex >= len(callExpr.Args) {
			return nil
		}
		schemaValue := p.schemaFromJSONExprWithFile(callExpr.Args[helper.dataArgIndex], localTypes, pf, 0)
		if schemaValue == nil {
			return nil
		}
		optionStart := helper.dataArgIndex + 1
		if optionStart > len(callExpr.Args) {
			optionStart = len(callExpr.Args)
		}
		code := responseStatusFromOptions(callExpr.Args[optionStart:], helperDefaultStatus(helper, "200"))
		responseSchema := p.specializedResponseSchema(helper.responseTypeName, helper.payloadField, schemaValue)
		return &inferredResponse{
			code:    code,
			schema:  responseSchema,
			example: p.exampleFromSchema(responseSchema),
		}
	case responseHelperError:
		if len(callExpr.Args) == 0 {
			return nil
		}
		optionStart := 1
		if optionStart > len(callExpr.Args) {
			optionStart = len(callExpr.Args)
		}
		code := responseStatusFromOptions(callExpr.Args[optionStart:], "400")
		responseSchema := p.responseSchemaFromType(helper.responseTypeName)
		return &inferredResponse{
			code:    code,
			schema:  responseSchema,
			example: p.exampleFromSchema(responseSchema),
		}
	default:
		return nil
	}
}

func (p *Plugin) responseFromConfiguredHelper(callExpr *ast.CallExpr, localTypes map[string]string, override responseHelperOverride) *inferredResponse {
	switch override.kind {
	case responseHelperSuccess:
		if len(callExpr.Args) < 2 {
			return nil
		}
		schemaValue := p.schemaFromJSONExpr(callExpr.Args[1], localTypes)
		if schemaValue == nil {
			return nil
		}
		responseSchema := p.specializedResponseSchema("", "data", schemaValue)
		return &inferredResponse{
			code:    "200",
			schema:  responseSchema,
			example: p.exampleFromSchema(responseSchema),
		}
	case responseHelperError:
		if len(callExpr.Args) == 0 {
			return nil
		}
		schemaName := override.schema
		if schemaName == "" {
			schemaName = p.defaultErrorSchemaName()
		}
		responseSchema := p.responseSchemaFromType(schemaName)
		return &inferredResponse{
			code:    "400",
			schema:  responseSchema,
			example: p.exampleFromSchema(responseSchema),
		}
	default:
		return nil
	}
}

func (p *Plugin) resolveHelperFunc(callExpr *ast.CallExpr) *ast.FuncDecl {
	switch fn := callExpr.Fun.(type) {
	case *ast.Ident:
		return p.routeFuncIndex[fn.Name]
	case *ast.SelectorExpr:
		return p.routeFuncIndex[fn.Sel.Name]
	case *ast.IndexExpr:
		return p.resolveFuncFromExpr(fn.X)
	case *ast.IndexListExpr:
		return p.resolveFuncFromExpr(fn.X)
	default:
		return nil
	}
}

func (p *Plugin) resolveFuncFromExpr(expr ast.Expr) *ast.FuncDecl {
	switch e := expr.(type) {
	case *ast.Ident:
		return p.routeFuncIndex[e.Name]
	case *ast.SelectorExpr:
		return p.routeFuncIndex[e.Sel.Name]
	default:
		return nil
	}
}

func (p *Plugin) analyzeResponseHelper(funcDecl *ast.FuncDecl) responseHelper {
	if funcDecl == nil {
		return responseHelper{}
	}
	if cached, ok := p.responseHelpers[funcDecl]; ok {
		return cached
	}

	helper := responseHelper{dataArgIndex: -1}
	if !isFiberHandler(funcDecl) || funcDecl.Body == nil {
		p.responseHelpers[funcDecl] = helper
		return helper
	}

	paramIndex := responseHelperParamIndex(funcDecl)
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		if helper.kind != responseHelperUnknown {
			return false
		}
		callExpr, ok := n.(*ast.CallExpr)
		if !ok || !isJSONCall(callExpr) || len(callExpr.Args) == 0 {
			return true
		}

		compositeLit, ok := callExpr.Args[0].(*ast.CompositeLit)
		if !ok {
			return true
		}
		kind, dataArgIndex, payloadField := responseHelperFromCompositeLit(compositeLit, paramIndex)
		if kind == responseHelperUnknown {
			return true
		}
		helper.kind = kind
		helper.dataArgIndex = dataArgIndex
		helper.responseTypeName = typeBaseName(compositeLit.Type)
		helper.payloadField = p.jsonFieldName(helper.responseTypeName, payloadField)
		if code := statusCodeFromJSONCall(callExpr); code != "" {
			helper.statusCode = code
		}
		return false
	})

	p.responseHelpers[funcDecl] = helper
	return helper
}

func responseHelperParamIndex(funcDecl *ast.FuncDecl) map[string]int {
	index := make(map[string]int)
	if funcDecl.Type.Params == nil {
		return index
	}
	position := 0
	for _, field := range funcDecl.Type.Params.List {
		if len(field.Names) == 0 {
			position++
			continue
		}
		for _, name := range field.Names {
			index[name.Name] = position
			position++
		}
	}
	return index
}

func isJSONCall(callExpr *ast.CallExpr) bool {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	return ok && selExpr.Sel.Name == "JSON"
}

func statusCodeFromJSONCall(callExpr *ast.CallExpr) string {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || selExpr.Sel.Name != "JSON" {
		return ""
	}
	statusCall, ok := selExpr.X.(*ast.CallExpr)
	if !ok || len(statusCall.Args) == 0 {
		return ""
	}
	statusSel, ok := statusCall.Fun.(*ast.SelectorExpr)
	if !ok || statusSel.Sel.Name != "Status" {
		return ""
	}
	return statusCodeFromExpr(statusCall.Args[0])
}

func helperDefaultStatus(helper responseHelper, fallback string) string {
	if helper.statusCode != "" {
		return helper.statusCode
	}
	return fallback
}

func responseHelperFromCompositeLit(lit *ast.CompositeLit, paramIndex map[string]int) (responseHelperKind, int, string) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "Data", "Payload", "Result", "Items":
			if ident, ok := kv.Value.(*ast.Ident); ok {
				if index, ok := paramIndex[ident.Name]; ok {
					return responseHelperSuccess, index, key.Name
				}
			}
			return responseHelperSuccess, -1, key.Name
		case "Errors", "Error", "Messages":
			return responseHelperError, -1, key.Name
		}
	}
	return responseHelperUnknown, -1, ""
}

func (p *Plugin) schemaFromJSONExpr(expr ast.Expr, localTypes map[string]string) *openapi.Schema {
	return p.schemaFromJSONExprWithFile(expr, localTypes, nil, 0)
}

func (p *Plugin) schemaFromJSONExprWithFile(expr ast.Expr, localTypes map[string]string, pf *parser.ParsedFile, line int) *openapi.Schema {
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		return p.schemaFromJSONExprWithFile(e.X, localTypes, pf, line)
	case *ast.Ident:
		if schemaName := localTypes[e.Name]; schemaName != "" {
			return schemaFromTypeName(schemaName)
		}
		if pf != nil {
			identLine := line
			if identLine == 0 {
				identLine = positionLine(pf, e.Pos())
			}
			if schemaName := p.typedIdentSchema(pf, identLine, e.Name); schemaName != "" {
				return schemaFromTypeName(schemaName)
			}
		}
	case *ast.CallExpr:
		if returnType := p.inferReturnTypeFromCall(e); returnType != "" {
			return schemaFromTypeName(returnType)
		}
	case *ast.CompositeLit:
		return schemaFromCompositeLit(e, localTypes, p)
	}
	return nil
}

func schemaFromTypeName(typeName string) *openapi.Schema {
	if strings.HasPrefix(typeName, "[]") {
		itemName := cleanTypeName(typeName)
		if itemName == "" {
			return nil
		}
		return &openapi.Schema{Type: "array", Items: schemaRef(itemName)}
	}
	schemaName := cleanTypeName(typeName)
	if schemaName != "" {
		return schemaRef(schemaName)
	}
	return nil
}

func schemaFromCompositeLit(lit *ast.CompositeLit, localTypes map[string]string, plugin *Plugin) *openapi.Schema {
	if isMapLikeCompositeType(lit.Type) {
		return schemaFromMapCompositeLit(lit, localTypes, plugin)
	}

	switch t := lit.Type.(type) {
	case *ast.ArrayType:
		itemName := cleanTypeName(astTypeString(t.Elt))
		if itemName == "" {
			return nil
		}
		return &openapi.Schema{Type: "array", Items: schemaRef(itemName)}
	case *ast.MapType:
		return &openapi.Schema{Type: "object"}
	default:
		if astTypeString(t) == "fiber.Map" {
			return schemaFromMapCompositeLit(lit, localTypes, plugin)
		}
		schemaName := typeBaseName(t)
		if schemaName == "" {
			return nil
		}
		return schemaRef(schemaName)
	}
}

func typeBaseName(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	case *ast.IndexExpr:
		return typeBaseName(e.X)
	case *ast.IndexListExpr:
		return typeBaseName(e.X)
	default:
		return cleanTypeName(astTypeString(expr))
	}
}

func (p *Plugin) jsonFieldName(structName, goFieldName string) string {
	if structName != "" && goFieldName != "" {
		if def, ok := p.structIndex[structName]; ok {
			for _, field := range def.Fields {
				if field.Name == goFieldName {
					return field.JSONName
				}
			}
		}
	}
	if goFieldName == "" {
		return ""
	}
	return strings.ToLower(goFieldName[:1]) + goFieldName[1:]
}

func (p *Plugin) responseSchemaFromType(typeName string) *openapi.Schema {
	typeName = cleanTypeName(typeName)
	if typeName == "" {
		return nil
	}
	p.markSchemaUsed(typeName)
	if schemaValue, ok := p.schemaExtractor.Registry().Get(typeName); ok && schemaValue != nil {
		return cloneSchema(schemaValue)
	}
	return schemaRef(typeName)
}

func (p *Plugin) specializedResponseSchema(responseTypeName, payloadField string, payloadSchema *openapi.Schema) *openapi.Schema {
	if payloadSchema == nil {
		return p.responseSchemaFromType(responseTypeName)
	}
	if responseTypeName == "" {
		return payloadSchema
	}

	p.markSchemaUsed(responseTypeName)
	if base, ok := p.schemaExtractor.Registry().Get(responseTypeName); ok && base != nil {
		return p.cloneSchemaWithPayloadField(base, payloadField, payloadSchema)
	}
	if payloadField != "" {
		return &openapi.Schema{
			Type: "object",
			Properties: map[string]*openapi.Schema{
				payloadField: payloadSchema,
			},
		}
	}
	return payloadSchema
}

func (p *Plugin) cloneSchemaWithPayloadField(base *openapi.Schema, payloadField string, payloadSchema *openapi.Schema) *openapi.Schema {
	if base == nil {
		return payloadSchema
	}
	if base.Ref != "" {
		name := strings.TrimPrefix(base.Ref, "#/components/schemas/")
		if resolved, ok := p.schemaExtractor.Registry().Get(name); ok && resolved != nil {
			base = resolved
		}
	}
	if base.Type != "object" || len(base.Properties) == 0 {
		return schemaRef(base.Title)
	}

	properties := make(map[string]*openapi.Schema, len(base.Properties))
	for name, prop := range base.Properties {
		properties[name] = prop
	}
	if payloadField != "" && payloadSchema != nil {
		properties[payloadField] = payloadSchema
	}

	cloned := &openapi.Schema{
		Type:       "object",
		Title:      base.Title,
		Properties: properties,
	}
	if len(base.Required) > 0 {
		cloned.Required = append([]string(nil), base.Required...)
	}
	return cloned
}

func cloneSchema(schemaValue *openapi.Schema) *openapi.Schema {
	if schemaValue == nil {
		return nil
	}
	if schemaValue.Ref != "" {
		return schemaRef(strings.TrimPrefix(schemaValue.Ref, "#/components/schemas/"))
	}
	cloned := &openapi.Schema{
		Type:       schemaValue.Type,
		Title:      schemaValue.Title,
		Format:     schemaValue.Format,
		Example:    schemaValue.Example,
		Enum:       schemaValue.Enum,
		Required:   append([]string(nil), schemaValue.Required...),
		Properties: make(map[string]*openapi.Schema, len(schemaValue.Properties)),
	}
	for name, prop := range schemaValue.Properties {
		cloned.Properties[name] = cloneSchema(prop)
	}
	if schemaValue.Items != nil {
		cloned.Items = cloneSchema(schemaValue.Items)
	}
	return cloned
}

func isMapLikeCompositeType(typeExpr ast.Expr) bool {
	switch t := typeExpr.(type) {
	case *ast.MapType:
		return true
	case *ast.Ident:
		return t.Name == "Map"
	case *ast.SelectorExpr:
		return t.Sel.Name == "Map"
	default:
		return false
	}
}

func schemaFromMapCompositeLit(lit *ast.CompositeLit, localTypes map[string]string, plugin *Plugin) *openapi.Schema {
	properties := make(map[string]*openapi.Schema)
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := parser.ExtractStringLiteral(kv.Key)
		if !ok || key == "" {
			continue
		}
		propSchema := schemaFromMapValue(kv.Value, localTypes, plugin)
		if propSchema == nil {
			propSchema = &openapi.Schema{Type: "string"}
		}
		properties[key] = propSchema
	}
	if len(properties) == 0 {
		return &openapi.Schema{Type: "object"}
	}
	return &openapi.Schema{Type: "object", Properties: properties}
}

func schemaFromMapValue(expr ast.Expr, localTypes map[string]string, plugin *Plugin) *openapi.Schema {
	switch e := expr.(type) {
	case *ast.BasicLit:
		switch e.Kind.String() {
		case "STRING":
			value, ok := parser.ExtractStringLiteral(e)
			if !ok {
				return &openapi.Schema{Type: "string"}
			}
			return &openapi.Schema{Type: "string", Example: value}
		case "INT", "FLOAT", "IMAG":
			return &openapi.Schema{Type: "number"}
		}
	case *ast.Ident:
		switch e.Name {
		case "true", "false":
			return &openapi.Schema{Type: "boolean", Example: e.Name == "true"}
		default:
			if typeName := localTypes[e.Name]; typeName != "" {
				return schemaFromTypeName(typeName)
			}
		}
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Now" {
			return &openapi.Schema{Type: "string", Format: "date-time"}
		}
		if plugin != nil {
			if returnType := plugin.inferReturnTypeFromCall(e); returnType != "" {
				return schemaFromTypeName(returnType)
			}
		}
	}
	return nil
}

func responseStatusFromOptions(args []ast.Expr, fallback string) string {
	for _, arg := range args {
		callExpr, ok := arg.(*ast.CallExpr)
		if !ok || len(callExpr.Args) == 0 {
			continue
		}
		switch fn := callExpr.Fun.(type) {
		case *ast.SelectorExpr:
			if fn.Sel.Name == "WithStatus" {
				return statusCodeFromExpr(callExpr.Args[0])
			}
		case *ast.Ident:
			if fn.Name == "WithStatus" {
				return statusCodeFromExpr(callExpr.Args[0])
			}
		}
	}
	return fallback
}

func exceptionStatusFromCall(callExpr *ast.CallExpr) string {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if selectorName(selExpr.X) == "fiber" && selExpr.Sel.Name == "NewError" && len(callExpr.Args) > 0 {
		return statusCodeFromExpr(callExpr.Args[0])
	}
	if selectorName(selExpr.X) != "exception" {
		return ""
	}
	switch selExpr.Sel.Name {
	case "BadRequest":
		return "400"
	case "Unauthorized":
		return "401"
	case "Forbidden":
		return "403"
	case "NotFound":
		return "404"
	case "Conflict":
		return "409"
	case "ValidationError":
		return "422"
	case "TooManyRequests":
		return "429"
	case "InternalServerError":
		return "500"
	case "ServiceUnavailable":
		return "503"
	case "GatewayTimeout":
		return "504"
	case "New":
		if len(callExpr.Args) > 0 {
			return statusCodeFromExpr(callExpr.Args[0])
		}
	}
	return ""
}

func isExceptionCall(callExpr *ast.CallExpr, name string) bool {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	return ok && selectorName(selExpr.X) == "exception" && selExpr.Sel.Name == name
}
