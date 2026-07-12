// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package lint

import (
	"fmt"
	"go/token"
	"strings"

	"github.com/whisskey/prism/internal/plugins"
	"github.com/whisskey/prism/internal/scanner"
	"github.com/whisskey/prism/pkg/openapi"
)

var successStatusCodes = map[string]bool{
	"200": true,
	"201": true,
	"202": true,
	"204": true,
	"default": true,
}

// LintPlugin extracts routes with the framework plugin and reports inference gaps.
func LintPlugin(plugin plugins.FrameworkPlugin, files []scanner.SourceFile) ([]Issue, error) {
	if plugin == nil {
		return nil, fmt.Errorf("framework plugin is nil")
	}
	routes, err := plugin.ExtractRoutes(files)
	if err != nil {
		return nil, err
	}
	return IssuesFromRoutes(routes), nil
}

// IssuesFromRoutes returns lint issues for extracted routes.
func IssuesFromRoutes(routes []openapi.Route) []Issue {
	issues := make([]Issue, 0, len(routes))
	for _, route := range routes {
		pos := routePosition(route)
		issues = append(issues, routeIssues(route, pos)...)
	}
	return issues
}

func routeIssues(route openapi.Route, pos token.Position) []Issue {
	var issues []Issue

	if reason := route.HandlerRef.UnresolvedReason; reason != "" {
		issues = append(issues, Issue{
			Pos: pos,
			Message: fmt.Sprintf(
				"%s %s handler could not be resolved (%s)",
				route.Method, route.Path, reason,
			),
		})
	} else if route.DiscoveryConfidence == "low" {
		issues = append(issues, Issue{
			Pos: pos,
			Message: fmt.Sprintf(
				"%s %s has low discovery confidence — annotations may be required",
				route.Method, route.Path,
			),
		})
	}

	if methodAllowsRequestBody(route.Method) && route.RequestBody == nil {
		issues = append(issues, Issue{
			Pos: pos,
			Message: fmt.Sprintf(
				"%s %s has no inferred request body — add handler binding or @openapi annotations",
				route.Method, route.Path,
			),
		})
	}

	if len(route.Responses) == 0 {
		issues = append(issues, Issue{
			Pos: pos,
			Message: fmt.Sprintf(
				"%s %s has no inferred responses — add return types or @openapi annotations",
				route.Method, route.Path,
			),
		})
	} else if !hasSuccessResponse(route.Responses) {
		issues = append(issues, Issue{
			Pos: pos,
			Message: fmt.Sprintf(
				"%s %s has no success response (2xx) — add return types or @openapi annotations",
				route.Method, route.Path,
			),
		})
	}

	return issues
}

func routePosition(route openapi.Route) token.Position {
	file := route.SourceFile
	line := route.SourceLine
	if file == "" {
		file = route.HandlerRef.SourceFile
		line = route.HandlerRef.SourceLine
	}
	if line <= 0 {
		line = 1
	}
	return token.Position{Filename: file, Line: line}
}

func methodAllowsRequestBody(method string) bool {
	switch strings.ToUpper(method) {
	case "POST", "PUT", "PATCH":
		return true
	default:
		return false
	}
}

func hasSuccessResponse(responses map[string]openapi.Response) bool {
	for code := range responses {
		if successStatusCodes[code] {
			return true
		}
		if len(code) == 3 && code[0] == '2' {
			return true
		}
	}
	return false
}
