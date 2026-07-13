// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package typecheck

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"github.com/whisskey/prism/internal/analyzer/fibergen"

	"golang.org/x/tools/go/packages"
)

// Program holds type information for a loaded Go module.
type Program struct {
	ModuleRoot      string
	Packages        []*packages.Package
	Fibergen        []*fibergen.Result
	FibergenIndex   *fibergen.Index
	FibergenCatalog *fibergen.Catalog
	fset            *token.FileSet
	byFile          map[string]*fileContext
}

type fileContext struct {
	file *ast.File
	info *types.Info
	pkg  *types.Package
}

// Load builds a Program from scanned file paths. Returns nil without error when
// no module root can be found or type checking fails.
func Load(filePaths []string) (*Program, error) {
	moduleRoot := FindModuleRoot(filePaths)
	if moduleRoot == "" {
		return nil, nil
	}

	patterns := PackagePatterns(moduleRoot, filePaths)
	pkgs, err := LoadPackages(moduleRoot, patterns)
	if err != nil {
		return nil, err
	}
	pkgs = flattenPackages(pkgs)

	prog := &Program{
		ModuleRoot: moduleRoot,
		Packages:   pkgs,
		byFile:     make(map[string]*fileContext),
	}

	for _, pkg := range pkgs {
		if pkg == nil || pkg.Types == nil || pkg.Fset == nil {
			continue
		}
		if prog.fset == nil {
			prog.fset = pkg.Fset
		}
		for i, file := range pkg.Syntax {
			if file == nil {
				continue
			}
			path := prog.fileKey(file, pkg, i)
			if path == "" {
				continue
			}
			prog.byFile[path] = &fileContext{
				file: file,
				info: pkg.TypesInfo,
				pkg:  pkg.Types,
			}
		}
	}

	if len(prog.byFile) == 0 {
		return nil, nil
	}

	if fibergenResults, err := RunFibergenFromPaths(filePaths); err == nil {
		prog.Fibergen = fibergenResults
		catalog := fibergen.NewCatalog(fibergenResults)
		prog.FibergenCatalog = catalog
		prog.FibergenIndex = &fibergen.Index{Calls: catalog.Calls}
	}

	return prog, nil
}

func (p *Program) fileKey(file *ast.File, pkg *packages.Package, index int) string {
	if file == nil || p == nil || p.fset == nil {
		return ""
	}
	pos := p.fset.Position(file.Pos())
	if pos.Filename != "" {
		return normalizePath(pos.Filename)
	}
	if pkg != nil && index < len(pkg.GoFiles) {
		return normalizePath(pkg.GoFiles[index])
	}
	return ""
}

func normalizePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

// TypeOfExpr resolves the type of an expression using go/types.
func (p *Program) TypeOfExpr(filePath string, expr ast.Expr) types.Type {
	if p == nil || expr == nil {
		return nil
	}
	ctx := p.contextForFile(filePath)
	if ctx == nil || ctx.info == nil {
		return nil
	}

	if node := p.matchNode(ctx, expr); node != nil {
		if e, ok := node.(ast.Expr); ok {
			if t := ctx.info.TypeOf(e); t != nil {
				return t
			}
		}
	}
	return nil
}

// TypeOfCallResult resolves the first return type of a call expression.
func (p *Program) TypeOfCallResult(filePath string, callExpr *ast.CallExpr) types.Type {
	if p == nil || callExpr == nil {
		return nil
	}
	ctx := p.contextForFile(filePath)
	if ctx == nil || ctx.info == nil {
		return nil
	}

	node := p.matchNode(ctx, callExpr)
	if node == nil {
		return nil
	}
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil
	}
	return FirstResultType(ctx.info.TypeOf(call))
}

// TypeOfIdent resolves the type of a local identifier by name at a use site line.
func (p *Program) TypeOfIdent(filePath string, line int, name string) types.Type {
	if p == nil || name == "" {
		return nil
	}
	ctx := p.contextForFile(filePath)
	if ctx == nil || ctx.info == nil || ctx.file == nil {
		return nil
	}

	var found types.Type
	ast.Inspect(ctx.file, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || ident.Name != name || ident.Obj == nil {
			return true
		}
		if p.fset.Position(ident.Pos()).Line != line {
			return true
		}
		if obj, ok := ctx.info.Defs[ident]; ok && obj != nil {
			found = obj.Type()
			return false
		}
		if obj, ok := ctx.info.Uses[ident]; ok && obj != nil {
			found = obj.Type()
			return false
		}
		return true
	})
	return found
}

func (p *Program) contextForFile(filePath string) *fileContext {
	if p == nil {
		return nil
	}
	key := normalizePath(filePath)
	if ctx, ok := p.byFile[key]; ok {
		return ctx
	}
	for path, ctx := range p.byFile {
		if strings.HasSuffix(path, string(filepath.Separator)+filepath.Base(key)) ||
			filepath.Base(path) == filepath.Base(key) {
			return ctx
		}
	}
	return nil
}

func (p *Program) matchNode(ctx *fileContext, expr ast.Expr) ast.Node {
	if ctx == nil || ctx.file == nil || p.fset == nil || expr == nil {
		return nil
	}

	// Tests and inline sources may not share the loaded file set; match by line/column.
	target := p.externalPosition(expr)
	if target.Line == 0 {
		return nil
	}

	var found ast.Node
	ast.Inspect(ctx.file, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		pos := p.fset.Position(n.Pos())
		if pos.Line != target.Line {
			return true
		}
		if target.Column != 0 && pos.Column != target.Column {
			return true
		}
		if _, ok := n.(ast.Expr); ok {
			found = n
			return false
		}
		return true
	})
	return found
}

type externalPosition struct {
	Line   int
	Column int
}

func (p *Program) externalPosition(expr ast.Expr) externalPosition {
	if expr == nil {
		return externalPosition{}
	}
	// When expr comes from another parse, Pos may be invalid; callers should pass
	// position via SetExprPosition in the plugin layer.
	if pos := expr.Pos(); pos.IsValid() && p.fset != nil {
		ppos := p.fset.Position(pos)
		if strings.HasSuffix(ppos.Filename, ".go") {
			return externalPosition{Line: ppos.Line, Column: ppos.Column}
		}
	}
	return externalPosition{}
}

// ExprPosition captures line/column for cross-parse matching.
type ExprPosition struct {
	Line   int
	Column int
}

// TypeOfExprAt resolves a type using explicit source position from the plugin AST.
func (p *Program) TypeOfExprAt(filePath string, pos ExprPosition, kind string) types.Type {
	pass := p.PassForFile(filePath)
	if pass == nil {
		return nil
	}
	return pass.TypeOfAt(pos, kind)
}

// FilesLoaded returns the number of source files with type information.
func (p *Program) FilesLoaded() int {
	if p == nil {
		return 0
	}
	return len(p.byFile)
}

// IndexInterfaceMethods builds method name -> first return type for all interfaces.
func (p *Program) IndexInterfaceMethods() map[string]string {
	result := make(map[string]string)
	if p == nil {
		return result
	}
	for _, pkg := range p.Packages {
		if pkg == nil || pkg.Types == nil {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			typeName, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}
			iface, ok := typeName.Type().Underlying().(*types.Interface)
			if !ok {
				continue
			}
			for i := 0; i < iface.NumMethods(); i++ {
				method := iface.Method(i)
				sig, ok := method.Type().(*types.Signature)
				if !ok || sig.Results().Len() == 0 {
					continue
				}
				ret := SchemaName(sig.Results().At(0).Type())
				if ret == "" {
					continue
				}
				result[method.Name()] = ret
			}
		}
	}
	return result
}
