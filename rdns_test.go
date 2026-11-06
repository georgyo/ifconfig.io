package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func stubLookupFunc(t *testing.T, f func(context.Context, string) ([]string, error)) {
	t.Helper()
	orig := lookupAddr
	lookupAddr = f
	t.Cleanup(func() { lookupAddr = orig })
}

func TestRDNSCachesResults(t *testing.T) {
	var calls atomic.Int32
	stubLookupFunc(t, func(_ context.Context, ip string) ([]string, error) {
		calls.Add(1)
		if ip == "192.0.2.1" {
			return nil, errors.New("nxdomain")
		}
		return []string{"name-of-" + ip}, nil
	})
	c := newRDNSCache(1000)

	for range 3 {
		if name, ok := c.Lookup("192.0.2.2", time.Second); !ok || name != "name-of-192.0.2.2" {
			t.Fatalf("got %q, %v", name, ok)
		}
		if _, ok := c.Lookup("192.0.2.1", time.Second); ok {
			t.Fatal("failed lookup reported success")
		}
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("lookups = %d, want 2 (results should be cached)", n)
	}
}

func TestRDNSExpiry(t *testing.T) {
	var calls atomic.Int32
	stubLookupFunc(t, func(context.Context, string) ([]string, error) {
		calls.Add(1)
		return []string{"x."}, nil
	})
	c := newRDNSCache(1000)
	c.ttl = time.Millisecond

	c.Lookup("192.0.2.3", time.Second)
	time.Sleep(5 * time.Millisecond)
	c.Lookup("192.0.2.3", time.Second)
	if n := calls.Load(); n != 2 {
		t.Fatalf("lookups = %d, want 2 after expiry", n)
	}
}

func TestRDNSTimeoutThenLateResultIsCached(t *testing.T) {
	release := make(chan struct{})
	stubLookupFunc(t, func(context.Context, string) ([]string, error) {
		<-release
		return []string{"slow.example."}, nil
	})
	c := newRDNSCache(1000)

	start := time.Now()
	if _, ok := c.Lookup("192.0.2.4", 20*time.Millisecond); ok {
		t.Fatal("expected timeout")
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Lookup blocked for %v", d)
	}

	close(release)
	if name, ok := c.Lookup("192.0.2.4", time.Second); !ok || name != "slow.example." {
		t.Fatalf("got %q, %v", name, ok)
	}
}

func TestRDNSCoalescesConcurrentLookups(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	stubLookupFunc(t, func(context.Context, string) ([]string, error) {
		calls.Add(1)
		<-release
		return []string{"x."}, nil
	})
	c := newRDNSCache(1000)

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if _, ok := c.Lookup("192.0.2.5", 5*time.Second); !ok {
				t.Error("lookup failed")
			}
		})
	}
	time.Sleep(10 * time.Millisecond)
	close(release)
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Fatalf("lookups = %d, want 1", n)
	}
}

func TestRDNSBoundedSize(t *testing.T) {
	stubLookupFunc(t, func(context.Context, string) ([]string, error) {
		return []string{"x."}, nil
	})
	c := newRDNSCache(rdnsShards * 16)
	for i := range 10000 {
		c.Lookup(fmt.Sprintf("10.0.%d.%d", i/256, i%256), time.Second)
	}
	for i := range c.shards {
		if n := len(c.shards[i].m); n > c.maxPerShard {
			t.Fatalf("shard %d has %d entries, max %d", i, n, c.maxPerShard)
		}
	}
}

func TestRDNSInflightLimit(t *testing.T) {
	release := make(chan struct{})
	stubLookupFunc(t, func(context.Context, string) ([]string, error) {
		<-release
		return []string{"x."}, nil
	})
	defer close(release)
	c := newRDNSCache(1000)
	c.maxInflight = 2

	for i := range 5 {
		c.Lookup(fmt.Sprintf("192.0.2.%d", 10+i), 0)
	}
	if n := c.inflight.Load(); n != 2 {
		t.Fatalf("inflight = %d, want 2", n)
	}
}
