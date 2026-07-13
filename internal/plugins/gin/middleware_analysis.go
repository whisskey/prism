// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package gin

import (
	"go/ast"
	"path/filepath"
	"strings"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/internal/typecheck"
	"github.com/whisskey/prism/pkg/openapi"
)

func (p *Plugin) analyzeMiddlewareChain(exprs []ast.Expr, pf *parser.ParsedFile) []MiddlewareAnalysis {
	if len(exprs) == 0 {
		return nil
	}
	analyses := make([]MiddlewareAnalysis, 0, len(exprs))
	for _, expr := range exprs {
		statuses := p.configuredMiddlewareStatuses(expr)
		statuses = append(statuses, p.middlewareStatuses(expr)...)
		configuredErrors := make([]string, 0, len(statuses))
		for _, status := range statuses {
			if isErrorHTTPStatus(status) {
				configuredErrors = append(configuredErrors, status)
			}
		}

		name := middlewareNameFromExpr(expr)
		funcDecl := p.resolveMiddlewareFuncDecl(expr, pf)
		analysis := p.analyzeMiddlewareFromDecl(name, funcDecl)
		if schema := p.middlewareRequestSchema(expr); schema != "" {
			analysis.ValidatorSchema = schema
		}
		analysis.ErrorStatuses = mergeStatusCodes(analysis.ErrorStatuses, configuredErrors...)
		analyses = append(analyses, analysis)
	}
	return analyses
}

func (p *Plugin) analyzeNamedMiddlewareChain(refs []openapi.MiddlewareRef) []MiddlewareAnalysis {
	if len(refs) == 0 {
		return nil
	}
	analyses := make([]MiddlewareAnalysis, 0, len(refs))
	for _, ref := range refs {
		analyses = append(analyses, p.analyzeMiddlewareFromDecl(ref.Name, p.resolveMiddlewareFuncByName(ref.Name)))
	}
	return analyses
}

func (p *Plugin) analyzeMiddlewareFromDecl(name string, funcDecl *ast.FuncDecl) MiddlewareAnalysis {
	analysis := MiddlewareAnalysis{Name: name, FuncDecl: funcDecl}
	if funcDecl == nil {
		return analysis
	}
	analysis.ValidatorSchema = p.middlewareBodySchemaFromFuncDecl(funcDecl)
	analysis.ErrorStatuses = mergeStatusCodes(
		writeHeaderErrorStatusesFromFunc(funcDecl),
		p.collectAppErrorStatusesFromFunc(funcDecl, make(map[*ast.FuncDecl]bool))...,
	)
	return analysis
}

func mergeStatusCodes(existing []string, extra ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(extra))
	merged := make([]string, 0, len(existing)+len(extra))
	for _, status := range existing {
		if _, ok := seen[status]; ok {
			continue
		}
		seen[status] = struct{}{}
		merged = append(merged, status)
	}
	for _, status := range extra {
		if _, ok := seen[status]; ok {
			continue
		}
		seen[status] = struct{}{}
		merged = append(merged, status)
	}
	return merged
}

func (p *Plugin) resolveMiddlewareFuncDecl(expr ast.Expr, pf *parser.ParsedFile) *ast.FuncDecl {
	if expr == nil {
		return nil
	}
	if pf != nil && p.typeProgram != nil {
		path := pf.Path
		if abs, err := filepath.Abs(pf.Path); err == nil {
			path = abs
		}
		pass := p.typeProgram.PassForFile(path)
		call, ok := expr.(*ast.CallExpr)
		if !ok {
			// JWTAuth as bare ident / With(RequireRole("admin")) already CallExpr
			if fn := p.resolveMiddlewareFuncByName(middlewareNameFromExpr(expr)); fn != nil {
				return fn
			}
		} else if pass != nil {
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
			pos := typecheck.ExprPosition{}
			if pf.FileSet != nil {
				ppos := pf.FileSet.Position(call.Pos())
				pos = typecheck.ExprPosition{Line: ppos.Line, Column: ppos.Column}
			}
			if fn := p.typeProgram.FuncDeclForCall(path, pos); fn != nil {
				return fn
			}
		}
	}
	return p.resolveMiddlewareFuncByName(middlewareNameFromExpr(expr))
}

func (p *Plugin) resolveMiddlewareFuncByName(name string) *ast.FuncDecl {
	if name == "" {
		return nil
	}
	if fn, ok := p.routeFuncIndex[name]; ok {
		return fn
	}
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		if fn, ok := p.routeFuncIndex[name[idx+1:]]; ok {
			return fn
		}
	}
	if p.typeProgram != nil {
		short := name
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			short = name[idx+1:]
		}
		return p.typeProgram.FuncDeclForName(short)
	}
	return nil
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
			existing.ErrorStatuses = mergeStatusCodes(existing.ErrorStatuses, item.ErrorStatuses...)
			byName[item.Name] = existing
		}
	}
	merged := make([]MiddlewareAnalysis, 0, len(order))
	for _, name := range order {
		merged = append(merged, byName[name])
	}
	return merged
}

func (p *Plugin) applyNamedMiddlewareInference(route *openapi.Route, analyses []MiddlewareAnalysis) {
	if route == nil {
		return
	}
	if route.Responses == nil {
		route.Responses = make(map[string]openapi.Response)
	}
	for _, mw := range analyses {
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
