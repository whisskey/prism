// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package typecheck

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunFibergen_Module(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	root := FindModuleRoot([]string{wd})
	require.NotEmpty(t, root)

	pkgs, err := LoadPackages(root, []string{"./internal/plugins/fiber"})
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	results, err := RunFibergen(root, []string{"./internal/plugins/fiber"})
	require.NoError(t, err)
	require.NotEmpty(t, results)
	assert.Equal(t, "fiber", results[0].PkgName)
	require.NotNil(t, results[0].Data)
	assert.NotEmpty(t, results[0].Data.Schemas)
}
