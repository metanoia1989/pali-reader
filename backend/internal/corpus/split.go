package corpus

import (
	"sort"
	"strings"
)

// Splitting a paragraph — or a verse — into the pieces the reader works on.
//
// One CST paragraph carries a whole prose passage — in the Saṃyutta ten
// sentences is ordinary — and one CST verse segment carries a whole gāthā. In
// both cases everything the reader attaches to a piece of text (a pick, an
// annotation, their own translation) attaches to the passage rather than to the
// sentence or the line they are working on. The boundaries are not invented
// here: ePitaka publishes one row per sentence and one row per line of a gāthā,
// and internal/importer aligns those rows onto this segment stream and hands the
// byte offsets to SplitSegments.
//
// The unit of the reader's anchors does not move. It was already
// (book, seq, word index); seq is per book, so a paragraph that becomes ten
// segments changes which seq a word sits under, and nothing else.

// SplitSegments replaces every segment its cut offsets divide with the
// sub-segments they define, renumbering Seq so it stays contiguous in reading
// order.
//
// cuts[i] holds byte offsets into segs[i].Text at which a new piece begins, in
// reading order. A segment with no usable cuts — a heading, a centred line, or a
// paragraph whose sentences could not be aligned — passes through untouched.
// That is what makes a failed alignment harmless: the text stays whole, and the
// only cost is that this paragraph is still read as one.
func SplitSegments(segs []Segment, cuts [][]int) []Segment {
	out := make([]Segment, 0, len(segs))
	for i, s := range segs {
		var pieces []Segment
		if i < len(cuts) && len(cuts[i]) > 0 && cuttable(s.Kind) {
			pieces = SplitAt(s, cuts[i])
		}
		if len(pieces) == 0 {
			pieces = []Segment{s}
		}
		out = append(out, pieces...)
	}
	for i := range out {
		out[i].Seq = i + 1
	}
	return out
}

// cuttable reports whether a segment may be divided at all.
//
// Prose is divided at the sentences ePitaka publishes, verse at the lines the
// edition prints. Everything else is one indivisible unit: a heading is a label
// the table of contents points at, and a centred block is a line of verse the
// edition did not mark as one — cutting either would move an entry in the
// contents off the heading it names, or take a line apart on no evidence at all.
func cuttable(kind string) bool {
	return kind == KindProse || kind == KindVerse
}


// SplitAt cuts one segment at the given byte offsets and returns the pieces in
// reading order, or nil when the cuts do not actually divide it.
//
// Everything that carries a position has to be recomputed, because a position
// that was right for the paragraph is wrong for a sentence: the tokenisation is
// left to the caller (it re-runs tokenize.Split on each piece's own text — the
// one implementation, unchanged), and bold runs and variant readings are
// re-based here. A run that straddles a cut is clipped to the piece it half
// covers rather than dropped: the edition's emphasis on a headword is the only
// thing marking it as a headword, and a variant reading is what a student of
// the canon came for.
func SplitAt(s Segment, cuts []int) []Segment {
	text := s.Text
	if text == "" || len(cuts) == 0 {
		return nil
	}

	// The cuts are byte offsets of the first word of each sentence. The first
	// sentence needs no cut, and an offset that does not divide the text — a
	// repeat, or one past the end — is dropped rather than trusted: a bad cut
	// would lose text, and losing text is far worse than an extra split.
	bounds := []int{0}
	for _, c := range cuts {
		if c > 0 && c < len(text) && c > bounds[len(bounds)-1] {
			bounds = append(bounds, c)
		}
	}
	if len(bounds) < 2 {
		return nil
	}
	bounds = append(bounds, len(text))

	// Recorded spans are in UTF-16 code units (the unit the client slices by)
	// and the cuts are bytes. Convert once, so all the geometry below is in one
	// unit, and convert back on the way out.
	um := newUnitMap(text)
	variants := make([]atText, 0, len(s.Variants))
	for _, v := range s.Variants {
		variants = append(variants, atText{at: um.byteAt(v.Offset), text: v.Text})
	}
	bold := make([][2]int, 0, len(s.Bold))
	for _, sp := range s.Bold {
		bold = append(bold, [2]int{um.byteAt(sp.Offset), um.byteAt(sp.Offset + sp.Length)})
	}

	// Whitespace separating two sentences belongs to neither. The cut is placed
	// at the sentence's first word, so the run before it is spacing the reader
	// should not see as the start of a line.
	const pad = " \t\r\n"

	out := make([]Segment, 0, len(bounds)-1)
	for k := 0; k+1 < len(bounds); k++ {
		lo, hi := bounds[k], bounds[k+1]
		raw := text[lo:hi]
		lead := len(raw) - len(strings.TrimLeft(raw, pad))
		trail := len(raw) - len(strings.TrimRight(raw, pad))
		if lead+trail >= len(raw) {
			// A piece that is nothing but spacing holds no sentence.
			continue
		}
		lo, hi = lo+lead, hi-trail

		p := Segment{
			Seq: s.Seq, ParaNo: s.ParaNo, Kind: s.Kind, PageNo: s.PageNo,
			TocName: s.TocName, TocLevel: s.TocLevel,
			Text: text[lo:hi],
		}
		// Edition page anchors opened the block this paragraph was; they belong
		// to where the block began, which is now the first sentence.
		if len(out) == 0 {
			p.Markers = s.Markers
		}
		// A variant reading is owned by the piece its position falls in, on the
		// raw bounds rather than the trimmed ones, so one that sits in the
		// spacing between two sentences is kept rather than dropped.
		for _, v := range variants {
			if v.at < bounds[k] || v.at >= bounds[k+1] {
				continue
			}
			at := clamp(v.at, lo, hi)
			p.Variants = append(p.Variants, Variant{
				Offset: um.unitAt(at) - um.unitAt(lo),
				Text:   v.text,
			})
		}
		for _, sp := range bold {
			a, b := clamp(sp[0], lo, hi), clamp(sp[1], lo, hi)
			if b <= a {
				continue
			}
			p.Bold = append(p.Bold, Span{
				Offset: um.unitAt(a) - um.unitAt(lo),
				Length: um.unitAt(b) - um.unitAt(a),
			})
		}
		p.HTML = renderHTML(p.Text, p.Variants, p.Bold)
		out = append(out, p)
	}
	if len(out) < 2 {
		// The cuts did not divide the text after all. Leaving the original
		// segment alone keeps its text and its markup byte-identical.
		return nil
	}
	return out
}

// atText pairs a byte position with the reading that stood there.
type atText struct {
	at   int
	text string
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// renderHTML rebuilds a segment's inline markup from the text it now holds and
// the runs recorded against it.
//
// Cutting the text invalidates the parent's markup string: its tags sit at byte
// positions that no longer exist, and a piece that kept the whole paragraph's
// markup would show the whole paragraph. So the markup is regenerated from
// exactly the data the client already rebuilds it from — the text, the variant
// offsets, the bold runs — which keeps the column saying what the reader draws
// rather than the result of offset arithmetic nobody can follow.
//
// The text is written back unescaped, which is what Markup does: the text has
// already been through html.UnescapeString, and re-escaping it here would make
// the two disagree about what the reading says.
func renderHTML(text string, variants []Variant, bold []Span) string {
	type event struct {
		at    int
		order int
		open  string
	}
	// At one position a closing tag has to come out before an opening one, or
	// the markup nests the wrong way round.
	const (
		orderClose = 0
		orderSelf  = 1
		orderOpen  = 2
	)
	evs := make([]event, 0, 2*len(bold)+len(variants))
	for _, sp := range bold {
		if sp.Length <= 0 {
			continue
		}
		evs = append(evs,
			event{at: sp.Offset, order: orderOpen, open: `<span class="b">`},
			event{at: sp.Offset + sp.Length, order: orderClose, open: `</span>`})
	}
	for _, v := range variants {
		evs = append(evs, event{at: v.Offset, order: orderSelf,
			open: `<span class="v">` + v.Text + `</span>`})
	}
	sort.SliceStable(evs, func(i, j int) bool {
		if evs[i].at != evs[j].at {
			return evs[i].at < evs[j].at
		}
		return evs[i].order < evs[j].order
	})

	um := newUnitMap(text)
	var b strings.Builder
	at := 0
	for _, e := range evs {
		if e.at > at {
			b.WriteString(text[um.byteAt(at):um.byteAt(e.at)])
			at = e.at
		}
		b.WriteString(e.open)
	}
	if end := um.byteAt(at); end < len(text) {
		b.WriteString(text[end:])
	}
	return b.String()
}

// unitMap is the two directions of the conversion between byte offsets and
// UTF-16 offsets for one text.
//
// Both are needed at once and neither can be derived from the other without
// walking the string: recorded spans are UTF-16 (a JavaScript string is indexed
// that way) while slicing happens in bytes. Every Pāḷi diacritic outside Latin-1
// — ā ī ū ṃ ṭ ḍ ṇ ḷ ṅ ñ — is two bytes but one unit, so the two drift apart from
// the first accented letter onwards.
type unitMap struct {
	bytes []int // byte offset of each rune, ascending
	units []int // its UTF-16 offset, ascending
}

func newUnitMap(s string) unitMap {
	m := unitMap{
		bytes: make([]int, 0, len(s)+1),
		units: make([]int, 0, len(s)+1),
	}
	u := 0
	for i, r := range s {
		m.bytes = append(m.bytes, i)
		m.units = append(m.units, u)
		u += utf16Units(r)
	}
	m.bytes = append(m.bytes, len(s))
	m.units = append(m.units, u)
	return m
}

// byteAt is the byte offset of a UTF-16 offset, rounded up to a rune boundary.
func (m unitMap) byteAt(unit int) int {
	if unit <= 0 {
		return m.bytes[0]
	}
	i := sort.SearchInts(m.units, unit)
	if i >= len(m.bytes) {
		return m.bytes[len(m.bytes)-1]
	}
	return m.bytes[i]
}

// unitAt is the UTF-16 offset of a byte offset. The byte offsets handed to it
// are rune boundaries — a cut, or a recorded span's own edge — so the answer is
// exact rather than rounded.
func (m unitMap) unitAt(b int) int {
	if b <= 0 {
		return m.units[0]
	}
	i := sort.SearchInts(m.bytes, b)
	if i >= len(m.units) {
		return m.units[len(m.units)-1]
	}
	return m.units[i]
}
