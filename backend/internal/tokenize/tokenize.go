// Package tokenize splits Pāḷi running text into word tokens.
//
// The same function runs at import time (to freeze the token offsets that user
// annotations anchor to) and, for safety, at request time. Both must agree, so
// there is exactly one implementation and it is deterministic.
package tokenize

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Flags carried by a token in the cached token array.
const (
	// FlagKnown marks a token whose normalised form has a dictionary entry.
	FlagKnown = 1
	// FlagCapital marks a token that was capitalised in the source, so the
	// reader can tell a proper noun from a common one without re-reading text.
	FlagCapital = 2
)

// Token is one word occurrence, addressed by byte offset into the source text.
type Token struct {
	Off   int    `json:"o"`
	Len   int    `json:"l"`
	Flags int    `json:"f"`
	Key   string `json:"-"` // normalised lookup key, not serialised
}

// Table is a text with its tokenisation.
type Table struct {
	Text   string  `json:"text"`
	Tokens []Token `json:"tokens"`
}

// isWordRune reports whether r can appear inside a Pāḷi word.
func isWordRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsMark(r) {
		return true
	}
	switch r {
	// The apostrophe is a genuine word character in Roman Pāḷi: it marks
	// elision, as in "buddhassā'ti" or "snāti'ti". Both the typographic and
	// the ASCII form occur in the CST text.
	case '\'', '\u2019', '\u02BC':
		return true
	// A hyphen joins the parts of some indeclinables and of the
	// "evarūpāya-tiracchānavijjāya" style compounds the commentaries use.
	case '-', '\u2010', '\u2011':
		return true
	}
	return false
}

// Split walks text and returns every maximal run of word characters.
func Split(text string) []Token {
	var out []Token
	i := 0
	for i < len(text) {
		r, size := utf8.DecodeRuneInString(text[i:])
		if !isWordRune(r) {
			i += size
			continue
		}
		start := i
		for i < len(text) {
			r, size = utf8.DecodeRuneInString(text[i:])
			if !isWordRune(r) {
				break
			}
			i += size
		}
		raw := text[start:i]
		key, lead, trail := trimEdges(raw)
		if key == "" {
			// The run was nothing but punctuation, e.g. a lone dash.
			continue
		}
		// A run of digits is not a word. The canon numbers its own paragraphs
		// in the running text, and a "28" that looks tappable and answers
		// "the dictionary has nothing for this" is worse than not tappable.
		if !hasLetter(key) {
			continue
		}
		flags := 0
		// Capitalisation is read from the visible form, not from the key: the
		// key is folded to lower case, so asking it would always answer no.
		if first, _ := utf8.DecodeRuneInString(raw[lead:]); unicode.IsUpper(first) {
			flags |= FlagCapital
		}
		out = append(out, Token{
			Off:   start + lead,
			Len:   len(raw) - lead - trail,
			Flags: flags,
			Key:   key,
		})
	}
	return out
}

// SplitBytes is Split for a byte slice.
func SplitBytes(b []byte) []Token { return Split(string(b)) }

// trimEdges removes leading and trailing characters that are word characters
// in isolation but never part of a headword. It returns the normalised key and
// how many bytes were removed from each side, so the caller can keep the token
// pointing at the visible form.
func trimEdges(raw string) (key string, lead, trail int) {
	const cut = "'\u2019\u02BC-\u2010\u2011"
	s := raw
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if !strings.ContainsRune(cut, r) {
			break
		}
		s = s[size:]
		lead += size
	}
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if !strings.ContainsRune(cut, r) {
			break
		}
		s = s[:len(s)-size]
		trail += size
	}
	return Normalize(s), lead, trail
}

// hasLetter reports whether s contains anything other than digits.
func hasLetter(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// Normalize produces the dictionary key for a surface form.
//
// It lower-cases, and removes apostrophes and hyphens from the two ends. Those
// characters are word characters inside a Pāḷi word ("buddhassā'ti",
// "evarūpāya-tiracchānavijjāya") but punctuation at its edge ("…sannisinnā"ti"),
// and the dictionary is keyed by the word without them. This is the single
// definition of "the key for this form" — the tokeniser, the importer and the
// query path all use it, so a form resolved one way is resolved that way
// everywhere.
func Normalize(s string) string {
	if s == "" {
		return ""
	}
	const cut = "'\u2019\u02BC-\u2010\u2011"
	s = strings.Trim(s, cut)
	if s == "" {
		return ""
	}
	ascii := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= utf8.RuneSelf || (c >= 'A' && c <= 'Z') {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	return strings.ToLower(s)
}

// Keys returns the distinct normalised keys in a tokenisation, in first-seen
// order. The importer uses it to resolve a whole segment in one query.
func Keys(tokens []Token) []string {
	seen := make(map[string]struct{}, len(tokens))
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if t.Key == "" {
			continue
		}
		if _, ok := seen[t.Key]; ok {
			continue
		}
		seen[t.Key] = struct{}{}
		out = append(out, t.Key)
	}
	return out
}

// Mark sets the known flag on tokens whose key is in known.
func Mark(tokens []Token, known map[string]struct{}) {
	for i := range tokens {
		if _, ok := known[tokens[i].Key]; ok {
			tokens[i].Flags |= FlagKnown
		}
	}
}
