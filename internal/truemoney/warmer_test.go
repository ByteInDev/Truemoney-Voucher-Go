package truemoney

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newProbeHarness spins a local upstream that answers probes with the exact
// TrueMoney error envelope. hits fires on every probe request received.
func newProbeHarness(t *testing.T) (*Client, *httptest.Server, <-chan struct{}) {
	t.Helper()
	hits := make(chan struct{}, 16)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("probe method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/campaign/vouchers/PROBE000000/redeem" {
			t.Errorf("probe path = %s, want invalid-code path", r.URL.Path)
		}
		select {
		case hits <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":{"code":"VOUCHER_NOT_FOUND","message":"no"},"data":null}`))
	}))

	c := NewClient()
	c.http.Transport = http.DefaultTransport // plain HTTP to the test server
	c.probeURL = srv.URL
	return c, srv, hits
}

// TestProbeOnce passes when the upstream answers a TrueMoney error
// envelope (proves probeOnce treats a real upstream error as healthy).
func TestProbeOnce(t *testing.T) {
	c, srv, _ := newProbeHarness(t)
	defer srv.Close()

	if err := c.probeOnce(context.Background()); err != nil {
		t.Fatalf("probeOnce = %v, want nil", err)
	}
}

// TestProbeOnceFailure: transport errors surface so the warmer can log them.
func TestProbeOnceFailure(t *testing.T) {
	c := NewClient()
	c.http.Transport = http.DefaultTransport
	c.probeURL = "http://127.0.0.1:1" // closed port: fails instantly, offline

	if err := c.probeOnce(context.Background()); err == nil {
		t.Fatal("probeOnce to a dead host must fail")
	}
}

// TestStartWarmerDisabled: interval <= 0 must not spawn anything.
func TestStartWarmerDisabled(t *testing.T) {
	c := NewClient()
	c.StartWarmer(context.Background(), 0, nil)
}

// TestStartWarmerTicks: the warmer probes on every tick and stops (and
// releases the goroutine) when the context is cancelled.
func TestStartWarmerTicks(t *testing.T) {
	c, srv, hits := newProbeHarness(t)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go c.StartWarmer(ctx, 1*time.Millisecond, nil)

	deadline := time.After(2 * time.Second)
	for got := 0; got < 2; {
		select {
		case <-hits:
			got++
		case <-deadline:
			t.Fatal("warmer never probed")
		}
	}
	cancel()
}