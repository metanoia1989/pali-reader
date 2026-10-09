package importer

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/metanoia/pali-reader/backend/internal/corpus"
	"github.com/metanoia/pali-reader/backend/internal/store"
	"github.com/metanoia/pali-reader/backend/internal/tokenize"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// txt is the tipitaka_pali.db handle the text and dictionary-extra importers
// share.
type txt struct{ db *sql.DB }

func openSQLite(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		return nil, err
	}
	// The corpus files are read in one long sequential pass; a single
	// connection keeps the page cache warm and avoids lock churn.
	db.SetMaxOpenConns(1)
	return db, nil
}

// ImportCatalog refreshes only the categories and the book list.
//
// Renaming a division, or fixing how a title is rendered in Chinese, should not
// cost a five-minute rebuild of the whole corpus.
func ImportCatalog(path string, g *store.DB, log func(string, ...any)) error {
	db, err := openSQLite(path)
	if err != nil {
		return err
	}
	defer db.Close()
	cat := &txt{db}
	if err := importCategories(cat, g, log); err != nil {
		return err
	}
	_, err = importBooks(cat, g, log)
	return err
}

// ImportText reads the canon out of tipitaka_pali.db and writes categories,
// books, segments and the table of contents.
//
// epitakaPath is the Pāḷi side of ePitaka, whose sentences table is the corpus's
// own statement of where its sentences begin. It is read here — not in a later
// step — because the boundaries decide how many segments a paragraph becomes,
// and the token offsets and the TOC are both built after that decision.
//
// knownKeys is the set of lookup keys already in the dictionary: it decides
// which words the reader underlines. It may be nil, in which case nothing is
// underlined and the reader underlines words as it meets them.
func ImportText(path, epitakaPath string, g *store.DB, log func(string, ...any)) error {
	db, err := openSQLite(path)
	if err != nil {
		return err
	}
	defer db.Close()
	cat := &txt{db}

	sent, err := openSentences(epitakaPath)
	if err != nil {
		return err
	}
	defer sent.Close()

	if err := importCategories(cat, g, log); err != nil {
		return err
	}
	books, err := importBooks(cat, g, log)
	if err != nil {
		return err
	}
	for i, b := range books {
		log("  [%d/%d] %s %s", i+1, len(books), b.ID, b.Name)
		if err := importBook(cat, g, b, sent, log); err != nil {
			return fmt.Errorf("book %s: %w", b.ID, err)
		}
	}
	log("  sentences: %d segments became %d (%d books gained one, %d books have no ePitaka volume)",
		sent.before, sent.after, sent.books, sent.noMap)
	// Anchored lines are lines of the ePitaka volumes whose wording was found
	// in this book. The unplaced ones are lines of text this reader's book does
	// not carry, so they cost a longer paragraph and nothing else — they are
	// reported rather than hidden because a sudden change in that number is how
	// a broken alignment would show itself.
	log("  sentences: %d ePitaka lines in the mapped volumes, %d placed, %d unplaced (%.2f%%): %d not text this book has, %d beginning on text that differs",
		sent.lines, sent.placed, sent.unplaced,
		100*float64(sent.unplaced)/float64(max(sent.lines, 1)), sent.unmatched, sent.shifted)
	return nil
}

func importCategories(cat *txt, g *store.DB, log func(string, ...any)) error {
	rows, err := cat.db.Query(`SELECT id, name, basket FROM category`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var out []store.TextCategory
	for rows.Next() {
		var c store.TextCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.Basket); err != nil {
			return err
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	for i := range out {
		out[i].Sort = i
		out[i].NameZh = CategoryNameZh(out[i].ID, out[i].Name)
		out[i].NamePi = CategoryNamePi(out[i].ID, out[i].Name)
	}
	if len(out) == 0 {
		return nil
	}
	log("  categories: %d", len(out))
	return g.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(out, 50).Error
}

func importBooks(cat *txt, g *store.DB, log func(string, ...any)) ([]store.TextBook, error) {
	rows, err := cat.db.Query(`SELECT id, basket, COALESCE(category,''), name, pagecount FROM books`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.TextBook
	for rows.Next() {
		var b store.TextBook
		if err := rows.Scan(&b.ID, &b.Basket, &b.Category, &b.Name, &b.PageCount); err != nil {
			return nil, err
		}
		b.NameZh = ChineseName(b.Name)
		out = append(out, b)
	}
	// Reading order is basket, then collection, then volume.
	rank := map[string]int{store.BasketMula: 0, store.BasketAttha: 1, store.BasketTika: 2, store.BasketAnnya: 3}
	sort.Slice(out, func(i, j int) bool {
		if rank[out[i].Basket] != rank[out[j].Basket] {
			return rank[out[i].Basket] < rank[out[j].Basket]
		}
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].ID < out[j].ID
	})
	for i := range out {
		out[i].Sort = i
	}
	if len(out) == 0 {
		return nil, nil
	}
	log("  books: %d", len(out))
	err = g.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(out, 100).Error
	return out, err
}

// importBook replaces one book's segments and TOC. Delete-then-insert keeps a
// re-import idempotent even when the upstream paragraphing shifts.
func importBook(cat *txt, g *store.DB, b store.TextBook, sent *sentenceSource, log func(string, ...any)) error {
	pageRows, err := cat.db.Query(`SELECT page, COALESCE(content,'') FROM pages WHERE bookid=? ORDER BY page`, b.ID)
	if err != nil {
		return err
	}
	var sb strings.Builder
	var starts []int
	for pageRows.Next() {
		var page int
		var content string
		if err := pageRows.Scan(&page, &content); err != nil {
			pageRows.Close()
			return err
		}
		starts = append(starts, sb.Len())
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	pageRows.Close()
	if sb.Len() == 0 {
		log("    (no pages, skipped)")
		return nil
	}

	segs := corpus.ParseBook(sb.String(), starts)
	if len(segs) == 0 {
		return nil
	}

	// One paragraph becomes one segment per sentence, and one gāthā one segment
	// per line, before anything is derived from the text. Splitting first is
	// what keeps this honest: the tokenisation below runs on each piece's own
	// text through the one implementation, so there is no offset arithmetic to
	// get wrong, and the token index a reader's pick is anchored to is the token
	// of the sentence or the line they were reading.
	before := len(segs)
	if cuts := sent.cutsFor(b.ID, segs); len(cuts) > 0 {
		if grown := corpus.SplitSegments(segs, cuts); len(grown) > before {
			segs = grown
		}
	}
	sent.counts(before, len(segs))

	// Resolve which words have dictionary entries, for the underline.
	known, err := knownKeys(g, segs)
	if err != nil {
		return err
	}

	rows := make([]store.TextSegment, 0, len(segs))
	var toc []store.TextTOC
	sectionID := ""
	sectionLvl := 0
	for i, s := range segs {
		text := s.Text
		// Headings are tokenised too. They used to be excluded, on the
		// reasoning that a heading is a label rather than text — but a heading
		// is where a reader meets a word they do not know (`oghataraṇasuttaṃ`,
		// `naḷavaggo`), and with no tokens the reader could not tap any of it.
		// A number is not a word and tokenize skips it, so `1. naḷavaggo`
		// yields exactly one clickable word.
		toks := tokenize.Split(text)
		tokenize.Mark(toks, known)
		var tokJSON string
		if len(toks) > 0 {
			tokJSON = encodeTokens(text, toks)
		}
		var markers string
		if len(s.Markers) > 0 {
			b, _ := json.Marshal(s.Markers)
			markers = string(b)
		}
		var variants string
		if len(s.Variants) > 0 {
			// [[offset, text], ...] — a positional pair per reading, matching
			// the token encoding so the client has one slicing convention.
			pairs := make([][2]any, 0, len(s.Variants))
			for _, v := range s.Variants {
				pairs = append(pairs, [2]any{v.Offset, v.Text})
			}
			b, _ := json.Marshal(pairs)
			variants = string(b)
		}

		// [[offset, length], ...] — the runs the edition sets in bold. The
		// markup is stripped from the text, so without this the commentary's
		// headwords lose the emphasis that marks them as headwords.
		var bold string
		if len(s.Bold) > 0 {
			spans := make([][2]int, 0, len(s.Bold))
			for _, sp := range s.Bold {
				spans = append(spans, [2]int{sp.Offset, sp.Length})
			}
			b, _ := json.Marshal(spans)
			bold = string(b)
		}

		if s.TocName != "" {
			// The heading is a segment in its own right, so the entry points
			// at it; the section a body segment belongs to is tracked below.
			sectionID = fmt.Sprintf("%s-%d", b.ID, i+1)
			sectionLvl = s.TocLevel
			toc = append(toc, store.TextTOC{
				BookID:     b.ID,
				Seq:        len(toc) + 1,
				Name:       s.TocName,
				Level:      s.TocLevel,
				Kind:       "heading",
				ParaNo:     s.ParaNo,
				PageNo:     s.PageNo,
				SegmentSeq: i + 1,
			})
		}

		rows = append(rows, store.TextSegment{
			BookID:     b.ID,
			Seq:        i + 1,
			ParaNo:     s.ParaNo,
			PageNo:     s.PageNo,
			Kind:       s.Kind,
			Text:       text,
			HTML:       s.HTML,
			Markers:    markers,
			Variants:   variants,
			Bold:       bold,
			CharLen:    len([]rune(text)),
			Tokens:     tokJSON,
			SectionID:  sectionID,
			SectionLvl: sectionLvl,
		})
	}

	err = g.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("book_id = ?", b.ID).Delete(&store.TextSegment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("book_id = ?", b.ID).Delete(&store.TextTOC{}).Error; err != nil {
			return err
		}
		if err := tx.CreateInBatches(rows, 400).Error; err != nil {
			return err
		}
		if len(toc) > 0 {
			if err := tx.CreateInBatches(toc, 400).Error; err != nil {
				return err
			}
		}
		return tx.Model(&store.TextBook{}).Where("id = ?", b.ID).
			Update("seg_count", len(rows)).Error
	})
	return err
}

// knownKeys collects the dictionary keys present in a book's segments with one
// query per chunk, so a whole book costs a handful of round trips rather than
// one per word.
func knownKeys(g *store.DB, segs []corpus.Segment) (map[string]struct{}, error) {
	seen := map[string]struct{}{}
	var keys []string
	for _, s := range segs {
		for _, t := range tokenize.Split(s.Text) {
			if t.Key == "" {
				continue
			}
			if _, ok := seen[t.Key]; ok {
				continue
			}
			seen[t.Key] = struct{}{}
			keys = append(keys, t.Key)
		}
	}
	out := make(map[string]struct{}, len(keys))
	const chunk = 5000
	for i := 0; i < len(keys); i += chunk {
		j := i + chunk
		if j > len(keys) {
			j = len(keys)
		}
		var found []string
		if err := g.Model(&store.DictLookup{}).
			Where("lookup_key IN ?", keys[i:j]).
			Pluck("lookup_key", &found).Error; err != nil {
			return nil, err
		}
		for _, k := range found {
			out[k] = struct{}{}
		}
	}
	return out, nil
}

// encodeTokens writes the tokenisation as [[offset, length, flags], ...].
//
// Two things about this format are load-bearing.
//
// The shape: a positional triple, not an object per token. The reader slices the
// segment's own text with it, so the payload stays close to the size of the text
// and the client never needs the word spelled out. internal/server decodes
// straight into [][3]int, so an object-per-token encoding silently produced
// empty words in the reader rather than an error.
//
// The unit: offsets are in UTF-16 code units, not bytes. The tokeniser works in
// bytes, but the reader slices a JavaScript string, where indexing is by UTF-16
// unit. Every Pāḷi diacritic outside Latin-1 — ā ī ū ṃ ṭ ḍ ṇ ḷ ṅ ñ — is two or
// three bytes but one unit, so byte offsets drift further out of step with every
// accented letter and the underlines land on the wrong words.
//
// Astral characters would be two units, which is why the length comes from
// utf16.RuneLen rather than from a rune count.
// utf16Units is how many UTF-16 code units a rune occupies: two above the BMP,
// one otherwise. Written out rather than taken from a helper because the two
// candidate helpers live in different packages and only one of them exists in
// every supported Go version.
func utf16Units(r rune) int {
	if r > 0xFFFF {
		return 2
	}
	return 1
}

func encodeTokens(text string, toks []tokenize.Token) string {
	if len(toks) == 0 {
		return "[]"
	}
	need := make(map[int]bool, len(toks)*2)
	for _, t := range toks {
		need[t.Off] = true
		need[t.Off+t.Len] = true
	}
	// units[i] is the UTF-16 offset of the rune starting at byte i.
	units := make(map[int]int, len(need))
	u := 0
	for i, r := range text {
		if need[i] {
			units[i] = u
		}
		u += utf16Units(r)
	}
	if need[len(text)] {
		units[len(text)] = u
	}

	out := make([][3]int, 0, len(toks))
	for _, t := range toks {
		start, okStart := units[t.Off]
		end, okEnd := units[t.Off+t.Len]
		if !okStart || !okEnd {
			// The token does not lie on rune boundaries of this text, which
			// means the text and the tokenisation disagree. Emit nothing for
			// it rather than a span that would slice mid-character.
			continue
		}
		out = append(out, [3]int{start, end - start, t.Flags})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
}
