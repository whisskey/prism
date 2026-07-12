// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package lint

import (
	"github.com/whisskey/prism/internal/analyzer/fibergen"
	"github.com/whisskey/prism/internal/typecheck"
)

func lintFiber(patterns []string) ([]Issue, error) {
	catalog, err := typecheck.LoadCatalogFromPatterns(patterns)
	if err != nil {
		return nil, err
	}
	warnings := typecheck.LintIssues(catalog)
	issues := make([]Issue, len(warnings))
	for i, warning := range warnings {
		issues[i] = Issue{Pos: warning.Pos, Message: warning.Message}
	}
	return issues, nil
}

// FromFibergenWarnings converts fibergen warnings to lint issues.
func FromFibergenWarnings(warnings []fibergen.Warning) []Issue {
	issues := make([]Issue, len(warnings))
	for i, warning := range warnings {
		issues[i] = Issue{Pos: warning.Pos, Message: warning.Message}
	}
	return issues
}
