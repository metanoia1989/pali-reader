package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/metanoia/pali-reader/backend/internal/store"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The reader's own work: chosen glosses, notes, translations, progress,
// vocabulary and bookmarks.
//
// Every one of these is anchored on (book, segment) and, where it applies, the
// index of the word inside that segment. Nothing anchors on a database row id
// of the corpus, so re-importing the canon never detaches a reader's work.

// pickReq is one assertion about one word. Kind decides which of the fields
// matter: a grammar reading uses the analysis columns, a meaning uses the gloss
// columns, a split uses Split.
type pickReq struct {
	BookID    string `json:"bookId"`
	Segment   int    `json:"segment"`
	WordIndex int    `json:"wordIndex"`
	Kind      string `json:"kind"`
	Surface   string `json:"surface"`
	Lemma     string `json:"lemma"`
	LemmaID   uint   `json:"lemmaId"`
	POS       string `json:"pos"`
	Gender    string `json:"gender"`
	Case      string `json:"case"`
	Number    string `json:"number"`
	Grammar   string `json:"grammar"`
	// Meaning is the gloss. MeaningKey stands in for it when the canonical key
	// is built, so that a long gloss does not have to be part of a unique index.
	Meaning       string `json:"meaning"`
	MeaningKey    string `json:"meaningKey"`
	MeaningSource string `json:"meaningSource"`
	Split         string `json:"split"`
	Note          string `json:"note"`
}

// handlePutPick records one candidate reading, meaning or split.
//
// Adding is idempotent: tapping the same row twice leaves one row, because the
// key is the identity of the assertion rather than of the tap.
func (s *Server) handlePutPick(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	var req pickReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if req.BookID == "" || req.Segment <= 0 || req.WordIndex < 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "缺少定位信息")
		return
	}
	if req.Kind != store.PickGrammar && req.Kind != store.PickMeaning && req.Kind != store.PickSplit {
		writeErr(w, http.StatusBadRequest, "bad_request", "未知的标注类型")
		return
	}

	var key string
	switch req.Kind {
	case store.PickGrammar:
		key = store.PickKey(req.Kind, req.Lemma, req.POS, req.Gender, req.Case, req.Number, req.Grammar)
	case store.PickMeaning:
		mk := req.MeaningKey
		if mk == "" {
			mk = req.Meaning
		}
		key = store.PickKey(req.Kind, req.MeaningSource, mk)
	default:
		key = store.PickKey(req.Kind, req.Split)
	}

	p := store.WordPick{
		UserID: u.ID, BookID: req.BookID, Segment: req.Segment, WordIndex: req.WordIndex,
		Key: key, Kind: req.Kind,
		Surface: clip(req.Surface, 191), Lemma: clip(req.Lemma, 191), LemmaID: req.LemmaID,
		POS: clip(req.POS, 64), Gender: clip(req.Gender, 24),
		Case: clip(req.Case, 24), Number: clip(req.Number, 24),
		Grammar: clip(req.Grammar, 191),
		Meaning: req.Meaning, MeaningSource: clip(req.MeaningSource, 64),
		Split: clip(req.Split, 255), Note: req.Note,
	}
	err := s.db.WithContext(r.Context()).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"}, {Name: "book_id"}, {Name: "segment"},
			{Name: "word_index"}, {Name: "key"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"surface", "lemma", "updated_at"}),
	}).Create(&p).Error
	if err != nil {
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, markPick{
		Key: key, Segment: p.Segment, WordIndex: p.WordIndex, Kind: p.Kind,
		Surface: p.Surface, Lemma: p.Lemma, LemmaID: p.LemmaID,
		POS: p.POS, Gender: p.Gender, Case: p.Case, Number: p.Number,
		Grammar: p.Grammar, Meaning: p.Meaning, MeaningSource: p.MeaningSource,
		Split: p.Split, Note: p.Note,
	})
}

// handleDeletePick removes one assertion, or every assertion about one word
// when no key is given — "never mind this word" is a single gesture.
func (s *Server) handleDeletePick(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	bookID := r.URL.Query().Get("bookId")
	seg := queryInt(r, "segment", 0)
	word := queryInt(r, "wordIndex", -1)
	if bookID == "" || seg <= 0 || word < 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "缺少定位信息")
		return
	}
	t := s.db.WithContext(r.Context()).
		Where("user_id = ? AND book_id = ? AND segment = ? AND word_index = ?", u.ID, bookID, seg, word)
	if key := r.URL.Query().Get("key"); key != "" {
		t = t.Where("`key` = ?", key)
	}
	if err := t.Delete(&store.WordPick{}).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type noteReq struct {
	ID        uint   `json:"id"`
	BookID    string `json:"bookId"`
	Segment   int    `json:"segment"`
	WordIndex int    `json:"wordIndex"`
	Kind      string `json:"kind"`
	Body      string `json:"body"`
	Quote     string `json:"quote"`
}

func (s *Server) handlePutNote(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	var req noteReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if req.BookID == "" || req.Segment <= 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "缺少定位信息")
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		writeErr(w, http.StatusBadRequest, "empty", "批注内容不能为空")
		return
	}
	if req.Kind == "" {
		req.Kind = store.NoteSegment
		if req.WordIndex >= 0 {
			req.Kind = store.NoteWord
		}
	}
	// An update may only touch the caller's own row.
	if req.ID != 0 {
		res := s.db.WithContext(r.Context()).Model(&store.Note{}).
			Where("id = ? AND user_id = ?", req.ID, u.ID).
			Updates(map[string]any{"body": body, "quote": clip(req.Quote, 255)})
		if res.Error != nil {
			writeServerErr(w, res.Error)
			return
		}
		if res.RowsAffected == 0 {
			writeErr(w, http.StatusNotFound, "not_found", "批注不存在")
			return
		}
		var n store.Note
		s.db.WithContext(r.Context()).Where("id = ?", req.ID).First(&n)
		writeJSON(w, http.StatusOK, markNote{
			ID: n.ID, Segment: n.Segment, WordIndex: n.WordIndex, Kind: n.Kind,
			Body: n.Body, Quote: n.Quote, UpdatedAt: n.UpdatedAt,
		})
		return
	}

	n := store.Note{
		UserID: u.ID, BookID: req.BookID, Segment: req.Segment, WordIndex: req.WordIndex,
		Kind: req.Kind, Body: body, Quote: clip(req.Quote, 255),
	}
	if err := s.db.WithContext(r.Context()).Create(&n).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, markNote{
		ID: n.ID, Segment: n.Segment, WordIndex: n.WordIndex, Kind: n.Kind,
		Body: n.Body, Quote: n.Quote, UpdatedAt: n.UpdatedAt,
	})
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	id := chi.URLParam(r, "id")
	res := s.db.WithContext(r.Context()).
		Where("id = ? AND user_id = ?", id, u.ID).Delete(&store.Note{})
	if res.Error != nil {
		writeServerErr(w, res.Error)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": res.RowsAffected})
}

type translationReq struct {
	BookID  string `json:"bookId"`
	Segment int    `json:"segment"`
	Text    string `json:"text"`
}

func (s *Server) handlePutTranslation(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	var req translationReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if req.BookID == "" || req.Segment <= 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "缺少定位信息")
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		s.db.WithContext(r.Context()).
			Where("user_id = ? AND book_id = ? AND segment = ?", u.ID, req.BookID, req.Segment).
			Delete(&store.Translation{})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": true})
		return
	}
	t := store.Translation{UserID: u.ID, BookID: req.BookID, Segment: req.Segment, Text: text}
	if err := s.db.WithContext(r.Context()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "book_id"}, {Name: "segment"}},
		DoUpdates: clause.AssignmentColumns([]string{"text", "updated_at"}),
	}).Create(&t).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	var saved store.Translation
	s.db.WithContext(r.Context()).
		Where("user_id = ? AND book_id = ? AND segment = ?", u.ID, req.BookID, req.Segment).
		First(&saved)
	writeJSON(w, http.StatusOK, markTranslation{
		Segment: saved.Segment, Text: saved.Text, UpdatedAt: saved.UpdatedAt,
	})
}

func (s *Server) handleDeleteTranslation(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	bookID := r.URL.Query().Get("bookId")
	seg := queryInt(r, "segment", 0)
	if bookID == "" || seg <= 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "缺少定位信息")
		return
	}
	if err := s.db.WithContext(r.Context()).
		Where("user_id = ? AND book_id = ? AND segment = ?", u.ID, bookID, seg).
		Delete(&store.Translation{}).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type progressReq struct {
	BookID  string `json:"bookId"`
	Segment int    `json:"segment"`
}

func (s *Server) handlePutProgress(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	var req progressReq
	if err := decodeBody(r, &req); err != nil || req.BookID == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	p := store.Progress{UserID: u.ID, BookID: req.BookID, Segment: req.Segment, UpdatedAt: time.Now()}
	if err := s.db.WithContext(r.Context()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "book_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"segment", "updated_at"}),
	}).Create(&p).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// progressItem is one row of "continue reading".
type progressItem struct {
	BookID     string    `json:"bookId"`
	BookName   string    `json:"bookName"`
	BookNameZh string    `json:"bookNameZh"`
	Segment    int       `json:"segment"`
	Para       int       `json:"para"`
	Total      int       `json:"total"`
	Percent    float64   `json:"percent"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (s *Server) handleGetProgress(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	var rows []store.Progress
	if err := s.db.WithContext(r.Context()).Where("user_id = ?", u.ID).
		Order("updated_at DESC").Limit(30).Find(&rows).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	out := make([]progressItem, 0, len(rows))
	for _, p := range rows {
		var b store.TextBook
		if err := s.db.WithContext(r.Context()).Where("id = ?", p.BookID).First(&b).Error; err != nil {
			continue
		}
		var para int
		s.db.WithContext(r.Context()).Model(&store.TextSegment{}).
			Where("book_id = ? AND seq = ?", p.BookID, p.Segment).
			Pluck("para_no", &para)
		item := progressItem{
			BookID: b.ID, BookName: b.Name, BookNameZh: b.NameZh,
			Segment: p.Segment, Para: para, Total: b.SegCount, UpdatedAt: p.UpdatedAt,
		}
		if b.SegCount > 0 {
			item.Percent = float64(p.Segment) / float64(b.SegCount) * 100
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// ---------------------------------------------------------------------------
// Vocabulary
// ---------------------------------------------------------------------------

type vocabReq struct {
	Lemma   string `json:"lemma"`
	LemmaID uint   `json:"lemmaId"`
	POS     string `json:"pos"`
	Meaning string `json:"meaning"`
	Note    string `json:"note"`
}

func (s *Server) handleVocabAdd(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	var req vocabReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	req.Lemma = strings.TrimSpace(req.Lemma)
	if req.Lemma == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "缺少词条")
		return
	}
	v := store.VocabItem{
		UserID: u.ID, Lemma: clip(req.Lemma, 191), LemmaID: req.LemmaID,
		POS: clip(req.POS, 64), Meaning: req.Meaning, Note: req.Note,
		AddedAt: time.Now(), LastSeen: time.Now(),
	}
	if err := s.db.WithContext(r.Context()).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "lemma"}},
		DoUpdates: clause.Assignments(map[string]any{
			"lemma_id":  req.LemmaID,
			"pos":       clip(req.POS, 64),
			"meaning":   req.Meaning,
			"note":      req.Note,
			"last_seen": time.Now(),
			"seen":      gorm.Expr("seen + 1"),
		}),
	}).Create(&v).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleVocabList(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	tx := s.db.WithContext(r.Context()).Where("user_id = ?", u.ID)
	if q != "" {
		tx = tx.Where("lemma LIKE ?", "%"+escapeLike(q)+"%")
	}
	var items []store.VocabItem
	if err := tx.Order("added_at DESC").Limit(1000).Find(&items).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	// Corpus frequency turns the list into something to study from: the rarest
	// words a reader has kept are the ones worth drilling.
	freq := map[string]int{}
	if len(items) > 0 {
		lemmas := make([]string, 0, len(items))
		for _, it := range items {
			lemmas = append(lemmas, it.Lemma)
		}
		var fs []store.WordFreq
		s.db.WithContext(r.Context()).Where("word IN ?", lemmas).Find(&fs)
		for _, f := range fs {
			freq[f.Word] = f.Count
		}
	}
	type item struct {
		store.VocabItem
		Count int `json:"count"`
	}
	out := make([]item, 0, len(items))
	for _, it := range items {
		out = append(out, item{VocabItem: it, Count: freq[it.Lemma]})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].AddedAt.After(out[j].AddedAt)
	})
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleVocabDelete(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	lemma := chi.URLParam(r, "lemma")
	if err := s.db.WithContext(r.Context()).
		Where("user_id = ? AND lemma = ?", u.ID, lemma).Delete(&store.VocabItem{}).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// Bookmarks
// ---------------------------------------------------------------------------

type bookmarkReq struct {
	ID      uint   `json:"id"`
	BookID  string `json:"bookId"`
	Segment int    `json:"segment"`
	ParaNo  int    `json:"paraNo"`
	Label   string `json:"label"`
}

func (s *Server) handleBookmarkPut(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	var req bookmarkReq
	if err := decodeBody(r, &req); err != nil || req.BookID == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	b := store.Bookmark{
		UserID: u.ID, BookID: req.BookID, Segment: req.Segment,
		ParaNo: req.ParaNo, Label: clip(req.Label, 191), CreatedAt: time.Now(),
	}
	if req.ID != 0 {
		if err := s.db.WithContext(r.Context()).Model(&store.Bookmark{}).
			Where("id = ? AND user_id = ?", req.ID, u.ID).
			Updates(map[string]any{"label": b.Label, "segment": b.Segment, "para_no": b.ParaNo}).
			Error; err != nil {
			writeServerErr(w, err)
			return
		}
		b.ID = req.ID
		writeJSON(w, http.StatusOK, b)
		return
	}
	if err := s.db.WithContext(r.Context()).Create(&b).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleBookmarks(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	var items []store.Bookmark
	if err := s.db.WithContext(r.Context()).Where("user_id = ?", u.ID).
		Order("created_at DESC").Limit(500).Find(&items).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleBookmarkDelete(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	if err := s.db.WithContext(r.Context()).
		Where("id = ? AND user_id = ?", chi.URLParam(r, "id"), u.ID).
		Delete(&store.Bookmark{}).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleSettings reads and writes the reader's display preferences.
//
// They are stored as the JSON the client produced rather than as columns: the
// set of options is a presentation concern that changes often, and a schema
// migration for every new toggle would be absurd for a handful of bytes.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	out := map[string]any{}
	if u.Settings != "" {
		if err := json.Unmarshal([]byte(u.Settings), &out); err != nil {
			out = map[string]any{}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": out})
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	var raw map[string]any
	if err := decodeBody(r, &raw); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if len(raw) > 40 {
		writeErr(w, http.StatusBadRequest, "too_many", "设置项过多")
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "设置格式错误")
		return
	}
	if len(b) > 8192 {
		writeErr(w, http.StatusBadRequest, "too_large", "设置内容过大")
		return
	}
	if err := s.db.WithContext(r.Context()).Model(&store.User{}).
		Where("id = ?", u.ID).Update("settings", string(b)).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	s.cache.Del(r.Context(), "s:"+bearer(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleExport returns everything the account has written, so the reader can
// take their work elsewhere. It is deliberately a single dump: the data is the
// reader's, and an API that can only give it back a page at a time is not
// really an export.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	ctx := r.Context()
	out := map[string]any{
		"exportedAt": time.Now().Format(time.RFC3339),
		"user":       map[string]any{"id": u.ID, "email": u.Email, "name": u.DisplayName},
	}
	var picks []store.WordPick
	s.db.WithContext(ctx).Where("user_id = ?", u.ID).Find(&picks)
	out["picks"] = picks
	var notes []store.Note
	s.db.WithContext(ctx).Where("user_id = ?", u.ID).Find(&notes)
	out["notes"] = notes
	var trans []store.Translation
	s.db.WithContext(ctx).Where("user_id = ?", u.ID).Find(&trans)
	out["translations"] = trans
	var vocab []store.VocabItem
	s.db.WithContext(ctx).Where("user_id = ?", u.ID).Find(&vocab)
	out["vocab"] = vocab
	var marks []store.Bookmark
	s.db.WithContext(ctx).Where("user_id = ?", u.ID).Find(&marks)
	out["bookmarks"] = marks
	var prog []store.Progress
	s.db.WithContext(ctx).Where("user_id = ?", u.ID).Find(&prog)
	out["progress"] = prog

	w.Header().Set("Content-Disposition", `attachment; filename="pali-reader-export.json"`)
	writeJSON(w, http.StatusOK, out)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
