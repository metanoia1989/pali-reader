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
	// Lang is set only on a hit inside a published translation: which language
	// the match was found in, so the result can say 中 or 英 rather than
	// presenting another author's sentence as if it were the canon.
	Lang string `json:"lang,omitempty"`
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
	// A seq window, for searching one chapter rather than a whole book. The
	// chapter a reader has selected in the contents is a range of segment
	// numbers, and it is the range — not "what happens to be loaded" — that
	// decides what is searched: the loaded window is bounded, so a client-side
	// search of it would return nothing for a match two hundred segments above
	// the reader, which is exactly the silent emptiness the whole feature is
	// meant to avoid.
	from := queryInt(r, "from", 0)
	to := queryInt(r, "to", 0)
	cacheKey := "search:v2:" + bookID + ":" + itoa(from) + ":" + itoa(to) + ":" +
		strings.ToLower(q) + ":" + itoa(limit)
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
	if from > 0 {
		tx = tx.Where("seq >= ?", from)
	}
	if to > 0 {
		tx = tx.Where("seq <= ?", to)
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

	names := s.bookNames(r)
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

// bookNames is the display name of every book, Chinese first. Small (179 rows)
// and needed by every kind of hit, so it is fetched whole rather than joined.
func (s *Server) bookNames(r *http.Request) map[string]string {
	names := map[string]string{}
	var books []store.TextBook
	if err := s.db.WithContext(r.Context()).Select("id", "name", "name_zh").Find(&books).Error; err != nil {
		return names
	}
	for _, b := range books {
		if b.NameZh != "" {
			names[b.ID] = b.NameZh
		} else {
			names[b.ID] = b.Name
		}
	}
	return names
}

// ---------------------------------------------------------------------------
// Search by name
// ---------------------------------------------------------------------------

// titleHit is one name the reader might be looking for.
//
// This is a different question from the other modes. "Where does the text say
// this" is answered by a passage; "which sutta is this" is answered by a name
// that is itself a destination, and a name is not in the segment text at all —
// the corpus search deliberately skips headings, because a heading is
// navigation rather than reading. So the names come from the two places that
// hold them: `text_books` for volumes, `text_toc` for the suttas and chapters
// inside them, and the published translations of those headings for a reader
// who knows the sutta by its Chinese name.
type titleHit struct {
	Kind     string `json:"kind"` // book | heading
	BookID   string `json:"bookId"`
	BookName string `json:"bookName"`
	// Segment is where to land. A book with no contents entry of its own starts
	// at its first segment.
	Segment int    `json:"segment,omitempty"`
	Name    string `json:"name"`
	// Ref is the heading's translation, when the corpus has one in a language
	// the reader reads.
	Ref   string `json:"ref,omitempty"`
	Level int    `json:"level,omitempty"`
}

type titleResp struct {
	Query string     `json:"query"`
	Hits  []titleHit `json:"hits"`
}

func (s *Server) handleTitleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		writeJSON(w, http.StatusOK, titleResp{Query: q, Hits: []titleHit{}})
		return
	}
	limit := queryInt(r, "limit", 20)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	langs := splitLangs(r.URL.Query().Get("lang"))
	cacheKey := "titles:v1:" + strings.ToLower(q) + ":" + itoa(limit) + ":" + strings.Join(langs, ",")
	var cached titleResp
	if s.cache.GetJSON(r.Context(), cacheKey, &cached) {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	like := "%" + escapeLike(q) + "%"
	out := titleResp{Query: q, Hits: []titleHit{}}
	names := s.bookNames(r)

	// Volumes first: a reader typing a name they half remember is more likely
	// to mean the book than a chapter inside it.
	var books []store.TextBook
	if err := s.db.WithContext(r.Context()).
		Where("name LIKE ? OR name_zh LIKE ?", like, like).
		Order("sort").Limit(limit).Find(&books).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	for _, b := range books {
		out.Hits = append(out.Hits, titleHit{
			Kind: "book", BookID: b.ID, BookName: names[b.ID],
			Name: b.Name, Ref: b.NameZh,
		})
	}

	// Then the headings, and their translations under the same key. Two queries
	// rather than one because a heading's Pali name and its Chinese name are
	// rows in different tables; the join is by (book_id, segment), which is the
	// unique key ref_translations already has, so the plan drives from the
	// 36k-row contents table and looks up one row at a time — measured at
	// ~50ms across the whole canon, no index added.
	type headRow struct {
		BookID     string
		SegmentSeq int
		Name       string
		Level      int
		Ref        string
	}
	var heads []headRow
	if err := s.db.WithContext(r.Context()).
		Model(&store.TextTOC{}).
		Select("book_id, segment_seq, name, level").
		Where("name LIKE ?", like).
		Order("book_id, segment_seq").Limit(limit).Scan(&heads).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	var trefs []headRow
	if err := s.db.WithContext(r.Context()).Raw(
		`SELECT t.book_id AS book_id, t.segment_seq AS segment_seq, t.name AS name,
		        t.level AS level, r.text AS ref
		   FROM text_toc t
		   JOIN ref_translations r
		     ON r.book_id = t.book_id AND r.segment = t.segment_seq
		  WHERE r.text LIKE ? AND r.lang IN ?
		  ORDER BY t.book_id, t.segment_seq
		  LIMIT ?`, like, langs, limit).Scan(&trefs).Error; err != nil {
		writeServerErr(w, err)
		return
	}

	refOf := map[string]string{}
	for _, t := range trefs {
		if _, ok := refOf[t.BookID+":"+itoa(t.SegmentSeq)]; !ok {
			refOf[t.BookID+":"+itoa(t.SegmentSeq)] = t.Ref
		}
	}
	// A heading is one hit even when both its Pali name and its Chinese name
	// match, so the two lists are merged on the key they share.
	seen := map[string]bool{}
	add := func(h headRow) {
		key := h.BookID + ":" + itoa(h.SegmentSeq)
		if h.SegmentSeq == 0 || seen[key] {
			return
		}
		seen[key] = true
		out.Hits = append(out.Hits, titleHit{
			Kind: "heading", BookID: h.BookID, BookName: names[h.BookID],
			Segment: h.SegmentSeq, Name: h.Name, Level: h.Level, Ref: refOf[key],
		})
	}
	for _, h := range heads {
		add(h)
	}
	// Then the ones only the translation found: a reader typing 渡流经 should
	// arrive at oghataraṇasuttaṃ, whose Pali name contains none of those letters.
	for _, t := range trefs {
		add(t)
	}

	s.cache.SetJSON(r.Context(), cacheKey, out, time.Hour)
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// Search inside the published translations
// ---------------------------------------------------------------------------

// handleRefSearch searches the reference translations of ONE book.
//
// The scope is the book, and that is a deliberate limit rather than an
// oversight. `ref_translations` holds 2.2 million rows; the unique key is
// (book_id, segment, lang, source), so a query that names the book reads one
// book's worth — about twelve thousand rows, measured at ~40ms — while a query
// that does not is a full scan of the mediumtext column, measured at ~1.4s,
// which is not something to run on every keystroke. Making it corpus-wide
// would mean a FULLTEXT index (with the ngram parser, for Chinese) over those
// 2.2 million rows: a migration of its own, with its own disk cost, for a mode
// the reader uses while reading a book. Book-scoped is exact, instant, and
// independent of what the client happens to have loaded — which matters,
// because with 参考译文 set to 隐藏 the client holds almost no translations at
// all and a client-side search would answer "not found" to everything.
func (s *Server) handleRefSearch(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	langs := splitLangs(r.URL.Query().Get("lang"))
	if len([]rune(q)) < 2 {
		writeJSON(w, http.StatusOK, searchResp{Query: q, Hits: []searchHit{}})
		return
	}
	limit := queryInt(r, "limit", 40)
	if limit <= 0 || limit > 200 {
		limit = 40
	}
	cacheKey := "refsearch:v1:" + bookID + ":" + strings.ToLower(q) + ":" +
		itoa(limit) + ":" + strings.Join(langs, ",")
	var cached searchResp
	if s.cache.GetJSON(r.Context(), cacheKey, &cached) {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	var rows []store.RefTranslation
	if err := s.db.WithContext(r.Context()).
		Where("book_id = ? AND lang IN ?", bookID, langs).
		Where("text LIKE ?", "%"+escapeLike(q)+"%").
		Order("segment").Limit(limit + 1).Find(&rows).Error; err != nil {
		writeServerErr(w, err)
		return
	}

	out := searchResp{Query: q, Hits: []searchHit{}}
	if len(rows) > limit {
		out.Limited = true
		rows = rows[:limit]
	}
	out.Total = len(rows)

	// The paragraph number travels with the hit so the result reads as a place
	// in the book rather than as a row id. Read from the segments themselves:
	// ref_translations carries a para_no too, but it is the aligned volume's
	// numbering, and the reader is looking at this book's.
	seqs := make([]int, 0, len(rows))
	for _, row := range rows {
		seqs = append(seqs, row.Segment)
	}
	type segMeta struct {
		Seq    int
		ParaNo int
		Kind   string
	}
	meta := map[int]segMeta{}
	if len(seqs) > 0 {
		var segs []store.TextSegment
		if err := s.db.WithContext(r.Context()).
			Select("seq", "para_no", "kind").
			Where("book_id = ? AND seq IN ?", bookID, seqs).
			Find(&segs).Error; err == nil {
			for _, sg := range segs {
				meta[sg.Seq] = segMeta{Seq: sg.Seq, ParaNo: sg.ParaNo, Kind: sg.Kind}
			}
		}
	}
	names := s.bookNames(r)
	for _, row := range rows {
		m := meta[row.Segment]
		out.Hits = append(out.Hits, searchHit{
			BookID: bookID, BookName: names[bookID], Segment: row.Segment,
			Para: m.ParaNo, Kind: m.Kind, Lang: row.Lang,
			Snippet: snippet(row.Text, q, 160),
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
