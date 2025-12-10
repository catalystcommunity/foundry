package host

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHost_Validate(t *testing.T) {
	tests := []struct {
		name    string
		host    *Host
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid host with hostname",
			host: &Host{
				Hostname:  "server1",
				Address:   "example.com",
				Port:      22,
				User:      "admin",
				SSHKeySet: false,
			},
			wantErr: false,
		},
		{
			name: "valid host with IP address",
			host: &Host{
				Hostname:  "server1",
				Address:   "192.168.1.100",
				Port:      22,
				User:      "admin",
				SSHKeySet: true,
			},
			wantErr: false,
		},
		{
			name: "valid host with custom port",
			host: &Host{
				Hostname:  "server2",
				Address:   "server2.example.com",
				Port:      2222,
				User:      "deploy",
				SSHKeySet: false,
			},
			wantErr: false,
		},
		{
			name: "valid host with subdomain",
			host: &Host{
				Hostname:  "web-prod",
				Address:   "web.prod.example.com",
				Port:      22,
				User:      "ubuntu",
				SSHKeySet: false,
			},
			wantErr: false,
		},
		{
			name: "empty hostname",
			host: &Host{
				Hostname:  "",
				Address:   "example.com",
				Port:      22,
				User:      "admin",
				SSHKeySet: false,
			},
			wantErr: true,
			errMsg:  "hostname cannot be empty",
		},
		{
			name: "invalid hostname - starts with hyphen",
			host: &Host{
				Hostname:  "-server",
				Address:   "example.com",
				Port:      22,
				User:      "admin",
				SSHKeySet: false,
			},
			wantErr: true,
			errMsg:  "invalid hostname format",
		},
		{
			name: "invalid hostname - special characters",
			host: &Host{
				Hostname:  "server@1",
				Address:   "example.com",
				Port:      22,
				User:      "admin",
				SSHKeySet: false,
			},
			wantErr: true,
			errMsg:  "invalid hostname format",
		},
		{
			name: "empty address",
			host: &Host{
				Hostname:  "server1",
				Address:   "",
				Port:      22,
				User:      "admin",
				SSHKeySet: false,
			},
			wantErr: true,
			errMsg:  "address cannot be empty",
		},
		{
			name: "invalid address",
			host: &Host{
				Hostname:  "server1",
				Address:   "invalid@address",
				Port:      22,
				User:      "admin",
				SSHKeySet: false,
			},
			wantErr: true,
			errMsg:  "invalid address format",
		},
		{
			name: "port zero",
			host: &Host{
				Hostname:  "server1",
				Address:   "example.com",
				Port:      0,
				User:      "admin",
				SSHKeySet: false,
			},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name: "port too high",
			host: &Host{
				Hostname:  "server1",
				Address:   "example.com",
				Port:      70000,
				User:      "admin",
				SSHKeySet: false,
			},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name: "empty user",
			host: &Host{
				Hostname:  "server1",
				Address:   "example.com",
				Port:      22,
				User:      "",
				SSHKeySet: false,
			},
			wantErr: true,
			errMsg:  "user cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.host.Validate()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestIsValidHostname(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		want     bool
	}{
		{"simple", "server1", true},
		{"with dash", "web-server", true},
		{"with dot", "server.example.com", true},
		{"subdomain", "api.prod.example.com", true},
		{"starts with number", "1server", true},
		{"ends with number", "server1", true},
		{"single char", "a", true},
		{"empty", "", false},
		{"starts with dash", "-server", false},
		{"ends with dash", "server-", false},
		{"starts with dot", ".server", false},
		{"ends with dot", "server.", false},
		{"double dot", "server..example", false},
		{"special chars", "server@example", false},
		{"space", "server 1", false},
		{"underscore", "server_1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidHostname(tt.hostname)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsValidAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    bool
	}{
		{"IPv4", "192.168.1.1", true},
		{"IPv6", "2001:0db8:85a3::8a2e:0370:7334", true},
		{"hostname", "example.com", true},
		{"subdomain", "server.example.com", true},
		{"localhost", "localhost", true},
		{"invalid", "invalid@address", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidAddress(tt.address)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHost_String(t *testing.T) {
	tests := []struct {
		name string
		host *Host
		want string
	}{
		{
			name: "without SSH key",
			host: &Host{
				Hostname:  "server1",
				Address:   "192.168.1.100",
				Port:      22,
				User:      "admin",
				SSHKeySet: false,
			},
			want: "server1 (admin@192.168.1.100:22) [no key]",
		},
		{
			name: "with SSH key",
			host: &Host{
				Hostname:  "server2",
				Address:   "example.com",
				Port:      2222,
				User:      "deploy",
				SSHKeySet: true,
			},
			want: "server2 (deploy@example.com:2222) [key set]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.host.String()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultHost(t *testing.T) {
	host := DefaultHost("server1", "192.168.1.100", "admin")

	assert.Equal(t, "server1", host.Hostname)
	assert.Equal(t, "192.168.1.100", host.Address)
	assert.Equal(t, 22, host.Port)
	assert.Equal(t, "admin", host.User)
	assert.False(t, host.SSHKeySet)
	assert.Nil(t, host.Labels)

	// Verify it passes validation
	assert.NoError(t, host.Validate())
}

func TestHost_LabelMethods(t *testing.T) {
	t.Run("HasLabel on nil labels", func(t *testing.T) {
		h := &Host{Labels: nil}
		assert.False(t, h.HasLabel("foo"))
	})

	t.Run("HasLabel on empty labels", func(t *testing.T) {
		h := &Host{Labels: map[string]string{}}
		assert.False(t, h.HasLabel("foo"))
	})

	t.Run("HasLabel finds existing label", func(t *testing.T) {
		h := &Host{Labels: map[string]string{"foo": "bar"}}
		assert.True(t, h.HasLabel("foo"))
	})

	t.Run("GetLabel on nil labels", func(t *testing.T) {
		h := &Host{Labels: nil}
		assert.Equal(t, "", h.GetLabel("foo"))
	})

	t.Run("GetLabel returns value", func(t *testing.T) {
		h := &Host{Labels: map[string]string{"foo": "bar"}}
		assert.Equal(t, "bar", h.GetLabel("foo"))
	})

	t.Run("GetLabel returns empty for missing", func(t *testing.T) {
		h := &Host{Labels: map[string]string{"foo": "bar"}}
		assert.Equal(t, "", h.GetLabel("baz"))
	})

	t.Run("SetLabel on nil labels", func(t *testing.T) {
		h := &Host{Labels: nil}
		h.SetLabel("foo", "bar")
		assert.Equal(t, "bar", h.Labels["foo"])
	})

	t.Run("SetLabel overwrites existing", func(t *testing.T) {
		h := &Host{Labels: map[string]string{"foo": "old"}}
		h.SetLabel("foo", "new")
		assert.Equal(t, "new", h.Labels["foo"])
	})

	t.Run("RemoveLabel on nil labels", func(t *testing.T) {
		h := &Host{Labels: nil}
		h.RemoveLabel("foo") // should not panic
		assert.Nil(t, h.Labels)
	})

	t.Run("RemoveLabel removes existing", func(t *testing.T) {
		h := &Host{Labels: map[string]string{"foo": "bar", "baz": "qux"}}
		h.RemoveLabel("foo")
		assert.False(t, h.HasLabel("foo"))
		assert.True(t, h.HasLabel("baz"))
	})
}

func TestValidateLabelKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
		errMsg  string
	}{
		// Valid keys
		{"simple", "foo", false, ""},
		{"with dash", "foo-bar", false, ""},
		{"with underscore", "foo_bar", false, ""},
		{"with dot", "foo.bar", false, ""},
		{"single char", "a", false, ""},
		{"numbers", "a123", false, ""},
		{"with prefix", "example.com/foo", false, ""},
		{"with subdomain prefix", "app.example.com/foo", false, ""},
		{"max name length", "a" + strings.Repeat("b", 61) + "z", false, ""},

		// Invalid keys
		{"empty", "", true, "cannot be empty"},
		{"starts with dash", "-foo", true, "must begin and end with alphanumeric"},
		{"ends with dash", "foo-", true, "must begin and end with alphanumeric"},
		{"starts with dot", ".foo", true, "must begin and end with alphanumeric"},
		{"ends with dot", "foo.", true, "must begin and end with alphanumeric"},
		{"special chars", "foo@bar", true, "must begin and end with alphanumeric"},
		{"space", "foo bar", true, "must begin and end with alphanumeric"},
		{"empty name with prefix", "example.com/", true, "label name cannot be empty"},
		{"name too long", "a" + strings.Repeat("b", 63) + "z", true, "must be 63 characters or less"},
		{"invalid prefix chars", "EXAMPLE.COM/foo", true, "must be a valid DNS subdomain"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLabelKey(tt.key)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateLabelValue(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
		errMsg  string
	}{
		// Valid values
		{"empty", "", false, ""},
		{"simple", "foo", false, ""},
		{"with dash", "foo-bar", false, ""},
		{"with underscore", "foo_bar", false, ""},
		{"with dot", "foo.bar", false, ""},
		{"single char", "a", false, ""},
		{"numbers", "123", false, ""},

		// Invalid values
		{"starts with dash", "-foo", true, "must begin and end with alphanumeric"},
		{"ends with dash", "foo-", true, "must begin and end with alphanumeric"},
		{"space", "foo bar", true, "must begin and end with alphanumeric"},
		{"special chars", "foo@bar", true, "must begin and end with alphanumeric"},
		{"too long", strings.Repeat("a", 64), true, "must be 63 characters or less"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLabelValue(tt.value)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestHost_ValidateWithLabels(t *testing.T) {
	validHost := func() *Host {
		return &Host{
			Hostname: "server1",
			Address:  "192.168.1.100",
			Port:     22,
			User:     "admin",
		}
	}

	t.Run("valid host with valid labels", func(t *testing.T) {
		h := validHost()
		h.Labels = map[string]string{
			"environment": "production",
			"zone":        "us-east-1a",
		}
		require.NoError(t, h.Validate())
	})

	t.Run("valid host with prefixed labels", func(t *testing.T) {
		h := validHost()
		h.Labels = map[string]string{
			"app.example.com/tier": "frontend",
		}
		require.NoError(t, h.Validate())
	})

	t.Run("invalid label key", func(t *testing.T) {
		h := validHost()
		h.Labels = map[string]string{
			"-invalid": "value",
		}
		err := h.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid label key")
	})

	t.Run("invalid label value", func(t *testing.T) {
		h := validHost()
		h.Labels = map[string]string{
			"valid": "-invalid",
		}
		err := h.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid label value")
	})
}
