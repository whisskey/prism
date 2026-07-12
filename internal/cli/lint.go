// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisskey/prism/internal/config"
	"github.com/whisskey/prism/internal/lint"
	_ "github.com/whisskey/prism/internal/plugins/chi"   // Register chi plugin
	_ "github.com/whisskey/prism/internal/plugins/echo"  // Register echo plugin
	_ "github.com/whisskey/prism/internal/plugins/fiber" // Register fiber plugin
	_ "github.com/whisskey/prism/internal/plugins/gin"   // Register gin plugin
)

var (
	lintWithGolangci bool
	lintStrict       bool
)

var lintCmd = &cobra.Command{
	Use:   "lint [paths...]",
	Short: "Report OpenAPI inference issues",
	Long: `Run static analysis and report OpenAPI inference warnings.

Uses fibergen for Fiber projects and route extraction for chi, gin, and echo.

Use --with-golangci to also run golangci-lint with the prism plugin when installed
(Fiber only).

Example:
  prism lint ./internal/...
  prism lint --framework chi ./...
  prism lint --with-golangci ./...`,
	RunE: runLint,
}

func init() {
	lintCmd.Flags().BoolVar(&lintWithGolangci, "with-golangci", false, "also run golangci-lint when available (Fiber)")
	lintCmd.Flags().BoolVar(&lintStrict, "strict", false, "exit with error when warnings are found")
}

func runLint(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if framework != "" {
		cfg.Framework = framework
	}

	patterns := lintPatterns(args, cfg)
	projectRoot, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("failed to determine project root: %w", err)
	}

	issues, err := lint.Run(lint.Options{
		Framework:   cfg.Framework,
		Config:      cfg,
		Patterns:    patterns,
		ProjectRoot: projectRoot,
	})
	if err != nil {
		return fmt.Errorf("lint failed: %w", err)
	}

	if len(issues) == 0 {
		printInfo("No inference warnings found")
	} else {
		for _, issue := range issues {
			if issue.Pos.IsValid() {
				fmt.Fprintf(os.Stderr, "%s:%d: %s\n", issue.Pos.Filename, issue.Pos.Line, issue.Message)
			} else {
				fmt.Fprintf(os.Stderr, "warning: %s\n", issue.Message)
			}
		}
		printInfo("Found %d inference warning(s)", len(issues))
	}

	if lintWithGolangci {
		if cfg.Framework != "" && cfg.Framework != "auto" && cfg.Framework != "fiber" {
			printInfo("Skipping golangci-lint plugin (Fiber only)")
		} else if err := runGolangciLint(patterns); err != nil {
			return err
		}
	}

	if lintStrict && len(issues) > 0 {
		return fmt.Errorf("%d inference warning(s) found", len(issues))
	}
	return nil
}

func lintPatterns(args []string, cfg *config.Config) []string {
	if len(args) > 0 {
		return args
	}
	paths := cfg.Source.Paths
	if len(paths) == 0 {
		return []string{"./..."}
	}
	patterns := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if strings.HasSuffix(path, "/") {
			path = path + "..."
		} else if !strings.Contains(path, "...") {
			info, err := os.Stat(path)
			if err == nil && info.IsDir() {
				path = filepath.Join(path, "...")
			}
		}
		patterns = append(patterns, path)
	}
	if len(patterns) == 0 {
		return []string{"./..."}
	}
	return patterns
}

func runGolangciLint(patterns []string) error {
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		return fmt.Errorf("golangci-lint not found in PATH")
	}
	args := append([]string{"run"}, patterns...)
	cmd := exec.Command("golangci-lint", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	printInfo("Running golangci-lint %s", strings.Join(args, " "))
	return cmd.Run()
}
