package ssh

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// keepAliveInterval is how often a keepalive request is sent on an idle
// connection. Without keepalives a host that disappears without closing its
// TCP connection (a reboot, a pulled cable) leaves callers blocked forever.
const keepAliveInterval = 15 * time.Second

// Connect establishes an SSH connection to a remote host
func Connect(opts *ConnectionOptions) (*Connection, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid connection options: %w", err)
	}

	timeout := time.Duration(opts.Timeout) * time.Second
	if opts.Timeout == 0 {
		timeout = 30 * time.Second
	}

	client, err := dial(opts, timeout)
	if err != nil {
		return nil, err
	}

	conn := &Connection{
		Host:       opts.Host,
		Port:       opts.Port,
		User:       opts.User,
		AuthMethod: opts.AuthMethod,
		timeout:    timeout,
		client:     client,
	}
	conn.startKeepAlive()

	return conn, nil
}

// dial opens one SSH client with TCP keepalives enabled.
func dial(opts *ConnectionOptions, timeout time.Duration) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User: opts.User,
		Auth: []ssh.AuthMethod{opts.AuthMethod},
		// TODO: In production, this should verify host keys
		// For now, we accept any host key (insecure but functional)
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	// TCP keepalives let the kernel fail the socket when the peer vanishes
	// without sending FIN or RST, which is what a reboot looks like.
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: keepAliveInterval}
	netConn, err := dialer.Dial("tcp", opts.Address())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", opts.Address(), err)
	}

	clientConn, channels, requests, err := ssh.NewClientConn(netConn, opts.Address(), config)
	if err != nil {
		netConn.Close()
		return nil, fmt.Errorf("failed to connect to %s: %w", opts.Address(), err)
	}

	return ssh.NewClient(clientConn, channels, requests), nil
}

// startKeepAlive sends periodic keepalive requests. When the peer stops
// answering, the client is closed so blocked and future calls fail with an
// error instead of waiting on a connection that is never coming back.
func (c *Connection) startKeepAlive() {
	done := make(chan struct{})
	client := c.client
	c.keepAliveDone = done

	go func() {
		ticker := time.NewTicker(keepAliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
					client.Close()
					return
				}
			}
		}
	}()
}

// stopKeepAlive halts the keepalive goroutine. Safe to call more than once.
func (c *Connection) stopKeepAlive() {
	if c.keepAliveDone != nil {
		close(c.keepAliveDone)
		c.keepAliveDone = nil
	}
}

// Close closes the SSH connection
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return fmt.Errorf("connection is not established")
	}
	c.stopKeepAlive()
	return c.client.Close()
}

// Reconnect re-establishes the connection to the same host, replacing the
// existing client. Use it when a host has rebooted or the connection dropped.
func (c *Connection) Reconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stopKeepAlive()
	if c.client != nil {
		_ = c.client.Close()
		c.client = nil
	}

	timeout := c.timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	opts := &ConnectionOptions{
		Host:       c.Host,
		Port:       c.Port,
		User:       c.User,
		AuthMethod: c.AuthMethod,
		Timeout:    int(timeout / time.Second),
	}

	client, err := dial(opts, timeout)
	if err != nil {
		return err
	}
	c.client = client
	c.timeout = timeout
	c.startKeepAlive()
	return nil
}

// WaitForReconnect retries Reconnect until it succeeds or maxWait elapses.
// A host that is rebooting refuses connections until sshd is back, so the
// first attempts are expected to fail.
func (c *Connection) WaitForReconnect(maxWait time.Duration, retryDelay time.Duration) error {
	if retryDelay <= 0 {
		retryDelay = 5 * time.Second
	}

	deadline := time.Now().Add(maxWait)
	var lastErr error
	for {
		if err := c.Reconnect(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("host did not come back within %v: %w", maxWait, lastErr)
		}
		time.Sleep(retryDelay)
	}
}

// IsConnected checks if the connection is still active
func (c *Connection) IsConnected() bool {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	if client == nil {
		return false
	}

	// Try to create a new session to verify the connection is alive
	session, err := client.NewSession()
	if err != nil {
		return false
	}
	session.Close()

	return true
}

// Client returns the underlying SSH client
// This is useful for advanced operations that aren't wrapped by our Connection type
func (c *Connection) Client() *ssh.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}
