// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package gin

import (
	"go/ast"
	"path/filepath"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/internal/typecheck"
)

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

func (p *Plugin) typedCallResultSchema(pf *parser.ParsedFile, callExpr *ast.CallExpr) string {
	if p.typeProgram == nil || pf == nil || callExpr == nil {
		return ""
	}
	path, _ := filepath.Abs(pf.Path)
	if pass := p.typeProgram.PassForFile(path); pass != nil {
		pos := typecheck.ExprPosition{}
		if pf.FileSet != nil {
			ppos := pf.FileSet.Position(callExpr.Pos())
			pos = typecheck.ExprPosition{Line: ppos.Line, Column: ppos.Column}
		}
		if t := pass.TypeOfAt(pos, "call"); t != nil {
			return typecheck.SchemaName(t)
		}
	}
	return ""
}

func (p *Plugin) typedSchemaName(pf *parser.ParsedFile, expr ast.Expr) string {
	if p.typeProgram == nil || pf == nil || expr == nil {
		return ""
	}
	path, _ := filepath.Abs(pf.Path)
	if pass := p.typeProgram.PassForFile(path); pass != nil {
		pos := typecheck.ExprPosition{}
		if pf.FileSet != nil {
			ppos := pf.FileSet.Position(expr.Pos())
			pos = typecheck.ExprPosition{Line: ppos.Line, Column: ppos.Column}
		}
		if t := pass.TypeOfAt(pos, ""); t != nil {
			return cleanTypeName(typecheck.SchemaName(t))
		}
	}
	return ""
}
