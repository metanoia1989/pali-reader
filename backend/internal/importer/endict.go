package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/metanoia/pali-reader/backend/internal/store"
)

// The English dictionary is an add-on, not part of the canon: it exists so a
// reader can tap a word in the English 参考译文 and see what it means. Two
// consequences shape everything here.
//
// It is optional. The importer is also run on machines and in runs that have no
// copy of the seed, so a missing file is a skipped step with a line in the log,
// never a failed import — the reader works without it, the words simply come
// back "no entry".
//
// It is not embedded in the API binary. The seed is a build-time input that
// lives beside the other sources (dpd.db, epitaka*.db) and is read only by
// this program; correcting one definition must not mean rebuilding and
// redeploying the service.

// EnglishDictSeed is the file name the step looks for under -sources.
const EnglishDictSeed = "dict_seed.json"

// englishBatchSize is rows per multi-value INSERT: 4 columns × 500 rows = 2000
// bind variables, comfortably under MySQL's per-statement placeholder limit.
const englishBatchSize = 500

// enSeedSense mirrors one sense in the seed. The seed is ECDICT-derived and
// keeps ECDICT's single-letter keys.
type enSeedSense struct {
	Pos string `json:"pos"`
	Def string `json:"def"`
}

type enSeedEntry struct {
	W string        `json:"w"`
	P string        `json:"p"`
	S []enSeedSense `json:"s"`
}

// ImportEnglishDictionary loads the ECDICT-derived English→Chinese dictionary
// into dict_en_entries.
//
// The table is emptied first and the whole run happens in one transaction. The
// step is then exactly reproducible from the file: an upsert would leave behind
// entries a corrected seed had dropped, and there is no way to tell a stale row
// from a live one afterwards. 89k rows are cheap to replace and the file is the
// source of truth.
//
// A missing seed is not an error. See the note at the top of this file.
func ImportEnglishDictionary(path string, g *store.DB, log func(string, ...any)) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log("  %s not found — skipping the English dictionary; "+
				"the reader works without it and English words will report no entry", EnglishDictSeed)
			return nil
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	start := time.Now()
	var (
		batch   []store.DictEnEntry
		total   int
		skipped int
	)
	err = g.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM " + store.DictEnEntry{}.TableName()).Error; err != nil {
			return err
		}
		n, skippedN, err := StreamEnglishSeed(f, func(e store.DictEnEntry) error {
			batch = append(batch, e)
			if len(batch) < englishBatchSize {
				return nil
			}
			chunk := batch
			batch = nil
			return tx.CreateInBatches(chunk, englishBatchSize).Error
		})
		total, skipped = n, skippedN
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		return tx.CreateInBatches(batch, englishBatchSize).Error
	})
	if err != nil {
		return fmt.Errorf("import english dictionary: %w", err)
	}

	log("  english entries: %d (skipped %d without a meaning) in %s",
		total, skipped, time.Since(start).Round(time.Millisecond))
	return nil
}

// StreamEnglishSeed reads the seed and hands each entry to emit as it goes.
//
// One entry at a time rather than unmarshalling the array: the decoded form of
// this file is several times its 8.9 MB on disk, and the machine that runs this
// also runs the reader. Separated from the database work so the parsing — the
// part that has to be right about a format somebody else produces — can be
// tested without a database.
//
// The count returned is what was emitted; entries with no word or no meaning
// are skipped and counted rather than written as empty rows.
func StreamEnglishSeed(r io.Reader, emit func(store.DictEnEntry) error) (imported, skipped int, err error) {
	dec := json.NewDecoder(r)
	tok, err := dec.Token()
	if err != nil {
		return 0, 0, fmt.Errorf("read seed: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return 0, 0, errors.New("the seed must be a JSON array of entries")
	}

	for dec.More() {
		var e enSeedEntry
		if err := dec.Decode(&e); err != nil {
			return imported, skipped, fmt.Errorf("decode entry %d: %w", imported+skipped, err)
		}
		row, ok := englishRow(e)
		if !ok {
			skipped++
			continue
		}
		if err := emit(row); err != nil {
			return imported, skipped, err
		}
		imported++
	}

	// The closing bracket is read on purpose: a file truncated mid-entry stops
	// the decoder without an error, and the dictionary would then be imported up
	// to whatever letter the transfer died at.
	if _, err := dec.Token(); err != nil {
		if errors.Is(err, io.EOF) {
			return imported, skipped, errors.New("the array is not closed; the seed is truncated")
		}
		return imported, skipped, fmt.Errorf("read seed: %w", err)
	}
	return imported, skipped, nil
}

// englishRow turns one seed entry into the row it is stored as.
func englishRow(e enSeedEntry) (store.DictEnEntry, bool) {
	head := strings.TrimSpace(e.W)
	if head == "" {
		return store.DictEnEntry{}, false
	}
	senses := make([]enSeedSense, 0, len(e.S))
	for _, s := range e.S {
		def := NormalizeDefText(s.Def)
		if def == "" {
			continue
		}
		senses = append(senses, enSeedSense{Pos: strings.TrimSpace(s.Pos), Def: def})
	}
	if len(senses) == 0 {
		return store.DictEnEntry{}, false
	}
	blob, err := json.Marshal(senses)
	if err != nil {
		return store.DictEnEntry{}, false
	}
	return store.DictEnEntry{
		// The key is folded here, once, because the column is binary collated:
		// see store.DictEnEntry.
		Word:     strings.ToLower(head),
		Head:     head,
		Phonetic: strings.TrimSpace(e.P),
		Senses:   string(blob),
	}, true
}

// NormalizeDefText turns the escape sequences ECDICT writes inside a
// definition into real newlines.
//
// ECDICT separates a word's glosses with a *literal* two-character `\n`
// (backslash then "n"), not with a line feed: `居住, 居住(于)\n[医] 住房`. The
// seed stores that convention verbatim, and the reader wraps definitions with
// `white-space: pre-line`, which only breaks on real newlines — shipped
// unchanged, the popup prints "\n" in the middle of a definition. 38,437 of
// the 89,501 entries carry at least one.
//
// It is applied when the row is written rather than when it is read: this table
// has exactly one writer, so there is no legacy data to clean up on the way out
// and the hot lookup path pays nothing. Idempotent, so a seed that has already
// been through it is unchanged.
func NormalizeDefText(s string) string {
	if !strings.Contains(s, `\`) {
		return strings.TrimSpace(s)
	}
	s = strings.ReplaceAll(s, `\r\n`, "\n")
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\r`, "\n")
	return strings.TrimSpace(s)
}
