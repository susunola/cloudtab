package pricing

// Minimal placeholder for a local KV cache.
// Real impl: bbolt (bucket "prices", key = sha256, value = raw JSON, TTL via
// a second bucket "expires"). Kept as a stub so M1 compiles without extra deps.

import "os"

type cache struct {
	path string
}

func openCache(path string) (*cache, error) {
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return nil, err
	}
	return &cache{path: path}, nil
}

func (c *cache) Get(key string) ([]byte, bool) { return nil, false }
func (c *cache) Put(key string, val []byte) error { return nil }
func (c *cache) Close() error { return nil }

func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}
