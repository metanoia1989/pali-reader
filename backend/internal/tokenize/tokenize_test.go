package tokenize

import (
	"strings"
	"testing"
)

// offsetsOf maps each token back to the text it covers, which is what the
// reader slices on. If this is wrong the wrong word gets looked up.
func offsetsOf(text string) []string {
	var out []string
	for _, t := range Split(text) {
		out = append(out, text[t.Off:t.Off+t.Len])
	}
	return out
}

func TestSplitKeepsOffsetsOnTheVisibleForm(t *testing.T) {
	text := "evaṃ me sutaṃ – ekaṃ samayaṃ bhagavā"
	got := strings.Join(offsetsOf(text), "|")
	want := "evaṃ|me|sutaṃ|ekaṃ|samayaṃ|bhagavā"
	if got != want {
		t.Fatalf("tokens = %q, want %q", got, want)
	}
}

func TestSplitHandlesElisionAndHyphen(t *testing.T) {
	// The apostrophe is a word character in Roman Pāḷi, but one at the edge of
	// a run is punctuation. Both forms occur in the canon.
	cases := []struct {
		text  string
		words []string
	}{
		{"buddhassā'ti", []string{"buddhassā'ti"}},
		{"atthasamhita'nti", []string{"atthasamhita'nti"}},
		{"evarūpāya-tiracchānavijjāya", []string{"evarūpāya-tiracchānavijjāya"}},
		{"“kāyanuttha, bhikkhave, etarahi kathāya sannisinnā”ti?", []string{"kāyanuttha", "bhikkhave", "etarahi", "kathāya", "sannisinnā", "ti"}},
		{"– iti vā –", []string{"iti", "vā"}},
	}
	for _, c := range cases {
		got := offsetsOf(c.text)
		if strings.Join(got, "|") != strings.Join(c.words, "|") {
			t.Errorf("Split(%q) = %q, want %q", c.text, got, c.words)
		}
	}
}

// Digits are not words. The canon numbers its paragraphs inline, and a "28"
// that looks tappable but answers nothing is worse than plain text.
func TestSplitSkipsRunsOfDigits(t *testing.T) {
	got := offsetsOf("28. atha kho bhagavā 3 ca")
	want := "atha|kho|bhagavā|ca"
	if strings.Join(got, "|") != want {
		t.Fatalf("tokens = %q, want %q", got, want)
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Buddho":       "buddho",
		"Dhammaṃ":      "dhammaṃ",
		"'ti":          "ti",
		"buddhassā'ti": "buddhassā'ti",
		"evaṃ":         "evaṃ",
		"":             "",
		"SĀDHUNĀ":      "sādhunā",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKeysAreDistinctAndOrdered(t *testing.T) {
	text := "buddho buddho dhammaṃ BUDDHO gacchati"
	got := Keys(Split(text))
	want := []string{"buddho", "dhammaṃ", "gacchati"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Keys = %q, want %q", got, want)
	}
}

func TestMarkSetsTheKnownFlag(t *testing.T) {
	toks := Split("buddho kho dhammaṃ")
	Mark(toks, map[string]struct{}{"buddho": {}, "dhammaṃ": {}})
	flags := make([]int, len(toks))
	for i, tk := range toks {
		flags[i] = tk.Flags & FlagKnown
	}
	want := []int{FlagKnown, 0, FlagKnown}
	for i := range want {
		if flags[i] != want[i] {
			t.Fatalf("flags = %v, want %v", flags, want)
		}
	}
	// Capitalisation is recorded separately so the reader can tell a proper
	// noun from a common one without re-reading the text.
	cap := Split("Buddho kho")
	if cap[0].Flags&FlagCapital == 0 {
		t.Error("a capitalised token should carry FlagCapital")
	}
	if cap[1].Flags&FlagCapital != 0 {
		t.Error("a lowercase token should not carry FlagCapital")
	}
}

func TestSplitIsStableForTheSameInput(t *testing.T) {
	text := "tena kho pana samayena suppiyo paribbājako addhānamaggappaṭipanno hoti"
	a := offsetsOf(text)
	for i := 0; i < 5; i++ {
		if got := offsetsOf(text); strings.Join(got, "|") != strings.Join(a, "|") {
			t.Fatalf("tokenisation is not deterministic: %q vs %q", got, a)
		}
	}
}
