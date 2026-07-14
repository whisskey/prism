// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ginplugin "github.com/whisskey/prism/internal/plugins/gin"
	"github.com/whisskey/prism/internal/scanner"
)

func TestGinPlugin_EndToEnd(t *testing.T) {
	routesSource := `package routes

import "github.com/gin-gonic/gin"

func SetupRoutes(r *gin.Engine) {
	api := r.Group("/api")
	api.GET("/health", HealthCheck)

	v1 := api.Group("/v1")
	users := v1.Group("/users")
	users.GET("/", ListUsers)
	users.POST("/", CreateUser)
	users.GET("/:userId", GetUser)
	users.PUT("/:userId", UpdateUser)
	users.DELETE("/:userId", DeleteUser)
	users.GET("/:userId/posts", ListUserPosts)
	users.POST("/:userId/posts", CreateUserPost)

	posts := v1.Group("/posts")
	posts.GET("/", ListPosts)
	posts.GET("/:postId", GetPost)
	posts.PUT("/:postId", UpdatePost)
	posts.DELETE("/:postId", DeletePost)
}
`

	p := ginplugin.New()
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

func TestGinPlugin_ComplexRoutePatterns(t *testing.T) {
	source := `package api

import (
	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine) {
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	r.GET("/", Home)
	r.GET("/health", Health)
	r.GET("/version", Version)

	articles := r.Group("/articles")
	articles.GET("/", ListArticles)
	articles.POST("/", CreateArticle)
	articles.GET("/:articleSlug", GetArticle)
	articles.PUT("/:articleSlug", UpdateArticle)
	articles.DELETE("/:articleSlug", DeleteArticle)
	articles.GET("/:articleSlug/comments", ListComments)
	articles.POST("/:articleSlug/comments", CreateComment)
	articles.GET("/:articleSlug/comments/:commentId", GetComment)

	admin := r.Group("/admin")
	admin.GET("/dashboard", AdminDashboard)
	admin.GET("/users", AdminUsers)
	admin.GET("/settings", AdminSettings)

	r.GET("/files/*filepath", ServeFile)
	r.GET("/users/:id", GetUserByID)
}
`

	p := ginplugin.New()
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

	filesRoute := routeMap["GET /files/{filepath}"]
	require.Len(t, filesRoute.Parameters, 1)
	assert.Equal(t, "filepath", filesRoute.Parameters[0].Name)

	usersRoute := routeMap["GET /users/{id}"]
	require.Len(t, usersRoute.Parameters, 1)
	assert.Equal(t, "id", usersRoute.Parameters[0].Name)

	commentRoute := routeMap["GET /articles/{articleSlug}/comments/{commentId}"]
	require.Len(t, commentRoute.Parameters, 2)
	paramNames := []string{commentRoute.Parameters[0].Name, commentRoute.Parameters[1].Name}
	assert.Contains(t, paramNames, "articleSlug")
	assert.Contains(t, paramNames, "commentId")
}
