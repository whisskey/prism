// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/pkg/openapi"
)

const modelsSource = `package models

import "time"

// User represents a user in the system.
type User struct {
	// ID is the unique identifier for the user.
	ID        string    ` + "`json:\"id\"`" + `

	// Name is the user's display name.
	Name      string    ` + "`json:\"name\" validate:\"required,min=1,max=255\"`" + `

	// Email is the user's email address.
	Email     string    ` + "`json:\"email\" validate:\"required,email\"`" + `

	// Age is the user's age in years.
	Age       *int      ` + "`json:\"age,omitempty\" validate:\"min=0,max=150\"`" + `

	// Role is the user's role in the system.
	Role      string    ` + "`json:\"role\" validate:\"oneof=admin user guest\"`" + `

	// Tags are custom tags for the user.
	Tags      []string  ` + "`json:\"tags\"`" + `

	// Metadata contains additional user metadata.
	Metadata  map[string]string ` + "`json:\"metadata,omitempty\"`" + `

	// CreatedAt is when the user was created.
	CreatedAt time.Time ` + "`json:\"created_at\"`" + `

	// UpdatedAt is when the user was last updated.
	UpdatedAt *time.Time ` + "`json:\"updated_at,omitempty\"`" + `

	// Password is never serialized.
	Password  string    ` + "`json:\"-\"`" + `
}

// CreateUserRequest is the request body for creating a user.
type CreateUserRequest struct {
	Name     string   ` + "`json:\"name\" validate:\"required,min=1,max=255\"`" + `
	Email    string   ` + "`json:\"email\" validate:\"required,email\"`" + `
	Password string   ` + "`json:\"password\" validate:\"required,min=8\"`" + `
	Role     string   ` + "`json:\"role\" validate:\"oneof=admin user guest\"`" + `
	Tags     []string ` + "`json:\"tags,omitempty\"`" + `
}

// UpdateUserRequest is the request body for updating a user.
type UpdateUserRequest struct {
	Name  *string  ` + "`json:\"name,omitempty\" validate:\"omitempty,min=1,max=255\"`" + `
	Email *string  ` + "`json:\"email,omitempty\" validate:\"omitempty,email\"`" + `
	Role  *string  ` + "`json:\"role,omitempty\" validate:\"omitempty,oneof=admin user guest\"`" + `
	Tags  []string ` + "`json:\"tags,omitempty\"`" + `
}

// Post represents a blog post.
type Post struct {
	ID        string    ` + "`json:\"id\"`" + `
	Title     string    ` + "`json:\"title\" validate:\"required,min=1,max=500\"`" + `
	Content   string    ` + "`json:\"content\" validate:\"required\"`" + `
	AuthorID  string    ` + "`json:\"author_id\"`" + `
	Published bool      ` + "`json:\"published\"`" + `
	Tags      []string  ` + "`json:\"tags\"`" + `
	CreatedAt time.Time ` + "`json:\"created_at\"`" + `
}
`

var standardExpectedRoutes = []string{
	"GET /api/health",
	"GET /api/v1/users",
	"POST /api/v1/users",
	"GET /api/v1/users/{userId}",
	"PUT /api/v1/users/{userId}",
	"DELETE /api/v1/users/{userId}",
	"GET /api/v1/users/{userId}/posts",
	"POST /api/v1/users/{userId}/posts",
	"GET /api/v1/posts",
	"GET /api/v1/posts/{postId}",
}

func routesByKey(routes []openapi.Route) map[string]openapi.Route {
	routeMap := make(map[string]openapi.Route, len(routes))
	for _, r := range routes {
		routeMap[r.Method+" "+r.Path] = r
	}
	return routeMap
}

func assertStandardRoutes(t *testing.T, routes []openapi.Route) {
	t.Helper()
	require.GreaterOrEqual(t, len(routes), 10, "expected at least 10 routes")
	routeMap := routesByKey(routes)
	for _, expected := range standardExpectedRoutes {
		route, exists := routeMap[expected]
		assert.True(t, exists, "route %s should exist", expected)
		if exists {
			assert.NotEmpty(t, route.Handler, "route %s should have a handler", expected)
		}
	}
	getUserRoute := routeMap["GET /api/v1/users/{userId}"]
	require.Len(t, getUserRoute.Parameters, 1, "GET /users/{userId} should have 1 path param")
	assert.Equal(t, "userId", getUserRoute.Parameters[0].Name)
	assert.Equal(t, "path", getUserRoute.Parameters[0].In)
	assert.True(t, getUserRoute.Parameters[0].Required)
	assert.Contains(t, getUserRoute.Tags, "users")
}

func assertStandardSchemas(t *testing.T, schemas []openapi.Schema) {
	t.Helper()
	require.GreaterOrEqual(t, len(schemas), 4, "expected at least 4 schemas")
	schemaMap := make(map[string]openapi.Schema, len(schemas))
	for _, s := range schemas {
		schemaMap[s.Title] = s
	}

	userSchema, exists := schemaMap["User"]
	require.True(t, exists, "User schema should exist")
	assert.Equal(t, "object", userSchema.Type)
	assert.Contains(t, userSchema.Required, "name")
	assert.Contains(t, userSchema.Required, "email")
	require.NotNil(t, userSchema.Properties)
	assert.NotContains(t, userSchema.Properties, "password")
	assert.Equal(t, "email", userSchema.Properties["email"].Format)
	assert.Len(t, userSchema.Properties["role"].Enum, 3)
	assert.Equal(t, "array", userSchema.Properties["tags"].Type)
	assert.Equal(t, "date-time", userSchema.Properties["created_at"].Format)
	assert.True(t, userSchema.Properties["age"].Nullable)

	createUserSchema, exists := schemaMap["CreateUserRequest"]
	require.True(t, exists, "CreateUserRequest schema should exist")
	assert.Contains(t, createUserSchema.Required, "name")
	assert.Contains(t, createUserSchema.Required, "email")
	assert.Contains(t, createUserSchema.Required, "password")

	updateUserSchema, exists := schemaMap["UpdateUserRequest"]
	require.True(t, exists, "UpdateUserRequest schema should exist")
	assert.NotContains(t, updateUserSchema.Required, "name")
	assert.NotContains(t, updateUserSchema.Required, "email")

	postSchema, exists := schemaMap["Post"]
	require.True(t, exists, "Post schema should exist")
	assert.Contains(t, postSchema.Required, "title")
	assert.Contains(t, postSchema.Required, "content")
}
