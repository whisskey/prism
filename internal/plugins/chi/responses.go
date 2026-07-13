// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package chi

import (
	"go/ast"
	"strings"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/pkg/openapi"
)

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
	payloadField     string
	statusCode       string
}

type inferredResponse struct {
	code    string
	schema  *openapi.Schema
	example interface{}
}

func (p *Plugin) responseFromHelperCall(callExpr *ast.CallExpr, localTypes map[string]string, pf *parser.ParsedFile) *inferredResponse {
	helperDecl := p.resolveHelperFunc(callExpr)
	if helperDecl == nil {
		return nil
	}
	helper := p.analyzeResponseHelper(helperDecl)
	if helper.kind != responseHelperSuccess {
		return nil
	}
	if helper.dataArgIndex < 0 || helper.dataArgIndex >= len(callExpr.Args) {
		return nil
	}
	schemaValue := p.schemaFromJSONExprWithFile(callExpr.Args[helper.dataArgIndex], localTypes, pf)
	if schemaValue == nil {
		return nil
	}
	code := helper.statusCode
	if code == "" {
		code = "200"
	}
	responseSchema := p.specializedResponseSchema(helper.responseTypeName, helper.payloadField, schemaValue)
	return &inferredResponse{
		code:    code,
		schema:  responseSchema,
		example: p.exampleFromSchema(responseSchema),
	}
}

func (p *Plugin) resolveHelperFunc(callExpr *ast.CallExpr) *ast.FuncDecl {
	switch fn := callExpr.Fun.(type) {
	case *ast.Ident:
		return p.routeFuncIndex[fn.Name]
	case *ast.SelectorExpr:
		return p.routeFuncIndex[fn.Sel.Name]
	default:
		return nil
	}
}

func (p *Plugin) analyzeResponseHelper(funcDecl *ast.FuncDecl) responseHelper {
	if funcDecl == nil || funcDecl.Body == nil {
		return responseHelper{dataArgIndex: -1}
	}
	if p.responseHelpers == nil {
		p.responseHelpers = make(map[*ast.FuncDecl]responseHelper)
	}
	if cached, ok := p.responseHelpers[funcDecl]; ok {
		return cached
	}

	helper := responseHelper{dataArgIndex: -1}
	paramIndex := responseHelperParamIndex(funcDecl)
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		if helper.kind != responseHelperUnknown {
			return false
		}
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		compositeLit := encodeCompositeLit(callExpr)
		if compositeLit == nil {
			return true
		}
		kind, dataArgIndex, payloadField := responseHelperFromCompositeLit(compositeLit, paramIndex)
		if kind == responseHelperUnknown {
			return true
		}
		helper.kind = kind
		helper.dataArgIndex = dataArgIndex
		helper.responseTypeName = typeBaseName(compositeLit.Type)
		helper.payloadField = payloadField
		if helper.payloadField == "" {
			helper.payloadField = "data"
		} else {
			helper.payloadField = p.jsonFieldName(helper.responseTypeName, payloadField)
		}
		helper.statusCode = "200"
		return false
	})
	p.responseHelpers[funcDecl] = helper
	return helper
}

func encodeCompositeLit(callExpr *ast.CallExpr) *ast.CompositeLit {
	if callExpr == nil || len(callExpr.Args) == 0 {
		return nil
	}
	// json.NewEncoder(w).Encode(Response{...})
	if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Encode" {
		if lit, ok := callExpr.Args[0].(*ast.CompositeLit); ok {
			return lit
		}
	}
	// render.JSON(w, r, Response{...})
	if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "JSON" && len(callExpr.Args) >= 3 {
		if lit, ok := callExpr.Args[len(callExpr.Args)-1].(*ast.CompositeLit); ok {
			return lit
		}
	}
	return nil
}

func responseHelperParamIndex(funcDecl *ast.FuncDecl) map[string]int {
	index := make(map[string]int)
	if funcDecl.Type.Params == nil {
		return index
	}
	position := 0
	for _, field := range funcDecl.Type.Params.List {
		names := field.Names
		if len(names) == 0 {
			position++
			continue
		}
		for _, name := range names {
			index[name.Name] = position
			position++
		}
	}
	return index
}

func responseHelperFromCompositeLit(lit *ast.CompositeLit, paramIndex map[string]int) (responseHelperKind, int, string) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyName := ""
		switch k := kv.Key.(type) {
		case *ast.Ident:
			keyName = k.Name
		}
		if keyName == "" {
			continue
		}
		lower := strings.ToLower(keyName)
		if lower != "data" && lower != "payload" && lower != "result" {
			continue
		}
		switch v := kv.Value.(type) {
		case *ast.Ident:
			if idx, ok := paramIndex[v.Name]; ok {
				return responseHelperSuccess, idx, keyName
			}
		}
	}
	return responseHelperUnknown, -1, ""
}

func (p *Plugin) schemaFromJSONExprWithFile(expr ast.Expr, localTypes map[string]string, pf *parser.ParsedFile) *openapi.Schema {
	switch e := expr.(type) {
	case *ast.Ident:
		if schemaName := localTypes[e.Name]; schemaName != "" {
			return schemaFromTypeName(schemaName)
		}
		if pf != nil {
			line := positionLine(pf, e.Pos())
			if schemaName := p.typedIdentSchema(pf, line, e.Name); schemaName != "" {
				return schemaFromTypeName(schemaName)
			}
		}
	case *ast.CompositeLit:
		return schemaFromCompositeLit(e)
	case *ast.CallExpr:
		if returnType := p.inferReturnTypeFromCall(e); returnType != "" {
			return schemaFromTypeName(returnType)
		}
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

func schemaFromCompositeLit(lit *ast.CompositeLit) *openapi.Schema {
	switch t := lit.Type.(type) {
	case *ast.ArrayType:
		itemName := cleanTypeName(astTypeString(t.Elt))
		if itemName == "" {
			return nil
		}
		return &openapi.Schema{Type: "array", Items: schemaRef(itemName)}
	default:
		schemaName := typeBaseName(lit.Type)
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
	default:
		return cleanTypeName(astTypeString(expr))
	}
}

func (p *Plugin) jsonFieldName(structName, goFieldName string) string {
	if structName != "" && goFieldName != "" {
		if def, ok := p.structIndex[structName]; ok {
			for _, field := range def.Fields {
				if field.Name == goFieldName {
					if field.JSONName != "" {
						return field.JSONName
					}
					break
				}
			}
		}
	}
	if goFieldName == "" {
		return ""
	}
	return strings.ToLower(goFieldName[:1]) + goFieldName[1:]
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
			Type:  "object",
			Title: responseTypeName,
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
	if base.Title == "" {
		cloned.Title = base.Title
	}
	if len(base.Required) > 0 {
		cloned.Required = append([]string(nil), base.Required...)
	}
	return cloned
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


func jsonResponseWithExample(description string, schema *openapi.Schema, example interface{}) openapi.Response {
	resp := openapi.Response{Description: description}
	if schema != nil {
		media := openapi.MediaType{Schema: schema}
		if example != nil {
			media.Example = example
		}
		resp.Content = map[string]openapi.MediaType{
			"application/json": media,
		}
	}
	return resp
}
