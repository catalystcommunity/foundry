package gatewayroute

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackend(t *testing.T) {
	route := Backend(BackendOptions{
		Name:           "example",
		Namespace:      "apps",
		Hostname:       "example.test",
		ParentSections: []string{"http", "https"},
		ServiceName:    "example-service",
		ServicePort:    8080,
		Ownership: Ownership{
			Component:        "example",
			ManagedBy:        "Helm",
			ReleaseName:      "example-release",
			ReleaseNamespace: "apps",
		},
	})

	metadata := route["metadata"].(map[string]interface{})
	assert.Equal(t, "example", metadata["name"])
	assert.Equal(t, "apps", metadata["namespace"])
	annotations := metadata["annotations"].(map[string]interface{})
	assert.Equal(t, "example-release", annotations["meta.helm.sh/release-name"])

	spec := route["spec"].(map[string]interface{})
	parents := spec["parentRefs"].([]interface{})
	require.Len(t, parents, 2)
	assert.Equal(t, "http", parents[0].(map[string]interface{})["sectionName"])
	assert.Equal(t, "https", parents[1].(map[string]interface{})["sectionName"])
	rules := spec["rules"].([]interface{})
	backendRefs := rules[0].(map[string]interface{})["backendRefs"].([]interface{})
	backend := backendRefs[0].(map[string]interface{})
	assert.Equal(t, "example-service", backend["name"])
	assert.Equal(t, 8080, backend["port"])
}

func TestRedirectToHTTPS(t *testing.T) {
	route := RedirectToHTTPS(RedirectOptions{
		Name:      "example-http-redirect",
		Namespace: "apps",
		Hostname:  "example.test",
		Ownership: Ownership{Component: "example"},
	})

	spec := route["spec"].(map[string]interface{})
	parent := spec["parentRefs"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "http", parent["sectionName"])
	rule := spec["rules"].([]interface{})[0].(map[string]interface{})
	filter := rule["filters"].([]interface{})[0].(map[string]interface{})
	redirect := filter["requestRedirect"].(map[string]interface{})
	assert.Equal(t, "https", redirect["scheme"])
	assert.Equal(t, 301, redirect["statusCode"])
}

func TestAppendObjectsPreservesExistingObjects(t *testing.T) {
	existing := map[string]interface{}{"kind": "ConfigMap"}
	added := map[string]interface{}{"kind": "HTTPRoute"}
	values := map[string]interface{}{"extraObjects": []map[string]interface{}{existing}}

	AppendObjects(values, "extraObjects", added)

	objects := values["extraObjects"].([]interface{})
	require.Len(t, objects, 2)
	assert.Equal(t, existing, objects[0])
	assert.Equal(t, added, objects[1])
}

func TestManifest(t *testing.T) {
	manifest, err := Manifest(
		map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap"},
		map[string]interface{}{"apiVersion": "v1", "kind": "Secret"},
	)
	require.NoError(t, err)

	documents := strings.Split(manifest, "\n---\n")
	require.Len(t, documents, 2)
	for _, document := range documents {
		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(document), &decoded))
	}
}
