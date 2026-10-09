package importer

// Aligning two word streams.
//
// This reader's text and ePitaka's are the same canon, so the question the
// importer has to answer — which of ePitaka's lines does this book carry, and
// where — is not a search for an opening that happens to match. It is an
// alignment of two sequences that are nearly the same sequence.
//
// What came before this asked, for each line, whether its first four words
// occurred verbatim anywhere in the book. That question has the wrong shape.
// It cannot see a line whose opening the two editions spell differently, and it
// cannot see a line whose words are all present but not consecutively, and when
// the answer is "no" it cannot say whether the line is missing from the book or
// merely worded differently here — the two need opposite answers. An alignment
// answers all of it at once: it says which stretches of the two streams are the
// same text and which are not, over the whole book rather than one line at a
// time, and a line that falls inside an identical stretch has been placed by
// evidence rather than by a heuristic.
//
// The algorithm is the one difflib uses — recursively take the longest run of
// equal elements, then do the same on what is left on either side of it — with
// the quadratic inner search replaced by a seed-and-extend one. Python's
// SequenceMatcher finds that longest run by comparing every position in one
// sequence against every position holding the same element in the other, which
// on a book of a hundred thousand words is six hundred million comparisons
// (measured: cmd/refdiag). Seeding on an eight-word run instead costs one hash
// lookup per position: over the whole canon the number of candidate pairs falls
// from hundreds of millions to tens of thousands, and the runs found are the
// same, because a run shorter than the seed is found by the recursion that
// follows anyway.
//
// Why not Myers: its cost is O((N+M)·D) in the edit distance D, and D here is
// the text this book does not carry — twenty per cent of the volume, so
// millions — while the recursion above never looks at the differing text at
// all. Myers also produces a minimal edit script, which is the right answer to
// a question nobody here is asking: what is wanted is which stretches are
// identical, not the cheapest way to turn one into the other.

// diffBlock is one run of identical words in two aligned streams:
// a[AO:AO+N] and b[BO:BO+N] hold the same words.
type diffBlock struct {
	AO, BO, N int
}

const (
	// seedGram is the length of the run a match is looked for by. Eight Pali
	// words are specific enough that a book of a hundred thousand words holds
	// only tens of thousands of them in common (measured), so the seeding is
	// nearly free; and it is not a limit on what can be matched, because the
	// recursion below takes whatever is left on either side of every match,
	// including runs shorter than the seed.
	seedGram = 8
	// diffExactPairs is the size of a region — the product of the two sides —
	// below which the longest common run is found by looking at every pair
	// rather than by seeding. A small region is where the bookkeeping of an
	// index is not worth paying, and where the tie-breaking of the exhaustive
	// search is worth having. Sixty-five thousand pairs is a few microseconds;
	// anything larger is seeded, and seeding at two words reaches every run the
	// exhaustive search would.
	diffExactPairs = 1 << 16
	// diffWorkBudget bounds the alignment of one book. It is not a tuning
	// knob: it exists so that a pathological pair of streams cannot make an
	// import take unbounded time, and reaching it costs coverage (the rest of
	// the book is left unaligned) rather than correctness. Against the real
	// corpus it is never approached — measured work is four orders of
	// magnitude below it.
	diffWorkBudget = 1 << 28
	// diffMaxDepth stops the recursion on input that splits badly. Reaching it
	// leaves the remainder unaligned, which is the same outcome as reaching the
	// budget: fewer lines placed, no line placed wrongly.
	diffMaxDepth = 4096
)

// wordDiff is one alignment in progress.
type wordDiff struct {
	a, b []uint32
	// at is where each word of b stands, in order. It is what makes the exact
	// search linear in the number of pairs rather than in the product.
	at map[uint32][]int32
	// dp and dpAlt are the two rows of the longest-common-run recurrence,
	// allocated once and cleaned by the touch list of the row that just
	// retired. Indexed by position in b, so both are len(b) long.
	dp, dpAlt []int32
	// budget counts the comparisons the alignment may spend, and exhausted
	// records that it ran out.
	budget    int
	exhausted bool
	spent     int
}

// diffResult is what aligning two streams produced: the runs they share, how
// much work the search cost in word comparisons, and whether the budget stopped
// it before it was finished.
type diffResult struct {
	Blocks []diffBlock
	// Spent is the number of word comparisons the alignment made. It is here
	// because the cost of a diff is the thing to watch when the corpus is
	// re-imported: cmd/refdiag reports it, so a change that makes an import
	// slow says so rather than being noticed.
	Spent int
	// Truncated is set when the work budget ran out. The alignment is then
	// partial: the blocks it did find are still valid and still monotone, but
	// the remainder of the book is unaligned, so lines are dropped. It is
	// reported rather than hidden because a book that is silently
	// half-aligned looks exactly like a book its volume does not match.
	Truncated bool
}

// matchBlocks aligns two word streams and returns the runs of identical words
// they share, in reading order of both.
//
// The result is a global alignment in the sense that matters here: it is
// computed over the whole of both streams at once, it is monotone in both by
// construction, and no block can overlap another. Every position in a is
// matched to at most one in b and the order is preserved, so a caller that
// carries an index from one stream to the other can never be sent backwards.
func matchBlocks(a, b []uint32) diffResult {
	if len(a) == 0 || len(b) == 0 {
		return diffResult{}
	}
	d := &wordDiff{a: a, b: b, budget: diffWorkBudget}
	d.at = make(map[uint32][]int32, len(b))
	for j, w := range b {
		d.at[w] = append(d.at[w], int32(j))
	}
	res := diffResult{Blocks: make([]diffBlock, 0, 64)}
	d.recurse(0, len(a), 0, len(b), &res.Blocks, 0)
	res.Spent = d.spent
	res.Truncated = d.exhausted
	return res
}

// recurse aligns a[alo:ahi] against b[blo:bhi] and appends the runs it finds.
//
// The three steps are difflib's, and the order matters. A run that starts at
// the region's own edge is taken first: it is the one thing about the region
// that needs no search, and taking it keeps a long identical stretch whole
// instead of splitting it around whatever the longest interior run happens to
// be. What is left in the middle is then split on its longest run, and the two
// halves are aligned the same way.
func (d *wordDiff) recurse(alo, ahi, blo, bhi int, out *[]diffBlock, depth int) {
	if alo >= ahi || blo >= bhi || d.exhausted || depth > diffMaxDepth {
		return
	}

	// The common prefix and suffix of the region.
	p := 0
	for alo+p < ahi && blo+p < bhi && d.a[alo+p] == d.b[blo+p] {
		p++
	}
	if p > 0 {
		*out = append(*out, diffBlock{AO: alo, BO: blo, N: p})
		alo += p
		blo += p
	}
	if alo >= ahi || blo >= bhi {
		return
	}
	s := 0
	for ahi-1-s >= alo && bhi-1-s >= blo && d.a[ahi-1-s] == d.b[bhi-1-s] {
		s++
	}
	sahi, sbhi := ahi-s, bhi-s
	d.spend(p + s)
	if alo >= sahi || blo >= sbhi {
		// The two sides are the same run end to end; the suffix is all that is
		// left of it. (sahi, sbhi) is (alo, blo) in this case, so the blocks
		// stay in order.
		if s > 0 {
			*out = append(*out, diffBlock{AO: sahi, BO: sbhi, N: s})
		}
		return
	}

	ai, bi, n := d.longest(alo, sahi, blo, sbhi, depth)
	if n > 0 {
		// Left of the run first, then the run, then right of it: the blocks
		// have to come out in the order they are read, or a caller carrying a
		// position across would be sent backwards.
		d.recurse(alo, ai, blo, bi, out, depth+1)
		*out = append(*out, diffBlock{AO: ai, BO: bi, N: n})
		d.recurse(ai+n, sahi, bi+n, sbhi, out, depth+1)
	}
	if s > 0 {
		*out = append(*out, diffBlock{AO: sahi, BO: sbhi, N: s})
	}
}

// longest finds the longest run of equal words in a[alo:ahi] and b[blo:bhi].
// Ties go to the earliest run in a, and then to the earliest in b, which keeps
// the result a function of the input rather than of the order things were
// visited in.
func (d *wordDiff) longest(alo, ahi, blo, bhi int, depth int) (bestA, bestB, bestN int) {
	// A small region is searched exhaustively: it is where runs shorter than
	// the seed live, and there is no point paying for an index to find them.
	if (ahi-alo)*(bhi-blo) <= diffExactPairs {
		return d.longestExact(alo, ahi, blo, bhi)
	}
	// Otherwise seeded, longest seed first. A region whose longest run is five
	// words is found by the four-word pass, and one whose longest is two by the
	// two-word pass; trying them in this order keeps the cheap, specific pass
	// first and only pays for the vague ones where the specific ones found
	// nothing at all.
	for _, k := range []int{seedGram, 4, 2} {
		if bestA, bestB, bestN = d.longestSeeded(alo, ahi, blo, bhi, k); bestN > 0 {
			return bestA, bestB, bestN
		}
		if d.exhausted {
			return 0, 0, 0
		}
	}
	// Nothing of two words or more is shared. There is no run to anchor on and
	// the recursion stops here; the region is text the two editions do not have
	// in common.
	return 0, 0, 0
}

// longestExact is difflib's find_longest_match: every pair of equal words is
// visited, and the run that starts at it is read off the recurrence. Its cost
// is the number of such pairs — which is why it is only used on small regions.
func (d *wordDiff) longestExact(alo, ahi, blo, bhi int) (bestA, bestB, bestN int) {
	if d.dp == nil {
		d.dp = make([]int32, len(d.b))
		d.dpAlt = make([]int32, len(d.b))
	}
	bestA, bestB = alo, blo
	prev, cur := d.dp, d.dpAlt
	var touchedPrev, touchedCur []int32
	for i := alo; i < ahi; i++ {
		touchedCur = touchedCur[:0]
		for _, j32 := range d.at[d.a[i]] {
			j := int(j32)
			if j < blo {
				continue
			}
			if j >= bhi {
				break
			}
			n := int32(1)
			if j > 0 {
				if p := prev[j-1]; p > 0 {
					n = p + 1
				}
			}
			cur[j] = n
			touchedCur = append(touchedCur, j32)
			if int(n) > bestN {
				bestN = int(n)
				bestA, bestB = i-int(n)+1, j-int(n)+1
			}
		}
		d.spend(len(touchedCur) + 1)
		// The row that has just been read is about to become this iteration's
		// row, so it has to be zeroed; only the entries this call wrote are
		// touched, which is what keeps the recurrence from clearing the whole
		// of b on every line.
		for _, j := range touchedPrev {
			prev[j] = 0
		}
		prev, cur = cur, prev
		touchedPrev, touchedCur = touchedCur, touchedPrev
		if d.exhausted {
			break
		}
	}
	for _, j := range touchedPrev {
		prev[j] = 0
	}
	return bestA, bestB, bestN
}

// longestSeeded finds the longest run by hashing every k-word run of a, then
// looking each k-word run of b up in it and growing the match outwards.
//
// Hashing is only a way of finding candidates; a candidate is confirmed word
// for word before it counts, so a collision costs a comparison and cannot
// produce a wrong block.
//
// Two prunings keep this linear on text that repeats, which the canon is.
// Without them a run of four hundred words is grown four hundred times, once
// from every position inside it, and a single book of ninety thousand words
// spends a quarter of a billion comparisons to find a handful of runs —
// measured, which is how this was found. Neither pruning can hide a run the
// search would otherwise have found:
//
//   - A candidate whose predecessor also matches is inside a run rather than at
//     its head, and the head is itself a candidate — its k words are the same k
//     words — so only the head is grown.
//   - A candidate with less room left than the best run already found cannot
//     beat it, and what is left of the region on either side of it says so
//     without growing anything.
func (d *wordDiff) longestSeeded(alo, ahi, blo, bhi, k int) (bestA, bestB, bestN int) {
	if ahi-alo < k || bhi-blo < k {
		return 0, 0, 0
	}
	index := make(map[uint64][]int32, ahi-alo)
	for i := alo; i+k <= ahi; i++ {
		h := hashWords(d.a[i : i+k])
		index[h] = append(index[h], int32(i))
	}
	d.spend(ahi - alo)
	for j := blo; j+k <= bhi; j++ {
		for _, i32 := range index[hashWords(d.b[j:j+k])] {
			i := int(i32)
			if i > alo && j > blo && d.a[i-1] == d.b[j-1] {
				continue
			}
			if min(ahi-i, bhi-j) <= bestN {
				continue
			}
			if !equalWords(d.a[i:i+k], d.b[j:j+k]) {
				continue
			}
			// Grow both ways: the seed may sit in the middle of a longer run,
			// and a block that starts where the run starts is what a caller
			// carrying a position across needs.
			n, m := 0, k
			for i-n > alo && j-n > blo && d.a[i-n-1] == d.b[j-n-1] {
				n++
			}
			for i+m < ahi && j+m < bhi && d.a[i+m] == d.b[j+m] {
				m++
			}
			d.spend(n + m - k)
			if total := n + m; total > bestN || (total == bestN && i-n < bestA) {
				bestA, bestB, bestN = i-n, j-n, total
			}
			if d.exhausted {
				return bestA, bestB, bestN
			}
		}
	}
	return bestA, bestB, bestN
}

// spend charges the alignment for work it has done, and stops it when the
// budget is gone. What the budget buys is that no book can make the import run
// for an unbounded time; what it costs is coverage — the remainder of that book
// is left unaligned, so lines are dropped rather than misplaced.
func (d *wordDiff) spend(n int) {
	d.spent += n
	d.budget -= n
	if d.budget <= 0 {
		d.exhausted = true
	}
}

func equalWords(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// hashWords is FNV-1a over the words of a run. The runs are short, so the cost
// of hashing them whole is not worth a rolling hash's bookkeeping.
func hashWords(w []uint32) uint64 {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)
	h := uint64(offset)
	for _, x := range w {
		for s := 0; s < 32; s += 8 {
			h ^= uint64(byte(x >> s))
			h *= prime
		}
	}
	return h
}

// internWords maps the two streams onto one dense id space, so that the diff
// compares integers rather than strings.
func internWords(streams ...[]string) [][]uint32 {
	ids := map[string]uint32{}
	out := make([][]uint32, len(streams))
	for si, s := range streams {
		v := make([]uint32, len(s))
		for i, w := range s {
			id, ok := ids[w]
			if !ok {
				id = uint32(len(ids))
				ids[w] = id
			}
			v[i] = id
		}
		out[si] = v
	}
	return out
}
