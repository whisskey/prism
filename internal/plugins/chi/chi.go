// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

// Package chi provides a plugin for extracting routes from chi router applications.
package chi

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

	"github.com/whisskey/prism/internal/config"
	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/internal/plugins"
	"github.com/whisskey/prism/internal/scanner"
	"github.com/whisskey/prism/internal/schema"
	"github.com/whisskey/prism/internal/typecheck"
	"github.com/whisskey/prism/pkg/openapi"
)

// chiImportPaths are the known import paths for chi router.
var chiImportPaths = []string{
	"github.com/go-chi/chi",
	"github.com/go-chi/chi/v5",
}

// httpMethods are the HTTP methods supported by chi router.
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
}

// Plugin implements the FrameworkPlugin interface for chi router.
type Plugin struct {
	goParser          *parser.GoParser
	schemaExtractor   *schema.GoSchemaExtractor
	handlerIndex      map[string]*ast.FuncDecl
	routeFuncIndex    map[string]*ast.FuncDecl
	returnTypeIndex   map[string]string
	structIndex       map[string]parser.StructDefinition
	parsedFileIndex   map[string]*parser.ParsedFile
	handlerFileIndex  map[*ast.FuncDecl]*parser.ParsedFile
	requestPathFields map[string]map[string]bool
	usedSchemas       map[string]bool
	filePaths         []string
	typeProgram       *typecheck.Program
	responseHelpers   map[*ast.FuncDecl]responseHelper
	inference         config.InferenceConfig
	routes            config.RoutesConfig
}

// New creates a new chi plugin instance.
func New() *Plugin {
	return &Plugin{
		goParser:        parser.NewGoParser(),
		schemaExtractor: schema.NewGoSchemaExtractor(),
	}
}

// Name returns the plugin identifier.
func (p *Plugin) Name() string {
	return "chi"
}

// Extensions returns the file extensions this plugin handles.
func (p *Plugin) Extensions() []string {
	return []string{".go"}
}

// Info returns plugin metadata.
func (p *Plugin) Info() plugins.PluginInfo {
	return plugins.PluginInfo{
		Name:        "chi",
		Version:     "1.0.0",
		Description: "Extracts routes from chi router applications",
		SupportedFrameworks: []string{
			"github.com/go-chi/chi",
			"github.com/go-chi/chi/v5",
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

// Detect checks if chi is used in the project by looking at go.mod.
func (p *Plugin) Detect(projectRoot string) (bool, error) {
	goModPath := filepath.Join(projectRoot, "go.mod")

	file, err := os.Open(goModPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to open go.mod: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		for _, importPath := range chiImportPaths {
			if strings.Contains(line, importPath) {
				return true, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("failed to read go.mod: %w", err)
	}

	return false, nil
}

// ExtractRoutes parses source files and extracts chi route definitions.
func (p *Plugin) ExtractRoutes(files []scanner.SourceFile) ([]openapi.Route, error) {
	bundles := p.discoverRouteBundles(files)
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

// hasChiImport checks if the file imports chi.
func (p *Plugin) hasChiImport(pf *parser.ParsedFile) bool {
	for _, importPath := range chiImportPaths {
		if p.goParser.HasImport(pf, importPath) {
			return true
		}
	}
	return false
}

// ExtractSchemas extracts schema definitions from Go structs.
func (p *Plugin) ExtractSchemas(files []scanner.SourceFile) ([]openapi.Schema, error) {
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
	}

	if len(p.usedSchemas) > 0 {
		p.expandUsedSchemas()
		return p.filteredSchemas(), nil
	}

	return p.schemaExtractor.Registry().ToSlice(), nil
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

// pathParamRegex matches chi path parameters like {id} or {userId}.
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

		// Check for regex pattern in param (e.g., {id:[0-9]+})
		if idx := strings.Index(paramName, ":"); idx > 0 {
			paramName = paramName[:idx]
		}

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

// Register registers the chi plugin with the global registry.
func Register() {
	plugins.MustRegister(New())
}

func init() {
	Register()
}
