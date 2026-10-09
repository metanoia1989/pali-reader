package corpus

import (
	"strings"
	"testing"
)

// A prose paragraph that becomes several segments is the whole point of the
// exercise: the reader annotates a sentence, so a sentence has to be the unit
// the reader is given.
func TestSplitAtCutsAParagraphIntoSentences(t *testing.T) {
	text := "1. evaṃ me sutaṃ. atha kho bhagavā sāvatthiyaṃ viharati. idamavoca bhagavā."
	cut1 := strings.Index(text, "atha")
	cut2 := strings.Index(text, "idam")

	segs := SplitAt(Segment{Seq: 7, ParaNo: 1, Kind: KindProse, Text: text}, []int{0, cut1, cut2})
	if len(segs) != 3 {
		t.Fatalf("got %d segments, want 3: %+v", len(segs), segs)
	}
	want := []string{"1. evaṃ me sutaṃ.", "atha kho bhagavā sāvatthiyaṃ viharati.", "idamavoca bhagavā."}
	for i, w := range want {
		if segs[i].Text != w {
			t.Errorf("piece %d = %q, want %q", i, segs[i].Text, w)
		}
		if segs[i].ParaNo != 1 || segs[i].Kind != KindProse {
			t.Errorf("piece %d lost the paragraph it came from: %+v", i, segs[i])
		}
	}
	// Nothing may be lost: the pieces have to spell the paragraph back out.
	if got := strings.Join([]string{segs[0].Text, segs[1].Text, segs[2].Text}, " "); got != text {
		t.Errorf("text was not preserved:\n got %q\nwant %q", got, text)
	}
}

// Every position the splitter writes has to be re-based on the piece it lands
// in. Off by one here and the reader underlines a neighbouring word for the
// rest of the book.
func TestSplitAtRebasesRecordedSpans(t *testing.T) {
	text := "ādi paṭhamaṃ. dutiyaṃ bhavati."
	// "paṭhamaṃ" sits in the first sentence, "bhavati" in the second. Offsets
	// are counted out of the string, never by hand.
	boldAt := strings.Index(text, "paṭhamaṃ")
	varAt := strings.Index(text, "bhavati")
	cut := strings.Index(text, "dutiyaṃ")

	parent := Segment{
		Seq: 3, ParaNo: 9, Kind: KindProse, PageNo: 12, Text: text,
		Bold:     []Span{{Offset: utf16Len(text[:boldAt]), Length: utf16Len("paṭhamaṃ")}},
		Variants: []Variant{{Offset: utf16Len(text[:varAt]), Text: "[bhūti (ka.)]"}},
	}
	segs := SplitAt(parent, []int{cut})
	if len(segs) != 2 {
		t.Fatalf("got %d segments, want 2", len(segs))
	}

	if len(segs[0].Bold) != 1 || len(segs[1].Bold) != 0 {
		t.Fatalf("bold belongs to the first sentence, got %+v / %+v", segs[0].Bold, segs[1].Bold)
	}
	b := segs[0].Bold[0]
	if got := slice16(segs[0].Text, b.Offset, b.Length); got != "paṭhamaṃ" {
		t.Errorf("bold now covers %q, want %q", got, "paṭhamaṃ")
	}
	if len(segs[1].Variants) != 1 || len(segs[0].Variants) != 0 {
		t.Fatalf("the variant moved to the wrong piece: %+v / %+v", segs[0].Variants, segs[1].Variants)
	}
	v := segs[1].Variants[0]
	if got := slice16(segs[1].Text, v.Offset, utf16Len("bhavati")); got != "bhavati" {
		t.Errorf("the variant now points at %q, want the word it annotates", got)
	}
	if segs[0].PageNo != 12 || segs[1].PageNo != 12 {
		t.Errorf("page numbers were not inherited: %d / %d", segs[0].PageNo, segs[1].PageNo)
	}
}

// A sentence boundary can fall inside a bold run, and the commentary's emphasis
// is the only thing marking a headword as one. Both halves keep the part of the
// run that is theirs.
func TestSplitAtClipsRunsThatStraddleTheCut(t *testing.T) {
	text := "kāmayamānassā ti vuccati"
	run := "mānassā ti"
	cut := strings.Index(text, "ti vuccati")
	parent := Segment{
		Kind: KindProse, Text: text,
		Bold: []Span{{Offset: utf16Len(text[:strings.Index(text, run)]), Length: utf16Len(run)}},
	}
	segs := SplitAt(parent, []int{cut})
	if len(segs) != 2 {
		t.Fatalf("got %d segments, want 2", len(segs))
	}
	for i, want := range []string{"mānassā", "ti"} {
		if len(segs[i].Bold) != 1 {
			t.Fatalf("piece %d lost its half of the run: %+v", i, segs[i].Bold)
		}
		sp := segs[i].Bold[0]
		if got := slice16(segs[i].Text, sp.Offset, sp.Length); got != want {
			t.Errorf("piece %d emphasises %q, want %q", i, got, want)
		}
	}
}

// Markup is stripped from the text, so a piece that kept the parent's markup
// would show the whole paragraph again. The markup is rebuilt from the piece's
// own text and its own recorded spans, and it has to agree with what the text
// says.
func TestSplitAtRebuildsMarkupForEachPiece(t *testing.T) {
	text := "evaṃ me sutaṃ. idaṃ vuttaṃ hoti."
	cut := strings.Index(text, "idaṃ")
	at := strings.Index(text, "vuttaṃ")
	parent := Segment{
		Kind: KindProse, Text: text,
		Variants: []Variant{{Offset: utf16Len(text[:at]), Text: "[vutta (syā.)]"}},
	}
	segs := SplitAt(parent, []int{cut})
	if len(segs) != 2 {
		t.Fatalf("got %d segments, want 2", len(segs))
	}
	if strings.Contains(segs[0].HTML, "vutta") {
		t.Errorf("the first piece carries the second piece's markup: %q", segs[0].HTML)
	}
	want := `idaṃ <span class="v">[vutta (syā.)]</span>vuttaṃ hoti.`
	if segs[1].HTML != want {
		t.Errorf("markup = %q, want %q", segs[1].HTML, want)
	}
	// What is left once the variant span is taken back out is the piece's own
	// text: the reading comes from the text, the variant is the part of the
	// edition the text deliberately does not hold.
	if got := strings.Replace(segs[1].HTML, `<span class="v">[vutta (syā.)]</span>`, "", 1); got != segs[1].Text {
		t.Errorf("markup reads %q, text reads %q", got, segs[1].Text)
	}
}

// The markup keeps whatever the text says, including a literal angle bracket:
// the text has already been through html.UnescapeString, and re-escaping it
// here would make the markup disagree with the reading.
func TestRenderHTMLKeepsTextVerbatim(t *testing.T) {
	got := renderHTML("idaṃ <vuttaṃ> hoti", nil, nil)
	if got != "idaṃ <vuttaṃ> hoti" {
		t.Errorf("renderHTML = %q", got)
	}
	// An empty run must not emit a pair of tags round nothing.
	if got := renderHTML("a b", nil, []Span{{Offset: 1, Length: 0}}); got != "a b" {
		t.Errorf("renderHTML = %q, want the text unchanged", got)
	}
}

// A paragraph nothing could be found for stays one segment. This is the
// failure mode that matters: losing a sentence to a bad split is worse than
// leaving a paragraph long.
func TestSplitAtLeavesUntouchedWithoutUsableCuts(t *testing.T) {
	s := Segment{Seq: 4, Kind: KindProse, Text: "evaṃ me sutaṃ."}
	for _, cuts := range [][]int{nil, {}, {0}, {len(s.Text)}, {999}, {0, 0}} {
		if got := SplitAt(s, cuts); got != nil {
			t.Errorf("cuts %v produced %d pieces, want none", cuts, len(got))
		}
	}
	// And a heading is never cut, whatever offsets arrive for it: the table of
	// contents points at it by seq.
	segs := []Segment{{Seq: 1, Kind: KindHeading, Text: "1. brahmajālasuttaṃ"}}
	got := SplitSegments(segs, [][]int{{5}})
	if len(got) != 1 || got[0].Text != segs[0].Text || got[0].Seq != 1 {
		t.Errorf("a heading was split: %+v", got)
	}
	// A centred line is a line of verse the edition did not mark as one, and it
	// is one indivisible unit for the same reason a heading is.
	centre := []Segment{{Seq: 1, Kind: KindCenter, Text: "ime ce pañca bhaṇḍāni,\npahāya yathā kathaṃci."}}
	if got := SplitSegments(centre, [][]int{{22}}); len(got) != 1 || got[0].Text != centre[0].Text {
		t.Errorf("a centred line was split: %+v", got)
	}
}

// A verse is cut at its own line breaks, and every piece is one printed line:
// no piece may keep a break, or the reader is handed a paragraph where the
// edition prints a line.
func TestSplitSegmentsCutsAVerseIntoItsLines(t *testing.T) {
	text := "13. vanappagumbe yatha phussitagge,\ngimhānamāse paṭhamasmiṃ gimhe,\nidampi buddhe ratanaṃ paṇītaṃ."
	cuts := []int{}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			cuts = append(cuts, i+1)
		}
	}
	segs := SplitSegments([]Segment{{Seq: 9, ParaNo: 13, Kind: KindVerse, Text: text}}, [][]int{cuts})
	if len(segs) != 3 {
		t.Fatalf("got %d segments, want 3: %q", len(segs), texts(segs))
	}
	for i, s := range segs {
		if strings.ContainsRune(s.Text, '\n') {
			t.Errorf("piece %d still holds a line break: %q", i, s.Text)
		}
		if s.Kind != KindVerse || s.ParaNo != 13 {
			t.Errorf("piece %d lost its kind or paragraph: %+v", i, s)
		}
		if s.Seq != i+1 {
			t.Errorf("piece %d has seq %d", i, s.Seq)
		}
	}
	// The number appears once, on the first line, and nothing is lost.
	if got := strings.Count(strings.Join(texts(segs), ""), "13."); got != 1 {
		t.Errorf("the verse number appears %d times", got)
	}
	if !strings.HasPrefix(segs[0].Text, "13. ") {
		t.Errorf("the number is not on the first line: %q", segs[0].Text)
	}
	joined := strings.Join(texts(segs), "")
	if stripSpace(joined) != stripSpace(text) {
		t.Errorf("the verse was not preserved:\n got %q\nwant %q", joined, text)
	}
}

// stripSpace is the losslessness check's own comparison: everything that is
// whitespace is dropped, and what is left has to be identical.
func stripSpace(s string) string {
	return strings.NewReplacer(" ", "", "\n", "", "\t", "", "\r", "").Replace(s)
}

// texts is the reading of each segment in order.
func texts(segs []Segment) []string {
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		out = append(out, s.Text)
	}
	return out
}

// Seq is a per-book ordinal and every anchor in the application hangs off it, so
// splitting has to leave it contiguous and in reading order.
func TestSplitSegmentsRenumbersSeq(t *testing.T) {
	segs := []Segment{
		{Seq: 1, Kind: KindHeading, Text: "sagāthāvaggo"},
		{Seq: 2, Kind: KindProse, Text: "paṭhamaṃ bhavati. dutiyaṃ bhavati."},
		{Seq: 3, Kind: KindVerse, Text: "a b c,\nd e f."},
		{Seq: 4, Kind: KindProse, Text: "tatiyaṃ bhavati."},
	}
	cut := strings.Index(segs[1].Text, "dutiyaṃ")
	got := SplitSegments(segs, [][]int{nil, {cut}, nil, nil})

	if len(got) != 5 {
		t.Fatalf("got %d segments, want 5", len(got))
	}
	for i, s := range got {
		if s.Seq != i+1 {
			t.Errorf("segment at index %d has seq %d", i, s.Seq)
		}
	}
	if got[0].Kind != KindHeading || got[3].Kind != KindVerse {
		t.Errorf("kinds were disturbed: %q %q", got[0].Kind, got[3].Kind)
	}
	if got[1].Text != "paṭhamaṃ bhavati." || got[2].Text != "dutiyaṃ bhavati." {
		t.Errorf("split pieces out of order: %q / %q", got[1].Text, got[2].Text)
	}
}

// slice16 reads a text the way the client does: by UTF-16 offset and length.
// Written out rather than taken from unitMap so the test does not check the
// conversion with the conversion.
func slice16(s string, off, length int) string {
	u, start, end := 0, -1, -1
	for i, r := range s {
		if u == off {
			start = i
		}
		if u == off+length {
			end = i
		}
		u += utf16Units(r)
	}
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = len(s)
	}
	return s[start:end]
}
