package pricing

import (
	"encoding/binary"
	"errors"
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
	return &cache{db: db, ttl: ttl}, nil
}

// Get returns the cached payload if present and not expired.
func (c *cache) Get(key string) ([]byte, bool) {
	if c == nil || c.db == nil {
		return nil, false
	}
	var (
		payload []byte
		found   bool
	)
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
		if time.Now().Unix() > expiry {
			return nil // expired → miss
		}
		// Copy out: the slice is only valid within the transaction.
		payload = append([]byte(nil), v[8:]...)
		found = true
		return nil
	})
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
