package importer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/metanoia/pali-reader/backend/internal/store"
)

// A read-only account of what the last import produced.
//
// The importer's own log says how many segments it wrote; it cannot say whether
// what it wrote is any good. The failure that matters is visible to a reader
// rather than to a count: several sentences of translation stacked under one
// short segment, because a line that could not be placed was attached to
// wherever the reading had got to. So this measures that directly — how many
// Chinese sentences sit under each segment — and reports the worst of them,
// which is the number to watch when the alignment changes.
//
// Nothing here writes. It is the step to run before and after an import whose
// effect has to be argued rather than asserted.

// StackReport is what one language's translations look like per segment.
type StackReport struct {
	Lang       string
	Rows       int
	Segments   int
	Books      int
	MaxStack   int
	Over10     int
	Over5      int
	Over1      int
	WorstBooks []BookStack
	// ByKind is the same count split by what the segment is. A verse is the
	// case that must be read apart from the rest: ePitaka keeps one row per
	// line of a gatha, and a gatha line is not a sentence, so several of its
	// translations under one verse segment is the data being right rather than
	// a translation piled in the wrong place. Prose is where stacking means the
	// alignment failed.
	ByKind map[string]int
}

// BookStack is the stacking count for one book.
type BookStack struct {
	Book   string
	Over10 int
	Max    int
}

// ReportStacking measures how many sentences of translation sit under each
// segment, per language, and names the books where the most are stacked.
func ReportStacking(g *store.DB, langs []string, log func(string, ...any)) error {
	for _, lang := range langs {
		var rows []store.RefTranslation
		if err := g.Select("book_id", "segment", "text").
			Where("lang = ?", lang).Order("book_id, segment").Find(&rows).Error; err != nil {
			return fmt.Errorf("reading %s translations: %w", lang, err)
		}

		rep := StackReport{Lang: lang, Rows: len(rows), ByKind: map[string]int{}}
		kind := segmentKinds(g, rows)
		segments := map[string]bool{}
		perBook := map[string]*BookStack{}
		for _, r := range rows {
			key := fmt.Sprintf("%s:%d", r.BookID, r.Segment)
			segments[key] = true
			n := len(splitSentences(r.Text))
			bs := perBook[r.BookID]
			if bs == nil {
				bs = &BookStack{Book: r.BookID}
				perBook[r.BookID] = bs
			}
			if n > bs.Max {
				bs.Max = n
			}
			if n > rep.MaxStack {
				rep.MaxStack = n
			}
			if n > 1 {
				rep.Over1++
			}
			if n > 5 {
				rep.Over5++
			}
			if n > 10 {
				rep.Over10++
				bs.Over10++
				rep.ByKind[kind[key]]++
			}
		}
		rep.Segments = len(segments)
		rep.Books = len(perBook)
		for _, bs := range perBook {
			if bs.Over10 > 0 {
				rep.WorstBooks = append(rep.WorstBooks, *bs)
			}
		}
		sort.Slice(rep.WorstBooks, func(i, j int) bool {
			if rep.WorstBooks[i].Over10 != rep.WorstBooks[j].Over10 {
				return rep.WorstBooks[i].Over10 > rep.WorstBooks[j].Over10
			}
			return rep.WorstBooks[i].Max > rep.WorstBooks[j].Max
		})

		log("  %s: %d rows over %d segments in %d books", lang, rep.Rows, rep.Segments, rep.Books)
		log("    segments holding more than 10 sentences: %d   more than 5: %d   more than 1: %d   worst: %d",
			rep.Over10, rep.Over5, rep.Over1, rep.MaxStack)
		var kinds []string
		for k, n := range rep.ByKind {
			kinds = append(kinds, fmt.Sprintf("%s=%d", k, n))
		}
		sort.Strings(kinds)
		log("      of those, by what the segment is: %s", strings.Join(kinds, " "))
		if len(rep.WorstBooks) > 0 {
			var parts []string
			for i, b := range rep.WorstBooks {
				if i >= 8 {
					break
				}
				parts = append(parts, fmt.Sprintf("%s(%d,max %d)", b.Book, b.Over10, b.Max))
			}
			log("    worst books: %s", strings.Join(parts, " "))
		}
	}
	return nil
}

// segmentKinds maps "book:segment" to the segment's kind, so the stacking count
// can say whether a pile is in prose (the alignment failed) or in a verse
// (ePitaka keeps one row per gatha line, so several translations under one
// verse segment is the data being right).
func segmentKinds(g *store.DB, rows []store.RefTranslation) map[string]string {
	out := make(map[string]string, len(rows))
	byBook := map[string][]int{}
	for _, r := range rows {
		byBook[r.BookID] = append(byBook[r.BookID], r.Segment)
	}
	for book, segs := range byBook {
		var found []store.TextSegment
		if err := g.Select("seq", "kind").Where("book_id = ? AND seq IN ?", book, segs).
			Find(&found).Error; err != nil {
			continue
		}
		for _, s := range found {
			out[fmt.Sprintf("%s:%d", book, s.Seq)] = s.Kind
		}
	}
	return out
}
