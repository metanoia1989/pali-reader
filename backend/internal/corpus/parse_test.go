package corpus

import (
	"strings"
	"testing"
)

// The page HTML below is the opening of sīlakkhandhavaggapāḷi (DN 1) reduced to
// the shapes that matter: a run of headings before any text, paragraph anchors
// that live *inside* their paragraph element, a hangnum block that carries
// nothing but a number, a variant reading, an edition page anchor, and a verse.
const pageHTML = `<p class="nikaya">dīghanikāyo</p>
<a name="dn1"></a><p class="book">sīlakkhandhavaggapāḷi</p>
<p class="centered"> namo tassa bhagavato arahato sammāsambuddhassa</p>
<a name="dn1_1"></a><p class="chapter">1. brahmajālasuttaṃ</p>
<p class="subhead">paribbājakakathā</p>
<p class="bodytext"><a name="para1"></a><a name="para1_dn1"></a><span class="paranum">1</span>. evaṃ <a name="M1.0001"></a> me sutaṃ – ekaṃ samayaṃ bhagavā antarā ca rājagahaṃ antarā ca nāḷandaṃ addhānamaggappaṭipanno hoti. suppiyopi kho paribbājako <span class="note">[suppiyo (sī.)]</span> anekapariyāyena buddhassa avaṇṇaṃ bhāsati.</p>
<p class="hangnum"><a name="para2"></a><a name="para2_dn1"></a><span class="paranum">2</span>.</p>
<p class="gatha1">makkaṭī vajjiputtā ca, gihī naggo ca titthiyā,</p>
<p class="gathalast">dārikuppalavaṇṇā ca, byañjanehipare duve.</p>
<p class="bodytext"><a name="para3"></a><span class="paranum">3</span>. atha kho bhagavā tesaṃ bhikkhūnaṃ imaṃ saṅkhiyadhammaṃ viditvā yena maṇḍalamāḷo tenupasaṅkami.</p>`

func TestParseBookKeepsHeadingsAsSegments(t *testing.T) {
	segs := ParseBook(pageHTML, []int{0})

	var headings []string
	for _, s := range segs {
		if s.Kind == KindHeading {
			headings = append(headings, s.Text)
		}
	}
	// The front of a volume has several headings in a row. Absorbing each into
	// the next block loses all but the last, which is what used to happen.
	want := []string{"dīghanikāyo", "sīlakkhandhavaggapāḷi", "1. brahmajālasuttaṃ", "paribbājakakathā"}
	if strings.Join(headings, "|") != strings.Join(want, "|") {
		t.Fatalf("headings = %q, want %q", headings, want)
	}
	for _, s := range segs {
		if s.Kind == KindHeading && s.TocName != s.Text {
			t.Errorf("heading %q should open a TOC entry, got %q", s.Text, s.TocName)
		}
	}
}

func TestParseBookReadsParagraphAnchors(t *testing.T) {
	segs := ParseBook(pageHTML, []int{0})

	// Every paragraph anchor in this edition sits inside its <p> element, so a
	// parser that only looks between elements sees no paragraph numbers at all.
	byText := map[string]int{}
	for _, s := range segs {
		byText[s.Text[:min(20, len(s.Text))]] = s.ParaNo
	}
	for _, s := range segs {
		if s.Kind != KindProse && s.Kind != KindVerse {
			continue
		}
		if s.ParaNo == 0 {
			t.Errorf("segment %d (%q…) has no paragraph number", s.Seq, s.Text[:min(24, len(s.Text))])
		}
	}

	// The verse has no anchor of its own and inherits paragraph 2 from the
	// hangnum block above it.
	for _, s := range segs {
		if s.Kind == KindVerse && s.ParaNo != 2 {
			t.Errorf("verse inherited paragraph %d, want 2", s.ParaNo)
		}
	}
}

func TestParseBookGroupsVersesAndKeepsVariants(t *testing.T) {
	segs := ParseBook(pageHTML, []int{0})

	var verse *Segment
	for i := range segs {
		if segs[i].Kind == KindVerse {
			verse = &segs[i]
		}
	}
	if verse == nil {
		t.Fatal("no verse segment")
	}
	if !strings.Contains(verse.Text, "\n") {
		t.Errorf("verse lines should be kept as lines, got %q", verse.Text)
	}
	if strings.Contains(verse.Text, "[") {
		t.Errorf("plain text should not carry variant readings: %q", verse.Text)
	}

	var prose *Segment
	for i := range segs {
		if segs[i].Kind == KindProse && strings.Contains(segs[i].Text, "evaṃ") {
			prose = &segs[i]
		}
	}
	if prose == nil {
		t.Fatal("no prose segment")
	}
	if strings.Contains(prose.Text, "suppiyo (sī.)") {
		t.Errorf("variant reading leaked into the plain text: %q", prose.Text)
	}
	if !strings.Contains(prose.HTML, `<span class="v">[suppiyo (sī.)]</span>`) {
		t.Errorf("variant reading should survive in the markup as <span class=v>: %q", prose.HTML)
	}
	if len(prose.Markers) != 1 || prose.Markers[0] != "M1.0001" {
		t.Errorf("edition markers = %v, want [M1.0001]", prose.Markers)
	}
}

// The paragraph number stays in the text.
//
// It was briefly stripped so the reader could draw its own § in the margin.
// That was wrong: the canon's numbering is the one checked by hand, and a
// number we synthesise carries no such guarantee. The reader shows the source's
// number, in bold, exactly where the source put it.
func TestParseBookKeepsTheSourceParagraphNumber(t *testing.T) {
	segs := ParseBook(pageHTML, []int{0})
	var first *Segment
	for i := range segs {
		if strings.Contains(segs[i].Text, "evaṃ me sutaṃ") {
			first = &segs[i]
		}
	}
	if first == nil {
		t.Fatal("no first paragraph")
	}
	if !strings.HasPrefix(first.Text, "1. evaṃ me sutaṃ") {
		t.Errorf("the source's number should lead the text, got %q", first.Text[:40])
	}
	// It is still recorded separately, so the segment can be cited by it.
	if first.ParaNo != 1 {
		t.Errorf("paragraph number = %d, want 1", first.ParaNo)
	}
}

// A paragraph that continues the previous one carries no number, and must not
// have its opening words mistaken for one.
func TestParseBookLeavesUnnumberedParagraphsAlone(t *testing.T) {
	html := `<p class="bodytext">no number here, just text that goes on and on.</p>`
	segs := ParseBook(html, []int{0})
	if len(segs) != 1 {
		t.Fatalf("got %d segments", len(segs))
	}
	if !strings.HasPrefix(segs[0].Text, "no number here") {
		t.Errorf("text was altered: %q", segs[0].Text)
	}
}

// Variant readings are removed from the text but not thrown away: the reader
// can switch them on, and they have to be put back where they stood.
func TestSplitRunsKeepsVariantPosition(t *testing.T) {
	text, variants, _ := SplitRuns(`evaṃ me <span class="note">[evaṃ (sī.)]</span>sutaṃ`)
	if text != "evaṃ me sutaṃ" {
		t.Fatalf("text = %q", text)
	}
	// A variant surrounded by spaces must not leave two of them behind: the
	// sentinel sat between the spaces and whitespace was collapsed before it
	// was resolved.
	if got, _, _ := SplitRuns(`anubandhā <span class="note">[x]</span> honti`); got != "anubandhā honti" {
		t.Errorf("spacing after removal = %q", got)
	}
	if len(variants) != 1 {
		t.Fatalf("got %d variants, want 1", len(variants))
	}
	if variants[0].Text != "[evaṃ (sī.)]" {
		t.Errorf("variant text = %q", variants[0].Text)
	}
	// Position is measured in UTF-16 units, the unit the client slices by.
	if variants[0].Offset != len([]rune("evaṃ me ")) {
		t.Errorf("offset = %d, want %d", variants[0].Offset, len([]rune("evaṃ me ")))
	}
}

// The commentary bolds the word it is about, and that emphasis is the only
// thing marking it as a headword. Stripping the markup left the text flat, so
// the runs are recorded and the client puts them back.
func TestSplitRunsRecordsBoldRuns(t *testing.T) {
	text, _, bold := SplitRuns(`atha <span class="bld">kāmayamānassā</span>ti vuttaṃ`)
	if text != "atha kāmayamānassāti vuttaṃ" {
		t.Fatalf("text = %q", text)
	}
	if len(bold) != 1 {
		t.Fatalf("got %d bold runs, want 1", len(bold))
	}
	if bold[0].Offset != len([]rune("atha ")) {
		t.Errorf("offset = %d, want %d", bold[0].Offset, len([]rune("atha ")))
	}
	if got := string([]rune(text)[bold[0].Offset : bold[0].Offset+bold[0].Length]); got != "kāmayamānassā" {
		t.Errorf("run covers %q", got)
	}
}

// A verse's number sits in a block of its own, and that block also marks where
// one verse ends and the next begins. Joining them all into one segment lost
// the boundaries the edition prints.
func TestParseBookSeparatesVersesOnNumberBlocks(t *testing.T) {
	html := `<p class="hangnum"><a name="para1"></a><span class="paranum">1</span>.</p>
<p class="gatha1">manopubbaṅgamā dhammā,</p>
<p class="gathalast">tato naṃ dukkhamanveti.</p>
<p class="hangnum"><a name="para2"></a><span class="paranum">2</span>.</p>
<p class="gatha1">manopubbaṅgamā dhammā,</p>
<p class="gathalast">tato naṃ sukhamanveti.</p>`
	segs := ParseBook(html, []int{0})
	var verses []Segment
	for _, s := range segs {
		if s.Kind == KindVerse {
			verses = append(verses, s)
		}
	}
	if len(verses) != 2 {
		t.Fatalf("got %d verse segments, want 2", len(verses))
	}
	if !strings.HasPrefix(verses[0].Text, "1. ") || !strings.HasPrefix(verses[1].Text, "2. ") {
		t.Errorf("verses should carry their own numbers: %q / %q", verses[0].Text, verses[1].Text)
	}
	// The lines stay as lines: the client turns the newlines into breaks.
	if !strings.Contains(verses[0].Text, "\n") {
		t.Errorf("verse lines were joined: %q", verses[0].Text)
	}
}

func TestParseBookDropsNumberOnlyBlocks(t *testing.T) {
	segs := ParseBook(pageHTML, []int{0})
	for _, s := range segs {
		if s.Kind == KindProse && strings.TrimSpace(s.Text) == "2." {
			t.Errorf("a hangnum block became a readable segment")
		}
	}
}

func TestPlainTextAndMarkup(t *testing.T) {
	in := `a <b>x</b> <span class="bld">kāmayamānassā</span>ti <span class="note">[y]</span> b`
	// The variant reading is removed whole, not left as bare text in the middle
	// of the sentence it annotates.
	if got := PlainText(in); got != "a x kāmayamānassāti b" {
		t.Errorf("PlainText = %q", got)
	}
	if got := Markup(in); !strings.Contains(got, `<span class="b">`) || !strings.Contains(got, `<span class="v">`) {
		t.Errorf("Markup lost a whitelisted span: %q", got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
