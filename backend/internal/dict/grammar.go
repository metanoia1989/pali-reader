package dict

import "strings"

// Splitting DPD's grammar strings into the columns the lookup panel shows.
//
// DPD writes one string per reading, in two shapes:
//
//	nominal   [gender] case number      "masc loc sg", "nt dat pl"
//	verbal    [qualifier] tense person number   "aor 3rd pl", "reflx pr 3rd pl"
//
// and sometimes with neither, when the form is irregular ("esi aor").
// The panel wants discrete columns, so the string is taken apart here rather
// than in the client: this is grammar, and getting it wrong silently tells the
// reader the wrong case.

var genders = map[string]string{
	"masc": "masc", "fem": "fem", "nt": "nt", "neut": "nt", "adj": "",
}

// numberWords are the endings that mark the final column. "3rd" is a person,
// not a number, so it is not in this set.
var numberWords = map[string]string{
	"sg": "sg", "pl": "pl", "du": "du",
}

// persons are the verbal subjects. When one is present the number column shows
// the person together with the number ("3rd pl") because that is how a verb
// form is identified.
var persons = map[string]string{"1st": "1st", "2nd": "2nd", "3rd": "3rd"}

var cases = map[string]string{
	"nom": "nom", "acc": "acc", "instr": "instr", "dat": "dat",
	"abl": "abl", "gen": "gen", "loc": "loc", "voc": "voc",
}

// ParseGrammarParts splits one reading into its columns. Unknown tokens are
// preserved in Qualifier rather than dropped, so nothing is silently lost.
func ParseGrammarParts(grammar string) (gender, grammaticalCase, number, qualifier string) {
	tokens := strings.Fields(grammar)
	if len(tokens) == 0 {
		return "", "", "", ""
	}

	// The number is the last token when it is one, and the person or case sits
	// immediately before it.
	if n, ok := numberWords[tokens[len(tokens)-1]]; ok {
		number = n
		rest := tokens[:len(tokens)-1]
		if len(rest) > 0 {
			last := rest[len(rest)-1]
			if c, ok := cases[last]; ok {
				grammaticalCase = c
			} else if p, ok := persons[last]; ok {
				grammaticalCase = "" // a verb has no case
				number = p + " " + n
			} else {
				grammaticalCase = last
			}
			rest = rest[:len(rest)-1]
		}
		tokens = rest
	}

	var rest []string
	for _, t := range tokens {
		if g, ok := genders[t]; ok {
			if g != "" && gender == "" {
				gender = g
				continue
			}
			if g == "" {
				// "adj" is a part of speech, not a gender; it stays a qualifier.
				continue
			}
		}
		rest = append(rest, t)
	}
	return gender, grammaticalCase, number, strings.Join(rest, " ")
}

// posLabel renders a DPD part of speech the way the panel groups them: the
// gender-only tags of a noun collapse to "noun", and the tense tags of a verb
// to "verb", because "pr" and "aor" are not what the reader is sorting by at
// this point — they are looking for "which word could this be".
func posLabel(pos string) string {
	switch pos {
	case "masc", "fem", "nt", "neut":
		return "noun"
	case "pr", "aor", "fut", "perf", "cond", "opt", "imp", "imperf",
		"pp", "prp", "ptp", "ger", "abs", "inf", "cs", "ve":
		return "verb"
	case "card", "ordin":
		return "numeral"
	}
	return pos
}
