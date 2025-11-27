package k3s

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateRegistriesYAML(t *testing.T) {
	tests := []struct {
		name     string
		zotURL   string
		insecure bool
		want     []string // strings that should be present in output
	}{
		{
			name:     "secure registry",
			zotURL:   "http://zot.infraexample.com:5000",
			insecure: false,
			want: []string{
				"mirrors:",
				"docker.io:",
				"ghcr.io:",
				"endpoint:",
				"- \"http://zot.infraexample.com:5000\"",
			},
		},
		{
			name:     "insecure registry",
			zotURL:   "http://zot.infraexample.com:5000",
			insecure: true,
			want: []string{
				"mirrors:",
				"docker.io:",
				"ghcr.io:",
				"endpoint:",
				"- \"http://zot.infraexample.com:5000\"",
				"configs:",
				"tls:",
				"insecure_skip_verify: true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateRegistriesYAML(tt.zotURL, tt.insecure)

			for _, want := range tt.want {
				assert.Contains(t, got, want, "expected config to contain: %s", want)
			}
		})
	}
}

func TestGenerateRegistriesConfig(t *testing.T) {
	tests := []struct {
		name    string
		zotAddr string
		want    []string // strings that should be present in output
	}{
		{
			name:    "simple address",
			zotAddr: "zot.infraexample.com",
			want: []string{
				"mirrors:",
				"docker.io:",
				"ghcr.io:",
				"endpoint:",
				"- \"http://zot.infraexample.com:5000\"",
				"configs:",
				"\"http://zot.infraexample.com:5000\":",
				"tls:",
				"insecure_skip_verify: true",
			},
		},
		{
			name:    "IP address",
			zotAddr: "192.168.1.50",
			want: []string{
				"mirrors:",
				"docker.io:",
				"ghcr.io:",
				"endpoint:",
				"- \"http://192.168.1.50:5000\"",
				"configs:",
				"\"http://192.168.1.50:5000\":",
				"tls:",
				"insecure_skip_verify: true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateRegistriesConfig(tt.zotAddr)

			// Verify all expected strings are present
			for _, want := range tt.want {
				assert.Contains(t, got, want, "expected config to contain: %s", want)
			}

			// Verify it's valid YAML-like structure
			assert.True(t, strings.Contains(got, "mirrors:"), "should have mirrors section")
			assert.True(t, strings.Contains(got, "configs:"), "should have configs section")
			assert.True(t, strings.Contains(got, "insecure_skip_verify: true"), "should be insecure")
		})
	}
}

func TestGenerateK3sServerFlags(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want []string // flags that should be present
	}{
		{
			name: "first control plane node",
			cfg: &Config{
				VIP:         "192.168.1.100",
				ClusterInit: true,
				ClusterToken: "cluster-token-123",
				AgentToken:   "agent-token-456",
				DisableComponents: []string{"traefik", "servicelb"},
			},
			want: []string{
				"--cluster-init",
				"--token cluster-token-123",
				"--agent-token agent-token-456",
				"--tls-san 192.168.1.100",
				"--disable=traefik",
				"--disable=servicelb",
			},
		},
		{
			name: "joining control plane node",
			cfg: &Config{
				VIP:          "192.168.1.100",
				ServerURL:    "https://192.168.1.100:6443",
				ClusterToken: "cluster-token-123",
				TLSSANs:      []string{"api.example.com"},
				DisableComponents: []string{"traefik"},
			},
			want: []string{
				"--server https://192.168.1.100:6443",
				"--token cluster-token-123",
				"--tls-san api.example.com",
				"--tls-san 192.168.1.100",
				"--disable=traefik",
			},
		},
		{
			name: "minimal config",
			cfg: &Config{
				VIP: "192.168.1.100",
			},
			want: []string{
				"--tls-san 192.168.1.100",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateK3sServerFlags(tt.cfg)
			gotStr := strings.Join(got, " ")

			for _, want := range tt.want {
				assert.Contains(t, gotStr, want, "expected flags to contain: %s", want)
			}
		})
	}
}

func TestGenerateK3sInstallCommand(t *testing.T) {
	cfg := &Config{
		VIP:          "192.168.1.100",
		ClusterInit:  true,
		ClusterToken: "test-token",
		DisableComponents: []string{"traefik"},
	}

	got := GenerateK3sInstallCommand(cfg)

	// Check base command
	assert.Contains(t, got, "curl -sfL https://get.k3s.io | sh -s - server")

	// Check flags are included
	assert.Contains(t, got, "--cluster-init")
	assert.Contains(t, got, "--token test-token")
	assert.Contains(t, got, "--tls-san 192.168.1.100")
	assert.Contains(t, got, "--disable=traefik")
}

func TestGenerateResolvConfContent(t *testing.T) {
	tests := []struct {
		name          string
		dnsServers    []string
		searchDomains []string
		want          []string
	}{
		{
			name:       "single DNS server",
			dnsServers: []string{"192.168.1.10"},
			want: []string{
				"nameserver 192.168.1.10",
			},
		},
		{
			name:       "multiple DNS servers",
			dnsServers: []string{"192.168.1.10", "8.8.8.8"},
			want: []string{
				"nameserver 192.168.1.10",
				"nameserver 8.8.8.8",
			},
		},
		{
			name:          "with search domains",
			dnsServers:    []string{"192.168.1.10"},
			searchDomains: []string{"example.com", "cluster.local"},
			want: []string{
				"search example.com cluster.local",
				"nameserver 192.168.1.10",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateResolvConfContent(tt.dnsServers, tt.searchDomains)

			for _, want := range tt.want {
				assert.Contains(t, got, want)
			}

			// Should end with newline
			assert.True(t, strings.HasSuffix(got, "\n"))
		})
	}
}

func TestGenerateSystemdResolvdConfig(t *testing.T) {
	tests := []struct {
		name          string
		dnsServers    []string
		searchDomains []string
		want          []string
	}{
		{
			name:       "single DNS server",
			dnsServers: []string{"192.168.1.10"},
			want: []string{
				"[Resolve]",
				"DNS=192.168.1.10",
			},
		},
		{
			name:       "multiple DNS servers",
			dnsServers: []string{"192.168.1.10", "8.8.8.8"},
			want: []string{
				"[Resolve]",
				"DNS=192.168.1.10 8.8.8.8",
			},
		},
		{
			name:          "with search domains",
			dnsServers:    []string{"192.168.1.10"},
			searchDomains: []string{"example.com", "cluster.local"},
			want: []string{
				"[Resolve]",
				"DNS=192.168.1.10",
				"Domains=example.com cluster.local",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateSystemdResolvdConfig(tt.dnsServers, tt.searchDomains)

			for _, want := range tt.want {
				assert.Contains(t, got, want)
			}

			// Should end with newline
			assert.True(t, strings.HasSuffix(got, "\n"))
		})
	}
}
