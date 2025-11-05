package dns

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZoneConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      ZoneConfig
		wantErr     bool
		errContains string
		wantName    string // Expected name after validation (with trailing dot)
	}{
		{
			name: "valid zone without trailing dot",
			config: ZoneConfig{
				Name: "example.com",
				Type: ZoneTypeNative,
			},
			wantErr:  false,
			wantName: "example.com.",
		},
		{
			name: "valid zone with trailing dot",
			config: ZoneConfig{
				Name: "example.com.",
				Type: ZoneTypeNative,
			},
			wantErr:  false,
			wantName: "example.com.",
		},
		{
			name: "valid public zone with CNAME",
			config: ZoneConfig{
				Name:        "example.com",
				Type:        ZoneTypeNative,
				IsPublic:    true,
				PublicCNAME: "home.example.com",
			},
			wantErr:  false,
			wantName: "example.com.",
		},
		{
			name: "empty zone name",
			config: ZoneConfig{
				Type: ZoneTypeNative,
			},
			wantErr:     true,
			errContains: "zone name cannot be empty",
		},
		{
			name: ".local zone cannot be public",
			config: ZoneConfig{
				Name:     "infra.local",
				Type:     ZoneTypeNative,
				IsPublic: true,
			},
			wantErr:     true,
			errContains: ".local zones cannot be public",
		},
		{
			name: ".local zone can be private",
			config: ZoneConfig{
				Name:     "infra.local",
				Type:     ZoneTypeNative,
				IsPublic: false,
			},
			wantErr:  false,
			wantName: "infra.local.",
		},
		{
			name: "public zone without CNAME",
			config: ZoneConfig{
				Name:     "example.com",
				Type:     ZoneTypeNative,
				IsPublic: true,
			},
			wantErr:     true,
			errContains: "public zones must have a public_cname",
		},
		{
			name: "defaults to Native type",
			config: ZoneConfig{
				Name: "example.com",
			},
			wantErr:  false,
			wantName: "example.com.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			if tt.wantName != "" {
				assert.Equal(t, tt.wantName, tt.config.Name)
			}
		})
	}
}

func TestCreateInfrastructureZone(t *testing.T) {
	tests := []struct {
		name           string
		config         ZoneConfig
		wantErr        bool
		errContains    string
		wantZoneName   string
		wantSOA        bool
		wantNSCount    int
		wantNameserver string
	}{
		{
			name: "create basic infrastructure zone",
			config: ZoneConfig{
				Name: "infraexample.com",
				Type: ZoneTypeNative,
			},
			wantErr:        false,
			wantZoneName:   "infraexample.com.",
			wantSOA:        true,
			wantNSCount:    1,
			wantNameserver: "ns1.infraexample.com.",
		},
		{
			name: "create zone with custom nameservers",
			config: ZoneConfig{
				Name:        "infraexample.com",
				Type:        ZoneTypeNative,
				Nameservers: []string{"ns1.example.com", "ns2.example.com"},
			},
			wantErr:      false,
			wantZoneName: "infraexample.com.",
			wantSOA:      true,
			wantNSCount:  2,
		},
		{
			name: "create public zone",
			config: ZoneConfig{
				Name:        "infraexample.com",
				Type:        ZoneTypeNative,
				IsPublic:    true,
				PublicCNAME: "home.example.com",
			},
			wantErr:      false,
			wantZoneName: "infraexample.com.",
			wantSOA:      true,
			wantNSCount:  1,
		},
		{
			name: "invalid config - .local as public",
			config: ZoneConfig{
				Name:     "infra.local",
				Type:     ZoneTypeNative,
				IsPublic: true,
			},
			wantErr:     true,
			errContains: ".local zones cannot be public",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Track requests
			var createdZone *Zone
			var addedRecords []Record

			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "POST" && r.URL.Path == "/api/v1/servers/localhost/zones":
					// Create zone
					var zone Zone
					json.NewDecoder(r.Body).Decode(&zone)
					createdZone = &zone
					w.WriteHeader(http.StatusCreated)
					json.NewEncoder(w).Encode(zone)

				case r.Method == "PATCH" && r.URL.Path == "/api/v1/servers/localhost/zones/"+tt.wantZoneName:
					// Add records
					var rrsets struct {
						RRSets []RRSet `json:"rrsets"`
					}
					json.NewDecoder(r.Body).Decode(&rrsets)
					for _, rrset := range rrsets.RRSets {
						for _, record := range rrset.Records {
							addedRecords = append(addedRecords, Record{
								Name:    rrset.Name,
								Type:    rrset.Type,
								Content: record.Content,
								TTL:     rrset.TTL,
							})
						}
					}
					w.WriteHeader(http.StatusNoContent)

				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")

			err := CreateInfrastructureZone(client, tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, createdZone)
			assert.Equal(t, tt.wantZoneName, createdZone.Name)

			// Check SOA record was added
			if tt.wantSOA {
				var foundSOA bool
				for _, record := range addedRecords {
					if record.Type == "SOA" {
						foundSOA = true
						assert.Contains(t, record.Content, "ns1")
						assert.Contains(t, record.Content, "hostmaster")
						break
					}
				}
				assert.True(t, foundSOA, "SOA record should be added")
			}

			// Check NS records
			var nsCount int
			for _, record := range addedRecords {
				if record.Type == "NS" {
					nsCount++
					if tt.wantNameserver != "" && nsCount == 1 {
						assert.Equal(t, tt.wantNameserver, record.Content)
					}
				}
			}
			assert.Equal(t, tt.wantNSCount, nsCount, "NS record count mismatch")
		})
	}
}

func TestCreateKubernetesZone(t *testing.T) {
	tests := []struct {
		name         string
		config       ZoneConfig
		wantErr      bool
		wantZoneName string
		wantSOA      bool
		wantNSCount  int
	}{
		{
			name: "create basic kubernetes zone",
			config: ZoneConfig{
				Name: "k8sexample.com",
				Type: ZoneTypeNative,
			},
			wantErr:      false,
			wantZoneName: "k8sexample.com.",
			wantSOA:      true,
			wantNSCount:  1,
		},
		{
			name: "create public kubernetes zone",
			config: ZoneConfig{
				Name:        "k8sexample.com",
				Type:        ZoneTypeNative,
				IsPublic:    true,
				PublicCNAME: "home.example.com",
			},
			wantErr:      false,
			wantZoneName: "k8sexample.com.",
			wantSOA:      true,
			wantNSCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Track requests
			var createdZone *Zone
			var addedRecords []Record

			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "POST" && r.URL.Path == "/api/v1/servers/localhost/zones":
					var zone Zone
					json.NewDecoder(r.Body).Decode(&zone)
					createdZone = &zone
					w.WriteHeader(http.StatusCreated)
					json.NewEncoder(w).Encode(zone)

				case r.Method == "PATCH" && r.URL.Path == "/api/v1/servers/localhost/zones/"+tt.wantZoneName:
					var rrsets struct {
						RRSets []RRSet `json:"rrsets"`
					}
					json.NewDecoder(r.Body).Decode(&rrsets)
					for _, rrset := range rrsets.RRSets {
						for _, record := range rrset.Records {
							addedRecords = append(addedRecords, Record{
								Name:    rrset.Name,
								Type:    rrset.Type,
								Content: record.Content,
								TTL:     rrset.TTL,
							})
						}
					}
					w.WriteHeader(http.StatusNoContent)

				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")

			err := CreateKubernetesZone(client, tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, createdZone)
			assert.Equal(t, tt.wantZoneName, createdZone.Name)

			// Check SOA
			if tt.wantSOA {
				var foundSOA bool
				for _, record := range addedRecords {
					if record.Type == "SOA" {
						foundSOA = true
						break
					}
				}
				assert.True(t, foundSOA)
			}

			// Check NS count
			var nsCount int
			for _, record := range addedRecords {
				if record.Type == "NS" {
					nsCount++
				}
			}
			assert.Equal(t, tt.wantNSCount, nsCount)
		})
	}
}

func TestAddSOARecord(t *testing.T) {
	tests := []struct {
		name             string
		zone             string
		wantErr          bool
		wantRecordName   string
		wantContentParts []string
	}{
		{
			name:           "add SOA to zone with trailing dot",
			zone:           "example.com.",
			wantErr:        false,
			wantRecordName: "example.com.",
			wantContentParts: []string{
				"ns1.example.com.",
				"hostmaster.example.com.",
			},
		},
		{
			name:           "add SOA to zone without trailing dot",
			zone:           "example.com",
			wantErr:        false,
			wantRecordName: "example.com.",
			wantContentParts: []string{
				"ns1.example.com.",
				"hostmaster.example.com.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addedRecord *Record

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PATCH" {
					var rrsets struct {
						RRSets []RRSet `json:"rrsets"`
					}
					json.NewDecoder(r.Body).Decode(&rrsets)
					if len(rrsets.RRSets) > 0 && len(rrsets.RRSets[0].Records) > 0 {
						addedRecord = &Record{
							Name:    rrsets.RRSets[0].Name,
							Type:    rrsets.RRSets[0].Type,
							Content: rrsets.RRSets[0].Records[0].Content,
							TTL:     rrsets.RRSets[0].TTL,
						}
					}
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")

			err := AddSOARecord(client, tt.zone)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, addedRecord)
			assert.Equal(t, "SOA", addedRecord.Type)
			assert.Equal(t, tt.wantRecordName, addedRecord.Name)
			for _, part := range tt.wantContentParts {
				assert.Contains(t, addedRecord.Content, part)
			}
		})
	}
}

func TestAddNSRecord(t *testing.T) {
	tests := []struct {
		name           string
		zone           string
		nameserver     string
		wantErr        bool
		wantRecordName string
		wantContent    string
	}{
		{
			name:           "add NS record",
			zone:           "example.com",
			nameserver:     "ns1.example.com",
			wantErr:        false,
			wantRecordName: "example.com.",
			wantContent:    "ns1.example.com.",
		},
		{
			name:           "add NS record with trailing dots",
			zone:           "example.com.",
			nameserver:     "ns1.example.com.",
			wantErr:        false,
			wantRecordName: "example.com.",
			wantContent:    "ns1.example.com.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addedRecord *Record

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PATCH" {
					var rrsets struct {
						RRSets []RRSet `json:"rrsets"`
					}
					json.NewDecoder(r.Body).Decode(&rrsets)
					if len(rrsets.RRSets) > 0 && len(rrsets.RRSets[0].Records) > 0 {
						addedRecord = &Record{
							Name:    rrsets.RRSets[0].Name,
							Type:    rrsets.RRSets[0].Type,
							Content: rrsets.RRSets[0].Records[0].Content,
							TTL:     rrsets.RRSets[0].TTL,
						}
					}
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")

			err := AddNSRecord(client, tt.zone, tt.nameserver)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, addedRecord)
			assert.Equal(t, "NS", addedRecord.Type)
			assert.Equal(t, tt.wantRecordName, addedRecord.Name)
			assert.Equal(t, tt.wantContent, addedRecord.Content)
		})
	}
}

func TestAddWildcardRecord(t *testing.T) {
	tests := []struct {
		name           string
		zone           string
		ip             string
		wantErr        bool
		wantRecordName string
		wantContent    string
	}{
		{
			name:           "add wildcard A record",
			zone:           "k8s.example.com",
			ip:             "192.168.1.100",
			wantErr:        false,
			wantRecordName: "*.k8s.example.com.",
			wantContent:    "192.168.1.100",
		},
		{
			name:           "add wildcard with trailing dot",
			zone:           "k8s.example.com.",
			ip:             "10.0.0.100",
			wantErr:        false,
			wantRecordName: "*.k8s.example.com.",
			wantContent:    "10.0.0.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addedRecord *Record

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PATCH" {
					var rrsets struct {
						RRSets []RRSet `json:"rrsets"`
					}
					json.NewDecoder(r.Body).Decode(&rrsets)
					if len(rrsets.RRSets) > 0 && len(rrsets.RRSets[0].Records) > 0 {
						addedRecord = &Record{
							Name:    rrsets.RRSets[0].Name,
							Type:    rrsets.RRSets[0].Type,
							Content: rrsets.RRSets[0].Records[0].Content,
							TTL:     rrsets.RRSets[0].TTL,
						}
					}
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")

			err := AddWildcardRecord(client, tt.zone, tt.ip)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, addedRecord)
			assert.Equal(t, "A", addedRecord.Type)
			assert.Equal(t, tt.wantRecordName, addedRecord.Name)
			assert.Equal(t, tt.wantContent, addedRecord.Content)
		})
	}
}

func TestAddARecord(t *testing.T) {
	tests := []struct {
		name           string
		zone           string
		recordName     string
		ip             string
		wantErr        bool
		wantRecordName string
		wantContent    string
	}{
		{
			name:           "add A record - short name",
			zone:           "example.com",
			recordName:     "openbao",
			ip:             "192.168.1.10",
			wantErr:        false,
			wantRecordName: "openbao.example.com.",
			wantContent:    "192.168.1.10",
		},
		{
			name:           "add A record - FQDN",
			zone:           "example.com",
			recordName:     "openbao.example.com",
			ip:             "192.168.1.10",
			wantErr:        false,
			wantRecordName: "openbao.example.com.",
			wantContent:    "192.168.1.10",
		},
		{
			name:           "add A record - with trailing dot",
			zone:           "example.com.",
			recordName:     "zot",
			ip:             "192.168.1.20",
			wantErr:        false,
			wantRecordName: "zot.example.com.",
			wantContent:    "192.168.1.20",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addedRecord *Record

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PATCH" {
					var rrsets struct {
						RRSets []RRSet `json:"rrsets"`
					}
					json.NewDecoder(r.Body).Decode(&rrsets)
					if len(rrsets.RRSets) > 0 && len(rrsets.RRSets[0].Records) > 0 {
						addedRecord = &Record{
							Name:    rrsets.RRSets[0].Name,
							Type:    rrsets.RRSets[0].Type,
							Content: rrsets.RRSets[0].Records[0].Content,
							TTL:     rrsets.RRSets[0].TTL,
						}
					}
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")

			err := AddARecord(client, tt.zone, tt.recordName, tt.ip)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, addedRecord)
			assert.Equal(t, "A", addedRecord.Type)
			assert.Equal(t, tt.wantRecordName, addedRecord.Name)
			assert.Equal(t, tt.wantContent, addedRecord.Content)
		})
	}
}

func TestAddCNAMERecord(t *testing.T) {
	tests := []struct {
		name           string
		zone           string
		recordName     string
		target         string
		wantErr        bool
		wantRecordName string
		wantContent    string
	}{
		{
			name:           "add CNAME record",
			zone:           "example.com",
			recordName:     "www",
			target:         "home.example.com",
			wantErr:        false,
			wantRecordName: "www.example.com.",
			wantContent:    "home.example.com.",
		},
		{
			name:           "add CNAME with trailing dots",
			zone:           "example.com.",
			recordName:     "www",
			target:         "home.example.com.",
			wantErr:        false,
			wantRecordName: "www.example.com.",
			wantContent:    "home.example.com.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addedRecord *Record

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PATCH" {
					var rrsets struct {
						RRSets []RRSet `json:"rrsets"`
					}
					json.NewDecoder(r.Body).Decode(&rrsets)
					if len(rrsets.RRSets) > 0 && len(rrsets.RRSets[0].Records) > 0 {
						addedRecord = &Record{
							Name:    rrsets.RRSets[0].Name,
							Type:    rrsets.RRSets[0].Type,
							Content: rrsets.RRSets[0].Records[0].Content,
							TTL:     rrsets.RRSets[0].TTL,
						}
					}
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")

			err := AddCNAMERecord(client, tt.zone, tt.recordName, tt.target)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, addedRecord)
			assert.Equal(t, "CNAME", addedRecord.Type)
			assert.Equal(t, tt.wantRecordName, addedRecord.Name)
			assert.Equal(t, tt.wantContent, addedRecord.Content)
		})
	}
}

func TestInfrastructureRecordConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      InfrastructureRecordConfig
		wantErr     bool
		errContains string
		wantZone    string
	}{
		{
			name: "valid config",
			config: InfrastructureRecordConfig{
				Zone:      "infraexample.com",
				OpenBAOIP: "192.168.1.10",
				DNSIP:     "192.168.1.10",
				ZotIP:     "192.168.1.10",
				K8sVIP:    "192.168.1.100",
			},
			wantErr:  false,
			wantZone: "infraexample.com.",
		},
		{
			name: "valid config with TrueNAS",
			config: InfrastructureRecordConfig{
				Zone:       "infraexample.com",
				OpenBAOIP:  "192.168.1.10",
				DNSIP:      "192.168.1.10",
				ZotIP:      "192.168.1.10",
				TrueNASIP:  "192.168.1.15",
				K8sVIP:     "192.168.1.100",
			},
			wantErr:  false,
			wantZone: "infraexample.com.",
		},
		{
			name: "valid config with public zone",
			config: InfrastructureRecordConfig{
				Zone:        "infraexample.com",
				OpenBAOIP:   "192.168.1.10",
				DNSIP:       "192.168.1.10",
				ZotIP:       "192.168.1.10",
				K8sVIP:      "192.168.1.100",
				IsPublic:    true,
				PublicCNAME: "home.example.com",
			},
			wantErr:  false,
			wantZone: "infraexample.com.",
		},
		{
			name: "missing zone",
			config: InfrastructureRecordConfig{
				OpenBAOIP: "192.168.1.10",
				DNSIP:     "192.168.1.10",
				ZotIP:     "192.168.1.10",
				K8sVIP:    "192.168.1.100",
			},
			wantErr:     true,
			errContains: "zone is required",
		},
		{
			name: "missing OpenBAO IP",
			config: InfrastructureRecordConfig{
				Zone:   "infraexample.com",
				DNSIP:  "192.168.1.10",
				ZotIP:  "192.168.1.10",
				K8sVIP: "192.168.1.100",
			},
			wantErr:     true,
			errContains: "openbao_ip is required",
		},
		{
			name: "missing DNS IP",
			config: InfrastructureRecordConfig{
				Zone:      "infraexample.com",
				OpenBAOIP: "192.168.1.10",
				ZotIP:     "192.168.1.10",
				K8sVIP:    "192.168.1.100",
			},
			wantErr:     true,
			errContains: "dns_ip is required",
		},
		{
			name: "missing Zot IP",
			config: InfrastructureRecordConfig{
				Zone:      "infraexample.com",
				OpenBAOIP: "192.168.1.10",
				DNSIP:     "192.168.1.10",
				K8sVIP:    "192.168.1.100",
			},
			wantErr:     true,
			errContains: "zot_ip is required",
		},
		{
			name: "missing K8s VIP",
			config: InfrastructureRecordConfig{
				Zone:      "infraexample.com",
				OpenBAOIP: "192.168.1.10",
				DNSIP:     "192.168.1.10",
				ZotIP:     "192.168.1.10",
			},
			wantErr:     true,
			errContains: "k8s_vip is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			if tt.wantZone != "" {
				assert.Equal(t, tt.wantZone, tt.config.Zone)
			}
		})
	}
}

func TestInitializeInfrastructureDNS(t *testing.T) {
	tests := []struct {
		name             string
		config           InfrastructureRecordConfig
		wantErr          bool
		errContains      string
		wantRecordCount  int
		wantRecordNames  []string
		wantRecordTypes  []string
	}{
		{
			name: "initialize infrastructure DNS - basic",
			config: InfrastructureRecordConfig{
				Zone:      "infraexample.com",
				OpenBAOIP: "192.168.1.10",
				DNSIP:     "192.168.1.10",
				ZotIP:     "192.168.1.10",
				K8sVIP:    "192.168.1.100",
			},
			wantErr:         false,
			wantRecordCount: 4, // openbao, dns, zot, k8s
			wantRecordNames: []string{
				"openbao.infraexample.com.",
				"dns.infraexample.com.",
				"zot.infraexample.com.",
				"k8s.infraexample.com.",
			},
			wantRecordTypes: []string{"A", "A", "A", "A"},
		},
		{
			name: "initialize infrastructure DNS - with TrueNAS",
			config: InfrastructureRecordConfig{
				Zone:       "infraexample.com",
				OpenBAOIP:  "192.168.1.10",
				DNSIP:      "192.168.1.10",
				ZotIP:      "192.168.1.10",
				TrueNASIP:  "192.168.1.15",
				K8sVIP:     "192.168.1.100",
			},
			wantErr:         false,
			wantRecordCount: 5, // openbao, dns, zot, truenas, k8s
			wantRecordNames: []string{
				"openbao.infraexample.com.",
				"dns.infraexample.com.",
				"zot.infraexample.com.",
				"truenas.infraexample.com.",
				"k8s.infraexample.com.",
			},
			wantRecordTypes: []string{"A", "A", "A", "A", "A"},
		},
		{
			name: "initialize infrastructure DNS - public zone",
			config: InfrastructureRecordConfig{
				Zone:        "infraexample.com",
				OpenBAOIP:   "192.168.1.10",
				DNSIP:       "192.168.1.10",
				ZotIP:       "192.168.1.10",
				K8sVIP:      "192.168.1.100",
				IsPublic:    true,
				PublicCNAME: "home.example.com",
			},
			wantErr:         false,
			wantRecordCount: 4, // openbao, dns, zot, k8s (split-horizon handled by PowerDNS config)
			wantRecordNames: []string{
				"openbao.infraexample.com.",
				"dns.infraexample.com.",
				"zot.infraexample.com.",
				"k8s.infraexample.com.",
			},
			wantRecordTypes: []string{"A", "A", "A", "A"},
		},
		{
			name: "invalid config - missing zone",
			config: InfrastructureRecordConfig{
				OpenBAOIP: "192.168.1.10",
				DNSIP:     "192.168.1.10",
				ZotIP:     "192.168.1.10",
				K8sVIP:    "192.168.1.100",
			},
			wantErr:     true,
			errContains: "zone is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addedRecords []Record

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PATCH" {
					var rrsets struct {
						RRSets []RRSet `json:"rrsets"`
					}
					json.NewDecoder(r.Body).Decode(&rrsets)
					for _, rrset := range rrsets.RRSets {
						for _, record := range rrset.Records {
							addedRecords = append(addedRecords, Record{
								Name:    rrset.Name,
								Type:    rrset.Type,
								Content: record.Content,
								TTL:     rrset.TTL,
							})
						}
					}
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")

			err := InitializeInfrastructureDNS(client, tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Len(t, addedRecords, tt.wantRecordCount)

			// Verify all expected records were added
			for i, expectedName := range tt.wantRecordNames {
				found := false
				for _, record := range addedRecords {
					if record.Name == expectedName {
						found = true
						assert.Equal(t, tt.wantRecordTypes[i], record.Type)
						break
					}
				}
				assert.True(t, found, "Expected record %s not found", expectedName)
			}
		})
	}
}

func TestKubernetesZoneConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      KubernetesZoneConfig
		wantErr     bool
		errContains string
		wantZone    string
	}{
		{
			name: "valid config",
			config: KubernetesZoneConfig{
				Zone:   "k8sexample.com",
				K8sVIP: "192.168.1.100",
			},
			wantErr:  false,
			wantZone: "k8sexample.com.",
		},
		{
			name: "valid config with public zone",
			config: KubernetesZoneConfig{
				Zone:        "k8sexample.com",
				K8sVIP:      "192.168.1.100",
				IsPublic:    true,
				PublicCNAME: "home.example.com",
			},
			wantErr:  false,
			wantZone: "k8sexample.com.",
		},
		{
			name: "missing zone",
			config: KubernetesZoneConfig{
				K8sVIP: "192.168.1.100",
			},
			wantErr:     true,
			errContains: "zone is required",
		},
		{
			name: "missing K8s VIP",
			config: KubernetesZoneConfig{
				Zone: "k8sexample.com",
			},
			wantErr:     true,
			errContains: "k8s_vip is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			if tt.wantZone != "" {
				assert.Equal(t, tt.wantZone, tt.config.Zone)
			}
		})
	}
}

func TestInitializeKubernetesDNS(t *testing.T) {
	tests := []struct {
		name            string
		config          KubernetesZoneConfig
		wantErr         bool
		errContains     string
		wantRecordName  string
		wantRecordType  string
		wantRecordValue string
	}{
		{
			name: "initialize kubernetes DNS - basic",
			config: KubernetesZoneConfig{
				Zone:   "k8sexample.com",
				K8sVIP: "192.168.1.100",
			},
			wantErr:         false,
			wantRecordName:  "*.k8sexample.com.",
			wantRecordType:  "A",
			wantRecordValue: "192.168.1.100",
		},
		{
			name: "initialize kubernetes DNS - public zone",
			config: KubernetesZoneConfig{
				Zone:        "k8sexample.com",
				K8sVIP:      "192.168.1.100",
				IsPublic:    true,
				PublicCNAME: "home.example.com",
			},
			wantErr:         false,
			wantRecordName:  "*.k8sexample.com.",
			wantRecordType:  "A",
			wantRecordValue: "192.168.1.100",
		},
		{
			name: "invalid config - missing zone",
			config: KubernetesZoneConfig{
				K8sVIP: "192.168.1.100",
			},
			wantErr:     true,
			errContains: "zone is required",
		},
		{
			name: "invalid config - missing VIP",
			config: KubernetesZoneConfig{
				Zone: "k8sexample.com",
			},
			wantErr:     true,
			errContains: "k8s_vip is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addedRecords []Record

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PATCH" {
					var rrsets struct {
						RRSets []RRSet `json:"rrsets"`
					}
					json.NewDecoder(r.Body).Decode(&rrsets)
					for _, rrset := range rrsets.RRSets {
						for _, record := range rrset.Records {
							addedRecords = append(addedRecords, Record{
								Name:    rrset.Name,
								Type:    rrset.Type,
								Content: record.Content,
								TTL:     rrset.TTL,
							})
						}
					}
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")

			err := InitializeKubernetesDNS(client, tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Len(t, addedRecords, 1, "Should have exactly one wildcard record")

			record := addedRecords[0]
			assert.Equal(t, tt.wantRecordName, record.Name)
			assert.Equal(t, tt.wantRecordType, record.Type)
			assert.Equal(t, tt.wantRecordValue, record.Content)
		})
	}
}
