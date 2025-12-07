package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/component/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	testAPIKey = "test-api-key-12345"
)

// TestPowerDNSIntegration tests the complete PowerDNS lifecycle with a real container
func TestPowerDNSIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start PowerDNS container
	container, apiURL, err := startPowerDNSContainer(ctx, t)
	require.NoError(t, err, "Failed to start PowerDNS container")
	t.Logf("PowerDNS container started at %s", apiURL)

	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Create client
	client := dns.NewClient(apiURL, testAPIKey)

	t.Run("Zone_Creation_Infrastructure", func(t *testing.T) {
		// Create infrastructure zone
		zoneConfig := dns.ZoneConfig{
			Name:        "infratest.com",
			Type:        dns.ZoneTypeNative,
			IsPublic:    false,
			Nameservers: []string{"ns1.infratest.com"},
		}

		err := dns.CreateInfrastructureZone(client, zoneConfig)
		require.NoError(t, err, "Should create infrastructure zone")

		// Verify zone exists
		zones, err := client.ListZones()
		require.NoError(t, err, "Should list zones")
		require.NotEmpty(t, zones, "Should have at least one zone")

		// Find our zone
		found := false
		for _, z := range zones {
			if z.Name == "infratest.com." {
				found = true
				assert.Equal(t, "Native", z.Kind, "Zone kind should be Native")
				break
			}
		}
		assert.True(t, found, "Infrastructure zone should exist")

		// Verify SOA and NS records were created
		records, err := client.ListRecords("infratest.com.")
		require.NoError(t, err, "Should list records")
		require.NotEmpty(t, records, "Should have records")

		// Check for SOA record
		hasSOA := false
		hasNS := false
		for _, rs := range records {
			if rs.Type == "SOA" {
				hasSOA = true
			}
			if rs.Type == "NS" {
				hasNS = true
			}
		}
		assert.True(t, hasSOA, "Should have SOA record")
		assert.True(t, hasNS, "Should have NS record")
	})

	t.Run("Zone_Creation_Kubernetes", func(t *testing.T) {
		// Create kubernetes zone
		zoneConfig := dns.ZoneConfig{
			Name:        "k8stest.com",
			Type:        dns.ZoneTypeNative,
			IsPublic:    false,
			Nameservers: []string{"ns1.k8stest.com"},
		}

		err := dns.CreateKubernetesZone(client, zoneConfig)
		require.NoError(t, err, "Should create kubernetes zone")

		// Verify zone exists
		zones, err := client.ListZones()
		require.NoError(t, err, "Should list zones")

		found := false
		for _, z := range zones {
			if z.Name == "k8stest.com." {
				found = true
				assert.Equal(t, "Native", z.Kind, "Zone kind should be Native")
				break
			}
		}
		assert.True(t, found, "Kubernetes zone should exist")
	})

	t.Run("Record_Management_A_Records", func(t *testing.T) {
		// Add A record for OpenBAO
		err := dns.AddARecord(client, "infratest.com", "openbao", "192.168.1.10")
		require.NoError(t, err, "Should add openbao A record")

		// Add A record for DNS
		err = dns.AddARecord(client, "infratest.com", "dns", "192.168.1.10")
		require.NoError(t, err, "Should add dns A record")

		// Add A record for Zot
		err = dns.AddARecord(client, "infratest.com", "zot", "192.168.1.10")
		require.NoError(t, err, "Should add zot A record")

		// Verify records exist
		records, err := client.ListRecords("infratest.com.")
		require.NoError(t, err, "Should list records")

		// Check for our A records
		foundOpenbao := false
		foundDNS := false
		foundZot := false

		for _, rs := range records {
			if rs.Type == "A" {
				for _, r := range rs.Records {
					switch r.Name {
					case "openbao.infratest.com.":
						foundOpenbao = true
						assert.Equal(t, "192.168.1.10", r.Content, "OpenBAO IP should match")
					case "dns.infratest.com.":
						foundDNS = true
						assert.Equal(t, "192.168.1.10", r.Content, "DNS IP should match")
					case "zot.infratest.com.":
						foundZot = true
						assert.Equal(t, "192.168.1.10", r.Content, "Zot IP should match")
					}
				}
			}
		}

		assert.True(t, foundOpenbao, "Should have openbao A record")
		assert.True(t, foundDNS, "Should have dns A record")
		assert.True(t, foundZot, "Should have zot A record")
	})

	t.Run("Record_Management_CNAME", func(t *testing.T) {
		// Add CNAME record
		err := dns.AddCNAMERecord(client, "infratest.com", "www", "openbao.infratest.com")
		require.NoError(t, err, "Should add CNAME record")

		// Verify CNAME exists
		records, err := client.ListRecords("infratest.com.")
		require.NoError(t, err, "Should list records")

		foundCNAME := false
		for _, rs := range records {
			if rs.Type == "CNAME" {
				for _, r := range rs.Records {
					if r.Name == "www.infratest.com." {
						foundCNAME = true
						assert.Equal(t, "openbao.infratest.com.", r.Content, "CNAME target should match")
					}
				}
			}
		}
		assert.True(t, foundCNAME, "Should have CNAME record")
	})

	t.Run("Record_Management_Wildcard", func(t *testing.T) {
		// Add wildcard A record to kubernetes zone
		err := dns.AddWildcardRecord(client, "k8stest.com", "192.168.1.100")
		require.NoError(t, err, "Should add wildcard record")

		// Verify wildcard exists
		records, err := client.ListRecords("k8stest.com.")
		require.NoError(t, err, "Should list records")

		foundWildcard := false
		for _, rs := range records {
			if rs.Type == "A" {
				for _, r := range rs.Records {
					if r.Name == "*.k8stest.com." {
						foundWildcard = true
						assert.Equal(t, "192.168.1.100", r.Content, "Wildcard IP should match")
					}
				}
			}
		}
		assert.True(t, foundWildcard, "Should have wildcard A record")
	})

	t.Run("Infrastructure_DNS_Initialization", func(t *testing.T) {
		// Create a new zone for this test
		zoneConfig := dns.ZoneConfig{
			Name:        "infra.example.com",
			Type:        dns.ZoneTypeNative,
			IsPublic:    false,
			Nameservers: []string{},
		}
		err := dns.CreateInfrastructureZone(client, zoneConfig)
		require.NoError(t, err, "Should create infrastructure zone")

		// Initialize infrastructure DNS
		infraConfig := dns.InfrastructureRecordConfig{
			Zone:        "infra.example.com",
			OpenBAOIP:   "192.168.1.10",
			DNSIP:       "192.168.1.10",
			ZotIP:       "192.168.1.10",
			K8sVIP:      "192.168.1.100",
			IsPublic:    false,
			PublicCNAME: "",
		}

		err = dns.InitializeInfrastructureDNS(client, infraConfig)
		require.NoError(t, err, "Should initialize infrastructure DNS")

		// Verify all infrastructure records exist
		records, err := client.ListRecords("infra.example.com.")
		require.NoError(t, err, "Should list records")

		foundRecords := make(map[string]bool)
		for _, rs := range records {
			if rs.Type == "A" {
				for _, r := range rs.Records {
					switch r.Name {
					case "openbao.infra.example.com.":
						foundRecords["openbao"] = true
						assert.Equal(t, "192.168.1.10", r.Content)
					case "dns.infra.example.com.":
						foundRecords["dns"] = true
						assert.Equal(t, "192.168.1.10", r.Content)
					case "zot.infra.example.com.":
						foundRecords["zot"] = true
						assert.Equal(t, "192.168.1.10", r.Content)
					case "k8s.infra.example.com.":
						foundRecords["k8s"] = true
						assert.Equal(t, "192.168.1.100", r.Content)
					}
				}
			}
		}

		assert.True(t, foundRecords["openbao"], "Should have openbao record")
		assert.True(t, foundRecords["dns"], "Should have dns record")
		assert.True(t, foundRecords["zot"], "Should have zot record")
		assert.True(t, foundRecords["k8s"], "Should have k8s record")
	})

	t.Run("Kubernetes_DNS_Initialization", func(t *testing.T) {
		// Create a new zone for this test
		zoneConfig := dns.ZoneConfig{
			Name:        "k8s.example.com",
			Type:        dns.ZoneTypeNative,
			IsPublic:    false,
			Nameservers: []string{},
		}
		err := dns.CreateKubernetesZone(client, zoneConfig)
		require.NoError(t, err, "Should create kubernetes zone")

		// Initialize kubernetes DNS
		k8sConfig := dns.KubernetesZoneConfig{
			Zone:        "k8s.example.com",
			K8sVIP:      "192.168.1.100",
			IsPublic:    false,
			PublicCNAME: "",
		}

		err = dns.InitializeKubernetesDNS(client, k8sConfig)
		require.NoError(t, err, "Should initialize kubernetes DNS")

		// Verify wildcard record exists
		records, err := client.ListRecords("k8s.example.com.")
		require.NoError(t, err, "Should list records")

		foundWildcard := false
		for _, rs := range records {
			if rs.Type == "A" {
				for _, r := range rs.Records {
					if r.Name == "*.k8s.example.com." {
						foundWildcard = true
						assert.Equal(t, "192.168.1.100", r.Content, "Wildcard IP should match K8s VIP")
					}
				}
			}
		}
		assert.True(t, foundWildcard, "Should have wildcard record for kubernetes zone")
	})

	t.Run("Record_Deletion", func(t *testing.T) {
		// Add a test record
		err := dns.AddARecord(client, "infratest.com", "test-delete", "192.168.1.99")
		require.NoError(t, err, "Should add test record")

		// Verify it exists
		records, err := client.ListRecords("infratest.com.")
		require.NoError(t, err, "Should list records")

		found := false
		for _, rs := range records {
			if rs.Type == "A" {
				for _, r := range rs.Records {
					if r.Name == "test-delete.infratest.com." {
						found = true
					}
				}
			}
		}
		assert.True(t, found, "Test record should exist before deletion")

		// Delete the record
		err = client.DeleteRecord("infratest.com.", "test-delete.infratest.com.", "A")
		require.NoError(t, err, "Should delete record")

		// Verify it's gone
		records, err = client.ListRecords("infratest.com.")
		require.NoError(t, err, "Should list records")

		found = false
		for _, rs := range records {
			if rs.Type == "A" {
				for _, r := range rs.Records {
					if r.Name == "test-delete.infratest.com." {
						found = true
					}
				}
			}
		}
		assert.False(t, found, "Test record should be deleted")
	})

	t.Run("Zone_Deletion", func(t *testing.T) {
		// Create a zone to delete
		err := client.CreateZone("delete-test.com.", "Native")
		require.NoError(t, err, "Should create zone for deletion test")

		// Verify it exists
		zones, err := client.ListZones()
		require.NoError(t, err, "Should list zones")

		found := false
		for _, z := range zones {
			if z.Name == "delete-test.com." {
				found = true
			}
		}
		assert.True(t, found, "Zone should exist before deletion")

		// Delete the zone
		err = client.DeleteZone("delete-test.com.")
		require.NoError(t, err, "Should delete zone")

		// Verify it's gone
		zones, err = client.ListZones()
		require.NoError(t, err, "Should list zones")

		found = false
		for _, z := range zones {
			if z.Name == "delete-test.com." {
				found = true
			}
		}
		assert.False(t, found, "Zone should be deleted")
	})
}

// startPowerDNSContainer starts a PowerDNS container and returns the container, API URL, and error
func startPowerDNSContainer(ctx context.Context, t *testing.T) (testcontainers.Container, string, error) {
	// PowerDNS Authoritative Server with API enabled
	req := testcontainers.ContainerRequest{
		Image:        "powerdns/pdns-auth-master:latest",
		ExposedPorts: []string{"8081/tcp"}, // PowerDNS API port
		Env: map[string]string{
			"PDNS_AUTH_API_KEY":    testAPIKey,
			"PDNS_AUTH_WEBSERVER":  "yes",
			"PDNS_AUTH_API":        "yes",
			"PDNS_AUTH_WEBSERVER_ADDRESS": "0.0.0.0",
			"PDNS_AUTH_WEBSERVER_PORT":    "8081",
			"PDNS_AUTH_WEBSERVER_ALLOW_FROM": "0.0.0.0/0",
		},
		WaitingFor: wait.ForHTTP("/api/v1/servers/localhost").
			WithPort("8081/tcp").
			WithHeaders(map[string]string{
				"X-API-Key": testAPIKey,
			}).
			WithStatusCodeMatcher(func(status int) bool {
				// API should return 200 when ready and authenticated
				return status == 200
			}).
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to start container: %w", err)
	}

	// Get the mapped port
	mappedPort, err := container.MappedPort(ctx, "8081")
	if err != nil {
		container.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get mapped port: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get host: %w", err)
	}

	apiURL := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	t.Logf("PowerDNS API available at %s", apiURL)

	// Give PowerDNS a moment to fully initialize
	time.Sleep(2 * time.Second)

	return container, apiURL, nil
}
