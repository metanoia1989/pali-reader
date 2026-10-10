package dict

import (
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestEnglishCoverage measures what the English lookup can actually answer, on
// the real corpus, with the real dictionary and the real candidate rules.
//
// It is env-gated because it reads two large files that do not live in this
// repository: the ECDICT seed (a build-time input, see AGENTS 铁律 22) and the
// English reference corpus. Run it when the rules change, and put the numbers
// it prints in the commit message — they are the honest scope of the feature:
//
//	ENDICT_SEED=/path/to/dict_seed.json \
//	ENDICT_CORPUS=../../pali-data/epitaka_en.db \
//	../go.sh test ./internal/dict -run TestEnglishCoverage -v
//
// A number in a comment goes stale silently; this one cannot.
func TestEnglishCoverage(t *testing.T) {
	seedPath := os.Getenv("ENDICT_SEED")
	corpusPath := os.Getenv("ENDICT_CORPUS")
	if seedPath == "" || corpusPath == "" {
		t.Skip("set ENDICT_SEED and ENDICT_CORPUS to measure lookup coverage")
	}

	lexicon, err := loadSeedLexicon(seedPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("dictionary headwords: %d", len(lexicon))

	db, err := sql.Open("sqlite", "file:"+corpusPath+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	rows, err := db.Query(`SELECT translation FROM sentences
		WHERE translation IS NOT NULL LIMIT 300000`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	// The same shape of token the client draws as a tappable word: letters with
	// their diacritics, joined by an apostrophe or a hyphen. Digits are not
	// words here for the same reason they are not in the Pāḷi text.
	word := regexp.MustCompile(`[\p{L}\p{M}]+(?:['’\-][\p{L}\p{M}]+)*`)

	var (
		lines                       int
		tokens, exact, viaCandidate int
		distinct                    = map[string]bool{}
		exactDistinct               = map[string]bool{}
		misses                      = map[string]int{}
		// The words that reach an entry only through a rule: what the layered
		// lookup is worth, named.
		viaOnly = map[string]int{}
	)
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			t.Fatal(err)
		}
		lines++
		for _, w := range word.FindAllString(text, -1) {
			key := NormalizeEnglishWord(w)
			if key == "" {
				continue
			}
			tokens++
			distinct[key] = true
			if lexicon[key] {
				exact++
				exactDistinct[key] = true
				continue
			}
			hit := ""
			for _, c := range EnglishCandidates(key) {
				if lexicon[c] {
					hit = c
					break
				}
			}
			if hit == "" {
				misses[key]++
				continue
			}
			viaCandidate++
			viaOnly[key]++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	pct := func(n int) float64 { return 100 * float64(n) / float64(tokens) }
	t.Logf("lines scanned: %d, word occurrences: %d, distinct words: %d",
		lines, tokens, len(distinct))
	t.Logf("exact headword:  %7d occurrences (%.1f%%), %d distinct",
		exact, pct(exact), len(exactDistinct))
	t.Logf("via inflection:  %7d occurrences (%.1f%%)", exact+viaCandidate, pct(exact+viaCandidate))
	t.Logf("unresolved:      %7d occurrences (%.1f%%)", tokens-exact-viaCandidate,
		pct(tokens-exact-viaCandidate))

	// The largest unresolved words, so "what is left" is a fact rather than an
	// assertion. They are the Pāḷi the translations keep verbatim.
	type pair struct {
		w string
		n int
	}
	var top []pair
	for w, n := range misses {
		top = append(top, pair{w, n})
	}
	for i := 1; i < len(top); i++ {
		for j := i; j > 0 && top[j].n > top[j-1].n; j-- {
			top[j], top[j-1] = top[j-1], top[j]
		}
	}
	if len(top) > 15 {
		top = top[:15]
	}
	var sb strings.Builder
	for _, p := range top {
		sb.WriteString(p.w + " ")
	}
	t.Logf("most frequent unresolved: %s", sb.String())

	// And the words the rules earn their keep on: surface forms with no entry
	// of their own that still resolve.
	var viaList []pair
	for w, n := range viaOnly {
		viaList = append(viaList, pair{w, n})
	}
	for i := 1; i < len(viaList); i++ {
		for j := i; j > 0 && viaList[j].n > viaList[j-1].n; j-- {
			viaList[j], viaList[j-1] = viaList[j-1], viaList[j]
		}
	}
	if len(viaList) > 15 {
		viaList = viaList[:15]
	}
	var vb strings.Builder
	for _, p := range viaList {
		vb.WriteString(p.w + " ")
	}
	t.Logf("most frequent resolved by a rule: %s", vb.String())
}

// loadSeedLexicon reads just the headwords out of the ECDICT seed. It is the
// same file the importer reads, and the key is folded the same way, because a
// measurement against a differently folded key would measure the wrong thing.
func loadSeedLexicon(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	if _, err := dec.Token(); err != nil { // the opening bracket
		return nil, err
	}
	out := map[string]bool{}
	for dec.More() {
		var e struct {
			W string `json:"w"`
		}
		if err := dec.Decode(&e); err != nil {
			return nil, err
		}
		// The same fold the importer writes the key with (importer.englishRow):
		// case and surrounding space, nothing more.
		if key := strings.ToLower(strings.TrimSpace(e.W)); key != "" {
			out[key] = true
		}
	}
	if _, err := dec.Token(); err != nil && err != io.EOF {
		return nil, err
	}
	return out, nil
}
