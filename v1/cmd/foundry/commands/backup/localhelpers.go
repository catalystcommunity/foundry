package backup

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

func itoa(i int) string { return strconv.Itoa(i) }

func stringsReader(s string) io.Reader { return strings.NewReader(s) }

// ensurePortFree returns an error if something is already listening on host:port.
func ensurePortFree(host string, port int) error {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("port %d on %s is already in use (a stale 'weed' from a previous run? check `ps aux | grep weed`); free it or pass --port: %w", port, host, err)
	}
	_ = ln.Close()
	return nil
}

// waitForPort blocks until host:port accepts a TCP connection or the timeout/ctx
// elapses.
func waitForPort(ctx context.Context, host string, port int, timeout time.Duration) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", addr)
		}
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
}
