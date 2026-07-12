// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"go/types"
	"reflect"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// SchemaBuilderPass builds OpenAPI schemas from Go struct types (Pass 3).
func SchemaBuilderPass(ctx Context, handlers map[string]HandlerDetail) map[string]Schema {
	if ctx.Pass == nil {
		return nil
	}

	names := collectSchemaTypeNames(ctx.Raw.Structs, handlers)
	schemas := make(map[string]Schema, len(names))
	for name := range names {
		if schema, ok := buildNamedSchema(ctx.Pass, name); ok {
			schemas[name] = schema
			continue
		}
		if info := structInfoByName(ctx.Raw.Structs, name); info != nil {
			if schema, ok := buildSchemaFromStructInfo(ctx.Pass, *info); ok {
				schemas[name] = schema
				continue
			}
		}
		warnUnresolvedSchema(ctx, name)
		schemas[name] = emptySchema(name)
	}
	return schemas
}

func emptySchema(name string) Schema {
	return Schema{
		Name:       name,
		Type:       "object",
		Properties: map[string]Property{},
	}
}

func collectSchemaTypeNames(structs []StructInfo, handlers map[string]HandlerDetail) map[string]struct{} {
	names := make(map[string]struct{}, len(structs))
	for _, info := range structs {
		names[info.Name] = struct{}{}
	}
	for _, handler := range handlers {
		addReferencedTypeName(names, handler.RequestBody)
		addReferencedTypeName(names, handler.ResponseType)
		for _, schemaName := range handler.ResponseSchemas {
			addReferencedTypeName(names, schemaName)
		}
	}
	return names
}

func addReferencedTypeName(names map[string]struct{}, typ string) {
	typ = strings.TrimPrefix(typ, "[]")
	if typ == "" || isPrimitiveName(typ) {
		return
	}
	names[typ] = struct{}{}
}

func structInfoByName(structs []StructInfo, name string) *StructInfo {
	for i := range structs {
		if structs[i].Name == name {
			return &structs[i]
		}
	}
	return nil
}

func buildNamedSchema(pass *analysis.Pass, typeName string) (Schema, bool) {
	if pass == nil || pass.Pkg == nil || typeName == "" {
		return Schema{}, false
	}
	obj := pass.Pkg.Scope().Lookup(typeName)
	if obj == nil {
		return Schema{}, false
	}
	named, ok := obj.Type().(*types.Named)
	if !ok {
		return Schema{}, false
	}
	strct, ok := named.Underlying().(*types.Struct)
	if !ok {
		return Schema{}, false
	}
	return buildSchemaFromStruct(pass, typeName, strct), true
}

func buildSchemaFromStructInfo(pass *analysis.Pass, info StructInfo) (Schema, bool) {
	if pass != nil && pass.TypesInfo != nil && info.TypeSpec != nil {
		if obj := pass.TypesInfo.Defs[info.TypeSpec.Name]; obj != nil {
			if strct := asStruct(obj.Type()); strct != nil {
				return buildSchemaFromStruct(pass, info.Name, strct), true
			}
		}
	}
	return buildSchemaFromAST(info)
}

func buildSchemaFromStruct(pass *analysis.Pass, typeName string, strct *types.Struct) Schema {
	schema := Schema{
		Name:       typeName,
		Type:       "object",
		Properties: make(map[string]Property),
	}

	for i := 0; i < strct.NumFields(); i++ {
		field := strct.Field(i)
		tag := reflect.StructTag(strct.Tag(i))

		if field.Anonymous() {
			mergeEmbeddedSchema(pass, &schema, field.Type())
			continue
		}
		if !field.Exported() {
			continue
		}

		jsonName, omitempty := parseJSONFieldTag(tag.Get("json"), field.Name())
		if jsonName == "-" {
			continue
		}

		prop := goTypeToOpenAPIProperty(pass, field.Type())
		applyValidateConstraints(&prop, tag.Get("validate"), field.Type())
		schema.Properties[jsonName] = prop

		if isFieldRequired(omitempty, tag.Get("validate")) {
			schema.Required = appendUniqueString(schema.Required, jsonName)
		}
	}

	return schema
}

func mergeEmbeddedSchema(pass *analysis.Pass, schema *Schema, typ types.Type) {
	embedded := buildSchemaFromGoType(pass, typ)
	if embedded.Type != "object" || len(embedded.Properties) == 0 {
		return
	}
	for name, prop := range embedded.Properties {
		if _, exists := schema.Properties[name]; !exists {
			schema.Properties[name] = prop
		}
	}
	for _, req := range embedded.Required {
		schema.Required = appendUniqueString(schema.Required, req)
	}
}

func buildSchemaFromGoType(pass *analysis.Pass, typ types.Type) Schema {
	if named := asNamed(typ); named != nil && named.Obj() != nil {
		if built, ok := buildNamedSchema(pass, named.Obj().Name()); ok {
			return built
		}
	}
	if strct := asStruct(typ); strct != nil {
		return buildSchemaFromStruct(pass, "", strct)
	}
	return Schema{}
}

func buildSchemaFromAST(info StructInfo) (Schema, bool) {
	if info.StructType == nil {
		return Schema{}, false
	}
	schema := Schema{
		Name:       info.Name,
		Type:       "object",
		Properties: make(map[string]Property),
	}
	for _, field := range info.StructType.Fields.List {
		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			tag := ""
			if field.Tag != nil {
				tag = field.Tag.Value
			}
			jsonTag := extractStructTagValue(tag, "json")
			jsonName, omitempty := parseJSONFieldTag(jsonTag, name.Name)
			if jsonName == "-" {
				continue
			}
			validateTag := extractStructTagValue(tag, "validate")
			prop := Property{Type: "string"}
			applyValidateConstraints(&prop, validateTag, nil)
			schema.Properties[jsonName] = prop
			if isFieldRequired(omitempty, validateTag) {
				schema.Required = appendUniqueString(schema.Required, jsonName)
			}
		}
	}
	return schema, len(schema.Properties) > 0
}

func goTypeToOpenAPIProperty(pass *analysis.Pass, typ types.Type) Property {
	if typ == nil {
		return Property{Type: "string"}
	}
	if ptr, ok := typ.(*types.Pointer); ok {
		return goTypeToOpenAPIProperty(pass, ptr.Elem())
	}

	switch t := typ.(type) {
	case *types.Basic:
		return basicTypeToProperty(t)
	case *types.Slice:
		if elem, ok := t.Elem().(*types.Basic); ok && elem.Kind() == types.Byte {
			return Property{Type: "string", Format: "byte"}
		}
		itemSchema := schemaForElementType(pass, t.Elem())
		return Property{Type: "array", Items: &itemSchema}
	case *types.Array:
		itemSchema := schemaForElementType(pass, t.Elem())
		return Property{Type: "array", Items: &itemSchema}
	case *types.Map:
		return Property{Type: "object"}
	case *types.Named:
		return namedTypeToProperty(t)
	case *types.Struct:
		return Property{Type: "object"}
	default:
		return Property{Type: "string"}
	}
}

func schemaForElementType(pass *analysis.Pass, elem types.Type) Schema {
	if ptr, ok := elem.(*types.Pointer); ok {
		elem = ptr.Elem()
	}
	if basic, ok := elem.(*types.Basic); ok {
		prop := basicTypeToProperty(basic)
		return propertyToItemSchema(prop)
	}
	if named, ok := elem.(*types.Named); ok {
		prop := namedTypeToProperty(named)
		return propertyToItemSchema(prop)
	}
	prop := goTypeToOpenAPIProperty(pass, elem)
	return propertyToItemSchema(prop)
}

func propertyToItemSchema(prop Property) Schema {
	if prop.Ref != "" {
		return Schema{Ref: prop.Ref}
	}
	return Schema{Type: prop.Type, Format: prop.Format}
}

func namedTypeToProperty(named *types.Named) Property {
	if named == nil || named.Obj() == nil {
		return Property{Type: "string"}
	}
	name := named.Obj().Name()
	if named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "time" && name == "Time" {
		return Property{Type: "string", Format: "date-time"}
	}
	return Property{Ref: "#/components/schemas/" + name}
}

func basicTypeToProperty(t *types.Basic) Property {
	switch t.Kind() {
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
		return Property{Type: "integer"}
	case types.Float32, types.Float64:
		return Property{Type: "number", Format: "float"}
	case types.Bool:
		return Property{Type: "boolean"}
	case types.String:
		return Property{Type: "string"}
	default:
		return Property{Type: "string"}
	}
}

func applyValidateConstraints(prop *Property, validateTag string, fieldType types.Type) {
	if validateTag == "" {
		return
	}
	for _, rule := range strings.Split(validateTag, ",") {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		parts := strings.SplitN(rule, "=", 2)
		switch parts[0] {
		case "required":
			// handled at schema level via isFieldRequired
		case "email":
			prop.Format = "email"
		case "url":
			prop.Format = "uri"
		case "uuid":
			prop.Format = "uuid"
		case "min":
			if len(parts) != 2 {
				continue
			}
			applyMinConstraint(prop, fieldType, parts[1])
		case "max":
			if len(parts) != 2 {
				continue
			}
			applyMaxConstraint(prop, fieldType, parts[1])
		case "oneof":
			if len(parts) == 2 {
				prop.Enum = strings.Split(parts[1], " ")
			}
		}
	}
}

func applyMinConstraint(prop *Property, fieldType types.Type, raw string) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return
	}
	if isStringKind(fieldType) {
		n := int(v)
		prop.MinLength = &n
		return
	}
	prop.Minimum = &v
}

func applyMaxConstraint(prop *Property, fieldType types.Type, raw string) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return
	}
	if isStringKind(fieldType) {
		n := int(v)
		prop.MaxLength = &n
		return
	}
	prop.Maximum = &v
}

func isStringKind(fieldType types.Type) bool {
	if fieldType == nil {
		return false
	}
	if basic, ok := fieldType.Underlying().(*types.Basic); ok {
		return basic.Kind() == types.String
	}
	return false
}

func parseJSONFieldTag(jsonTag, fieldName string) (jsonName string, omitempty bool) {
	jsonName = fieldName
	if jsonTag == "" {
		return jsonName, false
	}
	parts := strings.Split(jsonTag, ",")
	if parts[0] != "" {
		jsonName = parts[0]
	}
	for _, part := range parts[1:] {
		if part == "omitempty" {
			omitempty = true
		}
	}
	return jsonName, omitempty
}

func isFieldRequired(omitempty bool, validateTag string) bool {
	if strings.Contains(validateTag, "required") {
		return true
	}
	return !omitempty
}

func extractStructTagValue(tagLiteral, key string) string {
	if tagLiteral == "" {
		return ""
	}
	tag := strings.Trim(tagLiteral, "`")
	return reflect.StructTag(tag).Get(key)
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// buildSchema is the Pass entry point for a type name lookup.
func buildSchema(pass *analysis.Pass, typeName string) Schema {
	schema, ok := buildNamedSchema(pass, typeName)
	if !ok {
		return Schema{}
	}
	return schema
}
