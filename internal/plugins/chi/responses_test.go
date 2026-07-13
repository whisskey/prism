// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package chi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/scanner"
)

func TestExtractRoutes_SuccessHelperSliceDataArray(t *testing.T) {
	source := `package routes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type User struct {
	ID   string ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}

type Response struct {
	Data any ` + "`json:\"data\"`" + `
}

func Success(w http.ResponseWriter, data any) {
	_ = json.NewEncoder(w).Encode(Response{Data: data})
}

func ListUsers(w http.ResponseWriter, r *http.Request) {
	users := []User{}
	Success(w, users)
}

func Register(r chi.Router) {
	r.Get("/users", ListUsers)
}
`

	p := New()
	files := []scanner.SourceFile{{
		Path:     "routes.go",
		Language: "go",
		Content:  []byte(source),
	}}
	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 1)

	route := routes[0]
	require.NotNil(t, route.Responses)
	ok := route.Responses["200"]
	require.NotNil(t, ok.Content)
	schema := ok.Content["application/json"].Schema
	require.NotNil(t, schema)
	require.NotNil(t, schema.Properties)
	data := schema.Properties["data"]
	require.NotNil(t, data)
	assert.Equal(t, "array", data.Type)
	require.NotNil(t, data.Items)
	assert.Equal(t, "#/components/schemas/User", data.Items.Ref)
}
