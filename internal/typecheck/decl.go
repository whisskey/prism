// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package typecheck

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// FuncDeclForCall resolves the called function's AST declaration from a call site.
func (p *Program) FuncDeclForCall(filePath string, pos ExprPosition) *ast.FuncDecl {
	pass := p.PassForFile(filePath)
	if pass == nil || pass.Info == nil || pos.Line == 0 {
		return nil
	}

	node := pass.nodeAt(pos, "call")
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil
	}

	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		if sel := pass.Selection(fun); sel != nil {
			if decl := p.FuncDeclForObject(sel.Obj()); decl != nil {
				return decl
			}
		}
		if sel, ok := pass.Info.Selections[fun]; ok && sel != nil {
			if decl := p.FuncDeclForObject(sel.Obj()); decl != nil {
				return decl
			}
		}
		if fun.Sel != nil && fun.Sel.Name != "" {
			return p.FuncDeclForName(fun.Sel.Name)
		}
	case *ast.Ident:
		if obj := pass.Info.Uses[fun]; obj != nil {
			return p.FuncDeclForObject(obj)
		}
	}
	return nil
}

// FuncDeclForName finds a package-level function declaration by name across loaded packages.
func (p *Program) FuncDeclForName(name string) *ast.FuncDecl {
	if p == nil || name == "" {
		return nil
	}
	for _, pkg := range p.Packages {
		if fn := funcDeclNamedInPackage(pkg, name); fn != nil {
			return fn
		}
	}
	return nil
}

// FuncDeclForObject maps a type-checked object to its defining *ast.FuncDecl.
func (p *Program) FuncDeclForObject(obj types.Object) *ast.FuncDecl {
	if p == nil || obj == nil {
		return nil
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil
	}
	if fn.Pkg() == nil {
		return nil
	}
	for _, pkg := range p.Packages {
		if pkg == nil || pkg.PkgPath != fn.Pkg().Path() {
			continue
		}
		if decl := funcDeclForObjectInPackage(pkg, fn); decl != nil {
			return decl
		}
	}
	return nil
}

func funcDeclNamedInPackage(pkg *packages.Package, name string) *ast.FuncDecl {
	if pkg == nil || pkg.Syntax == nil {
		return nil
	}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil || fn.Name.Name != name {
				continue
			}
			return fn
		}
	}
	return nil
}

func funcDeclForObjectInPackage(pkg *packages.Package, fn *types.Func) *ast.FuncDecl {
	if pkg == nil || fn == nil || pkg.Syntax == nil || pkg.TypesInfo == nil {
		return nil
	}
	var found *ast.FuncDecl
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			decl, ok := n.(*ast.FuncDecl)
			if !ok || decl.Name == nil {
				return true
			}
			if obj, ok := pkg.TypesInfo.Defs[decl.Name]; ok && obj == fn {
				found = decl
				return false
			}
			return true
		})
		if found != nil {
			return found
		}
	}
	return nil
}
