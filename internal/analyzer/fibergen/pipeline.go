// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"golang.org/x/tools/go/analysis"
)

// RunPipeline executes all analyzer passes in order.
func RunPipeline(pass *analysis.Pass) *CollectedData {
	return runPasses(CollectPass(pass))
}
