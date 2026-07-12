// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package cli

import (
	"github.com/whisskey/prism/internal/config"
	"github.com/whisskey/prism/internal/plugins"
)

type configurablePlugin interface {
	Configure(*config.Config)
}

func configurePlugin(plugin plugins.FrameworkPlugin, cfg *config.Config) {
	if configurable, ok := plugin.(configurablePlugin); ok {
		configurable.Configure(cfg)
	}
}
