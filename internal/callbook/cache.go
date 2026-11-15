package callbook

import (
	"context"
	"sync"
	"time"
)

// Cache is a Looker that remembers the successful results of Next, keyed
// on the trimmed, upper-cased call. Errors, ErrNotFound included, are never
// cached, so the next lookup asks Next again.
//
// A cached result expires after TTL; a zero TTL keeps it for the life of
// the process. The zero value with Next set is ready to use. A Cache is
// safe for concurrent use and must not be copied after first use.
type Cache struct {
	// Next is the Looker whose results are cached.
	Next Looker
	// TTL is how long a result stays cached. Zero means for the session.
	TTL time.Duration
	// Now returns the current time. Nil means time.Now.
	Now func() time.Time

	mu      sync.Mutex
	entries map[string]cacheEntry
}

// cacheEntry is a cached result and when it was stored.
type cacheEntry struct {
	result Result
	stored time.Time
}

// Lookup implements Looker. Next is called without holding the cache's
// lock, so slow lookups of different calls do not block each other.
func (c *Cache) Lookup(ctx context.Context, call string) (Result, error) {
	key := normalize(call)
	if r, ok := c.get(key); ok {
		return r, nil
	}
	r, err := c.Next.Lookup(ctx, call)
	if err != nil {
		return Result{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]cacheEntry)
	}
	c.entries[key] = cacheEntry{result: r, stored: c.now()}
	return r, nil
}

// get returns the fresh cached result for key, if any.
func (c *Cache) get(key string) (Result, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return Result{}, false
	}
	if c.TTL > 0 && c.now().Sub(e.stored) >= c.TTL {
		delete(c.entries, key)
		return Result{}, false
	}
	return e.result, true
}

// now returns the current time from c.Now, or time.Now.
func (c *Cache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}
