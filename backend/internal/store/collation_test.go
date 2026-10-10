package store

import (
	"reflect"
	"strings"
	"testing"
)

// The dictionary key columns must be binary-collated.
//
// MySQL's default utf8mb4_general_ci folds many accented Latin letters onto
// their base letter, so "buḍḍhassa" and "buddhassa" compare equal and collide
// on a primary key. That is not a cosmetic difference in Pāḷi — ḍ ṭ ṇ ḷ ṃ are
// letters — and it silently replaced the dictionary entry for the Buddha with
// the adjective "aged". This test exists because the failure mode is invisible:
// the import succeeds, the row count looks right, and one word quietly answers
// for another.
func TestDictionaryKeyColumnsAreBinaryCollated(t *testing.T) {
	cases := []struct {
		model  any
		field  string
		column string
	}{
		{&DictLookup{}, "Key", "lookup_key"},
		{&DictHeadword{}, "Lemma1", "lemma_1"},
		{&DictHeadword{}, "Lemma2", "lemma_2"},
		{&DictTemplate{}, "Pattern", "pattern"},
		{&DictEntry{}, "Word", "word"},
		{&DictEnEntry{}, "Word", "word"},
		{&WordFreq{}, "Word", "word"},
		{&VocabItem{}, "Lemma", "lemma"},
	}

	for _, c := range cases {
		field, ok := reflect.TypeOf(c.model).Elem().FieldByName(c.field)
		if !ok {
			t.Errorf("%T has no field %s", c.model, c.field)
			continue
		}
		tag := field.Tag.Get("gorm")
		if !strings.Contains(tag, "COLLATE utf8mb4_bin") {
			t.Errorf("%T.%s (%s) must be declared COLLATE utf8mb4_bin, tag is %q",
				c.model, c.field, c.column, tag)
		}
	}
}

// The corpus anchor columns are not word keys, so they stay on the default
// collation: comparisons there are on identifiers and sequences, and a binary
// collation would only make the indexes larger.
func TestBookAndSegmentKeysAreNotBinary(t *testing.T) {
	field, ok := reflect.TypeOf(TextSegment{}).FieldByName("BookID")
	if !ok {
		t.Fatal("TextSegment has no BookID")
	}
	if strings.Contains(field.Tag.Get("gorm"), "collate") {
		t.Error("BookID holds an ASCII identifier and does not need a binary collation")
	}
}
