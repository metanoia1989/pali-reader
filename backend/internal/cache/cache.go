// Package cache is the Redis layer.
//
// Only immutable or widely shared things are cached: the corpus, dictionary
// lookups, and session tokens. Per-user annotations are never cached — they
// change on every keystroke and are cheap to read by index.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache wraps the Redis client. A zero-value Cache is usable and does nothing,
// so the server keeps working when Redis is down: every method degrades to a
// miss rather than an error.
type Cache struct {
	rdb *redis.Client
	ttl time.Duration
}

// Open connects to Redis. A failure is not fatal by design — the caller gets a
// disabled cache and a warning.
func Open(url string, ttl time.Duration) (*Cache, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	opt.MaxRetries = 1
	opt.DialTimeout = 2 * time.Second
	opt.ReadTimeout = 2 * time.Second
	opt.WriteTimeout = 2 * time.Second
	rdb := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &Cache{rdb: rdb, ttl: ttl}, nil
}

// Disabled returns a cache that never hits. Used when Redis is unreachable.
func Disabled() *Cache { return &Cache{} }

// Enabled reports whether the cache is connected.
func (c *Cache) Enabled() bool { return c != nil && c.rdb != nil }

// GetJSON decodes a cached value into v. A miss, an expired key or a decode
// failure all report false so the caller re-reads the source of truth.
func (c *Cache) GetJSON(ctx context.Context, key string, v any) bool {
	if !c.Enabled() {
		return false
	}
	b, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.Printf("cache get %s: %v", key, err)
		}
		return false
	}
	return json.Unmarshal(b, v) == nil
}

// SetJSON stores v, ignoring failures: a broken cache must never break a
// request that has already been served correctly.
func (c *Cache) SetJSON(ctx context.Context, key string, v any, ttl time.Duration) {
	if !c.Enabled() {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	if ttl <= 0 {
		ttl = c.ttl
	}
	if err := c.rdb.Set(ctx, key, b, ttl).Err(); err != nil {
		log.Printf("cache set %s: %v", key, err)
	}
}

// Del removes keys.
func (c *Cache) Del(ctx context.Context, keys ...string) {
	if !c.Enabled() || len(keys) == 0 {
		return
	}
	if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
		log.Printf("cache del: %v", err)
	}
}

// DelPrefix removes every key under a prefix by scanning. It is only used by
// administrative cache clears, never on a request path.
func (c *Cache) DelPrefix(ctx context.Context, prefix string) (int, error) {
	if !c.Enabled() {
		return 0, nil
	}
	var cursor uint64
	n := 0
	for {
		keys, next, err := c.rdb.Scan(ctx, cursor, prefix+"*", 500).Result()
		if err != nil {
			return n, err
		}
		if len(keys) > 0 {
			if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
				return n, err
			}
			n += len(keys)
		}
		cursor = next
		if cursor == 0 {
			return n, nil
		}
	}
}

// Incr counts an event inside a window and returns the new total. Used for
// rate limiting sign-in attempts.
func (c *Cache) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if !c.Enabled() {
		return 0, errors.New("cache disabled")
	}
	pipe := c.rdb.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// Stats reports the hit rate, for the health endpoint.
func (c *Cache) Stats(ctx context.Context) map[string]any {
	if !c.Enabled() {
		return map[string]any{"enabled": false}
	}
	info, err := c.rdb.Info(ctx, "stats").Result()
	if err != nil {
		return map[string]any{"enabled": true, "error": err.Error()}
	}
	out := map[string]any{"enabled": true}
	for _, line := range splitLines(info) {
		switch {
		case hasPrefix(line, "keyspace_hits:"):
			out["hits"] = line[len("keyspace_hits:"):]
		case hasPrefix(line, "keyspace_misses:"):
			out["misses"] = line[len("keyspace_misses:"):]
		case hasPrefix(line, "total_commands_processed:"):
			out["commands"] = line[len("total_commands_processed:"):]
		}
	}
	return out
}
