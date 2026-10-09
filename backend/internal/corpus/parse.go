// Package corpus turns the CST page HTML that ships inside tipitaka_pali.db
// into the segment stream the reader renders.
//
// The upstream file is one HTML blob per printed page. Its structure is simple
// and stable: a run of <p class="..."> blocks, each optionally preceded by
// paragraph anchors (<a name="para24">) and edition page anchors
// (<a name="M1.0010">). Everything the reader needs is derived here, once, at
// import time, so that serving a book is a single indexed range scan.
package corpus

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Segment kinds, matching the constants in internal/store.
const (
	KindProse   = "prose"
	KindVerse   = "verse"
	KindHeading = "heading"
	KindCenter  = "center"
	// KindNumber is a block holding nothing but a paragraph number. It is not
	// shown on its own; it separates two verses and supplies the number for the
	// one that follows.
	KindNumber = "number"
)

// block is one <p> element pulled out of the page HTML.
type block struct {
	class  string
	inner  string
	paraNo int    // paragraph number in force at this block, 0 if none yet
	marker string // edition page anchor that opened this block, if any
}

var (
	// One pass over the page finds both anchors and paragraph elements in
	// document order, which is what lets a paragraph number be attributed to
	// the block that follows it.
	tokenRe = regexp.MustCompile(`(?s)<a name="([^"]*)">\s*</a>|<p class="([^"]*)">(.*?)</p>`)
	// The plain "para24" anchor. The variant form "para24_vin1" is the same
	// paragraph in a different edition and is ignored.
	paraRe = regexp.MustCompile(`^para(\d+)$`)
	// CST puts the paragraph anchor *inside* the paragraph element
	// (<p class="bodytext"><a name="para24"></a>...), so the outer pass that
	// splits out <p> elements swallows it. The number is read from the block
	// body instead, and a <p class="hangnum"> that holds nothing but the anchor
	// still sets it for the block that follows.
	innerParaRe = regexp.MustCompile(`<a name="para(\d+)">`)
	// Edition page anchors: M = Myanmar, T = Thai, V = VRI, P = PTS, S = Sinhala.
	markerRe = regexp.MustCompile(`^([MTVPS])(\d+)\.(\d+)$`)
	tagRe    = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRe  = regexp.MustCompile(`[ \t\r\n]+`)
	// variantRe rewrites the CST variant-reading span into the class the
	// reader styles. The content is kept: a variant reading is exactly the
	// kind of thing a student of the canon wants to see.
	variantRe = regexp.MustCompile(`<span class="note">`)
	// The same span, removed whole. Stripping tags alone would leave the note's
	// text sitting in the middle of the sentence it annotates.
	// The optional trailing space matters: a variant sits between two words,
	// and removing the span alone leaves the two spaces that surrounded it.
	// Swallowing one keeps the text and every offset exact.
	noteSpanRe = regexp.MustCompile(`(?s)<span class="note">(.*?)</span>[ \t]?`)
	// The commentary bolds the word it is about. That emphasis is the only
	// thing that marks it as a headword, so it is recorded rather than lost.
	boldSpanRe = regexp.MustCompile(`(?s)<span class="bld">(.*?)</span>`)
	// The paragraph number is also written into the text, not only into the
	// anchor: <a name="para24"></a>...<span class="paranum">24</span>. The
	// reader shows the number in its own gutter, so leaving this copy in place
	// prints it twice.
	leadNumRe = regexp.MustCompile(`^\s*<span class="paranum">([^<]*)</span>\s*\.?\s*`)
	// The prefix a block may carry before that number: nothing but anchors.
	leadAnchorsRe = regexp.MustCompile(`^(?:\s*<a name="[^"]*">\s*</a>)*\s*`)
)

// classKind maps an upstream CSS class to the reader's segment kind.
func classKind(class string) (kind string, annotatable bool, level int) {
	switch strings.TrimSpace(class) {
	case "bodytext", "noindentbodytext", "unindented", "indent":
		return KindProse, true, 0
	case "gatha1", "gatha2", "gatha3", "gathalast":
		return KindVerse, true, 0
	case "centered":
		return KindCenter, true, 0
	case "hangnum", "paranum":
		return KindNumber, false, 0
	case "nikaya":
		return KindHeading, false, 1
	case "book":
		return KindHeading, false, 2
	case "chapter":
		return KindHeading, false, 3
	case "title":
		return KindHeading, false, 4
	case "subhead":
		return KindHeading, false, 5
	case "subsubhead":
		return KindHeading, false, 6
	}
	return "", false, 0
}

// Variant is one CST variant reading and where it stood.
type Variant struct {
	Offset int    `json:"o"`
	Text   string `json:"t"`
}

// Span is a run of the text the edition sets in bold — the headword of a
// commentary gloss, most often. Offsets and lengths are in UTF-16 units, the
// unit the client slices by.
type Span struct {
	Offset int `json:"o"`
	Length int `json:"l"`
}

// Segment is one reader-visible block of a book.
type Segment struct {
	Seq     int
	ParaNo  int
	Kind    string
	Text    string // plain reading text, no markup
	HTML    string // inline markup kept: variants, bold, verse breaks
	Markers []string
	// Variants records the CST variant readings that were removed from Text,
	// as a position in Text (UTF-16 units, the unit the client slices by) and
	// the reading itself. Removed but not discarded: a variant reading is
	// exactly what a student of the canon wants to see, and the reader can
	// switch them on.
	Variants []Variant
	// Bold records the runs the edition sets in bold. The markup is stripped
	// from Text like everything else, so without this the commentary's
	// headwords lose the emphasis that marks them as headwords.
	Bold []Span
	// TocName and TocLevel are set on a heading, which also opens a TOC entry.
	TocName  string
	TocLevel int
	PageNo   int // the printed page this block starts on, 1-based within the book
}

// ParseBook walks the concatenated page HTML of one book and returns the
// segment stream in reading order.
//
// pageHTML must be the pages of the book joined in page order. pageStarts
// gives, for each page in the same order, the byte offset at which it begins
// in pageHTML, so a segment can be attributed to a printed page.
func ParseBook(pageHTML string, pageStarts []int) []Segment {
	blocks := scan(pageHTML)

	var segs []Segment
	var pending []block // consecutive verse lines waiting to be grouped

	// pendingNum is the paragraph number of a block that carried nothing else.
	// For prose the number is written inside the paragraph itself; for verse it
	// sits in a block of its own, and that block is also what separates one
	// verse from the next. Joining every gāthā of a chapter into a single
	// segment, as this used to, loses the verse boundaries the edition prints.
	pendingNum := ""

	flushVerse := func() {
		if len(pending) == 0 {
			return
		}
		number := pendingNum
		pendingNum = ""
		var textLines, htmlLines []string
		markers := map[string]bool{}
		var order []string
		var variantList []Variant
		var boldList []Span
		variantBase := 0
		for _, b := range pending {
			t, vs, bs := SplitRuns(b.inner)
			textLines = append(textLines, t)
			for _, v := range vs {
				variantList = append(variantList, Variant{Offset: variantBase + v.Offset, Text: v.Text})
			}
			for _, sp := range bs {
				boldList = append(boldList, Span{Offset: variantBase + sp.Offset, Length: sp.Length})
			}
			variantBase += utf16Len(t) + 1 // +1 for the newline joining the lines
			htmlLines = append(htmlLines, Markup(b.inner))
			for _, m := range Markers(b.inner) {
				if !markers[m] {
					markers[m] = true
					order = append(order, m)
				}
			}
		}
		text := strings.Join(textLines, "\n")
		markup := strings.Join(htmlLines, "<br>")
		text, variantList, boldList = prependNumber(text, number, variantList, boldList)
		segs = append(segs, Segment{
			Seq:      len(segs) + 1,
			ParaNo:   pending[0].paraNo,
			Kind:     KindVerse,
			Text:     text,
			HTML:     markup,
			Markers:  order,
			Variants: variantList,
			Bold:     boldList,
		})
		pending = nil
	}

	for _, b := range blocks {
		kind, _, level := classKind(b.class)
		if kind == "" {
			continue
		}
		if kind == KindNumber {
			flushVerse()
			if b.paraNo > 0 {
				pendingNum = strconv.Itoa(b.paraNo)
			}
			continue
		}
		if kind == KindVerse {
			pending = append(pending, b)
			continue
		}
		flushVerse()
		if kind == KindHeading {
			name := PlainText(b.inner)
			if name != "" {
				segs = append(segs, Segment{
					Seq:      len(segs) + 1,
					ParaNo:   b.paraNo,
					Kind:     KindHeading,
					Text:     name,
					HTML:     Markup(b.inner),
					Markers:  Markers(b.inner),
					TocName:  name,
					TocLevel: level,
				})
			}
			continue
		}
		text, variants, bold := SplitRuns(b.inner)
		text, variants, bold = prependNumber(text, pendingNum, variants, bold)
		if strings.HasPrefix(text, pendingNum+". ") && pendingNum != "" {
			pendingNum = ""
		}
		segs = append(segs, Segment{
			Seq:      len(segs) + 1,
			ParaNo:   b.paraNo,
			Kind:     kind,
			Text:     text,
			HTML:     Markup(b.inner),
			Markers:  Markers(b.inner),
			Variants: variants,
			Bold:     bold,
		})
	}
	flushVerse()

	attributePages(segs, pageStarts, pageHTML)
	return segs
}

// prependNumber puts a paragraph number in front of a block that does not
// carry one — the verse case, where the number lives in its own block — and
// shifts the recorded offsets to match. A block that already starts with a
// digit is left alone: the source wrote its number there itself.
func prependNumber(text, num string, variants []Variant, bold []Span) (string, []Variant, []Span) {
	if num == "" || text == "" {
		return text, variants, bold
	}
	if r, _ := utf8.DecodeRuneInString(text); unicode.IsDigit(r) {
		return text, variants, bold
	}
	prefix := num + ". "
	shift := utf16Len(prefix)
	text = prefix + text
	for i := range variants {
		variants[i].Offset += shift
	}
	for i := range bold {
		bold[i].Offset += shift
	}
	return text, variants, bold
}

// attributePages fills PageNo for each segment from its byte position.
func attributePages(segs []Segment, starts []int, pageHTML string) {
	if len(starts) == 0 {
		return
	}
	// Recover each segment's offset by walking the page boundaries in step
	// with the segment order: both are monotone, so one pass is enough.
	// The offsets are approximated by re-finding the segment text; text is
	// unique enough within a page for this to be exact in practice, and a
	// wrong page number is cosmetic, never an anchor.
	page := 1
	searchFrom := 0
	for i := range segs {
		needle := segs[i].Text
		if needle == "" {
			segs[i].PageNo = page
			continue
		}
		if len(needle) > 60 {
			needle = needle[:60]
		}
		at := strings.Index(pageHTML[searchFrom:], needle)
		if at < 0 {
			at = strings.Index(pageHTML, needle)
			if at < 0 {
				segs[i].PageNo = page
				continue
			}
		} else {
			at += searchFrom
		}
		searchFrom = at
		for page < len(starts) && starts[page] <= at {
			page++
		}
		segs[i].PageNo = page
	}
}

// scan pulls every paragraph block out of the page HTML in document order,
// carrying the paragraph number and the edition marker that precede it.
func scan(pageHTML string) []block {
	var out []block
	para := 0
	pendingMarker := ""
	for _, m := range tokenRe.FindAllStringSubmatch(pageHTML, -1) {
		if m[1] != "" { // an anchor
			name := m[1]
			if pm := paraRe.FindStringSubmatch(name); pm != nil {
				if n := atoi(pm[1]); n > 0 {
					para = n
				}
				continue
			}
			if mm := markerRe.FindStringSubmatch(name); mm != nil {
				pendingMarker = name
			}
			continue
		}
		inner := m[3]
		if pm := innerParaRe.FindStringSubmatch(inner); pm != nil {
			if n := atoi(pm[1]); n > 0 {
				para = n
			}
		}
		// The number the source writes into the text is left where it is.
		// It was briefly stripped and redrawn in the margin, which was wrong:
		// the canon's own numbering is the one scholars checked by hand, and a
		// number we synthesise carries no such guarantee. It is read here only
		// so a segment can also be cited by it when no anchor gives one.
		if lead := leadAnchorsRe.FindString(inner); lead != "" {
			if nm := leadNumRe.FindStringSubmatch(inner[len(lead):]); nm != nil && para == 0 {
				if n := atoi(strings.TrimLeft(nm[1], "0")); n > 0 {
					para = n
				}
			}
		}
		out = append(out, block{
			class:  m[2],
			inner:  inner,
			paraNo: para,
			marker: pendingMarker,
		})
		pendingMarker = ""
	}
	return out
}

// Sentinels wrap the two things that have to survive having their markup
// stripped: a variant reading (which is removed from the text but must be
// locatable in it) and a bold run (which stays, but whose extent has to be
// recorded). They are control characters that cannot occur in the corpus.
const (
	sentinel  = "\u0000" // variant: sentinel index sentinel
	boldOpen  = "\u0001" // bold: boldOpen index boldOpen
	boldClose = "\u0002" //       boldClose index boldClose
)

// SplitRuns returns the plain text of a block together with the two things the
// markup carried: the variant readings that were removed from it, and the runs
// the edition sets in bold.
func SplitRuns(inner string) (string, []Variant, []Span) {
	var notes []string
	marked := noteSpanRe.ReplaceAllStringFunc(inner, func(m string) string {
		notes = append(notes, strings.TrimSpace(noteSpanRe.FindStringSubmatch(m)[1]))
		return fmt.Sprintf("%s%d%s", sentinel, len(notes)-1, sentinel)
	})
	marked = boldSpanRe.ReplaceAllStringFunc(marked, func(m string) string {
		i := len(notes)
		notes = append(notes, "")
		return fmt.Sprintf("%s%d%s%s%s%d%s", boldOpen, i, boldOpen,
			boldSpanRe.FindStringSubmatch(m)[1], boldClose, i, boldClose)
	})

	text := PlainText(marked)

	var variants []Variant
	var spans []Span
	var b strings.Builder
	units := 0
	open := map[int]int{} // bold index -> start offset

	rest := text
	for rest != "" {
		// Whichever marker comes first.
		i, kind := nextMarker(rest)
		if i < 0 {
			b.WriteString(rest)
			break
		}
		head := rest[:i]
		b.WriteString(head)
		units += utf16Len(head)
		rest = rest[i:]

		switch kind {
		case 'v':
			rest = rest[len(sentinel):]
			j := strings.Index(rest, sentinel)
			if j < 0 {
				break
			}
			if idx, err := strconv.Atoi(rest[:j]); err == nil && idx >= 0 && idx < len(notes) {
				variants = append(variants, Variant{Offset: units, Text: notes[idx]})
			}
			rest = rest[j+len(sentinel):]
		case 'b':
			rest = rest[len(boldOpen):]
			j := strings.Index(rest, boldOpen)
			if j < 0 {
				break
			}
			if idx, err := strconv.Atoi(rest[:j]); err == nil {
				open[idx] = units
			}
			rest = rest[j+len(boldOpen):]
		case 'e':
			rest = rest[len(boldClose):]
			j := strings.Index(rest, boldClose)
			if j < 0 {
				break
			}
			if idx, err := strconv.Atoi(rest[:j]); err == nil {
				if start, ok := open[idx]; ok && units > start {
					spans = append(spans, Span{Offset: start, Length: units - start})
				}
			}
			rest = rest[j+len(boldClose):]
		}
	}
	return b.String(), variants, spans
}

// nextMarker finds the earliest of the three sentinels in s.
func nextMarker(s string) (int, byte) {
	best := -1
	var kind byte
	for _, c := range []struct {
		m string
		k byte
	}{{sentinel, 'v'}, {boldOpen, 'b'}, {boldClose, 'e'}} {
		if i := strings.Index(s, c.m); i >= 0 && (best < 0 || i < best) {
			best, kind = i, c.k
		}
	}
	return best, kind
}

// utf16Len counts UTF-16 code units, which is how a JavaScript string is
// indexed. The client slices the text by these offsets.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16Units(r)
	}
	return n
}

// utf16Units is how many UTF-16 code units one rune occupies: two above the
// basic multilingual plane, one otherwise. Pāḷi itself is entirely inside the
// BMP, so this only ever matters for the odd editorial symbol.
func utf16Units(r rune) int {
	if r > 0xFFFF {
		return 2
	}
	return 1
}

// PlainText strips all markup and variant readings, and collapses whitespace.
// This is the text the reader sees and the tokeniser runs over.
func PlainText(inner string) string {
	s := noteSpanRe.ReplaceAllString(inner, "")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// Markup returns the inline HTML the reader may show: variant readings become
// <span class="v">, commentary emphasis keeps <span class="bld">, everything
// else is dropped. Anchor elements are removed — the numbers they carry live
// on the segment.
func Markup(inner string) string {
	s := strings.ReplaceAll(inner, "<a name=\"", "<a data-x=\"")
	s = tagRe.ReplaceAllStringFunc(s, func(tag string) string {
		low := strings.ToLower(tag)
		switch {
		case strings.HasPrefix(low, "<span class=\"note\""):
			return `<span class="v">`
		case strings.HasPrefix(low, "<span class=\"bld\""):
			return `<span class="b">`
		case strings.HasPrefix(low, "<span"):
			return ""
		case strings.HasPrefix(low, "</span"):
			return "</span>"
		default:
			return ""
		}
	})
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// Markers returns the edition page anchors contained in a block.
func Markers(inner string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`<a name="([^"]*)">`).FindAllStringSubmatch(inner, -1) {
		if markerRe.MatchString(m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return n
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}
