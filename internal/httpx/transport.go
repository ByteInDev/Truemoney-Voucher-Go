// Package httpx provides an http.RoundTripper that mimics a real browser
// at the TLS and HTTP/2 wire level. This is what allows requests through
// Cloudflare bot detection: uTLS Firefox 148 fingerprinting defeats
// JA3/JA4 detection, and the custom HTTP/2 framer sends browser-matching
// SETTINGS, HPACK and fixed header ordering (Go's header map iterates
// randomly, so headers are encoded manually on the wire).
//
// Connections are kept in a small idle pool and reused (like a browser
// keep-alive), which turns the ~120 ms TCP+TLS+HTTP/2 handshake cost into
// a one-time charge per connection. Idle and total-age limits recycle
// connections so a single fingerprint is never held open too long.
package httpx

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
)

// Pool limits: how many idle connections to keep per host, how long an
// idle connection may sit, and how old a connection may grow before it is
// recycled.
const (
	maxIdleConns = 4
	idleTimeout  = 30 * time.Second
	maxConnAge   = 60 * time.Second
)

// Transport is an http.RoundTripper that speaks HTTP/2 like a browser.
// Plain HTTP requests (e.g. tests) fall back to Go's default transport.
type Transport struct {
	dialer *net.Dialer

	// TLSConfig optionally overrides the TLS client configuration (root
	// CAs, etc.); ServerName is always derived from the request host.
	// Exporting this keeps dialH2 testable against local servers with
	// self-signed certificates; production uses system roots.
	TLSConfig *utls.Config

	// sessions is a shared TLS session cache so fresh connections can
	// resume with a PSK, exactly like a browser reopening its tabs.
	sessions utls.ClientSessionCache

	poolMu sync.Mutex
	idle   map[string][]*h2conn // host -> idle connections
}

// NewTransport builds a Transport with sane dial timeouts.
func NewTransport() *Transport {
	return &Transport{
		dialer: &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		},
		sessions: utls.NewLRUClientSessionCache(8),
		idle:     make(map[string][]*h2conn),
	}
}

// RoundTrip executes a single HTTP request/response over HTTP/2 using
// the browser fingerprint. An idle connection is reused when available;
// a request that fails on a stale pooled connection is retried once on a
// freshly dialed one.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return http.DefaultTransport.RoundTrip(req)
	}

	port := req.URL.Port()
	if port == "" {
		port = "443"
	}
	host := net.JoinHostPort(req.URL.Hostname(), port)

	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	h2, pooled := t.takeIdle(host)
	if h2 == nil {
		var err error
		h2, err = t.dial(ctx, host)
		if err != nil {
			return nil, err
		}
	}

	resp, err := h2.do(req, firefoxHeaderOrder)
	if err != nil {
		h2.Close()
		if !pooled || ctx.Err() != nil {
			return nil, err
		}
		// The pooled connection went stale (server closed it, GOAWAY,
		// etc.): retry once on a fresh connection.
		again, dialErr := t.dial(ctx, host)
		if dialErr != nil {
			return nil, err
		}
		resp, err = again.do(req, firefoxHeaderOrder)
		if err != nil {
			again.Close()
			return nil, err
		}
		h2 = again
	}

	if err == nil && h2.writeErr == nil {
		t.putIdle(host, h2)
	} else {
		h2.Close()
	}
	return resp, err
}

// dial opens a new connection with TLS session resumption enabled.
func (t *Transport) dial(ctx context.Context, host string) (*h2conn, error) {
	cfg := t.TLSConfig
	var clone *utls.Config
	if cfg != nil {
		clone = cfg.Clone()
	} else {
		clone = &utls.Config{}
	}
	clone.ClientSessionCache = t.sessions
	return dialH2(ctx, host, t.dialer, clone)
}

// takeIdle pops the freshest usable idle connection for host, discarding
// any that have expired. Returns nil when the pool is empty.
func (t *Transport) takeIdle(host string) (*h2conn, bool) {
	t.poolMu.Lock()
	defer t.poolMu.Unlock()

	conns := t.idle[host]
	now := time.Now()
	for len(conns) > 0 {
		// LIFO: the most recently used connection is the freshest.
		c := conns[len(conns)-1]
		conns = conns[:len(conns)-1]
		if c.writeErr != nil || now.Sub(c.lastUsed) > idleTimeout || now.Sub(c.created) > maxConnAge {
			c.Close()
			continue
		}
		t.idle[host] = conns
		return c, true
	}
	delete(t.idle, host)
	return nil, false
}

// putIdle returns a healthy connection to the pool, closing the oldest
// one when the pool is full.
func (t *Transport) putIdle(host string, h2 *h2conn) {
	t.poolMu.Lock()
	defer t.poolMu.Unlock()

	now := time.Now()
	conns := t.idle[host]
	// Drop expired connections first.
	kept := conns[:0]
	for _, c := range conns {
		if now.Sub(c.lastUsed) <= idleTimeout && now.Sub(c.created) <= maxConnAge && c.writeErr == nil {
			kept = append(kept, c)
		} else {
			c.Close()
		}
	}
	if len(kept) >= maxIdleConns {
		kept[0].Close()
		kept = kept[1:]
	}
	h2.lastUsed = now
	t.idle[host] = append(kept, h2)
}