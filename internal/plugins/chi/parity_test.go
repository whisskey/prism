// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package chi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/scanner"
	"github.com/whisskey/prism/pkg/openapi"
)

func TestExtractRoutes_PathParamIntegerTyping(t *testing.T) {
	source := `package routes

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func GetUser(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	_ = id
}

func Register(r chi.Router) {
	r.Get("/users/{id}", GetUser)
}
`

	p := New()
	routes, err := p.ExtractRoutes([]scanner.SourceFile{{Path: "routes.go", Language: "go", Content: []byte(source)}})
	require.NoError(t, err)
	require.Len(t, routes, 1)

	var idParam *openapi.Parameter
	for i := range routes[0].Parameters {
		if routes[0].Parameters[i].Name == "id" && routes[0].Parameters[i].In == "path" {
			idParam = &routes[0].Parameters[i]
			break
		}
	}
	require.NotNil(t, idParam)
	require.NotNil(t, idParam.Schema)
	assert.Equal(t, "integer", idParam.Schema.Type)
}

func TestExtractSchemas_FilteredByUsedSchemas(t *testing.T) {
	source := `package models

type User struct {
	ID   string ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}

type Unused struct {
	Field string ` + "`json:\"field\"`" + `
}
`

	routeSource := `package routes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Response struct {
	Data any ` + "`json:\"data\"`" + `
}

type User struct {
	ID   string ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}

func Success(w http.ResponseWriter, data any) {
	_ = json.NewEncoder(w).Encode(Response{Data: data})
}

func ListUsers(w http.ResponseWriter, r *http.Request) {
	users := []User{{ID: "1", Name: "Ada"}}
	Success(w, users)
}

func Register(r chi.Router) {
	r.Get("/users", ListUsers)
}
`

	p := New()
	_, err := p.ExtractRoutes([]scanner.SourceFile{
		{Path: "routes.go", Language: "go", Content: []byte(routeSource)},
	})
	require.NoError(t, err)

	schemas, err := p.ExtractSchemas([]scanner.SourceFile{
		{Path: "models.go", Language: "go", Content: []byte(source)},
		{Path: "routes.go", Language: "go", Content: []byte(routeSource)},
	})
	require.NoError(t, err)

	names := make(map[string]bool, len(schemas))
	for _, s := range schemas {
		names[s.Title] = true
	}
	assert.True(t, names["User"])
	assert.True(t, names["Response"])
	assert.False(t, names["Unused"])
}

func TestServiceReturnTypePropagation(t *testing.T) {
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

type UserService interface {
	GetUser(id string) (User, error)
}

type userService struct{}

func (s *userService) GetUser(id string) (User, error) {
	return User{ID: id}, nil
}

func Success(w http.ResponseWriter, data any) {
	_ = json.NewEncoder(w).Encode(Response{Data: data})
}

func GetUser(w http.ResponseWriter, r *http.Request) {
	var svc UserService = &userService{}
	user, _ := svc.GetUser("1")
	Success(w, user)
}

func Register(r chi.Router) {
	r.Get("/users/{id}", GetUser)
}
`

	p := New()
	routes, err := p.ExtractRoutes([]scanner.SourceFile{{Path: "routes.go", Language: "go", Content: []byte(source)}})
	require.NoError(t, err)
	require.Len(t, routes, 1)

	resp200, ok := routes[0].Responses["200"]
	require.True(t, ok)
	require.NotNil(t, resp200.Content)
	schema := resp200.Content["application/json"].Schema
	require.NotNil(t, schema)
	dataProp := schema.Properties["data"]
	require.NotNil(t, dataProp)
	assert.Equal(t, "#/components/schemas/User", dataProp.Ref)
}
