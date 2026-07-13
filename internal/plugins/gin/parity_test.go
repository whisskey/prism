// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package gin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/scanner"
)

func TestExtractRoutes_QueryParamInference(t *testing.T) {
	source := `package routes

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

type PaginationQuery struct {
	Page    int ` + "`json:\"page\" query:\"page\"`" + `
	PerPage int ` + "`json:\"per_page\" query:\"per_page\"`" + `
}

func paginationQuery(c *gin.Context) (int, int) {
	var q PaginationQuery
	q.Page, _ = strconv.Atoi(c.Query("page"))
	q.PerPage, _ = strconv.Atoi(c.Query("per_page"))
	return q.Page, q.PerPage
}

func ListUsers(c *gin.Context) {
	_ = c.Query("page")
	_, _ = paginationQuery(c)
}

func Register(r *gin.Engine) {
	r.GET("/users", ListUsers)
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

	"github.com/gin-gonic/gin"
)

func GetUser(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	_ = id
}

func Register(r *gin.Engine) {
	r.GET("/users/:id", GetUser)
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

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type User struct {
	ID   string ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}

type Response struct {
	Data any ` + "`json:\"data\"`" + `
}

func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{Data: data})
}

func ListUsers(c *gin.Context) {
	users := []User{{ID: "1", Name: "Ada"}}
	Success(c, users)
}

func Register(r *gin.Engine) {
	r.GET("/users", ListUsers)
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

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

type User struct {
	ID   string ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}

type Response struct {
	Success bool ` + "`json:\"success\"`" + `
	Data    any  ` + "`json:\"data,omitempty\"`" + `
}

func Success(c *gin.Context, data any) {
	c.JSON(200, Response{Success: true, Data: data})
}

func paginationQuery(c *gin.Context) (int, int) {
	var q PaginationQuery
	q.Page, _ = strconv.Atoi(c.Query("page"))
	q.PerPage, _ = strconv.Atoi(c.Query("per_page"))
	return q.Page, q.PerPage
}

// ListUsers returns users.
// @openapi:summary List users
func ListUsers(c *gin.Context) {
	_, _ = paginationQuery(c)
	users := []User{{ID: "1", Name: "Ada"}}
	Success(c, users)
}

func main() {
	r := gin.New()
	r.GET("/api/v1/users", ListUsers)
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
