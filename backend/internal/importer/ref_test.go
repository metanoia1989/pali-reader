package importer

import (
	"strings"
	"testing"
)

func TestSplitSentencesKeepsMarks(t *testing.T) {
	got := splitSentences("如是我闻。一时，世尊住舍卫城！诸比丘？")
	want := []string{"如是我闻。", "一时，世尊住舍卫城！", "诸比丘？"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q", got)
	}
	// A fragment with no letters must not become a sentence of its own: the
	// closing quote after a full stop belongs to the sentence before it.
	got = splitSentences("他说：“苦。””然后离开。")
	if len(got) != 2 {
		t.Fatalf("got %d sentences: %q", len(got), got)
	}
	if !strings.Contains(got[0], "苦") || !strings.Contains(got[0], "”") {
		t.Errorf("the stray quote was not merged back: %q", got[0])
	}
}

// The unit that carries a translation is the ePitaka line. Its key —
// (book_id, para_id, line_id) — names a row in the Pali file and the same row
// in the language file, so a line's Chinese is the row that shares its key and
// nothing is inferred. What the aligner has to decide is only which of this
// reader's segments holds that line.
func TestAnchorLinesPutsEachLineOnItsOwnSegment(t *testing.T) {
	flat := strings.Fields("alpha beta gamma delta epsilon zeta")
	lines := []epiLine{
		{para: 1, line: 1, words: []string{"alpha", "beta"}, zh: "甲。"},
		{para: 1, line: 2, words: []string{"gamma", "delta"}, zh: "乙。"},
		{para: 1, line: 3, words: []string{"epsilon", "zeta"}, zh: "丙。"},
	}
	anchors, st := anchorLines(flat, lines, false)
	if st.Anchored != 3 {
		t.Fatalf("anchored %d of 3: %+v", st.Anchored, st)
	}
	for i, want := range []int{0, 2, 4} {
		if anchors[i].At != want {
			t.Errorf("line %d anchored at %d, want %d", i+1, anchors[i].At, want)
		}
		if anchors[i].Idx != i {
			t.Errorf("line %d reports index %d", i+1, anchors[i].Idx)
		}
	}
}

// A line the book does not contain is dropped. It is not moved to wherever the
// reading had got to: a translation under the wrong sentence is worse than no
// translation, and the old behaviour here — attach it to the cursor — is what
// stacked a dozen Chinese sentences under one short paragraph.
func TestAnchorLinesDropsALineTheBookDoesNotHave(t *testing.T) {
	flat := strings.Fields("alpha beta gamma delta")
	lines := []epiLine{
		{para: 1, line: 1, words: []string{"alpha", "beta"}, zh: "甲。"},
		{para: 1, line: 2, words: []string{"sigma", "tau"}, zh: "无。"},
		{para: 1, line: 3, words: []string{"gamma", "delta"}, zh: "乙。"},
	}
	anchors, st := anchorLines(flat, lines, false)
	if st.Anchored != 2 {
		t.Fatalf("anchored %d, want 2", st.Anchored)
	}
	if st.NoVerbatim != 1 {
		t.Errorf("NoVerbatim = %d, want 1", st.NoVerbatim)
	}
	for _, a := range anchors {
		if a.Line == 2 {
			t.Fatalf("the line the book does not have was placed at %d", a.At)
		}
	}
}

// Two lines cannot claim the same text. The second of a repeated pair is
// dropped rather than stacked on the first, which is the other half of the
// defect: the same words placed twice put two translations under one sentence.
func TestAnchorLinesNeverStacksTwoLinesOnOneStretch(t *testing.T) {
	flat := strings.Fields("alpha beta gamma delta")
	lines := []epiLine{
		{para: 1, line: 1, words: []string{"alpha", "beta", "gamma", "delta"}, zh: "甲。"},
		{para: 1, line: 2, words: []string{"alpha", "beta", "gamma", "delta"}, zh: "乙。"},
	}
	anchors, st := anchorLines(flat, lines, false)
	if st.Anchored != 1 {
		t.Fatalf("anchored %d, want 1", st.Anchored)
	}
	if st.Contradicted != 1 {
		t.Errorf("Contradicted = %d, want 1", st.Contradicted)
	}
	if anchors[0].Line != 1 {
		t.Errorf("the first line was not the one kept: %+v", anchors[0])
	}
}

// The editions place the elision apostrophe differently, which can differ
// inside a line's opening. The line is still found, by stepping over the words
// that differ — and it is counted as having done so, so the anchors that rest
// on the assumption can be told from the ones that do not.
func TestAnchorLinesStepsOverAnOpeningThatDiffers(t *testing.T) {
	flat := strings.Fields("alpha beta gamma delta epsilon")
	lines := []epiLine{
		{para: 1, line: 1, words: []string{"other", "beta", "gamma", "delta", "epsilon"}, zh: "甲。"},
	}
	loose, st := anchorLines(flat, lines, false)
	if st.Anchored != 1 {
		t.Fatalf("not anchored: %+v", st)
	}
	if loose[0].Off == 0 {
		t.Errorf("Off = 0, want the step-over to be recorded")
	}
	if st.Exact != 0 {
		t.Errorf("Exact = %d, want 0", st.Exact)
	}
	// Asked only for the anchors it can vouch for, the aligner refuses it.
	_, strict := anchorLines(flat, lines, true)
	if strict.Anchored != 0 {
		t.Errorf("the strict pass anchored a line whose opening differs")
	}
}

// The index has to carry every anchor length locate() falls back through. It
// used to hold only six-word keys while the search tried five and four, so the
// fallback could never hit and a five-word paragraph — a verse, most often —
// could not be anchored at all. Its translation was silently dropped.
func TestLocateAnchorsAShortParagraph(t *testing.T) {
	flat := strings.Fields("evaṃ me sutaṃ atha kho cirassaṃ vata passāmi brāhmaṇaṃ parinibbutaṃ")
	index := map[string][]int{}
	for i := range flat {
		for n := minShingle; n <= shingle && i+n <= len(flat); n++ {
			index[strings.Join(flat[i:i+n], " ")] = append(index[strings.Join(flat[i:i+n], " ")], i)
		}
	}
	// Five words: too short for a six-word anchor, which is the case that was
	// broken.
	words := strings.Fields("cirassaṃ vata passāmi brāhmaṇaṃ parinibbutaṃ")
	pos, ok := locate(flat, index, words, 0, len(flat), 0)
	if !ok {
		t.Fatal("a five-word paragraph failed to anchor")
	}
	if pos != 5 {
		t.Errorf("anchored at %d, want 5", pos)
	}
}

// Nearly a third of the published rows carry inline markup, because ePitaka
// writes its translations as HTML. This reader renders a translation as text,
// so the tags have to go — a page showing a literal `<i>Sangha</i>` is worse
// than one without the italics.
func TestCleanTranslationRemovesInlineMarkup(t *testing.T) {
	got := cleanTranslation(`  [以及]具足戒等诸功德之<i>僧</i>，摄取一切言词之精华  `)
	want := `[以及]具足戒等诸功德之僧，摄取一切言词之精华`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// Text with no markup keeps its own spacing; only the ends are trimmed.
	if got := cleanTranslation("  a  b\nc "); got != "a  b\nc" {
		t.Errorf("plain text was altered: %q", got)
	}
	// An unbalanced or unknown tag is removed rather than interpreted.
	if got := cleanTranslation(`x<sup>2</sup>y<script>alert(1)</script>z`); got != "x2yalert(1)z" {
		t.Errorf("got %q", got)
	}
}

// The Pāḷi side carries markup too — 366,473 of ePitaka's 1.28 million rows,
// nearly all of them in the commentaries, wrap a headword in `<b>`, which is
// what this reader's CST pages write as `<span class="bld">` and cut out of the
// text. Left in, the tag is not markup to a word splitter but two more words:
// `b tatthā b ti` where this book has `tatthā ti` — and since a commentary
// begins its sentences at its headwords, that is exactly where the alignment
// needs to agree with the book.
func TestCleanInlineTagsStripsTheMarkupFromPali(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Taṃ pana yamakaṃ <b>subodhālaṅkāre</b> kiliṭṭhadosanti vuttaṃ.",
			"Taṃ pana yamakaṃ subodhālaṅkāre kiliṭṭhadosanti vuttaṃ."},
		{"eko <i>dīgho</i> <sup>1</sup> pāṭho", "eko dīgho 1 pāṭho"},
		{"no markup here", "no markup here"},
		// A '<' that never opens a tag is text, and is left where it stands.
		{"a < b c", "a < b c"},
	} {
		if got := cleanInlineTags(tc.in); got != tc.want {
			t.Errorf("cleanInlineTags(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// The tag must leave no word of its own behind: this reader's text of
	// `<span class="bld">tatthā</span>ti` is the single word `tatthāti`, and
	// ePitaka's of `<b>tatthā</b>ti` has to come out the same way. Untouched it
	// comes out as four words, two of which are the letter `b`.
	got := alignWords(dropVariantReadings(cleanInlineTags("b <b>tatthā</b>ti")))
	if strings.Join(got, " ") != "b tatthāti" {
		t.Errorf("the tag left words behind: %q", got)
	}
}
