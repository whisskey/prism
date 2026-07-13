// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/pkg/openapi"
)

// semanticIndex groups the static facts the Fiber plugin needs to turn route
// definitions into API contracts. It mirrors Scramble's framework-aware pass:
// index first, then resolve route/request/response semantics from that index.
type semanticIndex struct {
	files       map[string]*parser.ParsedFile
	functions   map[string]*ast.FuncDecl
	methods     map[string]*ast.FuncDecl
	handlers    map[string]*ast.FuncDecl
	returnTypes map[string]string
}

// routeContract is the intermediate API contract inferred for a Fiber route.
type routeContract struct {
	route       openapi.Route
	request     *requestContract
	responses   map[string]openapi.Response
	usedSchemas map[string]bool
}

// requestContract describes how a Fiber route receives input.
type requestContract struct {
	bodySchema  string
	pathParams  map[string]*openapi.Schema
	queryParams map[string]*openapi.Schema
}

// responseContract describes one possible response emitted by a handler.
type responseContract struct {
	statusCode  string
	description string
	schema      *openapi.Schema
}
