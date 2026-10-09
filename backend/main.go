// Command pali-reader serves the Pāḷi Tipiṭaka reading API.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/metanoia/pali-reader/backend/internal/cache"
	"github.com/metanoia/pali-reader/backend/internal/config"
	"github.com/metanoia/pali-reader/backend/internal/server"
	"github.com/metanoia/pali-reader/backend/internal/store"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[pali] ")

	flush := flag.Bool("flush-cache", false,
		"delete every cached corpus and dictionary entry, then exit. "+
			"Needed after a corpus re-import: segment numbers move, and a stale "+
			"window would show the previous paragraphing.")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := store.Open(cfg.MySQLDSN, cfg.Dev)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	if err := db.Migrate(); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	c, err := cache.Open(cfg.RedisURL, cfg.LookupCacheTTL)
	if err != nil {
		log.Printf("redis unavailable (%v) — serving without a cache", err)
		c = cache.Disabled()
	}

	if *flush {
		if !c.Enabled() {
			log.Fatalf("cannot flush: Redis is not reachable")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		total := 0
		for _, prefix := range []string{"seg:", "b:", "catalog:", "stats:", "d:", "search:"} {
			n, err := c.DelPrefix(ctx, prefix)
			if err != nil {
				log.Fatalf("flush %s: %v", prefix, err)
			}
			log.Printf("  %-10s %6d keys", prefix, n)
			total += n
		}
		log.Printf("flushed %d cache keys", total)
		return
	}

	srv := server.New(cfg, db, c)
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("listening on %s (dev=%v, cache=%v)", cfg.Addr, cfg.Dev, c.Enabled())
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Printf("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
