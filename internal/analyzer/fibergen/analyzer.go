// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"reflect"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

// Analyzer walks typed Go AST nodes to collect Fiber-relevant declarations.
var Analyzer = &analysis.Analyzer{
	Name:       "fibergen",
	Doc:        "collects Fiber handlers, route calls, and struct definitions for OpenAPI generation",
	Run:        run,
	Requires:   []*analysis.Analyzer{inspect.Analyzer},
	ResultType: reflect.TypeFor[*Result](),
}
