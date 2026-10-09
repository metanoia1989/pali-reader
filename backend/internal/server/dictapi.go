package server

import (
	"net/http"
	"strings"

	"github.com/metanoia/pali-reader/backend/internal/dict"
)

// Dictionary endpoints. The reader hits /dict/lookup on every tap, so it is
// the hottest path in the application: one request, one cached answer, no
// follow-up calls needed to render the whole panel.

func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	word := strings.TrimSpace(r.URL.Query().Get("word"))
	if word == "" || len([]rune(word)) > 80 {
		writeErr(w, http.StatusBadRequest, "bad_request", "缺少要查的词")
		return
	}
	res, err := s.dict.Lookup(r.Context(), word)
	if err != nil {
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleSuggest(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	items, err := s.dict.Suggestions(r.Context(), q, queryInt(r, "limit", 20))
	if err != nil {
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleDeclension(w http.ResponseWriter, r *http.Request) {
	pattern := strings.TrimSpace(r.URL.Query().Get("pattern"))
	stem := strings.TrimSpace(r.URL.Query().Get("stem"))
	form := strings.TrimSpace(r.URL.Query().Get("form"))
	d, err := s.dict.Declension(r.Context(), pattern, stem, form)
	if err != nil {
		if isNotFound(err) {
			writeJSON(w, http.StatusOK, map[string]any{"declension": nil})
			return
		}
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"declension": d})
}

// handleHealth reports whether the pieces the API depends on are reachable.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"ok": true, "version": "1.0.0"}
	sqlDB, err := s.db.SQL()
	if err != nil || sqlDB.PingContext(r.Context()) != nil {
		out["ok"] = false
		out["mysql"] = "down"
	} else {
		out["mysql"] = "up"
	}
	out["redis"] = s.cache.Stats(r.Context())
	writeJSON(w, http.StatusOK, out)
}

var _ = dict.CleanLemma
