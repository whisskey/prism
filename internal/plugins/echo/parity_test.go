// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package echo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/scanner"
)

func TestExtractRoutes_QueryParamInference(t *testing.T) {
	source := `package routes

import "github.com/labstack/echo/v4"

type PaginationQuery struct {
	Page    int ` + "`json:\"page\" query:\"page\"`" + `
	PerPage int ` + "`json:\"per_page\" query:\"per_page\"`" + `
}

func paginationQuery(c echo.Context) (int, int) {
	var q PaginationQuery
	q.Page = c.QueryInt("page")
	q.PerPage = c.QueryInt("per_page")
	return q.Page, q.PerPage
}

func ListUsers(c echo.Context) error {
	_ = c.QueryParam("page")
	_, _ = paginationQuery(c)
	return nil
}

func Register(e *echo.Echo) {
	e.GET("/users", ListUsers)
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
	assert.True(t, names["page"])
	assert.True(t, names["per_page"])
}

func TestExtractRoutes_PathParamIntegerTyping(t *testing.T) {
	source := `package routes

import (
	"strconv"

	"github.com/labstack/echo/v4"
)

func GetUser(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	_ = id
	return nil
}

func Register(e *echo.Echo) {
	e.GET("/users/:id", GetUser)
}
`

	p := New()
	routes, err := p.ExtractRoutes([]scanner.SourceFile{{Path: "routes.go", Language: "go", Content: []byte(source)}})
	require.NoError(t, err)
	require.Len(t, routes, 1)

	for _, param := range routes[0].Parameters {
		if param.Name == "id" && param.In == "path" {
			require.NotNil(t, param.Schema)
			assert.Equal(t, "integer", param.Schema.Type)
			return
		}
	}
	t.Fatal("expected integer path param id")
}

func TestExtractRoutes_SuccessHelperEnvelope(t *testing.T) {
	source := `package routes

import "github.com/labstack/echo/v4"

type User struct {
	ID   string ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}

type Response struct {
	Data any ` + "`json:\"data\"`" + `
}

func Success(c echo.Context, data any) error {
	return c.JSON(http.StatusOK, Response{Data: data})
}

func ListUsers(c echo.Context) error {
	users := []User{{ID: "1", Name: "Ada"}}
	return Success(c, users)
}

func Register(e *echo.Echo) {
	e.GET("/users", ListUsers)
}
`

	p := New()
	routes, err := p.ExtractRoutes([]scanner.SourceFile{{Path: "routes.go", Language: "go", Content: []byte(source)}})
	require.NoError(t, err)
	require.Len(t, routes, 1)

	resp200, ok := routes[0].Responses["200"]
	require.True(t, ok)
	schema := resp200.Content["application/json"].Schema
	require.NotNil(t, schema)
	dataProp := schema.Properties["data"]
	require.NotNil(t, dataProp)
	assert.Equal(t, "array", dataProp.Type)
}

func TestExtractRoutes_ListUsersWithPaginationHelper(t *testing.T) {
	source := `package main

import "github.com/labstack/echo/v4"

type User struct {
	ID   string ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}

type Response struct {
	Success bool ` + "`json:\"success\"`" + `
	Data    any  ` + "`json:\"data,omitempty\"`" + `
}

func Success(c echo.Context, data any) error {
	return c.JSON(200, Response{Success: true, Data: data})
}

func paginationQuery(c echo.Context) (int, int) {
	return c.QueryInt("page"), c.QueryInt("per_page")
}

// ListUsers returns users.
// @openapi:summary List users
func ListUsers(c echo.Context) error {
	_, _ = paginationQuery(c)
	users := []User{{ID: "1", Name: "Ada"}}
	return Success(c, users)
}

func main() {
	e := echo.New()
	e.GET("/api/v1/users", ListUsers)
}
`

	p := New()
	routes, err := p.ExtractRoutes([]scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}})
	require.NoError(t, err)
	require.Len(t, routes, 1)
	assert.Equal(t, "List users", routes[0].Summary)
	_, ok := routes[0].Responses["200"]
	assert.True(t, ok, "expected 200 response for ListUsers")
}
