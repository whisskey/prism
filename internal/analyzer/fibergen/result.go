// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"fmt"
	"go/ast"
	"go/token"
)

// Result is the fibergen analyzer output for one package.
type Result struct {
	PkgPath  string
	PkgName  string
	Data     *CollectedData
	Handlers []HandlerInfo
	Calls    []CallInfo
	Structs  []StructInfo
}

// HandlerInfo describes a Fiber HTTP handler function.
type HandlerInfo struct {
	Name     string
	Position token.Position
	FuncDecl *ast.FuncDecl
}

// CallInfo describes a typed HTTP route registration call (e.g. app.Get).
type CallInfo struct {
	Method         string
	Position       token.Position
	CallExpr       *ast.CallExpr
	ReceiverType   string
	ReceiverSchema string
}

// StructInfo describes a Go struct type declaration.
type StructInfo struct {
	Name       string
	Position   token.Position
	TypeSpec   *ast.TypeSpec
	StructType *ast.StructType
}

// PosKey returns a stable position key for cross-AST lookup.
func PosKey(pos token.Position) string {
	return fmt.Sprintf("%s:%d:%d", pos.Filename, pos.Line, pos.Column)
}

// Index builds a position index of route registration calls.
type Index struct {
	Calls map[string]CallInfo
}

// NewIndex indexes call sites by file:line:column.
func NewIndex(results []*Result) *Index {
	idx := &Index{Calls: make(map[string]CallInfo)}
	for _, result := range results {
		if result == nil {
			continue
		}
		for _, call := range result.Calls {
			idx.Calls[PosKey(call.Position)] = call
		}
	}
	return idx
}

// LookupCall returns a typed call at the given position, if indexed.
func (idx *Index) LookupCall(pos token.Position) (CallInfo, bool) {
	if idx == nil {
		return CallInfo{}, false
	}
	call, ok := idx.Calls[PosKey(pos)]
	return call, ok
}

// TotalHandlers returns the number of handlers across all results.
func TotalHandlers(results []*Result) int {
	n := 0
	for _, r := range results {
		if r != nil {
			n += len(r.Handlers)
		}
	}
	return n
}
