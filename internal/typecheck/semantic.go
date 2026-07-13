// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package typecheck

import (
	"go/ast"
	"go/token"
	"go/types"
)

// Pass holds semantic analysis (types.Info) for one Go source file.
// It resolves identifier bindings, expression types, and selector selections.
type Pass struct {
	FilePath string
	File     *ast.File
	Pkg      *types.Package
	Info     *types.Info
	FileSet  *token.FileSet
}

// PassForFile returns semantic info for a loaded file path.
func (p *Program) PassForFile(filePath string) *Pass {
	ctx := p.contextForFile(filePath)
	if ctx == nil || ctx.info == nil {
		return nil
	}
	return &Pass{
		FilePath: normalizePath(filePath),
		File:     ctx.file,
		Pkg:      ctx.pkg,
		Info:     ctx.info,
		FileSet:  p.fset,
	}
}

// TypeOf returns the type of an expression.
func (pass *Pass) TypeOf(expr ast.Expr) types.Type {
	if pass == nil || pass.Info == nil || expr == nil {
		return nil
	}
	if pass.FileSet != nil && pass.FileSet.Position(expr.Pos()).Filename != "" {
		if t := pass.Info.TypeOf(expr); t != nil {
			return t
		}
	}
	if node := pass.matchNode(expr); node != nil {
		if e, ok := node.(ast.Expr); ok {
			return pass.Info.TypeOf(e)
		}
	}
	return nil
}

// TypeOfAt resolves a type by source position (cross-parse bridge).
func (pass *Pass) TypeOfAt(pos ExprPosition, kind string) types.Type {
	if pass == nil || pass.Info == nil || pos.Line == 0 {
		return nil
	}
	node := pass.nodeAt(pos, kind)
	if node == nil {
		return nil
	}
	if call, ok := node.(*ast.CallExpr); ok {
		return FirstResultType(pass.Info.TypeOf(call))
	}
	if e, ok := node.(ast.Expr); ok {
		return pass.Info.TypeOf(e)
	}
	return nil
}

// CallResultType returns the first result type of a call expression.
func (pass *Pass) CallResultType(call *ast.CallExpr) types.Type {
	return FirstResultType(pass.TypeOf(call))
}

// ReceiverType returns the type of a selector's receiver (sel.X).
func (pass *Pass) ReceiverType(sel *ast.SelectorExpr) types.Type {
	if pass == nil || sel == nil {
		return nil
	}
	return pass.TypeOf(sel.X)
}

// Selection returns the types.Selection for x.f, if resolved.
func (pass *Pass) Selection(sel *ast.SelectorExpr) *types.Selection {
	if pass == nil || pass.Info == nil || sel == nil {
		return nil
	}
	if node := pass.matchNode(sel); node != nil {
		if s, ok := node.(*ast.SelectorExpr); ok {
			if selInfo, ok := pass.Info.Selections[s]; ok {
				return selInfo
			}
		}
	}
	return nil
}

// Def returns the object defined by ident.
func (pass *Pass) Def(ident *ast.Ident) types.Object {
	if pass == nil || pass.Info == nil || ident == nil {
		return nil
	}
	return pass.Info.Defs[ident]
}

// Use returns the object referenced by ident.
func (pass *Pass) Use(ident *ast.Ident) types.Object {
	if pass == nil || pass.Info == nil || ident == nil {
		return nil
	}
	return pass.Info.Uses[ident]
}

// ObjectType returns the type of a Def or Use object.
func ObjectType(obj types.Object) types.Type {
	if obj == nil {
		return nil
	}
	return obj.Type()
}

// ScopeAt returns the innermost scope for a node, if recorded.
func (pass *Pass) ScopeAt(node ast.Node) *types.Scope {
	if pass == nil || pass.Info == nil || node == nil {
		return nil
	}
	return pass.Info.Scopes[node]
}

func (pass *Pass) matchNode(expr ast.Expr) ast.Node {
	if pass == nil || pass.File == nil || pass.FileSet == nil || expr == nil {
		return nil
	}
	pos := pass.FileSet.Position(expr.Pos())
	if !pos.IsValid() {
		return nil
	}
	return pass.nodeAt(ExprPosition{Line: pos.Line, Column: pos.Column}, "")
}

func (pass *Pass) nodeAt(pos ExprPosition, kind string) ast.Node {
	if pass == nil || pass.File == nil || pass.FileSet == nil || pos.Line == 0 {
		return nil
	}
	var found ast.Node
	ast.Inspect(pass.File, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		ppos := pass.FileSet.Position(n.Pos())
		if ppos.Line != pos.Line {
			return true
		}
		if pos.Column != 0 && ppos.Column != pos.Column {
			return true
		}
		switch kind {
		case "call":
			if _, ok := n.(*ast.CallExpr); ok {
				found = n
				return false
			}
		default:
			if _, ok := n.(ast.Expr); ok {
				found = n
				return false
			}
		}
		return true
	})
	return found
}
