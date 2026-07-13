// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"
	"strings"

	"github.com/whisskey/prism/internal/parser"
)

type requestHintKind string

const (
	requestHintBodyParser     requestHintKind = "body_parser"
	requestHintLocalsPayload  requestHintKind = "locals_payload"
)

type requestHint struct {
	Kind       requestHintKind
	SchemaName string
}

// HandlerAnalysis holds raw handler AST findings (Layer 2 output).
type HandlerAnalysis struct {
	FuncDecl     *ast.FuncDecl
	Summary      string
	Description  string
	Doc          *parser.DocAnnotations
	ReturnCalls  []*ast.CallExpr
	RequestHints []requestHint
	ParamCalls   []*ast.CallExpr
	QueryCalls       []*ast.CallExpr
	QueryBindSchemas []string
	ErrorCalls       []*ast.CallExpr
	ErrorStatuses  []string
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

func (p *Plugin) collectHandlerAnalysis(funcDecl *ast.FuncDecl) HandlerAnalysis {
	analysis := HandlerAnalysis{FuncDecl: funcDecl}
	pf := p.handlerFileIndex[funcDecl]
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

	localTypes := make(map[string]string)
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.DeclStmt:
			p.collectDeclTypes(node, localTypes)
		case *ast.AssignStmt:
			p.collectAssignTypes(node, localTypes)
			if hint := requestHintFromAssign(node); hint.SchemaName != "" {
				analysis.RequestHints = append(analysis.RequestHints, hint)
			}
			if schemaName := querySchemaFromAssign(node, p, pf); schemaName != "" {
				analysis.QueryBindSchemas = appendUniqueSchemaNames(analysis.QueryBindSchemas, schemaName)
			}
		case *ast.ReturnStmt:
			for _, result := range node.Results {
				callExpr := callExprFromReturnResult(result)
				if callExpr == nil {
					continue
				}
				analysis.ReturnCalls = append(analysis.ReturnCalls, callExpr)
				if isExceptionCall(callExpr, "InternalServerError") || exceptionStatusFromCall(callExpr) == "500" {
					analysis.ErrorCalls = append(analysis.ErrorCalls, callExpr)
				}
			}
		case *ast.CallExpr:
			if schemaName := p.requestSchemaFromBodyParser(node, localTypes); schemaName != "" {
				analysis.RequestHints = append(analysis.RequestHints, requestHint{
					Kind:       requestHintBodyParser,
					SchemaName: schemaName,
				})
			}
			if schemaName := p.requestSchemaFromBodyBind(node, pf, localTypes); schemaName != "" {
				analysis.RequestHints = append(analysis.RequestHints, requestHint{
					Kind:       requestHintBodyParser,
					SchemaName: schemaName,
				})
			}
			if paramName, _ := paramTypeFromCall(node); paramName != "" {
				analysis.ParamCalls = append(analysis.ParamCalls, node)
			}
			if paramName, _ := queryParamFromCall(node); paramName != "" {
				analysis.QueryCalls = append(analysis.QueryCalls, node)
			}
			if schemaName := p.requestSchemaFromQueryBind(node, pf, localTypes); schemaName != "" {
				analysis.QueryBindSchemas = appendUniqueSchemaNames(analysis.QueryBindSchemas, schemaName)
			}
		}
		return true
	})

	analysis.ErrorStatuses = p.collectHandlerErrorStatuses(funcDecl)

	return analysis
}

func callExprFromReturnResult(expr ast.Expr) *ast.CallExpr {
	callExpr, ok := expr.(*ast.CallExpr)
	if ok {
		return callExpr
	}
	return nil
}

func requestHintFromAssign(stmt *ast.AssignStmt) requestHint {
	if len(stmt.Rhs) != 1 {
		return requestHint{}
	}

	typeAssert, ok := stmt.Rhs[0].(*ast.TypeAssertExpr)
	if !ok {
		return requestHint{}
	}

	localsCall, ok := typeAssert.X.(*ast.CallExpr)
	if !ok {
		return requestHint{}
	}

	selExpr, ok := localsCall.Fun.(*ast.SelectorExpr)
	if !ok || selExpr.Sel.Name != "Locals" || len(localsCall.Args) == 0 {
		return requestHint{}
	}

	key, ok := parser.ExtractStringLiteral(localsCall.Args[0])
	if !ok || !isLocalsPayloadKey(key) {
		return requestHint{}
	}

	schemaName := cleanTypeName(astTypeString(typeAssert.Type))
	if schemaName == "" {
		return requestHint{}
	}

	return requestHint{
		Kind:       requestHintLocalsPayload,
		SchemaName: schemaName,
	}
}

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
		analysis.ErrorStatuses = mergeErrorStatuses(analysis.ErrorStatuses, configuredErrors)
		analyses = append(analyses, analysis)
	}
	return analyses
}

func mergeErrorStatuses(existing, extra []string) []string {
	if len(extra) == 0 {
		return existing
	}
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

func (p *Plugin) analyzeRouteBundle(bundle *discoveredRouteBundle) RouteAnalysis {
	var handlerAnalysis *HandlerAnalysis
	if fn := p.resolveHandlerRef(bundle.Route.HandlerRef); fn != nil {
		analysis := p.collectHandlerAnalysis(fn)
		handlerAnalysis = &analysis
	}

	middlewareExprs := append([]ast.Expr{}, bundle.middlewareExprs...)
	pf := p.parsedFileIndex[bundle.Route.SourceFile]

	return RouteAnalysis{
		Handler: handlerAnalysis,
		Middleware: mergeMiddlewareAnalyses(
			p.analyzeMiddlewareChain(middlewareExprs, pf),
			p.analyzeNamedMiddlewareChain(bundle.Route.Middleware),
		),
	}
}
