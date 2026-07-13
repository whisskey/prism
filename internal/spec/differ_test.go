// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	oas "github.com/whisskey/prism/pkg/openapi"
)

func TestNewDiffer(t *testing.T) {
	differ := NewDiffer()
	assert.NotNil(t, differ)
}

func TestDiffer_Diff_NoDifferences(t *testing.T) {
	doc := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Info: oas.Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
		Paths: map[string]oas.PathItem{
			"/users": {
				Get: &oas.Operation{Summary: "List users"},
			},
		},
	}

	differ := NewDiffer()
	result, err := differ.Diff(doc, doc)

	require.NoError(t, err)
	assert.True(t, result.IsEmpty())
	assert.False(t, result.HasBreakingChanges)
	assert.Equal(t, "No changes detected", result.Summary)
}

func TestDiffer_Diff_AddedPath(t *testing.T) {
	a := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {Get: &oas.Operation{Summary: "List users"}},
		},
	}

	b := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {Get: &oas.Operation{Summary: "List users"}},
			"/posts": {Get: &oas.Operation{Summary: "List posts"}},
		},
	}

	differ := NewDiffer()
	result, err := differ.Diff(a, b)

	require.NoError(t, err)
	assert.Len(t, result.PathChanges, 1)
	assert.Equal(t, DiffTypeAdded, result.PathChanges[0].Type)
	assert.Equal(t, "/posts", result.PathChanges[0].Path)
	assert.Equal(t, "GET", result.PathChanges[0].Method)
	assert.False(t, result.HasBreakingChanges)
}

func TestDiffer_Diff_RemovedPath(t *testing.T) {
	a := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {Get: &oas.Operation{Summary: "List users"}},
			"/posts": {Get: &oas.Operation{Summary: "List posts"}},
		},
	}

	b := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {Get: &oas.Operation{Summary: "List users"}},
		},
	}

	differ := NewDiffer()
	result, err := differ.Diff(a, b)

	require.NoError(t, err)
	assert.Len(t, result.PathChanges, 1)
	assert.Equal(t, DiffTypeRemoved, result.PathChanges[0].Type)
	assert.Equal(t, "/posts", result.PathChanges[0].Path)
	assert.True(t, result.HasBreakingChanges)
}

func TestDiffer_Diff_AddedMethod(t *testing.T) {
	a := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {Get: &oas.Operation{Summary: "List users"}},
		},
	}

	b := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {
				Get:  &oas.Operation{Summary: "List users"},
				Post: &oas.Operation{Summary: "Create user"},
			},
		},
	}

	differ := NewDiffer()
	result, err := differ.Diff(a, b)

	require.NoError(t, err)
	assert.Len(t, result.PathChanges, 1)
	assert.Equal(t, DiffTypeAdded, result.PathChanges[0].Type)
	assert.Equal(t, "/users", result.PathChanges[0].Path)
	assert.Equal(t, "POST", result.PathChanges[0].Method)
}

func TestDiffer_Diff_RemovedMethod(t *testing.T) {
	a := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {
				Get:  &oas.Operation{Summary: "List users"},
				Post: &oas.Operation{Summary: "Create user"},
			},
		},
	}

	b := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {Get: &oas.Operation{Summary: "List users"}},
		},
	}

	differ := NewDiffer()
	result, err := differ.Diff(a, b)

	require.NoError(t, err)
	assert.Len(t, result.PathChanges, 1)
	assert.Equal(t, DiffTypeRemoved, result.PathChanges[0].Type)
	assert.Equal(t, "POST", result.PathChanges[0].Method)
	assert.True(t, result.HasBreakingChanges)
}

func TestDiffer_Diff_ModifiedOperation(t *testing.T) {
	a := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {
				Get: &oas.Operation{
					Summary:     "List users",
					Description: "Original description",
				},
			},
		},
	}

	b := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {
				Get: &oas.Operation{
					Summary:     "Get all users",
					Description: "Updated description",
				},
			},
		},
	}

	differ := NewDiffer()
	result, err := differ.Diff(a, b)

	require.NoError(t, err)
	assert.Len(t, result.PathChanges, 1)
	assert.Equal(t, DiffTypeModified, result.PathChanges[0].Type)
}

func TestDiffer_Diff_AddedSchema(t *testing.T) {
	a := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Components: &oas.Components{
			Schemas: map[string]*oas.Schema{
				"User": {Type: "object"},
			},
		},
	}

	b := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Components: &oas.Components{
			Schemas: map[string]*oas.Schema{
				"User": {Type: "object"},
				"Post": {Type: "object"},
			},
		},
	}

	differ := NewDiffer()
	result, err := differ.Diff(a, b)

	require.NoError(t, err)
	assert.Len(t, result.SchemaChanges, 1)
	assert.Equal(t, DiffTypeAdded, result.SchemaChanges[0].Type)
	assert.Equal(t, "Post", result.SchemaChanges[0].Name)
}

func TestDiffer_Diff_RemovedSchema(t *testing.T) {
	a := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Components: &oas.Components{
			Schemas: map[string]*oas.Schema{
				"User": {Type: "object"},
				"Post": {Type: "object"},
			},
		},
	}

	b := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Components: &oas.Components{
			Schemas: map[string]*oas.Schema{
				"User": {Type: "object"},
			},
		},
	}

	differ := NewDiffer()
	result, err := differ.Diff(a, b)

	require.NoError(t, err)
	assert.Len(t, result.SchemaChanges, 1)
	assert.Equal(t, DiffTypeRemoved, result.SchemaChanges[0].Type)
	assert.Equal(t, "Post", result.SchemaChanges[0].Name)
	assert.True(t, result.HasBreakingChanges)
}

func TestDiffer_Diff_ModifiedSchema(t *testing.T) {
	a := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Components: &oas.Components{
			Schemas: map[string]*oas.Schema{
				"User": {
					Type:        "object",
					Description: "Original",
				},
			},
		},
	}

	b := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Components: &oas.Components{
			Schemas: map[string]*oas.Schema{
				"User": {
					Type:        "object",
					Description: "Modified",
				},
			},
		},
	}

	differ := NewDiffer()
	result, err := differ.Diff(a, b)

	require.NoError(t, err)
	assert.Len(t, result.SchemaChanges, 1)
	assert.Equal(t, DiffTypeModified, result.SchemaChanges[0].Type)
}

func TestDiffer_Diff_NilDocuments(t *testing.T) {
	differ := NewDiffer()

	// Both nil
	result, err := differ.Diff(nil, nil)
	require.NoError(t, err)
	assert.True(t, result.IsEmpty())

	// First nil
	b := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {Get: &oas.Operation{}},
		},
	}
	result, err = differ.Diff(nil, b)
	require.NoError(t, err)
	assert.Len(t, result.PathChanges, 1)
	assert.Equal(t, DiffTypeAdded, result.PathChanges[0].Type)

	// Second nil
	a := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {Get: &oas.Operation{}},
		},
	}
	result, err = differ.Diff(a, nil)
	require.NoError(t, err)
	assert.Len(t, result.PathChanges, 1)
	assert.Equal(t, DiffTypeRemoved, result.PathChanges[0].Type)
}

func TestDiffer_Diff_MultipleChanges(t *testing.T) {
	a := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users":   {Get: &oas.Operation{}},
			"/removed": {Get: &oas.Operation{}},
		},
		Components: &oas.Components{
			Schemas: map[string]*oas.Schema{
				"User":          {Type: "object"},
				"RemovedSchema": {Type: "object"},
			},
		},
	}

	b := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {Get: &oas.Operation{}, Post: &oas.Operation{}},
			"/added": {Get: &oas.Operation{}},
		},
		Components: &oas.Components{
			Schemas: map[string]*oas.Schema{
				"User":      {Type: "object", Description: "Modified"},
				"NewSchema": {Type: "object"},
			},
		},
	}

	differ := NewDiffer()
	result, err := differ.Diff(a, b)

	require.NoError(t, err)
	assert.True(t, result.HasBreakingChanges)

	// Path changes: removed /removed (breaking), added /added, added POST /users
	pathAdded := 0
	pathRemoved := 0
	for _, c := range result.PathChanges {
		if c.Type == DiffTypeAdded {
			pathAdded++
		} else if c.Type == DiffTypeRemoved {
			pathRemoved++
		}
	}
	assert.Equal(t, 2, pathAdded)
	assert.Equal(t, 1, pathRemoved)

	// Schema changes: removed RemovedSchema (breaking), added NewSchema, modified User
	schemaAdded := 0
	schemaRemoved := 0
	schemaModified := 0
	for _, c := range result.SchemaChanges {
		switch c.Type {
		case DiffTypeAdded:
			schemaAdded++
		case DiffTypeRemoved:
			schemaRemoved++
		case DiffTypeModified:
			schemaModified++
		}
	}
	assert.Equal(t, 1, schemaAdded)
	assert.Equal(t, 1, schemaRemoved)
	assert.Equal(t, 1, schemaModified)
}

func TestDiffResult_IsEmpty(t *testing.T) {
	result := &DiffResult{
		PathChanges:   []PathChange{},
		SchemaChanges: []SchemaChange{},
	}
	assert.True(t, result.IsEmpty())

	result.PathChanges = append(result.PathChanges, PathChange{})
	assert.False(t, result.IsEmpty())
}

func TestFormatDiff_Empty(t *testing.T) {
	result := &DiffResult{}
	output := FormatDiff(result)
	assert.Equal(t, "No differences found.", output)
}

func TestFormatDiff_WithChanges(t *testing.T) {
	result := &DiffResult{
		PathChanges: []PathChange{
			{Type: DiffTypeAdded, Path: "/users", Method: "POST"},
			{Type: DiffTypeRemoved, Path: "/old", Method: "GET"},
			{Type: DiffTypeModified, Path: "/modified", Method: "PUT"},
		},
		SchemaChanges: []SchemaChange{
			{Type: DiffTypeAdded, Name: "NewSchema"},
			{Type: DiffTypeRemoved, Name: "OldSchema"},
		},
		HasBreakingChanges: true,
		Summary:            "3 path(s) changed, 2 schema(s) changed",
	}

	output := FormatDiff(result)

	assert.Contains(t, output, "=== OpenAPI Diff ===")
	assert.Contains(t, output, "--- Path Changes ---")
	assert.Contains(t, output, "+ POST /users")
	assert.Contains(t, output, "- GET /old")
	assert.Contains(t, output, "~ PUT /modified")
	assert.Contains(t, output, "--- Schema Changes ---")
	assert.Contains(t, output, "+ NewSchema")
	assert.Contains(t, output, "- OldSchema")
}

func TestDiffer_Diff_DeprecatedChange(t *testing.T) {
	a := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {
				Get: &oas.Operation{
					Summary:    "List users",
					Deprecated: false,
				},
			},
		},
	}

	b := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {
				Get: &oas.Operation{
					Summary:    "List users",
					Deprecated: true,
				},
			},
		},
	}

	differ := NewDiffer()
	result, err := differ.Diff(a, b)

	require.NoError(t, err)
	assert.Len(t, result.PathChanges, 1)
	assert.Equal(t, DiffTypeModified, result.PathChanges[0].Type)
}

func TestDiffer_Diff_ParameterCountChange(t *testing.T) {
	a := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {
				Get: &oas.Operation{
					Parameters: []oas.Parameter{
						{Name: "page", In: "query"},
					},
				},
			},
		},
	}

	b := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/users": {
				Get: &oas.Operation{
					Parameters: []oas.Parameter{
						{Name: "page", In: "query"},
						{Name: "limit", In: "query"},
					},
				},
			},
		},
	}

	differ := NewDiffer()
	result, err := differ.Diff(a, b)

	require.NoError(t, err)
	assert.Len(t, result.PathChanges, 1)
	assert.Equal(t, DiffTypeModified, result.PathChanges[0].Type)
}

func TestDiffer_Diff_AllMethods(t *testing.T) {
	a := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/test": {},
		},
	}

	b := &oas.OpenAPI{
		OpenAPI: "3.0.3",
		Paths: map[string]oas.PathItem{
			"/test": {
				Get:     &oas.Operation{},
				Post:    &oas.Operation{},
				Put:     &oas.Operation{},
				Delete:  &oas.Operation{},
				Patch:   &oas.Operation{},
				Options: &oas.Operation{},
				Head:    &oas.Operation{},
				Trace:   &oas.Operation{},
			},
		},
	}

	differ := NewDiffer()
	result, err := differ.Diff(a, b)

	require.NoError(t, err)
	assert.Len(t, result.PathChanges, 8)

	methods := make(map[string]bool)
	for _, c := range result.PathChanges {
		methods[c.Method] = true
	}

	assert.True(t, methods["GET"])
	assert.True(t, methods["POST"])
	assert.True(t, methods["PUT"])
	assert.True(t, methods["DELETE"])
	assert.True(t, methods["PATCH"])
	assert.True(t, methods["OPTIONS"])
	assert.True(t, methods["HEAD"])
	assert.True(t, methods["TRACE"])
}
