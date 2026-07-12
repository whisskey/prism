// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeResults_HandlersAndRoutes(t *testing.T) {
	catalog := MergeResults([]*Result{
		{
			Data: &CollectedData{
				Routes: []Route{{Method: "GET", Path: "/users", Handler: "userCtrl.List"}},
				Handlers: map[string]HandlerDetail{
					"List": {Name: "List", ResponseType: "[]User"},
				},
			},
		},
		{
			Data: &CollectedData{
				Schemas: map[string]Schema{
					"User": {Name: "User", Type: "object", Properties: map[string]Property{"id": {Type: "string"}}},
				},
			},
		},
	})

	require.NotNil(t, catalog.Merged)
	assert.Len(t, catalog.Merged.Routes, 1)
	assert.Contains(t, catalog.Merged.Schemas, "User")

	detail, ok := catalog.Merged.LookupHandler("userCtrl.List", "List")
	require.True(t, ok)
	assert.Equal(t, "[]User", detail.ResponseType)
}
