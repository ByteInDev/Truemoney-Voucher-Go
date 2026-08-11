package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"truemoney-voucher/internal/config"
	"truemoney-voucher/internal/truemoney"
)

func newTestRouter() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(&config.Config{}, logger, truemoney.NewClient())
}

func TestStatusIsLivenessProbe(t *testing.T) {
	h := newTestRouter()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestInvalidInputAnswers400Envelope(t *testing.T) {
	h := newTestRouter()
	// "AB CD" (URL-encoded space) fails the charset check locally, so
	// this never touches the network.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/truemoney/AB%20CD/0812345678", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("envelope status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"code":400`) {
		t.Fatalf("body = %s, want 400 envelope", rec.Body.String())
	}
}

func TestRootReportsMsPerPath(t *testing.T) {
	h := newTestRouter()

	for _, path := range []string{"/status", "/truemoney/AB%20CD/0812345678"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("warm-up %s failed: %d", path, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("root = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"message":"ByteInDev Service"`) {
		t.Fatalf("root missing message: %s", body)
	}
	for _, key := range []string{`"/status"`, `"/truemoney"`} {
		if !strings.Contains(body, key) {
			t.Fatalf("root missing %s: %s", key, body)
		}
	}
	// Latency values must be integer milliseconds.
	num := regexp.MustCompile(`"(?:/status|/truemoney)":\s*(\d+)`)
	matches := num.FindAllStringSubmatch(body, -1)
	if len(matches) != 2 {
		t.Fatalf("expected integer ms values, got: %s", body)
	}
	if strings.Contains(body, `"/truemoney/`) {
		t.Fatalf("redeem path must normalize, no code in root: %s", body)
	}
}

func TestUnknownPathIsJSON404(t *testing.T) {
	h := newTestRouter()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"code":404`) {
		t.Fatalf("body = %s, want 404 envelope", rec.Body.String())
	}
}

// End-to-end router benchmarks: the local per-request overhead a client
// sees on top of the upstream redeem (µs scale vs hundreds of ms upstream).
func serveLoop(b *testing.B, path string) {
	h := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
	}
}

func BenchmarkRouterStatus(b *testing.B) { serveLoop(b, "/status") }

func BenchmarkRouterRoot(b *testing.B) { serveLoop(b, "/") }