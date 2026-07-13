// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

// Package main is the entry point for the prism CLI.
package main

import (
	"fmt"
	"os"

	"github.com/whisskey/prism/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
