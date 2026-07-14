// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const mainModule = "module github.com/whisskey/prism"

// RepoRoot returns the prism repository root directory.
func RepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			data, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
			if strings.Contains(string(data), mainModule) {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}

// MockDir returns the path to a framework mock under tests/mock/.
func MockDir(t *testing.T, framework string) string {
	t.Helper()
	dir := filepath.Join(RepoRoot(t), "tests", "mock", framework)
	require.DirExists(t, dir)
	return dir
}

// AsString coerces a YAML-decoded value to string.
func AsString(v any) string {
	s, _ := v.(string)
	return s
}
