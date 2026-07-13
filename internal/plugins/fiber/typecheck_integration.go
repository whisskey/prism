// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"
	"go/token"
	"path/filepath"

	"github.com/whisskey/prism/internal/analyzer/fibergen"
	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/internal/scanner"
	"github.com/whisskey/prism/internal/typecheck"
)

func fibergenCatalogFromProgram(prog *typecheck.Program) *fibergen.Catalog {
	if prog == nil || len(prog.Fibergen) == 0 {
		return nil
	}
	if prog.FibergenCatalog != nil {
		return prog.FibergenCatalog
	}
	return fibergen.NewCatalog(prog.Fibergen)
}

func (p *Plugin) ensureFibergenCatalog(files []scanner.SourceFile) {
	if p.fibergenCatalog != nil {
		return
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		if file.Language == "go" {
			paths = append(paths, file.Path)
		}
	}
	p.loadTypeProgram(paths)
	p.fibergenCatalog = fibergenCatalogFromProgram(p.typeProgram)
}

func (p *Plugin) loadTypeProgram(filePaths []string) {
	if len(filePaths) == 0 {
		return
	}
	prog, err := typecheck.Load(filePaths)
	if err != nil || prog == nil {
		return
	}
	p.typeProgram = prog
	for method, ret := range prog.IndexInterfaceMethods() {
		if p.returnTypeIndex[method] == "" {
			p.returnTypeIndex[method] = ret
		}
	}
}

func (p *Plugin) exprPosition(pf *parser.ParsedFile, expr ast.Expr) typecheck.ExprPosition {
	if pf == nil || pf.FileSet == nil || expr == nil {
		return typecheck.ExprPosition{}
	}
	pos := pf.FileSet.Position(expr.Pos())
	return typecheck.ExprPosition{Line: pos.Line, Column: pos.Column}
}

func (p *Plugin) typedSchemaName(pf *parser.ParsedFile, expr ast.Expr) string {
	if p.typeProgram == nil || pf == nil || expr == nil {
		return ""
	}
	path, _ := filepath.Abs(pf.Path)
	if pass := p.typeProgram.PassForFile(path); pass != nil {
		if t := pass.TypeOfAt(p.exprPosition(pf, expr), ""); t != nil {
			return cleanTypeName(typecheck.SchemaName(t))
		}
	}
	return ""
}

func (p *Plugin) typedCallResultSchema(pf *parser.ParsedFile, callExpr *ast.CallExpr) string {
	if p.typeProgram == nil || pf == nil || callExpr == nil {
		return ""
	}
	path, _ := filepath.Abs(pf.Path)
	if pass := p.typeProgram.PassForFile(path); pass != nil {
		if t := pass.TypeOfAt(p.exprPosition(pf, callExpr), "call"); t != nil {
			return typecheck.SchemaName(t)
		}
	}
	return ""
}

func (p *Plugin) typedIdentSchema(pf *parser.ParsedFile, line int, name string) string {
	if p.typeProgram == nil || pf == nil || name == "" {
		return ""
	}
	path, _ := filepath.Abs(pf.Path)
	t := p.typeProgram.TypeOfIdent(path, line, name)
	if t == nil {
		return ""
	}
	return typecheck.SchemaName(t)
}

func (p *Plugin) currentFilePath(pf *parser.ParsedFile) string {
	if pf == nil {
		return ""
	}
	path, err := filepath.Abs(pf.Path)
	if err != nil {
		return pf.Path
	}
	return path
}

func positionLine(pf *parser.ParsedFile, pos token.Pos) int {
	if pf == nil || pf.FileSet == nil {
		return 0
	}
	return pf.FileSet.Position(pos).Line
}
