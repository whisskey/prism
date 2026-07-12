// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

// Package lint reports OpenAPI inference issues from static analysis.
package lint

import (
	"fmt"
	"path/filepath"

	"github.com/whisskey/prism/internal/config"
	"github.com/whisskey/prism/internal/plugins"
)

// Options configures a lint run.
type Options struct {
	Framework   string
	Config      *config.Config
	Patterns    []string
	ProjectRoot string
}

// Run executes framework-aware lint analysis.
func Run(opts Options) ([]Issue, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if opts.ProjectRoot == "" {
		wd, err := filepath.Abs(".")
		if err != nil {
			return nil, err
		}
		opts.ProjectRoot = wd
	}

	framework := opts.Framework
	if framework == "" {
		framework = opts.Config.Framework
	}
	if framework == "" || framework == "auto" {
		plugin, err := plugins.Detect(opts.ProjectRoot)
		if err != nil {
			return nil, fmt.Errorf("framework detection failed: %w", err)
		}
		framework = plugin.Name()
	}

	switch framework {
	case "fiber":
		return lintFiber(opts.Patterns)
	default:
		plugin := plugins.Get(framework)
		if plugin == nil {
			return nil, fmt.Errorf("unknown framework %q", framework)
		}
		files, err := scanSourceFiles(opts.Config, opts.Patterns)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, nil
		}
		return LintPlugin(plugin, files)
	}
}
