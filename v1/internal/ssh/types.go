package ssh

import (
	"fmt"
	"net"

	"golang.org/x/crypto/ssh"
)

// Connection represents an active SSH connection to a remote host
type Connection struct {
	Host       string
	Port       int
	User       string
	AuthMethod ssh.AuthMethod
	client     *ssh.Client
}

// KeyPair represents an SSH public/private key pair
type KeyPair struct {
	Private []byte
	Public  []byte
}

// ConnectionOptions contains options for establishing an SSH connection
type ConnectionOptions struct {
	Host       string
	Port       int
	User       string
	AuthMethod ssh.AuthMethod
	Timeout    int // timeout in seconds, default 30
}

// Validate validates the ConnectionOptions
func (opts *ConnectionOptions) Validate() error {
	if opts.Host == "" {
		return fmt.Errorf("host cannot be empty")
	}
	if opts.Port <= 0 || opts.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", opts.Port)
	}
	if opts.User == "" {
		return fmt.Errorf("user cannot be empty")
	}
	if opts.AuthMethod == nil {
		return fmt.Errorf("auth method cannot be nil")
	}
	if opts.Timeout < 0 {
		return fmt.Errorf("timeout cannot be negative")
	}
	return nil
}

// Address returns the host:port address string
func (opts *ConnectionOptions) Address() string {
	return net.JoinHostPort(opts.Host, fmt.Sprintf("%d", opts.Port))
}

// DefaultConnectionOptions returns ConnectionOptions with sensible defaults
func DefaultConnectionOptions(host, user string, auth ssh.AuthMethod) *ConnectionOptions {
	return &ConnectionOptions{
		Host:       host,
		Port:       22,
		User:       user,
		AuthMethod: auth,
		Timeout:    30,
	}
}
