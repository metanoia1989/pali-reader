package importer

import "testing"

// The emphasis markers are the only markup that survives import, and they have
// to be bytes that cannot occur in the source: the client splits on them and
// renders the middle as bold, with no HTML anywhere in the application.
func TestSanitizeMarkupKeepsOnlyEmphasis(t *testing.T) {
	cases := map[string]string{
		"aḍḍha + daṇḍak<b>assa</b>":            "aḍḍha + daṇḍak\u0001assa\u0002",
		"√budh + ta":                           "√budh + ta",
		"<i>italic</i> and <b>bold</b>":        "italic and \u0001bold\u0002",
		"<span class='x'>drop</span>":          "drop",
		"a &amp; b &lt;c&gt;":                  "a & b <c>",
		"":                                     "",
		"plain text with no markup at all":     "plain text with no markup at all",
		"nested <b>bold <i>and italic</i></b>": "nested \u0001bold and italic\u0002",
	}
	for in, want := range cases {
		if got := SanitizeMarkup(in); got != want {
			t.Errorf("SanitizeMarkup(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripMarkupRemovesEmphasisToo(t *testing.T) {
	got := StripMarkup("daṇḍak<b>assa</b>")
	if got != "daṇḍakassa" {
		t.Errorf("StripMarkup = %q, want daṇḍakassa", got)
	}
}
