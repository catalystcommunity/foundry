package backup

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	cryptossh "golang.org/x/crypto/ssh"
)

// reverseTunnel exposes a local TCP address on a remote host via SSH remote port
// forwarding (the `ssh -R` mechanism). It is used to make a SeaweedFS S3 endpoint
// running on the operator's machine reachable from inside the cluster at
// <nodeIP>:<port>, so Velero node-agent pods can stream File System Backup data
// to it. Requires GatewayPorts to be enabled on the node's sshd for the bind to
// land on the routable node IP (see gatewayports.go).
type reverseTunnel struct {
	client    *cryptossh.Client
	listener  net.Listener
	localAddr string
	stop      chan struct{}
	wg        sync.WaitGroup

	mu      sync.Mutex
	lastErr error

	connMu sync.Mutex
	conns  map[net.Conn]struct{}
}

func (rt *reverseTunnel) track(c net.Conn) {
	rt.connMu.Lock()
	rt.conns[c] = struct{}{}
	rt.connMu.Unlock()
}

func (rt *reverseTunnel) untrack(c net.Conn) {
	rt.connMu.Lock()
	delete(rt.conns, c)
	rt.connMu.Unlock()
}

// startReverseTunnel requests a remote listener on bindAddr (e.g. "10.16.0.31:33099")
// and forwards every accepted connection to localAddr (e.g. "127.0.0.1:33099").
func startReverseTunnel(client *cryptossh.Client, bindAddr, localAddr string) (*reverseTunnel, error) {
	listener, err := client.Listen("tcp", bindAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to open remote listener on %s (is GatewayPorts enabled on the node?): %w", bindAddr, err)
	}
	rt := &reverseTunnel{
		client:    client,
		listener:  listener,
		localAddr: localAddr,
		stop:      make(chan struct{}),
		conns:     make(map[net.Conn]struct{}),
	}
	rt.wg.Add(1)
	go rt.serve()
	go rt.keepalive()
	return rt, nil
}

func (rt *reverseTunnel) serve() {
	defer rt.wg.Done()
	for {
		remote, err := rt.listener.Accept()
		if err != nil {
			select {
			case <-rt.stop:
				return // expected: tunnel closed
			default:
				rt.setErr(fmt.Errorf("tunnel accept failed: %w", err))
				return
			}
		}
		rt.wg.Add(1)
		go rt.handle(remote)
	}
}

// handle pipes a single remote connection to the local S3 endpoint, both ways.
func (rt *reverseTunnel) handle(remote net.Conn) {
	defer rt.wg.Done()
	defer remote.Close()

	local, err := net.Dial("tcp", rt.localAddr)
	if err != nil {
		rt.setErr(fmt.Errorf("failed to dial local S3 %s: %w", rt.localAddr, err))
		return
	}
	defer local.Close()

	// Track both ends so Close() can force them shut and never block on a
	// connection Velero leaves open.
	rt.track(remote)
	rt.track(local)
	defer rt.untrack(remote)
	defer rt.untrack(local)

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(local, remote); done <- struct{}{} }()
	go func() { _, _ = io.Copy(remote, local); done <- struct{}{} }()
	<-done // first side to finish tears the pair down via the defers
}

// keepalive sends periodic SSH keepalives so a dropped connection is detected
// promptly rather than hanging a long File System Backup.
func (rt *reverseTunnel) keepalive() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-rt.stop:
			return
		case <-ticker.C:
			if _, _, err := rt.client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				rt.setErr(fmt.Errorf("ssh keepalive failed: %w", err))
				return
			}
		}
	}
}

func (rt *reverseTunnel) setErr(err error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.lastErr == nil {
		rt.lastErr = err
	}
}

// Err returns the first error the tunnel encountered, if any.
func (rt *reverseTunnel) Err() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.lastErr
}

// Close stops the tunnel: it stops accepting, force-closes any in-flight
// forwarded connections (so a connection Velero leaves open can't hang teardown),
// and waits for goroutines to drain with a bounded timeout.
func (rt *reverseTunnel) Close() error {
	select {
	case <-rt.stop:
		// already closed
	default:
		close(rt.stop)
	}
	err := rt.listener.Close()

	// Force-close active forwarded connections to unblock their io.Copy loops.
	rt.connMu.Lock()
	for c := range rt.conns {
		_ = c.Close()
	}
	rt.connMu.Unlock()

	// Bounded wait so teardown is never blocked indefinitely.
	done := make(chan struct{})
	go func() { rt.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	return err
}
