// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/whisskey/prism/tests/testutil"
)

func TestCLI_GenerateOpenAPI(t *testing.T) {
	frameworks := []string{"chi", "echo", "gin", "fiber"}
	for _, framework := range frameworks {
		t.Run(framework, func(t *testing.T) {
			assertGeneratedOpenAPI(t, framework)
		})
	}
}

func assertGeneratedOpenAPI(t *testing.T, framework string) {
	t.Helper()
	root := testutil.RepoRoot(t)
	fixtureDir := testutil.MockDir(t, framework)

	bin := filepath.Join(t.TempDir(), "prism")
	build := exec.Command("go", "build", "-o", bin, "./cmd")
	build.Dir = root
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))

	gen := exec.Command(bin, "generate")
	gen.Dir = fixtureDir
	out, err = gen.CombinedOutput()
	require.NoError(t, err, string(out))

	specPath := filepath.Join(fixtureDir, "openapi.yaml")
	data, err := os.ReadFile(specPath)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(data, &doc))

	paths, ok := doc["paths"].(map[string]any)
	require.True(t, ok)
	users, ok := paths["/api/v1/users"].(map[string]any)
	require.True(t, ok, "expected /api/v1/users path")
	get, ok := users["get"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "List users", get["summary"])

	security, _ := get["security"].([]any)
	require.NotEmpty(t, security, "expected BearerAuth security on list users")
	firstReq, _ := security[0].(map[string]any)
	assert.Contains(t, firstReq, "BearerAuth")

	params, _ := get["parameters"].([]any)
	paramNames := map[string]bool{}
	for _, raw := range params {
		p, _ := raw.(map[string]any)
		if p["in"] == "query" {
			paramNames[testutil.AsString(p["name"])] = true
		}
	}
	assert.True(t, paramNames["page"])
	assert.True(t, paramNames["per_page"])

	responses, ok := get["responses"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, responses, "401")
	assert.Contains(t, responses, "403")
	assert.Contains(t, responses, "200")

	ok200 := responses["200"].(map[string]any)
	content := ok200["content"].(map[string]any)
	appJSON := content["application/json"].(map[string]any)
	schema := appJSON["schema"].(map[string]any)
	props := schema["properties"].(map[string]any)
	dataProp := props["data"].(map[string]any)
	assert.Equal(t, "array", dataProp["type"])
	items := dataProp["items"].(map[string]any)
	assert.Equal(t, "#/components/schemas/User", items["$ref"])

	components := doc["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	responseSchema := schemas["Response"].(map[string]any)
	respProps := responseSchema["properties"].(map[string]any)
	dataField := respProps["data"].(map[string]any)
	assert.NotContains(t, testutil.AsString(dataField["$ref"]), "schemas/any")

	post, ok := users["post"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Create user", post["summary"])
	reqBody, ok := post["requestBody"].(map[string]any)
	require.True(t, ok)
	reqBodyContent := reqBody["content"].(map[string]any)
	postJSON := reqBodyContent["application/json"].(map[string]any)
	postSchema := postJSON["schema"].(map[string]any)
	assert.Equal(t, "#/components/schemas/CreateUserRequest", postSchema["$ref"])

	byID, ok := paths["/api/v1/users/{id}"].(map[string]any)
	require.True(t, ok)
	getByID, ok := byID["get"].(map[string]any)
	require.True(t, ok)
	idParams, _ := getByID["parameters"].([]any)
	var idType string
	for _, raw := range idParams {
		p, _ := raw.(map[string]any)
		if p["in"] == "path" && testutil.AsString(p["name"]) == "id" {
			schema := p["schema"].(map[string]any)
			idType = testutil.AsString(schema["type"])
		}
	}
	assert.Equal(t, "integer", idType)

	assert.Contains(t, schemas, "User")
	assert.Contains(t, schemas, "CreateUserRequest")
	assert.Contains(t, schemas, "Response")
}
