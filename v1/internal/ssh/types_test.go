package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
)

func TestConnectionOptions_Validate(t *testing.T) {
	validAuth := ssh.Password("test")

	tests := []struct {
		name    string
		opts    *ConnectionOptions
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid options",
			opts: &ConnectionOptions{
				Host:       "example.com",
				Port:       22,
				User:       "testuser",
				AuthMethod: validAuth,
				Timeout:    30,
			},
			wantErr: false,
		},
		{
			name: "valid options with custom port",
			opts: &ConnectionOptions{
				Host:       "192.168.1.100",
				Port:       2222,
				User:       "admin",
				AuthMethod: validAuth,
				Timeout:    60,
			},
			wantErr: false,
		},
		{
			name: "empty host",
			opts: &ConnectionOptions{
				Host:       "",
				Port:       22,
				User:       "testuser",
				AuthMethod: validAuth,
				Timeout:    30,
			},
			wantErr: true,
			errMsg:  "host cannot be empty",
		},
		{
			name: "invalid port - zero",
			opts: &ConnectionOptions{
				Host:       "example.com",
				Port:       0,
				User:       "testuser",
				AuthMethod: validAuth,
				Timeout:    30,
			},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name: "invalid port - too high",
			opts: &ConnectionOptions{
				Host:       "example.com",
				Port:       70000,
				User:       "testuser",
				AuthMethod: validAuth,
				Timeout:    30,
			},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name: "empty user",
			opts: &ConnectionOptions{
				Host:       "example.com",
				Port:       22,
				User:       "",
				AuthMethod: validAuth,
				Timeout:    30,
			},
			wantErr: true,
			errMsg:  "user cannot be empty",
		},
		{
			name: "nil auth method",
			opts: &ConnectionOptions{
				Host:       "example.com",
				Port:       22,
				User:       "testuser",
				AuthMethod: nil,
				Timeout:    30,
			},
			wantErr: true,
			errMsg:  "auth method cannot be nil",
		},
		{
			name: "negative timeout",
			opts: &ConnectionOptions{
				Host:       "example.com",
				Port:       22,
				User:       "testuser",
				AuthMethod: validAuth,
				Timeout:    -1,
			},
			wantErr: true,
			errMsg:  "timeout cannot be negative",
		},
		{
			name: "zero timeout is valid",
			opts: &ConnectionOptions{
				Host:       "example.com",
				Port:       22,
				User:       "testuser",
				AuthMethod: validAuth,
				Timeout:    0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConnectionOptions_Address(t *testing.T) {
	tests := []struct {
		name string
		opts *ConnectionOptions
		want string
	}{
		{
			name: "default port",
			opts: &ConnectionOptions{
				Host: "example.com",
				Port: 22,
			},
			want: "example.com:22",
		},
		{
			name: "custom port",
			opts: &ConnectionOptions{
				Host: "192.168.1.100",
				Port: 2222,
			},
			want: "192.168.1.100:2222",
		},
		{
			name: "IPv6 address",
			opts: &ConnectionOptions{
				Host: "::1",
				Port: 22,
			},
			want: "[::1]:22",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opts.Address()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultConnectionOptions(t *testing.T) {
	auth := ssh.Password("test")
	opts := DefaultConnectionOptions("example.com", "testuser", auth)

	assert.Equal(t, "example.com", opts.Host)
	assert.Equal(t, 22, opts.Port)
	assert.Equal(t, "testuser", opts.User)
	assert.NotNil(t, opts.AuthMethod)
	assert.Equal(t, 30, opts.Timeout)

	// Verify it passes validation
	assert.NoError(t, opts.Validate())
}
