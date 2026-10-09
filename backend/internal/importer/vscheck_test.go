package importer

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/metanoia/pali-reader/backend/internal/corpus"
)

// The verse split, checked against the whole corpus rather than a fixture.
//
// The fixtures beside this file pin the mechanism on passages with the shape of
// the real thing. This runs the real thing: every mapped book, parsed twice —
// once as the pages parse, once through the split — and compares the two. What
// it can prove that a fixture cannot is that nothing was lost anywhere in
// 1.2 million lines, which is the property the reader would notice last and
// suffer from most.
//
// It needs the source databases (the same ones the importer reads) and is
// skipped without them:
//
//	VSOURCES=../../.datawork go test ./internal/importer -run TestVerseSplit -v
func TestVerseSplitPreservesTheWholeCorpus(t *testing.T) {
	dir := os.Getenv("VSOURCES")
	if dir == "" {
		t.Skip("VSOURCES not set: point it at the directory holding tipitaka_pali.db and epitaka.db")
	}
	dir, _ = filepath.Abs(dir)
	db, err := openSQLite(filepath.Join(dir, "tipitaka_pali.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sent, err := openSentences(filepath.Join(dir, "epitaka.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sent.Close()

	only := map[string]bool{}
	for _, b := range strings.Split(os.Getenv("VBOOKS"), ",") {
		if b = strings.TrimSpace(b); b != "" {
			only[b] = true
		}
	}

	books, err := bookIDsInOrder(db)
	if err != nil {
		t.Fatal(err)
	}

	var (
		checked                           int
		segsBefore, segsAfter             int
		kindBefore                        = map[string]int{}
		kindAfter                         = map[string]int{}
		versesSplit, linesMade, breaksOff int
		versesWithNumeral                 int
		linesWithNumeralBefore            int
		linesWithNumeralAfter             int
	)
	for _, id := range books {
		if len(only) > 0 && !only[id] {
			continue
		}
		before, err := parseOnly(db, id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if len(before) == 0 {
			continue
		}
		after, err := segmentsFor(db, sent, id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		checked++
		segsBefore += len(before)
		segsAfter += len(after)
		for _, s := range before {
			kindBefore[s.Kind]++
		}
		for _, s := range after {
			kindAfter[s.Kind]++
		}

		// 1. Losslessness, per kind. The split may move a space from one piece
		// to the next and may not add or lose a letter, so every (book, kind)
		// group has to digest the same before and after.
		hBefore, hAfter := map[string]string{}, map[string]string{}
		for _, s := range before {
			hBefore[s.Kind] += stripWhitespace(s.Text)
		}
		for _, s := range after {
			hAfter[s.Kind] += stripWhitespace(s.Text)
		}
		for kind, text := range hBefore {
			if digest(text) != digest(hAfter[kind]) {
				t.Errorf("%s/%s: the text changed (%d bytes before, %d after)",
					id, kind, len(text), len(hAfter[kind]))
			}
		}
		for kind, text := range hAfter {
			if _, ok := hBefore[kind]; !ok && text != "" {
				t.Errorf("%s/%s: %d bytes appeared from nowhere", id, kind, len(text))
			}
		}

		// 2. Only verse may be divided. A heading is what a table-of-contents
		// entry points at and a centred line is a verse this edition did not
		// mark as one; both have to come out of the split exactly as they went
		// in, byte for byte and in the same order.
		var fixedBefore, fixedAfter []string
		for _, s := range before {
			if s.Kind == corpus.KindHeading || s.Kind == corpus.KindCenter {
				fixedBefore = append(fixedBefore, s.Kind+"\x00"+s.Text)
			}
		}
		for _, s := range after {
			if s.Kind == corpus.KindHeading || s.Kind == corpus.KindCenter {
				fixedAfter = append(fixedAfter, s.Kind+"\x00"+s.Text)
			}
		}
		if strings.Join(fixedBefore, "\x01") != strings.Join(fixedAfter, "\x01") {
			t.Errorf("%s: headings or centred lines were disturbed", id)
		}

		// 3. The word stream the alignment is computed on may not move. This is
		// what keeps a boundary and the translation under it one decision: the
		// reference import aligns the words after the split, and it can only
		// agree with the split if the split did not change them.
		if a, b := wordStreamOf(before), wordStreamOf(after); a != b {
			t.Errorf("%s: the cut changed the word stream", id)
		}

		// 4. A verse that was divided is divided into lines: no piece may hold
		// a line break, and the pieces must spell the verse back out. The two
		// lists are walked together rather than joined on para_no, because a
		// paragraph number is not unique in this corpus — Khuddakapāṭha writes
		// 13 on three different verses.
		var afterVerses []corpus.Segment
		for _, s := range after {
			if s.Kind == corpus.KindVerse {
				afterVerses = append(afterVerses, s)
			}
		}
		j := 0
		for _, s := range before {
			if s.Kind != corpus.KindVerse {
				continue
			}
			if hasNumeralPrefix(s.Text) {
				versesWithNumeral++
			}
			linesWithNumeralBefore += countNumeralLines(s.Text)
			want := stripWhitespace(s.Text)
			var joined strings.Builder
			n := 0
			for j+n < len(afterVerses) {
				joined.WriteString(stripWhitespace(afterVerses[j+n].Text))
				n++
				if joined.String() == want {
					break
				}
				if !strings.HasPrefix(want, joined.String()) {
					t.Errorf("%s: the pieces of verse %q do not spell it out: %q",
						id, want, joined.String())
					break
				}
			}
			if joined.String() != want {
				t.Errorf("%s: verse %q was not preserved: %q", id, want, joined.String())
			}
			if n > 1 {
				versesSplit++
				linesMade += n
				for k := 0; k < n; k++ {
					if strings.ContainsRune(afterVerses[j+k].Text, '\n') {
						breaksOff++
						t.Errorf("%s: a piece of verse %q still holds a line break: %q",
							id, want, afterVerses[j+k].Text)
					}
				}
			}
			j += n
		}
		if j != len(afterVerses) {
			t.Errorf("%s: %d verse segments came out of the split that no verse accounts for",
				id, len(afterVerses)-j)
		}
		for _, s := range afterVerses {
			linesWithNumeralAfter += countNumeralLines(s.Text)
		}
	}

	fmt.Printf("books checked %d\n", checked)
	fmt.Printf("segments      before %d   after %d   (+%d)\n", segsBefore, segsAfter, segsAfter-segsBefore)
	for _, k := range []string{corpus.KindProse, corpus.KindVerse, corpus.KindHeading, corpus.KindCenter} {
		fmt.Printf("  %-8s    before %7d   after %7d   (%+d)\n", k, kindBefore[k], kindAfter[k], kindAfter[k]-kindBefore[k])
	}
	fmt.Printf("verse          %d gathas split into %d lines; %d pieces still held a break\n", versesSplit, linesMade, breaksOff)
	fmt.Printf("numerals       %d verse segments begin with a number; lines beginning with one: %d before, %d after\n",
		versesWithNumeral, linesWithNumeralBefore, linesWithNumeralAfter)
}

// parseOnly rebuilds a book's segments as ParseBook alone leaves them: the state
// the split starts from, and therefore what the split is compared against.
func parseOnly(db *sql.DB, bookID string) ([]corpus.Segment, error) {
	rows, err := db.Query(`SELECT page, COALESCE(content,'') FROM pages WHERE bookid=? ORDER BY page`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sb strings.Builder
	var starts []int
	for rows.Next() {
		var page int
		var content string
		if err := rows.Scan(&page, &content); err != nil {
			return nil, err
		}
		starts = append(starts, sb.Len())
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	return corpus.ParseBook(sb.String(), starts), nil
}

// stripWhitespace is the comparison the losslessness check makes: the split may
// move a space from one piece to the next, and no letter may move at all.
func stripWhitespace(s string) string {
	return strings.NewReplacer(" ", "", "\n", "", "\t", "", "\r", "").Replace(s)
}

func digest(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// wordStreamOf is the book's words in reading order — the stream the aligner
// and the reference importer both work on.
func wordStreamOf(segs []corpus.Segment) string {
	var b strings.Builder
	for _, s := range segs {
		for _, w := range alignWords(s.Text) {
			b.WriteString(w)
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// hasNumeralPrefix reports whether a segment opens with the edition's own verse
// number, which is written once at the head of a gatha.
func hasNumeralPrefix(s string) bool {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i > 0 && i < len(s) && s[i] == '.'
}

// countNumeralLines counts the printed lines of a segment that open with a
// number. A verse number that leaked onto every line shows up here as a count
// equal to the line count.
func countNumeralLines(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if hasNumeralPrefix(strings.TrimSpace(line)) {
			n++
		}
	}
	return n
}
