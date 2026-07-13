// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"go/ast"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/config"
	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/internal/scanner"
	"github.com/whisskey/prism/pkg/openapi"
)

func TestPlugin_Name(t *testing.T) {
	p := New()
	assert.Equal(t, "fiber", p.Name())
}

func TestPlugin_Extensions(t *testing.T) {
	p := New()
	assert.Equal(t, []string{".go"}, p.Extensions())
}

func TestPlugin_Info(t *testing.T) {
	p := New()
	info := p.Info()

	assert.Equal(t, "fiber", info.Name)
	assert.NotEmpty(t, info.Version)
	assert.NotEmpty(t, info.Description)
	assert.Contains(t, info.SupportedFrameworks, "github.com/gofiber/fiber/v2")
}

func TestPlugin_Detect(t *testing.T) {
	tests := []struct {
		name         string
		goModContent string
		wantDetect   bool
	}{
		{
			name: "fiber v2 detected",
			goModContent: `module example.com/myapp

go 1.21

require (
	github.com/gofiber/fiber/v2 v2.50.0
)
`,
			wantDetect: true,
		},
		{
			name: "fiber v3 detected",
			goModContent: `module example.com/myapp

go 1.21

require (
	github.com/gofiber/fiber/v3 v3.0.0
)
`,
			wantDetect: true,
		},
		{
			name: "fiber v1 detected",
			goModContent: `module example.com/myapp

go 1.21

require (
	github.com/gofiber/fiber v1.14.0
)
`,
			wantDetect: true,
		},
		{
			name: "no fiber",
			goModContent: `module example.com/myapp

go 1.21

require (
	github.com/gin-gonic/gin v1.9.0
)
`,
			wantDetect: false,
		},
		{
			name: "empty go.mod",
			goModContent: `module example.com/myapp

go 1.21
`,
			wantDetect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			goModPath := filepath.Join(tmpDir, "go.mod")
			err := os.WriteFile(goModPath, []byte(tt.goModContent), 0644)
			require.NoError(t, err)

			p := New()
			detected, err := p.Detect(tmpDir)

			require.NoError(t, err)
			assert.Equal(t, tt.wantDetect, detected)
		})
	}
}

func TestPlugin_Detect_NoGoMod(t *testing.T) {
	tmpDir := t.TempDir()

	p := New()
	detected, err := p.Detect(tmpDir)

	require.NoError(t, err)
	assert.False(t, detected)
}

func TestPlugin_ExtractRoutes_Basic(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Get("/users", ListUsers)
	app.Post("/users", CreateUser)
	app.Get("/users/:id", GetUser)
	app.Put("/users/:id", UpdateUser)
	app.Delete("/users/:id", DeleteUser)
	app.Patch("/users/:id/status", UpdateUserStatus)
}
`

	p := New()
	files := []scanner.SourceFile{
		{
			Path:     "routes.go",
			Language: "go",
			Content:  []byte(source),
		},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 6)

	// Verify GET /users
	assert.Equal(t, "GET", routes[0].Method)
	assert.Equal(t, "/users", routes[0].Path)
	assert.Equal(t, "ListUsers", routes[0].Handler)
	assert.Contains(t, routes[0].Tags, "users")

	// Verify POST /users
	assert.Equal(t, "POST", routes[1].Method)
	assert.Equal(t, "/users", routes[1].Path)
	assert.Equal(t, "CreateUser", routes[1].Handler)

	// Verify GET /users/{id}
	assert.Equal(t, "GET", routes[2].Method)
	assert.Equal(t, "/users/{id}", routes[2].Path)
	require.Len(t, routes[2].Parameters, 1)
	assert.Equal(t, "id", routes[2].Parameters[0].Name)
	assert.Equal(t, "path", routes[2].Parameters[0].In)
	assert.True(t, routes[2].Parameters[0].Required)

	// Verify PUT /users/{id}
	assert.Equal(t, "PUT", routes[3].Method)
	assert.Equal(t, "/users/{id}", routes[3].Path)

	// Verify DELETE /users/{id}
	assert.Equal(t, "DELETE", routes[4].Method)
	assert.Equal(t, "/users/{id}", routes[4].Path)

	// Verify PATCH /users/{id}/status
	assert.Equal(t, "PATCH", routes[5].Method)
	assert.Equal(t, "/users/{id}/status", routes[5].Path)
	assert.Len(t, routes[5].Parameters, 1)
}

func TestPlugin_ExtractRoutes_Groups(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	api := app.Group("/api")
	api.Get("/health", HealthCheck)

	v1 := api.Group("/v1")
	v1.Get("/users", ListUsers)
	v1.Post("/users", CreateUser)
}
`

	p := New()
	files := []scanner.SourceFile{
		{
			Path:     "routes.go",
			Language: "go",
			Content:  []byte(source),
		},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 3)

	// Verify /api/health
	assert.Equal(t, "GET", routes[0].Method)
	assert.Equal(t, "/api/health", routes[0].Path)

	// Verify /api/v1/users GET
	assert.Equal(t, "GET", routes[1].Method)
	assert.Equal(t, "/api/v1/users", routes[1].Path)
	assert.Contains(t, routes[1].Tags, "users")

	// Verify /api/v1/users POST
	assert.Equal(t, "POST", routes[2].Method)
	assert.Equal(t, "/api/v1/users", routes[2].Path)
}

func TestPlugin_ExtractRoutes_RouteCallback(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Route("/users", func(router fiber.Router) {
		router.Get("/", ListUsers)
		router.Post("/", CreateUser)
		router.Get("/:id", GetUser)
	})
}
`

	p := New()
	files := []scanner.SourceFile{
		{
			Path:     "routes.go",
			Language: "go",
			Content:  []byte(source),
		},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 3)

	// Verify /users
	assert.Equal(t, "GET", routes[0].Method)
	assert.Equal(t, "/users", routes[0].Path)

	// Verify /users POST
	assert.Equal(t, "POST", routes[1].Method)
	assert.Equal(t, "/users", routes[1].Path)

	// Verify /users/{id}
	assert.Equal(t, "GET", routes[2].Method)
	assert.Equal(t, "/users/{id}", routes[2].Path)
}

func TestPlugin_ExtractRoutes_MethodHandler(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

type API struct{}

func SetupRoutes(app *fiber.App, api *API) {
	app.Get("/users", api.ListUsers)
	app.Get("/health", handlers.HealthCheck)
}
`

	p := New()
	files := []scanner.SourceFile{
		{
			Path:     "routes.go",
			Language: "go",
			Content:  []byte(source),
		},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 2)

	assert.Equal(t, "api.ListUsers", routes[0].Handler)
	assert.Equal(t, "handlers.HealthCheck", routes[1].Handler)
}

func TestPlugin_ExtractRoutes_AnonymousHandler(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Get("/inline", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})
}
`

	p := New()
	files := []scanner.SourceFile{
		{
			Path:     "routes.go",
			Language: "go",
			Content:  []byte(source),
		},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 1)

	assert.Equal(t, "<anonymous>", routes[0].Handler)
}

func TestPlugin_ExtractRoutes_NoFiberImport(t *testing.T) {
	source := `package main

import "net/http"

func Handler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}
`

	p := New()
	files := []scanner.SourceFile{
		{
			Path:     "handler.go",
			Language: "go",
			Content:  []byte(source),
		},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	assert.Empty(t, routes)
}

func TestPlugin_ExtractRoutes_NonGoFiles(t *testing.T) {
	p := New()
	files := []scanner.SourceFile{
		{
			Path:     "main.ts",
			Language: "typescript",
			Content:  []byte("export const foo = 1"),
		},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	assert.Empty(t, routes)
}

func TestPlugin_ExtractRoutes_AllHTTPMethods(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Get("/get", Handler)
	app.Post("/post", Handler)
	app.Put("/put", Handler)
	app.Delete("/delete", Handler)
	app.Patch("/patch", Handler)
	app.Head("/head", Handler)
	app.Options("/options", Handler)
	app.All("/all", Handler)
}
`

	p := New()
	files := []scanner.SourceFile{
		{
			Path:     "routes.go",
			Language: "go",
			Content:  []byte(source),
		},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 8)

	methods := make(map[string]bool)
	for _, r := range routes {
		methods[r.Method] = true
	}

	assert.True(t, methods["GET"])
	assert.True(t, methods["POST"])
	assert.True(t, methods["PUT"])
	assert.True(t, methods["DELETE"])
	assert.True(t, methods["PATCH"])
	assert.True(t, methods["HEAD"])
	assert.True(t, methods["OPTIONS"])
	assert.True(t, methods["ALL"])
}

func TestPlugin_ExtractRoutes_Wildcard(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Get("/files/*", ServeFiles)
	app.All("/api/*", APIHandler)
}
`

	p := New()
	files := []scanner.SourceFile{
		{
			Path:     "routes.go",
			Language: "go",
			Content:  []byte(source),
		},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 2)

	// Wildcard should be converted to {path}
	assert.Equal(t, "/files/{path}", routes[0].Path)
	assert.Equal(t, "/api/{path}", routes[1].Path)
}

func TestPlugin_ExtractRoutes_SourceInfo(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Get("/users", ListUsers)
}
`

	p := New()
	files := []scanner.SourceFile{
		{
			Path:     "/path/to/routes.go",
			Language: "go",
			Content:  []byte(source),
		},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 1)

	assert.Equal(t, "/path/to/routes.go", routes[0].SourceFile)
	assert.Greater(t, routes[0].SourceLine, 0)
}

func TestPlugin_ExtractSchemas(t *testing.T) {
	source := `package models

import "time"

// User represents a user in the system.
type User struct {
	ID        string    ` + "`json:\"id\"`" + `
	Name      string    ` + "`json:\"name\" validate:\"required\"`" + `
	Email     string    ` + "`json:\"email\" validate:\"required,email\"`" + `
	Age       *int      ` + "`json:\"age,omitempty\"`" + `
	Tags      []string  ` + "`json:\"tags\"`" + `
	CreatedAt time.Time ` + "`json:\"created_at\"`" + `
}

type Address struct {
	Street string ` + "`json:\"street\"`" + `
	City   string ` + "`json:\"city\"`" + `
}
`

	p := New()
	files := []scanner.SourceFile{
		{
			Path:     "models.go",
			Language: "go",
			Content:  []byte(source),
		},
	}

	schemas, err := p.ExtractSchemas(files)
	require.NoError(t, err)
	require.Len(t, schemas, 2)

	// Find User schema
	for _, s := range schemas {
		if s.Title == "User" {
			assert.Equal(t, "object", s.Type)
			assert.Contains(t, s.Required, "name")
			assert.Contains(t, s.Required, "email")
			assert.NotContains(t, s.Required, "age")

			// Check properties
			assert.NotNil(t, s.Properties["id"])
			assert.Equal(t, "string", s.Properties["id"].Type)

			assert.NotNil(t, s.Properties["email"])
			assert.Equal(t, "email", s.Properties["email"].Format)

			assert.NotNil(t, s.Properties["tags"])
			assert.Equal(t, "array", s.Properties["tags"].Type)

			assert.NotNil(t, s.Properties["created_at"])
			assert.Equal(t, "date-time", s.Properties["created_at"].Format)
			break
		}
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/users", "/users"},
		{"users", "/users"},
		{"/users/", "/users"},
		{"/users//posts", "/users/posts"},
		{"/", "/"},
		{"", "/"},
		{"//users", "/users"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizePath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertFiberPathParams(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/users", "/users"},
		{"/users/:id", "/users/{id}"},
		{"/users/:userId/posts/:postId", "/users/{userId}/posts/{postId}"},
		{"/files/*", "/files/{path}"},
		{"/:a/:b/:c", "/{a}/{b}/{c}"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := convertFiberPathParams(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractPathParams(t *testing.T) {
	tests := []struct {
		path       string
		paramNames []string
	}{
		{"/users", nil},
		{"/users/{id}", []string{"id"}},
		{"/users/{userId}/posts/{postId}", []string{"userId", "postId"}},
		{"/files/{path}", []string{"path"}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			params := extractPathParams(tt.path)

			if tt.paramNames == nil {
				assert.Empty(t, params)
				return
			}

			require.Len(t, params, len(tt.paramNames))
			for i, name := range tt.paramNames {
				assert.Equal(t, name, params[i].Name)
				assert.Equal(t, "path", params[i].In)
				assert.True(t, params[i].Required)
			}
		})
	}
}

func TestGenerateOperationID(t *testing.T) {
	tests := []struct {
		method  string
		path    string
		handler string
		want    string
	}{
		{"GET", "/users", "ListUsers", "getListUsers"},
		{"POST", "/users", "CreateUser", "postCreateUser"},
		{"GET", "/users/{id}", "GetUser", "getGetUser"},
		{"DELETE", "/users/{id}", "", "deleteUsersByid"},
		{"GET", "/", "", "get"},
		{"GET", "/api/v1/users", "ListUsers", "getListUsers"},
		{"GET", "/users", "<anonymous>", "getUsers"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			result := generateOperationID(tt.method, tt.path, tt.handler)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestInferTags(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"/users", []string{"users"}},
		{"/users/{id}", []string{"users"}},
		{"/api/users", []string{"users"}},
		{"/api/v1/users", []string{"users"}},
		{"/v1/orders", []string{"orders"}},
		{"/", nil},
		{"/{id}", nil},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := inferTags(tt.path)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestPlugin_ExtractRoutes_NestedGroups(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	api := app.Group("/api")
	v1 := api.Group("/v1")
	users := v1.Group("/users")

	users.Get("/", ListUsers)
	users.Get("/:id", GetUser)
	users.Post("/", CreateUser)
}
`

	p := New()
	files := []scanner.SourceFile{
		{
			Path:     "routes.go",
			Language: "go",
			Content:  []byte(source),
		},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 3)

	// All routes should have the full nested prefix
	assert.Equal(t, "/api/v1/users", routes[0].Path)
	assert.Equal(t, "/api/v1/users/{id}", routes[1].Path)
	assert.Equal(t, "/api/v1/users", routes[2].Path)
}

func TestPlugin_ExtractRoutes_FiberSpecMVP(t *testing.T) {
	routesSource := `package main

import "github.com/gofiber/fiber/v2"

type FooRouter struct {
	handler *FooHandler
}

func SetupRoutes(app *fiber.App, fooRouter *FooRouter) {
	api := app.Group("/api")
	fooRouter.Register(api)
}

func (r *FooRouter) Register(router fiber.Router) {
	router.Get("/foo", r.handler.GetAll)
	router.Post("/foo", r.handler.Create)
	router.Get("/foo/:id", r.handler.GetByID)
	router.Put("/foo/:id", r.handler.Update)
}
`

	handlerSource := `package main

import "github.com/gofiber/fiber/v2"

type FooHandler struct{}

func (h *FooHandler) GetAll(c *fiber.Ctx) error {
	foos := []Foo{}
	return c.JSON(foos)
}

func (h *FooHandler) Create(c *fiber.Ctx) error {
	var req CreateFooRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{})
	}
	foo := Foo{}
	return c.JSON(foo)
}

func (h *FooHandler) GetByID(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{})
	}
	foo := Foo{}
	return c.JSON(foo)
}

func (h *FooHandler) Update(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{})
	}
	req := new(UpdateFooRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{})
	}
	foo := Foo{}
	return c.JSON(foo)
}
`

	modelsSource := `package main

import "github.com/gofiber/fiber/v2"

type Foo struct {
	ID int ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}

type CreateFooRequest struct {
	Name string ` + "`json:\"name\" validate:\"required,min=3,max=50\"`" + `
}

type UpdateFooRequest struct {
	ID int ` + "`json:\"id\" validate:\"required\"`" + `
	Name string ` + "`json:\"name\" validate:\"required,min=3,max=50\"`" + `
}

type ErrorResponse struct {
	Message string ` + "`json:\"message\"`" + `
	Status StatusType ` + "`json:\"status\"`" + `
}

type StatusType string

type FooRepository struct {
	db any
}

type AppConfig struct {
	Host string
}
`

	p := New()
	files := []scanner.SourceFile{
		{Path: "routes.go", Language: "go", Content: []byte(routesSource)},
		{Path: "handler.go", Language: "go", Content: []byte(handlerSource)},
		{Path: "models.go", Language: "go", Content: []byte(modelsSource)},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 4)

	byMethodPath := make(map[string]openapi.Route)
	for _, route := range routes {
		byMethodPath[route.Method+" "+route.Path] = route
	}

	getAll := byMethodPath["GET /api/foo"]
	require.NotNil(t, getAll.Responses)
	assert.Equal(t, "array", getAll.Responses["200"].Content["application/json"].Schema.Type)
	assert.Equal(t, "#/components/schemas/Foo", getAll.Responses["200"].Content["application/json"].Schema.Items.Ref)
	assert.NotContains(t, getAll.Responses, "400")

	create := byMethodPath["POST /api/foo"]
	require.NotNil(t, create.RequestBody)
	assert.Equal(t, "#/components/schemas/CreateFooRequest", create.RequestBody.Content["application/json"].Schema.Ref)
	assert.Equal(t, "#/components/schemas/Foo", create.Responses["200"].Content["application/json"].Schema.Ref)
	assert.Equal(t, "#/components/schemas/ErrorResponse", create.Responses["400"].Content["application/json"].Schema.Ref)

	getByID := byMethodPath["GET /api/foo/{id}"]
	require.Len(t, getByID.Parameters, 1)
	assert.Equal(t, "integer", getByID.Parameters[0].Schema.Type)

	update := byMethodPath["PUT /api/foo/{id}"]
	require.NotNil(t, update.RequestBody)
	assert.Equal(t, "#/components/schemas/UpdateFooRequest", update.RequestBody.Content["application/json"].Schema.Ref)
	assert.Equal(t, "integer", update.Parameters[0].Schema.Type)

	schemas, err := p.ExtractSchemas(files)
	require.NoError(t, err)

	schemaNames := make(map[string]bool)
	for _, schema := range schemas {
		schemaNames[schema.Title] = true
	}

	assert.True(t, schemaNames["Foo"])
	assert.True(t, schemaNames["CreateFooRequest"])
	assert.True(t, schemaNames["UpdateFooRequest"])
	assert.True(t, schemaNames["ErrorResponse"])
	assert.True(t, schemaNames["StatusType"])
	assert.False(t, schemaNames["FooRepository"])
	assert.False(t, schemaNames["AppConfig"])
}

func TestPlugin_ExtractRoutes_FiberScrambleQuality(t *testing.T) {
	routesSource := `package main

import "github.com/gofiber/fiber/v2"

type FooRouter struct {
	handler *FooHandler
}

func SetupRoutes(app *fiber.App, fooRouter *FooRouter) {
	api := app.Group("/api")
	fooRouter.SetupRoutes(api)
}

func (r *FooRouter) SetupRoutes(router fiber.Router) {
	foo := router.Group("/foo")
	foo.Use(auth.RequireRole(), limiter.RateLimit(), validator.Validation422())
	foo.Get("/", auth.RequireAuth(), r.handler.GetAll)
	foo.Get("/concrete", r.handler.GetConcrete)
	foo.Get("/forward", r.handler.GetForward)
	foo.Get("/custom-error", r.handler.CustomError)
	foo.Get("/send-status", r.handler.SendStatus)
	foo.Get("/:id", r.handler.GetByID)
	foo.Post("/", middleware.RequestValidator[CreateFooRequest](), r.handler.Create)
	foo.Put("/:id", middleware.RequestValidator[UpdateFooRequest](), r.handler.Update)
	foo.Delete("/:id", middleware.RequestValidator[IDParamRequest](), r.handler.Delete)
}
`

	handlerSource := `package main

import "github.com/gofiber/fiber/v2"

type FooHandler struct {
	service IFooService
	concrete *FooService
}

type IFooService interface {
	GetAll(c any) ([]*Foo, error)
	Create(c any, req *CreateFooRequest) (*Foo, error)
	Update(c any, req *UpdateFooRequest) (*Foo, error)
	Delete(c any, id uint) (*Foo, error)
}

func (h *FooHandler) GetAll(c *fiber.Ctx) error {
	foos, err := h.service.GetAll(c.Context())
	if err != nil {
		return apiresp.Error(c, []string{err.Error()}, apiresp.WithStatus(fiber.StatusInternalServerError))
	}
	return apiresp.Success(c, foos)
}

func (h *FooHandler) GetConcrete(c *fiber.Ctx) error {
	result, err := h.concrete.Find(c.Context())
	if err != nil {
		return apiresp.Error(c, []string{err.Error()}, apiresp.WithStatus(fiber.StatusInternalServerError))
	}
	return apiresp.Success(c, result)
}

func (h *FooHandler) GetForward(c *fiber.Ctx) error {
	result, err := h.concrete.Forward(c.Context())
	if err != nil {
		return apiresp.Error(c, []string{err.Error()}, apiresp.WithStatus(fiber.StatusInternalServerError))
	}
	return apiresp.Success(c, result)
}

func (h *FooHandler) CustomError(c *fiber.Ctx) error {
	return c.Status(fiber.StatusTeapot).JSON(CustomErrorResponse{Message: "teapot"})
}

func (h *FooHandler) SendStatus(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusConflict)
}

func (h *FooHandler) GetByID(c *fiber.Ctx) error {
	id, _ := c.ParamsInt("id")
	result, err := h.service.GetByID(c.Context(), uint(id))
	if err != nil {
		return exception.NotFound(err.Error())
	}
	return apiresp.Success(c, result)
}

func (h *FooHandler) Create(c *fiber.Ctx) error {
	req := c.Locals("payload").(*CreateFooRequest)
	createdFoo, err := h.service.Create(c.Context(), req)
	if err != nil {
		return exception.NotFound(err.Error())
	}
	return apiresp.Success(c, createdFoo)
}

func (h *FooHandler) Update(c *fiber.Ctx) error {
	req := c.Locals("payload").(*UpdateFooRequest)
	updatedFoo, err := h.service.Update(c.Context(), req)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return apiresp.Success(c, updatedFoo)
}

func (h *FooHandler) Delete(c *fiber.Ctx) error {
	req := c.Locals("payload").(*IDParamRequest)
	deletedFoo, err := h.service.Delete(c.Context(), req.ID)
	if err != nil {
		return exception.InternalServerError(err.Error())
	}
	return apiresp.Success(c, deletedFoo)
}

type FooRepository struct{}

func (r *FooRepository) Load(c any) (*Foo, error) { return nil, nil }

type FooService struct{
	repo *FooRepository
}

func (s *FooService) GetAll(c any) ([]*Foo, error) { return nil, nil }
func (s *FooService) Find(c any) (*Foo, error) { return nil, nil }
func (s *FooService) Forward(c any) (any, error) { return s.repo.Load(c) }
func (s *FooService) Create(c any, req *CreateFooRequest) (*Foo, error) { return nil, nil }
`

	modelsSource := `package main

type Foo struct {
	ID uint ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}

type CreateFooRequest struct {
	Name string ` + "`json:\"name\" validate:\"required,min=3,max=50\"`" + `
}

type UpdateFooRequest struct {
	ID uint ` + "`uri:\"id\" validate:\"required\"`" + `
	Name string ` + "`json:\"name\" validate:\"required,min=3,max=50\"`" + `
}

type IDParamRequest struct {
	ID uint ` + "`uri:\"id\" validate:\"required\"`" + `
}

type CustomErrorResponse struct {
	Message string ` + "`json:\"message\"`" + `
	Code string ` + "`json:\"code\"`" + `
}

type ApiResponse[T any] struct {
	Status StatusType ` + "`json:\"status\"`" + `
	Message string ` + "`json:\"message\"`" + `
	Data T ` + "`json:\"data,omitempty\"`" + `
	Errors []string ` + "`json:\"errors,omitempty\"`" + `
}

type StatusType string

const (
	StatusSuccess StatusType = "Success"
	StatusError StatusType = "Error"
)

type Option func(*Config)
type Config struct { StatusCode int }

func WithStatus(code int) Option {
	return func(c *Config) { c.StatusCode = code }
}

func Success[T any](ctx *fiber.Ctx, data T, options ...Option) error {
	return ctx.Status(fiber.StatusOK).JSON(ApiResponse[T]{
		Status: StatusSuccess,
		Message: "OK",
		Data: data,
	})
}

func Error(ctx *fiber.Ctx, errors []string, options ...Option) error {
	return ctx.Status(fiber.StatusBadRequest).JSON(ApiResponse[any]{
		Status: StatusError,
		Message: "Error",
		Errors: errors,
	})
}

func RequireAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Get("Authorization") == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(ApiResponse[any]{
				Status: StatusError,
				Message: "Unauthorized",
				Errors: []string{"Unauthorized"},
			})
		}
		return c.Next()
	}
}

func RequireRole() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Get("Role") == "" {
			return fiber.NewError(fiber.StatusForbidden, "forbidden")
		}
		return c.Next()
	}
}

func RateLimit() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Get("X-Rate-Limited") == "true" {
			return c.SendStatus(fiber.StatusTooManyRequests)
		}
		return c.Next()
	}
}

func Validation422() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Get("X-Invalid") == "true" {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(ApiResponse[any]{
				Status: StatusError,
				Message: "Invalid",
				Errors: []string{"Invalid"},
			})
		}
		return c.Next()
	}
}
`

	p := New()
	files := []scanner.SourceFile{
		{Path: "routes.go", Language: "go", Content: []byte(routesSource)},
		{Path: "handler.go", Language: "go", Content: []byte(handlerSource)},
		{Path: "models.go", Language: "go", Content: []byte(modelsSource)},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 9)

	byMethodPath := make(map[string]openapi.Route)
	for _, route := range routes {
		byMethodPath[route.Method+" "+route.Path] = route
	}

	getAll := byMethodPath["GET /api/foo"]
	assert.Equal(t, "array", getAll.Responses["200"].Content["application/json"].Schema.Properties["data"].Type)
	assert.Equal(t, "#/components/schemas/Foo", getAll.Responses["200"].Content["application/json"].Schema.Properties["data"].Items.Ref)
	assert.Equal(t, "#/components/schemas/StatusType", getAll.Responses["200"].Content["application/json"].Schema.Properties["status"].Ref)
	getAllSuccessExample, ok := getAll.Responses["200"].Content["application/json"].Example.(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, getAllSuccessExample, "data")
	assert.Contains(t, getAllSuccessExample, "status")
	assert.Contains(t, getAllSuccessExample, "message")

	getAll500 := getAll.Responses["500"].Content["application/json"].Schema
	require.NotNil(t, getAll500)
	assert.Equal(t, "#/components/schemas/StatusType", getAll500.Properties["status"].Ref)
	assert.Equal(t, "array", getAll500.Properties["errors"].Type)

	getAll401 := getAll.Responses["401"].Content["application/json"].Schema
	require.NotNil(t, getAll401)
	assert.Equal(t, "#/components/schemas/ApiResponse", getAll401.Ref)

	_, has403Body := getAll.Responses["403"].Content["application/json"]
	assert.True(t, has403Body)
	_, has429Body := getAll.Responses["429"].Content["application/json"]
	assert.False(t, has429Body)

	concrete := byMethodPath["GET /api/foo/concrete"]
	assert.Equal(t, "#/components/schemas/Foo", concrete.Responses["200"].Content["application/json"].Schema.Properties["data"].Ref)
	_, hasConcrete403Body := concrete.Responses["403"].Content["application/json"]
	assert.True(t, hasConcrete403Body)

	forward := byMethodPath["GET /api/foo/forward"]
	assert.Equal(t, "#/components/schemas/Foo", forward.Responses["200"].Content["application/json"].Schema.Properties["data"].Ref)

	customError := byMethodPath["GET /api/foo/custom-error"]
	assert.Equal(t, "#/components/schemas/CustomErrorResponse", customError.Responses["418"].Content["application/json"].Schema.Ref)

	sendStatus := byMethodPath["GET /api/foo/send-status"]
	_, has409Body := sendStatus.Responses["409"].Content["application/json"]
	assert.False(t, has409Body)

	getByID := byMethodPath["GET /api/foo/{id}"]
	assert.Equal(t, "Get foo by ID", getByID.Summary)
	require.Len(t, getByID.Parameters, 1)
	assert.Equal(t, "Foo ID", getByID.Parameters[0].Description)
	assert.Equal(t, uint(1), getByID.Parameters[0].Example)

	create := byMethodPath["POST /api/foo"]
	assert.Equal(t, "#/components/schemas/CreateFooRequest", create.RequestBody.Content["application/json"].Schema.Ref)
	assert.Equal(t, "Create foo request body", create.RequestBody.Description)
	assert.Equal(t, "#/components/schemas/Foo", create.Responses["200"].Content["application/json"].Schema.Properties["data"].Ref)
	create422Schema := create.Responses["422"].Content["application/json"].Schema
	require.NotNil(t, create422Schema)
	assert.Equal(t, "#/components/schemas/ApiResponse", create422Schema.Ref)
	_, has404Body := create.Responses["404"].Content["application/json"]
	assert.False(t, has404Body)

	update := byMethodPath["PUT /api/foo/{id}"]
	require.Len(t, update.Parameters, 1)
	assert.Equal(t, "integer", update.Parameters[0].Schema.Type)
	assert.Equal(t, "Foo ID", update.Parameters[0].Description)
	assert.Equal(t, "#/components/schemas/UpdateFooRequest", update.RequestBody.Content["application/json"].Schema.Ref)
	assert.Equal(t, "Update foo request body", update.RequestBody.Description)
	_, hasUpdate404Body := update.Responses["404"].Content["application/json"]
	assert.False(t, hasUpdate404Body)

	schemas, err := p.ExtractSchemas(files)
	require.NoError(t, err)

	schemaByName := make(map[string]openapi.Schema)
	for _, schema := range schemas {
		schemaByName[schema.Title] = schema
	}

	assert.Contains(t, schemaByName, "Foo")
	assert.Contains(t, schemaByName, "CreateFooRequest")
	assert.Contains(t, schemaByName, "UpdateFooRequest")
	assert.Contains(t, schemaByName, "CustomErrorResponse")
	assert.Contains(t, schemaByName, "ApiResponse")
	assert.Contains(t, schemaByName, "StatusType")
	assert.NotContains(t, schemaByName, "ErrorResponse")
	assert.NotContains(t, schemaByName["UpdateFooRequest"].Properties, "ID")
}

func TestPlugin_ExtractRoutes_ConfiguredInferenceOverrides(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Get("/override", guard.RequireRole(), OverrideHandler)
}

func OverrideHandler(c *fiber.Ctx) error {
	result := Foo{}
	return custom.OK(c, result)
}

type Foo struct {
	ID uint ` + "`json:\"id\"`" + `
}
`

	cfg := config.Default()
	cfg.Inference.ResponseHelpers = []config.ResponseHelperInferenceConfig{
		{Pattern: "*.OK", Kind: "success"},
	}
	cfg.Inference.Middleware = []config.MiddlewareInferenceConfig{
		{Pattern: "*.RequireRole", Responses: []string{"403"}},
	}

	p := New()
	p.Configure(cfg)
	files := []scanner.SourceFile{
		{Path: "main.go", Language: "go", Content: []byte(source)},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 1)

	route := routes[0]
	assert.Equal(t, "#/components/schemas/Foo", route.Responses["200"].Content["application/json"].Schema.Ref)
	_, has403Body := route.Responses["403"].Content["application/json"]
	assert.True(t, has403Body)
}

func TestPlugin_ExtractRoutes_GlobalAppMiddleware(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func NewApp() *fiber.App {
	app := fiber.New()
	app.Use(limiter.RateLimit())
	return app
}

func SetupRoutes(app *fiber.App) {
	app.Get("/items", ListItems)
}

func ListItems(c *fiber.Ctx) error {
	return c.JSON([]string{"ok"})
}

func RateLimit() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Get("X-Rate-Limited") == "true" {
			return c.SendStatus(fiber.StatusTooManyRequests)
		}
		return c.Next()
	}
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}}
	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 1)

	route := routes[0]
	assert.NotContains(t, route.Responses, "429")
}

func TestPlugin_ExtractRoutes_QueryParamInference(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Get("/items", ListItems)
}

func ListItems(c *fiber.Ctx) error {
	page := c.Query("page")
	limit, _ := c.QueryInt("limit")
	_ = page
	_ = limit
	return c.JSON([]string{})
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}}
	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 1)

	params := routes[0].Parameters
	require.Len(t, params, 2)
	byName := make(map[string]openapi.Parameter)
	for _, param := range params {
		byName[param.Name] = param
	}
	assert.Equal(t, "query", byName["page"].In)
	assert.Equal(t, "string", byName["page"].Schema.Type)
	assert.Equal(t, "query", byName["limit"].In)
	assert.Equal(t, "integer", byName["limit"].Schema.Type)
}

func TestPlugin_ExtractRoutes_QueryBindHelperInference(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Get("/users", ListUsers)
}

type PaginationQuery struct {
	Page    int ` + "`query:\"page\"`" + `
	PerPage int ` + "`query:\"per_page\"`" + `
}

func paginationQuery(c *fiber.Ctx) (int, int, error) {
	var q PaginationQuery
	if err := c.Bind().Query(&q); err != nil {
		return 0, 0, err
	}
	return q.Page, q.PerPage, nil
}

func ListUsers(c *fiber.Ctx) error {
	page, perPage, err := paginationQuery(c)
	if err != nil {
		return err
	}
	return c.JSON([]string{page, perPage})
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}}
	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 1)

	params := routes[0].Parameters
	require.Len(t, params, 2)
	byName := make(map[string]openapi.Parameter)
	for _, param := range params {
		byName[param.Name] = param
	}
	assert.Equal(t, "query", byName["page"].In)
	assert.Equal(t, "integer", byName["page"].Schema.Type)
	assert.Equal(t, 1, byName["page"].Schema.Default)
	assert.Equal(t, "Page number", byName["page"].Description)
	assert.Equal(t, "query", byName["per_page"].In)
	assert.Equal(t, "integer", byName["per_page"].Schema.Type)
	assert.Equal(t, 10, byName["per_page"].Schema.Default)
	assert.Equal(t, "Items per page", byName["per_page"].Description)
}

func TestPlugin_ExtractRoutes_StatusCodeIntelligence(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Post("/", RequestValidator[CreateFooRequest](), Create)
}

func RequestValidator[T any]() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return defaultErrorHandler(c, fiber.NewError(fiber.StatusBadRequest, "invalid"))
	}
}

func defaultErrorHandler(c *fiber.Ctx, err error) error {
	return Error(c, []string{err.Error()}, WithStatus(fiber.StatusBadRequest))
}

func Create(c *fiber.Ctx) error {
	foo := Foo{}
	return Success(c, foo)
}

type Foo struct {
	ID uint ` + "`json:\"id\"`" + `
}

type CreateFooRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

type ApiResponse[T any] struct {
	Status  string ` + "`json:\"status\"`" + `
	Message string ` + "`json:\"message\"`" + `
	Data    T      ` + "`json:\"data\"`" + `
}

type Option func(*Config)
type Config struct { StatusCode int }

func WithStatus(code int) Option {
	return func(c *Config) { c.StatusCode = code }
}

func Success[T any](ctx *fiber.Ctx, data T) error {
	return ctx.Status(fiber.StatusOK).JSON(ApiResponse[T]{
		Status:  "Success",
		Message: "OK",
		Data:    data,
	})
}

func Error(ctx *fiber.Ctx, errors []string, options ...Option) error {
	statusCode := fiber.StatusBadRequest
	for _, opt := range options {
		cfg := &Config{}
		opt(cfg)
		if cfg.StatusCode != 0 {
			statusCode = cfg.StatusCode
		}
	}
	return ctx.Status(statusCode).JSON(ApiResponse[any]{
		Status:  "Error",
		Message: "Error",
		Errors:  errors,
	})
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}}
	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 1)

	route := routes[0]
	assert.Contains(t, route.Responses, "200")
	assert.NotContains(t, route.Responses, "201")
	assert.Contains(t, route.Responses, "400")
	assert.NotContains(t, route.Responses, "422")
}

func TestPlugin_ExtractRoutes_ValidationStatusFromMiddlewareCode(t *testing.T) {
	t.Run("StatusBadRequest", func(t *testing.T) {
		source := validationMiddlewareSource("fiber.StatusBadRequest")
		p := New()
		files := []scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}}
		routes, err := p.ExtractRoutes(files)
		require.NoError(t, err)
		require.Len(t, routes, 1)
		assert.Contains(t, routes[0].Responses, "400")
		assert.NotContains(t, routes[0].Responses, "422")
	})

	t.Run("StatusUnprocessableEntity", func(t *testing.T) {
		source := validationMiddlewareSource("fiber.StatusUnprocessableEntity")
		p := New()
		files := []scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}}
		routes, err := p.ExtractRoutes(files)
		require.NoError(t, err)
		require.Len(t, routes, 1)
		assert.Contains(t, routes[0].Responses, "422")
		assert.NotContains(t, routes[0].Responses, "400")
	})
}

func validationMiddlewareSource(statusExpr string) string {
	return `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Post("/", RequestValidator[CreateFooRequest](), Create)
}

func RequestValidator[T any]() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return defaultErrorHandler(c, fiber.NewError(` + statusExpr + `, "invalid"))
	}
}

func defaultErrorHandler(c *fiber.Ctx, err error) error {
	return Error(c, []string{err.Error()}, WithStatus(` + statusExpr + `))
}

func Create(c *fiber.Ctx) error {
	return c.JSON(CreateFooRequest{})
}

type CreateFooRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

type ApiResponse[T any] struct {
	Status  string ` + "`json:\"status\"`" + `
	Message string ` + "`json:\"message\"`" + `
	Errors  []string ` + "`json:\"errors,omitempty\"`" + `
}

type Option func(*Config)
type Config struct { StatusCode int }

func WithStatus(code int) Option {
	return func(c *Config) { c.StatusCode = code }
}

func Error(ctx *fiber.Ctx, errors []string, options ...Option) error {
	statusCode := fiber.StatusBadRequest
	for _, opt := range options {
		cfg := &Config{}
		opt(cfg)
		if cfg.StatusCode != 0 {
			statusCode = cfg.StatusCode
		}
	}
	return ctx.Status(statusCode).JSON(ApiResponse[any]{
		Status:  "Error",
		Message: "Error",
		Errors:  errors,
	})
}
`
}

func TestPlugin_ExtractRoutes_AuthMiddlewareSecurity(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Get("/secure", auth.RequireAuth(), SecureHandler)
}

func SecureHandler(c *fiber.Ctx) error {
	return c.JSON(map[string]string{"ok": "true"})
}

func RequireAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Get("Authorization") == "" {
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		}
		return c.Next()
	}
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}}
	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 1)

	require.Len(t, routes[0].Security, 1)
	assert.Contains(t, routes[0].Security[0], "BearerAuth")
}

func TestDiscoverRouteBundles_Layer1Metadata(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

type FooRouter struct {
	handler *FooHandler
}

func SetupRoutes(app *fiber.App, fooRouter *FooRouter) {
	api := app.Group("/api")
	foo := api.Group("/foo")
	foo.Use(auth.RequireAuth())
	foo.Post("/", middleware.RequestValidator[CreateFooRequest](), fooRouter.handler.Create)
}

func (h *FooHandler) Create(c *fiber.Ctx) error {
	return c.JSON(Foo{})
}

type FooHandler struct{}
type Foo struct{}
type CreateFooRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

func RequireAuth() fiber.Handler {
	return func(c *fiber.Ctx) error { return c.Next() }
}

func RequestValidator[T any]() fiber.Handler {
	return func(c *fiber.Ctx) error { return c.Next() }
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "routes.go", Language: "go", Content: []byte(source)}}
	bundles := p.discoverRouteBundles(files)
	require.Len(t, bundles, 1)

	route := bundles[0].Route
	assert.Equal(t, "POST", route.Method)
	assert.Equal(t, "/api/foo", route.Path)
	assert.Equal(t, "foo", route.AppVar)
	assert.Equal(t, "high", route.DiscoveryConfidence)
	assert.Equal(t, "fooRouter.handler.Create", route.HandlerRef.Qualified)
	assert.Equal(t, "Create", route.HandlerRef.Method)
	assert.True(t, route.HandlerRef.Resolved)
	assert.Nil(t, route.Responses)
	assert.Len(t, route.Middleware, 2)
	assert.Contains(t, route.Middleware[0].Name, "RequireAuth")
	assert.Equal(t, "middleware.RequestValidator[CreateFooRequest]", route.Middleware[1].Name)
}

func TestFilterRouteBundles_IncludeExclude(t *testing.T) {
	bundles := []discoveredRouteBundle{
		{Route: openapi.Route{Path: "/api/foo", Method: "GET"}},
		{Route: openapi.Route{Path: "/api/health", Method: "GET"}},
		{Route: openapi.Route{Path: "/internal/metrics", Method: "GET"}},
	}

	filtered := filterRouteBundles(bundles, config.RoutesConfig{
		IncludePrefixes: []string{"/api"},
		ExcludePaths:    []string{"/api/health"},
	})

	require.Len(t, filtered, 1)
	assert.Equal(t, "/api/foo", filtered[0].Route.Path)
}

func TestExtractRoutes_AppliesRouteFilterFromConfig(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	api := app.Group("/api")
	api.Get("/foo", GetFoo)
	api.Get("/health", Health)
}

func GetFoo(c *fiber.Ctx) error { return c.SendString("ok") }
func Health(c *fiber.Ctx) error { return c.SendString("ok") }
`

	p := New()
	p.Configure(&config.Config{
		Routes: config.RoutesConfig{
			IncludePrefixes: []string{"/api"},
			ExcludePaths:    []string{"/api/health"},
		},
	})

	files := []scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}}
	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	require.Len(t, routes, 1)
	assert.Equal(t, "/api/foo", routes[0].Path)
}

func TestExtractRoutes_FollowsDirectRouteRegistrationCalls(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App, deps Deps) {
	RegisterV1Routes(app.Group("/api/v1"), deps)
}

func RegisterV1Routes(v1 fiber.Router, deps Deps) {
	auth := v1.Group("/auth")
	auth.Post("/register", deps.AuthHandler.Register)
}

type Deps struct {
	AuthHandler *AuthHandler
}

type AuthHandler struct{}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	return Created(c, UserResponse{})
}

type UserResponse struct {
	ID int ` + "`json:\"id\"`" + `
}

func Created(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusCreated).JSON(Response{Success: true, Data: data})
}

type Response struct {
	Success bool ` + "`json:\"success\"`" + `
	Data    any  ` + "`json:\"data,omitempty\"`" + `
}
`

	p := New()
	routes, err := p.ExtractRoutes([]scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}})
	require.NoError(t, err)
	require.Len(t, routes, 1)
	assert.Equal(t, "POST", routes[0].Method)
	assert.Equal(t, "/api/v1/auth/register", routes[0].Path)
	assert.Contains(t, routes[0].Responses, "201")
	resp201 := routes[0].Responses["201"]
	assert.NotNil(t, resp201.Content)
}

func TestExtractRoutes_BindAndValidateRequestBody(t *testing.T) {
	source := `package handler

import "github.com/gofiber/fiber/v3"

type RegisterRequest struct {
	Email string ` + "`json:\"email\"`" + `
}

func Register(c fiber.Ctx) error {
	var req RegisterRequest
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}
	return nil
}

func bindAndValidate(c fiber.Ctx, req any) error {
	return c.Bind().Body(req)
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "handler.go", Language: "go", Content: []byte(source)}}
	p.discoverRouteBundles(files)

	analysis := p.collectHandlerAnalysis(p.routeFuncIndex["Register"])
	require.NotEmpty(t, analysis.RequestHints)
	assert.Equal(t, "RegisterRequest", analysis.RequestHints[0].SchemaName)
}

func TestCollectHandlerAnalysis_ReturnCallsOnly(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func Create(c *fiber.Ctx) error {
	if err := validate(); err != nil {
		return response.Error(c, []string{err.Error()})
	}
	foo := Foo{}
	_ = c.JSON(foo)
	return response.Success(c, foo)
}

func validate() error { return nil }

type Foo struct {
	ID uint ` + "`json:\"id\"`" + `
}

func Error(c *fiber.Ctx, errors []string) error {
	return c.Status(fiber.StatusBadRequest).JSON(map[string]string{"error": "bad"})
}

func Success(c *fiber.Ctx, data Foo) error {
	return c.Status(fiber.StatusOK).JSON(data)
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "handler.go", Language: "go", Content: []byte(source)}}
	p.discoverRouteBundles(files)

	fn := p.handlerIndex["Create"]
	require.NotNil(t, fn)

	analysis := p.collectHandlerAnalysis(fn)
	require.Len(t, analysis.ReturnCalls, 2)
	assert.Equal(t, "Error", returnCallName(analysis.ReturnCalls[0]))
	assert.Equal(t, "Success", returnCallName(analysis.ReturnCalls[1]))
}

func TestCollectHandlerAnalysis_LocalsPayload(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func Create(c *fiber.Ctx) error {
	req := c.Locals("payload").(*CreateFooRequest)
	return response.Success(c, req)
}

type CreateFooRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

func Success(c *fiber.Ctx, data any) error {
	return c.JSON(data)
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "handler.go", Language: "go", Content: []byte(source)}}
	p.discoverRouteBundles(files)

	fn := p.handlerIndex["Create"]
	require.NotNil(t, fn)

	analysis := p.collectHandlerAnalysis(fn)
	require.Len(t, analysis.RequestHints, 1)
	assert.Equal(t, requestHintLocalsPayload, analysis.RequestHints[0].Kind)
	assert.Equal(t, "CreateFooRequest", analysis.RequestHints[0].SchemaName)
}

func TestAnalyzeRouteBundle_MiddlewareLayer2(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Post("/", middleware.RequestValidator[CreateFooRequest](), Create)
}

func Create(c *fiber.Ctx) error {
	return c.JSON(CreateFooRequest{})
}

type CreateFooRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

func RequestValidator[T any]() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return response.Error(c, []string{"bad"}, WithStatus(fiber.StatusBadRequest))
	}
}

func Error(c *fiber.Ctx, errors []string, options ...Option) error {
	return c.Status(fiber.StatusBadRequest).JSON(errors)
}

type Option func(*Config)
type Config struct{ StatusCode int }

func WithStatus(code int) Option {
	return func(c *Config) { c.StatusCode = code }
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}}
	bundles := p.discoverRouteBundles(files)
	require.Len(t, bundles, 1)

	analysis := p.analyzeRouteBundle(&bundles[0])
	require.Len(t, analysis.Middleware, 1)
	assert.Equal(t, "middleware.RequestValidator[CreateFooRequest]", analysis.Middleware[0].Name)
	assert.Equal(t, "CreateFooRequest", analysis.Middleware[0].ValidatorSchema)
	assert.Contains(t, analysis.Middleware[0].ErrorStatuses, "400")
	assert.Nil(t, bundles[0].Route.Responses)
}

func returnCallName(callExpr *ast.CallExpr) string {
	switch fn := callExpr.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	default:
		return ""
	}
}

func TestResolveRequestTypes_FromMiddlewareValidator(t *testing.T) {
	p := New()
	route := &openapi.Route{
		Method:     "POST",
		Parameters: []openapi.Parameter{{Name: "id", In: "path"}},
	}
	analysis := RouteAnalysis{
		Middleware: []MiddlewareAnalysis{{
			Name:            "middleware.RequestValidator[CreateFooRequest]",
			ValidatorSchema: "CreateFooRequest",
			ErrorStatuses:   []string{"400"},
		}},
	}

	body, params := p.resolveRequestTypes(route, analysis)
	require.NotNil(t, body)
	assert.Equal(t, "CreateFooRequest", body.SchemaName)
	assert.Equal(t, "middleware.request_validator", body.Source)

	var idParam *ParameterResolution
	for i := range params {
		if params[i].Name == "id" {
			idParam = &params[i]
			break
		}
	}
	require.Nil(t, idParam)
}

func TestResolveRequestTypes_IDParamFromUpdateValidator(t *testing.T) {
	p := New()
	route := &openapi.Route{
		Method:     "PUT",
		Parameters: []openapi.Parameter{{Name: "id", In: "path"}},
	}
	p.structIndex = map[string]parser.StructDefinition{
		"UpdateFooRequest": {
			Name: "UpdateFooRequest",
			Fields: []parser.StructField{
				{
					Name:           "ID",
					JSONName:       "ID",
					URIName:        "id",
					Type:           "uint",
					TypeKind:       parser.KindPrimitive,
					IsRequired:     true,
					ValidationTags: map[string]string{},
				},
				{
					Name:           "Name",
					JSONName:       "name",
					Type:           "string",
					TypeKind:       parser.KindPrimitive,
					IsRequired:     true,
					ValidationTags: map[string]string{"min": "3", "max": "50"},
				},
			},
		},
	}

	analysis := RouteAnalysis{
		Middleware: []MiddlewareAnalysis{{
			ValidatorSchema: "UpdateFooRequest",
		}},
	}

	body, params := p.resolveRequestTypes(route, analysis)
	require.NotNil(t, body)
	assert.Equal(t, "UpdateFooRequest", body.SchemaName)
	require.NotNil(t, body.PathFields)
	assert.True(t, body.PathFields["id"])

	require.Len(t, params, 1)
	assert.Equal(t, "id", params[0].Name)
	assert.Equal(t, "path", params[0].In)
	require.NotNil(t, params[0].Schema)
	assert.Equal(t, "integer", params[0].Schema.Type)
}

func TestResolveRequestSchemaGraph_IDParamValidate(t *testing.T) {
	p := New()
	p.structIndex = map[string]parser.StructDefinition{
		"IDParamRequest": {
			Name: "IDParamRequest",
			Fields: []parser.StructField{
				{
					Name:       "ID",
					JSONName:   "ID",
					URIName:    "id",
					Type:       "uint",
					TypeKind:   parser.KindPrimitive,
					IsRequired: true,
					ValidationTags: map[string]string{
						"min": "1",
					},
				},
			},
		},
	}

	graph := p.resolveRequestSchemaGraph("IDParamRequest", &openapi.Route{
		Method:     "GET",
		Parameters: []openapi.Parameter{{Name: "id", In: "path"}},
	})

	assert.False(t, graph.HasBody)
	require.Len(t, graph.PathParams, 1)
	assert.Equal(t, "id", graph.PathParams[0].Name)
	require.NotNil(t, graph.PathParams[0].Schema)
	assert.Equal(t, "integer", graph.PathParams[0].Schema.Type)
	require.NotNil(t, graph.PathParams[0].Schema.Minimum)
	assert.Equal(t, float64(1), *graph.PathParams[0].Schema.Minimum)
	assert.True(t, graph.PathFields["id"])
}

func TestResolveRequestSchemaGraph_BodyValidateConstraints(t *testing.T) {
	p := New()
	p.structIndex = map[string]parser.StructDefinition{
		"CreateFooRequest": {
			Name: "CreateFooRequest",
			Fields: []parser.StructField{
				{
					Name:           "Name",
					JSONName:       "name",
					Type:           "string",
					TypeKind:       parser.KindPrimitive,
					IsRequired:     true,
					ValidationTags: map[string]string{"min": "3", "max": "50"},
				},
			},
		},
	}

	graph := p.resolveRequestSchemaGraph("CreateFooRequest", &openapi.Route{Method: "POST"})
	assert.True(t, graph.HasBody)
	assert.Empty(t, graph.PathParams)

	schema := p.schemaExtractor.FieldSchema(graphSchemaField(p, "CreateFooRequest", "name"))
	require.NotNil(t, schema)
	require.NotNil(t, schema.MinLength)
	require.NotNil(t, schema.MaxLength)
	assert.Equal(t, 3, *schema.MinLength)
	assert.Equal(t, 50, *schema.MaxLength)
}

func graphSchemaField(p *Plugin, schemaName, jsonName string) parser.StructField {
	def := p.structIndex[schemaName]
	for _, field := range def.Fields {
		if field.JSONName == jsonName {
			return field
		}
	}
	return parser.StructField{}
}

func TestResolveResponseTypes_ServiceForwarding(t *testing.T) {
	source := `package main

import (
	"context"

	"github.com/gofiber/fiber/v2"
)

type Foo struct {
	ID uint ` + "`json:\"id\"`" + `
}

type IFooService interface {
	GetAll(ctx context.Context) ([]*Foo, error)
}

type FooHandler struct {
	service IFooService
}

func (h *FooHandler) GetAll(c *fiber.Ctx) error {
	foos, err := h.service.GetAll(c.Context())
	if err != nil {
		return exception.InternalServerError(err.Error())
	}
	return Success(c, foos)
}

func (s *FooService) GetAll(ctx context.Context) ([]*Foo, error) {
	return nil, nil
}

type StatusType string

const StatusSuccess StatusType = "Success"

type ApiResponse[T any] struct {
	Status  StatusType ` + "`json:\"status\"`" + `
	Message string     ` + "`json:\"message\"`" + `
	Data    T          ` + "`json:\"data,omitempty\"`" + `
}

func Success(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusOK).JSON(ApiResponse[any]{
		Status:  StatusSuccess,
		Message: "OK",
		Data:    data,
	})
}

func InternalServerError(msg string) error {
	return fiber.ErrInternalServerError
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "handler.go", Language: "go", Content: []byte(source)}}
	p.discoverRouteBundles(files)

	fn := p.handlerIndex["GetAll"]
	require.NotNil(t, fn)

	analysis := p.collectHandlerAnalysis(fn)
	graph := RouteTypeGraph{Responses: make(map[string]ResponseResolution)}
	p.resolveHandlerResponseTypes(&openapi.Route{Method: "GET"}, &analysis, &graph)

	success, ok := graph.Responses["200"]
	require.True(t, ok, "expected 200 response")
	assert.Equal(t, "handler.response_helper.Success", success.Source)
	require.NotNil(t, success.Schema)
	require.NotNil(t, success.Schema.Properties["data"])
	assert.Equal(t, "array", success.Schema.Properties["data"].Type)
	require.NotNil(t, success.Schema.Properties["data"].Items)
	assert.Equal(t, "#/components/schemas/Foo", success.Schema.Properties["data"].Items.Ref)
}

func TestResolveRouteTypeGraph_Integration(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	app.Post("/api/foo/:id", middleware.RequestValidator[UpdateFooRequest](), Update)
}

func Update(c *fiber.Ctx) error {
	req := c.Locals("payload").(*UpdateFooRequest)
	if req.Name == "" {
		return Error(c, []string{"bad"})
	}
	return Success(c, req)
}

type UpdateFooRequest struct {
	Name string ` + "`json:\"name\"`" + `
}

type StatusType string

const (
	StatusSuccess StatusType = "Success"
	StatusError   StatusType = "Error"
)

type ApiResponse[T any] struct {
	Status  StatusType ` + "`json:\"status\"`" + `
	Message string     ` + "`json:\"message\"`" + `
	Data    T          ` + "`json:\"data,omitempty\"`" + `
	Errors  []string   ` + "`json:\"errors,omitempty\"`" + `
}

func RequestValidator[T any]() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return Error(c, []string{"invalid"}, WithStatus(fiber.StatusBadRequest))
	}
}

func Error(c *fiber.Ctx, errors []string, options ...Option) error {
	return c.Status(fiber.StatusBadRequest).JSON(ApiResponse[any]{
		Status:  StatusError,
		Message: "Bad Request",
		Errors:  errors,
	})
}

func Success(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusOK).JSON(ApiResponse[any]{
		Status:  StatusSuccess,
		Message: "OK",
		Data:    data,
	})
}

type Option func(*Config)
type Config struct{ StatusCode int }

func WithStatus(code int) Option {
	return func(c *Config) { c.StatusCode = code }
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "main.go", Language: "go", Content: []byte(source)}}
	bundles := p.discoverRouteBundles(files)
	require.Len(t, bundles, 1)

	route := bundles[0].Route
	analysis := p.analyzeRouteBundle(&bundles[0])
	graph := p.resolveRouteTypeGraph(&route, analysis)

	require.NotNil(t, graph.RequestBody)
	assert.Equal(t, "UpdateFooRequest", graph.RequestBody.SchemaName)

	assert.Contains(t, graph.Responses, "400")
	assert.Contains(t, graph.Responses, "200")

	p.applyRouteTypeGraph(&route, graph)
	assert.NotNil(t, route.RequestBody)
	assert.Contains(t, route.Responses, "400")
	assert.Contains(t, route.Responses, "200")
}

func TestMultiHopReturnType_RepoChain(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

type Item struct {
	ID uint ` + "`json:\"id\"`" + `
}

type Store struct{}

func (s *Store) Load() []*Item {
	return nil
}

type Service struct {
	store *Store
}

func (s *Service) FindItems() []*Item {
	data := s.store.Load()
	return data
}

type Handler struct {
	svc *Service
}

func (h *Handler) List(c *fiber.Ctx) error {
	items, err := h.svc.FindItems()
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	return Success(c, items)
}

type StatusType string

const StatusSuccess StatusType = "Success"

type ApiResponse[T any] struct {
	Status  StatusType ` + "`json:\"status\"`" + `
	Message string     ` + "`json:\"message\"`" + `
	Data    T          ` + "`json:\"data,omitempty\"`" + `
}

func Success(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusOK).JSON(ApiResponse[any]{
		Status:  StatusSuccess,
		Message: "OK",
		Data:    data,
	})
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "handler.go", Language: "go", Content: []byte(source)}}
	p.discoverRouteBundles(files)

	fn := p.handlerIndex["List"]
	require.NotNil(t, fn)

	analysis := p.collectHandlerAnalysis(fn)
	graph := RouteTypeGraph{Responses: make(map[string]ResponseResolution)}
	p.resolveHandlerResponseTypes(&openapi.Route{Method: "GET"}, &analysis, &graph)

	success, ok := graph.Responses["200"]
	require.True(t, ok)
	require.NotNil(t, success.Schema.Properties["data"])
	assert.Equal(t, "array", success.Schema.Properties["data"].Type)
	assert.Equal(t, "#/components/schemas/Item", success.Schema.Properties["data"].Items.Ref)
}

func TestMultiHopReturnType_HandlerHelperCall(t *testing.T) {
	source := `package main

import "github.com/gofiber/fiber/v2"

type Widget struct {
	Name string ` + "`json:\"name\"`" + `
}

type Handler struct {
	svc *Service
}

func (h *Handler) Show(c *fiber.Ctx) error {
	return Success(c, h.load())
}

func (h *Handler) load() *Widget {
	w, _ := h.svc.GetWidget()
	return w
}

type Service struct{}

func (s *Service) GetWidget() (*Widget, error) {
	return &Widget{Name: "x"}, nil
}

type StatusType string

const StatusSuccess StatusType = "Success"

type ApiResponse[T any] struct {
	Status  StatusType ` + "`json:\"status\"`" + `
	Message string     ` + "`json:\"message\"`" + `
	Data    T          ` + "`json:\"data,omitempty\"`" + `
}

func Success(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusOK).JSON(ApiResponse[any]{
		Status:  StatusSuccess,
		Message: "OK",
		Data:    data,
	})
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "handler.go", Language: "go", Content: []byte(source)}}
	p.discoverRouteBundles(files)

	fn := p.handlerIndex["Show"]
	require.NotNil(t, fn)

	analysis := p.collectHandlerAnalysis(fn)
	graph := RouteTypeGraph{Responses: make(map[string]ResponseResolution)}
	p.resolveHandlerResponseTypes(&openapi.Route{Method: "GET"}, &analysis, &graph)

	success, ok := graph.Responses["200"]
	require.True(t, ok)
	require.NotNil(t, success.Schema.Properties["data"])
	assert.Equal(t, "#/components/schemas/Widget", success.Schema.Properties["data"].Ref)
}

func TestSchemaFromFiberMapLiteral(t *testing.T) {
	source := `package main

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

func Check(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"time":   time.Now(),
		"status": "ok",
	})
}
`

	p := New()
	files := []scanner.SourceFile{{Path: "health.go", Language: "go", Content: []byte(source)}}
	p.discoverRouteBundles(files)

	fn := p.handlerIndex["Check"]
	require.NotNil(t, fn)

	analysis := p.collectHandlerAnalysis(fn)
	graph := RouteTypeGraph{Responses: make(map[string]ResponseResolution)}
	p.resolveHandlerResponseTypes(&openapi.Route{Method: "GET"}, &analysis, &graph)

	success, ok := graph.Responses["200"]
	require.True(t, ok)
	require.NotNil(t, success.Schema)
	assert.Equal(t, "object", success.Schema.Type)
	require.NotNil(t, success.Schema.Properties["status"])
	assert.Equal(t, "string", success.Schema.Properties["status"].Type)
	assert.Equal(t, "ok", success.Schema.Properties["status"].Example)
	require.NotNil(t, success.Schema.Properties["time"])
	assert.Equal(t, "string", success.Schema.Properties["time"].Type)
	assert.Equal(t, "date-time", success.Schema.Properties["time"].Format)
}

func TestApplyDocAnnotationOverrides(t *testing.T) {
	p := New()
	route := &openapi.Route{
		Method:      "GET",
		Path:        "/api/foo",
		Summary:     "inferred summary",
		OperationID: "getGetAll",
		Tags:        []string{"foo"},
		Responses: map[string]openapi.Response{
			"200": {Description: "OK"},
		},
	}

	p.applyDocAnnotationOverrides(route, &parser.DocAnnotations{
		Summary:     "Custom summary",
		Description: "Custom description",
		Tags:        []string{"custom"},
		OperationID: "listFoos",
		Deprecated:  true,
		Responses: []parser.ResponseAnnotation{
			{Code: "200", Description: "All foos returned"},
			{Code: "404", Description: "Not found", Type: "ErrorResponse"},
		},
	})

	assert.Equal(t, "Custom summary", route.Summary)
	assert.Equal(t, "Custom description", route.Description)
	assert.Equal(t, []string{"custom"}, route.Tags)
	assert.Equal(t, "listFoos", route.OperationID)
	assert.True(t, route.Deprecated)
	assert.Equal(t, "All foos returned", route.Responses["200"].Description)
	assert.Equal(t, "Not found", route.Responses["404"].Description)
	require.NotNil(t, route.Responses["404"].Content["application/json"].Schema)
	assert.Equal(t, "#/components/schemas/ErrorResponse", route.Responses["404"].Content["application/json"].Schema.Ref)
}