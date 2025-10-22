package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestConnect_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		opts    *ConnectionOptions
		wantErr string
	}{
		{
			name: "nil options",
			opts: &ConnectionOptions{
				Host:       "",
				Port:       22,
				User:       "test",
				AuthMethod: ssh.Password("test"),
			},
			wantErr: "host cannot be empty",
		},
		{
			name: "invalid port",
			opts: &ConnectionOptions{
				Host:       "example.com",
				Port:       0,
				User:       "test",
				AuthMethod: ssh.Password("test"),
			},
			wantErr: "port must be between 1 and 65535",
		},
		{
			name: "empty user",
			opts: &ConnectionOptions{
				Host:       "example.com",
				Port:       22,
				User:       "",
				AuthMethod: ssh.Password("test"),
			},
			wantErr: "user cannot be empty",
		},
		{
			name: "nil auth method",
			opts: &ConnectionOptions{
				Host:       "example.com",
				Port:       22,
				User:       "test",
				AuthMethod: nil,
			},
			wantErr: "auth method cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Connect(tt.opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestConnect_NetworkErrors(t *testing.T) {
	t.Run("connection refused", func(t *testing.T) {
		opts := &ConnectionOptions{
			Host:       "127.0.0.1",
			Port:       12345, // Random port that should not be listening
			User:       "test",
			AuthMethod: ssh.Password("test"),
			Timeout:    1, // Short timeout
		}

		_, err := Connect(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to connect")
	})

	t.Run("invalid host", func(t *testing.T) {
		opts := &ConnectionOptions{
			Host:       "invalid.host.that.does.not.exist.example.com",
			Port:       22,
			User:       "test",
			AuthMethod: ssh.Password("test"),
			Timeout:    1,
		}

		_, err := Connect(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to connect")
	})
}

func TestConnection_Close(t *testing.T) {
	t.Run("close nil client", func(t *testing.T) {
		conn := &Connection{
			Host:   "example.com",
			Port:   22,
			User:   "test",
			client: nil,
		}

		err := conn.Close()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connection is not established")
	})
}

func TestConnection_IsConnected(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		conn := &Connection{
			Host:   "example.com",
			Port:   22,
			User:   "test",
			client: nil,
		}

		assert.False(t, conn.IsConnected())
	})
}

func TestConnection_Client(t *testing.T) {
	conn := &Connection{
		Host:   "example.com",
		Port:   22,
		User:   "test",
		client: nil,
	}

	client := conn.Client()
	assert.Nil(t, client)
}
