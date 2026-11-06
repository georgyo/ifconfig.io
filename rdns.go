package main

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// lookupAddr performs reverse DNS lookups. It is a variable so tests can stub it.
var lookupAddr = net.DefaultResolver.LookupAddr

// rdnsCache resolves IP addresses to host names without letting slow PTR
// servers stall requests.
//
// Lookups run in the background with a generous timeout, but callers only wait
// up to their own (short) deadline. A lookup that finishes after the caller
// gave up still populates the cache, so a reload usually shows the name.
// Concurrent requests for the same address share a single lookup, and both
// successful and failed results are cached.
type rdnsCache struct {
	shards [rdnsShards]rdnsShard
	lookup func(ctx context.Context, addr string) ([]string, error)

	maxPerShard   int
	ttl           time.Duration // how long successful lookups are cached
	negativeTTL   time.Duration // how long failed lookups are cached
	lookupTimeout time.Duration // upper bound on a background lookup

	inflight    atomic.Int64
	maxInflight int64
}

const rdnsShards = 64

type rdnsShard struct {
	mu sync.Mutex
	m  map[string]*rdnsEntry
}

type rdnsEntry struct {
	done    chan struct{} // closed once name/expires are set
	name    string        // empty if the lookup failed
	expires time.Time
}

func newRDNSCache(size int) *rdnsCache {
	c := &rdnsCache{
		lookup:        lookupAddr,
		maxPerShard:   max(size/rdnsShards, 1),
		ttl:           5 * time.Minute,
		negativeTTL:   time.Minute,
		lookupTimeout: 5 * time.Second,
		maxInflight:   4096,
	}
	for i := range c.shards {
		c.shards[i].m = make(map[string]*rdnsEntry)
	}
	return c
}

func (c *rdnsCache) shard(ip string) *rdnsShard {
	// FNV-1a; cheap and good enough to spread addresses across shards.
	h := uint32(2166136261)
	for i := 0; i < len(ip); i++ {
		h ^= uint32(ip[i])
		h *= 16777619
	}
	return &c.shards[h%rdnsShards]
}

// Lookup returns the first PTR name for ip, waiting at most wait. It returns
// false if no name is known within that time.
func (c *rdnsCache) Lookup(ip string, wait time.Duration) (string, bool) {
	e := c.entry(ip)
	if e == nil {
		return "", false
	}

	select {
	case <-e.done:
	default:
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-e.done:
		case <-t.C:
			return "", false
		}
	}
	return e.name, e.name != ""
}

// entry returns the cache entry for ip, starting a lookup if there is no
// usable entry. It returns nil if too many lookups are already in flight.
func (c *rdnsCache) entry(ip string) *rdnsEntry {
	s := c.shard(ip)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if e, ok := s.m[ip]; ok {
		select {
		case <-e.done:
			if now.Before(e.expires) {
				return e
			}
		default:
			return e // lookup still running
		}
	}

	if c.inflight.Load() >= c.maxInflight {
		return nil
	}

	if len(s.m) >= c.maxPerShard {
		c.evict(s, now)
	}

	e := &rdnsEntry{done: make(chan struct{})}
	s.m[ip] = e
	c.inflight.Add(1)
	go c.resolve(ip, e)
	return e
}

// evict frees room in a full shard. Expired entries go first; if that is not
// enough, an arbitrary eighth of the completed entries are dropped so the cost
// is amortised over many inserts. Must be called with s.mu held.
func (c *rdnsCache) evict(s *rdnsShard, now time.Time) {
	target := c.maxPerShard - max(c.maxPerShard/8, 1)
	for ip, e := range s.m {
		select {
		case <-e.done:
			if !now.Before(e.expires) {
				delete(s.m, ip)
			}
		default:
		}
	}
	for ip, e := range s.m {
		if len(s.m) <= target {
			break
		}
		select {
		case <-e.done:
			delete(s.m, ip)
		default:
		}
	}
}

func (c *rdnsCache) resolve(ip string, e *rdnsEntry) {
	defer c.inflight.Add(-1)

	ctx, cancel := context.WithTimeout(context.Background(), c.lookupTimeout)
	names, err := c.lookup(ctx, ip)
	cancel()

	if err == nil && len(names) > 0 {
		e.name = names[0]
		e.expires = time.Now().Add(c.ttl)
	} else {
		e.expires = time.Now().Add(c.negativeTTL)
	}
	close(e.done)
}
