// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package e2e

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/tests/testutil"
)

func TestCLI_LintMocks(t *testing.T) {
	frameworks := []string{"chi", "echo", "gin", "fiber"}
	for _, framework := range frameworks {
		t.Run(framework, func(t *testing.T) {
			assertLintClean(t, framework)
		})
	}
}

func assertLintClean(t *testing.T, framework string) {
	t.Helper()
	root := testutil.RepoRoot(t)
	fixtureDir := testutil.MockDir(t, framework)

	bin := filepath.Join(t.TempDir(), "prism")
	build := exec.Command("go", "build", "-o", bin, "./cmd")
	build.Dir = root
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))

	lintCmd := exec.Command(bin, "lint", "--framework", framework, ".")
	lintCmd.Dir = fixtureDir
	out, err = lintCmd.CombinedOutput()
	require.NoError(t, err, string(out))
}
