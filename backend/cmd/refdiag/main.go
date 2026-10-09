// Command refdiag reports how well ePitaka's lines align onto this reader's
// segments, and why the ones that do not place failed.
//
// It is a build-time tool and reads only: tipitaka_pali.db for the text,
// epitaka.db for the lines. It rebuilds the segments the way ImportText does —
// parse the pages, cut the prose at ePitaka's boundaries — so what it measures
// is the corpus the importer would write, not an approximation of it.
//
//	go run ./cmd/refdiag -sources ../../.datawork
//	go run ./cmd/refdiag -sources ../../.datawork -book tika_vi_04 -v
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/metanoia/pali-reader/backend/internal/importer"

	_ "modernc.org/sqlite"
)

func main() {
	log.SetFlags(0)
	var (
		sources = flag.String("sources", "../data/sources", "directory holding tipitaka_pali.db and epitaka.db")
		book    = flag.String("book", "", "comma-separated book ids to measure (default: all mapped)")
		top     = flag.Int("top", 20, "how many books to list in the worst-offenders table")
		verbose = flag.Bool("v", false, "list every book")
	)
	flag.Parse()

	dir, err := filepath.Abs(*sources)
	if err != nil {
		log.Fatalf("refdiag: %v", err)
	}
	var books []string
	if strings.TrimSpace(*book) != "" {
		books = strings.Split(*book, ",")
	}

	rows, err := importer.Diagnose(
		filepath.Join(dir, "tipitaka_pali.db"),
		filepath.Join(dir, "epitaka.db"),
		importer.DiagOptions{Books: books, Log: func(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...) }},
	)
	if err != nil {
		log.Fatalf("refdiag: %v", err)
	}
	if len(rows) == 0 {
		log.Fatalf("refdiag: nothing measured")
	}

	if *verbose {
		fmt.Printf("%-20s %4s %8s %8s %7s | %-24s | %-24s | %s\n",
			"book", "vols", "words", "epiWords", "lines",
			"old: safe anchors", "new: diff alignment", "new failures / identical runs")
		for _, d := range rows {
			fmt.Printf("%-20s %4d %8d %8d %7d | %7d %6.1f%% stk %d/%d/%d | %7d %6.1f%% stk %d/%d/%d%s | unmatched=%d shifted=%d empty=%d runs=%d\n",
				d.Book, len(d.Volumes), d.Words, d.EpiWords, d.Lines,
				d.Safe.Anchored, 100*rate(d.Safe.Anchored, d.Safe.Lines),
				d.ProseS, d.VerseS, d.StackMaxS,
				d.Diff.Anchored, 100*rate(d.Diff.Anchored, d.Diff.Lines),
				d.ProseD, d.VerseD, d.StackMaxD, stackKind(d),
				d.Diff.Unmatched, d.Diff.Shifted, d.Diff.Empty, d.Diff.Blocks)
		}
		fmt.Println()
	}

	ceiling(rows)
	walk(rows)
	anchor(rows)
	stacking(rows)
	byVolumes(rows)
	worst(rows, *top)
}

// stackKind says what the most-loaded segment is, when it holds more than ten
// anchored lines. A verse is one segment holding every row of a gatha and a
// centered block is a gatha this edition did not mark as one; only a body
// paragraph piling up translations is a defect, so the count needs the label.
func stackKind(d importer.BookDiag) string {
	if d.StackKindD == "" {
		return ""
	}
	return "[" + d.StackKindD + "]"
}

func rate(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

func breakdown(d importer.BookDiag) string {
	parts := []string{}
	if d.NeverAttempted > 0 {
		parts = append(parts, fmt.Sprintf("never-attempted=%d", d.NeverAttempted))
	}
	keys := make([]string, 0, len(d.Fail))
	for k := range d.Fail {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	for _, k := range keys {
		if d.Fail[importer.LineFailure(k)] == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d", k, d.Fail[importer.LineFailure(k)]))
	}
	return strings.Join(parts, " ")
}

// ceiling is how much text the mapped volumes hold against how much the book
// holds. A volume carrying text the book does not have sets a hard limit on
// what any aligner can place, and a rate quoted without it invites the reading
// that the search is at fault.
func ceiling(rows []importer.BookDiag) {
	var words, epi int
	var lean, fat int
	for _, d := range rows {
		words += d.Words
		epi += d.EpiWords
		if d.EpiWords > d.Words {
			fat++
		} else {
			lean++
		}
	}
	fmt.Println("== how much text there was to place")
	fmt.Printf("   books %d   this reader's words %d   mapped volumes' words %d   ratio %.3f\n",
		len(rows), words, epi, float64(epi)/float64(max(words, 1)))
	fmt.Printf("   books whose volumes are larger than the book: %d   smaller: %d\n", fat, lean)
	fmt.Println()
}

func walk(rows []importer.BookDiag) {
	var lines, attempted, placed int
	fail := map[importer.LineFailure]int{}
	for _, d := range rows {
		lines += d.Lines
		attempted += d.Attempted
		placed += d.Placed
		for k, v := range d.Fail {
			fail[k] += v
		}
	}
	un := lines - placed
	fmt.Println("== the walk as it stands: forward search from a cursor, inside the paragraph's span")
	fmt.Printf("   lines in the mapped volumes %d\n", lines)
	fmt.Printf("   lines the walk never looked at %d (%.2f%%) — their paragraph never located at all\n",
		lines-attempted, 100*rate(lines-attempted, lines))
	fmt.Printf("   lines it did look at %d   placed %d   unplaced %d (%.2f%% of looked-at, %.2f%% of all)\n",
		attempted, placed, attempted-placed,
		100*rate(attempted-placed, attempted), 100*rate(attempted-placed, lines))
	fmt.Printf("   placed as a share of every line the volumes hold: %.2f%%\n", 100*rate(placed, lines))
	keys := make([]string, 0, len(fail))
	for k := range fail {
		keys = append(keys, string(k))
	}
	sort.Slice(keys, func(i, j int) bool { return fail[importer.LineFailure(keys[i])] > fail[importer.LineFailure(keys[j])] })
	for _, k := range keys {
		v := fail[importer.LineFailure(k)]
		fmt.Printf("     %-11s %8d  %5.1f%% of unplaced, %5.2f%% of all lines\n",
			k, v, 100*rate(v, un), 100*rate(v, lines))
	}
	fmt.Println()
}

// anchor reports the anchor aligner over the same lines, and the diff
// alignment the importer now uses, so the two can be read side by side.
func anchor(rows []importer.BookDiag) {
	var loose, strict, safe, diff importer.AnchorStats
	for _, d := range rows {
		add := func(dst *importer.AnchorStats, s importer.AnchorStats) {
			dst.Lines += s.Lines
			dst.Anchored += s.Anchored
			dst.Ambiguous += s.Ambiguous
			dst.Exact += s.Exact
			dst.NoVerbatim += s.NoVerbatim
			dst.Contradicted += s.Contradicted
			dst.Short += s.Short
			dst.Uncarried += s.Uncarried
			dst.Full += s.Full
			dst.Weak += s.Weak
			dst.Unmatched += s.Unmatched
			dst.Shifted += s.Shifted
			dst.Empty += s.Empty
			dst.Blocks += s.Blocks
			dst.BlockWords += s.BlockWords
			dst.Work += s.Work
			dst.Numbered += s.Numbered
			if s.Truncated {
				dst.Truncated = true
			}
		}
		add(&loose, d.Exact)
		add(&strict, d.Strict)
		add(&safe, d.Safe)
		add(&diff, d.Diff)
	}
	fmt.Println("== the anchor aligner: whole-book lookup of each line, monotone by construction")
	for _, s := range []struct {
		name string
		st   importer.AnchorStats
	}{
		{"loose  (ambiguous opening resolved by position)", loose},
		{"strict (four-word openings only)", strict},
		{"safe   (a short opening must be carried by more text) — what the importer used before", safe},
	} {
		fmt.Printf("   %s\n", s.name)
		fmt.Printf("     [accounting] lines %d anchored %d unplaced() %d anchored+unplaced %d\n",
			s.st.Lines, s.st.Anchored, s.st.Unplaced(), s.st.Anchored+s.st.Unplaced())
		fmt.Printf("     anchored %d of %d lines (%.2f%%), of those whole-line %d (%.2f%%), on four words %d (%.2f%%), weak %d\n",
			s.st.Anchored, s.st.Lines, 100*rate(s.st.Anchored, s.st.Lines),
			s.st.Full, 100*rate(s.st.Full, s.st.Lines),
			s.st.Exact, 100*rate(s.st.Exact, s.st.Lines), s.st.Weak)
		fmt.Printf("     unplaced %d (%.2f%%):  no verbatim opening %d   contradicted %d   short %d   ambiguous %d   too short to vouch for %d\n",
			s.st.Unplaced(), 100*rate(s.st.Unplaced(), s.st.Lines),
			s.st.NoVerbatim, s.st.Contradicted, s.st.Short, s.st.Ambiguous, s.st.Uncarried)
	}
	fmt.Println("   diff   (the whole book aligned against the whole volume) — what the importer uses now")
	fmt.Printf("     [accounting] lines %d anchored %d unplaced() %d anchored+unplaced %d\n",
		diff.Lines, diff.Anchored, diff.Unplaced(), diff.Anchored+diff.Unplaced())
	fmt.Printf("     anchored %d of %d lines (%.2f%%), of those resting on their own text throughout %d (%.2f%%)\n",
		diff.Anchored, diff.Lines, 100*rate(diff.Anchored, diff.Lines),
		diff.Full, 100*rate(diff.Full, diff.Lines))
	fmt.Printf("     identical runs found %d covering %d of the volumes' words, at a cost of %d word comparisons\n",
		diff.Blocks, diff.BlockWords, diff.Work)
	if diff.Truncated {
		fmt.Println("     THE WORK BUDGET WAS REACHED on at least one book: its alignment is partial")
	}
	fmt.Printf("     of the placed, %d (%0.2f%%) were found after stepping over the numerals at their head\n",
		diff.Numbered, 100*rate(diff.Numbered, diff.Lines))
	fmt.Printf("     unplaced %d (%.2f%%):  not in this book at all %d (%.2f%%)   begins on text that differs %d (%.2f%%)   no words %d\n",
		diff.Unplaced(), 100*rate(diff.Unplaced(), diff.Lines),
		diff.Unmatched, 100*rate(diff.Unmatched, diff.Lines),
		diff.Shifted, 100*rate(diff.Shifted, diff.Lines), diff.Empty)
	fmt.Println()
}

// byVolumes separates the books fed by one ePitaka volume from those fed by
// several, because a book spanning six volumes is where a local search has the
// most room to drift and is the first thing to check.
func byVolumes(rows []importer.BookDiag) {
	type acc struct{ books, lines, placed, anchored, safe int }
	groups := map[string]*acc{}
	for _, d := range rows {
		name := "one volume"
		if len(d.Volumes) > 1 {
			name = "several volumes"
		}
		a := groups[name]
		if a == nil {
			a = &acc{}
			groups[name] = a
		}
		a.books++
		a.lines += d.Lines
		a.placed += d.Placed
		a.anchored += d.Diff.Anchored
		a.safe += d.Safe.Anchored
	}
	fmt.Println("== by how many ePitaka volumes feed the book")
	for _, name := range []string{"one volume", "several volumes"} {
		a := groups[name]
		if a == nil {
			continue
		}
		fmt.Printf("   %-16s books %3d  lines %7d  walk %7d (%5.1f%%)  old %7d (%5.1f%%)  diff %7d (%5.1f%%)\n",
			name, a.books, a.lines, a.placed, 100*rate(a.placed, a.lines),
			a.safe, 100*rate(a.safe, a.lines), a.anchored, 100*rate(a.anchored, a.lines))
	}
	fmt.Println()
}

// stacking is the reader-visible defect: how many segments would end up with
// more than ten translations under them, at each anchor length.
func stacking(rows []importer.BookDiag) {
	type row struct {
		what              string
		over, max, pr, vs int
	}
	var accs []row
	diffRow := row{what: "diff (now)"}
	safeRow := row{what: "short+carried"}
	for _, d := range rows {
		safeRow.over += d.StackS
		safeRow.pr += d.ProseS
		safeRow.vs += d.VerseS
		if d.StackMaxS > safeRow.max {
			safeRow.max = d.StackMaxS
		}
		diffRow.over += d.StackD
		diffRow.pr += d.ProseD
		diffRow.vs += d.VerseD
		if d.StackMaxD > diffRow.max {
			diffRow.max = d.StackMaxD
		}
	}
	accs = append(accs, diffRow, safeRow)
	for _, g := range []int{2, 3, 4} {
		r := row{what: fmt.Sprintf("%d-word anchors", g)}
		for _, d := range rows {
			var over, max, pr, vs int
			switch g {
			case 2:
				over, max, pr, vs = d.Stack2, d.StackMax2, d.Prose2, d.Verse2
			case 3:
				over, max, pr, vs = d.Stack3, d.StackMax3, d.Prose3, d.Verse3
			default:
				over, max, pr, vs = d.Stack4, d.StackMax4, d.Prose4, d.Verse4
			}
			r.over += over
			r.pr += pr
			r.vs += vs
			if max > r.max {
				r.max = max
			}
		}
		accs = append(accs, r)
	}
	fmt.Println("== lines stacked under one segment (the reader-visible defect)")
	fmt.Println("   a count is not on its own a fault: a verse is one segment holding")
	fmt.Println("   every row of the gatha, and ePitaka's own rows are sometimes long.")
	for _, r := range accs {
		fmt.Printf("   %-14s %5d segments over ten (prose %d, verse %d), worst %d lines under one\n",
			r.what, r.over, r.pr, r.vs, r.max)
	}
	fmt.Println()
}

func worst(rows []importer.BookDiag, n int) {
	sorted := make([]importer.BookDiag, len(rows))
	copy(sorted, rows)
	// Ordered by the alignment the importer now uses, because that is the one
	// an argument about what is left has to be made from.
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Diff.Unplaced() > sorted[j].Diff.Unplaced()
	})
	if n > len(sorted) {
		n = len(sorted)
	}
	fmt.Printf("== worst %d books by lines the new alignment did not place\n", n)
	fmt.Printf("   %-22s %3s %8s | %8s %7s | %8s %7s | %s\n",
		"book", "vol", "lines", "old safe", "rate", "new diff", "rate", "new failures")
	for _, d := range sorted[:n] {
		fmt.Printf("   %-22s %3d %8d | %8d %6.1f%% | %8d %6.1f%% | unmatched=%d shifted=%d empty=%d\n",
			d.Book, len(d.Volumes), d.Lines,
			d.Safe.Anchored, 100*rate(d.Safe.Anchored, d.Safe.Lines),
			d.Diff.Anchored, 100*rate(d.Diff.Anchored, d.Diff.Lines),
			d.Diff.Unmatched, d.Diff.Shifted, d.Diff.Empty)
	}
	fmt.Println()
}
