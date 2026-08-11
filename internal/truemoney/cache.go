package truemoney

import (
	"container/list"
	"crypto/sha256"
	"encoding/json"
	"sync"
	"time"
)

// redeemCache is a small in-process idempotency store for redeem results,
// keyed by the normalized (code, mobile) pair.
//
// Why it exists: a client that times out and retries would otherwise hit
// TrueMoney twice and see TARGET_USER_REDEEMED, without knowing whether the
// first attempt actually succeeded. Replaying the cached answer for the
// real result of the first attempt removes that ambiguity — the retry gets
// the exact same passthrough body (SUCCESS or an error envelope).
//
// Only successful upstream answers are cached. Transport-level failures
// (timeouts, Cloudflare challenges, non-JSON bodies) are never stored, so
// a retry after those still really re-attempts the redeem.
//
// Eviction is a true LRU via container/list: get/put move the entry to the
// front, and overflow pops from the back — O(1) per operation.
type redeemCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	size    int
	entries map[[32]byte]*list.Element
	lru     *list.List // front = most recently used
}

type cacheEntry struct {
	key  [32]byte
	body json.RawMessage // passthrough body; never mutated after store
	ts   time.Time
}

// newRedeemCache builds a cache. A ttl <= 0 disables caching entirely.
func newRedeemCache(ttl time.Duration, size int) *redeemCache {
	if size < 1 {
		size = 1024
	}
	return &redeemCache{
		ttl:     ttl,
		size:    size,
		entries: make(map[[32]byte]*list.Element, 64),
		lru:     list.New(),
	}
}

// cacheKey derives the cache key for a normalized code + mobile pair.
func cacheKey(code, mobile string) [32]byte {
	return sha256.Sum256([]byte(code + "\x00" + mobile))
}

// get returns the cached body when a fresh-enough entry exists. A hit
// moves the entry to the front of the LRU list.
func (c *redeemCache) get(key [32]byte) (json.RawMessage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ttl <= 0 {
		return nil, false
	}
	elem, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	e := elem.Value.(*cacheEntry)
	if time.Since(e.ts) > c.ttl {
		c.removeElement(elem)
		return nil, false
	}
	c.lru.MoveToFront(elem)
	return e.body, true
}

// put stores a successful upstream answer, evicting the least-recently
// used entry when the cache is at capacity.
func (c *redeemCache) put(key [32]byte, body json.RawMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ttl <= 0 {
		return
	}
	now := time.Now()

	if elem, ok := c.entries[key]; ok {
		// Refresh an existing entry in place (same body, new timestamp).
		e := elem.Value.(*cacheEntry)
		if time.Since(e.ts) > c.ttl {
			c.removeElement(elem) // stale: drop, insert fresh below
		} else {
			e.ts = now
			c.lru.MoveToFront(elem)
			return
		}
	}

	for c.lru.Len() >= c.size {
		// Pop the least-recently used entry (back of the list).
		back := c.lru.Back()
		if back == nil {
			break
		}
		c.removeElement(back)
	}

	elem := c.lru.PushFront(&cacheEntry{key: key, body: body, ts: now})
	c.entries[key] = elem
}

// removeElement removes a list element and its map entry. The caller
// holds c.mu.
func (c *redeemCache) removeElement(elem *list.Element) {
	c.lru.Remove(elem)
	e := elem.Value.(*cacheEntry)
	delete(c.entries, e.key)
}