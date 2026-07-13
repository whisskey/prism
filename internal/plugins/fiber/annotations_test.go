// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fiber

import (
	"testing"

	"github.com/whisskey/prism/internal/parser"
	"github.com/whisskey/prism/pkg/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyAnnotationEscapeHatch_RequestBodyWins(t *testing.T) {
	p := New()
	p.usedSchemas = make(map[string]bool)

	route := &openapi.Route{
		Method: "POST",
		Path:   "/users",
		RequestBody: jsonRequestBody("InferredRequest"),
		HandlerRef: openapi.HandlerRef{
			Method: "CreateUser",
		},
	}

	doc := &parser.DocAnnotations{
		Parameters: []parser.ParamAnnotation{{
			Name: "body",
			In:   "body",
			Type: "AnnotatedRequest",
		}},
	}

	p.applyAnnotationEscapeHatch(route, &HandlerAnalysis{Doc: doc})

	require.NotNil(t, route.RequestBody)
	media := route.RequestBody.Content["application/json"]
	require.NotNil(t, media.Schema)
	assert.Equal(t, "#/components/schemas/AnnotatedRequest", media.Schema.Ref)
	assert.Contains(t, p.usedSchemas, "AnnotatedRequest")
}

func TestApplyAnnotationEscapeHatch_ResponseSchemaWins(t *testing.T) {
	p := New()
	p.usedSchemas = make(map[string]bool)

	route := &openapi.Route{
		Method: "GET",
		Path:   "/users/{id}",
		Responses: map[string]openapi.Response{
			"200": jsonResponseWithExample("ok", schemaRef("InferredUser"), nil),
		},
	}

	doc := &parser.DocAnnotations{
		Responses: []parser.ResponseAnnotation{{
			Code: "200",
			Type: "AnnotatedUser",
		}},
	}

	p.applyAnnotationEscapeHatch(route, &HandlerAnalysis{Doc: doc})

	resp := route.Responses["200"]
	require.NotNil(t, resp.Content)
	assert.Equal(t, "#/components/schemas/AnnotatedUser", resp.Content["application/json"].Schema.Ref)
}

func TestMergeRequestBodyWithAnnotations_PrefersAnnotatedWhenBothPresent(t *testing.T) {
	p := New()
	p.usedSchemas = make(map[string]bool)
	route := &openapi.Route{
		Method:      "POST",
		RequestBody: jsonRequestBody("Inferred"),
	}
	doc := &parser.DocAnnotations{
		Parameters: []parser.ParamAnnotation{{
			In:   "body",
			Type: "Annotated",
		}},
	}
	p.mergeRequestBodyWithAnnotations(route, doc)
	assert.Equal(t, "#/components/schemas/Annotated", route.RequestBody.Content["application/json"].Schema.Ref)
}
