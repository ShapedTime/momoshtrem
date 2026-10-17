package swarm

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// proxySegment is the most a proxy reads at once: one BitTorrent chunk.
const proxySegment = 16 << 10

// proxyQueue is how many segments a direction holds in flight, delayed by
// the latency: with the latency, it bounds the bytes in flight.
const proxyQueue = 256

// ProxyConfig shapes a proxy's traffic. The zero value forwards as is.
type ProxyConfig struct {
	// Latency delays every byte, each way. Jitter adds up to that much more
	// at random, with the bytes kept in order, as TCP does.
	Latency, Jitter time.Duration
	// Rate caps each direction, in bytes per second, over all the proxy's
	// connections: a link's bandwidth. 0 is unlimited.
	Rate int64
	// Seed seeds the jitter.
	Seed uint64
}

// ProxyStats is what a proxy did.
type ProxyStats struct {
	// Accepted counts the connections it forwarded, Refused those it
	// closed at once while the target was gone (Vanish), and Dropped those
	// it cut (Drop, Vanish).
	Accepted, Refused, Dropped int
	// Down is what it forwarded from the target, Up what it forwarded to
	// it.
	Down, Up int64
}

// Proxy is an in-process TCP proxy on loopback, between a leecher and a
// seeder: the leecher dials the proxy (its address as the magnet's x.pe)
// and the proxy dials the seeder. It adds latency and jitter, caps the
// bandwidth, and can cut its connections (Drop) or make the seeder vanish
// for a while (Vanish).
type Proxy struct {
	cfg      ProxyConfig
	target   string
	ln       net.Listener
	down, up *rate.Limiter // nil: unlimited
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	mu        sync.Mutex
	rng       *rand.Rand
	conns     map[*proxyConn]struct{}
	goneUntil time.Time
	stats     ProxyStats
	// Down and Up count as bytes pass, without the lock.
	downBytes, upBytes atomic.Int64
}

// StartProxy starts a proxy to target (host:port) on 127.0.0.1 on a free
// port.
func StartProxy(target string, cfg ProxyConfig) (*Proxy, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort(loopbackHost, "0"))
	if err != nil {
		return nil, fmt.Errorf("swarm: proxy: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Proxy{
		cfg:    cfg,
		target: target,
		ln:     ln,
		down:   bandwidth(cfg.Rate),
		up:     bandwidth(cfg.Rate),
		ctx:    ctx,
		cancel: cancel,
		rng:    rand.New(rand.NewPCG(cfg.Seed, 0x9e3779b97f4a7c15)),
		conns:  map[*proxyConn]struct{}{},
	}
	p.wg.Add(1)
	go p.accept()
	return p, nil
}

// bandwidth is a limiter of bps bytes per second; nil for 0.
func bandwidth(bps int64) *rate.Limiter {
	if bps <= 0 {
		return nil
	}
	return rate.NewLimiter(rate.Limit(bps), int(max(proxySegment, bps/10)))
}

// Addr is the proxy's address, 127.0.0.1:port: what the leecher dials.
func (p *Proxy) Addr() string { return p.ln.Addr().String() }

// Target is the address the proxy forwards to.
func (p *Proxy) Target() string { return p.target }

// Drop cuts every connection the proxy holds, both ways, as a peer whose
// connection resets does. It returns how many it cut.
func (p *Proxy) Drop() int {
	n := p.cut()
	p.mu.Lock()
	p.stats.Dropped += n
	p.mu.Unlock()
	return n
}

// cut closes every connection and returns how many. They leave the map
// at once: a later cut doesn't count them again.
func (p *Proxy) cut() int {
	p.mu.Lock()
	conns := make([]*proxyConn, 0, len(p.conns))
	for c := range p.conns {
		conns = append(conns, c)
		delete(p.conns, c)
	}
	p.mu.Unlock()
	for _, c := range conns {
		c.close()
	}
	return len(conns)
}

// Vanish makes the target go away for d: every connection is cut, and
// the ones made meanwhile are closed at once. It returns how many it cut.
func (p *Proxy) Vanish(d time.Duration) int {
	p.mu.Lock()
	p.goneUntil = time.Now().Add(d)
	p.mu.Unlock()
	return p.Drop()
}

// Gone reports whether the target is away (Vanish).
func (p *Proxy) Gone() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return time.Now().Before(p.goneUntil)
}

// Conns is how many connections the proxy holds.
func (p *Proxy) Conns() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.conns)
}

// Stats is what the proxy did so far.
func (p *Proxy) Stats() ProxyStats {
	p.mu.Lock()
	st := p.stats
	p.mu.Unlock()
	st.Down, st.Up = p.downBytes.Load(), p.upBytes.Load()
	return st
}

// Close stops the proxy: no more connections, and the ones it holds cut.
func (p *Proxy) Close() error {
	err := p.ln.Close()
	p.cancel()
	p.cut()
	p.wg.Wait()
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (p *Proxy) accept() {
	defer p.wg.Done()
	for {
		client, err := p.ln.Accept()
		if err != nil {
			return // closed
		}
		if p.Gone() {
			client.Close()
			p.mu.Lock()
			p.stats.Refused++
			p.mu.Unlock()
			continue
		}
		p.wg.Add(1)
		go p.serve(client)
	}
}

// serve forwards one connection until either side ends it or the proxy
// cuts it.
func (p *Proxy) serve(client net.Conn) {
	defer p.wg.Done()
	var d net.Dialer
	target, err := d.DialContext(p.ctx, "tcp", p.target)
	if err != nil {
		client.Close()
		p.mu.Lock()
		p.stats.Refused++
		p.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	c := &proxyConn{client: client, target: target, cancel: cancel}
	p.mu.Lock()
	gone := time.Now().Before(p.goneUntil)
	if !gone {
		p.conns[c] = struct{}{}
		p.stats.Accepted++
	} else {
		p.stats.Refused++
	}
	p.mu.Unlock()
	if gone {
		c.close()
		return
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); p.pipe(ctx, c, target, client, p.up, &p.upBytes) }()
	go func() { defer wg.Done(); p.pipe(ctx, c, client, target, p.down, &p.downBytes) }()
	wg.Wait()
	c.close()
	p.mu.Lock()
	delete(p.conns, c)
	p.mu.Unlock()
}

// segment is bytes read from one side, due on the other at a time.
type segment struct {
	b   []byte
	due time.Time
}

// pipe forwards src to dst, each segment after the latency and jitter and
// within the bandwidth, until src ends or the connection is cut. Bytes
// read before src ended are still delivered.
func (p *Proxy) pipe(ctx context.Context, c *proxyConn, dst, src net.Conn, lim *rate.Limiter, count *atomic.Int64) {
	q := make(chan segment, proxyQueue)
	go func() {
		defer close(q)
		var last time.Time
		for {
			b := make([]byte, proxySegment)
			n, err := src.Read(b)
			if n > 0 {
				due := time.Now().Add(p.delay())
				if due.Before(last) {
					due = last // in order
				}
				last = due
				select {
				case q <- segment{b: b[:n], due: due}:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	defer c.close() // one side done: the other can't carry on alone
	for seg := range q {
		if wait := time.Until(seg.due); wait > 0 {
			t := time.NewTimer(wait)
			select {
			case <-t.C:
			case <-ctx.Done():
				t.Stop()
				return
			}
		}
		for b := seg.b; len(b) > 0; {
			n := len(b)
			if lim != nil {
				n = min(n, lim.Burst())
				if lim.WaitN(ctx, n) != nil {
					return
				}
			}
			m, err := dst.Write(b[:n])
			count.Add(int64(m))
			if err != nil {
				return
			}
			b = b[m:]
		}
	}
}

// delay is one segment's latency: the latency plus a random jitter.
func (p *Proxy) delay() time.Duration {
	d := p.cfg.Latency
	if p.cfg.Jitter > 0 {
		p.mu.Lock()
		d += time.Duration(p.rng.Int64N(int64(p.cfg.Jitter)))
		p.mu.Unlock()
	}
	return d
}

// proxyConn is one forwarded connection: the leecher's side and the
// seeder's.
type proxyConn struct {
	client, target net.Conn
	cancel         context.CancelFunc
	once           sync.Once
}

// close cuts both sides; safe to call more than once.
func (c *proxyConn) close() {
	c.once.Do(func() {
		c.cancel()
		c.client.Close()
		c.target.Close()
	})
}
