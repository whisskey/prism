// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package lint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/whisskey/prism/internal/lint"
	"github.com/whisskey/prism/pkg/openapi"
)

func TestIssuesFromRoutes_UnresolvedHandler(t *testing.T) {
	issues := lint.IssuesFromRoutes([]openapi.Route{{
		Method: "GET",
		Path:   "/users",
		HandlerRef: openapi.HandlerRef{
			UnresolvedReason: "anonymous-handler",
		},
		SourceFile: "routes.go",
		SourceLine: 10,
	}})

	requireIssue(t, issues, "handler could not be resolved")
}

func TestIssuesFromRoutes_MissingRequestBody(t *testing.T) {
	issues := lint.IssuesFromRoutes([]openapi.Route{{
		Method:     "POST",
		Path:       "/users",
		Responses:  map[string]openapi.Response{"200": {Description: "ok"}},
		SourceFile: "routes.go",
		SourceLine: 12,
	}})

	requireIssue(t, issues, "no inferred request body")
}

func TestIssuesFromRoutes_MissingResponses(t *testing.T) {
	issues := lint.IssuesFromRoutes([]openapi.Route{{
		Method:     "GET",
		Path:       "/users",
		SourceFile: "routes.go",
		SourceLine: 14,
	}})

	requireIssue(t, issues, "no inferred responses")
}

func TestIssuesFromRoutes_NoSuccessResponse(t *testing.T) {
	issues := lint.IssuesFromRoutes([]openapi.Route{{
		Method: "GET",
		Path:   "/users",
		Responses: map[string]openapi.Response{
			"401": {Description: "unauthorized"},
		},
		SourceFile: "routes.go",
		SourceLine: 16,
	}})

	requireIssue(t, issues, "no success response")
}

func TestIssuesFromRoutes_CompleteRoute(t *testing.T) {
	issues := lint.IssuesFromRoutes([]openapi.Route{{
		Method:              "GET",
		Path:                "/users",
		DiscoveryConfidence: "high",
		Responses: map[string]openapi.Response{
			"200": {Description: "ok"},
		},
		SourceFile: "routes.go",
		SourceLine: 18,
	}})

	assert.Empty(t, issues)
}

func requireIssue(t *testing.T, issues []lint.Issue, contains string) {
	t.Helper()
	for _, issue := range issues {
		if assert.Contains(t, issue.Message, contains) {
			return
		}
	}
	t.Fatalf("expected issue containing %q, got %#v", contains, issues)
}
