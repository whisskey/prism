// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package echo

import (
	"go/ast"
)

const maxReturnTypeInferenceDepth = 8

func (p *Plugin) resolveMethodReturnType(funcDecl *ast.FuncDecl) string {
	if funcDecl == nil {
		return ""
	}
	visited := make(map[*ast.FuncDecl]bool)
	return p.resolveMethodReturnTypeDepth(funcDecl, visited, 0)
}

func (p *Plugin) resolveMethodReturnTypeDepth(funcDecl *ast.FuncDecl, visited map[*ast.FuncDecl]bool, depth int) string {
	if funcDecl == nil || visited[funcDecl] || depth > maxReturnTypeInferenceDepth {
		return ""
	}
	visited[funcDecl] = true
	defer delete(visited, funcDecl)

	if funcDecl.Type.Results != nil && len(funcDecl.Type.Results.List) > 0 {
		returnType := astTypeString(funcDecl.Type.Results.List[0].Type)
		if cleanTypeName(returnType) != "" {
			return returnType
		}
	}

	if funcDecl.Body == nil {
		return p.inferReturnTypeFromCallExprDecl(funcDecl)
	}

	locals := make(map[string]string)
	p.collectReturnTypesFromBlock(funcDecl.Body, locals, visited, depth+1)

	for i := len(funcDecl.Body.List) - 1; i >= 0; i-- {
		if returnType := p.inferReturnTypeFromStmt(funcDecl.Body.List[i], locals, visited, depth+1); returnType != "" {
			return returnType
		}
	}

	return ""
}

func (p *Plugin) inferReturnTypeFromCallExprDecl(funcDecl *ast.FuncDecl) string {
	name := funcDecl.Name.Name
	if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
		recvType := cleanTypeName(astTypeString(funcDecl.Recv.List[0].Type))
		if recvType != "" {
			name = recvType + "." + name
		}
	}
	if returnType := p.returnTypeIndex[name]; cleanTypeName(returnType) != "" {
		return returnType
	}
	if returnType := p.returnTypeIndex[funcDecl.Name.Name]; cleanTypeName(returnType) != "" {
		return returnType
	}
	return ""
}

func (p *Plugin) collectReturnTypesFromBlock(block *ast.BlockStmt, locals map[string]string, visited map[*ast.FuncDecl]bool, depth int) {
	if block == nil {
		return
	}
	for _, stmt := range block.List {
		p.collectReturnTypesFromStmt(stmt, locals, visited, depth)
	}
}

func (p *Plugin) collectReturnTypesFromStmt(stmt ast.Stmt, locals map[string]string, visited map[*ast.FuncDecl]bool, depth int) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		for i, lhs := range s.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if !ok {
				continue
			}
			rhsIndex := i
			if len(s.Rhs) == 1 {
				rhsIndex = 0
			}
			if rhsIndex >= len(s.Rhs) {
				continue
			}
			if returnType := p.inferReturnTypeFromExpr(s.Rhs[rhsIndex], locals, visited, depth); returnType != "" {
				locals[ident.Name] = returnType
			}
		}
	case *ast.DeclStmt:
		p.collectReturnTypesFromDecl(s, locals, visited, depth)
	case *ast.BlockStmt:
		p.collectReturnTypesFromBlock(s, locals, visited, depth)
	case *ast.IfStmt:
		p.collectReturnTypesFromBlock(s.Body, locals, visited, depth)
		if s.Else != nil {
			p.collectReturnTypesFromStmt(s.Else, locals, visited, depth)
		}
	}
}

func (p *Plugin) collectReturnTypesFromDecl(stmt *ast.DeclStmt, locals map[string]string, visited map[*ast.FuncDecl]bool, depth int) {
	genDecl, ok := stmt.Decl.(*ast.GenDecl)
	if !ok {
		return
	}
	for _, spec := range genDecl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range valueSpec.Names {
			if valueSpec.Type != nil {
				typeName := astTypeString(valueSpec.Type)
				if cleanTypeName(typeName) != "" {
					locals[name.Name] = typeName
					continue
				}
			}
			if i < len(valueSpec.Values) {
				if returnType := p.inferReturnTypeFromExpr(valueSpec.Values[i], locals, visited, depth); returnType != "" {
					locals[name.Name] = returnType
				}
			}
		}
	}
}

func (p *Plugin) inferReturnTypeFromStmt(stmt ast.Stmt, locals map[string]string, visited map[*ast.FuncDecl]bool, depth int) string {
	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		if len(s.Results) == 0 {
			return ""
		}
		return p.inferReturnTypeFromExpr(s.Results[0], locals, visited, depth)
	case *ast.BlockStmt:
		for i := len(s.List) - 1; i >= 0; i-- {
			if returnType := p.inferReturnTypeFromStmt(s.List[i], locals, visited, depth); returnType != "" {
				return returnType
			}
		}
	case *ast.IfStmt:
		if returnType := p.inferReturnTypeFromStmt(s.Body, locals, visited, depth); returnType != "" {
			return returnType
		}
		if s.Else != nil {
			return p.inferReturnTypeFromStmt(s.Else, locals, visited, depth)
		}
	}
	return ""
}

func (p *Plugin) inferReturnTypeFromExpr(expr ast.Expr, locals map[string]string, visited map[*ast.FuncDecl]bool, depth int) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return locals[e.Name]
	case *ast.UnaryExpr:
		if e.Op.String() == "&" {
			if inner := p.inferReturnTypeFromExpr(e.X, locals, visited, depth); inner != "" {
				return inner
			}
		}
		return p.inferReturnTypeFromExpr(e.X, locals, visited, depth)
	case *ast.CallExpr:
		return p.inferReturnTypeFromCallDepth(e, visited, depth)
	case *ast.StarExpr:
		if inner := p.inferReturnTypeFromExpr(e.X, locals, visited, depth); inner != "" {
			return inner
		}
	}
	return ""
}

func (p *Plugin) inferReturnTypeFromCall(callExpr *ast.CallExpr) string {
	visited := make(map[*ast.FuncDecl]bool)
	return p.inferReturnTypeFromCallDepth(callExpr, visited, 0)
}

func (p *Plugin) inferReturnTypeFromCallDepth(callExpr *ast.CallExpr, visited map[*ast.FuncDecl]bool, depth int) string {
	if callExpr == nil || depth > maxReturnTypeInferenceDepth {
		return ""
	}

	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}

	methodName := selExpr.Sel.Name
	if recvType := p.receiverTypeFromSelector(selExpr); recvType != "" {
		key := recvType + "." + methodName
		if returnType := p.returnTypeIndex[key]; cleanTypeName(returnType) != "" {
			return returnType
		}
		if callee := p.routeFuncIndex[key]; callee != nil {
			if returnType := p.resolveMethodReturnTypeDepth(callee, visited, depth+1); returnType != "" {
				return returnType
			}
		}
	}

	if callee := p.routeFuncIndex[methodName]; callee != nil {
		if returnType := p.resolveMethodReturnTypeDepth(callee, visited, depth+1); returnType != "" {
			return returnType
		}
	}

	if returnType := p.returnTypeIndex[methodName]; cleanTypeName(returnType) != "" {
		return returnType
	}

	return ""
}

func (p *Plugin) receiverTypeFromSelector(selExpr *ast.SelectorExpr) string {
	if selExpr == nil {
		return ""
	}
	switch recv := selExpr.X.(type) {
	case *ast.Ident:
		return cleanTypeName(recv.Name)
	case *ast.SelectorExpr:
		return cleanTypeName(recv.Sel.Name)
	}
	return ""
}
