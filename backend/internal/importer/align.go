package importer

import "sort"

// Placing ePitaka's lines by aligning the two whole word streams.
//
// This is what replaced the search for a verbatim opening (anchors.go). The
// unit is still the line and the answer is still "placed or dropped", but what
// decides is now an alignment of the whole book against the whole volume: the
// runs of words the two have in common are found once, globally, and a line
// whose first word falls inside one of those runs has been placed by the
// strongest evidence there is — the book's text at that point is that line's
// own words.
//
// What that buys, and why it is worth the diff:
//
//   - A line whose opening the editions spell differently is placed by the text
//     around it rather than lost. The old rule needed the opening itself to be
//     there, four words verbatim, and a line that diverged at its first word
//     was dropped however much of the rest of it the book held.
//   - A line that the book does not carry is told apart from a line the book
//     words differently. The old rule reported both as "no verbatim opening",
//     which is a number nobody can act on: one is text that is not here and
//     should be dropped silently, the other is text that is here and was
//     missed.
//   - A repeated formula cannot drag the rest of the book with it. The old rule
//     resolved repetition by taking the longest chain of whole lines; the
//     alignment resolves it by position, once, for the whole book.
//
// What it does not do is guess. A line is placed only when its first word lies
// inside a run of text identical to the book's. Where the alignment puts a line
// on text that is not identical, the line is dropped and counted — the
// reference implementation writes those out with exact = 0 and reports them,
// and so does this: see AnchorStats.Shifted and AnchorStats.Unmatched.
//
// There is no fallback. A dropped line's translation is not written anywhere.

// alignByDiff places every ePitaka line of a book in the book's word stream.
//
// flat is the book's words in reading order, one word per position, across all
// its segments. lines are the volume's lines in reading order. The result is
// monotone by construction: the runs the alignment returns are in order and do
// not overlap, so two lines are never placed out of reading order and never on
// the same word.
func alignByDiff(flat []string, lines []epiLine) ([]lineAnchor, AnchorStats) {
	var st AnchorStats
	st.Lines = len(lines)
	if len(flat) == 0 || len(lines) == 0 {
		st.Empty = len(lines)
		return nil, st
	}

	// The volume's own stream: every line's words run together, with where each
	// line begins. The alignment is between words, but what is placed is lines,
	// so the boundary between them has to be carried along.
	starts := make([]int, len(lines))
	flatB := make([]string, 0, len(flat))
	for i := range lines {
		starts[i] = len(flatB)
		flatB = append(flatB, lines[i].words...)
	}
	if len(flatB) == 0 {
		st.Empty = len(lines)
		return nil, st
	}

	interned := internWords(flat, flatB)
	res := matchBlocks(interned[0], interned[1])
	blocks := res.Blocks
	st.Blocks, st.Work, st.Truncated = len(blocks), res.Spent, res.Truncated
	for i := range blocks {
		st.BlockWords += blocks[i].N
	}

	// Which lines the alignment placed, and how much of each line's own text
	// the book actually holds. The second is what separates a line the book
	// does not carry from one it merely words differently, and it is also what
	// says whether a placed line rests on its own text throughout.
	anchors := make([]lineAnchor, 0, len(lines))
	placed := make([]bool, len(lines))
	covered := make([]int, len(lines))
	for i := range lines {
		words := lines[i].words
		if len(words) == 0 {
			continue
		}
		lo := starts[i]
		hi := lo + len(words)
		covered[i] = coveredIn(blocks, lo, hi)
		bi := blockAt(blocks, lo)
		skipped := 0
		if bi < 0 {
			// The line does not begin on text this book shares. Before giving
			// up on it, step over the numerals at its head and look again: both
			// editions number their verses, this one writes the number once at
			// the head of a gatha and ePitaka writes it at the head of every
			// line of one, and a numeral is not a word of the sentence — the
			// reader cannot click it either (internal/tokenize). What is then
			// matched is the line's own first word, on identical text, exactly
			// as for any other line; the only thing that is not matched is the
			// number in front of it. It is counted apart for that reason
			// (AnchorStats.Numbered) and it never produces a sentence boundary
			// (see cutsFor).
			num := numeralPrefix(words)
			if num == 0 || num == len(words) {
				continue
			}
			lo += num
			if bi = blockAt(blocks, lo); bi < 0 {
				continue
			}
			skipped = num
		}
		placed[i] = true
		anchors = append(anchors, lineAnchor{
			Idx:        i,
			Para:       lines[i].para,
			Line:       lines[i].line,
			At:         blocks[bi].AO + (lo - blocks[bi].BO),
			Run:        covered[i],
			Words:      len(words),
			SkippedNum: skipped,
		})
	}

	for i := range anchors {
		st.Anchored++
		if anchors[i].Run >= anchors[i].Words {
			st.Full++
		}
		if anchors[i].SkippedNum > 0 {
			st.Numbered++
		}
	}
	for i := range lines {
		if placed[i] {
			continue
		}
		switch {
		case len(lines[i].words) == 0, numeralPrefix(lines[i].words) == len(lines[i].words):
			st.Empty++
		case covered[i] == 0:
			// Not one word of the line is text this book shares with the
			// volume. The book does not carry the line.
			st.Unmatched++
		default:
			// Part of the line is here, but not its beginning: where the line
			// starts cannot be established from identical words. Dropped, and
			// counted apart from the class above, because this is the one a
			// further change could recover.
			st.Shifted++
		}
	}
	return anchors, st
}

// numeralPrefix counts the numerals a line begins with. A line of nothing but
// numerals has no text to place and is not a line for this purpose.
func numeralPrefix(words []string) int {
	n := 0
	for n < len(words) && isNumeral(words[n]) {
		n++
	}
	return n
}

// isNumeral reports whether a word is nothing but digits. The alignment keeps
// numerals in the stream — they are part of the text and the paragraph numbers
// are written into it — but they are not words a sentence rests on.
func isNumeral(w string) bool {
	if w == "" {
		return false
	}
	for _, r := range w {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// blockAt returns the index of the identical run that holds position pos in the
// volume's stream, or -1 when pos is in text the book words differently.
func blockAt(blocks []diffBlock, pos int) int {
	k := sort.Search(len(blocks), func(i int) bool { return blocks[i].BO > pos })
	if k == 0 {
		return -1
	}
	if blk := blocks[k-1]; pos < blk.BO+blk.N {
		return k - 1
	}
	return -1
}

// coveredIn counts how many of b[lo:hi) fall inside an identical run, which is
// how much of a line's own text this book holds.
func coveredIn(blocks []diffBlock, lo, hi int) int {
	k := sort.Search(len(blocks), func(i int) bool { return blocks[i].BO+blocks[i].N > lo })
	n := 0
	for ; k < len(blocks) && blocks[k].BO < hi; k++ {
		start, end := max(blocks[k].BO, lo), min(blocks[k].BO+blocks[k].N, hi)
		if end > start {
			n += end - start
		}
	}
	return n
}
