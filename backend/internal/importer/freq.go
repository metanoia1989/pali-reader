package importer

import (
	"sort"

	"github.com/metanoia/pali-reader/backend/internal/store"
	"github.com/metanoia/pali-reader/backend/internal/tokenize"
	"gorm.io/gorm/clause"
)

// maxWordLen bounds what counts as a word. The longest real Pāḷi compound in
// the canon is well under this; anything longer came from a damaged page.
const maxWordLen = 96

// ImportWordFrequency counts every word in the corpus.
//
// The reader uses the counts for two things: marking a word as rare enough to
// be worth keeping, and showing which words on the current page the reader has
// not met before. Both are advisory, so the pass reads the stored text rather
// than the token cache — one less thing to keep in step.
func ImportWordFrequency(g *store.DB, log func(string, ...any)) error {
	rows, err := g.Model(&store.TextSegment{}).
		Select("text").
		Where("kind <> ?", "heading").
		Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return err
		}
		for _, t := range tokenize.Split(text) {
			// A run of letters with no separator is not a word. The corpus has
			// a handful of these from damaged pages, and they would otherwise
			// overflow the word column.
			if t.Key == "" || len(t.Key) > maxWordLen {
				continue
			}
			counts[t.Key]++
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	freqs := make([]store.WordFreq, 0, len(counts))
	for w, c := range counts {
		freqs = append(freqs, store.WordFreq{Word: w, Count: c})
	}
	// Rank by descending count; ties broken alphabetically so a rebuild of the
	// same corpus always produces the same ranks.
	sort.Slice(freqs, func(i, j int) bool {
		if freqs[i].Count != freqs[j].Count {
			return freqs[i].Count > freqs[j].Count
		}
		return freqs[i].Word < freqs[j].Word
	})
	for i := range freqs {
		freqs[i].Rank = i + 1
	}
	log("  distinct words: %d", len(freqs))
	if len(freqs) == 0 {
		return nil
	}
	if err := g.Where("1 = 1").Delete(&store.WordFreq{}).Error; err != nil {
		return err
	}
	return g.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(freqs, 2000).Error
}
