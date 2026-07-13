// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package typecheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// FindModuleRoot walks upward from paths to locate the nearest go.mod directory.
func FindModuleRoot(paths []string) string {
	var best string
	for _, path := range paths {
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil {
			continue
		}
		dir := abs
		if !info.IsDir() {
			dir = filepath.Dir(abs)
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				if best == "" || len(dir) < len(best) {
					best = dir
				}
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return best
}

// PackagePatterns derives load patterns from scanned file paths relative to moduleRoot.
func PackagePatterns(moduleRoot string, paths []string) []string {
	seen := make(map[string]struct{})
	var patterns []string
	for _, path := range paths {
		if path == "" || !strings.HasSuffix(path, ".go") {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(moduleRoot, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		if dir == "." {
			dir = "."
		}
		pattern := "./" + dir
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		patterns = append(patterns, pattern)
	}
	if len(patterns) == 0 {
		return []string{"./..."}
	}
	return patterns
}

// LoadPackages type-checks Go packages under moduleRoot for the given patterns.
func LoadPackages(moduleRoot string, patterns []string) ([]*packages.Package, error) {
	if moduleRoot == "" {
		return nil, fmt.Errorf("module root is required")
	}
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	cfg := &packages.Config{
		Mode:  packages.LoadSyntax | packages.NeedModule | packages.NeedDeps,
		Dir:   moduleRoot,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	_ = packages.PrintErrors(pkgs)
	return pkgs, nil
}

func flattenPackages(pkgs []*packages.Package) []*packages.Package {
	seen := make(map[string]*packages.Package)
	var walk func(*packages.Package)
	walk = func(pkg *packages.Package) {
		if pkg == nil {
			return
		}
		if _, ok := seen[pkg.ID]; ok {
			return
		}
		seen[pkg.ID] = pkg
		for _, imp := range pkg.Imports {
			walk(imp)
		}
	}
	for _, pkg := range pkgs {
		walk(pkg)
	}
	flat := make([]*packages.Package, 0, len(seen))
	for _, pkg := range seen {
		flat = append(flat, pkg)
	}
	return flat
}
