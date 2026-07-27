package pricing

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

// cache is a small on-disk KV cache backed by bbolt.
//
// Layout: a single bucket "prices" mapping
//
//	key   = sha256(request)                (hex string)
//	value = [8-byte big-endian unix expiry][raw JSON payload]
//
// Entries past their expiry are treated as misses and lazily overwritten on
// the next Put. This keeps us well under Tencent Cloud's InquiryPrice QPS limit
// when a plan references the same instance_type / disk spec repeatedly, and it
// makes `diff` cheap because the unchanged resources hit the cache on the
// second pass.
type cache struct {
	db  *bolt.DB
	ttl time.Duration
}

const cacheBucket = "prices"

// defaultTTL matches what the README advertises.
const defaultTTL = 24 * time.Hour

func openCache(path string, ttl time.Duration) (*cache, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// Timeout avoids hanging forever if another process holds the file lock.
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, err
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, e := tx.CreateBucketIfNotExists([]byte(cacheBucket))
		return e
	}); err != nil {
		_ = db.Close()
		return nil, err
	}
	if ttl <= 0 {
		ttl = defaultTTL
	}
	c := &cache{db: db, ttl: ttl}
	// Best-effort startup sweep so unique SKUs from one-off plans do not grow
	// the cache file without bound. This is the only place expired-but-never-
	// re-read entries get reclaimed (Get only evicts on access). Failures are
	// non-fatal: the engine still works, just without a clean sweep.
	if err := c.SweepExpired(); err != nil {
		fmt.Fprintf(os.Stderr, "cloudtab: warning: price cache sweep failed (%v); continuing\n", err)
	}
	return c, nil
}

// SweepExpired deletes every expired (or malformed) entry from the cache bucket.
// It is safe to call at any time; the engine invokes it once on open as a
// startup cleanup. Unique SKUs fetched by a one-off plan would otherwise sit in
// the file forever (Get only evicts an entry when it is read again), so this
// keeps the on-disk cache from growing unbounded.
func (c *cache) SweepExpired() error {
	if c == nil || c.db == nil {
		return nil
	}
	now := time.Now().Unix()
	return c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(cacheBucket))
		if b == nil {
			return nil
		}
		var toDelete [][]byte
		_ = b.ForEach(func(k, v []byte) error {
			if len(v) < 8 {
				toDelete = append(toDelete, k) // malformed -> drop
				return nil
			}
			expiry := int64(binary.BigEndian.Uint64(v[:8]))
			if now > expiry {
				toDelete = append(toDelete, k)
			}
			return nil
		})
		for _, k := range toDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

// Get returns the cached payload if present and not expired. Expired entries
// are deleted on read so the on-disk cache does not grow unbounded with unique
// SKUs that are only priced once.
func (c *cache) Get(key string) ([]byte, bool) {
	if c == nil || c.db == nil {
		return nil, false
	}
	var (
		payload []byte
		found   bool
	)
	now := time.Now().Unix()
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(cacheBucket))
		if b == nil {
			return nil
		}
		v := b.Get([]byte(key))
		if len(v) < 8 {
			return nil
		}
		expiry := int64(binary.BigEndian.Uint64(v[:8]))
		if now > expiry {
			return nil // expired → miss (evicted below)
		}
		// Copy out: the slice is only valid within the transaction.
		payload = append([]byte(nil), v[8:]...)
		found = true
		return nil
	})
	if !found {
		// Best-effort eviction of the expired/malformed entry. Failures are
		// ignored: the next Put will overwrite the key anyway.
		_ = c.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(cacheBucket))
			if b == nil {
				return nil
			}
			return b.Delete([]byte(key))
		})
	}
	return payload, found
}

// Put stores payload with the configured TTL.
func (c *cache) Put(key string, val []byte) error {
	if c == nil || c.db == nil {
		return nil
	}
	expiry := time.Now().Add(c.ttl).Unix()
	buf := make([]byte, 8+len(val))
	binary.BigEndian.PutUint64(buf[:8], uint64(expiry))
	copy(buf[8:], val)
	return c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(cacheBucket))
		if b == nil {
			return errors.New("cache bucket missing")
		}
		return b.Put([]byte(key), buf)
	})
}

func (c *cache) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}
