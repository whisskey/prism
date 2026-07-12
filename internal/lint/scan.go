// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package lint

import (
	"path/filepath"
	"strings"

	"github.com/whisskey/prism/internal/config"
	"github.com/whisskey/prism/internal/scanner"
)

func scanSourceFiles(cfg *config.Config, patterns []string) ([]scanner.SourceFile, error) {
	scannerCfg := scanner.Config{
		IncludePatterns: cfg.Source.Include,
		ExcludePatterns: cfg.Source.Exclude,
	}

	var files []scanner.SourceFile
	for _, pattern := range patterns {
		basePath := pattern
		if strings.HasSuffix(basePath, "/...") {
			basePath = strings.TrimSuffix(basePath, "/...")
		} else if strings.HasSuffix(basePath, "...") {
			basePath = strings.TrimSuffix(basePath, "...")
			basePath = strings.TrimSuffix(basePath, "/")
		}
		absPath, err := filepath.Abs(basePath)
		if err != nil {
			return nil, err
		}
		scannerCfg.BasePath = absPath
		s := scanner.New(scannerCfg)
		pathFiles, err := s.Scan()
		if err != nil {
			return nil, err
		}
		files = append(files, pathFiles...)
	}
	return files, nil
}
