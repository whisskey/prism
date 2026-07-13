// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package gin

import (
	"strings"

	"github.com/whisskey/prism/pkg/openapi"
)

func (p *Plugin) exampleFromSchema(schemaValue *openapi.Schema) interface{} {
	return p.exampleFromSchemaSeen(schemaValue, make(map[string]bool))
}

func (p *Plugin) exampleFromSchemaSeen(schemaValue *openapi.Schema, seen map[string]bool) interface{} {
	if schemaValue == nil {
		return nil
	}
	if schemaValue.Example != nil {
		return schemaValue.Example
	}
	if schemaValue.Ref != "" {
		name := strings.TrimPrefix(schemaValue.Ref, "#/components/schemas/")
		if name == "" || seen[name] || p.schemaExtractor == nil {
			return map[string]interface{}{}
		}
		refSchema, ok := p.schemaExtractor.Registry().Get(name)
		if !ok || refSchema == nil {
			return map[string]interface{}{}
		}
		seen[name] = true
		example := p.exampleFromSchemaSeen(refSchema, seen)
		delete(seen, name)
		return example
	}
	switch schemaValue.Type {
	case "array":
		itemExample := p.exampleFromSchemaSeen(schemaValue.Items, seen)
		if itemExample == nil {
			itemExample = "string"
		}
		return []interface{}{itemExample}
	case "object":
		if len(schemaValue.Properties) == 0 {
			return map[string]interface{}{}
		}
		example := make(map[string]interface{}, len(schemaValue.Properties))
		for name, prop := range schemaValue.Properties {
			if value := p.exampleFromSchemaSeen(prop, seen); value != nil {
				example[name] = value
			}
		}
		return example
	case "integer":
		return 0
	case "number":
		return 0
	case "boolean":
		return true
	case "string":
		if len(schemaValue.Enum) > 0 {
			return schemaValue.Enum[0]
		}
		switch schemaValue.Format {
		case "date-time":
			return "2026-06-18T23:22:25Z"
		case "date":
			return "2026-06-18"
		case "uuid":
			return "00000000-0000-0000-0000-000000000000"
		default:
			return "string"
		}
	default:
		return nil
	}
}
