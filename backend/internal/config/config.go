// Package config turns environment variables into a validated runtime
// configuration. Everything that differs between a laptop and the test server
// lives here and nowhere else.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the whole runtime configuration of the API server.
type Config struct {
	// HTTP
	Addr string
	// Dev enables verbose logging and permissive CORS for the Vite dev server.
	Dev bool
	// StaticDir, when non-empty, is served for any path the API does not own.
	// In production this is the built Vue bundle; nginx serves it directly and
	// this is only a convenience for running the binary on its own.
	StaticDir string

	// Data stores
	MySQLDSN string
	RedisURL string

	// Cache
	SegmentCacheTTL time.Duration
	LookupCacheTTL  time.Duration
	SessionTTL      time.Duration

	// Import
	SourceDir string
}

// Load reads the environment and applies defaults suited to local development.
func Load() (*Config, error) {
	c := &Config{
		Addr:            env("ADDR", ":8090"),
		Dev:             envBool("DEV", false),
		StaticDir:       env("STATIC_DIR", ""),
		MySQLDSN:        env("DB_DSN", ""),
		RedisURL:        env("REDIS_URL", "redis://127.0.0.1:6379/0"),
		SegmentCacheTTL: envDuration("SEGMENT_CACHE_TTL", 24*time.Hour),
		LookupCacheTTL:  envDuration("LOOKUP_CACHE_TTL", 12*time.Hour),
		SessionTTL:      envDuration("SESSION_TTL", 90*24*time.Hour),
		SourceDir:       env("SOURCE_DIR", "data/source"),
	}

	if c.MySQLDSN == "" {
		if c.Dev {
			c.MySQLDSN = "root@tcp(127.0.0.1:3306)/pali_reading?charset=utf8mb4&parseTime=true&loc=Local"
		} else {
			return nil, fmt.Errorf("DB_DSN is required")
		}
	}
	c.MySQLDSN = normaliseDSN(c.MySQLDSN)
	return c, nil
}

// normaliseDSN fills in the parameters GORM's MySQL driver needs but that are
// easy to forget when hand-writing a DSN. Anything already present wins.
func normaliseDSN(dsn string) string {
	base, query, _ := strings.Cut(dsn, "?")
	want := []string{"charset=utf8mb4", "parseTime=true", "loc=Local"}
	if query == "" {
		return base + "?" + strings.Join(want, "&")
	}
	have := map[string]bool{}
	for _, kv := range strings.Split(query, "&") {
		k, _, _ := strings.Cut(kv, "=")
		have[strings.ToLower(k)] = true
	}
	for _, kv := range want {
		k, _, _ := strings.Cut(kv, "=")
		if !have[strings.ToLower(k)] {
			query += "&" + kv
		}
	}
	return base + "?" + query
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
