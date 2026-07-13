// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package echo

import (
	"go/ast"
	"strings"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/pkg/openapi"
)

// HandlerAnalysis holds raw handler AST findings (Layer 2 output).
type HandlerAnalysis struct {
	FuncDecl         *ast.FuncDecl
	Summary          string
	Description      string
	Doc              *parser.DocAnnotations
	ErrorStatuses    []string
	QueryParams      []ParameterResolution
	RequestHints     []requestHint
	ParamCalls       []*ast.CallExpr
	ReturnCalls      []*ast.CallExpr
}

// MiddlewareAnalysis holds raw middleware AST findings (Layer 2 output).
type MiddlewareAnalysis struct {
	Name            string
	FuncDecl        *ast.FuncDecl
	ValidatorSchema string
	ErrorStatuses   []string
}

// RouteAnalysis combines per-route handler and middleware analysis.
type RouteAnalysis struct {
	Handler    *HandlerAnalysis
	Middleware []MiddlewareAnalysis
}

func (p *Plugin) analyzeRouteBundle(bundle *discoveredRouteBundle) RouteAnalysis {
	var handlerAnalysis *HandlerAnalysis
	if fn := p.resolveHandlerRef(bundle.Route.HandlerRef); fn != nil {
		analysis := p.collectHandlerAnalysis(fn)
		handlerAnalysis = &analysis
	}
	pf := p.parsedFileIndex[bundle.Route.SourceFile]
	return RouteAnalysis{
		Handler: handlerAnalysis,
		Middleware: mergeMiddlewareAnalyses(
			p.analyzeMiddlewareChain(bundle.middlewareExprs, pf),
			p.analyzeNamedMiddlewareChain(bundle.Route.Middleware),
		),
	}
}

func (p *Plugin) collectHandlerAnalysis(funcDecl *ast.FuncDecl) HandlerAnalysis {
	analysis := HandlerAnalysis{FuncDecl: funcDecl}
	if funcDecl == nil {
		return analysis
	}
	if funcDecl.Doc != nil {
		comment := strings.TrimSpace(funcDecl.Doc.Text())
		analysis.Doc = parser.ParseDocComment(comment)
		if analysis.Doc.Summary != "" {
			analysis.Summary = analysis.Doc.Summary
		} else if comment != "" {
			analysis.Summary = firstSentence(comment)
		}
		if analysis.Doc.Description != "" {
			analysis.Description = analysis.Doc.Description
		} else {
			analysis.Description = comment
		}
	}
	if funcDecl.Body == nil {
		return analysis
	}

	pf := p.handlerFileIndex[funcDecl]
	analysis.QueryParams = p.collectQueryParamsFromHandler(funcDecl)
	localTypes := make(map[string]string)
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.DeclStmt:
			p.collectDeclTypes(node, localTypes)
		case *ast.AssignStmt:
			p.collectAssignTypes(node, localTypes, pf)
			if hint := requestHintFromAssign(node); hint.SchemaName != "" {
				analysis.RequestHints = append(analysis.RequestHints, hint)
			}
		case *ast.CallExpr:
			if schemaName := p.requestSchemaFromBind(node, localTypes); schemaName != "" {
				analysis.RequestHints = append(analysis.RequestHints, requestHint{
					Kind:       requestHintBodyDecode,
					SchemaName: schemaName,
				})
			}
			if schemaName := p.requestSchemaFromBodyDecode(node, localTypes); schemaName != "" {
				analysis.RequestHints = append(analysis.RequestHints, requestHint{
					Kind:       requestHintBodyDecode,
					SchemaName: schemaName,
				})
			}
			if schemaName := p.requestSchemaFromUnmarshal(node, localTypes); schemaName != "" {
				analysis.RequestHints = append(analysis.RequestHints, requestHint{
					Kind:       requestHintBodyDecode,
					SchemaName: schemaName,
				})
			}
			if schemaName := p.requestSchemaFromRenderDecode(node, localTypes); schemaName != "" {
				analysis.RequestHints = append(analysis.RequestHints, requestHint{
					Kind:       requestHintBodyDecode,
					SchemaName: schemaName,
				})
			}
			if paramName, _ := paramTypeFromCall(node); paramName != "" {
				analysis.ParamCalls = append(analysis.ParamCalls, node)
			}
			if resp := p.responseFromHelperCall(node, localTypes, pf); resp != nil && resp.schema != nil {
				analysis.ReturnCalls = append(analysis.ReturnCalls, node)
			}
			if resp := p.responseFromJSONCall(node, localTypes, pf); resp != nil && resp.schema != nil {
				analysis.ReturnCalls = append(analysis.ReturnCalls, node)
			}
		}
		return true
	})
	analysis.ErrorStatuses = append(analysis.ErrorStatuses, p.collectAppErrorStatusesFromFunc(funcDecl, make(map[*ast.FuncDecl]bool))...)
	analysis.ErrorStatuses = append(analysis.ErrorStatuses, writeHeaderErrorStatusesFromFunc(funcDecl)...)
	return analysis
}

func (p *Plugin) resolveHandlerRef(ref openapi.HandlerRef) *ast.FuncDecl {
	if ref.Method != "" {
		if fn := p.handlerIndex[ref.Method]; fn != nil {
			return fn
		}
		if fn := p.routeFuncIndex[ref.Method]; fn != nil {
			return fn
		}
	}
	if ref.Qualified != "" {
		if fn := p.handlerIndex[ref.Qualified]; fn != nil {
			return fn
		}
		if fn := p.routeFuncIndex[ref.Qualified]; fn != nil {
			return fn
		}
		if idx := strings.LastIndex(ref.Qualified, "."); idx >= 0 {
			short := ref.Qualified[idx+1:]
			if fn := p.handlerIndex[short]; fn != nil {
				return fn
			}
			if fn := p.routeFuncIndex[short]; fn != nil {
				return fn
			}
		}
	}
	return nil
}

func (p *Plugin) applyRouteAnalysis(route *openapi.Route, analysis RouteAnalysis, middlewareExprs []ast.Expr) {
	if route == nil {
		return
	}
	if analysis.Handler != nil {
		if route.Summary == "" {
			route.Summary = analysis.Handler.Summary
		}
		if route.Description == "" {
			route.Description = analysis.Handler.Description
		}
	}
	graph := p.resolveRouteTypeGraph(route, analysis)
	p.applyRouteTypeGraph(route, graph)
	p.applyMiddlewareInference(route, middlewareExprs)
	p.applyAuthSecurity(route, middlewareExprs)
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
