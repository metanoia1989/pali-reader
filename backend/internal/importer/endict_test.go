package importer

import (
	"strings"
	"testing"

	"github.com/metanoia/pali-reader/backend/internal/store"
)

// The seed's line breaks are two characters, not one.
//
// ECDICT writes `居住(于)\n[医] 住房` with a literal backslash and "n", and the
// popup renders definitions with `white-space: pre-line`, which breaks on real
// newlines only. Shipped unchanged, every multi-gloss entry prints "\n" in the
// middle of the definition and looks like a bug in the dictionary. 38,437 of
// the 89,501 entries carry at least one.
func TestNormalizeDefTextTurnsEscapesIntoLineBreaks(t *testing.T) {
	cases := map[string]string{
		`a\nb`:            "a\nb",
		`a\r\nb`:          "a\nb",
		`a\rb`:            "a\nb",
		"already\nbreaks": "already\nbreaks",
		`no escapes here`: "no escapes here",
		`  padded\n`:      "padded",
		`C:\path`:         `C:\path`,
	}
	for in, want := range cases {
		if got := NormalizeDefText(in); got != want {
			t.Errorf("NormalizeDefText(%q) = %q, want %q", in, got, want)
		}
	}
	// Idempotent: the seed is read once, but a definition that came through a
	// second pass must not grow a second newline.
	for _, in := range []string{`a\nb`, "a\nb", `x`} {
		once := NormalizeDefText(in)
		if twice := NormalizeDefText(once); twice != once {
			t.Errorf("NormalizeDefText is not idempotent: %q -> %q", once, twice)
		}
	}
}

const seedSample = `[
 {"w":"Aachen","p":"'ɑ:kәn","s":[{"pos":"","def":"亚琛[德意志联邦共和国西部城市]"}]},
 {"w":"Dwelling","p":"'dweliŋ","s":[{"pos":"n.","def":"住处\\n[医] 住房"}]},
 {"w":"","p":"","s":[{"pos":"n.","def":"orphan"}]},
 {"w":"empty","p":"","s":[{"pos":"","def":""}]},
 {"w":" Mono ","p":"","s":[{"pos":"n.","def":"单声道"}]}
]`

func TestStreamEnglishSeedFoldsKeysAndSkipsEmptyRows(t *testing.T) {
	var rows []store.DictEnEntry
	n, skipped, err := StreamEnglishSeed(strings.NewReader(seedSample), func(e store.DictEnEntry) error {
		rows = append(rows, e)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 || skipped != 2 {
		t.Fatalf("imported %d, skipped %d; want 3 and 2", n, skipped)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows", len(rows))
	}

	// The key is folded and trimmed; the head keeps the dictionary's spelling,
	// which is what the popup prints.
	if rows[0].Word != "aachen" || rows[0].Head != "Aachen" {
		t.Errorf("first row = %+v", rows[0])
	}
	if rows[1].Word != "dwelling" {
		t.Errorf("second row key = %q", rows[1].Word)
	}
	// The escape became a real line break inside the stored JSON.
	if !strings.Contains(rows[1].Senses, `住处\n[医]`) {
		t.Errorf("senses not normalised: %q", rows[1].Senses)
	}
	if rows[2].Word != "mono" || rows[2].Head != "Mono" {
		t.Errorf("third row = %+v", rows[2])
	}
}

// A transfer that died mid-entry leaves a file that json.Decoder reads happily
// up to the cut, and a decoder that does not insist on the closing bracket
// imports a dictionary that silently stops at whatever letter the transfer
// reached — which looks exactly like those words having no entry.
func TestStreamEnglishSeedRejectsATruncatedFile(t *testing.T) {
	// Cut inside an entry: the decoder itself notices.
	midEntry := seedSample[:strings.Index(seedSample, `{"w":" Mono`)]
	if _, _, err := StreamEnglishSeed(strings.NewReader(midEntry), func(store.DictEnEntry) error { return nil }); err == nil {
		t.Error("a seed cut inside an entry was accepted")
	}

	// Cut after a whole entry but before the closing bracket. The decoder is
	// satisfied by this file; only reading the last token catches it.
	noBracket := `[{"w":"a","p":"","s":[{"pos":"n.","def":"x"}]}`
	n, _, err := StreamEnglishSeed(strings.NewReader(noBracket), func(store.DictEnEntry) error { return nil })
	if err == nil {
		t.Fatal("a seed with no closing bracket was accepted")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("imported %d entries before noticing the truncation, want 1", n)
	}

	if _, _, err := StreamEnglishSeed(strings.NewReader(`{"w":"a"}`), func(store.DictEnEntry) error { return nil }); err == nil {
		t.Fatal("a seed that is not an array was accepted")
	}
}

func TestStreamEnglishSeedPropagatesWriteErrors(t *testing.T) {
	boom := strings.NewReader(`[{"w":"a","s":[{"pos":"","def":"x"}]}]`)
	if _, _, err := StreamEnglishSeed(boom, func(store.DictEnEntry) error {
		return errStop
	}); err != errStop {
		t.Fatalf("err = %v, want the write error", err)
	}
}

var errStop = &stopError{}

type stopError struct{}

func (*stopError) Error() string { return "stop" }
