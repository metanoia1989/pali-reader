package importer

import (
	"html"
	"regexp"
	"strings"
)

// DPD embeds presentation markup in a few text fields. The useful case is the
// case ending in a compound split, which it marks with <b>:
//
//	aḍḍha + daṇḍak<b>assa</b>
//
// That emphasis is worth keeping — it is exactly what a learner is looking at
// when they wonder why the second member of the compound is not in its
// dictionary form.
//
// The reader never renders HTML, so the tags are rewritten into two sentinel
// bytes that the client turns back into emphasis. Any other tag is dropped and
// entities are decoded, so what leaves here is plain text plus two markers that
// cannot appear in the source.
const (
	markEmphasisOn  = "\u0001"
	markEmphasisOff = "\u0002"
)

var (
	anyTagRe   = regexp.MustCompile(`(?s)<[^>]*>`)
	emphasisRe = regexp.MustCompile(`(?i)</?b\s*/?>`)
)

// SanitizeMarkup turns a DPD text field into plain text with emphasis markers.
func SanitizeMarkup(s string) string {
	if s == "" {
		return ""
	}
	if !strings.ContainsRune(s, '<') && !strings.ContainsRune(s, '&') {
		return s
	}
	s = emphasisRe.ReplaceAllStringFunc(s, func(tag string) string {
		if strings.HasPrefix(tag, "</") {
			return markEmphasisOff
		}
		return markEmphasisOn
	})
	s = anyTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(s)
}

// StripMarkup is SanitizeMarkup for places that want no emphasis at all.
func StripMarkup(s string) string {
	return strings.NewReplacer(markEmphasisOn, "", markEmphasisOff, "").
		Replace(SanitizeMarkup(s))
}
