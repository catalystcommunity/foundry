package webui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/discovery"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/topology"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerRequiresAuthenticationAndSameOrigin(t *testing.T) {
	server, token, _ := newTestServer(t, nil)

	response := performRequest(server.Handler(), http.MethodGet, "/api/v1/config", nil, "", "")
	assert.Equal(t, http.StatusUnauthorized, response.Code)

	response = performRequest(server.Handler(), http.MethodPost, "/api/v1/session", map[string]string{"token": token}, "", "")
	assert.Equal(t, http.StatusForbidden, response.Code)

	response = performRequest(server.Handler(), http.MethodPost, "/api/v1/session", map[string]string{"token": token}, "", "http://foundry.test")
	require.Equal(t, http.StatusOK, response.Code)
	cookies := response.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.True(t, cookies[0].HttpOnly)
	assert.Equal(t, http.SameSiteStrictMode, cookies[0].SameSite)

	request := httptest.NewRequest(http.MethodPost, "http://foundry.test/api/v1/plan", bytes.NewBufferString("{}"))
	request.AddCookie(cookies[0])
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusForbidden, response.Code)
}

func TestPlanDoesNotSaveAndApplyDoes(t *testing.T) {
	applied := make(chan string, 1)
	server, token, configPath := newTestServer(t, func(_ context.Context, path string) error {
		applied <- path
		return nil
	})
	before, err := os.ReadFile(configPath)
	require.NoError(t, err)
	wizard := validWizard()

	response := performRequest(server.Handler(), http.MethodPost, "/api/v1/plan", wizard, token, "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var plan Plan
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &plan))
	assert.True(t, plan.Valid)
	assert.Contains(t, nodeIDs(plan.Topology.Nodes), "vip")
	afterPlan, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, before, afterPlan)

	response = performRequest(server.Handler(), http.MethodPost, "/api/v1/apply", map[string]any{"config": wizard, "confirm": false}, token, "")
	assert.Equal(t, http.StatusBadRequest, response.Code)

	response = performRequest(server.Handler(), http.MethodPost, "/api/v1/apply", map[string]any{"config": wizard, "confirm": true}, token, "")
	require.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
	select {
	case path := <-applied:
		assert.Equal(t, configPath, path)
	case <-time.After(2 * time.Second):
		t.Fatal("apply callback was not called")
	}
	saved, err := config.Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, saved.Management)
	assert.Equal(t, "node-1", saved.Management.Host)
	assert.True(t, saved.Hosts[0].HasRole(host.RoleManagement))
	assert.Equal(t, "preserved", saved.Components["k3s"].Config["custom"])
}

func TestInvalidWizardIsRejectedWithoutChangingConfig(t *testing.T) {
	server, token, configPath := newTestServer(t, nil)
	before, err := os.ReadFile(configPath)
	require.NoError(t, err)
	wizard := validWizard()
	wizard.Hosts[0].Address = "not an address"

	response := performRequest(server.Handler(), http.MethodPost, "/api/v1/plan", wizard, token, "")
	assert.Equal(t, http.StatusBadRequest, response.Code)
	after, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestOnlyOneApplyCanRunAtATime(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server, token, _ := newTestServer(t, func(_ context.Context, _ string) error {
		close(started)
		<-release
		return nil
	})
	first := performRequest(server.Handler(), http.MethodPost, "/api/v1/apply/current", nil, token, "")
	require.Equal(t, http.StatusAccepted, first.Code)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first apply did not start")
	}
	second := performRequest(server.Handler(), http.MethodPost, "/api/v1/apply/current", nil, token, "")
	assert.Equal(t, http.StatusConflict, second.Code)
	close(release)
}

func TestStaticAssetsHaveSecurityHeaders(t *testing.T) {
	server, _, _ := newTestServer(t, nil)
	response := performRequest(server.Handler(), http.MethodGet, "/", nil, "", "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Header().Get("Content-Security-Policy"), "default-src 'self'")
	assert.Equal(t, "DENY", response.Header().Get("X-Frame-Options"))
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
}

func TestRuntimeAndOverviewAreAuthenticated(t *testing.T) {
	server, token, _ := newTestServer(t, nil)
	server.mode = "external"
	server.inspect = func(_ context.Context, _ *config.Config) discovery.Snapshot {
		return discovery.Snapshot{ClusterAvailable: true, Services: []discovery.ServiceLink{{Name: "Grafana", URL: "https://grafana.example.test"}}}
	}

	response := performRequest(server.Handler(), http.MethodGet, "/api/v1/runtime", nil, token, "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"mode":"external"`)

	response = performRequest(server.Handler(), http.MethodGet, "/api/v1/overview", nil, token, "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "https://grafana.example.test")

	response = performRequest(server.Handler(), http.MethodGet, "/api/v1/overview", nil, "", "")
	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func newTestServer(t *testing.T, apply ApplyFunc) (*Server, string, string) {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "stack.yaml")
	cfg := &config.Config{
		Network:    &config.NetworkConfig{Gateway: "192.0.2.1", Netmask: "255.255.255.0"},
		Cluster:    config.ClusterConfig{Name: "old", PrimaryDomain: "old.example", VIP: "192.0.2.10"},
		Hosts:      []*host.Host{{Hostname: "node-1", Address: "192.0.2.11", Port: 22, User: "root", Roles: []string{host.RoleClusterControlPlane}}},
		Components: config.ComponentMap{"k3s": {Config: map[string]any{"custom": "preserved"}}},
	}
	require.NoError(t, config.Save(cfg, configPath))
	auth := NewAuthStore()
	token, err := auth.NewToken("test", time.Hour)
	require.NoError(t, err)
	server, err := New(Options{ConfigPath: configPath, Auth: auth, Apply: apply})
	require.NoError(t, err)
	return server, token, configPath
}

func validWizard() WizardConfig {
	return WizardConfig{
		ClusterName: "test", PrimaryDomain: "example.test", VIP: "192.0.2.10",
		Gateway: "192.0.2.1", Netmask: "255.255.255.0",
		ManagementHost: "node-1", ManagementPort: 9080, ManagementImage: "example.invalid/foundry",
		Hosts: []WizardHost{{Hostname: "node-1", Address: "192.0.2.11", Port: 22, User: "root", Roles: []string{host.RoleClusterControlPlane}}},
	}
}

func performRequest(handler http.Handler, method, path string, body any, bearer, origin string) *httptest.ResponseRecorder {
	var payload bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&payload).Encode(body)
	}
	request := httptest.NewRequest(method, "http://foundry.test"+path, &payload)
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func nodeIDs(nodes []topology.Node) []string {
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return ids
}
