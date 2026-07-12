// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"path/filepath"
	"go/token"

	"golang.org/x/tools/go/analysis"
)

func reportWarnings(pass *analysis.Pass, warnings []Warning) {
	if pass == nil || len(warnings) == 0 {
		return
	}
	for _, warning := range warnings {
		pos := warningPos(pass, warning.Pos)
		if !pos.IsValid() {
			pass.Reportf(token.NoPos, "%s", warning.Message)
			continue
		}
		pass.Report(analysis.Diagnostic{Pos: pos, Message: warning.Message})
	}
}

func warningPos(pass *analysis.Pass, pos token.Position) token.Pos {
	if pass == nil || pass.Fset == nil || pos.Filename == "" || pos.Line <= 0 {
		return token.NoPos
	}
	targetBase := filepath.Base(pos.Filename)
	for _, file := range pass.Files {
		filePos := pass.Fset.File(file.Pos())
		if filePos == nil {
			continue
		}
		if filepath.Base(filePos.Name()) != targetBase && filePos.Name() != pos.Filename {
			continue
		}
		if pos.Line > filePos.LineCount() {
			continue
		}
		return filePos.LineStart(pos.Line)
	}
	return token.NoPos
}
