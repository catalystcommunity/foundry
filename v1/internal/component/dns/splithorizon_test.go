package dns

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsInternalQuery(t *testing.T) {
	tests := []struct {
		name           string
		sourceIP       string
		internalRanges []string
		wantInternal   bool
		wantErr        bool
	}{
		{
			name:         "RFC1918 - 10.x.x.x",
			sourceIP:     "10.0.0.1",
			wantInternal: true,
			wantErr:      false,
		},
		{
			name:         "RFC1918 - 192.168.x.x",
			sourceIP:     "192.168.1.100",
			wantInternal: true,
			wantErr:      false,
		},
		{
			name:         "RFC1918 - 172.16-31.x.x",
			sourceIP:     "172.16.0.1",
			wantInternal: true,
			wantErr:      false,
		},
		{
			name:         "RFC1918 - 172.31.x.x (end of range)",
			sourceIP:     "172.31.255.254",
			wantInternal: true,
			wantErr:      false,
		},
		{
			name:         "Public IP - Google DNS",
			sourceIP:     "8.8.8.8",
			wantInternal: false,
			wantErr:      false,
		},
		{
			name:         "Public IP - Cloudflare DNS",
			sourceIP:     "1.1.1.1",
			wantInternal: false,
			wantErr:      false,
		},
		{
			name:         "Outside RFC1918 172 range - 172.15.x.x",
			sourceIP:     "172.15.255.254",
			wantInternal: false,
			wantErr:      false,
		},
		{
			name:         "Outside RFC1918 172 range - 172.32.x.x",
			sourceIP:     "172.32.0.1",
			wantInternal: false,
			wantErr:      false,
		},
		{
			name:         "Invalid IP",
			sourceIP:     "not.an.ip.address",
			wantInternal: false,
			wantErr:      true,
		},
		{
			name:         "Empty IP",
			sourceIP:     "",
			wantInternal: false,
			wantErr:      true,
		},
		{
			name:           "Custom internal range - match",
			sourceIP:       "203.0.113.5",
			internalRanges: []string{"203.0.113.0/24"},
			wantInternal:   true,
			wantErr:        false,
		},
		{
			name:           "Custom internal range - no match",
			sourceIP:       "203.0.114.5",
			internalRanges: []string{"203.0.113.0/24"},
			wantInternal:   false,
			wantErr:        false,
		},
		{
			name:           "Multiple custom ranges - first match",
			sourceIP:       "203.0.113.5",
			internalRanges: []string{"203.0.113.0/24", "198.51.100.0/24"},
			wantInternal:   true,
			wantErr:        false,
		},
		{
			name:           "Multiple custom ranges - second match",
			sourceIP:       "198.51.100.5",
			internalRanges: []string{"203.0.113.0/24", "198.51.100.0/24"},
			wantInternal:   true,
			wantErr:        false,
		},
		{
			name:           "Invalid CIDR in custom ranges",
			sourceIP:       "192.168.1.1",
			internalRanges: []string{"not-a-cidr"},
			wantInternal:   false,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsInternalQuery(tt.sourceIP, tt.internalRanges)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantInternal, got)
		})
	}
}

func TestGenerateCNAMERecord(t *testing.T) {
	tests := []struct {
		name        string
		publicCNAME string
		want        string
	}{
		{
			name:        "without trailing dot",
			publicCNAME: "home.example.com",
			want:        "home.example.com.",
		},
		{
			name:        "with trailing dot",
			publicCNAME: "home.example.com.",
			want:        "home.example.com.",
		},
		{
			name:        "subdomain without trailing dot",
			publicCNAME: "ddns.home.example.com",
			want:        "ddns.home.example.com.",
		},
		{
			name:        "empty string",
			publicCNAME: "",
			want:        ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateCNAMERecord(tt.publicCNAME)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateARecord(t *testing.T) {
	tests := []struct {
		name    string
		localIP string
		want    string
		wantErr bool
	}{
		{
			name:    "valid IPv4 - 192.168.1.10",
			localIP: "192.168.1.10",
			want:    "192.168.1.10",
			wantErr: false,
		},
		{
			name:    "valid IPv4 - 10.0.0.1",
			localIP: "10.0.0.1",
			want:    "10.0.0.1",
			wantErr: false,
		},
		{
			name:    "valid IPv4 - 172.16.0.1",
			localIP: "172.16.0.1",
			want:    "172.16.0.1",
			wantErr: false,
		},
		{
			name:    "invalid IP",
			localIP: "not.an.ip",
			want:    "",
			wantErr: true,
		},
		{
			name:    "empty IP",
			localIP: "",
			want:    "",
			wantErr: true,
		},
		{
			name:    "IPv6 address",
			localIP: "2001:db8::1",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateARecord(tt.localIP)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetermineRecordContent(t *testing.T) {
	tests := []struct {
		name           string
		sourceIP       string
		config         SplitHorizonConfig
		wantRecordType string
		wantContent    string
		wantErr        bool
	}{
		{
			name:     "internal query - RFC1918",
			sourceIP: "192.168.1.100",
			config: SplitHorizonConfig{
				PublicCNAME: "home.example.com",
				LocalIP:     "192.168.1.10",
			},
			wantRecordType: "A",
			wantContent:    "192.168.1.10",
			wantErr:        false,
		},
		{
			name:     "external query - public IP",
			sourceIP: "8.8.8.8",
			config: SplitHorizonConfig{
				PublicCNAME: "home.example.com",
				LocalIP:     "192.168.1.10",
			},
			wantRecordType: "CNAME",
			wantContent:    "home.example.com.",
			wantErr:        false,
		},
		{
			name:     "internal query - 10.x.x.x",
			sourceIP: "10.0.0.5",
			config: SplitHorizonConfig{
				PublicCNAME: "ddns.home.example.com",
				LocalIP:     "10.0.0.10",
			},
			wantRecordType: "A",
			wantContent:    "10.0.0.10",
			wantErr:        false,
		},
		{
			name:     "internal query - 172.16.x.x",
			sourceIP: "172.16.50.1",
			config: SplitHorizonConfig{
				PublicCNAME: "home.example.com",
				LocalIP:     "172.16.0.10",
			},
			wantRecordType: "A",
			wantContent:    "172.16.0.10",
			wantErr:        false,
		},
		{
			name:     "custom internal range - match",
			sourceIP: "203.0.113.50",
			config: SplitHorizonConfig{
				PublicCNAME:    "home.example.com",
				LocalIP:        "203.0.113.10",
				InternalRanges: []string{"203.0.113.0/24"},
			},
			wantRecordType: "A",
			wantContent:    "203.0.113.10",
			wantErr:        false,
		},
		{
			name:     "custom internal range - no match",
			sourceIP: "203.0.114.50",
			config: SplitHorizonConfig{
				PublicCNAME:    "home.example.com",
				LocalIP:        "203.0.113.10",
				InternalRanges: []string{"203.0.113.0/24"},
			},
			wantRecordType: "CNAME",
			wantContent:    "home.example.com.",
			wantErr:        false,
		},
		{
			name:     "invalid source IP",
			sourceIP: "not.an.ip",
			config: SplitHorizonConfig{
				PublicCNAME: "home.example.com",
				LocalIP:     "192.168.1.10",
			},
			wantRecordType: "",
			wantContent:    "",
			wantErr:        true,
		},
		{
			name:     "invalid local IP",
			sourceIP: "192.168.1.100",
			config: SplitHorizonConfig{
				PublicCNAME: "home.example.com",
				LocalIP:     "not.an.ip",
			},
			wantRecordType: "",
			wantContent:    "",
			wantErr:        true,
		},
		{
			name:     "CNAME with trailing dot",
			sourceIP: "8.8.8.8",
			config: SplitHorizonConfig{
				PublicCNAME: "home.example.com.",
				LocalIP:     "192.168.1.10",
			},
			wantRecordType: "CNAME",
			wantContent:    "home.example.com.",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRecordType, gotContent, err := DetermineRecordContent(tt.sourceIP, tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, gotRecordType)
				assert.Empty(t, gotContent)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRecordType, gotRecordType)
			assert.Equal(t, tt.wantContent, gotContent)
		})
	}
}
