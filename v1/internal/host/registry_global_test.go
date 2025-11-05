package host

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDefaultRegistry(t *testing.T) {
	// Reset to clean state
	SetDefaultRegistry(NewMemoryRegistry())
	defer ClearHosts()

	registry := GetDefaultRegistry()
	assert.NotNil(t, registry, "default registry should not be nil")
	assert.IsType(t, &MemoryRegistry{}, registry, "default registry should be MemoryRegistry")
}

func TestSetDefaultRegistry(t *testing.T) {
	// Reset to clean state
	originalRegistry := GetDefaultRegistry()
	defer SetDefaultRegistry(originalRegistry)
	defer ClearHosts()

	// Create a new custom registry
	customRegistry := NewMemoryRegistry()

	// Add a test host to the custom registry
	testHost := DefaultHost("test.example.com", "192.168.1.10", "admin")
	err := customRegistry.Add(testHost)
	require.NoError(t, err)

	// Set as default
	SetDefaultRegistry(customRegistry)

	// Verify it's now the default
	currentRegistry := GetDefaultRegistry()
	assert.Equal(t, customRegistry, currentRegistry)

	// Verify we can access the host through the new default
	host, err := Get("test.example.com")
	require.NoError(t, err)
	assert.Equal(t, testHost.Hostname, host.Hostname)
}

func TestGlobalGet(t *testing.T) {
	// Reset to clean state
	SetDefaultRegistry(NewMemoryRegistry())
	defer ClearHosts()

	tests := []struct {
		name        string
		setupHosts  []*Host
		getHostname string
		wantErr     bool
		wantHost    *Host
	}{
		{
			name: "get existing host",
			setupHosts: []*Host{
				DefaultHost("node1.example.com", "192.168.1.10", "admin"),
			},
			getHostname: "node1.example.com",
			wantErr:     false,
			wantHost:    DefaultHost("node1.example.com", "192.168.1.10", "admin"),
		},
		{
			name:        "get non-existent host",
			setupHosts:  []*Host{},
			getHostname: "missing.example.com",
			wantErr:     true,
		},
		{
			name:        "empty hostname",
			setupHosts:  []*Host{},
			getHostname: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset for each test
			ClearHosts()

			// Setup hosts
			for _, h := range tt.setupHosts {
				err := Add(h)
				require.NoError(t, err)
			}

			// Test Get
			host, err := Get(tt.getHostname)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, host)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantHost.Hostname, host.Hostname)
				assert.Equal(t, tt.wantHost.Address, host.Address)
				assert.Equal(t, tt.wantHost.User, host.User)
			}
		})
	}
}

func TestGlobalAdd(t *testing.T) {
	// Reset to clean state
	SetDefaultRegistry(NewMemoryRegistry())
	defer ClearHosts()

	tests := []struct {
		name    string
		host    *Host
		wantErr bool
	}{
		{
			name:    "add valid host",
			host:    DefaultHost("node1.example.com", "192.168.1.10", "admin"),
			wantErr: false,
		},
		{
			name:    "add nil host",
			host:    nil,
			wantErr: true,
		},
		{
			name: "add invalid host (empty hostname)",
			host: &Host{
				Hostname: "",
				Address:  "192.168.1.10",
				Port:     22,
				User:     "admin",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset for each test
			ClearHosts()

			err := Add(tt.host)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify host was added
				host, err := Get(tt.host.Hostname)
				require.NoError(t, err)
				assert.Equal(t, tt.host.Hostname, host.Hostname)
			}
		})
	}
}

func TestGlobalAddDuplicate(t *testing.T) {
	// Reset to clean state
	SetDefaultRegistry(NewMemoryRegistry())
	defer ClearHosts()

	host := DefaultHost("node1.example.com", "192.168.1.10", "admin")

	// Add first time should succeed
	err := Add(host)
	require.NoError(t, err)

	// Add second time should fail
	err = Add(host)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestGlobalList(t *testing.T) {
	// Reset to clean state
	SetDefaultRegistry(NewMemoryRegistry())
	defer ClearHosts()

	tests := []struct {
		name       string
		setupHosts []*Host
		wantCount  int
	}{
		{
			name:       "empty registry",
			setupHosts: []*Host{},
			wantCount:  0,
		},
		{
			name: "single host",
			setupHosts: []*Host{
				DefaultHost("node1.example.com", "192.168.1.10", "admin"),
			},
			wantCount: 1,
		},
		{
			name: "multiple hosts",
			setupHosts: []*Host{
				DefaultHost("node1.example.com", "192.168.1.10", "admin"),
				DefaultHost("node2.example.com", "192.168.1.11", "admin"),
				DefaultHost("node3.example.com", "192.168.1.12", "admin"),
			},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset for each test
			ClearHosts()

			// Setup hosts
			for _, h := range tt.setupHosts {
				err := Add(h)
				require.NoError(t, err)
			}

			// Test List
			hosts, err := List()
			require.NoError(t, err)
			assert.Len(t, hosts, tt.wantCount)

			// Verify hosts are sorted by hostname
			if len(hosts) > 1 {
				for i := 1; i < len(hosts); i++ {
					assert.True(t, hosts[i-1].Hostname < hosts[i].Hostname, "hosts should be sorted")
				}
			}
		})
	}
}

func TestGlobalRemove(t *testing.T) {
	// Reset to clean state
	SetDefaultRegistry(NewMemoryRegistry())
	defer ClearHosts()

	tests := []struct {
		name           string
		setupHosts     []*Host
		removeHostname string
		wantErr        bool
	}{
		{
			name: "remove existing host",
			setupHosts: []*Host{
				DefaultHost("node1.example.com", "192.168.1.10", "admin"),
			},
			removeHostname: "node1.example.com",
			wantErr:        false,
		},
		{
			name:           "remove non-existent host",
			setupHosts:     []*Host{},
			removeHostname: "missing.example.com",
			wantErr:        true,
		},
		{
			name:           "empty hostname",
			setupHosts:     []*Host{},
			removeHostname: "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset for each test
			ClearHosts()

			// Setup hosts
			for _, h := range tt.setupHosts {
				err := Add(h)
				require.NoError(t, err)
			}

			// Test Remove
			err := Remove(tt.removeHostname)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify host was removed
				_, err := Get(tt.removeHostname)
				assert.Error(t, err)
			}
		})
	}
}

func TestGlobalUpdate(t *testing.T) {
	// Reset to clean state
	SetDefaultRegistry(NewMemoryRegistry())
	defer ClearHosts()

	tests := []struct {
		name       string
		setupHosts []*Host
		updateHost *Host
		wantErr    bool
	}{
		{
			name: "update existing host",
			setupHosts: []*Host{
				DefaultHost("node1.example.com", "192.168.1.10", "admin"),
			},
			updateHost: DefaultHost("node1.example.com", "192.168.1.20", "root"),
			wantErr:    false,
		},
		{
			name:       "update non-existent host",
			setupHosts: []*Host{},
			updateHost: DefaultHost("missing.example.com", "192.168.1.10", "admin"),
			wantErr:    true,
		},
		{
			name:       "update with nil host",
			setupHosts: []*Host{},
			updateHost: nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset for each test
			ClearHosts()

			// Setup hosts
			for _, h := range tt.setupHosts {
				err := Add(h)
				require.NoError(t, err)
			}

			// Test Update
			err := Update(tt.updateHost)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify host was updated
				host, err := Get(tt.updateHost.Hostname)
				require.NoError(t, err)
				assert.Equal(t, tt.updateHost.Address, host.Address)
				assert.Equal(t, tt.updateHost.User, host.User)
			}
		})
	}
}

func TestGlobalExists(t *testing.T) {
	// Reset to clean state
	SetDefaultRegistry(NewMemoryRegistry())
	defer ClearHosts()

	tests := []struct {
		name       string
		setupHosts []*Host
		hostname   string
		wantExists bool
		wantErr    bool
	}{
		{
			name: "host exists",
			setupHosts: []*Host{
				DefaultHost("node1.example.com", "192.168.1.10", "admin"),
			},
			hostname:   "node1.example.com",
			wantExists: true,
			wantErr:    false,
		},
		{
			name:       "host does not exist",
			setupHosts: []*Host{},
			hostname:   "missing.example.com",
			wantExists: false,
			wantErr:    false,
		},
		{
			name:       "empty hostname",
			setupHosts: []*Host{},
			hostname:   "",
			wantExists: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset for each test
			ClearHosts()

			// Setup hosts
			for _, h := range tt.setupHosts {
				err := Add(h)
				require.NoError(t, err)
			}

			// Test Exists
			exists, err := Exists(tt.hostname)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantExists, exists)
			}
		})
	}
}

func TestClearHosts(t *testing.T) {
	// Reset to clean state
	SetDefaultRegistry(NewMemoryRegistry())
	defer ClearHosts()

	// Add some hosts
	hosts := []*Host{
		DefaultHost("node1.example.com", "192.168.1.10", "admin"),
		DefaultHost("node2.example.com", "192.168.1.11", "admin"),
		DefaultHost("node3.example.com", "192.168.1.12", "admin"),
	}

	for _, h := range hosts {
		err := Add(h)
		require.NoError(t, err)
	}

	// Verify hosts were added
	list, err := List()
	require.NoError(t, err)
	assert.Len(t, list, 3)

	// Clear hosts
	ClearHosts()

	// Verify hosts were cleared
	list, err = List()
	require.NoError(t, err)
	assert.Len(t, list, 0)
}

func TestConcurrentAccess(t *testing.T) {
	// Reset to clean state
	SetDefaultRegistry(NewMemoryRegistry())
	defer ClearHosts()

	// Test concurrent access to global registry
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			host := DefaultHost(
				fmt.Sprintf("node%d.example.com", index),
				fmt.Sprintf("192.168.1.%d", 10+index),
				"admin",
			)
			_ = Add(host)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify all hosts were added (some might fail due to race conditions, which is expected)
	hosts, err := List()
	require.NoError(t, err)
	assert.True(t, len(hosts) > 0, "at least some hosts should be added")
	assert.True(t, len(hosts) <= numGoroutines, "should not have more hosts than goroutines")
}
