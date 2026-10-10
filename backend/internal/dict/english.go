package dict

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/metanoia/pali-reader/backend/internal/store"
)

// The English dictionary is a second, separate dictionary: it answers for the
// words in the English 参考译文, it shares nothing with the Pāḷi lookup, and it
// is consulted by the reader tapping a word rather than by the panel. It is
// deliberately not merged into dict_headwords — a Pāḷi form and an English word
// that happen to be spelled alike are different questions, and the English
// entry (a Chinese gloss of an English word) would be nonsense as an answer to
// the Pāḷi one.
//
// Resolution is layered for the same reason it is on the Pāḷi side: the word on
// the page is a surface form, and the dictionary holds headwords. "dwelling"
// and "said" are entries, but "monks" is only the plural of one, "isn't" is two
// words written as one, and "well-known" is two headwords with a hyphen. A
// dictionary that only matched exact strings would answer a fraction of a real
// page. Measured on 300,000 sentences of this project's English reference
// corpus (TestEnglishCoverage, 5,736,328 word occurrences): matching the
// headword exactly resolves 91.1%, and the layered lookup below resolves 94.2%.
// What is left is the Pāḷi the translations keep
// verbatim (kamma, dhamma, jhāna, nibbāna, bhikkhu, arahant, sutta, saṅgha) —
// there is no English entry for those and there must not be a fabricated one.
// The Pāḷi is one line up in the reading column, clickable, with DPD behind it.

// EnSense is one meaning line. The popup draws one row per line.
type EnSense struct {
	Pos string `json:"pos"`
	Def string `json:"def"`
}

// EnResult is what the English popup draws.
type EnResult struct {
	// Query is the surface form that was clicked.
	Query string `json:"query"`
	// Word is the entry's own spelling, which is what the popup prints. Empty
	// when nothing was found.
	Word string `json:"word"`
	// Phonetic is ECDICT's phonetic transcription, when it has one.
	Phonetic string `json:"phonetic,omitempty"`
	// Via is the candidate form that matched, when it is not the word itself.
	// It is what lets the popup say "uses → use" instead of quietly answering a
	// question the reader did not ask.
	Via   string `json:"via,omitempty"`
	Found bool   `json:"found"`
	// Available is false when the dictionary holds nothing at all — the
	// importer has not been run on this installation. The reader is told that
	// rather than being told the word has no entry, which would be a lie.
	Available bool      `json:"available"`
	Senses    []EnSense `json:"senses"`
}

// englishContractions maps a contraction to the word it contracts.
//
// Carried across from the reference project unchanged: the map is a fact about
// English rather than about either program, and "don't" is a word a reader will
// certainly tap.
var englishContractions = map[string]string{
	"don't": "do", "doesn't": "does", "didn't": "do",
	"can't": "can", "cannot": "can", "won't": "will",
	"wouldn't": "would", "couldn't": "could", "shouldn't": "should",
	"isn't": "is", "aren't": "are", "wasn't": "was", "weren't": "were",
	"hasn't": "has", "haven't": "have", "hadn't": "had",
	"i'm": "i", "i've": "i", "i'd": "i", "i'll": "i",
	"you're": "you", "you've": "you", "you'd": "you", "you'll": "you",
	"he's": "he", "she's": "she", "it's": "it",
	"we're": "we", "we've": "we", "we'd": "we", "we'll": "we",
	"they're": "they", "they've": "they", "they'd": "they", "they'll": "they",
	"that's": "that", "there's": "there", "what's": "what",
}

// englishIrregulars are the few forms no rule recovers.
//
// ECDICT holds the past tense and the plural of most words as headwords of
// their own — "said", "men", "went" are all entries — so the rules below only
// have to cover what it happens to be missing, and among the common verb forms
// that is "are" and "were" (the dictionary has "am", "is", "been", "being" and
// "be", but not these two). Measured on the same corpus: they are the most
// frequent unresolved words after the Pāḷi loanwords, so leaving them out would
// be visible.
var englishIrregulars = map[string]string{
	"are": "be", "were": "be", "aren't": "be", "weren't": "be",
}

// NormalizeEnglishWord reduces a token from the page to a lookup key: lower
// case, no punctuation at either end, and no possessive.
//
// Lower case is not cosmetic here. The column is binary collated (铁律 1), so
// unlike a database on MySQL's default collation this one compares "The" and
// "the" as different strings — folding the case before the query is what makes
// the lookup case-insensitive.
func NormalizeEnglishWord(raw string) string {
	w := strings.ToLower(strings.TrimSpace(raw))
	w = strings.Trim(w, " \t\r\n.,;:!?\"'()[]{}“”‘’«»…—–-")
	// A possessive is the noun, not a word of its own.
	w = strings.TrimSuffix(w, "'s")
	w = strings.TrimSuffix(w, "’s")
	return strings.Trim(w, " \t\r\n.,;:!?\"'()[]{}“”‘’«»…—–-")
}

// EnglishCandidates lists the forms worth trying for a clicked word, in the
// order they should win.
//
// Order is the whole of the correctness argument here. A rule that strips two
// letters before one that strips a single letter turns "uses" into "us" and
// "used" into "us" — both of which are dictionary headwords — so the answer
// would be a real entry for a word the reader did not tap. The rules below
// therefore try the shorter removal first where English inflects by adding a
// letter ("use" + "d", "house" + "s") and the doubling case after the plain
// stem, and the tests pin the traps.
func EnglishCandidates(raw string) []string {
	word := NormalizeEnglishWord(raw)
	if word == "" {
		return nil
	}

	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		// 64 is the column width; a longer candidate could only ever miss.
		if s == "" || len(s) > 64 || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}

	add(word)
	if base, ok := englishContractions[word]; ok {
		add(base)
	}
	if base, ok := englishIrregulars[word]; ok {
		add(base)
	}
	// Inside a word: an apostrophe and a hyphen each hold a second word.
	if i := strings.IndexAny(word, "'’"); i > 0 {
		add(word[:i])
	}
	// A hyphenated compound is answered by the half that is a word of its own:
	// mind-produced → mind, rebirth-linking → rebirth, kamma-produced →
	// produced. Both halves are tried, the first one first, because either can
	// be the word the reader wants.
	//
	// Neither half is tried when it is shorter than three letters. Two-letter
	// fragments are prefixes rather than words, and the dictionary has entries
	// for some of them that mean something else entirely: "co-nascence"
	// (sahajāta, a term of the Abhidhamma) resolved to "Co", the chemical
	// symbol for cobalt, which is worse than saying nothing. The minimum is on
	// the hyphen split only — a contraction's base really can be two letters
	// ("isn't" → "is"), and that is what the map above is for.
	if i := strings.IndexByte(word, '-'); i > 0 {
		if i >= 3 {
			add(word[:i])
		}
		if len(word)-i-1 >= 3 {
			add(word[i+1:])
		}
	}
	// Then the inflections, applied to every candidate reached so far: the base
	// of a contraction is itself often inflected ("aren't" → "are" → "be").
	direct := len(out)
	for _, base := range out[:direct] {
		for _, form := range englishInflections(base) {
			add(form)
		}
	}
	return out
}

// englishInflections returns the headwords an inflected form may have come
// from, most likely first.
//
// Two rules carry the weight, and both are about the order rather than the
// endings. English forms the past tense by adding "d" to a verb that already
// ends in "e" and "ed" otherwise, so "used" is "use"+"d" — while "us" is a
// headword of its own, and a rule that strips both letters first answers the
// reader's tap on "used" with the entry for "us". The same trap sits in the
// plural: "uses" is "use"+"s". So the shorter removal is tried first, and a
// stem shorter than three letters is not a stem at all.
func englishInflections(w string) []string {
	var out []string
	add := func(s string) {
		if s != "" {
			out = append(out, s)
		}
	}
	stem := func(n int) string { return w[:len(w)-n] }
	// The doubled final consonant: stopped → stopp → stop, bigger → bigg → big.
	doubled := func(s string) {
		if len(s) > 2 && s[len(s)-1] == s[len(s)-2] {
			add(s[:len(s)-1])
		}
	}

	switch {
	case strings.HasSuffix(w, "ies") && len(stem(3)) >= 3:
		add(stem(3) + "y") // cities → city
	case strings.HasSuffix(w, "es") && len(w) > 3:
		add(stem(1)) // houses → house; uses → use
		if s := stem(2); len(s) >= 3 {
			// The "-es" that is a whole syllable: boxes → box, churches → church.
			add(s)
		}
	case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && len(stem(1)) >= 3:
		add(stem(1)) // monks → monk
	}

	switch {
	case strings.HasSuffix(w, "ing") && len(stem(3)) >= 3:
		s := stem(3)
		add(s) // dwelling → dwell
		add(s + "e")
		doubled(s) // running → run
	case strings.HasSuffix(w, "ied") && len(stem(3)) >= 3:
		add(stem(3) + "y") // studied → study
	case strings.HasSuffix(w, "ed") && len(stem(2)) >= 2:
		s := stem(2)
		if len(s) >= 3 {
			add(s) // added → add
		}
		add(stem(1)) // used → use
		doubled(s)   // stopped → stop
	case strings.HasSuffix(w, "iest") && len(stem(4)) >= 3:
		add(stem(4) + "y") // easiest → easy
	case strings.HasSuffix(w, "ier") && len(stem(3)) >= 3:
		add(stem(3) + "y") // easier → easy
	case strings.HasSuffix(w, "est") && len(stem(3)) >= 3:
		s := stem(3)
		add(s)
		add(s + "e")
		doubled(s)
	case strings.HasSuffix(w, "er") && len(stem(2)) >= 3:
		s := stem(2)
		add(s) // walker → walk
		add(s + "e")
		doubled(s) // bigger → big
	case strings.HasSuffix(w, "ily") && len(stem(3)) >= 3:
		add(stem(3) + "y") // happily → happy
	case strings.HasSuffix(w, "ly") && len(stem(2)) >= 3:
		add(stem(2)) // quickly → quick
	}
	return out
}

// LookupEnglish resolves one word of the English reference translation.
func (s *Service) LookupEnglish(ctx context.Context, form string) (*EnResult, error) {
	key := NormalizeEnglishWord(form)
	if key == "" {
		return &EnResult{Query: form, Available: true, Senses: []EnSense{}}, nil
	}

	cacheKey := "d:en:" + key
	var cached EnResult
	if s.cache.GetJSON(ctx, cacheKey, &cached) {
		cached.Query = form
		return &cached, nil
	}

	res, err := s.buildEnglish(ctx, form, key)
	if err != nil {
		return nil, err
	}
	// An "unavailable" answer is never cached: it says something about this
	// installation (the add-on has not been imported yet), not about the word,
	// and caching it would keep the popup saying so for the whole TTL after the
	// dictionary had been imported.
	if s.cache != nil && res.Available {
		s.cache.SetJSON(ctx, cacheKey, res, seconds(s.ttl))
	}
	res.Query = form
	return res, nil
}

func (s *Service) buildEnglish(ctx context.Context, form, key string) (*EnResult, error) {
	res := &EnResult{Query: form, Available: true, Senses: []EnSense{}}

	cands := EnglishCandidates(key)
	if len(cands) == 0 {
		return res, nil
	}

	// One query for the whole candidate list rather than one per candidate: a
	// word the dictionary does not have is the common case in this corpus (the
	// translations keep Pāḷi words the English dictionary has never heard of),
	// and ten round trips to answer "no" would be ten times the cost of
	// answering "yes".
	var rows []store.DictEnEntry
	if err := s.db.WithContext(ctx).Where("word IN ?", cands).Find(&rows).Error; err != nil {
		log.Printf("english lookup %q: %v", form, err)
		return &EnResult{Query: form, Available: false, Senses: []EnSense{}}, nil
	}

	// The candidate order decides, not the database's row order.
	byWord := make(map[string]store.DictEnEntry, len(rows))
	for _, r := range rows {
		byWord[r.Word] = r
	}
	for _, cand := range cands {
		entry, ok := byWord[cand]
		if !ok {
			continue
		}
		res.Found = true
		res.Word = entry.Head
		if res.Word == "" {
			res.Word = entry.Word
		}
		res.Phonetic = entry.Phonetic
		if cand != key {
			res.Via = cand
		}
		if err := json.Unmarshal([]byte(entry.Senses), &res.Senses); err != nil {
			// A row that cannot be decoded is one broken entry, not a broken
			// dictionary: report the word as unknown rather than failing the
			// request.
			log.Printf("english entry %q: %v", cand, err)
			res.Found = false
			res.Senses = []EnSense{}
		}
		return res, nil
	}

	// Nothing matched. Only now is it worth asking whether the dictionary is
	// there at all: a hit already proves it is, and counting the table on the
	// way to every hit cost each cold lookup a scan of 89k rows — 10ms instead
	// of 1ms, measured on the server. The answer matters here, because "no
	// entry" is a fact about the word and "not imported" is a fact about this
	// installation, and the reader is told which one it is.
	//
	// EXISTS rather than COUNT(*): the question is whether there is a
	// dictionary, not how big it is, and EXISTS stops at the first row.
	var has int
	if err := s.db.WithContext(ctx).
		Raw("SELECT EXISTS(SELECT 1 FROM " + store.DictEnEntry{}.TableName() + ")").Scan(&has).Error; err != nil {
		log.Printf("english dictionary unavailable: %v", err)
		return &EnResult{Query: form, Available: false, Senses: []EnSense{}}, nil
	}
	if has == 0 {
		return &EnResult{Query: form, Available: false, Senses: []EnSense{}}, nil
	}
	return res, nil
}
