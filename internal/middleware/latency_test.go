package middleware

import (
	"testing"
	"time"
)

func TestLatencyRecorder(t *testing.T) {
	rec := NewLatencyRecorder()

	rec.Record("/status", 2*time.Millisecond)
	rec.Record("/truemoney/ABCD1234EFGH/0812345678", 345*time.Millisecond)

	snap := rec.Snapshot()
	if snap["/status"] != 2 {
		t.Fatalf("wrong /status ms: %d", snap["/status"])
	}
	if snap["/truemoney"] != 345 {
		t.Fatalf("redeem path must normalize to /truemoney, got %+v", snap)
	}
	if got := len(snap); got != 2 {
		t.Fatalf("unexpected extra keys: %+v", snap)
	}

	// Later requests overwrite the last value.
	rec.Record("/status", time.Millisecond)
	if snap := rec.Snapshot(); snap["/status"] != 1 {
		t.Fatalf("record must keep the last value, got %d", snap["/status"])
	}
}

func TestRouteKey(t *testing.T) {
	cases := map[string]string{
		"/":                                    "/",
		"/status":                              "/status",
		"/truemoney/CODE123/0812345678":        "/truemoney",
		"/truemoney/https%3A%2F%2F.../0812345678": "/truemoney",
		"/nope":                                "/nope",
	}
	for in, want := range cases {
		if got := RouteKey(in); got != want {
			t.Errorf("RouteKey(%q) = %q, want %q", in, got, want)
		}
	}
}