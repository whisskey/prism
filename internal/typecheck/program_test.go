// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package typecheck

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_PrismModule(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	root := FindModuleRoot([]string{filepath.Join(wd, "loader.go")})
	require.NotEmpty(t, root)

	prog, err := Load([]string{
		filepath.Join(root, "internal/plugins/fiber/fiber.go"),
		filepath.Join(root, "internal/typecheck/program.go"),
	})
	require.NoError(t, err)
	require.NotNil(t, prog)
	assert.NotEmpty(t, prog.Packages)
	assert.Greater(t, prog.FilesLoaded(), 0)
}
