package store

import (
	"reflect"
	"testing"

	"gorm.io/gorm/schema"
)

// A model's table name must be the one we meant, and it must be the one GORM
// would have produced on its own.
//
// Both halves matter. GORM pluralises by inflection, so a struct named after a
// noun with an irregular plural quietly lands in a differently named table
// than the one the drop list and the importer name in SQL — and the failure is
// a table that is never found, not an error at the point of the mistake. The
// source project lost "dictionary" to "dictionaries" exactly this way.
// TableName() pins the name; this test stops the pin and the struct from
// drifting apart, because renaming the struct moves the inferred name while
// the declaration stays put.
func TestDeclaredTableNamesMatchWhatGormWouldInfer(t *testing.T) {
	cases := []struct {
		model any
		want  string
	}{
		{&DictEnEntry{}, "dict_en_entries"},
		{&DictEntry{}, "dict_entries"},
		{&DictSource{}, "dict_sources"},
		{&DictHeadword{}, "dict_headwords"},
		{&RefTranslation{}, "ref_translations"},
	}

	ns := schema.NamingStrategy{}
	for _, c := range cases {
		named, ok := c.model.(interface{ TableName() string })
		if !ok {
			t.Errorf("%T declares no TableName()", c.model)
			continue
		}
		if got := named.TableName(); got != c.want {
			t.Errorf("%T.TableName() = %q, want %q", c.model, got, c.want)
			continue
		}
		name := reflect.TypeOf(c.model).Elem().Name()
		if inferred := ns.TableName(name); inferred != c.want {
			t.Errorf("%s would be inferred as table %q by GORM but is declared %q",
				name, inferred, c.want)
		}
	}
}
