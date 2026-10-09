package importer

import (
	"sort"
	"strings"
)

// Locating ePitaka's lines in this reader's text.
//
// ePitaka publishes the canon one sentence per row, keyed (book_id, para_id,
// line_id), and each language file carries a translation under the same key. A
// translation is therefore not something to be distributed over a paragraph by
// guesswork: once a row's key is known, its translation is that row's, and
// nothing has to be inferred. So the whole job of the alignment is to establish
// that key, once — and to say plainly which rows it could not establish.
//
// What this replaced searched forward from a cursor, inside the span the
// surrounding paragraph was thought to occupy, accepting the first four words
// that matched. Each of those three restrictions loses lines for reasons that
// have nothing to do with the text: a paragraph located a little too short
// hides its own last lines, a line whose wording differs by a word makes the
// cursor step over the next line's start, and a volume carrying text this book
// does not have burns the drift budget for everything after it. The importer
// reported all of that as one number — 27% of the lines it looked at — which
// cannot be acted on, because the causes need opposite answers.
//
// So the search is not local any more. Every line is looked up in the whole
// book by a four-word opening; the result is accepted only as part of the
// longest chain of anchors that runs forward through the book without ever
// going backwards. That last part is what makes it safe: a single wrong anchor
// cannot drag the rest off course, because a chain containing it is shorter
// than the one without it and the longest chain wins.
//
// A line whose opening is nowhere in the book is not a defect to fix. It is a
// line this book does not carry, and it is dropped with that said out loud.

// anchorGram is the run of words an anchor rests on. Four is long enough that
// an ordinary Pali sentence opening is distinctive, and short enough that a
// line diverging at its fifth word is still found.
const anchorGram = 4

// anchorSkip is how far into a line the opening may be looked for. Three words
// covers the difference this matters for — this reader holds `visattika"nti`
// where ePitaka writes `visattikan '' ti`, and a variant reading at the head of
// a sentence — while what is still required is four consecutive words verbatim.
// Going further would begin matching the middle of one line to another's.
const anchorSkip = 3

// minAnchorGram is the shortest run that may anchor a line, used only when four
// words and then three found nothing.
const minAnchorGram = 2

// anchorGramPenalty pushes a weaker anchor below a stronger one in a chain of
// the same length. Small against anchorQuality on purpose: it must never be
// worth dropping an anchor to avoid it.
const anchorGramPenalty = 150

// anchorQuality is what one anchored line is worth, against which the small
// bonuses below are tie-breaks. Maximising anchors comes first; among the
// chains that anchor the same number of lines, the one whose anchors verify
// further and start at the line's own first word is preferred.
const anchorQuality = 1000

// lineAnchor is one ePitaka line and where it was found.
type lineAnchor struct {
	// Idx is the line's index in the slice anchorLines was given, so a caller
	// holding the lines can reach the translation that was read with them.
	Idx        int
	Para, Line int
	// At is the position of the line's first word in the book's word stream.
	At int
	// Run is how many of the line's words matched verbatim from At.
	Run int
	// Words is the line's length, so Run >= Words means the whole line was
	// found, not only its opening.
	Words int
	// Off is how many of the line's words were stepped over before the verified
	// run began. Non-zero means the anchor rests on the assumption that the two
	// editions agree word for word around a local difference — the best reading
	// available, but a reading, so it is counted separately.
	Off int
	// Ambiguous is set when the book contains more than one equally good place
	// for this line, so the choice between them was made by the chain rather
	// than by the evidence.
	Ambiguous bool
	// Gram is how many words the anchor rests on: 4 normally, 3 or 2 when the
	// editions differ at the head of the line.
	Gram int
	// SkippedNum is how many numerals at the head of the line were stepped over
	// before its text was found. Both editions number their verses; this one
	// writes the number once at the head of a gatha, ePitaka writes it again at
	// the head of every line of one. A number is not a word of the sentence
	// (internal/tokenize does not make it clickable either), so stepping over
	// it costs nothing and losing the line costs its translation.
	SkippedNum int
}

// AnchorStats is what the alignment did, in the terms the answer needs.
type AnchorStats struct {
	Lines    int
	Anchored int
	// Full: anchored lines whose whole text was found, not just the opening.
	Full int
	// Exact: anchored lines found from their own first word, with no step-over.
	Exact int
	// Ambiguous: anchored lines the book holds more than one place for.
	Ambiguous int
	// NoVerbatim: no four-word opening of the line occurs anywhere in this
	// book. The volume carries text the book does not have; there is nothing to
	// place and nothing to fix.
	NoVerbatim int
	// Contradicted: the line's opening does occur, but only at places that
	// would put it before text already accounted for. A repeated formula, most
	// often.
	Contradicted int
	// Short: fewer than two words, so there is no opening to look up.
	Short int
	// Uncarried: the line's opening was found, but only as two or three words
	// with nothing after them to confirm it. Counted apart from NoVerbatim
	// because the two say different things: one is text this book does not
	// have, the other is text it has and would not vouch for.
	Uncarried int
	// Weak: anchored on only two or three words rather than four, because the
	// editions differ where the line begins.
	Weak int

	// The reasons the alignment in align.go gives are not the reasons above,
	// and mixing them would make both unreadable: the gram aligner asks whether
	// a line's opening occurs anywhere, the diff asks where in the alignment
	// the line falls. These three are the diff's answers.
	//
	// Unmatched: not one word of the line lies in a run of text this book
	// shares with the volume. The book does not carry the line; there is
	// nothing to place and nothing to fix.
	Unmatched int
	// Shifted: some of the line is here, but not its beginning — the alignment
	// reaches this line on text that is not identical to it, so where the line
	// starts cannot be established. Dropped, and counted apart from Unmatched
	// because the two say opposite things: one is text this book does not have,
	// the other is text it has and the alignment would not vouch for. This is
	// the class the reference implementation records as exact = 0.
	Shifted int
	// Empty: the line carries no words at all — or nothing but numerals, which
	// is the same thing for a sentence — so there is nothing to place it by. A
	// one-word line is not Empty: the diff can place it on the strength of the
	// word itself, which the gram aligner could not.
	Empty int
	// Numbered: lines placed after stepping over the numerals at their head.
	// Counted apart because they are the one class where what was matched is
	// not quite the whole of the line's beginning: what is matched is its first
	// word, and a number stands in front of it here and not there.
	Numbered int

	// Blocks and BlockWords are the diff's own account of what it found: how
	// many runs of identical words the book and its volume share, and how many
	// of the volume's words those runs cover. They are not line counts, so they
	// take no part in the accounting identity below; they are here because they
	// are what says whether a low rate is the alignment's fault or the book's.
	Blocks, BlockWords int
	// Work is how many word comparisons the alignment spent on this book, and
	// Truncated is set when that was more than the budget allowed — in which
	// case the alignment is partial and lines were dropped for that reason
	// rather than because the book does not carry them.
	Work      int
	Truncated bool
}

// Unplaced is the number of lines that yielded nothing.
func (s AnchorStats) Unplaced() int {
	return s.NoVerbatim + s.Contradicted + s.Short + s.Uncarried +
		s.Unmatched + s.Shifted + s.Empty
}

// gramIndex is every two-, three- and four-word run of a book's word stream, by
// position.
//
// Three lengths because a line is looked up at four words first and only falls
// back to three and then two when four finds nothing: the shorter the run, the
// less it says, so the shorter ones are a second chance rather than the rule.
// And a line of two or three words has no four-word opening to look up at all,
// which is exactly the case of a title.
type gramIndex struct {
	grams map[int]map[string][]int32
}

func newGramIndex(flat []string) *gramIndex {
	idx := &gramIndex{grams: make(map[int]map[string][]int32, 3)}
	for g := minAnchorGram; g <= anchorGram; g++ {
		m := make(map[string][]int32, len(flat))
		for i := 0; i+g <= len(flat); i++ {
			k := strings.Join(flat[i:i+g], " ")
			m[k] = append(m[k], int32(i))
		}
		idx.grams[g] = m
	}
	return idx
}

func (idx *gramIndex) at(g int, key string) []int32 { return idx.grams[g][key] }

// candidate is one place a line's opening was found, with how far it verified.
type candidate struct {
	at  int
	run int
	off int
	// end is where the anchor's text finishes, which is what the next line has
	// to come after.
	end int
	// quality is the tie-break among chains of equal length.
	quality int
	// gram is how many words the anchor rests on, 4 being the first length
	// tried and 2 the last.
	gram int
	// ambiguous records that the line had other, equally good candidates.
	ambiguous bool
}

// findLine returns every position in the book where line occurs and verifies
// for at least gram words, trying each offset from the line's first word up to
// anchorSkip.
//
// The offsets exist because the two editions place the elision apostrophe
// differently — this reader holds `visattika"nti` where ePitaka writes
// `visattikan \'\' ti`, two tokens against two, but not the same two — and a
// difference inside the opening would otherwise lose the whole line. Every
// offset is tried rather than stopping at the first that finds something: a run
// that is common in the book will match somewhere for almost any line, and it
// is the chain, not the first hit, that decides.
//
// A line shorter than gram words is not looked up at this length at all; the
// caller falls back to a shorter gram, and only a whole-line match anchors it.
func findLine(flat []string, idx *gramIndex, words []string, gram int) []candidate {
	if len(words) < 2 || len(words) < gram {
		return nil
	}
	maxOff := anchorSkip
	if maxOff > len(words)-gram {
		maxOff = len(words) - gram
	}
	var out []candidate
	for off := 0; off <= maxOff; off++ {
		rest := words[off:]
		for _, k := range idx.at(gram, strings.Join(rest[:gram], " ")) {
			at := int(k)
			run := extendMatch(flat, rest, at)
			if run < gram {
				continue
			}
			// The line's own first word is off words before the verified run.
			// The editions agree word for word around a local difference, so
			// stepping back by the same count puts the anchor on the line's
			// first word; clamped, because a book that begins mid-line has
			// nowhere to step back to.
			start := at - off
			if start < 0 {
				start = 0
			}
			q := anchorQuality + min(run+off, 300)
			if off == 0 {
				q += 400
			}
			if run+off >= len(words) {
				q += 300
			}
			// A three- or two-word run is a weaker thing to have rested on, so
			// it orders below a four-word one when the chain is the same
			// length. It never stops the chain from being longer: an extra
			// anchor is worth more than any penalty.
			q -= (anchorGram - gram) * anchorGramPenalty
			out = append(out, candidate{
				at: start, run: run + off, off: off,
				end: at + run, quality: q, gram: gram,
			})
		}
	}
	markAmbiguous(out)
	return out
}

// candidatesFor looks a line up at four words, and only if that finds nothing
// at three and then two. The shorter lengths are a second chance for a line the
// editions spell differently at its head; they are not tried alongside the
// longer one because a two-word run matches somewhere in a book of fifty
// thousand words almost always, and the book would fill up with them.
func candidatesFor(flat []string, idx *gramIndex, words []string, minGram int, opt AnchorOptions) ([]candidate, bool) {
	uncarried := false
	for g := anchorGram; g >= minGram; g-- {
		if len(words) < g {
			continue
		}
		cands := findLine(flat, idx, words, g)
		if len(cands) == 0 {
			continue
		}
		need := requiredRun(g, opt)
		kept := cands[:0:0]
		for _, c := range cands {
			// A short opening only counts when the line carries on after it:
			// matching two words is not evidence, matching two words and then
			// four more is.
			if g < anchorGram && c.run < need && c.run < len(words) {
				uncarried = true
				continue
			}
			kept = append(kept, c)
		}
		if len(kept) > 0 {
			return kept, uncarried
		}
	}
	return nil, uncarried
}

// markAmbiguous records, on each candidate, that the line had more than one
// place in the book that fitted it.
func markAmbiguous(cands []candidate) {
	if len(cands) < 2 {
		return
	}
	for i := range cands {
		cands[i].ambiguous = true
	}
}

// extendMatch counts how many of words appear at at, in order, verbatim.
func extendMatch(flat, words []string, at int) int {
	n := 0
	for n < len(words) && at+n < len(flat) && flat[at+n] == words[n] {
		n++
	}
	return n
}

// anchorLines places every ePitaka line of a book in its word stream.
//
// Lines are given in reading order — (para_id, line_id) — and the result is
// monotone by construction: it is the longest chain of anchors that never runs
// backwards and never overlaps itself, so two lines can never claim the same
// words and a translation can never be placed above text that comes after it.
//
// exact refuses the anchors that stepped over a difference in the line's
// opening. It is the difference between "this book contains this line's own
// words here" and "this book contains this line's words here, if the editions
// agree around the part that differs" — the second is usually right, and the
// two are counted apart so the choice between them is a number rather than an
// opinion.
func anchorLines(flat []string, lines []epiLine, exact bool) ([]lineAnchor, AnchorStats) {
	return anchorLinesFrom(flat, lines, AnchorOptions{MinGram: minAnchorGram, Exact: exact})
}

// AnchorOptions is how much evidence an anchor has to rest on.
type AnchorOptions struct {
	// MinGram is the shortest run that may anchor a line. Four is the length
	// the search is built around; allowing two or three buys coverage and, it
	// turns out, buys wrong anchors with it — a two-word run matches somewhere
	// in a book of fifty thousand words almost always, and a chain of them can
	// walk through a stretch of text this book does not have at all, putting a
	// dozen translations under one paragraph. Measured: see cmd/refdiag.
	MinGram int
	// Exact refuses every anchor that did not begin at the line's own first
	// word with four words verbatim.
	Exact bool
	// Safe makes a shorter opening prove itself by continuing further: two
	// words must be followed by four more, three by two more, before either
	// counts. A two-word run on its own is the weakest evidence in the book —
	// it matches somewhere for almost any pair of Pali words — and what makes
	// it trustworthy is not the pair but the text that carries on after it.
	Safe bool
}

// requiredRun is how far a match must verify to anchor, given the opening it
// was found by. Four words always suffice; a shorter opening has to be carried
// by more of the line behind it.
func requiredRun(gram int, opt AnchorOptions) int {
	if !opt.Safe {
		return gram
	}
	switch gram {
	case 2:
		return 6
	case 3:
		return 5
	}
	return gram
}

func anchorLinesFrom(flat []string, lines []epiLine, opt AnchorOptions) ([]lineAnchor, AnchorStats) {
	var st AnchorStats
	st.Lines = len(lines)
	if len(flat) < anchorGram {
		st.Short = len(lines)
		return nil, st
	}
	minGram := opt.MinGram
	if minGram < 2 {
		minGram = 2
	}
	if minGram > anchorGram {
		minGram = anchorGram
	}
	idx := newGramIndex(flat)

	// candidate sets, one per line, and the total count for sizing.
	sets := make([][]candidate, len(lines))
	weakOnly := make([]bool, len(lines))
	for i, ln := range lines {
		if len(ln.words) < 2 {
			continue
		}
		cands, uncarried := candidatesFor(flat, idx, ln.words, minGram, opt)
		weakOnly[i] = uncarried && len(cands) == 0
		if opt.Exact {
			kept := cands[:0:0]
			for _, c := range cands {
				if c.off == 0 && c.gram == anchorGram {
					kept = append(kept, c)
				}
			}
			cands = kept
		}
		sets[i] = cands
	}

	// A Fenwick tree over the book's positions holding the best chain that ends
	// at or before each one. Lines are processed in order and their candidates
	// inserted together afterwards, so a line can never chain to itself.
	ends := make([]int, 0, len(flat))
	for _, set := range sets {
		for _, c := range set {
			ends = append(ends, c.end)
		}
	}
	sort.Ints(ends)
	ends = dedupeInts(ends)
	fw := newFenwick(len(ends))

	type state struct {
		score   int
		line    int
		cand    int
		prev    int
		endAt   int
	}
	states := make([]state, 0, len(ends))

	for li, set := range sets {
		if len(set) == 0 {
			continue
		}
		type pending struct {
			pos   int
			score int
			idx   int
		}
		var add []pending
		for ci, c := range set {
			// The best chain that ends at or before this anchor begins. Line
			// after line this is an equality — one line's text ends exactly
			// where the next one's starts — so the boundary has to be included
			// rather than excluded, or the chain breaks at every sentence.
			at := rankLE(ends, c.at)
			score, prev := fw.query(at)
			states = append(states, state{
				score: score + c.quality, line: li, cand: ci, prev: prev, endAt: c.end,
			})
			add = append(add, pending{
				pos:   rankLE(ends, c.end),
				score: score + c.quality,
				idx:   len(states) - 1,
			})
		}
		// Inserted only now, so a line can never chain to another of its own
		// candidates and take the same sentence twice.
		for _, p := range add {
			fw.update(p.pos, p.score, p.idx)
		}
	}

	bestScore, bestIdx := fw.query(len(ends))
	_ = bestScore

	// Walk the chain back, then forward again in reading order. An empty chain
	// is a book where nothing anchored at all, which the classification below
	// reports line by line like any other failure.
	var chain []int
	for i := bestIdx; i >= 0; i = states[i].prev {
		chain = append(chain, i)
	}
	sort.Ints(chain)

	out := make([]lineAnchor, 0, len(chain))
	for _, si := range chain {
		s := states[si]
		c := sets[s.line][s.cand]
		ln := lines[s.line]
		a := lineAnchor{
			Idx:  s.line,
			Para: ln.para, Line: ln.line,
			At: c.at, Run: c.run, Words: len(ln.words),
			Off: c.off, Ambiguous: c.ambiguous, Gram: c.gram,
		}
		if a.Run >= a.Words {
			st.Full++
		}
		if a.Off == 0 && a.Gram == anchorGram {
			st.Exact++
		}
		if a.Gram < anchorGram {
			st.Weak++
		}
		if a.Ambiguous {
			st.Ambiguous++
		}
		out = append(out, a)
		st.Anchored++
	}

	// What did not make the chain, by reason.
	placed := make(map[int]bool, len(chain))
	for _, si := range chain {
		placed[states[si].line] = true
	}
	for i, set := range sets {
		if placed[i] {
			continue
		}
		if len(lines[i].words) < 2 {
			st.Short++
			continue
		}
		if len(set) == 0 {
			if weakOnly[i] {
				// The line is somewhere in the book, but only behind an
				// opening too short to be sure of. Refusing it is the point:
				// a translation under the wrong sentence is worse than none.
				st.Uncarried++
				continue
			}
			st.NoVerbatim++
			continue
		}
		st.Contradicted++
	}
	return out, st
}

func dedupeInts(v []int) []int {
	out := v[:0]
	var last int
	for i, x := range v {
		if i > 0 && x == last {
			continue
		}
		out = append(out, x)
		last = x
	}
	return out
}

// rankLE is how many of the sorted values are at most x. It is the coordinate
// both halves of the chain test are expressed in: an anchor may follow one
// that finishes at or before where it begins, and an anchor that finishes at v
// is filed at rankLE(v). Using the same coordinate for both is what makes
// "ends at 4, begins at 0" a refusal rather than a chain.
func rankLE(sorted []int, x int) int {
	return sort.Search(len(sorted), func(i int) bool { return sorted[i] > x })
}

// fenwick is a max-tree over positions, with the index that achieved each max
// so the chain can be walked back.
type fenwick struct {
	tree []int // score
	who  []int // state index
	n    int
}

func newFenwick(n int) *fenwick {
	f := &fenwick{tree: make([]int, n+1), who: make([]int, n+1), n: n}
	for i := range f.who {
		f.who[i] = -1
	}
	return f
}

// update raises position i to score, keeping the larger.
func (f *fenwick) update(i, score, who int) {
	for ; i <= f.n; i += i & -i {
		if score > f.tree[i] {
			f.tree[i], f.who[i] = score, who
		}
	}
}

// query returns the best score at any position up to and including i, and the
// state that achieved it. i is 1-based; query(0) is the empty chain.
func (f *fenwick) query(i int) (int, int) {
	best, who := 0, -1
	if i > f.n {
		i = f.n
	}
	for ; i > 0; i -= i & -i {
		if f.tree[i] > best {
			best, who = f.tree[i], f.who[i]
		}
	}
	return best, who
}
