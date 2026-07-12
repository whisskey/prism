// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/whisskey/prism/internal/config"
	"github.com/whisskey/prism/internal/spec"
	oas "github.com/whisskey/prism/pkg/openapi"
)

var printCmd = &cobra.Command{
	Use:   "print [file]",
	Short: "Print the OpenAPI specification to stdout",
	Long: `Print the OpenAPI specification to standard output.

If a file is provided, it will print that file. Otherwise, it will
generate and print the specification from the current source code.

This is useful for piping the output to other tools or for quick inspection.

Example:
  prism print                      # Generate and print
  prism print openapi.yaml         # Print existing file
  prism print -f json              # Print in JSON format
  prism print | jq '.paths'        # Pipe to jq for processing`,
	RunE: runPrint,
}

func runPrint(cmd *cobra.Command, args []string) error {
	// Load config
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply command-line overrides
	if format != "" {
		cfg.Format = format
	}
	if framework != "" {
		cfg.Framework = framework
	}

	outputFormat := cfg.Format
	if outputFormat == "" {
		outputFormat = "yaml"
	}

	printVerbose("Print configuration:")
	printVerbose("  Format: %s", outputFormat)

	var doc *oas.OpenAPI

	if len(args) > 0 {
		// Print existing file
		filePath := args[0]
		printVerbose("Reading spec from: %s", filePath)

		// Check if file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", filePath)
		}

		doc, err = spec.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}
	} else {
		// Generate spec from code
		printVerbose("Generating spec from source code...")

		doc, err = generateSpecFromCode(cfg, cfg.Source.Paths)
		if err != nil {
			return fmt.Errorf("failed to generate spec: %w", err)
		}
	}

	// Write to stdout
	writer := spec.NewWriter()

	var output string
	switch outputFormat {
	case "json":
		output, err = writer.ToJSON(doc)
	default:
		output, err = writer.ToYAML(doc)
	}

	if err != nil {
		return fmt.Errorf("failed to serialize spec: %w", err)
	}

	// Print to stdout (without using printInfo to avoid newline issues)
	fmt.Print(output)

	return nil
}
