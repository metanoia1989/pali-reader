package store

import (
	"encoding/json"
	"testing"
	"time"
)

// Models that are serialised straight to the client must not rely on Go's
// default field names.
//
// store.User went out as {"DisplayName": ...} while the client read
// user.displayName, so a signed-in reader saw the fallback avatar and no name.
// Nothing errored — the field was simply always undefined — which is exactly
// why this is worth a test rather than a comment.
func TestUserSerialisesAsCamelCase(t *testing.T) {
	u := User{ID: 7, Email: "reader@example.com", DisplayName: "读经人", PassHash: "secret"}

	b, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "email", "displayName"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing %q in %s", key, b)
		}
	}
	if _, leaked := got["passHash"]; leaked {
		t.Errorf("the password hash must never be serialised: %s", b)
	}
	if _, leaked := got["PassHash"]; leaked {
		t.Errorf("unexpected PascalCase key: %s", b)
	}
}

func TestVocabItemSerialisesAsCamelCase(t *testing.T) {
	v := VocabItem{
		ID: 1, UserID: 2, Lemma: "dhamma", LemmaID: 34677, POS: "masc",
		Meaning: "teaching", AddedAt: time.Now(), LastSeen: time.Now(),
	}
	b, _ := json.Marshal(v)
	var got map[string]any
	_ = json.Unmarshal(b, &got)
	for _, key := range []string{"lemma", "lemmaId", "pos", "meaning", "addedAt", "lastSeen"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing %q in %s", key, b)
		}
	}
	// The owner is implied by the session and has no business in the payload.
	if _, leaked := got["userID"]; leaked {
		t.Errorf("userID should not be serialised: %s", b)
	}
}

func TestBookmarkSerialisesAsCamelCase(t *testing.T) {
	bk := Bookmark{ID: 1, UserID: 2, BookID: "mula_di_01", Segment: 6, ParaNo: 1, Label: "开端"}
	b, _ := json.Marshal(bk)
	var got map[string]any
	_ = json.Unmarshal(b, &got)
	for _, key := range []string{"id", "bookId", "segment", "paraNo", "label"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing %q in %s", key, b)
		}
	}
}

// DictRoot goes out inside a lookup result, so its field names cross the wire.
func TestDictRootSerialisesAsCamelCase(t *testing.T) {
	r := DictRoot{Root: "√dhar", RootMeaning: "hold, carry, endure", RootSign: "a", RootGroup: 1}
	b, _ := json.Marshal(r)
	var got map[string]any
	_ = json.Unmarshal(b, &got)
	for _, key := range []string{"root", "meaning", "sign", "group"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing %q in %s", key, b)
		}
	}
}
