// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

// Command plugin registers fibergen as a golangci-lint module plugin.
//
// Build for local golangci-lint custom binary integration:
//
//	go build -o prism-gcl ./internal/golangci/plugin
package main

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"github.com/whisskey/prism/internal/analyzer/fibergen"
)

type plugin struct{}

func (plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{fibergen.Analyzer}, nil
}

func (plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func init() {
	register.Plugin("prism", func(conf any) (register.LinterPlugin, error) {
		return plugin{}, nil
	})
}

func main() {}
