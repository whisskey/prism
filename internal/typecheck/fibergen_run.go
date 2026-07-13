// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package typecheck

import (
	"github.com/whisskey/prism/internal/analyzer/fibergen"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
)

// RunFibergen loads packages and runs the fibergen go/analysis pass.
func RunFibergen(moduleRoot string, patterns []string) ([]*fibergen.Result, error) {
	if moduleRoot == "" {
		return nil, nil
	}
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	pkgs, err := LoadPackages(moduleRoot, patterns)
	if err != nil {
		return nil, err
	}

	graph, err := checker.Analyze([]*analysis.Analyzer{fibergen.Analyzer}, pkgs, nil)
	if err != nil {
		return nil, err
	}

	var results []*fibergen.Result
	for act := range graph.All() {
		if act.Analyzer.Name != fibergen.Analyzer.Name || act.Result == nil {
			continue
		}
		if result, ok := act.Result.(*fibergen.Result); ok {
			results = append(results, result)
		}
	}
	return results, nil
}

// RunFibergenFromPaths runs fibergen for file paths under a module.
func RunFibergenFromPaths(filePaths []string) ([]*fibergen.Result, error) {
	moduleRoot := FindModuleRoot(filePaths)
	if moduleRoot == "" {
		return nil, nil
	}
	patterns := PackagePatterns(moduleRoot, filePaths)
	return RunFibergen(moduleRoot, patterns)
}

// FibergenAnalyzers returns registered prism analyzers.
func FibergenAnalyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{fibergen.Analyzer}
}
