package importer

import (
	"database/sql"
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/metanoia/pali-reader/backend/internal/store"
	"github.com/metanoia/pali-reader/backend/internal/tokenize"
	"gorm.io/gorm/clause"
)

// ImportDictionary loads the Digital Pāḷi Dictionary.
//
// Three things come across: the headwords, the form-to-headword index, and the
// roots and inflection grids the panel renders.
//
// The index is assembled in memory because its inputs live in two databases
// and have to be joined on the lookup key. Writing it row by row through GORM
// exhausted MySQL's prepared-statement budget and took minutes; one batched
// upsert of the merged rows takes seconds.
func ImportDictionary(dpdPath, tipitakaPath string, g *store.DB, log func(string, ...any)) error {
	db, err := openSQLite(dpdPath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := importHeadwords(db, g, log); err != nil {
		return err
	}
	if err := importRoots(db, g, log); err != nil {
		return err
	}
	if err := importTemplates(db, g, log); err != nil {
		return err
	}

	index, order, err := loadLookup(db, log)
	if err != nil {
		return err
	}
	if err := addInflections(db, index, &order, log); err != nil {
		return err
	}
	if tipitakaPath != "" {
		tip, err := openSQLite(tipitakaPath)
		if err != nil {
			return err
		}
		defer tip.Close()
		if err := mergeGrammar(tip, index, log); err != nil {
			return err
		}
		if err := mergeWordSplit(tip, index, log); err != nil {
			return err
		}
	}
	return writeLookup(g, index, order, log)
}

func importHeadwords(db *sql.DB, g *store.DB, log func(string, ...any)) error {
	const q = `SELECT id, COALESCE(lemma_1,''), COALESCE(lemma_2,''), COALESCE(pos,''),
		COALESCE(grammar,''), COALESCE(meaning_1,''), COALESCE(meaning_lit,''), COALESCE(meaning_2,''),
		COALESCE(construction,''), COALESCE(compound_construction,''), COALESCE(compound_type,''),
		COALESCE(stem,''), COALESCE(pattern,''), COALESCE(root_key,''), COALESCE(root_sign,''),
		COALESCE(root_base,''), COALESCE(family_root,''), COALESCE(family_word,''),
		COALESCE(family_compound,''), COALESCE(family_idioms,''), COALESCE(family_set,''),
		COALESCE(derivative,''), COALESCE(suffix,''), COALESCE(phonetic,''), COALESCE(sanskrit,''),
		COALESCE(cognate,''), COALESCE(antonym,''), COALESCE(synonym,''), COALESCE(variant,''),
		COALESCE(notes,''), COALESCE(inflections,''), COALESCE(ebt_count,0)
		FROM dpd_headwords`
	rows, err := db.Query(q)
	if err != nil {
		return err
	}
	defer rows.Close()

	var batch []store.DictHeadword
	total := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := g.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(batch, 500).Error
		total += len(batch)
		batch = batch[:0]
		return err
	}
	for rows.Next() {
		var h store.DictHeadword
		if err := rows.Scan(&h.ID, &h.Lemma1, &h.Lemma2, &h.POS, &h.Grammar,
			&h.Meaning1, &h.MeaningLit, &h.Meaning2, &h.Construction,
			&h.CompoundConstruction, &h.CompoundType, &h.Stem, &h.Pattern,
			&h.RootKey, &h.RootSign, &h.RootBase, &h.FamilyRoot, &h.FamilyWord,
			&h.FamilyCompound, &h.FamilyIdiom, &h.FamilySet, &h.Derivative,
			&h.Suffix, &h.Phonetic, &h.Sanskrit, &h.Cognate, &h.Antonym,
			&h.Synonym, &h.Variant, &h.Notes, &h.Inflections, &h.EBTCount); err != nil {
			return err
		}
		sanitizeHeadword(&h)
		batch = append(batch, h)
		if len(batch) >= 1000 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	log("  headwords: %d", total)
	return nil
}

// sanitizeHeadword clears the presentation markup DPD carries in its text
// fields, keeping only the <b> emphasis as sentinel bytes.
func sanitizeHeadword(h *store.DictHeadword) {
	for _, f := range []*string{
		&h.Meaning1, &h.MeaningLit, &h.Meaning2,
		&h.Construction, &h.CompoundConstruction, &h.CompoundType,
		&h.Grammar, &h.Notes, &h.Synonym, &h.Antonym, &h.Variant,
		&h.Sanskrit, &h.FamilyRoot, &h.FamilyWord, &h.FamilyCompound,
		&h.FamilyIdiom, &h.FamilySet,
	} {
		*f = SanitizeMarkup(*f)
	}
}

func importRoots(db *sql.DB, g *store.DB, log func(string, ...any)) error {
	rows, err := db.Query(`SELECT root, COALESCE(root_meaning,''), COALESCE(root_sign,''),
		COALESCE(root_group,0), COALESCE(root_in_comps,''), COALESCE(sanskrit_root,''), COALESCE(note,'')
		FROM dpd_roots`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var batch []store.DictRoot
	for rows.Next() {
		var r store.DictRoot
		if err := rows.Scan(&r.Root, &r.RootMeaning, &r.RootSign, &r.RootGroup,
			&r.RootInComps, &r.SanskritRoot, &r.Note); err != nil {
			return err
		}
		batch = append(batch, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(batch) == 0 {
		return nil
	}
	log("  roots: %d", len(batch))
	return g.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(batch, 500).Error
}

func importTemplates(db *sql.DB, g *store.DB, log func(string, ...any)) error {
	rows, err := db.Query(`SELECT pattern, COALESCE("like",''), COALESCE(data,'') FROM inflection_templates`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var batch []store.DictTemplate
	for rows.Next() {
		var t store.DictTemplate
		if err := rows.Scan(&t.Pattern, &t.Like, &t.Data); err != nil {
			return err
		}
		batch = append(batch, t)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(batch) == 0 {
		return nil
	}
	log("  inflection templates: %d", len(batch))
	return g.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(batch, 200).Error
}

// loadLookup reads DPD's own index into memory.
func loadLookup(db *sql.DB, log func(string, ...any)) (map[string]*store.DictLookup, []string, error) {
	rows, err := db.Query(`SELECT lookup_key, COALESCE(headwords,''), COALESCE(deconstructor,''),
		COALESCE(spelling,''), COALESCE("see",''), COALESCE(variant,''), COALESCE(roots,'')
		FROM lookup`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	index := make(map[string]*store.DictLookup, 240000)
	var order []string
	for rows.Next() {
		var l store.DictLookup
		if err := rows.Scan(&l.Key, &l.HeadwordIDs, &l.Deconstructor, &l.Spelling,
			&l.See, &l.Variant, &l.Roots); err != nil {
			return nil, nil, err
		}
		if l.Key == "" {
			continue
		}
		cp := l
		index[l.Key] = &cp
		order = append(order, l.Key)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	log("  lookup keys from dpd: %d", len(order))
	return index, order, nil
}

// addInflections folds each headword's own generated form list into the index.
//
// DPD ships these forms on the headword but never put them in lookup, so
// without this pass a reader meets a perfectly regular form such as sādhunā
// and is told the word is unknown.
func addInflections(db *sql.DB, index map[string]*store.DictLookup, order *[]string, log func(string, ...any)) error {
	rows, err := db.Query(`SELECT id, COALESCE(inflections,'') FROM dpd_headwords WHERE inflections != ''`)
	if err != nil {
		return err
	}
	defer rows.Close()

	extra := map[string]map[int]bool{}
	for rows.Next() {
		var id int
		var infl string
		if err := rows.Scan(&id, &infl); err != nil {
			return err
		}
		for _, form := range strings.Split(infl, ",") {
			key := tokenize.Normalize(strings.TrimSpace(form))
			if key == "" {
				continue
			}
			if _, ok := index[key]; ok {
				continue
			}
			if extra[key] == nil {
				extra[key] = map[int]bool{}
			}
			extra[key][id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ids := make([]int, 0, len(extra[k]))
		for id := range extra[k] {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		b, _ := json.Marshal(ids)
		index[k] = &store.DictLookup{Key: k, HeadwordIDs: string(b)}
		*order = append(*order, k)
	}
	log("  lookup keys added from headword inflections: %d", len(keys))
	return nil
}

// mergeGrammar folds tipitaka_pali.db's per-form grammar table into the index.
// That table holds DPD's rendered analysis, so it is parsed back into rows.
func mergeGrammar(tip *sql.DB, index map[string]*store.DictLookup, log func(string, ...any)) error {
	rows, err := tip.Query(`SELECT word, COALESCE(definition,'') FROM dpd_grammar`)
	if err != nil {
		return err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var word, html string
		if err := rows.Scan(&word, &html); err != nil {
			return err
		}
		key := tokenize.Normalize(strings.TrimSpace(word))
		if key == "" {
			continue
		}
		l, ok := index[key]
		if !ok {
			continue
		}
		a := ParseGrammar(html)
		if len(a) == 0 {
			continue
		}
		b, _ := json.Marshal(a)
		l.Grammar = string(b)
		n++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	log("  grammar analyses merged: %d", n)
	return nil
}

// mergeWordSplit adds DPD's alternative compound splits, which cover the words
// whose own form is not a headword.
func mergeWordSplit(tip *sql.DB, index map[string]*store.DictLookup, log func(string, ...any)) error {
	rows, err := tip.Query(`SELECT word, COALESCE(breakup,'') FROM dpd_word_split WHERE breakup != ''`)
	if err != nil {
		return err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var word, breakup string
		if err := rows.Scan(&word, &breakup); err != nil {
			return err
		}
		key := tokenize.Normalize(strings.TrimSpace(word))
		l, ok := index[key]
		if !ok || l.Deconstructor != "" {
			continue
		}
		b, _ := json.Marshal(strings.Split(breakup, ","))
		l.Deconstructor = string(b)
		n++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	log("  compound splits merged: %d", n)
	return nil
}

func writeLookup(g *store.DB, index map[string]*store.DictLookup, order []string, log func(string, ...any)) error {
	batch := make([]store.DictLookup, 0, 2000)
	total := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := g.Clauses(clause.OnConflict{UpdateAll: true}).
			CreateInBatches(batch, 2000).Error; err != nil {
			return err
		}
		total += len(batch)
		batch = batch[:0]
		return nil
	}
	for _, k := range order {
		if l, ok := index[k]; ok {
			batch = append(batch, *l)
		}
		if len(batch) >= 2000 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}
	log("  lookup rows written: %d", total)
	return nil
}

// The cell opening tags carry attributes — `<td colspan='3'>indeclineable` is
// how DPD writes a reading that has no case or number. Requiring a bare `<td>`
// made the pattern skip those rows and then match across one, so the panel
// printed a fragment of the table's own HTML in the 格 column.
var grammarRowRe = regexp.MustCompile(
	`(?s)<tr><td[^>]*><b>(.*?)</b></td><td[^>]*>(.*?)</td>` +
		`(?:<td[^>]*>of</td><td[^>]*>(.*?)</td>)?</tr>`)

// GrammarAnalysis is one reading of an inflected form.
type GrammarAnalysis struct {
	POS     string `json:"pos"`
	Grammar string `json:"grammar"`
	Lemma   string `json:"lemma"`
}

// ParseGrammar turns DPD's rendered grammar table back into structured rows.
// The HTML is generated rather than authored, so the shape is regular; the
// pattern is anchored on whole <tr> elements so one malformed row cannot
// swallow the rest of the table.
func ParseGrammar(html string) []GrammarAnalysis {
	var out []GrammarAnalysis
	for _, m := range grammarRowRe.FindAllStringSubmatch(html, -1) {
		a := GrammarAnalysis{
			POS:     strings.TrimSpace(m[1]),
			Grammar: strings.TrimSpace(m[2]),
			Lemma:   strings.TrimSpace(m[3]),
		}
		if a.POS == "" && a.Grammar == "" {
			continue
		}
		out = append(out, a)
	}
	return out
}
