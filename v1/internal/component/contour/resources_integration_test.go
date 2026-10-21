package contour

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestChartResources uses the actual pinned chart, including its shared templates.
// Set FOUNDRY_CONTOUR_TEST_CHART to a downloaded contour-0.1.0.tgz archive.
func TestChartResources(t *testing.T) {
	chart := os.Getenv("FOUNDRY_CONTOUR_TEST_CHART")
	if testing.Short() || chart == "" {
		t.Skip("set FOUNDRY_CONTOUR_TEST_CHART to run the Helm rendering test")
	}
	for _, custom := range []bool{false, true} {
		cfg := DefaultConfig()
		memory := "512Mi"
		if custom {
			memory = "1Gi"
			for _, name := range []string{"contour", "envoy"} {
				cfg.Values[name] = map[string]interface{}{"resources": map[string]interface{}{
					"requests": map[string]interface{}{"memory": memory},
					"limits":   map[string]interface{}{"memory": memory},
				}}
			}
		}
		helmValues := buildHelmValues(cfg)
		selectGatewayMode(helmValues)
		values, err := yaml.Marshal(helmValues)
		require.NoError(t, err)
		file := filepath.Join(t.TempDir(), "values.yaml")
		require.NoError(t, os.WriteFile(file, values, 0600))
		output, err := exec.Command("helm", "template", "contour", chart, "-n", "projectcontour", "-f", file).CombinedOutput()
		require.NoError(t, err, "%s", output)
		decoder := yaml.NewDecoder(bytes.NewReader(output))
		seen := map[string]bool{}
		for {
			var object map[string]interface{}
			err := decoder.Decode(&object)
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			if object["kind"] != "Deployment" && object["kind"] != "DaemonSet" {
				continue
			}
			spec := object["spec"].(map[string]interface{})
			if object["kind"] == "Deployment" {
				assert.Equal(t, 2, spec["replicas"], "chart must honor the configured controller replica count")
			}
			pod := spec["template"].(map[string]interface{})["spec"].(map[string]interface{})
			for _, raw := range pod["containers"].([]interface{}) {
				container := raw.(map[string]interface{})
				name := container["name"].(string)
				if name != "contour" && name != "envoy" {
					continue
				}
				seen[name] = true
				if name == "contour" {
					assert.Contains(t, container["args"], "--contour-config-name=contour")
					assert.NotContains(t, container["args"], "--config-path=/config/contour.yaml")
				}
				resources := container["resources"].(map[string]interface{})
				assert.Equal(t, memory, resources["requests"].(map[string]interface{})["memory"])
				assert.Equal(t, memory, resources["limits"].(map[string]interface{})["memory"])
				assert.Equal(t, "500m", resources["limits"].(map[string]interface{})["cpu"])
			}
		}
		assert.Equal(t, map[string]bool{"contour": true, "envoy": true}, seen)
	}
}
