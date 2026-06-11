package backup

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestReverseTunnel_CloseDoesNotHang reproduces the teardown hang: a forwarded
// connection that Velero leaves open must not block Close(). Close() must
// force-close tracked connections and return promptly.
func TestReverseTunnel_CloseDoesNotHang(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	rt := &reverseTunnel{
		listener: ln,
		stop:     make(chan struct{}),
		conns:    make(map[net.Conn]struct{}),
	}

	// Simulate an in-flight forward stuck in io.Copy (blocked Read) like a
	// connection Velero hasn't closed.
	c1, c2 := net.Pipe()
	rt.track(c1)
	rt.track(c2)
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		buf := make([]byte, 1)
		_, _ = c1.Read(buf) // unblocks only when c1 is closed by Close()
	}()

	done := make(chan error, 1)
	go func() { done <- rt.Close() }()

	select {
	case <-done:
		// success: Close returned without hanging
	case <-time.After(8 * time.Second):
		t.Fatal("Close() hung on an open forwarded connection")
	}
	_ = c2.Close()
}
