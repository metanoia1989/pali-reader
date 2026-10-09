package importer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/metanoia/pali-reader/backend/internal/tokenize"
)

// The stored tokenisation has one consumer that matters: internal/server, which
// decodes it into [][3]int and hands the reader offset/length pairs to slice the
// segment text with. An earlier version wrote one JSON object per token, which
// decoded into nothing at all — and because the error was swallowed, every word
// in the reader rendered as an empty span. This test pins the wire shape.
func TestEncodeTokensRoundTripsIntoIntTriples(t *testing.T) {
	text := "evaṃ me sutaṃ – ekaṃ samayaṃ"
	toks := tokenize.Split(text)
	if len(toks) != 5 {
		t.Fatalf("expected 5 tokens, got %d", len(toks))
	}

	encoded := encodeTokens(text, toks)

	var decoded [][3]int
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("server cannot decode %s: %v", encoded, err)
	}
	if len(decoded) != len(toks) {
		t.Fatalf("decoded %d tokens, want %d", len(decoded), len(toks))
	}
	// The reader slices a JavaScript string, so the offsets must be UTF-16 code
	// units. Slicing the Go string with them would be wrong for every token
	// after the first diacritic, which is how the underlines came to sit on the
	// wrong words.
	units := []rune(text)
	for i, d := range decoded {
		off, length := d[0], d[1]
		if length <= 0 {
			t.Errorf("token %d has length %d", i, length)
			continue
		}
		if off+length > len(units) {
			t.Errorf("token %d runs past the text", i)
			continue
		}
		if got := string(units[off : off+length]); tokenize.Normalize(got) != toks[i].Key {
			t.Errorf("token %d slices to %q, want key %q", i, got, toks[i].Key)
		}
	}
}

// Every Pāḷi diacritic outside Latin-1 is wider in bytes than in UTF-16 units,
// so a byte-offset encoding drifts a little further out of step per accented
// letter. This is the case that made it obvious: the word after "evaṃ" was
// underlined as "evaṃ m".
func TestEncodeTokensCountsUTF16UnitsNotBytes(t *testing.T) {
	text := "evaṃ me sutaṃ"
	toks := tokenize.Split(text)
	var decoded [][3]int
	if err := json.Unmarshal([]byte(encodeTokens(text, toks)), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded[0][0] != 0 || decoded[0][1] != 4 {
		t.Errorf("evaṃ should be [0,4] in UTF-16 units, got %v", decoded[0])
	}
	if decoded[1][0] != 5 || decoded[1][1] != 2 {
		t.Errorf("me should be [5,2], got %v", decoded[1])
	}
	units := []rune(text)
	for i, d := range decoded {
		if got := string(units[d[0] : d[0]+d[1]]); got != toks[i].Key {
			t.Errorf("token %d = %q, want %q", i, got, toks[i].Key)
		}
	}
}

func TestEncodeTokensKeepsTheKnownFlag(t *testing.T) {
	text := "buddho kho dhammaṃ"
	toks := tokenize.Split(text)
	tokenize.Mark(toks, map[string]struct{}{"buddho": {}, "dhammaṃ": {}})

	var decoded [][3]int
	if err := json.Unmarshal([]byte(encodeTokens(text, toks)), &decoded); err != nil {
		t.Fatal(err)
	}
	want := []int{1, 0, 1}
	for i := range want {
		if decoded[i][2]&tokenize.FlagKnown != want[i] {
			t.Errorf("token %d known flag = %d, want %d", i, decoded[i][2], want[i])
		}
	}
}

func TestEncodeTokensRejectsAnEmptyRun(t *testing.T) {
	if got := encodeTokens("", nil); got != "[]" {
		t.Errorf("encodeTokens(empty) = %q, want []", got)
	}
}

// Chinese titles are composed from a term dictionary. The Pāḷi elides a final
// -a before a vowel, so "sīlakkhandhavagga" + "aṭṭhakathā" is written
// "sīlakkhandhavaggaṭṭhakathā" with one a. Matching the terms literally left
// "戒蕴品ṭhakathā" — half translated.
func TestChineseNameMatchesAcrossElision(t *testing.T) {
	cases := map[string]string{
		"sīlakkhandhavaggapāḷi":      "戒蕴品",
		"sīlakkhandhavaggaṭṭhakathā": "戒蕴品义注",
		"dhammasaṅgaṇīpāḷi":          "法聚论",
		"dhammasaṅgaṇīaṭṭhakathā":    "法聚论义注",
		"jātakaaṭṭhakathā (pa)":      "本生义注（第一册）",
		"visuddhimaggo (pa)":         "清净道论（第一册）",
		// A multi-word title: the space between the words has to be consumed,
		// or the first word matches and the rest is left in Pāḷi.
		"sagāthāvaggasaṃyuttapāḷi":          "有偈品相应",
		"vinayavinicchayo uttaravinicchayo": "律决定·后决定",
		"vuttodayaṃ":                        "韵律",
		"khuddasikkhā mūlasikkhā":           "小戒·根本戒",
		// Consonant doubling across a join, and a bracketed part name.
		"saddanītippakaraṇaṃ (padamālā)": "善语法论书（词鬘）",
		"moggallānapañcikāṭīkā":          "目犍连文法注复注",
	}
	for in, want := range cases {
		if got := ChineseName(in); got != want {
			t.Errorf("ChineseName(%q) = %q, want %q", in, got, want)
		}
	}
}

// DPD writes a reading with no case or number as `<td colspan='3'>indeclineable`.
// A parser that insists on a bare `<td>` skips the row, then matches across the
// row boundary and hands the panel a piece of the table's own markup.
func TestParseGrammarHandlesCellsWithAttributes(t *testing.T) {
	html := `<div class='dpd_grammar'><table class='dpd_grammar'>` +
		`<tr><td><b>letter</b></td><td colspan='3'>indeclineable</td></tr>` +
		`<tr><td><b>adj</b></td><td>masc nom sg</td><td>of</td><td>akakkasa</td></tr>` +
		`</table></div>`
	got := ParseGrammar(html)
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(got), got)
	}
	if got[0].POS != "letter" || got[0].Grammar != "indeclineable" || got[0].Lemma != "" {
		t.Errorf("row 0 = %+v", got[0])
	}
	if got[1].POS != "adj" || got[1].Grammar != "masc nom sg" || got[1].Lemma != "akakkasa" {
		t.Errorf("row 1 = %+v", got[1])
	}
	// Nothing may carry markup through to the panel.
	for _, a := range got {
		for _, field := range []string{a.POS, a.Grammar, a.Lemma} {
			if strings.ContainsAny(field, "<>") {
				t.Errorf("markup leaked into %q", field)
			}
		}
	}
}
