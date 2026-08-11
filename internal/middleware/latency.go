package middleware

import (
	"strings"
	"sync"
	"time"
)

// LatencyRecorder keeps the last observed request latency per normalized
// route path, surfaced on the root endpoint as
// {"ms": {"/": 1, "/status": 0, "/truemoney": 342}, "message": ...}.
// Path keys are stable across requests (RouteKey), so voucher codes and
// phone numbers never appear in the map.
type LatencyRecorder struct {
	mu sync.Mutex
	ms map[string]int64 // path -> last latency in ms (0 until first hit)
}

// NewLatencyRecorder builds an empty recorder.
func NewLatencyRecorder() *LatencyRecorder {
	return &LatencyRecorder{ms: make(map[string]int64, 4)}
}

// Record stores the last latency of a request for the given raw path.
func (r *LatencyRecorder) Record(path string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ms[RouteKey(path)] = d.Milliseconds()
}

// Snapshot returns a copy safe for concurrent readers.
func (r *LatencyRecorder) Snapshot() map[string]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int64, len(r.ms))
	for k, v := range r.ms {
		out[k] = v
	}
	return out
}

// RouteKey normalizes a raw request path to a stable root key: the
// redeem path (whose segments carry a cash-equivalent code and a phone
// number) collapses to "/truemoney"; every other path stays as-is
// (including unknown paths, so 404s also appear in the map).
func RouteKey(path string) string {
	const prefix = "/truemoney/"
	if strings.HasPrefix(path, prefix) {
		return strings.TrimSuffix(prefix, "/")
	}
	return path
}