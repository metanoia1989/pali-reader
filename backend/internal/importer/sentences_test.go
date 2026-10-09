package importer

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/metanoia/pali-reader/backend/internal/corpus"

	_ "modernc.org/sqlite"
)

// The canon writes `…etadavoca – "‘kathaṃ nu tvaṃ…`. A cut placed at the word
// alone would leave the dash and the opening quote at the end of the sentence
// before it, where they read as a line that trails off.
func TestSentenceStartKeepsTheLeadInWithItsSentence(t *testing.T) {
	text := `ekamantaṃ ṭhitā kho sā devatā bhagavantaṃ etadavoca – "‘kathaṃ nu tvaṃ, mārisa, oghamatarī’ti? ‘appatiṭṭhaṃ khvāhaṃ`
	at := sentenceStart(text, strings.Index(text, "kathaṃ"), 0)
	if got := strings.TrimLeft(text[at:], " \t"); !strings.HasPrefix(got, `– "‘kathaṃ`) {
		t.Errorf("cut leaves %q", got)
	}

	// A full stop is not a lead-in: the question mark closes the sentence
	// before it and stays there.
	at = sentenceStart(text, strings.Index(text, "appatiṭṭhaṃ"), 0)
	if got := strings.TrimLeft(text[at:], " \t"); !strings.HasPrefix(got, "‘appatiṭṭhaṃ") {
		t.Errorf("cut leaves %q", got)
	}
	if before := text[:at]; !strings.HasSuffix(before, "?") {
		t.Errorf("the question mark moved: %q", before)
	}

	// The floor is the previous cut and is never crossed.
	if got := sentenceStart(text, strings.Index(text, "appatiṭṭhaṃ"), 10); got < 10 {
		t.Errorf("walked back past the floor to %d", got)
	}
}

func TestOwnerAtFindsTheSegmentHoldingTheWord(t *testing.T) {
	// Segment 1 has no words of its own, so it starts where segment 2 does and
	// owns nothing.
	starts := []int{0, 5, 5, 9}
	for _, tc := range []struct{ at, want int }{
		{0, 0}, {4, 0}, {5, 2}, {8, 2}, {9, 3}, {40, 3},
	} {
		if got := ownerAt(starts, tc.at); got != tc.want {
			t.Errorf("ownerAt(%d) = %d, want %d", tc.at, got, tc.want)
		}
	}
	if got := ownerAt(nil, 3); got != -1 {
		t.Errorf("ownerAt on an empty stream = %d", got)
	}
}

// The end-to-end path: ePitaka's rows located in this reader's word stream, the
// boundaries turned into byte offsets in the segment's own text, and the
// paragraph cut there. This is the whole mechanism on a passage that has the
// shape of the real thing — a numbered paragraph of four sentences, two of
// which ePitaka spells with more words than this reader holds, a verse of two
// lines, and headings on either side.
func TestCutsForSplitsProseAndVerse(t *testing.T) {
	prose := "1. evaṃ me sutaṃ – ekaṃ samayaṃ bhagavā sāvatthiyaṃ viharati jetavane anāthapiṇḍikassa ārāme. " +
		"atha kho aññatarā devatā abhikkantāya rattiyā yena bhagavā tenupasaṅkami. " +
		"upasaṅkamitvā bhagavantaṃ abhivādetvā ekamantaṃ aṭṭhāsi. " +
		"idamavoca sā devatā."
	verse := "cirassaṃ vata passāmi, brāhmaṇaṃ parinibbutaṃ;\nappatiṭṭhaṃ anāyūhaṃ, tiṇṇaṃ loke visattikan\"ti."
	segs := []corpus.Segment{
		{Seq: 1, Kind: corpus.KindHeading, Text: "1. oghataraṇasuttaṃ"},
		{Seq: 2, ParaNo: 1, Kind: corpus.KindProse, Text: prose},
		{Seq: 3, ParaNo: 1, Kind: corpus.KindVerse, Text: verse},
		{Seq: 4, Kind: corpus.KindHeading, Text: "2. nimokkhasuttaṃ"},
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE sentences(book_id TEXT, para_id INT, line_id INT, pali TEXT)`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	for _, r := range []struct {
		para, line int
		pali       string
	}{
		{1, 1, "1. Oghataraṇasuttaṃ"},
		{2, 1, "1. Evaṃ me sutaṃ – ekaṃ samayaṃ bhagavā sāvatthiyaṃ viharati jetavane anāthapiṇḍikassa ārāme."},
		// The second line carries a variant reading, which this reader cuts out
		// of its text and ePitaka keeps inline. Counted as words it makes the
		// line longer here than there, and the walk past it steps over the next
		// sentence's opening.
		{2, 2, "Atha kho aññatarā devatā abhikkantāya rattiyā yena bhagavā tenupasaṅkami [upasaṅkami (ka.)]."},
		// And this one is simply wordier than this reader's text, which is the
		// same problem with no bracket to explain it.
		{2, 3, "Upasaṅkamitvā bhagavantaṃ abhivādetvā ekamantaṃ aṭṭhāsi idāni."},
		{2, 4, "Idamavoca sā devatā."},
		// One row per verse *line*, which is what ePitaka publishes for a gāthā.
		{3, 1, "Cirassaṃ vata passāmi, brāhmaṇaṃ parinibbutaṃ;"},
		{4, 1, "Appatiṭṭhaṃ anāyūhaṃ, tiṇṇaṃ loke visattikan\"ti."},
		{5, 1, "2. Nimokkhasuttaṃ"},
	} {
		if _, err := db.Exec(`INSERT INTO sentences VALUES('EPI-x', ?, ?, ?)`, r.para, r.line, r.pali); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	src := &sentenceSource{db: db, mapping: map[string][]string{"mula_x_01": {"EPI-x"}}}
	cuts := src.cutsFor("mula_x_01", segs)

	want := []string{
		"1. evaṃ me sutaṃ – ekaṃ samayaṃ bhagavā sāvatthiyaṃ viharati jetavane anāthapiṇḍikassa ārāme.",
		"atha kho aññatarā devatā abhikkantāya rattiyā yena bhagavā tenupasaṅkami.",
		"upasaṅkamitvā bhagavantaṃ abhivādetvā ekamantaṃ aṭṭhāsi.",
		"idamavoca sā devatā.",
	}
	got := corpus.SplitSegments(segs, cuts)
	if len(got) != 8 {
		t.Fatalf("got %d segments, want 8: %+v", len(got), texts(got))
	}
	if got[0].Text != "1. oghataraṇasuttaṃ" {
		t.Errorf("heading moved: %q", got[0].Text)
	}
	for i, w := range want {
		if got[1+i].Text != w {
			t.Errorf("sentence %d = %q, want %q", i, got[1+i].Text, w)
		}
		if got[1+i].Kind != corpus.KindProse || got[1+i].ParaNo != 1 {
			t.Errorf("sentence %d lost its kind or paragraph: %+v", i, got[1+i])
		}
	}
	// The verse becomes one segment per line: ePitaka's two rows are the two
	// lines the edition prints, and a line is what a translation lands on.
	lines := []string{
		"cirassaṃ vata passāmi, brāhmaṇaṃ parinibbutaṃ;",
		"appatiṭṭhaṃ anāyūhaṃ, tiṇṇaṃ loke visattikan\"ti.",
	}
	for i, w := range lines {
		if got[5+i].Text != w {
			t.Errorf("verse line %d = %q, want %q", i, got[5+i].Text, w)
		}
		if got[5+i].Kind != corpus.KindVerse {
			t.Errorf("verse line %d lost its kind: %+v", i, got[5+i])
		}
		if strings.ContainsRune(got[5+i].Text, '\n') {
			t.Errorf("verse line %d still carries a break: %q", i, got[5+i].Text)
		}
	}
	if got[7].Text != "2. nimokkhasuttaṃ" {
		t.Errorf("the second heading moved: %q", got[7].Text)
	}
	// Nothing may be lost, and Seq has to stay contiguous for the anchors.
	joined := strings.Join(texts(got)[1:5], " ")
	if joined != prose {
		t.Errorf("text was not preserved:\n got %q\nwant %q", joined, prose)
	}
	if rejoined := strings.Join(texts(got)[5:7], ""); stripAll(rejoined) != stripAll(verse) {
		t.Errorf("verse was not preserved:\n got %q\nwant %q", rejoined, verse)
	}
	for i, s := range got {
		if s.Seq != i+1 {
			t.Errorf("segment %d has seq %d", i, s.Seq)
		}
	}
	if src.unplaced != 0 {
		t.Errorf("%d ePitaka lines were not placed", src.unplaced)
	}
}

// The verse number is written once, at the head of the gāthā, and ePitaka
// writes it at the head of every line of one. A line whose row had to step over
// that number to be placed must still be a line the reader is given, and the
// number must stay where this edition wrote it: on the first line, once.
func TestCutsForKeepsTheVerseNumberOnTheFirstLine(t *testing.T) {
	// Two lines of one gāthā. The first row carries the number; the second is
	// spelled by ePitaka with a number of its own that this edition does not
	// have there.
	verse := "2. yo rāgamudacchidā asesaṃ, bhisapupphaṃva saroruhaṃ vigayha,\nso bhikkhu jahāti orapāraṃ, urago jiṇṇamivattacaṃ purāṇaṃ."
	segs := []corpus.Segment{
		{Seq: 1, ParaNo: 2, Kind: corpus.KindVerse, Text: verse},
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE sentences(book_id TEXT, para_id INT, line_id INT, pali TEXT)`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	for _, r := range []struct {
		para, line int
		pali       string
	}{
		// The numeral is stepped over: it is not a word of the line.
		{1, 1, "3 yo rāgamudacchidā asesaṃ, bhisapupphaṃva saroruhaṃ vigayha,"},
		{2, 1, "2 so bhikkhu jahāti orapāraṃ, urago jiṇṇamivattacaṃ purāṇaṃ."},
	} {
		if _, err := db.Exec(`INSERT INTO sentences VALUES('EPI-y', ?, ?, ?)`, r.para, r.line, r.pali); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	src := &sentenceSource{db: db, mapping: map[string][]string{"mula_x_05": {"EPI-y"}}}
	cuts := src.cutsFor("mula_x_05", segs)
	got := corpus.SplitSegments(segs, cuts)
	if len(got) != 2 {
		t.Fatalf("got %d segments, want 2: %q", len(got), texts(got))
	}
	if got[0].Text != "2. yo rāgamudacchidā asesaṃ, bhisapupphaṃva saroruhaṃ vigayha," {
		t.Errorf("first line = %q", got[0].Text)
	}
	if got[1].Text != "so bhikkhu jahāti orapāraṃ, urago jiṇṇamivattacaṃ purāṇaṃ." {
		t.Errorf("second line = %q", got[1].Text)
	}
	// The number is on the first line, exactly once, and no piece is a number
	// on its own — which is what cutting at the row's own first word would give.
	if n := strings.Count(strings.Join(texts(got), ""), "2."); n != 1 {
		t.Errorf("the verse number appears %d times", n)
	}
	if rest := strings.TrimSpace(strings.TrimPrefix(got[0].Text, "2.")); rest == "" {
		t.Errorf("the first piece is nothing but the number: %q", got[0].Text)
	}
	if src.unplaced != 0 {
		t.Errorf("%d ePitaka lines were not placed", src.unplaced)
	}
}

// A verse the alignment never reaches is left whole: a boundary the alignment
// cannot vouch for is not worth inventing, and the reader loses nothing by
// reading the gāthā as one.
func TestCutsForLeavesAnUnreachedVerseWhole(t *testing.T) {
	verse := "cirassaṃ vata passāmi, brāhmaṇaṃ parinibbutaṃ;\nappatiṭṭhaṃ anāyūhaṃ, tiṇṇaṃ loke visattikan\"ti."
	segs := []corpus.Segment{{Seq: 1, Kind: corpus.KindVerse, Text: verse}}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE sentences(book_id TEXT, para_id INT, line_id INT, pali TEXT)`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sentences VALUES('EPI-z', 1, 1, 'na idaṃ tasseva vacanaṃ')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	src := &sentenceSource{db: db, mapping: map[string][]string{"mula_x_06": {"EPI-z"}}}
	got := corpus.SplitSegments(segs, src.cutsFor("mula_x_06", segs))
	if len(got) != 1 || got[0].Text != verse {
		t.Errorf("an unreached verse was split: %q", texts(got))
	}
}

// verseLines is the verse's own line structure: the byte after every break.
func TestVerseLines(t *testing.T) {
	text := "a b,\nc d,\n\ne f."
	got := verseLines(text)
	want := []int{5, 10, 11}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if got := verseLines("no breaks here"); got != nil {
		t.Errorf("a one-line verse produced cuts: %v", got)
	}
}

// The bracketed variant readings ePitaka writes inline are exactly what this
// reader has already cut out of the text. Counted as words of the line they
// make every walk past it step too far.
func TestDropVariantReadings(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"tadāssu nibbuyhāmi [nivuyhāmi (syā. kaṃ. ka.)].", "tadāssu nibbuyhāmi ."},
		{"a [x [y] z] b", "a  b"},
		{"katibhi [katīhi (sī.)] rajamādeti", "katibhi  rajamādeti"},
		{"no brackets here", "no brackets here"},
		// An unbalanced bracket is an editorial one CST keeps in the body text;
		// removing its content would make the two editions disagree the other
		// way round.
		{"[Kha) punapi tena", "[Kha) punapi tena"},
		{"araññavihārena.[[] etthantare pāṭho", "araññavihārena.[ etthantare pāṭho"},
	} {
		if got := dropVariantReadings(tc.in); got != tc.want {
			t.Errorf("dropVariantReadings(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A book with no ePitaka volume keeps its paragraphs whole rather than being
// split on a guess.
func TestCutsForWithoutAMappingLeavesEverythingAlone(t *testing.T) {
	src := &sentenceSource{mapping: map[string][]string{}}
	honest := &sentenceSource{} // no database either
	for _, s := range []*sentenceSource{src, honest, nil} {
		if got := s.cutsFor("mula_x_01", []corpus.Segment{{Kind: corpus.KindProse, Text: "a b c"}}); got != nil {
			t.Errorf("got cuts from a source with no mapping: %v", got)
		}
	}
}

// stripAll drops every byte of whitespace, which is how losslessness is
// compared: the split may move a space from one piece to the next, but it may
// not add or lose a letter.
func stripAll(s string) string {
	return strings.NewReplacer(" ", "", "\n", "", "\t", "", "\r", "").Replace(s)
}

func texts(segs []corpus.Segment) []string {
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		out = append(out, s.Text)
	}
	return out
}

// Cutting and translating are one mapping, and this is the property that makes
// them one: the sentence splitter aligns the book's words before the cut, the
// reference importer aligns them after it, and the two can only agree if the cut
// neither loses a word nor invents one. If it ever did, a segment boundary and
// the translation under it would come from two different alignments and nothing
// else would notice — the page would simply show the wrong Chinese.
func TestTheCutDoesNotChangeTheWordStream(t *testing.T) {
	prose := "1. evaṃ me sutaṃ – ekaṃ samayaṃ bhagavā sāvatthiyaṃ viharati jetavane anāthapiṇḍikassa ārāme. " +
		"atha kho aññatarā devatā abhikkantāya rattiyā yena bhagavā tenupasaṅkami. " +
		"upasaṅkamitvā bhagavantaṃ abhivādetvā ekamantaṃ aṭṭhāsi. idamavoca sā devatā."
	verse := "cirassaṃ vata passāmi, brāhmaṇaṃ parinibbutaṃ;\nappatiṭṭhaṃ anāyūhaṃ, tiṇṇaṃ loke visattikan\"ti."
	segs := []corpus.Segment{
		{Seq: 1, Kind: corpus.KindHeading, Text: "1. oghataraṇasuttaṃ"},
		{Seq: 2, ParaNo: 1, Kind: corpus.KindProse, Text: prose},
		{Seq: 3, ParaNo: 1, Kind: corpus.KindVerse, Text: verse},
	}
	// Cut wherever the sentences begin, which is all cutsFor does; which
	// offsets those are is beside the point of this test.
	cuts := make([][]int, len(segs))
	at := strings.Index(prose, "atha kho")
	cuts[1] = []int{at, strings.Index(prose, "upasaṅkamitvā"), strings.Index(prose, "idamavoca")}

	split := corpus.SplitSegments(segs, cuts)
	if len(split) <= len(segs) {
		t.Fatalf("the fixture did not split: %d segments", len(split))
	}
	var before []string // the one word stream the aligner is given
	for _, s := range segs {
		before = append(before, alignWords(s.Text)...)
	}
	var after []string
	for _, s := range split {
		after = append(after, alignWords(s.Text)...)
	}
	if strings.Join(before, " ") != strings.Join(after, " ") {
		t.Errorf("the cut changed the word stream:\n before %q\n after  %q",
			strings.Join(before, " "), strings.Join(after, " "))
	}
}
