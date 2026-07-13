// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

// Package gin provides a plugin for extracting routes from Gin framework applications.
package gin

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

var ginImportPaths = []string{
	"github.com/gin-gonic/gin",
}

var httpMethods = map[string]bool{
	"GET":     true,
	"POST":    true,
	"PUT":     true,
	"DELETE":  true,
	"PATCH":   true,
	"HEAD":    true,
	"OPTIONS": true,
	"Any":     true,
	"Handle":  true,
}

// Plugin implements the FrameworkPlugin interface for Gin framework.
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

// New creates a new Gin plugin instance.
func New() *Plugin {
	return &Plugin{
		goParser:        parser.NewGoParser(),
		schemaExtractor: schema.NewGoSchemaExtractor(),
	}
}

func (p *Plugin) Name() string { return "gin" }

func (p *Plugin) Extensions() []string { return []string{".go"} }

func (p *Plugin) Info() plugins.PluginInfo {
	return plugins.PluginInfo{
		Name:        "gin",
		Version:     "1.0.0",
		Description: "Extracts routes from Gin framework applications",
		SupportedFrameworks: []string{
			"github.com/gin-gonic/gin",
		},
	}
}

func (p *Plugin) Configure(cfg *config.Config) {
	if cfg == nil {
		return
	}
	p.inference = cfg.Inference
	p.routes = cfg.Routes
}

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

	scanr := bufio.NewScanner(file)
	for scanr.Scan() {
		line := scanr.Text()
		for _, importPath := range ginImportPaths {
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

func (p *Plugin) ExtractRoutes(files []scanner.SourceFile) ([]openapi.Route, error) {
	bundles := p.discoverRouteBundles(files)
	p.enrichRouteBundles(bundles)
	return routeBundlesToRoutes(bundles), nil
}

func (p *Plugin) extractHandlerName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if ident, ok := e.X.(*ast.Ident); ok {
			return ident.Name + "." + e.Sel.Name
		}
		return e.Sel.Name
	case *ast.FuncLit:
		return "<anonymous>"
	case *ast.CallExpr:
		return "<wrapped>"
	default:
		return ""
	}
}

func (p *Plugin) hasGinImport(pf *parser.ParsedFile) bool {
	for _, importPath := range ginImportPaths {
		if p.goParser.HasImport(pf, importPath) {
			return true
		}
	}
	return false
}

func (p *Plugin) ExtractSchemas(files []scanner.SourceFile) ([]openapi.Schema, error) {
	for _, file := range files {
		if file.Language != "go" {
			continue
		}
		pf, err := p.goParser.ParseSource(file.Path, string(file.Content))
		if err != nil {
			continue
		}
		for _, def := range p.goParser.ExtractStructs(pf) {
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

func normalizePath(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}
	return path
}

func convertGinPathParams(path string) string {
	path = regexp.MustCompile(`:(\w+)`).ReplaceAllString(path, "{$1}")
	path = regexp.MustCompile(`\*(\w+)`).ReplaceAllString(path, "{$1}")
	return path
}

var pathParamRegex = regexp.MustCompile(`\{([^}]+)\}`)

func extractPathParams(path string) []openapi.Parameter {
	var params []openapi.Parameter
	matches := pathParamRegex.FindAllStringSubmatch(path, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		paramName := match[1]
		if idx := strings.Index(paramName, ":"); idx > 0 {
			paramName = paramName[:idx]
		}
		params = append(params, openapi.Parameter{
			Name:     paramName,
			In:       "path",
			Required: true,
			Schema:   &openapi.Schema{Type: "string"},
		})
	}
	return params
}

func generateOperationID(method, path, handler string) string {
	if handler != "" && handler != "<anonymous>" && handler != "<wrapped>" {
		parts := strings.Split(handler, ".")
		name := parts[len(parts)-1]
		return strings.ToLower(method) + name
	}
	path = pathParamRegex.ReplaceAllString(path, "By${1}")
	path = strings.ReplaceAll(path, "/", " ")
	path = strings.TrimSpace(path)
	words := strings.Fields(path)
	if len(words) == 0 {
		return strings.ToLower(method)
	}
	var sb strings.Builder
	sb.WriteString(strings.ToLower(method))
	titleCaser := cases.Title(language.English)
	for _, word := range words {
		word = titleCaser.String(strings.ToLower(word))
		sb.WriteString(word)
	}
	return sb.String()
}

func inferTags(path string) []string {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		return nil
	}
	skipPrefixes := map[string]bool{"api": true, "v1": true, "v2": true, "v3": true}
	var tagPart string
	for _, part := range parts {
		if part == "" || skipPrefixes[part] || strings.HasPrefix(part, "{") {
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

func Register() { plugins.MustRegister(New()) }

func init() { Register() }
