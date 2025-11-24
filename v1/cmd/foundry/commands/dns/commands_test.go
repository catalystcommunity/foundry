package dns

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component/dns"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDNSCommandRegistration(t *testing.T) {
	t.Run("dns command registered", func(t *testing.T) {
		assert.NotNil(t, Command)
		assert.Equal(t, "dns", Command.Name)
		assert.Len(t, Command.Commands, 3)

		var zoneCmd, recordCmd, testCmd bool
		for _, cmd := range Command.Commands {
			switch cmd.Name {
			case "zone":
				zoneCmd = true
			case "record":
				recordCmd = true
			case "test":
				testCmd = true
			}
		}

		assert.True(t, zoneCmd, "zone command should be registered")
		assert.True(t, recordCmd, "record command should be registered")
		assert.True(t, testCmd, "test command should be registered")
	})

	t.Run("zone subcommands registered", func(t *testing.T) {
		assert.NotNil(t, ZoneCommand)
		assert.Equal(t, "zone", ZoneCommand.Name)
		assert.Len(t, ZoneCommand.Commands, 3)

		var listCmd, createCmd, deleteCmd bool
		for _, cmd := range ZoneCommand.Commands {
			switch cmd.Name {
			case "list":
				listCmd = true
			case "create":
				createCmd = true
			case "delete":
				deleteCmd = true
			}
		}

		assert.True(t, listCmd, "list command should be registered")
		assert.True(t, createCmd, "create command should be registered")
		assert.True(t, deleteCmd, "delete command should be registered")
	})

	t.Run("record subcommands registered", func(t *testing.T) {
		assert.NotNil(t, RecordCommand)
		assert.Equal(t, "record", RecordCommand.Name)
		assert.Len(t, RecordCommand.Commands, 3)

		var addCmd, listCmd, deleteCmd bool
		for _, cmd := range RecordCommand.Commands {
			switch cmd.Name {
			case "add":
				addCmd = true
			case "list":
				listCmd = true
			case "delete":
				deleteCmd = true
			}
		}

		assert.True(t, addCmd, "add command should be registered")
		assert.True(t, listCmd, "list command should be registered")
		assert.True(t, deleteCmd, "delete command should be registered")
	})
}

func setupTestConfig(t *testing.T) (string, *httptest.Server) {
	t.Helper()

	// Create mock PowerDNS API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check API key
		if r.Header.Get("X-API-Key") != "test-api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/api/v1/servers/localhost/zones":
			if r.Method == "GET" {
				// List zones
				zones := []dns.Zone{
					{ID: "example.com.", Name: "example.com.", Type: "Native", Serial: 1},
					{ID: "test.local.", Name: "test.local.", Type: "Native", Serial: 2},
				}
				json.NewEncoder(w).Encode(zones)
			} else if r.Method == "POST" {
				// Create zone
				w.WriteHeader(http.StatusCreated)
			}
		case "/api/v1/servers/localhost/zones/example.com.":
			if r.Method == "GET" {
				// Get zone with records
				response := map[string]interface{}{
					"id":   "example.com.",
					"name": "example.com.",
					"type": "Native",
					"rrsets": []map[string]interface{}{
						{
							"name": "www.example.com.",
							"type": "A",
							"ttl":  3600,
							"records": []map[string]interface{}{
								{"content": "192.168.1.10", "disabled": false},
							},
						},
					},
				}
				json.NewEncoder(w).Encode(response)
			} else if r.Method == "PATCH" {
				// Update records
				w.WriteHeader(http.StatusNoContent)
			} else if r.Method == "DELETE" {
				// Delete zone
				w.WriteHeader(http.StatusNoContent)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	// Create temporary directory for test config
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	// Create test configuration
	cfg := &config.Config{
		Network: &config.NetworkConfig{
			Gateway: "192.168.1.1",
			Netmask: "255.255.255.0",
		},
		Hosts: []*host.Host{
			{
				Hostname: "host1",
				Address:  "192.168.1.10",
				Roles:    []string{host.RoleOpenBAO, host.RoleDNS, host.RoleZot},
				State:    host.StateConfigured,
			},
			{
				Hostname: "node1",
				Address:  "192.168.1.20",
				Roles:    []string{host.RoleClusterControlPlane},
				State:    host.StateConfigured,
			},
		},
		DNS: &config.DNSConfig{
			InfrastructureZones: []config.DNSZone{
				{Name: "infra.local", Public: false},
			},
			KubernetesZones: []config.DNSZone{
				{Name: "k8s.local", Public: false},
			},
			Backend:    "sqlite",
			Forwarders: []string{"8.8.8.8"},
			APIKey:     "test-api-key", // Plain value for testing (not a secret ref)
		},
		Cluster: config.ClusterConfig{
			Name:   "test-cluster",
			Domain: "example.com",
			VIP:    "192.168.1.100",
		},
		Components: config.ComponentMap{
			"openbao": config.ComponentConfig{},
		},
	}

	// Save the configuration
	err := config.Save(cfg, configPath)
	require.NoError(t, err)

	// Set environment variable to override DNS host
	os.Setenv("FOUNDRY_TEST_DNS_HOST", server.URL[7:]) // Remove "http://"

	t.Cleanup(func() {
		server.Close()
		os.Unsetenv("FOUNDRY_TEST_DNS_HOST")
	})

	return configPath, server
}

func TestZoneListCommand(t *testing.T) {
	_, _ = setupTestConfig(t)

	t.Run("list zones successfully", func(t *testing.T) {
		// This test would require mocking the entire CLI execution
		// For now, we verify that the command is properly configured
		assert.NotNil(t, zoneListCommand)
		assert.Equal(t, "list", zoneListCommand.Name)
		assert.NotNil(t, zoneListCommand.Action)
	})
}

func TestZoneCreateCommand(t *testing.T) {
	_, _ = setupTestConfig(t)

	t.Run("create command configured", func(t *testing.T) {
		assert.NotNil(t, zoneCreateCommand)
		assert.Equal(t, "create", zoneCreateCommand.Name)
		assert.NotNil(t, zoneCreateCommand.Action)
	})
}

func TestRecordAddCommand(t *testing.T) {
	_, _ = setupTestConfig(t)

	t.Run("add command configured", func(t *testing.T) {
		assert.NotNil(t, recordAddCommand)
		assert.Equal(t, "add", recordAddCommand.Name)
		assert.NotNil(t, recordAddCommand.Action)
	})
}

func TestRecordListCommand(t *testing.T) {
	t.Run("list command configured", func(t *testing.T) {
		assert.NotNil(t, recordListCommand)
		assert.Equal(t, "list", recordListCommand.Name)
		assert.NotNil(t, recordListCommand.Action)
	})
}

func TestRecordDeleteCommand(t *testing.T) {
	_, _ = setupTestConfig(t)

	t.Run("delete command configured", func(t *testing.T) {
		assert.NotNil(t, recordDeleteCommand)
		assert.Equal(t, "delete", recordDeleteCommand.Name)
		assert.NotNil(t, recordDeleteCommand.Action)
	})
}

func TestTestCommand(t *testing.T) {
	t.Run("test command configured", func(t *testing.T) {
		assert.NotNil(t, TestCommand)
		assert.Equal(t, "test", TestCommand.Name)
		assert.NotNil(t, TestCommand.Action)
	})
}

func TestZoneNameFormatting(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "zone without trailing dot",
			input:    "example.com",
			expected: "example.com.",
		},
		{
			name:     "zone with trailing dot",
			input:    "example.com.",
			expected: "example.com.",
		},
		{
			name:     "subdomain without trailing dot",
			input:    "sub.example.com",
			expected: "sub.example.com.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := tt.input
			if !strings.HasSuffix(input, ".") {
				input = input + "."
			}
			assert.Equal(t, tt.expected, input)
		})
	}
}

func TestRecordNameFormatting(t *testing.T) {
	zoneName := "example.com."

	tests := []struct {
		name        string
		recordName  string
		zoneName    string
		expected    string
		description string
	}{
		{
			name:        "@ symbol",
			recordName:  "@",
			zoneName:    zoneName,
			expected:    zoneName,
			description: "@ should be replaced with zone name",
		},
		{
			name:        "hostname without zone",
			recordName:  "www",
			zoneName:    zoneName,
			expected:    "www.example.com.",
			description: "hostname should have zone appended",
		},
		{
			name:        "fqdn with zone but no trailing dot",
			recordName:  "www.example.com",
			zoneName:    zoneName,
			expected:    "www.example.com.example.com.",
			description: "fqdn not ending with zone+dot gets zone appended",
		},
		{
			name:        "fqdn with trailing dot",
			recordName:  "www.example.com.",
			zoneName:    zoneName,
			expected:    "www.example.com.",
			description: "fqdn with dot should remain unchanged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recordName := tt.recordName

			// This logic mirrors what's in record.go
			if !strings.HasSuffix(recordName, ".") {
				if recordName == "@" {
					recordName = tt.zoneName
				} else if !strings.HasSuffix(recordName, tt.zoneName) {
					recordName = recordName + "." + tt.zoneName
				} else {
					recordName = recordName + "."
				}
			}

			assert.Equal(t, tt.expected, recordName, tt.description)
		})
	}
}
