// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package chi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/scanner"
)

func TestExtractRoutes_QueryParamInference(t *testing.T) {
	source := `package routes

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type PaginationQuery struct {
	Page    int ` + "`json:\"page\" query:\"page\"`" + `
	PerPage int ` + "`json:\"per_page\" query:\"per_page\"`" + `
}

func paginationQuery(r *http.Request) (int, int) {
	var q PaginationQuery
	q.Page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	q.PerPage, _ = strconv.Atoi(r.URL.Query().Get("per_page"))
	return q.Page, q.PerPage
}

func ListUsers(w http.ResponseWriter, r *http.Request) {
	page := r.URL.Query().Get("page")
	_ = page
	_, _ = paginationQuery(r)
}

func Register(r chi.Router) {
	r.Get("/users", ListUsers)
}
`

	p := New()
	routes, err := p.ExtractRoutes([]scanner.SourceFile{{
		Path:     "routes.go",
		Language: "go",
		Content:  []byte(source),
	}})
	require.NoError(t, err)
	require.Len(t, routes, 1)

	names := map[string]bool{}
	for _, param := range routes[0].Parameters {
		if param.In == "query" {
			names[param.Name] = true
		}
	}
	assert.True(t, names["page"], "expected page query param")
	assert.True(t, names["per_page"], "expected per_page query param")
}
