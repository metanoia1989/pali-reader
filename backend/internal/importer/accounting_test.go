package importer

import (
	"strings"
	"testing"
)

// Every line is either anchored or accounted for by a reason. The identity is
// worth a test because the reason is what the report is read for: a line that
// falls through both is silently missing from the numbers an argument is made
// with, and nothing else would show it.
func TestEveryLineIsAnchoredOrAccountedFor(t *testing.T) {
	flat := strings.Fields("a b c d e f g h i j k l m n o p q r s t u v w x y z")
	lines := []epiLine{
		{para: 1, line: 1, words: []string{"a", "b", "c", "d"}},
		{para: 1, line: 2, words: []string{"e", "f", "g", "h"}},
		{para: 1, line: 3, words: []string{"z", "z", "z", "z"}},
		{para: 1, line: 4, words: []string{"q", "r"}},
		{para: 1, line: 5, words: []string{"i", "j", "k", "l", "m"}},
		{para: 1, line: 6, words: []string{"w"}},
		{para: 1, line: 7, words: []string{"y", "z", "x", "v", "u", "t"}},
	}
	for _, opt := range []AnchorOptions{
		{MinGram: 2},
		{MinGram: 3},
		{MinGram: 4},
		{MinGram: 2, Safe: true},
		{MinGram: 3, Safe: true},
		{MinGram: 2, Safe: true, Exact: true},
	} {
		_, st := anchorLinesFrom(flat, lines, opt)
		if got := st.Anchored + st.Unplaced(); got != st.Lines {
			t.Errorf("%+v: anchored %d + unplaced %d = %d, want %d lines "+
				"(no verbatim %d, contradicted %d, short %d, uncarried %d)",
				opt, st.Anchored, st.Unplaced(), got, st.Lines,
				st.NoVerbatim, st.Contradicted, st.Short, st.Uncarried)
		}
	}
}
