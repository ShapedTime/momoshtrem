package swarm

import (
	"bytes"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// echoServer echoes every connection back on loopback until the test ends.
func echoServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	t.Cleanup(func() { ln.Close(); wg.Wait() })
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer c.Close()
				io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().String()
}

func startProxy(t *testing.T, target string, cfg ProxyConfig) *Proxy {
	t.Helper()
	p, err := StartProxy(target, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

// WaitStats polls p's stats until ok holds or timeout passes, and returns
// the last. The proxy counts bytes after it has written them, and a
// refused connection after it has closed it, so a client can be done
// before the counters catch up. It's exported (in a test file) for the
// swarm_test package's tests.
func WaitStats(t *testing.T, p *Proxy, ok func(ProxyStats) bool, timeout time.Duration) ProxyStats {
	t.Helper()
	deadline := time.Now().Add(timeout)
	st := p.Stats()
	for !ok(st) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		st = p.Stats()
	}
	return st
}

// roundTrip sends n bytes through c and reads them back, checking them.
func roundTrip(t *testing.T, c net.Conn, n int) error {
	t.Helper()
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(i * 31)
	}
	errc := make(chan error, 1)
	go func() {
		_, err := c.Write(out)
		errc <- err
	}()
	in := make([]byte, n)
	if _, err := io.ReadFull(c, in); err != nil {
		return err
	}
	if err := <-errc; err != nil {
		return err
	}
	if !bytes.Equal(in, out) {
		t.Fatal("the bytes came back changed")
	}
	return nil
}

func TestProxy_Forwards(t *testing.T) {
	p := startProxy(t, echoServer(t), ProxyConfig{Jitter: 2 * time.Millisecond, Seed: 1})
	c, err := net.Dial("tcp", p.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// Jitter mustn't reorder: 1 MiB in many segments comes back intact.
	if err := roundTrip(t, c, 1<<20); err != nil {
		t.Fatal(err)
	}
	want := func(st ProxyStats) bool { return st.Accepted == 1 && st.Up == 1<<20 && st.Down == 1<<20 }
	if st := WaitStats(t, p, want, 2*time.Second); !want(st) {
		t.Errorf("stats = %+v", st)
	}
}

// Latency and the bandwidth cap slow traffic down at least as much as
// they say. Floors only: a loaded machine only takes longer.
func TestProxy_Shapes(t *testing.T) {
	const latency = 50 * time.Millisecond
	p := startProxy(t, echoServer(t), ProxyConfig{Latency: latency})
	c, err := net.Dial("tcp", p.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	start := time.Now()
	if err := roundTrip(t, c, 100); err != nil {
		t.Fatal(err)
	}
	if rtt := time.Since(start); rtt < 2*latency {
		t.Errorf("round trip %v, want at least %v", rtt, 2*latency)
	}

	const bps = 512 << 10
	q := startProxy(t, echoServer(t), ProxyConfig{Rate: bps})
	d, err := net.Dial("tcp", q.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	n := 512 << 10
	start = time.Now()
	if err := roundTrip(t, d, n); err != nil {
		t.Fatal(err)
	}
	if floor := time.Duration(float64(n-bps/10) / bps * 0.8 * float64(time.Second)); time.Since(start) < floor {
		t.Errorf("%d bytes took %v at %d B/s", n, time.Since(start), bps)
	}
}

// Drop cuts the connections; a new one goes through. Vanish refuses new
// ones until it's over.
func TestProxy_DropAndVanish(t *testing.T) {
	p := startProxy(t, echoServer(t), ProxyConfig{})
	c, err := net.Dial("tcp", p.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := roundTrip(t, c, 1000); err != nil {
		t.Fatal(err)
	}
	if n := p.Drop(); n != 1 {
		t.Errorf("Drop cut %d connections, want 1", n)
	}
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Error("a dropped connection still reads")
	}

	d, err := net.Dial("tcp", p.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := roundTrip(t, d, 1000); err != nil {
		t.Fatalf("after a drop: %v", err)
	}

	const away = 300 * time.Millisecond
	p.Vanish(away)
	if !p.Gone() {
		t.Error("not gone after Vanish")
	}
	e, err := net.Dial("tcp", p.Addr())
	if err == nil {
		e.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, err := e.Read(make([]byte, 1)); err == nil {
			t.Error("a connection to a vanished target reads")
		}
		e.Close()
	}
	for p.Gone() {
		time.Sleep(10 * time.Millisecond)
	}
	f, err := net.Dial("tcp", p.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := roundTrip(t, f, 1000); err != nil {
		t.Fatalf("after the target came back: %v", err)
	}
	want := func(st ProxyStats) bool { return st.Dropped == 2 && st.Refused == 1 && st.Accepted == 3 }
	if st := WaitStats(t, p, want, 2*time.Second); !want(st) {
		t.Errorf("stats = %+v, want 2 dropped, 1 refused, 3 accepted", st)
	}
}
