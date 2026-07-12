// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapAuth_KnownMiddlewares(t *testing.T) {
	tests := []struct {
		middleware string
		component  string
		scheme     SecurityScheme
	}{
		{
			middleware: "authMiddleware",
			component:  "BearerAuth",
			scheme:     SecurityScheme{Type: "http", Scheme: "bearer"},
		},
		{
			middleware: "jwtMiddleware",
			component:  "BearerAuth",
			scheme:     SecurityScheme{Type: "http", Scheme: "bearer"},
		},
		{
			middleware: "apiKeyMiddleware",
			component:  "ApiKeyAuth",
			scheme:     SecurityScheme{Type: "apiKey", In: "header", Name: "X-API-Key"},
		},
		{
			middleware: "basicAuthMiddleware",
			component:  "BasicAuth",
			scheme:     SecurityScheme{Type: "http", Scheme: "basic"},
		},
		{
			middleware: "oauthMiddleware",
			component:  "OAuth2",
			scheme:     SecurityScheme{Type: "oauth2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.middleware, func(t *testing.T) {
			component, scheme := mapAuth([]string{"RateLimit", tc.middleware})
			assert.Equal(t, tc.component, component)
			require.NotNil(t, scheme)
			assert.Equal(t, tc.scheme, *scheme)
		})
	}
}

func TestMapAuth_Unknown(t *testing.T) {
	component, scheme := mapAuth([]string{"RateLimit", "Logger"})
	assert.Empty(t, component)
	assert.Nil(t, scheme)
}

func TestAuthMapperPass_JWT(t *testing.T) {
	routes := []Route{{
		Method:      "GET",
		Path:        "/api/users",
		Middlewares: []string{"RateLimit", "JWTAuth"},
	}}
	result := AuthMapperPass(routes)
	requireSchemes(t, result.Requirements["GET /api/users"], "BearerAuth")
	require.Contains(t, result.Schemes, "BearerAuth")
	assert.Equal(t, SecurityScheme{Type: "http", Scheme: "bearer"}, result.Schemes["BearerAuth"])
}

func TestAuthMapperPass_APIKey(t *testing.T) {
	routes := []Route{{
		Method:      "POST",
		Path:        "/api/items",
		Middlewares: []string{"apiKeyMiddleware"},
	}}
	result := AuthMapperPass(routes)
	requireSchemes(t, result.Requirements["POST /api/items"], "ApiKeyAuth")
	require.Contains(t, result.Schemes, "ApiKeyAuth")
	assert.Equal(t, "X-API-Key", result.Schemes["ApiKeyAuth"].Name)
}

func TestNormalizeRoutePath(t *testing.T) {
	assert.Equal(t, "/users", normalizeRoutePath("/users"))
	assert.Equal(t, "/users", normalizeRoutePath("users"))
}

func TestOpenAPIAssemblerPass(t *testing.T) {
	draft := OpenAPIAssemblerPass(&CollectedData{
		PkgName: "handler",
		Routes:  []Route{{Method: "GET", Path: "/health"}},
		Schemas: map[string]Schema{"User": {Name: "User"}},
		SecuritySchemes: map[string]SecurityScheme{
			"BearerAuth": {Type: "http", Scheme: "bearer"},
		},
	})
	assert.Equal(t, "handler API", draft.Title)
	assert.Len(t, draft.Routes, 1)
	assert.Contains(t, draft.Schemas, "User")
	assert.Contains(t, draft.SecuritySchemes, "BearerAuth")
}

func requireSchemes(t *testing.T, reqs []SecurityRequirement, want string) {
	t.Helper()
	for _, req := range reqs {
		if req.Scheme == want {
			return
		}
	}
	t.Fatalf("expected scheme %q in %+v", want, reqs)
}
