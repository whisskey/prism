// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package plugins

import (
	"github.com/whisskey/prism/internal/analyzer/fibergen"
)

// FibergenProvider is implemented by plugins that run the fibergen analyzer.
type FibergenProvider interface {
	FibergenCatalog() *fibergen.Catalog
}
