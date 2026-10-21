package component

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/setup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayDependencyUsesCRDs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   bool
	}{{"installed without Helm release", http.StatusOK, true}, {"missing CRD", http.StatusNotFound, false}, {"API failure", http.StatusServiceUnavailable, false}} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Contains(t, r.URL.Path, "/customresourcedefinitions/")
				requests++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, `{"apiVersion":"apiextensions.k8s.io/v1","kind":"CustomResourceDefinition","metadata":{"name":"test","annotations":{"gateway.networking.k8s.io/bundle-version":"v1.3.0"}}}`)
			}))
			defer server.Close()
			dir := t.TempDir()
			t.Setenv("FOUNDRY_CONFIG_DIR", dir)
			kubeconfig := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: test\n  cluster:\n    server: %s\ncontexts:\n- name: test\n  context:\n    cluster: test\ncurrent-context: test\n", server.URL)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "kubeconfig"), []byte(kubeconfig), 0600))
			cfg := &config.Config{SetupState: &setup.SetupState{K8sInstalled: true}}
			assert.Equal(t, tc.want, isDependencyInstalled("gateway-api", cfg))
			assert.Greater(t, requests, 0)
			cfg.SetupState.K8sInstalled = false
			assert.False(t, isDependencyInstalled("gateway-api", cfg))
		})
	}
}

func TestGatewayDependencyWithoutKubeconfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FOUNDRY_CONFIG_DIR", dir)
	cfg := &config.Config{SetupState: &setup.SetupState{K8sInstalled: true}}
	assert.False(t, isDependencyInstalled("gateway-api", cfg))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kubeconfig"), []byte("invalid"), 0600))
	assert.False(t, isDependencyInstalled("gateway-api", cfg))
}
