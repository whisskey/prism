// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoutesConfig_AllowsPath(t *testing.T) {
	t.Run("no filters allows all", func(t *testing.T) {
		cfg := RoutesConfig{}
		assert.True(t, cfg.AllowsPath("/api/foo"))
		assert.True(t, cfg.AllowsPath("/health"))
	})

	t.Run("include prefix", func(t *testing.T) {
		cfg := RoutesConfig{IncludePrefixes: []string{"/api"}}
		assert.True(t, cfg.AllowsPath("/api/foo"))
		assert.False(t, cfg.AllowsPath("/health"))
	})

	t.Run("exclude path", func(t *testing.T) {
		cfg := RoutesConfig{
			IncludePrefixes: []string{"/api"},
			ExcludePaths:    []string{"/api/health"},
		}
		assert.True(t, cfg.AllowsPath("/api/foo"))
		assert.False(t, cfg.AllowsPath("/api/health"))
	})
}
