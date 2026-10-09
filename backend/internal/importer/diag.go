package importer

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/metanoia/pali-reader/backend/internal/corpus"
)

// Diagnostics for the ePitaka alignment.
//
// The importer reports one number — "N ePitaka lines walked, M found, K
// unplaced" — and a number that size says nothing about what to do next. The
// walk fails for reasons that need opposite answers: a line this book does not
// contain at all is not a defect and should be dropped silently, while a line
// that is right there in the text and was still missed is a bug in the search.
//
// So the same walk is run here with the failure classified. It reads the same
// sources the importer does and builds the same segments — everything below
// mirrors ImportText's own path, deliberately, because a diagnostic that
// measures a different pipeline measures nothing.

// LineFailure is why one ePitaka line did not yield a boundary.
type LineFailure string

const (
	// FailShort: fewer than two words, so no prefix can be tried.
	FailShort LineFailure = "short"
	// FailOvershoot: found, but only behind where the previous line ended —
	// the cursor stepped past it. sentences.go retries from the previous
	// line's start; placeLines does not.
	FailOvershoot LineFailure = "overshoot"
	// FailAfterSpan: the line's text is there, but past the end of the span
	// the paragraph was located at — the located paragraph is shorter than the
	// text it covers.
	FailAfterSpan LineFailure = "after-span"
	// FailAbsent: no two-to-four word prefix of the line occurs anywhere in
	// this book. The volume carries text this reader's book does not have;
	// there is nothing to place and nothing to fix.
	FailAbsent LineFailure = "absent"
	// FailFar: it occurs somewhere, but nowhere the walk could reach — inside
	// the paragraph's span yet behind the cursor, or outside both. A paragraph
	// located far from where it belongs, or a recurring formula.
	FailFar LineFailure = "far"
)

// BookDiag is one book's alignment, with the failures split by cause.
type BookDiag struct {
	Book    string
	Volumes []string

	Words int // length of this book's alignment word stream
	// EpiWords is the same measure for the ePitaka volumes this book is mapped
	// to. A volume carrying text the book does not have cannot place those
	// lines however good the search is, so this is the ceiling any aligner is
	// working under — and the reason a rate quoted without it means little.
	EpiWords int
	// VolumesOverlap is EpiWords/Words. Below 1 the book is larger than its
	// volumes; above 1 the volumes carry more than the book.
	VolumesOverlap float64

	Paras    int // ePitaka paragraphs walked
	Anchored int // ... that located in this book

	// Lines is every ePitaka line of every mapped volume. The walk below only
	// ever sees the lines of paragraphs that located, so Attempted is what it
	// actually looked at and NeverAttempted the rest — lines the old walk did
	// not fail on so much as never consider, and which the importer's "unplaced"
	// figure therefore leaves out entirely.
	Lines         int
	Attempted     int
	NeverAttempted int
	Placed        int // ... that produced a boundary
	Fail          map[LineFailure]int

	SeqsBefore, SeqsAfter int // segments before and after the sentence split

	// Exact and Strict are the anchor aligner run over the same lines, loose
	// and refusing every ambiguous opening. The gap between them is what
	// strictness costs.
	Exact  AnchorStats
	Strict AnchorStats
	// Diff is the alignment the importer uses now (align.go), over the same
	// lines and the same word stream.
	Diff AnchorStats
	// DiffBlocks is how many runs of identical words that alignment found, and
	// DiffWords how many words of the volume they cover. They are the diff's
	// own account of the region: a book whose blocks are few and small is a
	// book its volume does not much resemble.
	DiffBlocks, DiffWords int

	// Stack2/Stack4 are how many segments would hold more than ten lines under
	// them, allowing two-word anchors and requiring four-word ones. This is the
	// reader-visible defect (a dozen translations under one short paragraph)
	// measured without a database.
	Stack2, StackMax2 int
	Stack3, StackMax3 int
	Stack4, StackMax4 int

	// ByMinGram is how many segments hold more than ten anchored lines, split
	// by what the segment is. Prose is where stacking is the alignment's fault.
	Prose2, Prose3, Prose4 int
	Verse2, Verse3, Verse4 int

	// Safe is two-word anchors required to be carried by four more words: the
	// setting the importer used before the diff replaced it, kept so the two
	// can be read side by side.
	Safe      AnchorStats
	StackS    int
	StackMaxS int
	ProseS    int
	VerseS    int

	// StackD is the same measure for the alignment the importer now uses. It
	// is the correctness metric, not the coverage one: back when a line that
	// could not be placed was attached to wherever the reading had got to, the
	// worst prose segment held 601 translations. Nothing here may put more than
	// a handful under one prose segment.
	StackD    int
	StackMaxD int
	ProseD    int
	VerseD    int
	// StackKindD is what the segment holding the most anchored lines is. The
	// count on its own cannot say whether it is a defect: a verse is one
	// segment holding every row of a gatha, and a "centered" block is a gatha
	// that this edition did not mark as one. Both are the data being right; a
	// body paragraph holding a dozen lines is not.
	StackKindD string
}

// prosePile names the segment that carries the most anchored lines among those
// the stacking count calls prose, and says what the corpus calls it. The count
// on its own cannot say whether such a pile is a defect: a verse is one segment
// holding every row of a gatha, and a "centered" block is a gatha this edition
// did not mark as one. Only a body paragraph holding a dozen lines is the
// alignment's fault, so the count has to come with the label.
func prosePile(anchors []lineAnchor, owner []int, segs []segText) (kind string, count int) {
	per := map[int]int{}
	for _, a := range anchors {
		if a.At >= 0 && a.At < len(owner) {
			per[owner[a.At]]++
		}
	}
	for i, n := range per {
		if n <= count || i >= len(segs) || segs[i].Heading || segs[i].Verse {
			continue
		}
		count, kind = n, segs[i].Kind
	}
	return kind, count
}

// stackOf counts the segments that more than ten anchored lines fall on.
func stackOf(anchors []lineAnchor, owner []int, segs []segText) (over10, max, prose, verse int) {
	per := map[int]int{}
	for _, a := range anchors {
		if a.At >= 0 && a.At < len(owner) {
			per[owner[a.At]]++
		}
	}
	for i, n := range per {
		if n > 10 {
			over10++
			if i < len(segs) && segs[i].Heading {
				continue
			}
			if i < len(segs) && segs[i].Verse {
				verse++
			} else {
				prose++
			}
		}
		if n > max {
			max = n
		}
	}
	return over10, max, prose, verse
}

// Unplaced is the number of lines that produced no boundary.
func (b BookDiag) Unplaced() int {
	n := 0
	for _, v := range b.Fail {
		n += v
	}
	return n
}

// DiagOptions selects what to measure.
type DiagOptions struct {
	Books []string // nil means every mapped book
	Log   func(string, ...any)
}

// Diagnose runs both alignments over the mapped books and reports how they
// differ, line by line and cause by cause.
func Diagnose(tipitakaPath, epitakaPath string, opt DiagOptions) ([]BookDiag, error) {
	txtDB, err := openSQLite(tipitakaPath)
	if err != nil {
		return nil, err
	}
	defer txtDB.Close()

	sent, err := openSentences(epitakaPath)
	if err != nil {
		return nil, err
	}
	defer sent.Close()

	mapping := parseBookMap()
	books, err := bookIDsInOrder(txtDB)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, b := range opt.Books {
		want[strings.TrimSpace(b)] = true
	}

	var out []BookDiag
	for i, id := range books {
		targets := mapping[id]
		if len(targets) == 0 {
			continue
		}
		if len(want) > 0 && !want[id] {
			continue
		}
		if opt.Log != nil && i%10 == 0 {
			opt.Log("  [%d/%d] %s", i+1, len(books), id)
		}
		d, err := diagnoseBook(txtDB, sent, id, targets)
		if err != nil {
			return nil, fmt.Errorf("book %s: %w", id, err)
		}
		out = append(out, d)
	}
	return out, nil
}

// bookIDsInOrder lists the books the way ImportText does: basket, collection,
// then volume. A report ordered differently from the importer would be a second
// thing to keep in step.
func bookIDsInOrder(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT id, basket, COALESCE(category,'') FROM books`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type rec struct{ id, basket, category string }
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.basket, &r.category); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	rank := map[string]int{"Mula": 0, "Attha": 1, "Tika": 2, "Annya": 3}
	sort.SliceStable(recs, func(i, j int) bool {
		if rank[recs[i].basket] != rank[recs[j].basket] {
			return rank[recs[i].basket] < rank[recs[j].basket]
		}
		if recs[i].category != recs[j].category {
			return recs[i].category < recs[j].category
		}
		return recs[i].id < recs[j].id
	})
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.id)
	}
	return out, nil
}

// segmentsFor rebuilds one book's segments the way ImportText builds them:
// parse the pages, cut the prose at ePitaka's sentence boundaries and the verse
// at the lines the edition prints, renumber.
func segmentsFor(db *sql.DB, sent *sentenceSource, bookID string) ([]corpus.Segment, error) {
	rows, err := db.Query(`SELECT page, COALESCE(content,'') FROM pages WHERE bookid=? ORDER BY page`, bookID)
	if err != nil {
		return nil, err
	}
	var sb strings.Builder
	var starts []int
	for rows.Next() {
		var page int
		var content string
		if err := rows.Scan(&page, &content); err != nil {
			rows.Close()
			return nil, err
		}
		starts = append(starts, sb.Len())
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	rows.Close()
	if sb.Len() == 0 {
		return nil, nil
	}
	segs := corpus.ParseBook(sb.String(), starts)
	if len(segs) == 0 {
		return nil, nil
	}
	if cuts := sent.cutsFor(bookID, segs); len(cuts) > 0 {
		if grown := corpus.SplitSegments(segs, cuts); len(grown) > len(segs) {
			segs = grown
		}
	}
	return segs, nil
}

func diagnoseBook(txtDB *sql.DB, sent *sentenceSource, bookID string, targets []string) (BookDiag, error) {
	d := BookDiag{Book: bookID, Volumes: targets, Fail: map[LineFailure]int{}}

	segs, err := segmentsFor(txtDB, sent, bookID)
	if err != nil {
		return d, err
	}
	d.SeqsAfter = len(segs)
	texts := make([]segText, 0, len(segs))
	for _, s := range segs {
		w := alignWords(s.Text)
		if len(w) == 0 {
			continue
		}
		texts = append(texts, segText{
			Seq: s.Seq, Para: s.ParaNo, Words: w,
			Heading: s.Kind == corpus.KindHeading, Verse: s.Kind == corpus.KindVerse,
			Kind: s.Kind,
		})
	}

	var flat []string
	var owner []int
	for i, s := range texts {
		for _, w := range s.Words {
			flat = append(flat, w)
			owner = append(owner, i)
		}
	}
	d.Words = len(flat)
	if len(flat) < shingle {
		return d, nil
	}
	_ = owner

	// Every line the volumes hold, so the walk can be measured against the
	// whole of what there was to place rather than only against what it tried.
	lines := loadEpiLines(sent.db, nil, targets)
	d.Lines = len(lines)
	for _, ln := range lines {
		d.EpiWords += len(ln.words)
	}
	if d.Words > 0 {
		d.VolumesOverlap = float64(d.EpiWords) / float64(d.Words)
	}

	// The walk under test.
	d.Paras, d.Anchored = locateParas(sent.db, nil, targets, flat, wordIndex(flat),
		func(pos, end int, lines []epiLine) {
			cur := pos
			prev := pos - 1
			for _, ln := range lines {
				if len(ln.words) == 0 {
					continue
				}
				d.Attempted++
				at := matchLine(flat, ln.words, cur, end)
				if at < 0 {
					if from := prev + 1; from < cur && from < end {
						at = matchLine(flat, ln.words, from, end)
					}
				}
				if at >= 0 {
					d.Placed++
					prev = at
					cur = at + len(ln.words)
					if cur > end {
						cur = end
					}
					continue
				}
				d.Fail[classifyFail(flat, ln.words, end, prev)]++
			}
		})
	d.NeverAttempted = d.Lines - d.Attempted

	// The exact aligner, loose and strict, for the comparison this diagnostic
	// exists to make; and the safe setting the importer used before the diff
	// replaced it.
	_, stats := anchorLinesFrom(flat, lines, AnchorOptions{MinGram: 2})
	_, strict := anchorLinesFrom(flat, lines, AnchorOptions{MinGram: 4})
	safeAnchors, safe := anchorLinesFrom(flat, lines, AnchorOptions{MinGram: minAnchorGram, Safe: true})
	d.Exact = stats
	d.Strict = strict
	d.Safe = safe

	// The alignment the importer now uses, on the same lines and the same word
	// stream.
	diffAnchors, diffStats := alignByDiff(flat, lines)
	d.Diff = diffStats
	d.DiffBlocks, d.DiffWords = diffStats.Blocks, diffStats.BlockWords

	// What each of them would leave under one segment. A translation is written
	// per anchored line and joined under the segment that owns it, so the
	// number of anchors on a segment is the number of sentences that would
	// appear beneath it — which is the defect being measured, and it can be
	// counted here without a database.
	over, maxS, prS, vsS := stackOf(safeAnchors, owner, texts)
	d.StackS, d.StackMaxS, d.ProseS, d.VerseS = over, maxS, prS, vsS
	d.StackD, d.StackMaxD, d.ProseD, d.VerseD = stackOf(diffAnchors, owner, texts)
	if k, n := prosePile(diffAnchors, owner, texts); n > 10 && d.ProseD > 0 {
		d.StackKindD = k
	}
	for _, g := range []int{2, 3, 4} {
		anchors, _ := anchorLinesFrom(flat, lines, AnchorOptions{MinGram: g})
		over, max, prose, verse := stackOf(anchors, owner, texts)
		switch g {
		case 2:
			d.Stack2, d.StackMax2, d.Prose2, d.Verse2 = over, max, prose, verse
		case 3:
			d.Stack3, d.StackMax3, d.Prose3, d.Verse3 = over, max, prose, verse
		default:
			d.Stack4, d.StackMax4, d.Prose4, d.Verse4 = over, max, prose, verse
		}
	}
	return d, nil
}

// classifyFail works out which cause a missed line belongs to, by asking where
// its text actually is in the book.
func classifyFail(flat, words []string, end, prev int) LineFailure {
	if len(words) < 2 {
		return FailShort
	}
	at := matchLine(flat, words, 0, len(flat))
	if at < 0 {
		return FailAbsent
	}
	if at < prev {
		return FailOvershoot
	}
	if at >= end {
		return FailAfterSpan
	}
	return FailFar
}
