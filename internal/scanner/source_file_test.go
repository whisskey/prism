// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package scanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"main.go", "go"},
		{"handler.Go", "go"},
		{"app.ts", ""},
		{"utils.js", ""},
		{"readme.md", ""},
		{"config.yaml", ""},
		{"Makefile", ""},
		{"/path/to/file.go", "go"},
		{"/path/to/file.ts", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := DetectLanguage(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSupportedExtensions(t *testing.T) {
	exts := SupportedExtensions()

	assert.NotEmpty(t, exts)
	assert.Contains(t, exts, ".go")
	assert.NotContains(t, exts, ".ts")
	assert.NotContains(t, exts, ".js")
}

func TestIsSupportedFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"main.go", true},
		{"app.ts", false},
		{"utils.js", false},
		{"readme.md", false},
		{"config.yaml", false},
		{"Makefile", false},
		{"image.png", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := IsSupportedFile(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}
