// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiberplugin "github.com/whisskey/prism/internal/plugins/fiber"
	"github.com/whisskey/prism/internal/scanner"
)

func TestFiberPlugin_EndToEnd(t *testing.T) {
	routesSource := `package routes

import "github.com/gofiber/fiber/v2"

func SetupRoutes(app *fiber.App) {
	api := app.Group("/api")
	api.Get("/health", HealthCheck)

	v1 := api.Group("/v1")
	users := v1.Group("/users")
	users.Get("/", ListUsers)
	users.Post("/", CreateUser)
	users.Get("/:userId", GetUser)
	users.Put("/:userId", UpdateUser)
	users.Delete("/:userId", DeleteUser)
	users.Get("/:userId/posts", ListUserPosts)
	users.Post("/:userId/posts", CreateUserPost)

	posts := v1.Group("/posts")
	posts.Get("/", ListPosts)
	posts.Get("/:postId", GetPost)
	posts.Put("/:postId", UpdatePost)
	posts.Delete("/:postId", DeletePost)
}
`

	p := fiberplugin.New()
	files := []scanner.SourceFile{
		{Path: "models/user.go", Language: "go", Content: []byte(modelsSource)},
		{Path: "routes/routes.go", Language: "go", Content: []byte(routesSource)},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)
	schemas, err := p.ExtractSchemas(files)
	require.NoError(t, err)

	t.Run("routes extraction", func(t *testing.T) {
		assertStandardRoutes(t, routes)
	})
	t.Run("schemas extraction", func(t *testing.T) {
		assertStandardSchemas(t, schemas)
	})
}

func TestFiberPlugin_ComplexRoutePatterns(t *testing.T) {
	source := `package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func SetupRoutes(app *fiber.App) {
	app.Use(logger.New())
	app.Use(recover.New())

	app.Get("/", Home)
	app.Get("/health", Health)
	app.Get("/version", Version)

	articles := app.Group("/articles")
	articles.Get("/", ListArticles)
	articles.Post("/", CreateArticle)
	articles.Get("/:articleSlug", GetArticle)
	articles.Put("/:articleSlug", UpdateArticle)
	articles.Delete("/:articleSlug", DeleteArticle)
	articles.Get("/:articleSlug/comments", ListComments)
	articles.Post("/:articleSlug/comments", CreateComment)
	articles.Get("/:articleSlug/comments/:commentId", GetComment)

	admin := app.Group("/admin")
	admin.Get("/dashboard", AdminDashboard)
	admin.Get("/users", AdminUsers)
	admin.Get("/settings", AdminSettings)

	app.Get("/files/*", ServeFile)
	app.Get("/users/:id", GetUserByID)
}
`

	p := fiberplugin.New()
	routes, err := p.ExtractRoutes([]scanner.SourceFile{{
		Path: "api/routes.go", Language: "go", Content: []byte(source),
	}})
	require.NoError(t, err)

	routeMap := routesByKey(routes)
	assert.Contains(t, routeMap, "GET /")
	assert.Contains(t, routeMap, "GET /health")
	assert.Contains(t, routeMap, "GET /version")
	assert.Contains(t, routeMap, "GET /articles")
	assert.Contains(t, routeMap, "POST /articles")
	assert.Contains(t, routeMap, "GET /articles/{articleSlug}")
	assert.Contains(t, routeMap, "GET /articles/{articleSlug}/comments")
	assert.Contains(t, routeMap, "GET /articles/{articleSlug}/comments/{commentId}")
	assert.Contains(t, routeMap, "GET /admin/dashboard")
	assert.Contains(t, routeMap, "GET /admin/users")
	assert.Contains(t, routeMap, "GET /admin/settings")

	filesRoute := routeMap["GET /files/{path}"]
	require.Len(t, filesRoute.Parameters, 1)
	assert.Equal(t, "path", filesRoute.Parameters[0].Name)

	usersRoute := routeMap["GET /users/{id}"]
	require.Len(t, usersRoute.Parameters, 1)
	assert.Equal(t, "id", usersRoute.Parameters[0].Name)

	commentRoute := routeMap["GET /articles/{articleSlug}/comments/{commentId}"]
	require.Len(t, commentRoute.Parameters, 2)
	paramNames := []string{commentRoute.Parameters[0].Name, commentRoute.Parameters[1].Name}
	assert.Contains(t, paramNames, "articleSlug")
	assert.Contains(t, paramNames, "commentId")
}
