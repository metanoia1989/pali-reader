package dict

import (
	"strings"
	"testing"

	"github.com/metanoia/pali-reader/backend/internal/store"
)

// DPD's grids hold *endings*, not whole words, and its `stem` column is the
// bare stem without a connecting vowel — "buddh", not "buddha". Stem plus
// ending is the form as it appears in the text, which is what lets the panel
// draw "buddh" in body weight and "assa" in ink blue.
const nounTemplate = `[
 [[""],["masc sg"],[""],["masc pl"],[""]],
 [["nom"],["o"],["masc nom sg"],["ā","āse"],["masc nom pl"],[""]],
 [["acc"],["aṃ"],["masc acc sg"],["e"],["masc acc pl"],[""]],
 [["instr"],["ena","ā"],["masc instr sg"],["ehi"],["masc instr pl"],[""]],
 [["dat"],["assa"],["masc dat sg"],["ānaṃ"],["masc dat pl"],[""]],
 [["abl"],["ato"],["masc abl sg"],["ehi"],["masc abl pl"],[""]],
 [["gen"],["assa"],["masc gen sg"],["ānaṃ"],["masc gen pl"],[""]],
 [["loc"],["asmiṃ"],["masc loc sg"],["esu"],["masc loc pl"],[""]],
 [["voc"],["a"],["masc voc sg"],["ā"],["masc voc pl"],[""]]
]`

// An adjective grid has six columns rather than two; the same reader has to
// handle both without the header drifting out of step with the body.
const adjTemplate = `[
 [[""],["masc sg"],[""],["nt sg"],[""],["fem sg"],[""]],
 [["nom"],["o"],["masc nom sg"],["aṃ"],["nt nom sg"],["ā"],["fem nom sg"],[""]]
]`

func TestRenderDeclensionColumnsLineUpWithRows(t *testing.T) {
	d, err := RenderDeclension(store.DictTemplate{Pattern: "a masc", Like: "buddha", Data: nounTemplate}, "buddh", "buddhassa")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(d.Columns, "|") != "masc sg|masc pl" {
		t.Fatalf("columns = %v", d.Columns)
	}
	if len(d.Rows) != 8 {
		t.Fatalf("got %d rows, want 8", len(d.Rows))
	}
	for _, r := range d.Rows {
		if len(r.Cells) != 2 {
			t.Fatalf("row %s has %d cells, want 2", r.Case, len(r.Cells))
		}
	}
}

func TestRenderDeclensionFindsTheClickedForm(t *testing.T) {
	d, _ := RenderDeclension(store.DictTemplate{Pattern: "a masc", Data: nounTemplate}, "buddh", "buddhassa")
	if d.Hit == nil {
		t.Fatal("no cell matched buddhassa")
	}
	// buddhassa is both the dative and the genitive singular; the first row
	// that contains it wins, which is what the panel highlights.
	if d.Rows[d.Hit.Row].Case != "dat" {
		t.Fatalf("matched row %q, want dat", d.Rows[d.Hit.Row].Case)
	}
	if d.Hit.Column != 0 {
		t.Fatalf("matched column %d, want 0", d.Hit.Column)
	}
	if d.Hit.Form != "buddhassa" {
		t.Fatalf("matched form %q", d.Hit.Form)
	}
	// Cells hold the ending, not the whole word, so the client can draw the
	// stem in body weight and the ending in ink blue.
	if d.Hit.Suffix != "assa" {
		t.Fatalf("matched suffix %q, want assa", d.Hit.Suffix)
	}
	if d.Stem != "buddh" {
		t.Fatalf("stem = %q", d.Stem)
	}
}

func TestRenderDeclensionHandlesSixColumnGrids(t *testing.T) {
	d, err := RenderDeclension(store.DictTemplate{Pattern: "a adj", Data: adjTemplate}, "akakkas", "akakkasā")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Columns) != 3 {
		t.Fatalf("columns = %v, want 3", d.Columns)
	}
	if len(d.Rows[0].Cells) != 3 {
		t.Fatalf("row has %d cells, want 3", len(d.Rows[0].Cells))
	}
	if d.Hit == nil || d.Hit.Column != 2 {
		t.Fatalf("hit = %+v, want column 2", d.Hit)
	}
}

// The grid is the stem plus an ending, so an ending that merely looks like the
// word must not match: "assa" is a dative ending, but "buddha" + "assa" is only
// the dative of "buddha".
func TestRenderDeclensionMatchesStemPlusEndingNotEndingAlone(t *testing.T) {
	d, _ := RenderDeclension(store.DictTemplate{Pattern: "a masc", Data: nounTemplate}, "dhamm", "buddhassa")
	if d.Hit != nil {
		t.Fatalf("buddhassa should not match the dhamma paradigm, got %+v", d.Hit)
	}
	d2, _ := RenderDeclension(store.DictTemplate{Pattern: "a masc", Data: nounTemplate}, "buddh", "buddhānaṃ")
	if d2.Hit == nil || d2.Hit.Column != 1 {
		t.Fatalf("buddhānaṃ should match the plural dative, got %+v", d2.Hit)
	}
}

func TestRenderDeclensionWithoutAMatch(t *testing.T) {
	d, _ := RenderDeclension(store.DictTemplate{Pattern: "a masc", Data: nounTemplate}, "buddh", "gacchati")
	if d.Hit != nil {
		t.Fatalf("an unrelated form should not match, got %+v", d.Hit)
	}
}

func TestRenderDeclensionRejectsBrokenData(t *testing.T) {
	if _, err := RenderDeclension(store.DictTemplate{Pattern: "x", Data: "not json"}, "", ""); err == nil {
		t.Fatal("malformed template data should be an error, not a silent empty table")
	}
	if d, err := RenderDeclension(store.DictTemplate{Pattern: "x", Data: "[]"}, "", ""); err != nil || d != nil {
		t.Fatalf("an empty template should render nothing, got %v / %v", d, err)
	}
}

func TestCleanLemmaAndHomonym(t *testing.T) {
	cases := []struct{ in, lemma, hom string }{
		{"buddha 1", "buddha", "1"},
		{"akaṅkha 2.1", "akaṅkha", "2.1"},
		{"buddha", "buddha", ""},
		{"a 1.1", "a", "1.1"},
		{"dhammacakka", "dhammacakka", ""},
	}
	for _, c := range cases {
		if got := CleanLemma(c.in); got != c.lemma {
			t.Errorf("CleanLemma(%q) = %q, want %q", c.in, got, c.lemma)
		}
		if got := Homonym(c.in); got != c.hom {
			t.Errorf("Homonym(%q) = %q, want %q", c.in, got, c.hom)
		}
	}
}

func TestDecodeLookupLists(t *testing.T) {
	// DPD writes these as JSON; older exports wrote a bare list. Both have to
	// survive, because a lookup that silently loses its headwords shows the
	// reader an empty panel.
	if got := decodeInts(`[49139, 49140]`); len(got) != 2 || got[0] != 49139 {
		t.Errorf("decodeInts(json) = %v", got)
	}
	if got := decodeInts(`49139, 49140`); len(got) != 2 {
		t.Errorf("decodeInts(bare) = %v", got)
	}
	if got := decodeInts(""); got != nil {
		t.Errorf("decodeInts(empty) = %v", got)
	}
	if got := decodeStrings(`["a + b","c + d"]`); len(got) != 2 {
		t.Errorf("decodeStrings = %v", got)
	}
	a := decodeAnalyses(`[{"pos":"noun","grammar":"masc nom sg","lemma":"buddha"}]`)
	if len(a) != 1 || a[0].Grammar != "masc nom sg" || a[0].Lemma != "buddha" {
		t.Errorf("decodeAnalyses = %+v", a)
	}
	if got := decodeAnalyses("not json"); got != nil {
		t.Errorf("decodeAnalyses should ignore malformed data, got %v", got)
	}
}
