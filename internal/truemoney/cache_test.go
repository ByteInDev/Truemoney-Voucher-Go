package truemoney

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCacheHitAndMiss(t *testing.T) {
	c := newRedeemCache(time.Minute, 1024)
	key := cacheKey("ABC123", "0812345678")
	body := json.RawMessage(`{"status":{"code":"SUCCESS"}}`)

	if _, ok := c.get(key); ok {
		t.Fatal("empty cache must miss")
	}
	c.put(key, body)
	got, ok := c.get(key)
	if !ok {
		t.Fatal("expected hit after put")
	}
	if string(got) != string(body) {
		t.Fatalf("wrong body: %s", got)
	}
}

func TestCacheExpiry(t *testing.T) {
	c := newRedeemCache(50*time.Millisecond, 1024)
	key := cacheKey("ABC123", "0812345678")
	c.put(key, json.RawMessage(`{"status":{"code":"SUCCESS"}}`))

	if _, ok := c.get(key); !ok {
		t.Fatal("expected hit before expiry")
	}
	time.Sleep(60 * time.Millisecond)
	if _, ok := c.get(key); ok {
		t.Fatal("expired entry must miss")
	}
}

func TestCacheDisabled(t *testing.T) {
	c := newRedeemCache(0, 1024)
	key := cacheKey("ABC123", "0812345678")
	c.put(key, json.RawMessage(`{"status":{"code":"SUCCESS"}}`))
	if _, ok := c.get(key); ok {
		t.Fatal("ttl<=0 must never hit")
	}
}

func TestCacheLRUEviction(t *testing.T) {
	c := newRedeemCache(time.Minute, 2)

	// keys: A used, B used, C used -> A evicted.
	a, b, cc := cacheKey("AAAA", "1"), cacheKey("BBBB", "1"), cacheKey("CCCC", "1")
	c.put(a, json.RawMessage(`{"a":1}`))
	c.put(b, json.RawMessage(`{"b":1}`))
	if _, ok := c.get(a); !ok {
		t.Fatal("expected hit for A")
	}
	c.put(cc, json.RawMessage(`{"c":1}`)) // evicts B, not A (A was touched)

	if _, ok := c.get(a); !ok {
		t.Fatal("A must survive LRU eviction (most recently used)")
	}
	if _, ok := c.get(b); ok {
		t.Fatal("B must be evicted (least recently used)")
	}
	if _, ok := c.get(cc); !ok {
		t.Fatal("expected hit for C")
	}
}

func TestCacheRefreshKeepsFresh(t *testing.T) {
	c := newRedeemCache(time.Minute, 2)
	key := cacheKey("ABC123", "0812345678")
	body := json.RawMessage(`{"status":{"code":"SUCCESS"}}`)
	c.put(key, body)

	// Re-putting the same key refreshes the timestamp instead of adding
	// a duplicate entry.
	for i := 0; i < 5; i++ {
		c.put(key, body)
	}
	if c.lru.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", c.lru.Len())
	}
}