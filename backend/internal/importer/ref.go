package importer

import (
	"bufio"
	"database/sql"
	_ "embed"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/metanoia/pali-reader/backend/internal/store"
	"gorm.io/gorm/clause"
)

// Reference translations.
//
// ePitaka publishes a Chinese rendering of the canon, but it paragraphs the
// text differently from the CST pages this reader is built on: its paragraph
// counter is its own, not the canon's § numbering, so the two cannot be joined
// on a key. What they do share is the Pāḷi itself.
//
// So the alignment is done on the text. Both sides are reduced to word streams
// and every ePitaka paragraph is located in this reader's segment stream by its
// opening words, searched near where the previous paragraph landed. The texts
// are near-identical — ePitaka is a CST edition too — so the anchors are dense.
//
// A paragraph that failed to anchor contributes nothing: a translation under the
// wrong paragraph is worse than no translation, and a book whose alignment is
// poor overall is skipped whole.

//go:embed data/book_map.txt
var bookMapReport string

// shingle is the longest anchor tried: long enough that an ordinary opening is
// distinctive, short enough to survive a small wording difference. The search
// falls back to shorter ones for paragraphs that are themselves short, so the
// index has to carry all of them.
const (
	shingle    = 6
	minShingle = 4
)

// parseBookMap reads tipitaka-pali-reader's book map report. That project
// matched the two paragraphings by text continuity and published the result;
// reusing it rather than re-deriving it keeps a well-tested correspondence in
// one place.
//
// Format, one line per book:
//
//	mula_vi_02   -> Vin-ii [507-49855]  +  Vin-ii-b [50024-83148]   covered 98.0%
func parseBookMap() map[string][]string {
	out := map[string][]string{}
	sc := bufio.NewScanner(strings.NewReader(bookMapReport))
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), "->")
		if !ok {
			continue
		}
		key := strings.TrimSpace(name)
		if key == "" || strings.ContainsAny(key, " :") {
			continue
		}
		var books []string
		for _, part := range strings.Split(rest, "+") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			end := strings.IndexAny(part, " [")
			if end < 0 {
				end = len(part)
			}
			if id := strings.TrimSpace(part[:end]); id != "" {
				books = append(books, id)
			}
		}
		if len(books) > 0 {
			out[key] = books
		}
	}
	return out
}

// epiLine is one line of the edition: its Pāḷi, and the translation of that
// same line.
//
// The two ePitaka files are keyed identically — (book_id, para_id, line_id) —
// so a line's translation is not something to be inferred, it is the row that
// shares its key. This is the pairing the whole alignment rests on, and it is
// why a translation can sit under exactly the Pāḷi it renders instead of being
// distributed by guesswork.
type epiLine struct {
	// para and line are ePitaka's own key. They are carried with the text
	// because they are what a translation is filed under: the language files
	// key their rows the same way, so a line's translation is the row that
	// shares its key rather than something inferred from position.
	para  int
	line  int
	words []string
	zh    string
}

// epiPara is one ePitaka paragraph: its Pāḷi as a word list, for locating it in
// this reader's segment stream, and its lines in order.
type epiPara struct {
	words []string
	lines []epiLine
}

// RefSource is one published translation: a language tag and the ePitaka file
// holding it. The Pāḷi side comes from the untranslated database either way.
type RefSource struct {
	Lang string
	Path string
}

// ImportReferenceTranslations aligns each published translation onto this
// reader's segments and stores the result.
//
// The alignment is computed once per language rather than once for all of
// them: each file paragraphs the canon slightly differently, and a segment
// boundary that matches in Chinese may not match in English.
func ImportReferenceTranslations(epitakaPath string, sources []RefSource, g *store.DB, minCoverage float64, log func(string, ...any)) error {
	mapping := parseBookMap()
	log("  book map entries: %d", len(mapping))

	epi, err := openSQLite(epitakaPath)
	if err != nil {
		return err
	}
	defer epi.Close()

	for _, src := range sources {
		log("  --- %s", src.Lang)
		tr, err := openSQLite(src.Path)
		if err != nil {
			return err
		}
		err = importOneTranslation(epi, tr, src.Lang, mapping, g, minCoverage, log)
		tr.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func importOneTranslation(epi, tr *sql.DB, lang string, mapping map[string][]string, g *store.DB, minCoverage float64, log func(string, ...any)) error {

	var books []store.TextBook
	if err := g.Order("sort").Find(&books).Error; err != nil {
		return err
	}

	var total, aligned, skipped int
	for _, b := range books {
		targets := mapping[b.ID]
		if len(targets) == 0 {
			continue
		}
		segs, err := loadSegmentTexts(g, b.ID)
		if err != nil {
			return err
		}
		if len(segs) == 0 {
			continue
		}

		res := alignBook(epi, tr, b.ID, targets, segs)
		// The book is replaced whether or not it clears the bar. Deleting only
		// the books that are written leaves the ones that fall out still
		// holding the previous run's rows — and a bar raised after a change to
		// the aligner would leave translations sitting under sentences that no
		// longer agree with them, which is the one thing this file exists to
		// prevent.
		if err := g.Where("book_id = ? AND lang = ?", b.ID, lang).
			Delete(&store.RefTranslation{}).Error; err != nil {
			return err
		}
		if res.score() < minCoverage || len(res.rows) == 0 {
			skipped++
			// Both numbers, because they say different things: the first is
			// what a reader would see placed, the second is that rate against
			// the most the volume's size allows. Only the second decides.
			log("    %s: %d of %d lines anchored (%.1f%% of the volume); its size allows %.1f%%, so %.0f%% of the ceiling — below the %.0f%% bar, nothing written",
				b.ID, res.anchored, res.lines, 100*res.rate(),
				100*res.reach, 100*res.score(), 100*minCoverage)
			continue
		}
		rows := res.rows
		for i := range rows {
			rows[i].Lang = lang
		}
		if err := g.Clauses(clause.OnConflict{UpdateAll: true}).
			CreateInBatches(rows, 500).Error; err != nil {
			return err
		}
		total += len(rows)
		aligned++
	}
	log("    %s: %d segments across %d books (%d books below the confidence bar)",
		lang, total, aligned, skipped)
	return nil
}

// segText is one of this reader's segments, reduced to what alignment needs.
type segText struct {
	Seq   int
	Para  int
	Words []string
	// Heading is a section title rather than body text. It is aligned like
	// anything else; the flag is here so a caller can tell them apart.
	Heading bool
	// Verse is a line of a gatha. The corpus's verse segments have already been
	// cut at their own line breaks, so an ePitaka verse row lands on the line
	// whose words it holds, one row per line; the flag is here so a caller can
	// tell a verse segment from a body paragraph.
	Verse bool
	// Kind is the corpus's own name for the segment. It is carried because
	// Heading and Verse are not the whole story: a "centered" block is a gatha
	// this edition did not mark as one, and a report that called it prose would
	// be counting a fault that is not there.
	Kind string
}

func loadSegmentTexts(g *store.DB, bookID string) ([]segText, error) {
	var rows []store.TextSegment
	if err := g.Select("seq", "para_no", "kind", "text").
		Where("book_id = ?", bookID).Order("seq").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]segText, 0, len(rows))
	for _, r := range rows {
		w := alignWords(r.Text)
		if len(w) == 0 {
			continue
		}
		// Headings are text like any other and the translation carries them, so
		// they stay in the stream. Leaving them out gave a heading's Chinese no
		// segment to land on and it attached itself to the paragraph before it.
		out = append(out, segText{
			Seq: r.Seq, Para: r.ParaNo, Words: w,
			Heading: r.Kind == "heading", Verse: r.Kind == "verse",
			Kind: r.Kind,
		})
	}
	return out, nil
}

// alignWords tokenises for alignment: letters and digits only, case folded.
//
// This differs from tokenize.Split on purpose. Alignment needs the two editions
// to produce streams of the same shape, and they place the elision apostrophe
// differently — "atthasamhita'nti" against "atthasamhitan ' ti" — which
// splitting on the apostrophe would turn into a length difference.
func alignWords(s string) []string {
	words, _ := alignWordOffsets(s)
	return words
}

// dropVariantReadings removes the bracketed variant readings from an ePitaka
// line, so that its word stream is the length of the reading this reader holds.
//
// CST keeps a variant reading in a <span class="note"> and this reader cuts it
// out of the text; ePitaka writes the same thing as "[nivuyhāmi (syā. kaṃ.
// ka.)]" in the middle of the sentence. Left in, those words are counted as
// words of the line, and everything that walks the line word by word then steps
// past where the next one actually begins — which is how a sentence boundary
// goes missing. Nearly one ePitaka line in fifty carries one.
//
// Only properly nested pairs are removed, innermost first, and an unbalanced
// bracket is left exactly where it is: an editorial bracket CST keeps in the
// body text has to stay in both editions or the two streams disagree in the
// other direction, where the reader's text is the longer one.
func dropVariantReadings(s string) string {
	for {
		// The innermost pair: scanning back, the first '[' that has a ']' after
		// it. Removing innermost first is what makes nesting come out right;
		// an unbalanced '[' never qualifies and is left where it stands.
		open := -1
		for i := len(s) - 1; i >= 0; i-- {
			if s[i] == '[' && strings.IndexByte(s[i:], ']') >= 0 {
				open = i
				break
			}
		}
		if open < 0 {
			return s
		}
		close := strings.IndexByte(s[open:], ']') + open
		s = s[:open] + s[close+1:]
	}
}

// alignWordOffsets is alignWords with the byte offset each word starts at.
//
// The sentence splitter locates a boundary in the word stream and then has to
// turn it back into a place to cut the text; without the offsets that would mean
// tokenising the segment a second time with a second notion of what a word is,
// which is exactly how the two drift apart.
func alignWordOffsets(s string) ([]string, []int) {
	var words []string
	var offs []int
	var cur strings.Builder
	start := -1
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, strings.ToLower(cur.String()))
			offs = append(offs, start)
			cur.Reset()
		}
		start = -1
	}
	for i, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if start < 0 {
				start = i
			}
			cur.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return words, offs
}

// alignBook places one book's ePitaka lines in its segment stream and files
// each line's translation under the segment that owns it.
//
// The unit is the line, not the paragraph, because the line is what the
// translation key identifies: (book_id, para_id, line_id) names a row in the
// Pali file and the same row in the language file. A paragraph is only a
// convenient run of lines.
//
// Nothing is inferred here. A line whose wording was found in this book puts
// its translation on the segment holding that wording, and a line that was not
// found contributes nothing at all. There is no fallback: a translation under
// the wrong sentence is worse than no translation, which is the whole reason
// the search above is built to answer "not found" rather than to guess.
func alignBook(epi, tr *sql.DB, bookID string, targets []string, segs []segText) alignment {
	flat := make([]string, 0, len(segs)*8)
	owner := make([]int, 0, len(segs)*8)
	for i, s := range segs {
		for range s.Words {
			owner = append(owner, i)
		}
		flat = append(flat, s.Words...)
	}
	if len(flat) < anchorGram {
		return alignment{}
	}

	lines := loadEpiLines(epi, tr, targets)
	if len(lines) == 0 {
		return alignment{}
	}
	// The alignment of the whole book against the whole volume. See align.go:
	// a line is placed when the book's text where it falls is the line's own
	// words, and dropped, with the reason recorded, when it is not.
	anchors, stats := alignByDiff(flat, lines)

	bySeg := map[int]*acc{}
	for _, a := range anchors {
		if a.At < 0 || a.At >= len(owner) {
			continue
		}
		zh := cleanTranslation(lines[a.Idx].zh)
		if zh == "" {
			continue
		}
		i := owner[a.At]
		if i < 0 || i >= len(segs) {
			continue
		}
		// One segment can hold several ePitaka lines — a heading running to two
		// rows, a verse whose lines ePitaka keeps apart — and their
		// translations are joined in the order the lines come in.
		entry := bySeg[i]
		if entry == nil {
			entry = &acc{para: segs[i].Para}
			bySeg[i] = entry
		}
		entry.lines = append(entry.lines, zh)
	}

	idx := make([]int, 0, len(bySeg))
	for i := range bySeg {
		idx = append(idx, i)
	}
	sort.Ints(idx)

	out := make([]store.RefTranslation, 0, len(idx))
	for _, i := range idx {
		a := bySeg[i]
		text := strings.Join(a.lines, "")
		if strings.TrimSpace(text) == "" {
			continue
		}
		out = append(out, store.RefTranslation{
			BookID: bookID, Segment: segs[i].Seq, Lang: "zh", Source: "epitaka",
			Text: text, ParaNo: a.para, MatchHow: "text",
			// Lang is overwritten by the caller with the language being
			// imported; the alignment itself does not depend on it.
		})
	}
	return alignment{rows: out, lines: stats.Lines, anchored: stats.Anchored,
		reach: reachOf(len(flat), wordsOf(lines))}
}

// alignment is what one book's alignment produced, in the terms the confidence
// bar is decided on.
type alignment struct {
	rows     []store.RefTranslation
	lines    int // lines the mapped volumes hold
	anchored int // ... that this book's text was found for
	// reach is the share of the volume's lines this book could hold at all,
	// from the two word streams' sizes.
	reach float64
}

// rate is the plain share of the volume's lines that anchored. It is what a
// reader sees, and on its own it is misleading: a volume several times the size
// of the book holds lines that are not the book's text and that no alignment
// can place.
func (a alignment) rate() float64 {
	if a.lines == 0 {
		return 0
	}
	return float64(a.anchored) / float64(a.lines)
}

// score is that rate against what the volume's size allows, and it is what the
// confidence bar is applied to.
//
// Scoring against a hundred per cent instead calls a correct mapping wrong: a
// book fed a volume twenty times its size cannot exceed five per cent however
// perfect the alignment is. That is not hypothetical — it is mula_ku_01 (the
// Khuddakapāṭha, 1,255 words, whose map also lists Sn, 21,000 words that are
// not this book) and attha_sa_05 (the Mahāvagga commentary, 30,660 words,
// whose map also lists Ps-i, 137,710 words that are not this book). Both
// anchored nearly every line of the volume that is theirs and were thrown away
// whole.
//
// A mapping that is genuinely wrong does not score well on this either: there
// is no run of identical text for its lines to anchor on, so nothing is
// anchored whatever the sizes are.
func (a alignment) score() float64 {
	if a.reach <= 0 {
		return 0
	}
	return a.rate() / a.reach
}

// reachOf is the largest share of a volume's lines a book of wordsA words could
// hold, when the volume holds wordsB.
func reachOf(wordsA, wordsB int) float64 {
	if wordsB <= wordsA || wordsA <= 0 {
		return 1
	}
	return float64(wordsA) / float64(wordsB)
}

// wordsOf is the volume's own size, in words.
func wordsOf(lines []epiLine) int {
	n := 0
	for i := range lines {
		n += len(lines[i].words)
	}
	return n
}

// wordIndex keys every run of minShingle..shingle words in a book's stream, so
// an ePitaka paragraph can be looked up by its opening words.//
// The index has to hold every anchor length locate() will ask for. It used to
// hold only the six-word keys while the search fell back through five and four
// — so the fallback could never hit, and a paragraph of five words (a verse,
// most often) could not be anchored at all and lost its translation. Short
// paragraphs are exactly the ones that need the shorter anchor.
func wordIndex(flat []string) map[string][]int {
	index := make(map[string][]int, len(flat)*3)
	for i := range flat {
		for n := minShingle; n <= shingle && i+n <= len(flat); n++ {
			key := strings.Join(flat[i:i+n], " ")
			index[key] = append(index[key], i)
		}
	}
	return index
}

// locateParas walks every ePitaka paragraph of a book through the word stream
// and hands each paragraph that anchored to visit, with the span it occupies.
//
// The importer no longer walks this way: it anchors whole lines through the
// whole book (see anchors.go), because a forward walk bounded by a paragraph
// span loses lines for reasons that have nothing to do with the text. This, and
// wordIndex and matchLine with it, are kept because cmd/refdiag measures the
// two against each other — which is how the replacement was justified. They are
// not on the import path any more.
//
// Two callers need the same walk and differ only in what they do with the span:
// the reference importer drops each line's translation on the segment that owns
// it, and the sentence splitter records where each line begins. Sharing the walk
// keeps one notion of where a paragraph sits — a second implementation would be
// a second chance to disagree with the translations.
func locateParas(epi, tr *sql.DB, targets []string, flat []string, index map[string][]int,
	visit func(pos, end int, lines []epiLine)) (paras, anchored int) {
	cursor := 0
	// budget is how far the next anchor is allowed to sit from the cursor. It
	// grows with each miss, so a paragraph that genuinely differs costs one
	// paragraph rather than stranding everything after it.
	budget := 0
	for _, book := range targets {
		for _, p := range loadEParas(epi, tr, book) {
			paras++
			pos, ok := locate(flat, index, p.words, cursor, len(flat), budget)
			if !ok {
				budget += len(p.words)
				continue
			}
			anchored++
			budget = 0
			cursor = pos

			end := pos + len(p.words)
			if end > len(flat) {
				end = len(flat)
			}
			visit(pos, end, p.lines)
		}
	}
	return paras, anchored
}

// acc is the translation accumulating under one segment.
type acc struct {
	para  int
	lines []string
}

// matchLine finds where a line sits in flat, searching from `from` and not past
// `limit`. It compares the longest prefix it can, so a line whose tail differs
// still lands on its head.
func matchLine(flat, words []string, from, limit int) int {
	if from < 0 {
		from = 0
	}
	if limit > len(flat) {
		limit = len(flat)
	}
	for n := 4; n >= 2; n-- {
		if len(words) < n {
			continue
		}
		head := words[:n]
		for i := from; i+n <= limit; i++ {
			ok := true
			for j := range head {
				if flat[i+j] != head[j] {
					ok = false
					break
				}
			}
			if ok {
				return i
			}
		}
	}
	return -1
}

// splitSentences cuts a translation into sentences on the punctuation that ends
// one in Chinese and in English, keeping the mark with the sentence.
func splitSentences(s string) []string {
	var out []string
	var cur strings.Builder
	for _, r := range s {
		cur.WriteRune(r)
		switch r {
		case '\u3002', '\uff01', '\uff1f', '\uff1b', '\u2026', '.', '!', '?', ';':
			// A full stop inside a number ("2. 3") is not a sentence end; the
			// next rune decides.
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
	}
	if rest := strings.TrimSpace(cur.String()); rest != "" {
		out = append(out, rest)
	}
	// A closing quote or bracket that follows the full stop belongs to the
	// sentence that just ended, not to the one starting after it: cutting at
	// "。" would otherwise leave "”然后离开。" as a sentence of its own and the
	// reader would see a quotation mark open the next line.
	merged := make([]string, 0, len(out))
	for _, x := range out {
		if x == "" {
			continue
		}
		if len(merged) > 0 {
			if lead := leadingClosers(x); lead > 0 {
				merged[len(merged)-1] += x[:lead]
				x = x[lead:]
			}
		}
		if x == "" {
			continue
		}
		// A fragment with no letters at all is punctuation on its own; it
		// attaches to whatever came before.
		if !hasLetters(x) {
			if len(merged) > 0 {
				merged[len(merged)-1] += x
			}
			continue
		}
		merged = append(merged, x)
	}
	return merged
}

// leadingClosers counts the closing punctuation at the start of s.
func leadingClosers(s string) int {
	n := 0
	for _, r := range s {
		switch r {
		case '\u201d', '\u2019', '\u300d', '\u300f', '\u3011', '\u3009', '\u300b', ')', ']', '\u00bb':
			n += len(string(r))
		default:
			return n
		}
	}
	return n
}

func hasLetters(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// locate finds where a paragraph sits in the segment stream.
//
// Three things make this robust on real text. The anchor is tried at several
// shingle lengths, because a paragraph whose opening words differ between the
// two editions may still match from its third word on. Candidates are chosen by
// distance from the cursor rather than by taking the first hit, so a stock
// formula that also occurs a thousand words away cannot pull the alignment off
// course. And the accepted distance grows with the drift budget, so a paragraph
// that genuinely failed to anchor does not take the rest of the book with it.
func locate(flat []string, index map[string][]int, words []string, cursor, limit, budget int) (int, bool) {
	// A paragraph shorter than the shortest anchor — a heading, most often —
	// cannot be located by shingle. It is still worth looking for it next to
	// the cursor rather than assuming it sits exactly there: the heading
	// "2. Nimokkhasuttaṃ" would otherwise be placed inside the paragraph
	// before it and its translation would appear under the wrong text.
	if len(words) < minShingle {
		lo := cursor - 60
		if lo < 0 {
			lo = 0
		}
		hi := cursor + 60
		if hi > limit {
			hi = limit
		}
		if at := matchLine(flat, words, lo, hi); at >= 0 {
			return at, true
		}
		if cursor > 0 && cursor < limit {
			return cursor, true
		}
		return 0, false
	}

	window := 400
	if budget > window {
		window = budget
	}
	if window > 4000 {
		window = 4000
	}

	best, bestDist := -1, 1<<30
	consider := func(from int) {
		for n := shingle; n >= minShingle; n-- {
			if from+n > len(words) {
				continue
			}
			for _, c := range index[strings.Join(words[from:from+n], " ")] {
				d := c - cursor
				if d < 0 {
					d = -d
				}
				if d < bestDist {
					bestDist, best = d, c
				}
			}
			if best >= 0 {
				return
			}
		}
	}
	consider(0)
	if best < 0 {
		// The opening may be a variant reading or an editorial insertion the
		// other edition lacks. Try a little further in before giving up.
		for from := 1; from < len(words)-4 && from < 12; from++ {
			consider(from)
			if best >= 0 {
				break
			}
		}
	}
	if best < 0 || bestDist > window {
		return 0, false
	}
	if best >= limit {
		best = limit - 1
	}
	return best, true
}

// loadEParas reads one ePitaka book's paragraphs, keeping each line paired
// with its own translation.
//
// The two files are keyed the same way, so the pairing is a join rather than an
// inference: (book_id, para_id, line_id) identifies a line in both. Reading
// them separately and appending the translations in order — which is what this
// did — threw that key away and left the Chinese to be distributed by guesswork
// over the paragraph.
//
// tr may be nil, which is the sentence splitter's case: it wants the Pāḷi lines
// only, and there is no second file to join them to.
func loadEParas(epi, tr *sql.DB, book string) []epiPara {
	type key struct {
		para int
		line int
	}
	zh := map[key]string{}
	if tr != nil {
		zrows, err := tr.Query(`SELECT para_id, line_id, COALESCE(translation,'') FROM sentences
			WHERE book_id = ?`, book)
		if err == nil {
			for zrows.Next() {
				var k key
				var t string
				if err := zrows.Scan(&k.para, &k.line, &t); err != nil {
					break
				}
				zh[k] = t
			}
			zrows.Close()
		}
	}

	rows, err := epi.Query(`SELECT para_id, line_id, COALESCE(pali,'') FROM sentences
		WHERE book_id = ? ORDER BY para_id, line_id`, book)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []epiPara
	byID := map[int]int{}
	for rows.Next() {
		var pid, lid int
		var pali string
		if err := rows.Scan(&pid, &lid, &pali); err != nil {
			return out
		}
		i, ok := byID[pid]
		if !ok {
			out = append(out, epiPara{})
			i = len(out) - 1
			byID[pid] = i
		}
		w := alignWords(dropVariantReadings(cleanInlineTags(pali)))
		out[i].words = append(out[i].words, w...)
		out[i].lines = append(out[i].lines, epiLine{
			para: pid, line: lid, words: w, zh: zh[key{pid, lid}],
		})
	}
	return out
}

// loadEpiLines reads a book's ePitaka lines in reading order, flattened out of
// their paragraphs.
//
// The aligner works on lines rather than paragraphs because the line is the
// unit the translation is filed under: (book_id, para_id, line_id) keys a row
// in the Pali file and the same row in a language file. A paragraph is only a
// convenient run of them.
func loadEpiLines(epi, tr *sql.DB, targets []string) []epiLine {
	var out []epiLine
	for _, book := range targets {
		for _, p := range loadEParas(epi, tr, book) {
			out = append(out, p.lines...)
		}
	}
	return out
}

// lineCount is how many ePitaka lines a book's mapped volumes hold, for the
// message that says a book was passed over: a share of nothing would not say
// whether the alignment failed or the book is simply larger than its volumes.
func lineCount(epi *sql.DB, targets []string) int {
	n := 0
	for _, book := range targets {
		var c int
		if err := epi.QueryRow(`SELECT count(*) FROM sentences WHERE book_id = ?`, book).Scan(&c); err == nil {
			n += c
		}
	}
	return n
}

// inlineTag matches the markup ePitaka keeps inside its text.
var inlineTag = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)

// cleanInlineTags removes the inline markup from an ePitaka row.
//
// The markup is not confined to the translations: 366,473 of ePitaka's 1.28
// million Pāḷi rows — 28.6%, nearly all of them in the commentarial literature
// — wrap a headword in `<b>`, which is the same thing this reader's CST pages
// write as `<span class="bld">`, and which ParseBook cuts out of the text and
// records in the bold column. Left in, the tag is not markup to a word splitter
// but two more words: `b tatthā b ti`, where this book has `tatthā ti`. Every
// line that begins at a headword then begins, as far as the alignment can see,
// in text the book does not have — and it is precisely the commentary's
// headwords that begin its sentences. Measured on tika_vi_06 (Vmv-ii..v):
// stripping the tags is worth thirty points of placed lines.
func cleanInlineTags(s string) string {
	if !strings.ContainsRune(s, '<') {
		return s
	}
	return inlineTag.ReplaceAllString(s, "")
}

// cleanTranslation removes the inline markup from a published translation.
//
// Nearly a third of the Chinese rows and a quarter of the English ones carry
// `<i>` and `<b>` — 819,000 and 780,000 of them — because ePitaka writes its
// translations as HTML and the app that consumes them renders that HTML. This
// reader renders a translation as text, so the tags would arrive on the page as
// literal `<i>Sangha</i>`, which is worse than losing the italics. Nothing else
// is done here: whitespace is left alone, because a translation's own spacing
// is part of it, and an unknown tag is removed rather than interpreted. The
// emphasis itself is worth having, and restoring it as markup is a change to
// the reader's rendering rather than something to smuggle through as text.
func cleanTranslation(s string) string {
	return strings.TrimSpace(cleanInlineTags(s))
}
