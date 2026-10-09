package server

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/metanoia/pali-reader/backend/internal/store"
)

// Catalog: the browsing tree the home page and the left rail are built from.

// CatalogBook is one volume as the catalog lists it.
type CatalogBook struct {
	ID        string `json:"id"`
	Basket    string `json:"basket"`
	Category  string `json:"category"`
	Name      string `json:"name"`
	NameZh    string `json:"nameZh"`
	PageCount int    `json:"pageCount"`
	SegCount  int    `json:"segCount"`
}

// CatalogCategory is a division inside a basket.
type CatalogCategory struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	NamePi string `json:"namePi"`
	NameZh string `json:"nameZh"`
	// Pitaka groups the divisions the way the canon itself does — the three
	// piṭakas. The basket a book carries (mūla / aṭṭhakathā / ṭīkā) says which
	// layer of literature it belongs to; this says which collection inside that
	// layer. A reader looking for the Aṅguttara wants the second, and the home
	// page has to show it or the suttas read as an undifferentiated heap.
	Pitaka      string        `json:"pitaka"`
	PitakaLabel string        `json:"pitakaLabel"`
	Books       []CatalogBook `json:"books"`
}

// pitakaOfDivision maps a source division onto the collection it belongs to.
var pitakaOfDivision = map[string]string{
	"vi": "vinaya",
	"di": "sutta", "ma": "sutta", "sa": "sutta", "an": "sutta", "ku": "sutta",
	"bi":          "abhidhamma",
	"annya_vi":    "other",
	"annya_bi":    "other",
	"annya_sadda": "other",
}

var pitakaLabel = map[string]string{
	"sutta":      "经藏 · Sutta",
	"vinaya":     "律藏 · Vinaya",
	"abhidhamma": "论藏 · Abhidhamma",
	"other":      "藏外 · Other",
}

// CatalogBasket is one of the four divisions of the canon.
type CatalogBasket struct {
	Code       string            `json:"code"`
	Name       string            `json:"name"`
	NamePi     string            `json:"namePi"`
	Categories []CatalogCategory `json:"categories"`
	BookCount  int               `json:"bookCount"`
}

type catalogResp struct {
	Baskets []CatalogBasket `json:"baskets"`
	Total   int             `json:"total"`
}

// divisionRank orders the divisions by how much of the canon they hold, which
// is also the order a reader looks for them in: the Sutta piṭaka first (43 of
// the 61 books), then the Vinaya, then the Abhidhamma.
//
// It used to be the order a canon is bound in — Vinaya first — and that made
// the catalogue open on the Vinaya, which is the smallest and the least likely
// thing anyone came for.
var divisionRank = map[string]int{
	"di": 0, "ma": 1, "sa": 2, "an": 3, "ku": 4, "vi": 5, "bi": 6,
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	var cached catalogResp
	if s.cache.GetJSON(r.Context(), "catalog:v1", &cached) {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	var cats []store.TextCategory
	if err := s.db.WithContext(r.Context()).Order("sort").Find(&cats).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	var books []store.TextBook
	if err := s.db.WithContext(r.Context()).Order("sort").Find(&books).Error; err != nil {
		writeServerErr(w, err)
		return
	}

	// The grouping is driven by the books, not by the category table: that table
	// groups by the source's own notion ("tri" for the three piṭakas, "annya"
	// for everything else), which does not line up with the basket a book
	// carries. Walking the books and using the category table only for a display
	// name keeps the two in step.
	meta := map[string]store.TextCategory{}
	for _, c := range cats {
		meta[c.ID] = c
	}

	byBasket := map[string][]CatalogCategory{}
	catIndex := map[string]*CatalogCategory{}
	for _, b := range books {
		key := b.Basket + "/" + b.Category
		cat, ok := catIndex[key]
		if !ok {
			m := meta[b.Category]
			name := m.Name
			if name == "" {
				name = b.Category
			}
			pk := pitakaOfDivision[b.Category]
			byBasket[b.Basket] = append(byBasket[b.Basket], CatalogCategory{
				ID: b.Category, Name: name, NamePi: m.NamePi, NameZh: m.NameZh,
				Pitaka: pk, PitakaLabel: pitakaLabel[pk],
			})
			cat = &byBasket[b.Basket][len(byBasket[b.Basket])-1]
			catIndex[key] = cat
		}
		cat.Books = append(cat.Books, CatalogBook{
			ID: b.ID, Basket: b.Basket, Category: b.Category, Name: b.Name,
			NameZh: b.NameZh, PageCount: b.PageCount, SegCount: b.SegCount,
		})
	}

	order := []string{store.BasketMula, store.BasketAttha, store.BasketTika, store.BasketAnnya}
	out := catalogResp{}
	for _, code := range order {
		cs := byBasket[code]
		if len(cs) == 0 {
			continue
		}
		sort.SliceStable(cs, func(i, j int) bool {
			if ri, rj := divisionRank[cs[i].ID], divisionRank[cs[j].ID]; ri != rj {
				return ri < rj
			}
			return cs[i].ID < cs[j].ID
		})
		b := CatalogBasket{
			Code:       code,
			Name:       store.BasketLabel[code],
			NamePi:     store.BasketPali[code],
			Categories: cs,
		}
		for _, c := range cs {
			b.BookCount += len(c.Books)
		}
		out.Total += b.BookCount
		out.Baskets = append(out.Baskets, b)
	}

	s.cache.SetJSON(r.Context(), "catalog:v1", out, 24*time.Hour)
	writeJSON(w, http.StatusOK, out)
}

type tocEntry struct {
	Name  string `json:"name"`
	Level int    `json:"level"`
	Seq   int    `json:"seq"`
	Para  int    `json:"para"`
	Page  int    `json:"page"`
}

type bookResp struct {
	Book CatalogBook `json:"book"`
	TOC  []tocEntry  `json:"toc"`
	// TOCRefs carries the published translation of each heading, keyed by
	// segment and then by language.
	//
	// A reader following the contents wants to know what "1. Naḷavaggo" is
	// before going there, and the segment window that /marks serves only covers
	// the part of the book that is loaded — so the headings' translations have
	// to come with the headings.
	TOCRefs map[string]map[string]string `json:"tocRefs,omitempty"`
	// Prev and Next let the reader move through the canon without going back
	// to the catalog.
	Prev *CatalogBook `json:"prev,omitempty"`
	Next *CatalogBook `json:"next,omitempty"`
}

func (s *Server) handleBook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "bookID")
	var cached bookResp
	if s.cache.GetJSON(r.Context(), "b:"+id, &cached) {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	var b store.TextBook
	if err := s.db.WithContext(r.Context()).Where("id = ?", id).First(&b).Error; err != nil {
		if isNotFound(err) {
			writeErr(w, http.StatusNotFound, "not_found", "没有这本书")
			return
		}
		writeServerErr(w, err)
		return
	}
	var toc []store.TextTOC
	if err := s.db.WithContext(r.Context()).Where("book_id = ?", id).
		Order("seq").Find(&toc).Error; err != nil {
		writeServerErr(w, err)
		return
	}

	out := bookResp{Book: CatalogBook{
		ID: b.ID, Basket: b.Basket, Category: b.Category, Name: b.Name,
		NameZh: b.NameZh, PageCount: b.PageCount, SegCount: b.SegCount,
	}}
	for _, t := range toc {
		out.TOC = append(out.TOC, tocEntry{
			Name: t.Name, Level: t.Level, Seq: t.SegmentSeq, Para: t.ParaNo, Page: t.PageNo,
		})
	}
	out.TOCRefs = s.tocRefs(r, id, toc)
	out.Prev, out.Next = s.neighbours(r, &b)
	s.cache.SetJSON(r.Context(), "b:"+id, out, 24*time.Hour)
	writeJSON(w, http.StatusOK, out)
}

// tocRefs reads the published translations of a book's headings, in every
// language the corpus carries. One query for the whole contents.
func (s *Server) tocRefs(r *http.Request, bookID string, toc []store.TextTOC) map[string]map[string]string {
	if len(toc) == 0 {
		return nil
	}
	seqs := make([]int, 0, len(toc))
	for _, t := range toc {
		if t.SegmentSeq > 0 {
			seqs = append(seqs, t.SegmentSeq)
		}
	}
	if len(seqs) == 0 {
		return nil
	}
	var rows []store.RefTranslation
	if err := s.db.WithContext(r.Context()).
		Where("book_id = ? AND segment IN ?", bookID, seqs).
		Find(&rows).Error; err != nil {
		return nil
	}
	out := map[string]map[string]string{}
	for _, row := range rows {
		k := strconv.Itoa(row.Segment)
		if out[k] == nil {
			out[k] = map[string]string{}
		}
		out[k][row.Lang] = row.Text
	}
	return out
}

func (s *Server) neighbours(r *http.Request, b *store.TextBook) (*CatalogBook, *CatalogBook) {
	var all []store.TextBook
	if err := s.db.WithContext(r.Context()).Order("sort").Find(&all).Error; err != nil {
		return nil, nil
	}
	conv := func(x store.TextBook) *CatalogBook {
		return &CatalogBook{ID: x.ID, Basket: x.Basket, Category: x.Category, Name: x.Name, NameZh: x.NameZh}
	}
	for i := range all {
		if all[i].ID != b.ID {
			continue
		}
		var prev, next *CatalogBook
		if i > 0 {
			prev = conv(all[i-1])
		}
		if i+1 < len(all) {
			next = conv(all[i+1])
		}
		return prev, next
	}
	return nil, nil
}

type statsResp struct {
	Books       int64 `json:"books"`
	Segments    int64 `json:"segments"`
	Paragraphs  int64 `json:"paragraphs"`
	Characters  int64 `json:"characters"`
	Headwords   int64 `json:"headwords"`
	LookupKeys  int64 `json:"lookupKeys"`
	DictEntries int64 `json:"dictEntries"`
	Translated  int64 `json:"translated"`
	Words       int64 `json:"words"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	var out statsResp
	if s.cache.GetJSON(r.Context(), "stats:v1", &out) {
		writeJSON(w, http.StatusOK, out)
		return
	}
	db := s.db.WithContext(r.Context())
	db.Model(&store.TextBook{}).Count(&out.Books)
	db.Model(&store.TextSegment{}).Count(&out.Segments)
	db.Model(&store.TextSegment{}).Where("para_no > 0").Distinct("book_id", "para_no").Count(&out.Paragraphs)
	db.Model(&store.TextSegment{}).Select("COALESCE(SUM(char_len),0)").Scan(&out.Characters)
	db.Model(&store.DictHeadword{}).Count(&out.Headwords)
	db.Model(&store.DictLookup{}).Count(&out.LookupKeys)
	db.Model(&store.DictEntry{}).Count(&out.DictEntries)
	db.Model(&store.RefTranslation{}).Count(&out.Translated)
	db.Model(&store.WordFreq{}).Count(&out.Words)
	s.cache.SetJSON(r.Context(), "stats:v1", out, 6*time.Hour)
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

type searchHit struct {
	BookID   string `json:"bookId"`
	BookName string `json:"bookName"`
	Segment  int    `json:"segment"`
	Para     int    `json:"para"`
	Page     int    `json:"page"`
	Kind     string `json:"kind"`
	// Snippet has the match wrapped in <mark> so the client does not have to
	// find it again.
	Snippet string `json:"snippet"`
}

type searchResp struct {
	Query   string      `json:"query"`
	Total   int         `json:"total"`
	Hits    []searchHit `json:"hits"`
	Limited bool        `json:"limited"`
}

// handleSearch searches inside one book.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	s.runSearch(w, r, bookID)
}

// handleSearchAll searches the whole canon.
func (s *Server) handleSearchAll(w http.ResponseWriter, r *http.Request) {
	s.runSearch(w, r, "")
}

func (s *Server) runSearch(w http.ResponseWriter, r *http.Request, bookID string) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		writeJSON(w, http.StatusOK, searchResp{Query: q})
		return
	}
	limit := queryInt(r, "limit", 60)
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	cacheKey := "search:v1:" + bookID + ":" + strings.ToLower(q) + ":" + itoa(limit)
	var cached searchResp
	if s.cache.GetJSON(r.Context(), cacheKey, &cached) {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	tx := s.db.WithContext(r.Context()).Model(&store.TextSegment{}).
		Where("kind <> ?", "heading").
		Where("text LIKE ?", "%"+escapeLike(q)+"%")
	if bookID != "" {
		tx = tx.Where("book_id = ?", bookID)
	}
	var segs []store.TextSegment
	if err := tx.Order("book_id, seq").Limit(limit + 1).Find(&segs).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	// An empty result set is an empty array, not null: the client iterates it
	// unconditionally, and `null` in a JSON response is the kind of thing that
	// only shows up as a crash in someone else's client.
	out := searchResp{Query: q, Hits: []searchHit{}}
	if len(segs) > limit {
		out.Limited = true
		segs = segs[:limit]
	}
	out.Total = len(segs)

	names := map[string]string{}
	var books []store.TextBook
	if err := s.db.WithContext(r.Context()).Select("id", "name", "name_zh").Find(&books).Error; err == nil {
		for _, b := range books {
			if b.NameZh != "" {
				names[b.ID] = b.NameZh
			} else {
				names[b.ID] = b.Name
			}
		}
	}
	for _, seg := range segs {
		out.Hits = append(out.Hits, searchHit{
			BookID: seg.BookID, BookName: names[seg.BookID], Segment: seg.Seq,
			Para: seg.ParaNo, Page: seg.PageNo, Kind: seg.Kind,
			Snippet: snippet(seg.Text, q, 160),
		})
	}
	s.cache.SetJSON(r.Context(), cacheKey, out, time.Hour)
	writeJSON(w, http.StatusOK, out)
}

// snippet cuts a window around the first match and marks it. The corpus text
// is trusted, but it is still escaped — a stray '<' in a variant reading must
// not become markup on the client.
func snippet(text, q string, width int) string {
	lowerText := strings.ToLower(text)
	lowerQ := strings.ToLower(q)
	i := strings.Index(lowerText, lowerQ)
	if i < 0 {
		if len([]rune(text)) > width {
			return escapeHTML(string([]rune(text)[:width])) + "…"
		}
		return escapeHTML(text)
	}
	start := i - width/3
	if start < 0 {
		start = 0
	}
	end := i + len(q) + width
	if end > len(text) {
		end = len(text)
	}
	// Snap to rune boundaries so a multi-byte letter is never cut in half.
	for start > 0 && !isRuneStart(text[start]) {
		start--
	}
	for end < len(text) && !isRuneStart(text[end]) {
		end++
	}
	pre := ""
	if start > 0 {
		pre = "…"
	}
	post := ""
	if end < len(text) {
		post = "…"
	}
	return pre + escapeHTML(text[start:i]) +
		"<mark>" + escapeHTML(text[i:i+len(q)]) + "</mark>" +
		escapeHTML(text[i+len(q):end]) + post
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// escapeLike neutralises the LIKE wildcards so a query containing % or _ does
// not turn into a table scan of everything.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
