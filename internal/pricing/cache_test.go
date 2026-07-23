package pricing

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	c, err := openCache(path, 0)
	if err != nil {
		t.Fatalf("openCache: %v", err)
	}
	defer c.Close()

	key := "cvm:S5.MEDIUM4:ap-shanghai"
	payload := []byte(`{"monthly":123.45}`)

	if _, ok := c.Get(key); ok {
		t.Fatal("expected miss on empty cache")
	}
	if err := c.Put(key, payload); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok := c.Get(key)
	if !ok {
		t.Fatal("expected hit after Put")
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("payload = %q, want %q", got, payload)
	}
}

func TestCacheExpiry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	c, err := openCache(path, 0)
	if err != nil {
		t.Fatalf("openCache: %v", err)
	}
	defer c.Close()

	// Force an already-expired TTL so the entry is written in the past.
	c.ttl = -1 * time.Hour

	if err := c.Put("k", []byte("v")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, ok := c.Get("k"); ok {
		t.Error("expected expired entry to be a miss")
	}
}

func TestCachePersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	c1, err := openCache(path, 0)
	if err != nil {
		t.Fatalf("openCache: %v", err)
	}
	if err := c1.Put("k", []byte("persisted")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	c1.Close()

	c2, err := openCache(path, 0)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer c2.Close()
	got, ok := c2.Get("k")
	if !ok || !bytes.Equal(got, []byte("persisted")) {
		t.Errorf("after reopen got %q (ok=%v), want persisted", got, ok)
	}
}

func TestNilCacheSafe(t *testing.T) {
	var c *cache
	if _, ok := c.Get("k"); ok {
		t.Error("nil cache Get should miss")
	}
	if err := c.Put("k", []byte("v")); err != nil {
		t.Errorf("nil cache Put should be a no-op, got %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("nil cache Close should be a no-op, got %v", err)
	}
}
