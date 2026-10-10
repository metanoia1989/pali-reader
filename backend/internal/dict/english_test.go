package dict

import (
	"strings"
	"testing"
)

// resolve is what the lookup does with a candidate list: the first form that is
// a headword wins. Testing the candidates that way rather than as a fixed
// sequence pins the thing that matters — which entry the reader gets — and not
// the order of the attempts that miss.
func resolve(cands []string, lexicon map[string]bool) string {
	for _, c := range cands {
		if lexicon[c] {
			return c
		}
	}
	return ""
}

func lex(words ...string) map[string]bool {
	m := map[string]bool{}
	for _, w := range words {
		m[w] = true
	}
	return m
}

// Order is the correctness argument, so it is the thing worth testing.
//
// A rule that removes two letters before one that removes a single letter
// answers a tap on "uses" with the entry for "us", and a tap on "used" with the
// same — both of which are real headwords, and neither of which is the word the
// reader touched. Where a lexicon below holds the wrong answer as well as the
// right one, the test fails if the order slips. It does not hold the
// intermediate forms that are not words ("stoppe", "bigg"): no ordering can
// know those are wrong, and none has to — the dictionary decides, and the
// dictionary does not have them.
func TestEnglishLookupPrefersTheRealHeadword(t *testing.T) {
	cases := []struct {
		tap     string
		lexicon map[string]bool
		want    string
		why     string
	}{
		{"said", lex("said", "say"), "said", "an exact entry wins outright"},
		{"dwelling", lex("dwelling", "dwell"), "dwelling", "an exact entry wins outright"},
		{"monks", lex("monk"), "monk", "the plural"},
		{"uses", lex("us", "use"), "use", "use+s, not us+es"},
		{"used", lex("us", "use"), "use", "use+d, not us+ed"},
		{"added", lex("add", "ad"), "add", "add+ed; the doubled consonant is not stripped here"},
		{"stopped", lex("stop"), "stop", "a doubled final consonant"},
		{"houses", lex("house"), "house", "the e belongs to the stem"},
		{"boxes", lex("box"), "box", "the -es that is a syllable"},
		{"making", lex("make"), "make", "the silent e"},
		{"running", lex("run"), "run", "the doubled consonant"},
		{"studied", lex("study"), "study", "-ied is -y"},
		{"happily", lex("happy"), "happy", "-ily is -y"},
		{"bigger", lex("big"), "big", "the comparative"},
		{"quickly", lex("quick"), "quick", "the adverb"},
		{"isn't", lex("is"), "is", "a contraction"},
		{"don't", lex("do"), "do", "a contraction"},
		{"aren't", lex("be", "are"), "are", "the contraction's own base is tried first"},
		{"are", lex("be"), "be", "irregular: ECDICT has no entry for are"},
		{"were", lex("be"), "be", "irregular: ECDICT has no entry for were"},
		{"well-known", lex("well", "known"), "well", "the hyphen holds two words"},
		{"mind-produced", lex("mind", "produced"), "mind", "the first half wins when both are words"},
		{"kamma-produced", lex("produced"), "produced", "the second half answers when the first is not a word"},
		{"Bhikkhus", lex("bhikkhu"), "bhikkhu", "case and punctuation are not the word"},
	}

	for _, c := range cases {
		got := resolve(EnglishCandidates(c.tap), c.lexicon)
		if got != c.want {
			t.Errorf("tap %q resolved to %q, want %q — %s", c.tap, got, c.want, c.why)
		}
	}
}

// The Pāḷi the English translations keep. The rules may strip a plural and stop
// there — "bhikkhus" → "bhikkhu" — and whether that lands on an entry is the
// dictionary's business. ECDICT has none of these, so the popup says there is
// no entry, which is the honest answer; what must not happen is a rule bending
// one word onto an unrelated one.
func TestEnglishCandidatesForLoanwordsOnlyStripEndings(t *testing.T) {
	cases := map[string][]string{
		"jhāna":    {"jhāna"},
		"nibbāna":  {"nibbāna"},
		"bhikkhus": {"bhikkhus", "bhikkhu"},
		"suttas":   {"suttas", "sutta"},
	}
	for in, want := range cases {
		got := EnglishCandidates(in)
		if len(got) != len(want) {
			t.Errorf("EnglishCandidates(%q) = %v, want %v", in, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("EnglishCandidates(%q) = %v, want %v", in, got, want)
				break
			}
		}
	}
}

// A two-letter fragment of a compound is not a word to answer with.
//
// "co-nascence" is sahajāta, a term of the Abhidhamma; ECDICT has an entry for
// "Co" — the chemical symbol for cobalt — and answering the reader's tap on the
// first with the second is worse than answering nothing. Found by reading the
// rule-resolved words out of TestEnglishCoverage, not by reasoning about it.
func TestEnglishTwoLetterCompoundHalvesAreNotWords(t *testing.T) {
	if got := resolve(EnglishCandidates("co-nascence"), lex("co")); got != "" {
		t.Errorf("co-nascence resolved to %q; a two-letter prefix is not a word", got)
	}
	if got := resolve(EnglishCandidates("non-root"), lex("non", "root")); got != "non" {
		t.Errorf("non-root resolved to %q, want non", got)
	}
	// The apostrophe split is not affected: a contraction's base is a word.
	if got := resolve(EnglishCandidates("isn't"), lex("is")); got != "is" {
		t.Errorf("isn't resolved to %q, want is", got)
	}
}

func TestEnglishCandidatesAreBoundedAndUnique(t *testing.T) {
	for _, w := range []string{
		"antidisestablishmentarianism", "dwelling", "unabandoned", "isn't",
		"well-known", "houses", "stopped", "studied", "monks", "sāvatthī",
	} {
		got := EnglishCandidates(w)
		if len(got) > 16 {
			t.Errorf("EnglishCandidates(%q) returned %d forms: %v", w, len(got), got)
		}
		seen := map[string]bool{}
		for _, c := range got {
			if seen[c] {
				t.Errorf("EnglishCandidates(%q) repeats %q: %v", w, c, got)
			}
			seen[c] = true
			if len(c) > 64 {
				t.Errorf("EnglishCandidates(%q) returns %q, longer than the column", w, c)
			}
		}
		if len(got) > 0 && got[0] != NormalizeEnglishWord(w) {
			t.Errorf("EnglishCandidates(%q) does not start with the word itself: %v", w, got)
		}
	}
}

func TestNormalizeEnglishWord(t *testing.T) {
	cases := map[string]string{
		"The":         "the",
		"Dwelling.":   "dwelling",
		"“Monks,”":    "monks",
		"bhikkhu’s":   "bhikkhu",
		"  Thus ":     "thus",
		"—":           "",
		"sāvatthī":    "sāvatthī",
		"well-known?": "well-known",
	}
	for in, want := range cases {
		if got := NormalizeEnglishWord(in); got != want {
			t.Errorf("NormalizeEnglishWord(%q) = %q, want %q", in, got, want)
		}
	}
	if got := EnglishCandidates("—"); len(got) != 0 {
		t.Errorf("EnglishCandidates(\"—\") = %v, want nothing", got)
	}
	if got := EnglishCandidates(""); len(got) != 0 {
		t.Errorf("EnglishCandidates(\"\") = %v, want nothing", got)
	}
}

// Every candidate is lower case: the column is binary collated, so a candidate
// that kept its capital would be a query that cannot match a row.
func TestEnglishCandidatesAreLowerCased(t *testing.T) {
	for _, c := range EnglishCandidates("Dwelling") {
		if c != strings.ToLower(c) {
			t.Errorf("candidate %q is not folded", c)
		}
	}
}
