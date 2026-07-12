// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen_test

import (
	"os"
	"testing"

	"github.com/whisskey/prism/internal/analyzer/fibergen"
	"github.com/whisskey/prism/internal/typecheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunFibergen_FiberPlugin(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	root := typecheck.FindModuleRoot([]string{wd})
	require.NotEmpty(t, root)

	results, err := typecheck.RunFibergen(root, []string{"./internal/plugins/fiber"})
	require.NoError(t, err)
	require.NotEmpty(t, results)

	var schemas int
	for _, result := range results {
		assert.Equal(t, "fiber", result.PkgName)
		require.NotNil(t, result.Data)
		schemas += len(result.Data.Schemas)
	}
	assert.Greater(t, schemas, 0)
}

func TestAnalyzerRegistered(t *testing.T) {
	assert.Equal(t, "fibergen", fibergen.Analyzer.Name)
	require.NotNil(t, fibergen.Analyzer.Run)
	require.NotEmpty(t, fibergen.Analyzer.Requires)
}
