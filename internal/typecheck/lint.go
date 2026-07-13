// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package typecheck

import (
	"fmt"
	"os"

	"github.com/whisskey/prism/internal/analyzer/fibergen"
)

// LoadCatalogFromPatterns loads packages and returns a merged fibergen catalog.
func LoadCatalogFromPatterns(patterns []string) (*fibergen.Catalog, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	moduleRoot := FindModuleRoot([]string{wd})
	if moduleRoot == "" {
		return nil, fmt.Errorf("go.mod not found")
	}
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	results, err := RunFibergen(moduleRoot, patterns)
	if err != nil {
		return nil, err
	}
	return fibergen.NewCatalog(results), nil
}

// LintIssues returns fibergen warnings across loaded packages.
func LintIssues(catalog *fibergen.Catalog) []fibergen.Warning {
	if catalog == nil || catalog.Merged == nil {
		return nil
	}
	return append([]fibergen.Warning(nil), catalog.Merged.Warnings...)
}
