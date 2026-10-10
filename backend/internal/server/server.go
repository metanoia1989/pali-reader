// Package server exposes the HTTP API.
//
// The shape of the API follows the reader: a catalog to browse, a segment
// range to read, a dictionary to consult, and a small set of endpoints for the
// reader's own marks. Corpus reads are cached in Redis; anything belonging to
// a user is read straight from MySQL by index and never cached.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/metanoia/pali-reader/backend/internal/cache"
	"github.com/metanoia/pali-reader/backend/internal/config"
	"github.com/metanoia/pali-reader/backend/internal/dict"
	"github.com/metanoia/pali-reader/backend/internal/store"
)

// Server holds the dependencies every handler needs.
type Server struct {
	cfg   *config.Config
	db    *store.DB
	cache *cache.Cache
	dict  *dict.Service
	mux   chi.Router
}

// New builds the router.
func New(cfg *config.Config, db *store.DB, c *cache.Cache) *Server {
	s := &Server{
		cfg:   cfg,
		db:    db,
		cache: c,
		dict:  dict.New(db, c, int64(cfg.LookupCacheTTL.Seconds())),
	}
	s.routes()
	return s
}

// Handler returns the root http.Handler.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(s.requestLog)
	r.Use(s.cors)

	r.Get("/api/health", s.handleHealth)

	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/register", s.handleRegister)
		r.Post("/verify", s.handleVerify)
		r.Post("/resend", s.handleResend)
		r.Post("/login", s.handleLogin)
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/logout", s.handleLogout)
			r.Get("/me", s.handleMe)
			r.Patch("/me", s.handleUpdateMe)
		})
	})

	r.Route("/api", func(r chi.Router) {
		// Public corpus and dictionary reads.
		r.Get("/catalog", s.handleCatalog)
		r.Get("/stats", s.handleStats)
		r.Get("/books/{bookID}", s.handleBook)
		r.Get("/books/{bookID}/segments", s.handleSegments)
		r.Get("/books/{bookID}/search", s.handleSearch)
		r.Get("/books/{bookID}/refs/search", s.handleRefSearch)
		r.Get("/search", s.handleSearchAll)
		r.Get("/search/titles", s.handleTitleSearch)
		r.Get("/dict/lookup", s.handleLookup)
		// The English add-on dictionary, for words in the 参考译文. Public like
		// the other dictionary reads: it answers about a word, not a reader.
		r.Get("/dict/en/lookup", s.handleEnLookup)
		r.Get("/dict/suggest", s.handleSuggest)
		r.Get("/dict/declension", s.handleDeclension)

		// Anything with the reader's own marks needs a session.
		r.Group(func(r chi.Router) {
			r.Use(s.optionalAuth)
			r.Get("/books/{bookID}/marks", s.handleMarks)
		})
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/work/picks", s.handlePutPick)
			r.Delete("/work/picks", s.handleDeletePick)
			r.Put("/work/notes", s.handlePutNote)
			r.Delete("/work/notes/{id}", s.handleDeleteNote)
			r.Put("/work/translations", s.handlePutTranslation)
			r.Delete("/work/translations", s.handleDeleteTranslation)
			r.Put("/work/progress", s.handlePutProgress)
			r.Get("/work/progress", s.handleGetProgress)
			r.Get("/work/vocab", s.handleVocabList)
			r.Post("/work/vocab", s.handleVocabAdd)
			r.Delete("/work/vocab/{lemma}", s.handleVocabDelete)
			r.Get("/work/bookmarks", s.handleBookmarks)
			r.Put("/work/bookmarks", s.handleBookmarkPut)
			r.Delete("/work/bookmarks/{id}", s.handleBookmarkDelete)
			r.Get("/work/settings", s.handleGetSettings)
			r.Put("/work/settings", s.handlePutSettings)
			r.Get("/work/export", s.handleExport)
		})
	})

	if s.cfg.StaticDir != "" {
		spa := spaHandler(s.cfg.StaticDir)
		r.NotFound(spa)
		r.Get("/*", spa)
	}
	s.mux = r
}

// ---------------------------------------------------------------------------
// middleware
// ---------------------------------------------------------------------------

func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		// Static assets are noisy and already logged by nginx.
		if strings.HasPrefix(r.URL.Path, "/api/") || ww.Status() >= 400 {
			log.Printf("%s %s %d %s", r.Method, r.URL.Path, ww.Status(), time.Since(start).Round(time.Microsecond))
		}
	})
}

// cors allows the Vite dev server to call the API directly. In production the
// SPA and the API share an origin through nginx, so this stays off.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Dev {
			origin := r.Header.Get("Origin")
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type ctxKey int

const ctxUser ctxKey = iota

// optionalAuth attaches the user when a token is present and valid, and does
// nothing otherwise.
func (s *Server) optionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := s.userFromRequest(r); u != nil {
			r = r.WithContext(context.WithValue(r.Context(), ctxUser, u))
		}
		next.ServeHTTP(w, r)
	})
}

// requireAuth rejects the request unless a valid token is present.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := s.userFromRequest(r)
		if u == nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "请先登录")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUser, u)))
	})
}

func user(r *http.Request) *store.User {
	u, _ := r.Context().Value(ctxUser).(*store.User)
	return u
}

// userFromRequest resolves the bearer token. The session row is looked up in
// Redis first: it is read on every authenticated request and changes only on
// sign-in and sign-out.
func (s *Server) userFromRequest(r *http.Request) *store.User {
	tok := bearer(r)
	if tok == "" {
		return nil
	}
	var u store.User
	if s.cache.GetJSON(r.Context(), "s:"+tok, &u) {
		if u.ID != 0 {
			return &u
		}
		return nil
	}
	var sess store.Session
	if err := s.db.WithContext(r.Context()).Where("token = ?", tok).First(&sess).Error; err != nil {
		return nil
	}
	if time.Now().After(sess.ExpiresAt) {
		s.db.Where("token = ?", tok).Delete(&store.Session{})
		return nil
	}
	if err := s.db.WithContext(r.Context()).Where("id = ?", sess.UserID).First(&u).Error; err != nil {
		return nil
	}
	s.cache.SetJSON(r.Context(), "s:"+tok, u, time.Until(sess.ExpiresAt))
	return &u
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

// apiError is the single error shape the client understands.
type apiError struct {
	Error string `json:"error"`
	Msg   string `json:"msg"`
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, apiError{Error: code, Msg: msg})
}

func writeServerErr(w http.ResponseWriter, err error) {
	log.Printf("server error: %v", err)
	writeErr(w, http.StatusInternalServerError, "internal", "服务器内部错误")
}

func decodeBody(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20))
	return dec.Decode(v)
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func newToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func isNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }
