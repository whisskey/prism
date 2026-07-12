// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package config

import (
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// RoutesConfig configures route discovery filtering (Layer 1).
type RoutesConfig struct {
	// IncludePrefixes limits output to routes whose path starts with one of these prefixes.
	IncludePrefixes []string `mapstructure:"includePrefixes" yaml:"includePrefixes" json:"includePrefixes"`

	// ExcludePaths removes routes matching these glob patterns (e.g. "/api/health").
	ExcludePaths []string `mapstructure:"excludePaths" yaml:"excludePaths" json:"excludePaths"`
}

// AllowsPath reports whether a discovered route path passes include/exclude filters.
func (c RoutesConfig) AllowsPath(path string) bool {
	if len(c.IncludePrefixes) > 0 {
		allowed := false
		for _, prefix := range c.IncludePrefixes {
			if strings.HasPrefix(path, prefix) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	for _, pattern := range c.ExcludePaths {
		matched, err := doublestar.Match(pattern, path)
		if err == nil && matched {
			return false
		}
	}

	return true
}
