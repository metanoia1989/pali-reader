// Package dict resolves a surface form into everything the lookup panel shows.
//
// Resolution is layered, because a Pāḷi word on the page is not the same thing
// as a dictionary entry: it is an inflected form that may belong to several
// headwords, it may be a compound whose parts are themselves entries, and it
// may be spelled in a way DPD only lists as a variant. The panel needs all of
// that at once, so the work is done here in one pass and cached as one value.
package dict

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/metanoia/pali-reader/backend/internal/cache"
	"github.com/metanoia/pali-reader/backend/internal/store"
	"github.com/metanoia/pali-reader/backend/internal/tokenize"
)

// Service answers dictionary questions.
type Service struct {
	db    *store.DB
	cache *cache.Cache
	ttl   int64 // seconds
}

// New builds a Service.
func New(db *store.DB, c *cache.Cache, ttlSeconds int64) *Service {
	return &Service{db: db, cache: c, ttl: ttlSeconds}
}

// Analysis is one grammatical reading of the clicked form, split into the
// columns the panel's 词形分析 table shows.
type Analysis struct {
	POS     string `json:"pos"`
	Grammar string `json:"grammar"`
	Lemma   string `json:"lemma"`
	// Gender, Case and Number are the parsed columns. Qualifier keeps whatever
	// else DPD wrote (a tense, "reflx", a stem class) so nothing is lost.
	Gender    string `json:"gender,omitempty"`
	Case      string `json:"case,omitempty"`
	Number    string `json:"number,omitempty"`
	Qualifier string `json:"qualifier,omitempty"`
	// HeadwordID ties the reading to the entry it belongs to, when one is known.
	HeadwordID uint `json:"headwordId,omitempty"`
}

// Meaning is one selectable gloss. It is deliberately flat: the reader picks a
// meaning, and everything needed to record that choice travels with it.
type Meaning struct {
	Source string `json:"source"` // machine code, e.g. "dpd"
	Name   string `json:"name"`   // display name of the dictionary
	Lang   string `json:"lang"`   // "en" | "zh"
	POS    string `json:"pos,omitempty"`
	Text   string `json:"text"`
	// HeadwordID is set when the gloss came from a DPD headword, so the reader
	// can record which entry was chosen and not only which words.
	HeadwordID uint `json:"headwordId,omitempty"`
	// ViaLemma is true when the dictionary has no entry for the inflected form
	// and this gloss was taken from the entry for its lemma instead.
	ViaLemma bool `json:"viaLemma,omitempty"`
}

// Split is one decomposition of a compound.
type Split struct {
	Parts []string `json:"parts"`
	Type  string   `json:"type,omitempty"`
	Note  string   `json:"note,omitempty"`
	// Resolved is true when every part can itself be looked up.
	Resolved bool `json:"resolved"`
}

// DeclRow is one row of a declension table, already column-aligned.
type DeclRow struct {
	Case  string     `json:"case"`
	Cells [][]string `json:"cells"`
}

// Declension is an inflection grid.
//
// Cells hold the *ending*, not the whole word, and Stem is DPD's stem without
// a connecting vowel — "buddh" for buddha. Stem plus ending is the form as it
// appears in the text, and showing the two apart is the point: the reader sees
// that every form in a column shares one ending, and where this occurrence
// sits among them. The client draws the stem in body weight and the ending in
// ink blue.
type Declension struct {
	Pattern string    `json:"pattern"`
	Like    string    `json:"like"`
	Stem    string    `json:"stem"`
	Columns []string  `json:"columns"`
	Rows    []DeclRow `json:"rows"`
	// Hit is the cell matching the form the reader clicked, if any.
	Hit *DeclHit `json:"hit,omitempty"`
}

// DeclHit points at one cell of a Declension.
type DeclHit struct {
	Row    int `json:"row"`
	Column int `json:"column"`
	// Suffix is the ending in the matched cell; Form is what it makes with the
	// stem, which is the word as it appears in the text.
	Suffix string `json:"suffix"`
	Form   string `json:"form"`
}

// Headword is one dictionary entry offered for the clicked form.
type Headword struct {
	ID         uint   `json:"id"`
	Lemma      string `json:"lemma"`
	Homonym    string `json:"homonym,omitempty"`
	POS        string `json:"pos"`
	Grammar    string `json:"grammar,omitempty"`
	Phonetic   string `json:"phonetic,omitempty"`
	Meaning1   string `json:"meaning1,omitempty"`
	MeaningLit string `json:"meaningLit,omitempty"`
	Meaning2   string `json:"meaning2,omitempty"`
	// Construction is the word-formation analysis, e.g. "√dhar + ma".
	Construction string `json:"construction,omitempty"`
	// CompoundConstruction is the compound split, e.g. "dhamma + cakka",
	// with CompoundType naming the relation (kammadhāraya, tappurisa, ...).
	CompoundConstruction string `json:"compoundConstruction,omitempty"`
	CompoundType         string `json:"compoundType,omitempty"`
	Stem                 string `json:"stem,omitempty"`
	Pattern              string `json:"pattern,omitempty"`
	RootKey              string `json:"rootKey,omitempty"`
	RootBase             string `json:"rootBase,omitempty"`
	RootMeaning          string `json:"rootMeaning,omitempty"`
	Sanskrit             string `json:"sanskrit,omitempty"`
	Synonym              string `json:"synonym,omitempty"`
	Antonym              string `json:"antonym,omitempty"`
	Variant              string `json:"variant,omitempty"`
	Notes                string `json:"notes,omitempty"`
	EBTCount             int    `json:"ebtCount,omitempty"`
	// Analyses lists every reading of the clicked form that belongs to this
	// headword, so the panel can say "masc nom sg" and not merely "noun".
	Analyses []Analysis `json:"analyses,omitempty"`
	// Families are the DPD word-family memberships, for the 词族 tab.
	Families []FamilyRef `json:"families,omitempty"`
	// Declension is filled on demand by the inflection endpoint as well as
	// inline here, because the panel previews it without a second request.
	Declension *Declension `json:"declension,omitempty"`
}

// FamilyRef is one word-family the headword belongs to.
type FamilyRef struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// Result is the whole answer for one clicked form.
type Result struct {
	// Query is the form as it appeared in the text; Key is the normalised form
	// the dictionaries are keyed by. They differ when the text has an
	// apostrophe or a capital.
	Query string `json:"query"`
	Key   string `json:"key"`
	Found bool   `json:"found"`
	// Headwords are the entries this form can belong to, best first.
	Headwords []Headword `json:"headwords"`
	// Analyses are the form-level readings, including ones whose headword is
	// ambiguous.
	Analyses []Analysis `json:"analyses,omitempty"`
	// Meanings is every gloss from every dictionary, in panel order.
	Meanings []Meaning `json:"meanings"`
	// Splits are the compound decompositions, most likely first.
	Splits []Split `json:"splits,omitempty"`
	// Spelling holds near-miss suggestions; See holds redirections.
	Spelling []string `json:"spelling,omitempty"`
	See      []string `json:"see,omitempty"`
	// Roots are the verbal roots behind the headwords.
	Roots []store.DictRoot `json:"roots,omitempty"`
	// Freq is how often the form occurs in the corpus, 0 when unattested.
	Freq int `json:"freq,omitempty"`
	Rank int `json:"rank,omitempty"`
}

// Lookup resolves a surface form.
func (s *Service) Lookup(ctx context.Context, form string) (*Result, error) {
	key := tokenize.Normalize(strings.TrimSpace(form))
	if key == "" {
		return &Result{Query: form, Found: false}, nil
	}

	cacheKey := "d:lk:" + key
	var cached Result
	if s.cache.GetJSON(ctx, cacheKey, &cached) {
		cached.Query = form
		return &cached, nil
	}

	r, err := s.build(ctx, form, key)
	if err != nil {
		return nil, err
	}
	if s.cache != nil {
		s.cache.SetJSON(ctx, cacheKey, r, seconds(s.ttl))
	}
	r.Query = form
	return r, nil
}

func (s *Service) build(ctx context.Context, form, key string) (*Result, error) {
	res := &Result{Query: form, Key: key}

	var lk store.DictLookup
	err := s.db.WithContext(ctx).Where("lookup_key = ?", key).First(&lk).Error
	if err != nil && !store.IsNotFound(err) {
		return nil, err
	}
	res.Found = err == nil

	if res.Found {
		res.Analyses = decodeAnalyses(lk.Grammar)
		for i := range res.Analyses {
			a := &res.Analyses[i]
			a.POS = posLabel(a.POS)
			a.Gender, a.Case, a.Number, a.Qualifier = ParseGrammarParts(a.Grammar)
		}
		res.Spelling = decodeStrings(lk.Spelling)
		res.See = decodeStrings(lk.See)
	}

	// A word nobody indexes may still be a compound of things that are.
	splits := decodeStrings(lk.Deconstructor)
	if len(splits) == 0 {
		splits = strings.Split(strings.Trim(lk.Variant, "[]\""), ",")
	}

	idSet := map[uint]bool{}
	var ids []uint
	if res.Found {
		for _, id := range decodeInts(lk.HeadwordIDs) {
			if !idSet[id] {
				idSet[id] = true
				ids = append(ids, id)
			}
		}
	}

	var heads []store.DictHeadword
	if len(ids) > 0 {
		if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&heads).Error; err != nil {
			return nil, err
		}
		sort.SliceStable(heads, func(i, j int) bool {
			return indexOf(ids, heads[i].ID) < indexOf(ids, heads[j].ID)
		})
	}

	patternSet := map[string]bool{}
	for _, h := range heads {
		hw := s.toHeadword(h, res.Analyses, key)
		res.Headwords = append(res.Headwords, hw)
		patternSet[h.Pattern] = true
		res.Meanings = append(res.Meanings, Meaning{
			Source: "dpd", Name: "DPD", Lang: "en", POS: h.POS,
			Text: meaningText(h.Meaning1, h.MeaningLit), HeadwordID: h.ID,
		})
		if h.Meaning2 != "" && h.Meaning2 != h.Meaning1 {
			res.Meanings = append(res.Meanings, Meaning{
				Source: "dpd:2", Name: "DPD", Lang: "en", POS: h.POS,
				Text: h.Meaning2, HeadwordID: h.ID,
			})
		}
		// DPD's own split of the compound, when it has one.
		if h.CompoundConstruction != "" {
			splits = append([]string{h.CompoundConstruction}, splits...)
		}
	}

	// Other dictionaries. They are keyed by headword, not by inflected form:
	// 《巴漢詞典》 has an entry for buddha, never for buddhassa. So the clicked
	// form is tried first — some dictionaries do list inflections — and
	// otherwise the entries for the headwords this form resolves to are used,
	// attributed to their lemma so the reader is not misled about which word
	// the gloss is for.
	var entries []store.DictEntry
	if err := s.db.WithContext(ctx).Where("word = ?", key).Find(&entries).Error; err != nil {
		return nil, err
	}
	viaLemma := false
	if len(entries) == 0 && len(heads) > 0 {
		lemmas := make([]string, 0, len(heads))
		for _, h := range heads {
			if l := tokenize.Normalize(CleanLemma(h.Lemma1)); l != "" {
				lemmas = append(lemmas, l)
			}
		}
		if len(lemmas) > 0 {
			if err := s.db.WithContext(ctx).Where("word IN ?", lemmas).Find(&entries).Error; err != nil {
				return nil, err
			}
			viaLemma = len(entries) > 0
		}
	}
	if len(entries) > 0 {
		codes := make([]string, 0, len(entries))
		for _, e := range entries {
			codes = append(codes, e.Dict)
		}
		var srcs []store.DictSource
		if err := s.db.WithContext(ctx).Where("code IN ?", codes).Find(&srcs).Error; err != nil {
			return nil, err
		}
		meta := map[string]store.DictSource{}
		for _, s := range srcs {
			meta[s.Code] = s
		}
		sort.SliceStable(entries, func(i, j int) bool {
			return meta[entries[i].Dict].Sort < meta[entries[j].Dict].Sort
		})
		for _, e := range entries {
			m := meta[e.Dict]
			text := e.Body
			if viaLemma {
				text = e.Word + "：" + text
			}
			res.Meanings = append(res.Meanings, Meaning{
				Source: e.Dict, Name: m.Name, Lang: m.Lang, Text: text,
				// Records that this gloss is for the lemma rather than for the
				// form the reader tapped.
				ViaLemma: viaLemma,
			})
		}
	}

	res.Splits = s.buildSplits(ctx, splits)
	res.Roots = s.loadRoots(ctx, heads)

	// Every entry carries its own grid: the panel expands one entry at a time,
	// and a form shared by several headwords has a different paradigm for each.
	for i := range res.Headwords {
		if res.Headwords[i].Pattern == "" {
			continue
		}
		if d, err := s.Declension(ctx, res.Headwords[i].Pattern, res.Headwords[i].Stem, key); err == nil {
			res.Headwords[i].Declension = d
		}
	}

	var wf store.WordFreq
	if err := s.db.WithContext(ctx).Where("word = ?", key).First(&wf).Error; err == nil {
		res.Freq = wf.Count
		res.Rank = wf.Rank
	}

	return res, nil
}

func meaningText(m1, lit string) string {
	if lit == "" {
		return m1
	}
	if m1 == "" {
		return "lit. " + lit
	}
	return m1 + " (lit. " + lit + ")"
}

func (s *Service) toHeadword(h store.DictHeadword, all []Analysis, key string) Headword {
	lemma := CleanLemma(h.Lemma1)
	hw := Headword{
		ID: h.ID, Lemma: lemma, Homonym: Homonym(h.Lemma1), POS: h.POS,
		Grammar: h.Grammar, Phonetic: h.Phonetic,
		Meaning1: h.Meaning1, MeaningLit: h.MeaningLit, Meaning2: h.Meaning2,
		Construction: h.Construction, CompoundConstruction: h.CompoundConstruction,
		CompoundType: h.CompoundType, Stem: h.Stem, Pattern: h.Pattern,
		RootKey: h.RootKey, RootBase: h.RootBase, Sanskrit: h.Sanskrit,
		Synonym: h.Synonym, Antonym: h.Antonym, Variant: h.Variant,
		Notes: h.Notes, EBTCount: h.EBTCount,
	}
	// Keep only the analyses that belong to this headword, so a shared form
	// does not attribute one entry's case ending to another.
	for _, a := range all {
		if a.Lemma == "" || a.Lemma == lemma || a.Lemma == h.Lemma1 {
			hw.Analyses = append(hw.Analyses, a)
		}
	}
	// The clicked form may not be the lemma; say which form was matched so the
	// panel can show it next to the headword.
	_ = key

	for _, f := range []struct{ kind, val string }{
		{"root", h.FamilyRoot}, {"word", h.FamilyWord},
		{"compound", h.FamilyCompound}, {"idiom", h.FamilyIdiom},
		{"set", h.FamilySet},
	} {
		if f.val != "" {
			hw.Families = append(hw.Families, FamilyRef{Kind: f.kind, Value: f.val})
		}
	}
	return hw
}

// buildSplits turns raw decomposition strings into parts the panel can offer
// as buttons, marking each one that is itself a lookup key.
func (s *Service) buildSplits(ctx context.Context, raw []string) []Split {
	seen := map[string]bool{}
	var out []Split
	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r == "" || !strings.Contains(r, "+") {
			continue
		}
		if seen[r] {
			continue
		}
		seen[r] = true
		parts := make([]string, 0, 4)
		for _, p := range strings.Split(r, "+") {
			p = strings.TrimSpace(p)
			if p != "" {
				parts = append(parts, p)
			}
		}
		if len(parts) < 2 {
			continue
		}
		out = append(out, Split{Parts: parts})
		if len(out) >= 12 {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	// Mark which candidates actually resolve, in one query. DPD marks a case
	// ending inside a part with <b>, which import stored as a sentinel byte;
	// those have to come off before the part can be looked up, or
	// "addhānamaggaṃ" is reported unresolved when "addhānamagga" is an entry.
	clean := func(p string) string {
		return strings.TrimSpace(strings.NewReplacer("\u0001", "", "\u0002", "").Replace(p))
	}
	keys := make([]string, 0, len(out)*3)
	for _, sp := range out {
		for _, p := range sp.Parts {
			keys = append(keys, tokenize.Normalize(clean(p)))
		}
	}
	var found []string
	if err := s.db.WithContext(ctx).Model(&store.DictLookup{}).
		Where("lookup_key IN ?", keys).Pluck("lookup_key", &found).Error; err != nil {
		return out
	}
	ok := map[string]bool{}
	for _, f := range found {
		ok[f] = true
	}
	for i := range out {
		all := true
		for _, p := range out[i].Parts {
			if !ok[tokenize.Normalize(clean(p))] {
				all = false
				break
			}
		}
		out[i].Resolved = all
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Resolved != out[j].Resolved {
			return out[i].Resolved
		}
		return len(out[i].Parts) < len(out[j].Parts)
	})
	return out
}

func (s *Service) loadRoots(ctx context.Context, heads []store.DictHeadword) []store.DictRoot {
	var keys []string
	seen := map[string]bool{}
	for _, h := range heads {
		if h.RootKey != "" && !seen[h.RootKey] {
			seen[h.RootKey] = true
			keys = append(keys, h.RootKey)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	var roots []store.DictRoot
	if err := s.db.WithContext(ctx).Where("root IN ?", keys).Find(&roots).Error; err != nil {
		return nil
	}
	return roots
}

// Declension renders the inflection grid for a DPD pattern, marking the cell
// that produces form when it appears.
func (s *Service) Declension(ctx context.Context, pattern, stem, form string) (*Declension, error) {
	if pattern == "" {
		return nil, nil
	}
	cacheKey := "d:dc:" + pattern + ":" + stem + ":" + tokenize.Normalize(form)
	var cached Declension
	if s.cache.GetJSON(ctx, cacheKey, &cached) {
		return &cached, nil
	}

	var t store.DictTemplate
	if err := s.db.WithContext(ctx).Where("pattern = ?", pattern).First(&t).Error; err != nil {
		return nil, err
	}
	d, err := RenderDeclension(t, stem, form)
	if err != nil {
		return nil, err
	}
	if s.cache != nil {
		s.cache.SetJSON(ctx, cacheKey, d, seconds(s.ttl))
	}
	return d, nil
}

// RenderDeclension turns DPD's matrix into a table.
//
// DPD stores the grid as a list of rows; the first row carries the column
// headings at odd indices and each later row carries, for every column, the
// forms at 1+2c and the full label at 2+2c. Reading it positionally is what
// makes the header and the body line up for every stem type, including the
// adjective grids that carry six columns rather than two.
func RenderDeclension(t store.DictTemplate, stem, form string) (*Declension, error) {
	var matrix [][][]string
	if err := json.Unmarshal([]byte(t.Data), &matrix); err != nil {
		return nil, fmt.Errorf("template %s: %w", t.Pattern, err)
	}
	if len(matrix) == 0 {
		return nil, nil
	}
	head := matrix[0]
	ncol := (len(head) - 1) / 2
	if ncol <= 0 {
		return nil, nil
	}
	cleanStem := strings.NewReplacer("!", "", "*", "").Replace(stem)
	d := &Declension{Pattern: t.Pattern, Like: t.Like, Stem: cleanStem}
	for c := 0; c < ncol; c++ {
		label := ""
		if 1+2*c < len(head) && len(head[1+2*c]) > 0 {
			label = head[1+2*c][0]
		}
		d.Columns = append(d.Columns, label)
	}
	needle := tokenize.Normalize(form)
	for r := 1; r < len(matrix); r++ {
		row := matrix[r]
		if len(row) == 0 {
			continue
		}
		dr := DeclRow{}
		if len(row[0]) > 0 {
			dr.Case = row[0][0]
		}
		for c := 0; c < ncol; c++ {
			var forms []string
			if 1+2*c < len(row) {
				forms = row[1+2*c]
			}
			dr.Cells = append(dr.Cells, forms)
			if d.Hit == nil && needle != "" {
				for _, f := range forms {
					if tokenize.Normalize(cleanStem+f) == needle {
						d.Hit = &DeclHit{Row: r - 1, Column: c, Suffix: f, Form: cleanStem + f}
						break
					}
				}
			}
		}
		d.Rows = append(d.Rows, dr)
	}
	return d, nil
}

// Suggestions returns headwords whose lemma starts with the query, for the
// search box.
func (s *Service) Suggestions(ctx context.Context, q string, limit int) ([]Headword, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var hs []store.DictHeadword
	if err := s.db.WithContext(ctx).
		Where("lemma_1 LIKE ?", q+"%").
		Order("ebt_count DESC, lemma_1").
		Limit(limit).Find(&hs).Error; err != nil {
		return nil, err
	}
	out := make([]Headword, 0, len(hs))
	for _, h := range hs {
		out = append(out, Headword{
			ID: h.ID, Lemma: CleanLemma(h.Lemma1), Homonym: Homonym(h.Lemma1),
			POS: h.POS, Meaning1: h.Meaning1, MeaningLit: h.MeaningLit,
		})
	}
	return out, nil
}

// ---- decoding helpers -----------------------------------------------------

func decodeInts(s string) []uint {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil
	}
	var out []uint
	if err := json.Unmarshal([]byte(s), &out); err == nil {
		return out
	}
	// Older exports wrote a bare comma separated list.
	for _, part := range strings.Split(strings.Trim(s, "[]"), ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			out = append(out, uint(n))
		}
	}
	return out
}

func decodeStrings(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err == nil {
		return out
	}
	return []string{s}
}

func decodeAnalyses(s string) []Analysis {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil
	}
	var out []Analysis
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func indexOf(a []uint, v uint) int {
	for i, x := range a {
		if x == v {
			return i
		}
	}
	return len(a)
}

func seconds(n int64) time.Duration { return time.Duration(n) * time.Second }
