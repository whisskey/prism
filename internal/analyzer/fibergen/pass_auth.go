// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import "strings"

type authMapping struct {
	component string
	scheme    SecurityScheme
}

var knownAuthMiddlewares = map[string]authMapping{
	"authMiddleware":      {component: "BearerAuth", scheme: SecurityScheme{Type: "http", Scheme: "bearer"}},
	"jwtMiddleware":       {component: "BearerAuth", scheme: SecurityScheme{Type: "http", Scheme: "bearer"}},
	"bearerMiddleware":    {component: "BearerAuth", scheme: SecurityScheme{Type: "http", Scheme: "bearer"}},
	"apiKeyMiddleware":    {component: "ApiKeyAuth", scheme: SecurityScheme{Type: "apiKey", In: "header", Name: "X-API-Key"}},
	"basicAuthMiddleware": {component: "BasicAuth", scheme: SecurityScheme{Type: "http", Scheme: "basic"}},
	"oauthMiddleware":     {component: "OAuth2", scheme: SecurityScheme{Type: "oauth2"}},
}

// AuthMapperResult holds route security requirements and component scheme definitions.
type AuthMapperResult struct {
	Requirements map[string][]SecurityRequirement
	Schemes      map[string]SecurityScheme
}

// AuthMapperPass maps route middleware names to OpenAPI security (Pass 4).
func AuthMapperPass(routes []Route) AuthMapperResult {
	result := AuthMapperResult{
		Requirements: make(map[string][]SecurityRequirement),
		Schemes:      make(map[string]SecurityScheme),
	}

	for _, route := range routes {
		component, scheme := mapAuth(route.Middlewares)
		if component == "" || scheme == nil {
			continue
		}
		key := route.Method + " " + route.Path
		result.Requirements[key] = []SecurityRequirement{{Scheme: component}}
		result.Schemes[component] = *scheme
	}

	return result
}

func mapAuth(middlewares []string) (componentName string, scheme *SecurityScheme) {
	for _, mw := range middlewares {
		if mapping, ok := knownAuthMiddlewares[mw]; ok {
			s := mapping.scheme
			return mapping.component, &s
		}
		if mapping, ok := inferAuthMiddleware(mw); ok {
			s := mapping.scheme
			return mapping.component, &s
		}
	}
	return "", nil
}

func inferAuthMiddleware(name string) (authMapping, bool) {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "apikey") || strings.Contains(lower, "api_key"):
		return knownAuthMiddlewares["apiKeyMiddleware"], true
	case strings.Contains(lower, "basicauth") || strings.Contains(lower, "basic_auth"):
		return knownAuthMiddlewares["basicAuthMiddleware"], true
	case strings.Contains(lower, "oauth"):
		return knownAuthMiddlewares["oauthMiddleware"], true
	case strings.Contains(lower, "jwt") || strings.Contains(lower, "bearer") || strings.Contains(lower, "auth"):
		return knownAuthMiddlewares["authMiddleware"], true
	default:
		return authMapping{}, false
	}
}

// SchemeForMiddleware returns the OpenAPI security scheme name for a middleware identifier.
func SchemeForMiddleware(name string) (string, bool) {
	component, scheme := mapAuth([]string{name})
	if component == "" || scheme == nil {
		return "", false
	}
	return component, true
}

// SecuritySchemeForName returns a component security scheme definition when known.
func SecuritySchemeForName(name string) (SecurityScheme, bool) {
	_, scheme := mapAuth([]string{name})
	if scheme == nil {
		return SecurityScheme{}, false
	}
	return *scheme, true
}
