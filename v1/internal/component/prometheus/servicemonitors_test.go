package prometheus

import (
	"strings"
	"testing"
)

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
