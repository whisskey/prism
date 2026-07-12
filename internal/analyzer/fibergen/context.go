// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// Context carries shared state across analyzer passes.
type Context struct {
	Pass      *analysis.Pass
	Inspector *inspector.Inspector
	Raw       RawCollection
	Warn      *WarnLogger
}

// RawCollection is the output of the initial AST collection pass.
type RawCollection struct {
	Handlers []HandlerInfo
	Calls    []CallInfo
	Structs  []StructInfo
	Groups   []GroupInfo
}

// GroupInfo describes a router group prefix registration.
type GroupInfo struct {
	Prefix   string
	Position token.Position
	CallExpr *ast.CallExpr
}

// CollectedData is the combined output of all analyzer passes for one package.
type CollectedData struct {
	PkgPath         string
	PkgName         string
	Routes          []Route
	Handlers        map[string]HandlerDetail
	Schemas         map[string]Schema
	Security        map[string][]SecurityRequirement
	SecuritySchemes map[string]SecurityScheme
	OpenAPI         DocumentDraft
	Warnings        []Warning
}

// DocumentDraft holds Pass 5 output before YAML serialization.
type DocumentDraft struct {
	Title           string
	Version         string
	Routes          []Route
	Schemas         map[string]Schema
	SecuritySchemes map[string]SecurityScheme
}
