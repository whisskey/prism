// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"strings"

	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var httpMethods = map[string]bool{
	"Get": true, "Post": true, "Put": true, "Delete": true,
	"Patch": true, "Head": true, "Options": true, "Trace": true,
	"Connect": true, "All": true,
}

var collectFilter = []ast.Node{
	(*ast.CallExpr)(nil),
	(*ast.FuncDecl)(nil),
	(*ast.GenDecl)(nil),
}

// CollectPass gathers raw AST nodes relevant to Fiber OpenAPI generation.
func CollectPass(pass *analysis.Pass) Context {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	ctx := Context{
		Pass:      pass,
		Inspector: insp,
		Warn:      NewWarnLogger(true),
	}

	insp.Preorder(collectFilter, func(node ast.Node) {
		switch n := node.(type) {
		case *ast.CallExpr:
			if call := classifyCall(pass, n); call != nil {
				ctx.Raw.Calls = append(ctx.Raw.Calls, *call)
			}
			if group := classifyGroup(pass, n); group != nil {
				ctx.Raw.Groups = append(ctx.Raw.Groups, *group)
			}
		case *ast.FuncDecl:
			if handler := classifyHandler(pass, n); handler != nil {
				ctx.Raw.Handlers = append(ctx.Raw.Handlers, *handler)
			}
		case *ast.GenDecl:
			if n.Tok != token.TYPE {
				return
			}
			for _, spec := range n.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				ctx.Raw.Structs = append(ctx.Raw.Structs, StructInfo{
					Name:       typeSpec.Name.Name,
					Position:   pass.Fset.Position(typeSpec.Pos()),
					TypeSpec:   typeSpec,
					StructType: structType,
				})
			}
		}
	})

	return ctx
}

func classifyGroup(pass *analysis.Pass, call *ast.CallExpr) *GroupInfo {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Group" || len(call.Args) == 0 {
		return nil
	}
	prefix, ok := stringLiteral(call.Args[0])
	if !ok {
		return nil
	}
	return &GroupInfo{
		Prefix:   prefix,
		Position: pass.Fset.Position(call.Pos()),
		CallExpr: call,
	}
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s := lit.Value
	if len(s) >= 2 && (s[0] == '"' || s[0] == '`') {
		s = s[1 : len(s)-1]
	}
	return s, true
}

func isFiberRouterType(typeStr string) bool {
	return strings.Contains(typeStr, "fiber.App") ||
		strings.Contains(typeStr, "fiber.Router") ||
		strings.Contains(typeStr, "fiber.Group")
}

func classifyHandler(pass *analysis.Pass, fn *ast.FuncDecl) *HandlerInfo {
	if fn == nil || fn.Type == nil || fn.Type.Params == nil {
		return nil
	}
	if !hasFiberCtxParam(pass, fn) {
		return nil
	}
	return &HandlerInfo{
		Name:     fn.Name.Name,
		Position: pass.Fset.Position(fn.Pos()),
		FuncDecl: fn,
	}
}

func hasFiberCtxParam(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	for _, field := range fn.Type.Params.List {
		typ := pass.TypesInfo.TypeOf(field.Type)
		if typ == nil {
			continue
		}
		if strings.Contains(types.TypeString(typ, nil), "fiber.Ctx") {
			return true
		}
	}
	return false
}

func classifyCall(pass *analysis.Pass, call *ast.CallExpr) *CallInfo {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	method := sel.Sel.Name
	if !httpMethods[method] {
		return nil
	}

	recvType := pass.TypesInfo.TypeOf(sel.X)
	return &CallInfo{
		Method:         method,
		Position:       pass.Fset.Position(call.Pos()),
		CallExpr:       call,
		ReceiverType:   types.TypeString(recvType, nil),
		ReceiverSchema: shortTypeName(recvType),
	}
}

func shortTypeName(t types.Type) string {
	if t == nil {
		return ""
	}
	s := types.TypeString(t, nil)
	s = strings.TrimPrefix(s, "*")
	if idx := strings.LastIndex(s, "."); idx >= 0 {
		s = s[idx+1:]
	}
	return s
}
