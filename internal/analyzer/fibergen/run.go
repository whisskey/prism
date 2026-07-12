// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"go/token"

	"golang.org/x/tools/go/analysis"
)

func run(pass *analysis.Pass) (any, error) {
	ctx := CollectPass(pass)
	data := runPasses(ctx)

	result := &Result{
		PkgPath:  pass.Pkg.Path(),
		PkgName:  pass.Pkg.Name(),
		Data:     data,
		Handlers: ctx.Raw.Handlers,
		Calls:    ctx.Raw.Calls,
		Structs:  ctx.Raw.Structs,
	}
	return result, nil
}

func runPasses(ctx Context) *CollectedData {
	routes := RouteCollectorPass(ctx)
	handlers := HandlerAnalyzerPass(ctx, routes)
	schemas := SchemaBuilderPass(ctx, handlers)
	security := AuthMapperPass(routes)

	data := &CollectedData{
		PkgPath:         ctx.Pass.Pkg.Path(),
		PkgName:         ctx.Pass.Pkg.Name(),
		Routes:          routes,
		Handlers:        handlers,
		Schemas:         schemas,
		Security:        security.Requirements,
		SecuritySchemes: security.Schemes,
	}
	data.OpenAPI = OpenAPIAssemblerPass(data)
	if ctx.Warn != nil {
		data.Warnings = ctx.Warn.All()
		reportWarnings(ctx.Pass, data.Warnings)
	}
	return data
}

func positionFromRoute(route Route) token.Position {
	return token.Position{Filename: route.SourceFile, Line: route.SourceLine}
}
