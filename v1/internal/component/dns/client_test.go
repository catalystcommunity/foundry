package dns

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	client := NewClient("http://localhost:8081", "test-key")
	assert.NotNil(t, client)
	assert.Equal(t, "http://localhost:8081", client.baseURL)
	assert.Equal(t, "test-key", client.apiKey)
	assert.NotNil(t, client.httpClient)
}

func TestCreateZone(t *testing.T) {
	tests := []struct {
		name       string
		zoneName   string
		zoneType   string
		wantErr    bool
		errMsg     string
		serverFunc func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:     "successful zone creation",
			zoneName: "example.com",
			zoneType: "Native",
			wantErr:  false,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/api/v1/servers/localhost/zones", r.URL.Path)
				assert.Equal(t, "test-api-key", r.Header.Get("X-API-Key"))

				var payload map[string]interface{}
				err := json.NewDecoder(r.Body).Decode(&payload)
				require.NoError(t, err)
				assert.Equal(t, "example.com", payload["name"])
				assert.Equal(t, "Native", payload["kind"])

				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"id":"example.com","name":"example.com"}`))
			},
		},
		{
			name:     "empty zone name",
			zoneName: "",
			zoneType: "Native",
			wantErr:  true,
			errMsg:   "zone name cannot be empty",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("should not make request with empty zone name")
			},
		},
		{
			name:     "default zone type",
			zoneName: "example.com",
			zoneType: "",
			wantErr:  false,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]interface{}
				json.NewDecoder(r.Body).Decode(&payload)
				assert.Equal(t, "Native", payload["kind"])

				w.WriteHeader(http.StatusCreated)
			},
		},
		{
			name:     "API error",
			zoneName: "example.com",
			zoneType: "Native",
			wantErr:  true,
			errMsg:   "API request failed",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"Zone already exists"}`))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")
			err := client.CreateZone(tt.zoneName, tt.zoneType)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDeleteZone(t *testing.T) {
	tests := []struct {
		name       string
		zoneName   string
		wantErr    bool
		errMsg     string
		serverFunc func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:     "successful deletion",
			zoneName: "example.com",
			wantErr:  false,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "DELETE", r.Method)
				assert.Equal(t, "/api/v1/servers/localhost/zones/example.com", r.URL.Path)
				assert.Equal(t, "test-api-key", r.Header.Get("X-API-Key"))

				w.WriteHeader(http.StatusNoContent)
			},
		},
		{
			name:     "empty zone name",
			zoneName: "",
			wantErr:  true,
			errMsg:   "zone name cannot be empty",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("should not make request with empty zone name")
			},
		},
		{
			name:     "zone not found",
			zoneName: "nonexistent.com",
			wantErr:  true,
			errMsg:   "API request failed",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"Zone not found"}`))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")
			err := client.DeleteZone(tt.zoneName)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestListZones(t *testing.T) {
	tests := []struct {
		name       string
		wantErr    bool
		errMsg     string
		wantZones  int
		serverFunc func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:      "successful list",
			wantErr:   false,
			wantZones: 2,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/api/v1/servers/localhost/zones", r.URL.Path)
				assert.Equal(t, "test-api-key", r.Header.Get("X-API-Key"))

				zones := []Zone{
					{ID: "example.com", Name: "example.com", Type: "Native"},
					{ID: "test.com", Name: "test.com", Type: "Native"},
				}

				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(zones)
			},
		},
		{
			name:      "empty list",
			wantErr:   false,
			wantZones: 0,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`[]`))
			},
		},
		{
			name:    "API error",
			wantErr: true,
			errMsg:  "API request failed",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"Internal server error"}`))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")
			zones, err := client.ListZones()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Len(t, zones, tt.wantZones)
			}
		})
	}
}

func TestAddRecord(t *testing.T) {
	tests := []struct {
		name       string
		zone       string
		recordName string
		recordType string
		content    string
		ttl        int
		wantErr    bool
		errMsg     string
		serverFunc func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:       "successful record addition",
			zone:       "example.com",
			recordName: "test.example.com",
			recordType: "A",
			content:    "192.168.1.10",
			ttl:        3600,
			wantErr:    false,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "PATCH", r.Method)
				assert.Equal(t, "/api/v1/servers/localhost/zones/example.com", r.URL.Path)
				assert.Equal(t, "test-api-key", r.Header.Get("X-API-Key"))

				var payload map[string]interface{}
				err := json.NewDecoder(r.Body).Decode(&payload)
				require.NoError(t, err)

				rrsets, ok := payload["rrsets"].([]interface{})
				require.True(t, ok)
				require.Len(t, rrsets, 1)

				rrset := rrsets[0].(map[string]interface{})
				assert.Equal(t, "test.example.com", rrset["name"])
				assert.Equal(t, "A", rrset["type"])
				assert.Equal(t, float64(3600), rrset["ttl"])
				assert.Equal(t, "REPLACE", rrset["changetype"])

				w.WriteHeader(http.StatusNoContent)
			},
		},
		{
			name:       "default TTL",
			zone:       "example.com",
			recordName: "test.example.com",
			recordType: "A",
			content:    "192.168.1.10",
			ttl:        0,
			wantErr:    false,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]interface{}
				json.NewDecoder(r.Body).Decode(&payload)

				rrsets := payload["rrsets"].([]interface{})
				rrset := rrsets[0].(map[string]interface{})
				assert.Equal(t, float64(3600), rrset["ttl"]) // Default TTL

				w.WriteHeader(http.StatusNoContent)
			},
		},
		{
			name:       "missing required field",
			zone:       "",
			recordName: "test.example.com",
			recordType: "A",
			content:    "192.168.1.10",
			ttl:        3600,
			wantErr:    true,
			errMsg:     "zone, name, type, and content are required",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("should not make request with missing field")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")
			err := client.AddRecord(tt.zone, tt.recordName, tt.recordType, tt.content, tt.ttl)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDeleteRecord(t *testing.T) {
	tests := []struct {
		name       string
		zone       string
		recordName string
		recordType string
		wantErr    bool
		errMsg     string
		serverFunc func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:       "successful deletion",
			zone:       "example.com",
			recordName: "test.example.com",
			recordType: "A",
			wantErr:    false,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "PATCH", r.Method)
				assert.Equal(t, "/api/v1/servers/localhost/zones/example.com", r.URL.Path)

				var payload map[string]interface{}
				err := json.NewDecoder(r.Body).Decode(&payload)
				require.NoError(t, err)

				rrsets, ok := payload["rrsets"].([]interface{})
				require.True(t, ok)
				require.Len(t, rrsets, 1)

				rrset := rrsets[0].(map[string]interface{})
				assert.Equal(t, "test.example.com", rrset["name"])
				assert.Equal(t, "A", rrset["type"])
				assert.Equal(t, "DELETE", rrset["changetype"])

				w.WriteHeader(http.StatusNoContent)
			},
		},
		{
			name:       "missing required field",
			zone:       "example.com",
			recordName: "",
			recordType: "A",
			wantErr:    true,
			errMsg:     "zone, name, and type are required",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("should not make request with missing field")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")
			err := client.DeleteRecord(tt.zone, tt.recordName, tt.recordType)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestListRecords(t *testing.T) {
	tests := []struct {
		name         string
		zone         string
		wantErr      bool
		errMsg       string
		wantRecords  int
		serverFunc   func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:        "successful list",
			zone:        "example.com",
			wantErr:     false,
			wantRecords: 2,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/api/v1/servers/localhost/zones/example.com", r.URL.Path)

				zoneData := map[string]interface{}{
					"id":   "example.com",
					"name": "example.com",
					"rrsets": []map[string]interface{}{
						{
							"name": "test.example.com",
							"type": "A",
							"ttl":  3600,
							"records": []map[string]interface{}{
								{"content": "192.168.1.10", "disabled": false},
							},
						},
						{
							"name": "mail.example.com",
							"type": "MX",
							"ttl":  7200,
							"records": []map[string]interface{}{
								{"content": "10 mail.example.com", "disabled": false},
							},
						},
					},
				}

				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(zoneData)
			},
		},
		{
			name:    "empty zone name",
			zone:    "",
			wantErr: true,
			errMsg:  "zone name cannot be empty",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("should not make request with empty zone name")
			},
		},
		{
			name:    "zone not found",
			zone:    "nonexistent.com",
			wantErr: true,
			errMsg:  "API request failed",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"Zone not found"}`))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			client := NewClient(server.URL, "test-api-key")
			records, err := client.ListRecords(tt.zone)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Len(t, records, tt.wantRecords)
			}
		})
	}
}
