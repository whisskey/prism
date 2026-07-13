// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package typecheck

import (
	"github.com/whisskey/prism/internal/config"
	"github.com/whisskey/prism/internal/spec"
	oas "github.com/whisskey/prism/pkg/openapi"
)

// OpenAPIDocument assembles a full OpenAPI document from fibergen catalog output.
func (p *Program) OpenAPIDocument(cfg *config.Config) (*oas.OpenAPI, error) {
	if cfg == nil {
		cfg = config.Default()
	}
	if p == nil || p.FibergenCatalog == nil {
		return spec.NewAssembler(cfg).Assemble(nil, nil)
	}
	return spec.NewAssembler(cfg).AssembleFromFibergenCatalog(p.FibergenCatalog)
}
