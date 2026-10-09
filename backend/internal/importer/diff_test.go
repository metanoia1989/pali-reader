package importer

import (
	"fmt"
	"strings"
	"testing"
)

// ids turns words into the dense ids the diff works on, so a test can be
// written in words.
func ids(words ...string) []uint32 {
	r := internWords(words)
	return r[0]
}

// blocksOf aligns two space-separated streams and returns the runs as
// "a:b:n" strings, which is what a failure has to show.
func blocksOf(a, b string) []string {
	in := internWords(strings.Fields(a), strings.Fields(b))
	blocks := matchBlocks(in[0], in[1]).Blocks
	out := make([]string, 0, len(blocks))
	for _, blk := range blocks {
		out = append(out, fmt.Sprintf("%d:%d:%d", blk.AO, blk.BO, blk.N))
	}
	return out
}

func TestMatchBlocksFindsTheRunsAroundADifference(t *testing.T) {
	for _, tc := range []struct{ a, b, want string }{
		// The ordinary case: the same words.
		{"a b c d", "a b c d", "0:0:4"},
		// A substitution in the middle is stepped over, and the text on either
		// side of it is matched whole.
		{"a b c d e f", "a b x y e f", "0:0:2 4:4:2"},
		// Words the volume has and the book does not.
		{"a b c d", "a b x y z c d", "0:0:2 2:5:2"},
		// Words the book has and the volume does not.
		{"a b x y z c d", "a b c d", "0:0:2 5:2:2"},
		// A whole line the book does not carry, between two it does.
		{"a b c d e f g h", "a b c d Q Q Q Q e f g h", "0:0:4 4:8:4"},
		// Nothing in common.
		{"a b c", "x y z", ""},
		// The ends of both streams are the same run, which is the case the
		// prefix/suffix trim is for: it has to come back as one block and not
		// be split around an interior match.
		{"a b c d e f g h i j", "a b c d e f g h i j", "0:0:10"},
	} {
		got := strings.Join(blocksOf(tc.a, tc.b), " ")
		if got != tc.want {
			t.Errorf("matchBlocks(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

// The blocks are what carries a position from one stream to the other, so the
// three properties they must have are worth a test of their own: each is a run
// of equal words, they never overlap, and they come in the order they are read.
// A block that went backwards would let a translation be filed under a segment
// that comes after its own text.
func TestMatchBlocksAreOrderedEqualAndDisjoint(t *testing.T) {
	texts := []string{
		"a b c d e f g h i j k l m n o p",
		"a b X c d e Y f g h",
		"a b c d e f g h i j k l m n o p q r s t",
		"z z z a b z z a b c",
		"",
		"a",
		"a a a a a a a a a a a a",
		"p o n m l k j i h g f e d c b a",
	}
	for _, x := range texts {
		for _, y := range texts {
			in := internWords(strings.Fields(x), strings.Fields(y))
			a, b := in[0], in[1]
			blocks := matchBlocks(a, b).Blocks
			prevA, prevB := 0, 0
			for i, blk := range blocks {
				if blk.AO < 0 || blk.BO < 0 || blk.AO+blk.N > len(a) || blk.BO+blk.N > len(b) {
					t.Fatalf("%q vs %q: block %+v is out of range", x, y, blk)
				}
				if i > 0 && (blk.AO < prevA || blk.BO < prevB) {
					t.Fatalf("%q vs %q: block %+v overlaps or precedes the one before it (ended at %d/%d)",
						x, y, blk, prevA, prevB)
				}
				if !equalWords(a[blk.AO:blk.AO+blk.N], b[blk.BO:blk.BO+blk.N]) {
					t.Fatalf("%q vs %q: block %+v does not hold equal words", x, y, blk)
				}
				if blk.N <= 0 {
					t.Fatalf("%q vs %q: empty block %+v", x, y, blk)
				}
				prevA, prevB = blk.AO+blk.N, blk.BO+blk.N
			}
		}
	}
}

// The three ways a line can fail to be placed are told apart, because they need
// opposite answers: text this book does not carry is not a defect, and text it
// does carry but words differently is one.
func TestAlignByDiffTellsTheFailuresApart(t *testing.T) {
	flat := strings.Fields("alpha beta gamma delta epsilon zeta eta theta")
	lines := []epiLine{
		{para: 1, line: 1, words: []string{"alpha", "beta"}},                 // placed
		{para: 1, line: 2, words: []string{"gamma", "delta", "epsilon"}},     // placed
		{para: 1, line: 3, words: []string{"sigma", "tau", "upsilon"}},       // not in the book
		{para: 1, line: 4, words: []string{"omega", "zeta", "eta", "theta"}}, // begins on text that differs
		{para: 1, line: 5, words: nil},                                       // no words
	}
	anchors, st := alignByDiff(flat, lines)
	if st.Lines != 5 {
		t.Fatalf("lines = %d", st.Lines)
	}
	if st.Anchored+st.Unplaced() != st.Lines {
		t.Fatalf("anchored %d + unplaced %d != %d lines", st.Anchored, st.Unplaced(), st.Lines)
	}
	if st.Anchored != 2 {
		t.Errorf("anchored %d, want 2: %+v", st.Anchored, st)
	}
	if st.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1", st.Unmatched)
	}
	if st.Shifted != 1 {
		t.Errorf("Shifted = %d, want 1", st.Shifted)
	}
	if st.Empty != 1 {
		t.Errorf("Empty = %d, want 1", st.Empty)
	}
	for _, a := range anchors {
		if a.Line == 3 || a.Line == 4 || a.Line == 5 {
			t.Errorf("line %d should not have been placed, at %d", a.Line, a.At)
		}
	}
	if anchors[0].At != 0 || anchors[1].At != 2 {
		t.Errorf("anchors landed at %d, %d; want 0, 2", anchors[0].At, anchors[1].At)
	}
}

// The two classes the old rule could not tell apart, on the case that produced
// most of them: a line whose opening the editions spell differently — the
// elision apostrophe is one token on one side and two on the other — and a line
// whose whole text is simply not here.
//
// The first is dropped, not guessed: the alignment reaches it on text that is
// not identical, and a translation filed under a sentence that is merely near
// the right one is the defect this whole design exists to prevent. The second
// is dropped because there is nothing to place.
func TestAlignByDiffDropsALineWhoseOpeningDiffers(t *testing.T) {
	flat := strings.Fields("tassa evaṃ hoti visattika nti tiṇṇaṃ loke")
	lines := []epiLine{
		{para: 1, line: 1, words: []string{"tassa", "evaṃ", "hoti"}},
		// This reader holds `visattika"nti` as two words where ePitaka writes
		// `visattikan '' ti`: the line begins on a difference, so it is not
		// placed even though every word of it is here.
		{para: 1, line: 2, words: []string{"visattikan", "ti", "tiṇṇaṃ", "loke"}},
	}
	anchors, st := alignByDiff(flat, lines)
	if st.Anchored != 1 {
		t.Fatalf("anchored %d, want 1: %+v", st.Anchored, st)
	}
	if st.Shifted != 1 {
		t.Errorf("Shifted = %d, want 1", st.Shifted)
	}
	if anchors[0].Line != 1 {
		t.Errorf("the wrong line was kept: %+v", anchors[0])
	}
}

// A one-word line is placed. The old rule needed two words to anchor at all, so
// forty thousand of them could never be placed however plainly the book held
// them.
func TestAlignByDiffPlacesAOneWordLine(t *testing.T) {
	flat := strings.Fields("alpha beta gamma delta")
	lines := []epiLine{{para: 1, line: 1, words: []string{"gamma"}}}
	anchors, st := alignByDiff(flat, lines)
	if st.Anchored != 1 {
		t.Fatalf("a one-word line was not placed: %+v", st)
	}
	if anchors[0].At != 2 {
		t.Errorf("placed at %d, want 2", anchors[0].At)
	}
}

// Two identical lines cannot both be placed on one stretch of text. The book
// holds the words once, so one line is placed and the other is dropped — the
// other half of the defect that stacked a dozen translations under one short
// paragraph.
func TestAlignByDiffPlacesTwoIdenticalLinesOnlyOnce(t *testing.T) {
	flat := strings.Fields("alpha beta gamma delta")
	lines := []epiLine{
		{para: 1, line: 1, words: []string{"alpha", "beta", "gamma", "delta"}},
		{para: 1, line: 2, words: []string{"alpha", "beta", "gamma", "delta"}},
	}
	anchors, st := alignByDiff(flat, lines)
	if st.Anchored != 1 {
		t.Fatalf("anchored %d, want 1: %+v", st.Anchored, st)
	}
	if anchors[0].Line != 1 {
		t.Errorf("the first line was not the one kept: %+v", anchors[0])
	}
}

// A line whose tail is wordier here than there is still placed: its own first
// word is in an identical run, and where a line begins is what decides which
// segment owns it. The words the volume has and the book does not cost nothing,
// which is the case that produced most of the unplaced lines before.
func TestAlignByDiffPlacesALineWhoseTailDiffers(t *testing.T) {
	flat := strings.Fields("alpha beta gamma delta")
	lines := []epiLine{{para: 1, line: 1, words: []string{"alpha", "beta", "sigma", "tau"}}}
	anchors, st := alignByDiff(flat, lines)
	if st.Anchored != 1 {
		t.Fatalf("not placed: %+v", st)
	}
	if anchors[0].At != 0 {
		t.Errorf("placed at %d, want 0", anchors[0].At)
	}
	if anchors[0].Run != 2 || anchors[0].Words != 4 {
		t.Errorf("the line's own coverage was not recorded: %+v", anchors[0])
	}
	if st.Full != 0 {
		t.Errorf("a line half of which is not here was counted as resting on its own text throughout")
	}
}

// Words repeated on both sides must not let the alignment slide: the block that
// covers a position has to be the one the recursion chose, and every position
// inside a block has to map to the same offset in the other stream.
func TestMatchBlocksMapPositionsByOffset(t *testing.T) {
	a := ids("x y z p q r x y z p q r")
	b := ids("x y z p q r x y z p q r")
	blocks := matchBlocks(a, b).Blocks
	if len(blocks) != 1 || blocks[0].N != len(a) {
		t.Fatalf("blocks = %+v, want the whole stream as one run", blocks)
	}
	for pos := range b {
		if got := blockAt(blocks, pos); got != 0 {
			t.Fatalf("blockAt(%d) = %d", pos, got)
		}
		if at := blocks[0].AO + (pos - blocks[0].BO); at != pos {
			t.Fatalf("position %d mapped to %d", pos, at)
		}
	}
	if got := blockAt(blocks, len(b)); got != -1 {
		t.Errorf("a position past the end was placed in a run")
	}
}

// Every line is either placed or accounted for by a reason, for the alignment
// as well as for the old one. The identity is worth a test of its own here
// because the reasons are what a book being thin has to be argued from: a line
// that fell through both would be missing from the numbers the argument is made
// with, and nothing else would show it.
func TestDiffAccountsForEveryLine(t *testing.T) {
	flat := strings.Fields("a b c d e f g h i j k l m n o p")
	lines := []epiLine{
		{para: 1, line: 1, words: []string{"a", "b", "c", "d"}},
		{para: 1, line: 2, words: []string{"e", "f", "g", "h"}},
		{para: 1, line: 3, words: []string{"z", "z", "z", "z"}},
		{para: 1, line: 4, words: []string{"q", "r"}},
		{para: 1, line: 5, words: []string{"i", "j", "k", "l", "m"}},
		{para: 1, line: 6, words: nil},
		{para: 1, line: 7, words: []string{"7", "8"}},           // numerals only
		{para: 1, line: 8, words: []string{"9", "n", "o", "p"}}, // a numeral in front
	}
	anchors, st := alignByDiff(flat, lines)
	if got := st.Anchored + st.Unplaced(); got != st.Lines {
		t.Errorf("anchored %d + unplaced %d = %d, want %d lines (unmatched %d, shifted %d, empty %d)",
			st.Anchored, st.Unplaced(), got, st.Lines, st.Unmatched, st.Shifted, st.Empty)
	}
	if st.Numbered != 1 {
		t.Errorf("Numbered = %d, want 1 — the line whose numeral is not in the book", st.Numbered)
	}
	if st.Empty != 2 {
		t.Errorf("Empty = %d, want 2 — no words, and nothing but numerals", st.Empty)
	}
	// The line placed by stepping over its numeral lands on the word after it.
	for _, a := range anchors {
		if a.Line == 8 && (a.At != 13 || a.SkippedNum != 1) {
			t.Errorf("line 8 placed at %d skipping %d numerals; want 13 and 1", a.At, a.SkippedNum)
		}
	}
}
