// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/whisskey/prism/internal/config"
	oas "github.com/whisskey/prism/pkg/openapi"
)

func TestPrepareRoutes_PreservesDiscoveryOrder(t *testing.T) {
	routes := []oas.Route{
		{Method: "GET", Path: "/z-last", OperationID: "getZLast"},
		{Method: "GET", Path: "/a-first", OperationID: "getAFirst"},
	}

	prepared := prepareRoutes(routes, true)
	require.Len(t, prepared, 2)
	assert.Equal(t, "/z-last", prepared[0].Path)
	assert.Equal(t, "/a-first", prepared[1].Path)
}

func TestPrepareRoutes_SortsWhenDisabled(t *testing.T) {
	routes := []oas.Route{
		{Method: "GET", Path: "/z-last", OperationID: "getZLast"},
		{Method: "GET", Path: "/a-first", OperationID: "getAFirst"},
	}

	prepared := prepareRoutes(routes, false)
	require.Len(t, prepared, 2)
	assert.Equal(t, "/a-first", prepared[0].Path)
	assert.Equal(t, "/z-last", prepared[1].Path)
}

func TestAssembler_Assemble_SortsSchemasAlphabetically(t *testing.T) {
	cfg := config.Default()

	schemas := []oas.Schema{
		{Title: "Zebra", Type: "object"},
		{Title: "Alpha", Type: "object"},
		{Title: "Zebra", Type: "object", Description: "duplicate"},
	}

	assembler := NewAssembler(cfg)
	doc, err := assembler.Assemble(nil, schemas)
	require.NoError(t, err)
	require.NotNil(t, doc.Components)

	keys := SortedSchemas(doc.Components.Schemas)
	require.Len(t, keys, 2)
	assert.Equal(t, "Alpha", keys[0])
	assert.Equal(t, "Zebra", keys[1])
}

func TestPrepareComponentSchemas_DeduplicatesByTitle(t *testing.T) {
	schemas := prepareComponentSchemas([]oas.Schema{
		{Title: "Foo", Type: "object", Description: "first"},
		{Title: "Foo", Type: "object", Description: "second"},
		{Title: "Bar", Type: "object"},
	})

	require.Len(t, schemas, 2)
	assert.Equal(t, "Bar", schemas[0].Title)
	assert.Equal(t, "Foo", schemas[1].Title)
}
