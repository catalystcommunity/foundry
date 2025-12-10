package prometheus

import (
	"context"
	"strings"
	"testing"
)

// mockK8sApplyClient is a mock implementation of K8sApplyClient for testing
type mockK8sApplyClient struct {
	appliedManifests []string
	shouldFail       bool
}

func (m *mockK8sApplyClient) ApplyManifest(ctx context.Context, manifest string) error {
	if m.shouldFail {
		return context.DeadlineExceeded
	}
	m.appliedManifests = append(m.appliedManifests, manifest)
	return nil
}

func TestGetServiceMonitorManifest(t *testing.T) {
	tests := []struct {
		name     string
		cfg      ServiceMonitorConfig
		contains []string
	}{
		{
			name: "basic config with port name",
			cfg: ServiceMonitorConfig{
				Name:      "test-monitor",
				Namespace: "monitoring",
				Selector:  map[string]string{"app": "test"},
				Port:      "metrics",
			},
			contains: []string{
				"name: test-monitor",
				"namespace: monitoring",
				"app: test",
				"port: metrics",
				"path: /metrics",
				"interval: 30s",
			},
		},
		{
			name: "config with target port",
			cfg: ServiceMonitorConfig{
				Name:       "test-monitor-port",
				Namespace:  "monitoring",
				Selector:   map[string]string{"app": "test"},
				TargetPort: 9090,
				Path:       "/custom/metrics",
				Interval:   "60s",
			},
			contains: []string{
				"name: test-monitor-port",
				"targetPort: 9090",
				"path: /custom/metrics",
				"interval: 60s",
			},
		},
		{
			name: "config with scrape timeout",
			cfg: ServiceMonitorConfig{
				Name:          "test-with-timeout",
				Namespace:     "monitoring",
				Selector:      map[string]string{"app": "slow"},
				Port:          "metrics",
				ScrapeTimeout: "15s",
			},
			contains: []string{
				"scrapeTimeout: 15s",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := GetServiceMonitorManifest(tt.cfg)
			for _, want := range tt.contains {
				if !strings.Contains(manifest, want) {
					t.Errorf("manifest missing expected content %q\nGot:\n%s", want, manifest)
				}
			}
		})
	}
}

func TestCoreServiceMonitors(t *testing.T) {
	monitors := CoreServiceMonitors()

	if len(monitors) == 0 {
		t.Error("CoreServiceMonitors returned empty list")
	}

	// Verify all monitors have required fields
	for _, m := range monitors {
		if m.Name == "" {
			t.Error("ServiceMonitor missing name")
		}
		if m.Namespace == "" {
			t.Error("ServiceMonitor missing namespace")
		}
		if len(m.Selector) == 0 {
			t.Errorf("ServiceMonitor %s missing selector", m.Name)
		}
		if m.Port == "" && m.TargetPort == 0 {
			t.Errorf("ServiceMonitor %s missing port or targetPort", m.Name)
		}
	}

	// Check for expected monitors
	expectedNames := []string{
		"contour-envoy",
		"contour-controller",
		"seaweedfs-master",
		"seaweedfs-volume",
		"seaweedfs-filer",
		"loki",
		"longhorn-manager",
		"cert-manager",
		"external-dns",
	}

	names := GetServiceMonitorNames()
	for _, expected := range expectedNames {
		found := false
		for _, name := range names {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected ServiceMonitor: %s", expected)
		}
	}
}

func TestInstallCoreServiceMonitors(t *testing.T) {
	ctx := context.Background()

	t.Run("successful installation", func(t *testing.T) {
		mockClient := &mockK8sApplyClient{}
		err := InstallCoreServiceMonitors(ctx, mockClient)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Should have applied manifests for all monitors
		expectedCount := len(CoreServiceMonitors())
		if len(mockClient.appliedManifests) != expectedCount {
			t.Errorf("expected %d manifests, got %d", expectedCount, len(mockClient.appliedManifests))
		}
	})

	t.Run("nil client", func(t *testing.T) {
		err := InstallCoreServiceMonitors(ctx, nil)
		if err == nil {
			t.Error("expected error for nil client")
		}
	})

	t.Run("continues on individual failures", func(t *testing.T) {
		mockClient := &mockK8sApplyClient{shouldFail: true}
		// Should not return error - just log warnings
		err := InstallCoreServiceMonitors(ctx, mockClient)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestGetServiceMonitorNames(t *testing.T) {
	names := GetServiceMonitorNames()

	if len(names) == 0 {
		t.Error("GetServiceMonitorNames returned empty list")
	}

	// All names should be non-empty
	for _, name := range names {
		if name == "" {
			t.Error("GetServiceMonitorNames returned empty name")
		}
	}
}
