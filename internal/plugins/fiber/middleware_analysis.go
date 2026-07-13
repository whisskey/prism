// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"
	"path/filepath"
	"strings"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/internal/typecheck"
	"github.com/whisskey/prism/pkg/openapi"
)

func (p *Plugin) resolveMiddlewareFuncDecl(expr ast.Expr, pf *parser.ParsedFile) *ast.FuncDecl {
	if expr == nil {
		return nil
	}
	if pf != nil && p.typeProgram != nil {
		path := p.currentFilePath(pf)
		pass := p.typeProgram.PassForFile(path)
		if pass != nil {
			if call, ok := expr.(*ast.CallExpr); ok {
				switch fun := call.Fun.(type) {
				case *ast.SelectorExpr:
					if sel := pass.Selection(fun); sel != nil {
						if fn := p.typeProgram.FuncDeclForObject(sel.Obj()); fn != nil {
							return fn
						}
					}
				case *ast.Ident:
					if obj := pass.Use(fun); obj != nil {
						if fn := p.typeProgram.FuncDeclForObject(obj); fn != nil {
							return fn
						}
					}
				}
				if fn := p.typeProgram.FuncDeclForCall(path, p.exprPosition(pf, call)); fn != nil {
					return fn
				}
			}
		}
	}
	return p.resolveMiddlewareFuncByName(middlewareNameFromExpr(expr))
}

func (p *Plugin) resolveMiddlewareFuncByName(name string) *ast.FuncDecl {
	if fn := p.resolveMiddlewareFunc(name); fn != nil {
		return fn
	}
	if p.typeProgram == nil || name == "" {
		return nil
	}
	shortName := name
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		shortName = name[idx+1:]
	}
	return p.typeProgram.FuncDeclForName(shortName)
}

func (p *Plugin) analyzeMiddlewareFromDecl(name string, funcDecl *ast.FuncDecl) MiddlewareAnalysis {
	analysis := MiddlewareAnalysis{Name: name, FuncDecl: funcDecl}
	if funcDecl == nil {
		return analysis
	}
	analysis.ValidatorSchema = p.middlewareBodySchemaFromFuncDecl(funcDecl)
	analysis.ErrorStatuses = mergeStatusCodes(
		middlewareErrorStatusesFromFuncDecl(funcDecl),
		p.collectAppErrorStatusesFromFunc(funcDecl, make(map[*ast.FuncDecl]bool))...,
	)
	return analysis
}

func (p *Plugin) analyzeNamedMiddleware(name string) MiddlewareAnalysis {
	return p.analyzeMiddlewareFromDecl(name, p.resolveMiddlewareFuncByName(name))
}

func (p *Plugin) analyzeNamedMiddlewareRef(ref openapi.MiddlewareRef) MiddlewareAnalysis {
	funcDecl := p.resolveMiddlewareFuncByName(ref.Name)
	if funcDecl == nil && ref.SourceFile != "" && ref.SourceLine > 0 && p.typeProgram != nil {
		path, err := filepath.Abs(ref.SourceFile)
		if err != nil {
			path = ref.SourceFile
		}
		funcDecl = p.typeProgram.FuncDeclForCall(path, typecheck.ExprPosition{Line: ref.SourceLine})
	}
	return p.analyzeMiddlewareFromDecl(ref.Name, funcDecl)
}

func (p *Plugin) analyzeNamedMiddlewareChain(refs []openapi.MiddlewareRef) []MiddlewareAnalysis {
	if len(refs) == 0 {
		return nil
	}
	analyses := make([]MiddlewareAnalysis, 0, len(refs))
	for _, ref := range refs {
		analyses = append(analyses, p.analyzeNamedMiddlewareRef(ref))
	}
	return analyses
}

func mergeMiddlewareAnalyses(chains ...[]MiddlewareAnalysis) []MiddlewareAnalysis {
	byName := make(map[string]MiddlewareAnalysis)
	order := make([]string, 0)
	for _, chain := range chains {
		for _, item := range chain {
			if item.Name == "" {
				continue
			}
			existing, ok := byName[item.Name]
			if !ok {
				byName[item.Name] = item
				order = append(order, item.Name)
				continue
			}
			if existing.FuncDecl == nil {
				existing.FuncDecl = item.FuncDecl
			}
			if existing.ValidatorSchema == "" {
				existing.ValidatorSchema = item.ValidatorSchema
			}
			existing.ErrorStatuses = mergeErrorStatuses(existing.ErrorStatuses, item.ErrorStatuses)
			byName[item.Name] = existing
		}
	}
	merged := make([]MiddlewareAnalysis, 0, len(order))
	for _, name := range order {
		merged = append(merged, byName[name])
	}
	return merged
}

func (p *Plugin) middlewareBodySchemaFromFuncDecl(funcDecl *ast.FuncDecl) string {
	if funcDecl == nil || funcDecl.Body == nil {
		return ""
	}

	localTypes := make(map[string]string)
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.DeclStmt:
			p.collectDeclTypes(node, localTypes)
		case *ast.AssignStmt:
			p.collectAssignTypes(node, localTypes)
		}
		return true
	})

	var schemaName string
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		if schemaName != "" {
			return false
		}
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if name := p.schemaFromValidatorCall(callExpr, localTypes); name != "" {
			schemaName = name
			return false
		}
		return true
	})
	return schemaName
}

func (p *Plugin) schemaFromValidatorCall(callExpr *ast.CallExpr, localTypes map[string]string) string {
	if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
		switch selExpr.Sel.Name {
		case "BodyParser", "Body":
			if len(callExpr.Args) > 0 {
				return cleanTypeName(p.schemaNameFromExpr(callExpr.Args[0], localTypes))
			}
		case "Unmarshal":
			if len(callExpr.Args) >= 2 {
				return cleanTypeName(p.schemaNameFromExpr(callExpr.Args[1], localTypes))
			}
		}
	}
	switch callNameFromExpr(callExpr) {
	case "bindAndValidate", "bindBody":
		if len(callExpr.Args) >= 2 {
			if name := cleanTypeName(p.schemaNameFromExpr(callExpr.Args[1], localTypes)); name != "" {
				return name
			}
		}
		if len(callExpr.Args) >= 1 {
			return cleanTypeName(p.schemaNameFromExpr(callExpr.Args[0], localTypes))
		}
	}
	return ""
}

func middlewareErrorStatusesFromFuncDecl(funcDecl *ast.FuncDecl) []string {
	if funcDecl == nil || funcDecl.Body == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var statuses []string
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, code := range errorStatusesFromMiddlewareCallExtended(callExpr) {
			if _, exists := seen[code]; exists {
				continue
			}
			seen[code] = struct{}{}
			statuses = append(statuses, code)
		}
		return true
	})
	return statuses
}

func errorStatusesFromMiddlewareCall(call *ast.CallExpr) []string {
	var codes []string
	if code := statusCodeFromMiddlewareCall(call); code != "" && isErrorHTTPStatus(code) {
		codes = append(codes, code)
	}
	name := callNameFromExpr(call)
	switch {
	case strings.Contains(name, "Unauthorized"):
		codes = append(codes, "401")
	case strings.Contains(name, "Forbidden"):
		codes = append(codes, "403")
	case strings.Contains(name, "BadRequest"):
		codes = append(codes, "400")
	case strings.Contains(name, "NotFound"):
		codes = append(codes, "404")
	case strings.Contains(name, "InternalServerError"):
		codes = append(codes, "500")
	}
	return codes
}

func statusCodeFromMiddlewareCall(call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if sel.Sel.Name != "Status" || len(call.Args) == 0 {
		return ""
	}
	return statusCodeFromExpr(call.Args[0])
}

func (p *Plugin) applyNamedMiddlewareInference(route *openapi.Route, analyses []MiddlewareAnalysis) {
	if route == nil {
		return
	}
	for _, mw := range analyses {
		if mw.ValidatorSchema != "" && route.RequestBody == nil && methodAllowsRequestBody(route.Method) {
			schemaName := cleanTypeName(mw.ValidatorSchema)
			if schemaName != "" {
				p.markSchemaUsed(schemaName)
				route.RequestBody = jsonRequestBody(schemaName)
				p.rememberRequestPathFields(schemaName, route.Parameters)
			}
		}
		if route.Responses == nil {
			route.Responses = make(map[string]openapi.Response)
		}
		for _, status := range mw.ErrorStatuses {
			if !isErrorHTTPStatus(status) {
				continue
			}
			if _, exists := route.Responses[status]; exists {
				continue
			}
			response := p.errorResponseFromFunc(mw.FuncDecl, status)
			route.Responses[status] = jsonResponseWithExample(responseDescription(status), response.schema, response.example)
			p.markSchemaRefs(response.schema)
		}
	}
}
