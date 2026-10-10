package server

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The snippet is what the search result actually shows, and it is the one place
// where a string from the database becomes markup on the client. Both of the
// ways that can go wrong are silent: an unescaped '<' renders as an element,
// and a cut in the middle of a multi-byte letter renders as a replacement
// character. Neither shows up in a Go build.

func TestSnippetMarksTheMatch(t *testing.T) {
	got := snippet("evaṃ me sutaṃ", "me", 40)
	if !strings.Contains(got, "<mark>me</mark>") {
		t.Fatalf("no marker around the match: %q", got)
	}
}

// The corpus is Pāḷi and the translations are Chinese; offset arithmetic that
// assumes bytes cuts both in half.
func TestSnippetIsRuneSafe(t *testing.T) {
	text := strings.Repeat("涅槃", 200) + "dhamma" + strings.Repeat("法", 200)
	got := snippet(text, "dhamma", 60)
	if !utf8.ValidString(strings.NewReplacer("<mark>", "", "</mark>", "", "…", "").Replace(got)) {
		t.Fatalf("snippet is not valid UTF-8: %q", got)
	}
	if !strings.Contains(got, "<mark>dhamma</mark>") {
		t.Fatalf("match lost: %q", got)
	}
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("a window cut at both ends should say so: %q", got)
	}
}

// A stray '<' in a variant reading must not become markup.
func TestSnippetEscapesTheText(t *testing.T) {
	got := snippet("a <b>bold</b> word", "bold", 80)
	if strings.Contains(got, "<b>") {
		t.Fatalf("markup survived: %q", got)
	}
	if !strings.Contains(got, "&lt;b&gt;") {
		t.Fatalf("angle brackets should be escaped: %q", got)
	}
}

// A query with no match still shows where it came from, not a blank row.
func TestSnippetWithoutAMatch(t *testing.T) {
	got := snippet("evaṃ me sutaṃ", "zzz", 80)
	if got == "" || strings.Contains(got, "<mark>") {
		t.Fatalf("unexpected snippet: %q", got)
	}
}

// % and _ are LIKE wildcards. A reader typing 「100%」 must not turn the query
// into "match everything", which is a table scan dressed up as a search.
func TestEscapeLikeNeutralisesWildcards(t *testing.T) {
	got := escapeLike(`50%_x\y`)
	want := `50\%\_x\\y`
	if got != want {
		t.Fatalf("escapeLike = %q, want %q", got, want)
	}
}

// The reader's 参考译文 setting decides which languages are searched. An empty
// parameter means "both", not "none" — the failure mode of the other reading is
// a search that silently returns nothing.
func TestSplitLangsDefaultsToBoth(t *testing.T) {
	if got := splitLangs(""); len(got) != 2 || got[0] != "zh" || got[1] != "en" {
		t.Fatalf("splitLangs(\"\") = %v", got)
	}
	if got := splitLangs("zh,en"); len(got) != 2 {
		t.Fatalf("splitLangs(zh,en) = %v", got)
	}
	if got := splitLangs(" en "); len(got) != 1 || got[0] != "en" {
		t.Fatalf("splitLangs( en ) = %v", got)
	}
}
