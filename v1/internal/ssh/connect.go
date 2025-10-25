package ssh

import (
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

// Connect establishes an SSH connection to a remote host
func Connect(opts *ConnectionOptions) (*Connection, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid connection options: %w", err)
	}

	// Create SSH client config
	config := &ssh.ClientConfig{
		User: opts.User,
		Auth: []ssh.AuthMethod{opts.AuthMethod},
		// TODO: In production, this should verify host keys
		// For now, we accept any host key (insecure but functional)
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(opts.Timeout) * time.Second,
	}

	// If timeout is 0, use default
	if opts.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	// Connect to the SSH server
	client, err := ssh.Dial("tcp", opts.Address(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", opts.Address(), err)
	}

	conn := &Connection{
		Host:       opts.Host,
		Port:       opts.Port,
		User:       opts.User,
		AuthMethod: opts.AuthMethod,
		client:     client,
	}

	return conn, nil
}

// Close closes the SSH connection
func (c *Connection) Close() error {
	if c.client == nil {
		return fmt.Errorf("connection is not established")
	}
	return c.client.Close()
}

// IsConnected checks if the connection is still active
func (c *Connection) IsConnected() bool {
	if c.client == nil {
		return false
	}

	// Try to create a new session to verify the connection is alive
	session, err := c.client.NewSession()
	if err != nil {
		return false
	}
	session.Close()

	return true
}

// Client returns the underlying SSH client
// This is useful for advanced operations that aren't wrapped by our Connection type
func (c *Connection) Client() *ssh.Client {
	return c.client
}
