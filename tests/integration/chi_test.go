// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	chiplugin "github.com/whisskey/prism/internal/plugins/chi"
	"github.com/whisskey/prism/internal/scanner"
)

// TestChiPlugin_EndToEnd tests the full flow from Go source to OpenAPI routes and schemas.
// This simulates a real-world chi API project with routes and models.
func TestChiPlugin_EndToEnd(t *testing.T) {
	routesSource := `package routes

import (
	"github.com/go-chi/chi/v5"
)

func SetupRoutes(r chi.Router) {
	r.Route("/api", func(r chi.Router) {
		r.Get("/health", HealthCheck)

		r.Route("/v1", func(r chi.Router) {
			r.Route("/users", func(r chi.Router) {
				r.Get("/", ListUsers)
				r.Post("/", CreateUser)

				r.Route("/{userId}", func(r chi.Router) {
					r.Get("/", GetUser)
					r.Put("/", UpdateUser)
					r.Delete("/", DeleteUser)

					r.Route("/posts", func(r chi.Router) {
						r.Get("/", ListUserPosts)
						r.Post("/", CreateUserPost)
					})
				})
			})

			r.Route("/posts", func(r chi.Router) {
				r.Get("/", ListPosts)
				r.Get("/{postId}", GetPost)
				r.Put("/{postId}", UpdatePost)
				r.Delete("/{postId}", DeletePost)
			})
		})
	})
}
`

	p := chiplugin.New()
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

// TestChiPlugin_ComplexRoutePatterns tests various complex chi routing patterns.
func TestChiPlugin_ComplexRoutePatterns(t *testing.T) {
	source := `package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func SetupRoutes(r chi.Router) {
	// Middleware usage (should not affect route extraction)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Simple routes
	r.Get("/", Home)
	r.Get("/health", Health)
	r.Get("/version", Version)

	// RESTful resources
	r.Route("/articles", func(r chi.Router) {
		r.Get("/", ListArticles)      // GET /articles
		r.Post("/", CreateArticle)    // POST /articles

		r.Route("/{articleSlug}", func(r chi.Router) {
			r.Get("/", GetArticle)         // GET /articles/{articleSlug}
			r.Put("/", UpdateArticle)      // PUT /articles/{articleSlug}
			r.Delete("/", DeleteArticle)   // DELETE /articles/{articleSlug}

			// Nested resources
			r.Route("/comments", func(r chi.Router) {
				r.Get("/", ListComments)       // GET /articles/{articleSlug}/comments
				r.Post("/", CreateComment)     // POST /articles/{articleSlug}/comments
				r.Get("/{commentId}", GetComment) // GET /articles/{articleSlug}/comments/{commentId}
			})
		})
	})

	// Groups with shared configuration
	r.Group(func(r chi.Router) {
		r.Get("/admin/dashboard", AdminDashboard)
		r.Get("/admin/users", AdminUsers)
		r.Get("/admin/settings", AdminSettings)
	})

	// Route with regex constraint (should still extract param name)
	r.Get("/files/{path:.*}", ServeFile)
	r.Get("/users/{id:[0-9]+}", GetUserByID)
}
`

	p := chiplugin.New()
	files := []scanner.SourceFile{
		{
			Path:     "api/routes.go",
			Language: "go",
			Content:  []byte(source),
		},
	}

	routes, err := p.ExtractRoutes(files)
	require.NoError(t, err)

	routeMap := routesByKey(routes)
	assert.Contains(t, routeMap, "GET /")
	assert.Contains(t, routeMap, "GET /health")
	assert.Contains(t, routeMap, "GET /version")

	// Test RESTful routes
	assert.Contains(t, routeMap, "GET /articles")
	assert.Contains(t, routeMap, "POST /articles")
	assert.Contains(t, routeMap, "GET /articles/{articleSlug}")
	assert.Contains(t, routeMap, "PUT /articles/{articleSlug}")
	assert.Contains(t, routeMap, "DELETE /articles/{articleSlug}")

	// Test nested routes
	assert.Contains(t, routeMap, "GET /articles/{articleSlug}/comments")
	assert.Contains(t, routeMap, "POST /articles/{articleSlug}/comments")
	assert.Contains(t, routeMap, "GET /articles/{articleSlug}/comments/{commentId}")

	// Test group routes
	assert.Contains(t, routeMap, "GET /admin/dashboard")
	assert.Contains(t, routeMap, "GET /admin/users")
	assert.Contains(t, routeMap, "GET /admin/settings")

	// Test regex path params
	filesRoute := routeMap["GET /files/{path:.*}"]
	require.Len(t, filesRoute.Parameters, 1)
	assert.Equal(t, "path", filesRoute.Parameters[0].Name)

	usersRoute := routeMap["GET /users/{id:[0-9]+}"]
	require.Len(t, usersRoute.Parameters, 1)
	assert.Equal(t, "id", usersRoute.Parameters[0].Name)

	// Verify multiple path params
	commentRoute := routeMap["GET /articles/{articleSlug}/comments/{commentId}"]
	require.Len(t, commentRoute.Parameters, 2)
	paramNames := []string{commentRoute.Parameters[0].Name, commentRoute.Parameters[1].Name}
	assert.Contains(t, paramNames, "articleSlug")
	assert.Contains(t, paramNames, "commentId")
}
