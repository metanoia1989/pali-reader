// Package store owns the database schema. Models are the single source of
// truth: change them and AutoMigrate brings MySQL in line on the next start.
package store

import (
	"hash/fnv"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Canon text
//
// The corpus is read-only for the application: it is produced by
// cmd/importer from the upstream sources and never written at runtime.
// ---------------------------------------------------------------------------

// Basket groups books into the four divisions of the canon.
const (
	BasketMula  = "mula"  // 根本 — the canon itself
	BasketAttha = "attha" // 义注 — aṭṭhakathā commentaries
	BasketTika  = "tika"  // 复注 — ṭīkā sub-commentaries
	BasketAnnya = "annya" // 藏外 — other (grammar, chronicles, ...)
)

// BasketLabel maps a basket code to the label shown in the interface.
var BasketLabel = map[string]string{
	BasketMula:  "根本三藏",
	BasketAttha: "义注",
	BasketTika:  "复注",
	BasketAnnya: "藏外",
}

// BasketPali is the same label as the canon writes it. A reader who works in
// Pāḷi scans for "Aṭṭhakathā", not "义注", and the interface has a setting for
// which of the two leads — so both have to exist.
var BasketPali = map[string]string{
	BasketMula:  "Mūlasāsana",
	BasketAttha: "Aṭṭhakathā",
	BasketTika:  "Ṭīkā",
	BasketAnnya: "Pakiṇṇaka",
}

// TextCategory is a division inside a basket: a nikāya, a piṭaka, a collection.
type TextCategory struct {
	ID string `gorm:"primaryKey;size:32" json:"id"`
	// Basket is the source's own grouping ("tri" / "annya"), which does not
	// match the basket a book carries ("mula" / "attha" / ...). Grouping is
	// therefore done on the book's basket, and this column is left as the
	// source wrote it.
	Basket string `gorm:"size:16;index" json:"-"`
	Name   string `gorm:"size:191" json:"name"`
	// NamePi is the division as a reader would cite it — "Dīghanikāya", not the
	// source's file grouping "suttantapiṭaka (dīghanikāya)".
	NamePi string `gorm:"size:191" json:"namePi"`
	NameZh string `gorm:"size:191" json:"nameZh"`
	Sort   int    `gorm:"index" json:"-"`
}

func (TextCategory) TableName() string { return "text_categories" }

// TextBook is one volume of the canon.
type TextBook struct {
	ID        string `gorm:"primaryKey;size:32"`
	Basket    string `gorm:"size:16;index"`
	Category  string `gorm:"size:32;index"`
	Name      string `gorm:"size:191"`
	NameZh    string `gorm:"type:text"`
	PageCount int
	SegCount  int
	Sort      int `gorm:"index"`
}

func (TextBook) TableName() string { return "text_books" }

// Segment kinds. The reader styles each kind differently and only prose and
// verse are annotatable.
const (
	KindProse   = "prose"   // bodytext, noindentbodytext, unindented, indent
	KindVerse   = "verse"   // a run of gatha lines
	KindHeading = "heading" // nikaya, book, chapter, title, subhead, subsubhead
	KindCenter  = "center"  // centred lines: homage, colophons
)

// TextSegment is one readable block: a paragraph, a verse, or a heading.
//
// Seq is the ordinal within the book and is what every anchor hangs off —
// user annotations, translations and the reader's scroll position all point at
// (BookID, Seq), never at a database row id, so re-importing the corpus does
// not detach anybody's work.
type TextSegment struct {
	ID     uint   `gorm:"primaryKey"`
	BookID string `gorm:"size:32;uniqueIndex:uk_segment,priority:1"`
	Seq    int    `gorm:"uniqueIndex:uk_segment,priority:2"`
	ParaNo int    `gorm:"index"` // CST paragraph number, inherited from anchors
	PageNo int
	Kind   string `gorm:"size:16"`
	// Text is the plain reading text: no markup, variant readings removed.
	Text string `gorm:"type:mediumtext"`
	// HTML keeps inline markup: <span class="v"> variant readings,
	// <span class="bld"> commentary emphases, and verse line breaks.
	HTML string `gorm:"type:mediumtext"`
	// Markers records the edition page anchors inside this block as a compact
	// JSON array of "M1.0010" style strings, empty when there are none.
	Markers string `gorm:"type:text"`
	// Variants holds the CST variant readings that were cut out of Text, as
	// [[utf16 offset, reading], ...]. The offset is in UTF-16 units because the
	// client slices the text with it. Kept rather than dropped: the reader can
	// turn variant readings on, and they belong at the word they annotate.
	Variants string `gorm:"type:text"`
	// Bold holds the runs the edition sets in bold as [[utf16Offset, length], …].
	// The markup is stripped from Text, so this is what keeps the commentary's
	// headwords emphasised.
	Bold    string `gorm:"type:text"`
	CharLen int
	// Tokens is the cached tokenisation as JSON: [[offset, length, flags], ...].
	// Filled by the importer so the reader never tokenises at request time.
	Tokens string `gorm:"type:mediumtext"`
	// Head is true for the first segment of a TOC section, and SectionID names
	// that section, so the reader can jump and the scrollspy can resolve.
	SectionID  string `gorm:"size:64;index"`
	SectionLvl int
}

func (TextSegment) TableName() string { return "text_segments" }

// TextTOC is one entry of a book's table of contents.
type TextTOC struct {
	ID     uint   `gorm:"primaryKey"`
	BookID string `gorm:"size:32;uniqueIndex:uk_toc,priority:1"`
	Seq    int    `gorm:"uniqueIndex:uk_toc,priority:2"`
	Name   string `gorm:"type:text"`
	Level  int
	Kind   string `gorm:"size:16"`
	ParaNo int
	PageNo int
	// SegmentSeq points at the segment this entry should scroll to.
	SegmentSeq int `gorm:"index"`
}

func (TextTOC) TableName() string { return "text_toc" }

// The word-key column type, spelled out at each column that needs it:
//
//	varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin
//
// 191 characters is the widest a utf8mb4 column can be and still fit inside
// MySQL 5.7's index limit on the default row format.
//
// ---------------------------------------------------------------------------
// Dictionary
//
// Collation matters here more than anywhere else in the schema. MySQL's default
// utf8mb4_general_ci folds many accented Latin letters onto their base letter,
// so "buḍḍhassa" and "buddhassa" compare equal and collide on a primary key —
// which silently replaced the Buddha's entry with the adjective "aged". In Pāḷi
// ḍ ṭ ṇ ḷ ṃ are distinct letters, not decoration.
//
// The collation is declared inside `type:` rather than with a `collate:` tag
// because GORM 1.25 and its MySQL driver do not implement that tag: it parses
// without complaint and is then dropped, leaving the column on the database
// default. Spelling the type out is verbose and is the only form that works.
//
// binaryText is that type. Use it for every column that keys a word.
// ---------------------------------------------------------------------------

// DictHeadword is one DPD entry.
type DictHeadword struct {
	ID           uint   `gorm:"primaryKey"`
	Lemma1       string `gorm:"column:lemma_1;type:varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;index"`
	Lemma2       string `gorm:"column:lemma_2;type:varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"`
	POS          string `gorm:"size:48;index"`
	Grammar      string `gorm:"size:191"`
	Meaning1     string `gorm:"column:meaning_1;type:text"`
	MeaningLit   string `gorm:"type:text"`
	Meaning2     string `gorm:"column:meaning_2;type:text"`
	Construction string `gorm:"type:text"`
	// CompoundConstruction is DPD's own split of a compound headword, e.g.
	// "dhamma + cakka". The deconstructor column on DictLookup holds every
	// alternative DPD generated; this holds the canonical one.
	CompoundConstruction string `gorm:"type:text"`
	CompoundType         string `gorm:"size:128"`
	Stem                 string `gorm:"size:191"`
	Pattern              string `gorm:"size:64;index;type:varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"`
	RootKey              string `gorm:"size:64;index"`
	RootSign             string `gorm:"size:64"`
	RootBase             string `gorm:"size:191"`
	FamilyRoot           string `gorm:"size:191"`
	FamilyWord           string `gorm:"size:191"`
	FamilyCompound       string `gorm:"size:191"`
	FamilyIdiom          string `gorm:"size:191"`
	FamilySet            string `gorm:"size:191"`
	Derivative           string `gorm:"size:64"`
	Suffix               string `gorm:"size:64"`
	Phonetic             string `gorm:"size:191"`
	Sanskrit             string `gorm:"size:191"`
	Cognate              string `gorm:"size:191"`
	Antonym              string `gorm:"size:191"`
	Synonym              string `gorm:"type:text"`
	Variant              string `gorm:"type:text"`
	Notes                string `gorm:"type:text"`
	// Inflections is the comma separated list of every form DPD generated.
	Inflections string `gorm:"type:mediumtext"`
	EBTCount    int
	FreqRank    int
}

func (DictHeadword) TableName() string { return "dict_headwords" }

// DictLookup maps one inflected form to the headwords it can belong to.
// It is the hot table: every tap on a word queries it by primary key.
type DictLookup struct {
	Key string `gorm:"primaryKey;column:lookup_key;type:varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"`
	// HeadwordIDs is a JSON array of DictHeadword ids, most likely first.
	HeadwordIDs string `gorm:"type:text"`
	// Deconstructor is a JSON array of alternative splits, each a string of
	// "part + part + part".
	Deconstructor string `gorm:"type:text"`
	// Grammar is a JSON array of {pos, grammar, lemma} analyses.
	Grammar string `gorm:"type:text"`
	// Spelling is a JSON array of spelling corrections for near misses.
	Spelling string `gorm:"size:191"`
	// See is a JSON array of "see X" redirections.
	See string `gorm:"size:191"`
	// Variant is a JSON array of variant spellings that resolve here.
	Variant string `gorm:"type:text"`
	// Roots is a JSON array of root keys.
	Roots string `gorm:"type:text"`
}

func (DictLookup) TableName() string { return "dict_lookup" }

// DictTemplate is a DPD inflection grid. Data is the JSON matrix DPD ships:
// a header row followed by one row per case.
type DictTemplate struct {
	Pattern string `gorm:"primaryKey;type:varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"`
	Like    string `gorm:"size:64"`
	Data    string `gorm:"type:mediumtext"`
}

func (DictTemplate) TableName() string { return "dict_templates" }

// DictRoot is a Pāḷi verbal root.
type DictRoot struct {
	Root         string `gorm:"primaryKey;type:varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin" json:"root"`
	RootMeaning  string `gorm:"size:191" json:"meaning"`
	RootSign     string `gorm:"size:32" json:"sign"`
	RootGroup    int    `json:"group"`
	RootInComps  string `gorm:"size:48" json:"inComps"`
	SanskritRoot string `gorm:"size:64" json:"sanskrit"`
	Note         string `gorm:"type:text" json:"note,omitempty"`
}

func (DictRoot) TableName() string { return "dict_roots" }

// DictEntry is one meaning line from a non-DPD dictionary. These are the
// Chinese and English dictionaries that sit alongside DPD in the lookup panel.
type DictEntry struct {
	ID     uint   `gorm:"primaryKey"`
	Word   string `gorm:"uniqueIndex:uk_entry,priority:1;type:varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"`
	Dict   string `gorm:"size:48;uniqueIndex:uk_entry,priority:2"`
	Body   string `gorm:"type:text"`
	Source int    `gorm:"uniqueIndex:uk_entry,priority:3"`
}

func (DictEntry) TableName() string { return "dict_entries" }

// DictSource describes one of those dictionaries for the interface.
type DictSource struct {
	Code    string `gorm:"primaryKey;size:48"`
	Name    string `gorm:"size:191"`
	Lang    string `gorm:"size:8"`
	License string `gorm:"size:191"`
	Sort    int
	Entries int
}

func (DictSource) TableName() string { return "dict_sources" }

// DictEnEntry is one English headword with the meanings ECDICT gives it. It is
// the dictionary the English 参考译文 is read against, and it is a SECOND
// dictionary, not part of the Pāḷi one: it shares no key, no lookup and no
// meaning with dict_headwords, and merging the two would let an English word
// answer for a Pāḷi form that happens to be spelled the same way.
//
// Word is the lookup key and is stored lowercased. The column is binary
// collated (铁律 1), so unlike the source project — which leaned on MySQL's
// utf8mb4_general_ci to fold case — the case folding has to happen before the
// row is written. Head keeps the spelling the dictionary itself uses, so the
// popup can show "Aachen" rather than the key it was filed under.
//
// Senses holds [{"pos":"n.","def":"…"}] as JSON. It is text rather than rows
// because it is only ever read whole, for one headword, and never queried into.
type DictEnEntry struct {
	Word     string `gorm:"primaryKey;column:word;type:varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"`
	Head     string `gorm:"size:64"`
	Phonetic string `gorm:"size:191"`
	Senses   string `gorm:"type:text;not null"`
}

// TableName is declared rather than inferred. GORM pluralises, and a table
// whose name is produced by an inflector is a table whose name changes when
// somebody renames the struct — the source project lost "dictionary" to
// "dictionaries" this way. store_test.go asserts that the declared name is the
// one GORM would infer, so the two cannot drift apart unnoticed.
func (DictEnEntry) TableName() string { return "dict_en_entries" }

// ---------------------------------------------------------------------------
// Reference translations
// ---------------------------------------------------------------------------

// RefTranslation is a published translation of one segment, imported from
// ePitaka. The reader shows it as 参考译文 and never edits it.
type RefTranslation struct {
	ID       uint   `gorm:"primaryKey"`
	BookID   string `gorm:"size:32;uniqueIndex:uk_ref,priority:1"`
	Segment  int    `gorm:"uniqueIndex:uk_ref,priority:2"`
	Lang     string `gorm:"size:8;uniqueIndex:uk_ref,priority:3"`
	Source   string `gorm:"size:48;uniqueIndex:uk_ref,priority:4"`
	Text     string `gorm:"type:mediumtext"`
	ParaNo   int
	MatchHow string `gorm:"size:16"` // how it was aligned: para, seq, none
}

func (RefTranslation) TableName() string { return "ref_translations" }

// ---------------------------------------------------------------------------
// Users and their work
// ---------------------------------------------------------------------------

// User is an account. Email verification is simulated in this build: the code
// is returned to the client instead of being mailed.
type User struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Email       string `gorm:"size:191;uniqueIndex" json:"email"`
	DisplayName string `gorm:"size:64" json:"displayName"`
	PassHash    string `gorm:"size:191" json:"-"`
	// Settings is the reader's 阅读设置, stored as the JSON the client wrote.
	// Opaque here on purpose: which options exist is a presentation concern
	// that changes often, and a migration for every new toggle would be absurd
	// for a handful of bytes.
	Settings   string    `gorm:"type:text" json:"settings,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

func (User) TableName() string { return "users" }

// PendingRegistration holds a signup waiting for its code.
type PendingRegistration struct {
	Email     string `gorm:"primaryKey;size:191"`
	Code      string `gorm:"size:12"`
	PassHash  string `gorm:"size:191"`
	Name      string `gorm:"size:64"`
	Attempts  int
	ExpiresAt time.Time
	CreatedAt time.Time
}

func (PendingRegistration) TableName() string { return "pending_registrations" }

// Session is a bearer token.
type Session struct {
	Token     string `gorm:"primaryKey;size:64"`
	UserID    uint   `gorm:"index"`
	ExpiresAt time.Time
	CreatedAt time.Time
}

func (Session) TableName() string { return "sessions" }

// WordPick is one thing the reader asserted about one word occurrence.
//
// A Pāḷi surface form is genuinely ambiguous. "dhammā" can be nominative or
// accusative, plural, of more than one headword, and a reader working through a
// sentence records the candidates as they go and strikes them out as the
// sentence resolves. One row is one such assertion: a grammar reading, a
// meaning, or the compound split they settled on.
//
// The anchor is (BookID, Segment, WordIndex) — the Nth token of the Nth segment.
// WordIndex is stable because the tokenisation is frozen in the corpus at import
// time; changing it would be a corpus change, and the reader flags a stale
// anchor rather than silently moving it.
type WordPick struct {
	ID      uint   `gorm:"primaryKey"`
	UserID  uint   `gorm:"uniqueIndex:uk_pick,priority:1"`
	BookID  string `gorm:"size:32;uniqueIndex:uk_pick,priority:2"`
	Segment int    `gorm:"uniqueIndex:uk_pick,priority:3"`
	// WordIndex < 0 marks an assertion about the whole segment rather than one word.
	WordIndex int `gorm:"uniqueIndex:uk_pick,priority:4"`
	// Key is the canonical identity of the pick. It exists so that tapping the
	// same row twice is idempotent without the unique index having to span every
	// column — a meaning is free text and would blow past the index limit.
	Key string `gorm:"size:160;uniqueIndex:uk_pick,priority:5"`

	// Kind is "grammar", "meaning" or "split".
	Kind    string `gorm:"size:16;index"`
	Surface string `gorm:"size:191"`
	Lemma   string `gorm:"size:191"`
	LemmaID uint

	// A grammar pick: one row of DPD's analysis table.
	POS    string `gorm:"size:64"`
	Gender string `gorm:"size:24"`
	Case   string `gorm:"size:24"`
	Number string `gorm:"size:24"`
	// Grammar is DPD's own wording, kept for display when the parts are empty.
	Grammar string `gorm:"size:191"`

	// A meaning pick.
	Meaning       string `gorm:"type:text"`
	MeaningSource string `gorm:"size:64"`

	// A split pick: the compound decomposition, as "+" joined parts.
	Split string `gorm:"size:255"`

	// Note lets a reader gloss a word the dictionary does not cover.
	Note      string `gorm:"type:text"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (WordPick) TableName() string { return "word_picks" }

// retiredWordChoice exists only so AutoMigrate drops the table it names. The
// shape of a word annotation changed from one decision per word to any number
// of them, and an old row cannot be reinterpreted as the new thing.
type retiredWordChoice struct{}

func (retiredWordChoice) TableName() string { return "word_choices" }

// The kinds of assertion a reader can make about a word.
const (
	PickGrammar = "grammar"
	PickMeaning = "meaning"
	PickSplit   = "split"
)

// PickKey builds the canonical identity of a pick. Two taps that mean the same
// thing have to produce the same key, and two that do not must not collide.
func PickKey(kind string, parts ...string) string {
	h := fnv.New64a()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	tag := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) > 24 {
			p = p[:24]
		}
		tag = append(tag, p)
	}
	return kind + "|" + strings.Join(tag, "|") + "|" + strconv.FormatUint(h.Sum64(), 36)
}

// Annotation kinds.
const (
	NoteSegment = "segment" // a remark on the whole segment
	NoteWord    = "word"    // a remark pinned to one word
)

// Note is a reader's own commentary.
type Note struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"index:idx_note_lookup,priority:1"`
	BookID    string `gorm:"size:32;index:idx_note_lookup,priority:2"`
	Segment   int    `gorm:"index:idx_note_lookup,priority:3"`
	WordIndex int    `gorm:"index:idx_note_lookup,priority:4"`
	Kind      string `gorm:"size:16"`
	Body      string `gorm:"type:text"`
	Quote     string `gorm:"size:255"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Note) TableName() string { return "notes" }

// Translation is the reader's own rendering of a segment (WordIndex = -1) or
// of one sentence inside it.
type Translation struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"uniqueIndex:uk_trans,priority:1"`
	BookID    string `gorm:"size:32;uniqueIndex:uk_trans,priority:2"`
	Segment   int    `gorm:"uniqueIndex:uk_trans,priority:3"`
	Text      string `gorm:"type:text"`
	UpdatedAt time.Time
	CreatedAt time.Time
}

func (Translation) TableName() string { return "user_translations" }

// Progress is where a reader left off in a book.
type Progress struct {
	UserID    uint      `gorm:"primaryKey" json:"-"`
	BookID    string    `gorm:"primaryKey;size:32" json:"bookId"`
	Segment   int       `gorm:"index" json:"segment"`
	UpdatedAt time.Time `gorm:"index" json:"updatedAt"`
}

func (Progress) TableName() string { return "reading_progress" }

// Bookmark is a saved position, optionally named.
type Bookmark struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index:idx_bm,priority:1" json:"-"`
	BookID    string    `gorm:"size:32;index:idx_bm,priority:2" json:"bookId"`
	Segment   int       `json:"segment"`
	ParaNo    int       `json:"paraNo"`
	Label     string    `gorm:"size:191" json:"label"`
	CreatedAt time.Time `json:"createdAt"`
}

func (Bookmark) TableName() string { return "bookmarks" }

// VocabItem is a word the reader decided to keep.
type VocabItem struct {
	ID       uint      `gorm:"primaryKey" json:"id"`
	UserID   uint      `gorm:"uniqueIndex:uk_vocab,priority:1" json:"-"`
	Lemma    string    `gorm:"uniqueIndex:uk_vocab,priority:2;type:varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin" json:"lemma"`
	LemmaID  uint      `json:"lemmaId"`
	POS      string    `gorm:"size:64" json:"pos"`
	Meaning  string    `gorm:"type:text" json:"meaning"`
	Note     string    `gorm:"type:text" json:"note"`
	Seen     int       `json:"seen"`
	AddedAt  time.Time `json:"addedAt"`
	LastSeen time.Time `json:"lastSeen"`
}

func (VocabItem) TableName() string { return "vocab_items" }

// ---------------------------------------------------------------------------
// Corpus statistics used by the home page and the reader's word hints
// ---------------------------------------------------------------------------

// WordFreq is a corpus-wide frequency table, used to mark rare words and to
// drive the "new words on this page" list.
type WordFreq struct {
	Word  string `gorm:"primaryKey;type:varchar(96) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"`
	Count int    `gorm:"index"`
	Rank  int    `gorm:"index"`
}

func (WordFreq) TableName() string { return "word_freq" }
