package importer

import (
	"database/sql"
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/metanoia/pali-reader/backend/internal/corpus"
)

// Sentence boundaries for the corpus.
//
// A CST paragraph in this reader is one row of text_segments, and a prose
// paragraph of the Saṃyutta runs to ten sentences. Everything the reader hangs
// off a segment — a word pick, an annotation, their own translation of it —
// therefore hangs off the whole passage, and the reader cannot work sentence by
// sentence. ePitaka publishes the canon one sentence per row, so the boundaries
// already exist upstream; the work here is finding where each of them falls in
// this reader's text.
//
// The two editions paragraph differently and their numbering does not join:
// ePitaka's para_id is its own counter, not the canon's § number. What they do
// share is the Pāḷi, so the boundaries are found the same way the reference
// translations are — by locating ePitaka's rows in this reader's word stream,
// reusing alignWords, locate and matchLine rather than inventing a second
// aligner that could disagree with them.
//
// A boundary that cannot be found is skipped and the text stays where it is.
// That is the whole safety argument: an unaligned sentence costs a longer
// paragraph, never a lost one.

// sentenceSource is the ePitaka side of the corpus import: the database the
// sentence rows come from, and the volume correspondence that says which of its
// books each of ours is. It also carries what the split did, so a run can report
// it without every book printing a line of its own.
type sentenceSource struct {
	db      *sql.DB
	mapping map[string][]string

	before int // segments as parsed
	after  int // segments after splitting
	books  int // books that gained at least one segment
	noMap  int // books with no ePitaka volume to align against

	// What the alignment did, in the terms the answer needs. A line that could
	// not be placed is one of two things and they are counted apart: text this
	// book does not have, which costs a longer paragraph and nothing else, and
	// text it has but not where the line begins, which is the class to look at
	// first if a cut ever lands in the wrong place. See align.go.
	lines     int
	placed    int
	unplaced  int
	unmatched int
	shifted   int
}

func openSentences(path string) (*sentenceSource, error) {
	db, err := openSQLite(path)
	if err != nil {
		return nil, fmt.Errorf("sentence source %s: %w", path, err)
	}
	return &sentenceSource{db: db, mapping: parseBookMap()}, nil
}

func (s *sentenceSource) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// counts records what the split did to one book.
func (s *sentenceSource) counts(before, after int) {
	if s == nil {
		return
	}
	s.before += before
	s.after += after
	if after > before {
		s.books++
	}
}

// cutsFor returns, for each of a book's segments, the byte offsets in its text
// at which a new piece begins.
//
// A nil entry — or a nil result — means the segment is left exactly as it is,
// which is the answer for every segment whose sentences could not be aligned.
func (s *sentenceSource) cutsFor(bookID string, segs []corpus.Segment) [][]int {
	if s == nil || s.db == nil {
		return nil
	}
	targets := s.mapping[bookID]
	if len(targets) == 0 {
		s.noMap++
		return nil
	}

	starts, offsets, flat, cuts := s.wordStream(segs)
	if len(flat) < anchorGram {
		return nil
	}

	lines := loadEpiLines(s.db, nil, targets)
	// The same alignment the reference translations are filed by — the same
	// function, on the same word stream, over the same lines. A boundary and
	// the translation that sits under it are therefore one decision and cannot
	// disagree; see align.go.
	anchors, stats := alignByDiff(flat, lines)
	s.lines += stats.Lines
	s.placed += stats.Anchored
	s.unplaced += stats.Unplaced()
	s.unmatched += stats.Unmatched
	s.shifted += stats.Shifted

	// Which verse segments the alignment put at least one of its lines in. A
	// verse is cut from that fact and not from the line's byte position: the
	// position of a line's first word is inside the line, after the verse
	// number this edition writes at the head of the gatha (and after an opening
	// quote), and cutting there would leave `13.` alone as a segment and hand
	// the numeral to nobody. See verseLines.
	verse := make([]bool, len(segs))

	// A boundary is a place where a line of ePitaka begins. The anchors are
	// already in reading order and already monotone, so the only thing left is
	// to refuse one that would not actually divide anything.
	last := -1
	for _, a := range anchors {
		if a.At <= last {
			continue
		}
		last = a.At
		if i := ownerAt(starts, a.At); i >= 0 && segs[i].Kind == corpus.KindVerse {
			// A verse line placed here, whether or not the walk had to step
			// over a numeral to find it. Both kinds count: a line whose number
			// ePitaka writes and this edition does not is still a line the
			// translation belongs to, and stepping over that number is exactly
			// what AnchorStats.Numbered records.
			verse[i] = true
			continue
		}
		// A line placed after stepping over a leading numeral begins, here, at
		// its first word rather than at the number in front of it. Cutting
		// there would leave the number at the end of the sentence before it,
		// where it reads as part of that sentence — so no cut is taken from
		// such an anchor. The cost is one paragraph left whole; see align.go.
		if a.SkippedNum > 0 {
			continue
		}
		recordCut(segs, starts, offsets, cuts, a.At)
	}

	// A verse the alignment reached is divided at every line the edition
	// prints, not only at the lines an ePitaka row happened to start on. The
	// lines are in the text already — the edition separates them with \n and
	// the break is content, not whitespace (rule 8) — and a line whose own row
	// could not be placed is still a line the beginner reads: leaving it joined
	// to its neighbour would put two lines under one translation and give the
	// reader a paragraph where the text has a line. Nothing is cut where the
	// alignment placed nothing at all: an unreached verse stays whole.
	for i := range segs {
		if !verse[i] {
			continue
		}
		cuts[i] = append(cuts[i], verseLines(segs[i].Text)...)
	}
	return cuts
}

// verseLines returns the byte offset at which each line of a verse begins: the
// first byte after every line break.
//
// The break itself is left with the line it ends — SplitAt trims the edges of
// every piece — so each piece comes out as exactly one printed line with no
// break left inside it, and the pieces joined back together are the verse.
func verseLines(text string) []int {
	var out []int
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			out = append(out, i+1)
		}
	}
	return out
}

// recordCut turns a position in the book's word stream into a byte offset in the
// segment's own text, when that position is the start of a sentence the reader
// will actually be given as its own segment.
func recordCut(segs []corpus.Segment, starts []int, offsets [][]int, cuts [][]int, at int) {
	i := ownerAt(starts, at)
	if i < 0 {
		return
	}
	// Verses are already one gāthā per segment and a heading is one line.
	// ePitaka's verse rows are one *line* of a gāthā — the Dhammapada comes out
	// as three rows for a three-line verse — so cutting there would take a verse
	// apart at its own line breaks, which is not a sentence boundary at all.
	if segs[i].Kind != corpus.KindProse {
		return
	}
	local := at - starts[i]
	if local <= 0 || local >= len(offsets[i]) {
		return
	}
	floor := 0
	if n := len(cuts[i]); n > 0 {
		floor = cuts[i][n-1]
	}
	cuts[i] = append(cuts[i], sentenceStart(segs[i].Text, offsets[i][local], floor))
}

// wordStream flattens a book's segments into one word stream, exactly as the
// reference aligner does, and keeps what is needed to get back from a position
// in that stream to a place in the text: where each segment starts, and where
// each of its words starts.
func (s *sentenceSource) wordStream(segs []corpus.Segment) (starts []int, offsets [][]int, flat []string, cuts [][]int) {
	starts = make([]int, len(segs))
	offsets = make([][]int, len(segs))
	cuts = make([][]int, len(segs))
	for i, seg := range segs {
		starts[i] = len(flat)
		words, offs := alignWordOffsets(seg.Text)
		offsets[i] = offs
		flat = append(flat, words...)
	}
	return starts, offsets, flat, cuts
}

// ownerAt finds the segment a word position belongs to: the last one that starts
// at or before it. Segments with no words of their own share a start with the
// next, and are never the owner of anything.
func ownerAt(starts []int, at int) int {
	lo, hi := 0, len(starts)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if starts[mid] <= at {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo - 1
}

// sentenceStart walks back from the first word of a sentence over the marks that
// open one — spacing, quotes, dashes, brackets — so the quote or the dash that
// introduces a speech stays with the speech instead of being left dangling at
// the end of the sentence before it.
//
// The canon writes `…etadavoca – "‘kathaṃ nu tvaṃ…`; cutting at the word alone
// would put `– "‘` on the wrong side of the line, where it reads as a trailing
// dash and a quotation that never opens.
func sentenceStart(text string, wordOff, floor int) int {
	if wordOff > len(text) {
		wordOff = len(text)
	}
	at := wordOff
	for at > floor {
		r, size := utf8.DecodeLastRuneInString(text[:at])
		if !isOpener(r) {
			break
		}
		if at-size < floor {
			break
		}
		at -= size
	}
	return at
}

func isOpener(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case '"', '\'', '\u2018', '\u201c', '\u00ab', '\u00bb', '\u300c', '\u300e',
		'\uff08', '(', '[', '\u3010', '\u3008', '\u300a', '\u2013', '\u2014', '\u2212', '-', '\u2026':
		return true
	}
	return false
}
