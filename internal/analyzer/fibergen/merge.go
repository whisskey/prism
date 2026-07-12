// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"strings"
)

// MergedData aggregates fibergen output across all loaded packages.
type MergedData struct {
	Title           string
	Version         string
	Routes          []Route
	Handlers        map[string]HandlerDetail
	Schemas         map[string]Schema
	Security        map[string][]SecurityRequirement
	SecuritySchemes map[string]SecurityScheme
	Warnings        []Warning
}

// Catalog indexes fibergen results for cross-package lookup.
type Catalog struct {
	Merged *MergedData
	Calls  map[string]CallInfo
}

// MergeResults combines per-package fibergen results into one catalog.
func MergeResults(results []*Result) *Catalog {
	merged := &MergedData{
		Handlers:        make(map[string]HandlerDetail),
		Schemas:         make(map[string]Schema),
		Security:        make(map[string][]SecurityRequirement),
		SecuritySchemes: make(map[string]SecurityScheme),
	}

	routeKeys := make(map[string]struct{})
	for _, result := range results {
		if result == nil || result.Data == nil {
			continue
		}
		data := result.Data

		if merged.Title == "" && data.OpenAPI.Title != "" {
			merged.Title = data.OpenAPI.Title
		}
		if merged.Version == "" && data.OpenAPI.Version != "" {
			merged.Version = data.OpenAPI.Version
		}

		for _, route := range data.Routes {
			key := routeKey(route.Method, route.Path)
			if _, exists := routeKeys[key]; exists {
				continue
			}
			routeKeys[key] = struct{}{}
			merged.Routes = append(merged.Routes, route)
		}

		for name, detail := range data.Handlers {
			mergeHandlerDetail(merged.Handlers, name, detail)
		}
		for _, route := range data.Routes {
			if route.Handler == "" {
				continue
			}
			key := handlerLookupKey(route.Handler)
			if detail, ok := data.Handlers[key]; ok {
				mergeHandlerDetail(merged.Handlers, route.Handler, detail)
				mergeHandlerDetail(merged.Handlers, key, detail)
			}
		}

		for name, schema := range data.Schemas {
			if existing, ok := merged.Schemas[name]; !ok || len(schema.Properties) > len(existing.Properties) {
				merged.Schemas[name] = schema
			}
		}

		for key, reqs := range data.Security {
			if _, exists := merged.Security[key]; !exists {
				merged.Security[key] = reqs
			}
		}
		for name, scheme := range data.SecuritySchemes {
			merged.SecuritySchemes[name] = scheme
		}

		merged.Warnings = append(merged.Warnings, data.Warnings...)
	}

	return &Catalog{
		Merged: merged,
		Calls:  NewIndex(results).Calls,
	}
}

// NewCatalog builds a catalog from fibergen analyzer results.
func NewCatalog(results []*Result) *Catalog {
	if len(results) == 0 {
		return &Catalog{Merged: &MergedData{}, Calls: map[string]CallInfo{}}
	}
	return MergeResults(results)
}

// LookupHandler finds handler analysis by qualified or short name.
func (m *MergedData) LookupHandler(candidates ...string) (HandlerDetail, bool) {
	if m == nil || len(m.Handlers) == 0 {
		return HandlerDetail{}, false
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if detail, ok := m.Handlers[candidate]; ok {
			return detail, true
		}
		if detail, ok := m.Handlers[handlerLookupKey(candidate)]; ok {
			return detail, true
		}
	}
	return HandlerDetail{}, false
}

func routeKey(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

func mergeHandlerDetail(index map[string]HandlerDetail, key string, detail HandlerDetail) {
	if key == "" {
		return
	}
	if existing, ok := index[key]; ok {
		index[key] = mergeHandlerDetails(existing, detail)
		return
	}
	index[key] = detail
}

func mergeHandlerDetails(a, b HandlerDetail) HandlerDetail {
	out := a
	if out.RequestBody == "" {
		out.RequestBody = b.RequestBody
	}
	if out.ResponseType == "" {
		out.ResponseType = b.ResponseType
	}
	if len(out.ResponseSchemas) == 0 {
		out.ResponseSchemas = b.ResponseSchemas
	} else if len(b.ResponseSchemas) > 0 {
		if out.ResponseSchemas == nil {
			out.ResponseSchemas = make(map[string]string)
		}
		for code, schema := range b.ResponseSchemas {
			if _, exists := out.ResponseSchemas[code]; !exists {
				out.ResponseSchemas[code] = schema
			}
		}
	}
	if len(out.PathParams) == 0 {
		out.PathParams = b.PathParams
	}
	if len(out.QueryParams) == 0 {
		out.QueryParams = b.QueryParams
	}
	if out.SourceFile == "" {
		out.SourceFile = b.SourceFile
		out.SourceLine = b.SourceLine
	}
	return out
}
