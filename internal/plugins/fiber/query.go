// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/pkg/openapi"
)

func queryParamFromCall(callExpr *ast.CallExpr) (string, *openapi.Schema) {
	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || len(callExpr.Args) == 0 {
		return "", nil
	}

	name, ok := parser.ExtractStringLiteral(callExpr.Args[0])
	if !ok || name == "" {
		return "", nil
	}

	switch selExpr.Sel.Name {
	case "Query":
		return name, &openapi.Schema{Type: "string"}
	case "QueryInt":
		return name, &openapi.Schema{Type: "integer"}
	case "QueryBool":
		return name, &openapi.Schema{Type: "boolean"}
	default:
		return "", nil
	}
}
