// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// HandlerAnalyzerPass analyzes handler bodies for request/response types (Pass 2).
func HandlerAnalyzerPass(ctx Context, routes []Route) map[string]HandlerDetail {
	index := indexHandlers(ctx.Raw.Handlers)
	result := make(map[string]HandlerDetail)

	for _, route := range routes {
		key := handlerLookupKey(route.Handler)
		if _, exists := result[key]; exists {
			continue
		}
		if route.Handler == "" {
			continue
		}
		if handler, ok := index[key]; ok {
			detail := analyzeHandler(ctx, handler.FuncDecl, key, handler.Position.Filename, handler.Position.Line)
			warnMissingResponseType(ctx, detail)
			result[key] = detail
			continue
		}
		if fn := findFuncDecl(ctx.Pass, route.Handler); fn != nil {
			pos := ctx.Pass.Fset.Position(fn.Pos())
			detail := analyzeHandler(ctx, fn, key, pos.Filename, pos.Line)
			warnMissingResponseType(ctx, detail)
			result[key] = detail
			continue
		}
		warnHandlerNotFound(ctx, route)
		result[key] = HandlerDetail{
			Name:            key,
			ResponseSchemas: make(map[string]string),
			SourceFile:      route.SourceFile,
			SourceLine:      route.SourceLine,
		}
	}

	for _, handler := range ctx.Raw.Handlers {
		key := handlerLookupKey(handler.Name)
		if _, exists := result[key]; exists {
			continue
		}
		result[key] = analyzeHandler(ctx, handler.FuncDecl, key, handler.Position.Filename, handler.Position.Line)
		warnMissingResponseType(ctx, result[key])
	}

	return result
}

func indexHandlers(handlers []HandlerInfo) map[string]HandlerInfo {
	index := make(map[string]HandlerInfo, len(handlers))
	for _, handler := range handlers {
		index[handlerLookupKey(handler.Name)] = handler
	}
	return index
}

func findFuncDecl(pass *analysis.Pass, handlerRef string) *ast.FuncDecl {
	if pass == nil {
		return nil
	}
	methodName := handlerLookupKey(handlerRef)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name.Name != methodName {
				continue
			}
			if hasFiberCtxParam(pass, fn) {
				return fn
			}
		}
	}
	return nil
}

func analyzeHandler(ctx Context, funcDecl *ast.FuncDecl, name, sourceFile string, sourceLine int) HandlerDetail {
	info := HandlerDetail{
		Name:            name,
		ResponseSchemas: make(map[string]string),
		SourceFile:      sourceFile,
		SourceLine:      sourceLine,
	}
	if funcDecl == nil || funcDecl.Body == nil {
		return info
	}

	locals := make(map[string]string)
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.DeclStmt:
			collectLocalTypesFromDecl(ctx, node, locals)
		case *ast.AssignStmt:
			collectLocalTypesFromAssign(ctx, node, locals)
		case *ast.CallExpr:
			applyHandlerCall(ctx, node, locals, &info)
		}
		return true
	})

	return info
}

func applyHandlerCall(ctx Context, call *ast.CallExpr, locals map[string]string, info *HandlerDetail) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok {
		switch sel.Sel.Name {
		case "JSON":
			if len(call.Args) > 0 {
				pos := token.Position{}
				if ctx.Pass != nil && ctx.Pass.Fset != nil {
					pos = ctx.Pass.Fset.Position(call.Pos())
				}
				if typ := resolveSchemaTypeName(ctx, call.Args[0], locals, info.Name, "response", pos); typ != "" {
					info.ResponseType = typ
					info.ResponseSchemas["200"] = typ
				}
			}
			return
		case "BodyParser":
			if len(call.Args) > 0 {
				pos := token.Position{}
				if ctx.Pass != nil && ctx.Pass.Fset != nil {
					pos = ctx.Pass.Fset.Position(call.Pos())
				}
				if typ := resolveSchemaTypeName(ctx, call.Args[0], locals, info.Name, "request body", pos); typ != "" && info.RequestBody == "" {
					info.RequestBody = typ
				}
			}
			return
		case "Query":
			if len(call.Args) > 0 {
				if name, ok := stringLiteral(call.Args[0]); ok {
					info.QueryParams = appendUniqueParam(info.QueryParams, Param{
						Name:   name,
						GoType: "string",
					})
				}
			}
			return
		case "QueryInt", "QueryInt64", "QueryFloat", "QueryBool":
			if len(call.Args) > 0 {
				if name, ok := stringLiteral(call.Args[0]); ok {
					info.QueryParams = appendUniqueParam(info.QueryParams, Param{
						Name:   name,
						GoType: queryIntGoType(sel.Sel.Name),
					})
				}
			}
			return
		case "Params":
			if len(call.Args) > 0 {
				if name, ok := stringLiteral(call.Args[0]); ok {
					info.PathParams = appendUniqueParam(info.PathParams, Param{
						Name:     name,
						GoType:   "string",
						Required: true,
					})
				}
			}
			return
		case "ParamsInt", "ParamsInt64":
			if len(call.Args) > 0 {
				if name, ok := stringLiteral(call.Args[0]); ok {
					info.PathParams = appendUniqueParam(info.PathParams, Param{
						Name:     name,
						GoType:   "integer",
						Required: true,
					})
				}
			}
			return
		}
	}

	if schema := requestSchemaFromBindCall(ctx, call, locals); schema != "" && info.RequestBody == "" {
		info.RequestBody = schema
	}
	if code, schema := responseFromHelperCall(ctx, call, locals); code != "" && schema != "" {
		info.ResponseSchemas[code] = schema
		if info.ResponseType == "" {
			info.ResponseType = schema
		}
	}
}

func queryIntGoType(method string) string {
	switch method {
	case "QueryBool":
		return "boolean"
	case "QueryFloat":
		return "number"
	default:
		return "integer"
	}
}

func collectLocalTypesFromDecl(ctx Context, stmt *ast.DeclStmt, locals map[string]string) {
	genDecl, ok := stmt.Decl.(*ast.GenDecl)
	if !ok {
		return
	}
	for _, spec := range genDecl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		typeName := typeExprName(ctx, valueSpec.Type)
		for _, name := range valueSpec.Names {
			if typeName != "" {
				locals[name.Name] = typeName
			}
		}
	}
}

func collectLocalTypesFromAssign(ctx Context, stmt *ast.AssignStmt, locals map[string]string) {
	for i, lhs := range stmt.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok {
			continue
		}
		rhsIndex := i
		if len(stmt.Rhs) == 1 {
			rhsIndex = 0
		}
		if rhsIndex >= len(stmt.Rhs) {
			continue
		}
		if typ := inferTypeFromExpr(ctx, stmt.Rhs[rhsIndex]); typ != "" {
			locals[ident.Name] = typ
		}
	}
}

func inferTypeFromExpr(ctx Context, expr ast.Expr) string {
	if ctx.Pass == nil || ctx.Pass.TypesInfo == nil {
		return typeNameFromASTExpr(expr)
	}
	t := ctx.Pass.TypesInfo.TypeOf(expr)
	if t == nil {
		return typeNameFromASTExpr(expr)
	}
	return schemaName(t)
}

func typeExprName(ctx Context, expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	if ctx.Pass != nil && ctx.Pass.TypesInfo != nil {
		if t := ctx.Pass.TypesInfo.TypeOf(expr); t != nil {
			return schemaName(t)
		}
	}
	return typeNameFromASTExpr(expr)
}

func resolveSchemaTypeName(ctx Context, expr ast.Expr, locals map[string]string, handlerName, usage string, pos token.Position) string {
	if ctx.Pass != nil && ctx.Pass.TypesInfo != nil {
		if t := ctx.Pass.TypesInfo.TypeOf(expr); isAnyType(t) {
			if handlerName == "" {
				handlerName = "unknown"
			}
			warnInterfaceType(ctx, pos, handlerName, usage)
			return ""
		}
	}
	return resolveTypeName(ctx.Pass, expr, locals)
}

func requestSchemaFromBindCall(ctx Context, call *ast.CallExpr, locals map[string]string) string {
	name := callName(call)
	if name != "bindAndValidate" && name != "bindBody" {
		return ""
	}
	if len(call.Args) < 2 {
		return ""
	}
	return resolveTypeName(ctx.Pass, call.Args[1], locals)
}

func callName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	default:
		return ""
	}
}

func responseFromHelperCall(ctx Context, call *ast.CallExpr, locals map[string]string) (code, schema string) {
	name := callName(call)
	switch {
	case strings.HasPrefix(name, "Created") || strings.Contains(name, "Created"):
		code = "201"
	case strings.HasPrefix(name, "Success"):
		code = "200"
	case strings.HasPrefix(name, "NoContent"):
		code = "204"
	default:
		return "", ""
	}
	if len(call.Args) > 0 {
		pos := token.Position{}
		if ctx.Pass != nil && ctx.Pass.Fset != nil {
			pos = ctx.Pass.Fset.Position(call.Pos())
		}
		if s := resolveSchemaTypeName(ctx, call.Args[0], locals, callName(call), "response", pos); s != "" {
			return code, s
		}
	}
	return code, ""
}

func appendUniqueParam(params []Param, param Param) []Param {
	for _, existing := range params {
		if existing.Name == param.Name {
			return params
		}
	}
	return append(params, param)
}
