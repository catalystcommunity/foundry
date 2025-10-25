package host

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMemoryRegistry(t *testing.T) {
	registry := NewMemoryRegistry()
	assert.NotNil(t, registry)
	assert.Equal(t, 0, registry.Count())
}

func TestMemoryRegistry_Add(t *testing.T) {
	registry := NewMemoryRegistry()

	t.Run("add valid host", func(t *testing.T) {
		host := DefaultHost("server1", "192.168.1.100", "admin")
		err := registry.Add(host)
		require.NoError(t, err)
		assert.Equal(t, 1, registry.Count())
	})

	t.Run("add duplicate hostname", func(t *testing.T) {
		host := DefaultHost("server1", "192.168.1.101", "admin")
		err := registry.Add(host)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("add nil host", func(t *testing.T) {
		err := registry.Add(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "host cannot be nil")
	})

	t.Run("add invalid host", func(t *testing.T) {
		host := &Host{
			Hostname: "server2",
			Address:  "192.168.1.102",
			Port:     22,
			User:     "", // Invalid: empty user
		}
		err := registry.Add(host)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid host")
	})

	t.Run("modifications to original don't affect stored host", func(t *testing.T) {
		host := DefaultHost("server3", "192.168.1.103", "admin")
		err := registry.Add(host)
		require.NoError(t, err)

		// Modify the original
		host.Port = 2222

		// Get the stored host
		stored, err := registry.Get("server3")
		require.NoError(t, err)

		// Should still be the default port
		assert.Equal(t, 22, stored.Port)
	})
}

func TestMemoryRegistry_Get(t *testing.T) {
	registry := NewMemoryRegistry()
	host := DefaultHost("server1", "192.168.1.100", "admin")
	_ = registry.Add(host)

	t.Run("get existing host", func(t *testing.T) {
		retrieved, err := registry.Get("server1")
		require.NoError(t, err)
		assert.Equal(t, "server1", retrieved.Hostname)
		assert.Equal(t, "192.168.1.100", retrieved.Address)
	})

	t.Run("get non-existent host", func(t *testing.T) {
		_, err := registry.Get("nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("get with empty hostname", func(t *testing.T) {
		_, err := registry.Get("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "hostname cannot be empty")
	})

	t.Run("modifications to retrieved don't affect stored host", func(t *testing.T) {
		retrieved, err := registry.Get("server1")
		require.NoError(t, err)

		// Modify the retrieved host
		retrieved.Port = 2222

		// Get again
		retrieved2, err := registry.Get("server1")
		require.NoError(t, err)

		// Should still be the default port
		assert.Equal(t, 22, retrieved2.Port)
	})
}

func TestMemoryRegistry_List(t *testing.T) {
	registry := NewMemoryRegistry()

	t.Run("empty registry", func(t *testing.T) {
		hosts, err := registry.List()
		require.NoError(t, err)
		assert.Empty(t, hosts)
	})

	t.Run("list multiple hosts", func(t *testing.T) {
		_ = registry.Add(DefaultHost("server3", "192.168.1.103", "admin"))
		_ = registry.Add(DefaultHost("server1", "192.168.1.101", "admin"))
		_ = registry.Add(DefaultHost("server2", "192.168.1.102", "admin"))

		hosts, err := registry.List()
		require.NoError(t, err)
		assert.Len(t, hosts, 3)

		// Should be sorted by hostname
		assert.Equal(t, "server1", hosts[0].Hostname)
		assert.Equal(t, "server2", hosts[1].Hostname)
		assert.Equal(t, "server3", hosts[2].Hostname)
	})

	t.Run("modifications to list don't affect registry", func(t *testing.T) {
		hosts, err := registry.List()
		require.NoError(t, err)

		// Modify one of the hosts
		hosts[0].Port = 9999

		// Get the host directly
		retrieved, err := registry.Get("server1")
		require.NoError(t, err)

		// Should still be the default port
		assert.Equal(t, 22, retrieved.Port)
	})
}

func TestMemoryRegistry_Remove(t *testing.T) {
	registry := NewMemoryRegistry()
	host := DefaultHost("server1", "192.168.1.100", "admin")
	_ = registry.Add(host)

	t.Run("remove existing host", func(t *testing.T) {
		err := registry.Remove("server1")
		require.NoError(t, err)
		assert.Equal(t, 0, registry.Count())
	})

	t.Run("remove non-existent host", func(t *testing.T) {
		err := registry.Remove("nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("remove with empty hostname", func(t *testing.T) {
		err := registry.Remove("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "hostname cannot be empty")
	})
}

func TestMemoryRegistry_Update(t *testing.T) {
	registry := NewMemoryRegistry()
	host := DefaultHost("server1", "192.168.1.100", "admin")
	_ = registry.Add(host)

	t.Run("update existing host", func(t *testing.T) {
		updated := DefaultHost("server1", "192.168.1.200", "root")
		updated.Port = 2222

		err := registry.Update(updated)
		require.NoError(t, err)

		retrieved, err := registry.Get("server1")
		require.NoError(t, err)
		assert.Equal(t, "192.168.1.200", retrieved.Address)
		assert.Equal(t, "root", retrieved.User)
		assert.Equal(t, 2222, retrieved.Port)
	})

	t.Run("update non-existent host", func(t *testing.T) {
		host := DefaultHost("nonexistent", "192.168.1.200", "admin")
		err := registry.Update(host)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("update with nil host", func(t *testing.T) {
		err := registry.Update(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "host cannot be nil")
	})

	t.Run("update with invalid host", func(t *testing.T) {
		invalid := &Host{
			Hostname: "server1",
			Address:  "",
			Port:     22,
			User:     "admin",
		}
		err := registry.Update(invalid)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid host")
	})
}

func TestMemoryRegistry_Exists(t *testing.T) {
	registry := NewMemoryRegistry()
	host := DefaultHost("server1", "192.168.1.100", "admin")
	_ = registry.Add(host)

	t.Run("exists - true", func(t *testing.T) {
		exists, err := registry.Exists("server1")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("exists - false", func(t *testing.T) {
		exists, err := registry.Exists("nonexistent")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("exists with empty hostname", func(t *testing.T) {
		_, err := registry.Exists("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "hostname cannot be empty")
	})
}

func TestMemoryRegistry_Count(t *testing.T) {
	registry := NewMemoryRegistry()

	assert.Equal(t, 0, registry.Count())

	_ = registry.Add(DefaultHost("server1", "192.168.1.100", "admin"))
	assert.Equal(t, 1, registry.Count())

	_ = registry.Add(DefaultHost("server2", "192.168.1.101", "admin"))
	assert.Equal(t, 2, registry.Count())

	_ = registry.Remove("server1")
	assert.Equal(t, 1, registry.Count())

	registry.Clear()
	assert.Equal(t, 0, registry.Count())
}

func TestMemoryRegistry_Clear(t *testing.T) {
	registry := NewMemoryRegistry()

	_ = registry.Add(DefaultHost("server1", "192.168.1.100", "admin"))
	_ = registry.Add(DefaultHost("server2", "192.168.1.101", "admin"))
	assert.Equal(t, 2, registry.Count())

	registry.Clear()
	assert.Equal(t, 0, registry.Count())

	hosts, err := registry.List()
	require.NoError(t, err)
	assert.Empty(t, hosts)
}

func TestMemoryRegistry_ThreadSafety(t *testing.T) {
	registry := NewMemoryRegistry()
	var wg sync.WaitGroup

	// Number of concurrent operations
	numOps := 100

	// Concurrent adds
	wg.Add(numOps)
	for i := 0; i < numOps; i++ {
		go func(n int) {
			defer wg.Done()
			host := DefaultHost(
				fmt.Sprintf("server%d", n),
				fmt.Sprintf("192.168.1.%d", n),
				"admin",
			)
			_ = registry.Add(host)
		}(i)
	}
	wg.Wait()

	// Should have added all hosts
	assert.Equal(t, numOps, registry.Count())

	// Concurrent reads
	wg.Add(numOps)
	for i := 0; i < numOps; i++ {
		go func(n int) {
			defer wg.Done()
			_, _ = registry.Get(fmt.Sprintf("server%d", n))
			_, _ = registry.List()
			_, _ = registry.Exists(fmt.Sprintf("server%d", n))
		}(i)
	}
	wg.Wait()

	// Concurrent updates and reads
	wg.Add(numOps * 2)
	for i := 0; i < numOps; i++ {
		// Update
		go func(n int) {
			defer wg.Done()
			host := DefaultHost(
				fmt.Sprintf("server%d", n),
				fmt.Sprintf("10.0.0.%d", n),
				"root",
			)
			_ = registry.Update(host)
		}(i)

		// Read
		go func(n int) {
			defer wg.Done()
			_, _ = registry.Get(fmt.Sprintf("server%d", n))
		}(i)
	}
	wg.Wait()

	// Should still have all hosts
	assert.Equal(t, numOps, registry.Count())
}

func TestHostRegistry_Interface(t *testing.T) {
	// Verify that MemoryRegistry implements HostRegistry interface
	var _ HostRegistry = (*MemoryRegistry)(nil)
}
