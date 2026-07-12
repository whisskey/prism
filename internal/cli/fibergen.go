// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package cli

import (
	"github.com/whisskey/prism/internal/analyzer/fibergen"
	"github.com/whisskey/prism/internal/plugins"
)

func fibergenCatalogFromPlugin(plugin plugins.FrameworkPlugin) *fibergen.Catalog {
	if plugin == nil {
		return nil
	}
	provider, ok := plugin.(plugins.FibergenProvider)
	if !ok {
		return nil
	}
	return provider.FibergenCatalog()
}
