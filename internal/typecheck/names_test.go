// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package typecheck

import (
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaName_Primitives(t *testing.T) {
	assert.Equal(t, "", SchemaName(types.Typ[types.String]))
	assert.Equal(t, "", SchemaName(types.Typ[types.Int]))
}

func TestSchemaName_Named(t *testing.T) {
	pkg := types.NewPackage("example.com/dto", "dto")
	named := types.NewNamed(
		types.NewTypeName(0, pkg, "RegisterRequest", nil),
		types.NewStruct(nil, nil),
		nil,
	)
	assert.Equal(t, "RegisterRequest", SchemaName(named))
	assert.Equal(t, "RegisterRequest", SchemaName(types.NewPointer(named)))
}

func TestFindModuleRoot(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	root := FindModuleRoot([]string{filepath.Join(wd, "loader.go")})
	require.NotEmpty(t, root)
	_, err = os.Stat(filepath.Join(root, "go.mod"))
	require.NoError(t, err)
}

func TestPackagePatterns(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	root := FindModuleRoot([]string{filepath.Join(wd, "loader.go")})
	require.NotEmpty(t, root)
	patterns := PackagePatterns(root, []string{
		filepath.Join(root, "internal/plugins/fiber/fiber.go"),
		filepath.Join(root, "internal/typecheck/program.go"),
	})
	assert.Contains(t, patterns, "./internal/plugins/fiber")
	assert.Contains(t, patterns, "./internal/typecheck")
}
