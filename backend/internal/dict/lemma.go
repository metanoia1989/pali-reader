package dict

import (
	"strconv"
	"strings"
)

// CleanLemma strips DPD's homonym numbering, so "buddha 1" and "akaṅkha 2.1"
// display as "buddha" and "akaṅkha".
func CleanLemma(lemma string) string {
	if i := strings.LastIndexByte(lemma, ' '); i > 0 {
		if _, err := strconv.ParseFloat(lemma[i+1:], 64); err == nil {
			return lemma[:i]
		}
	}
	return lemma
}

// Homonym returns the numbering CleanLemma removed, or "" when there is none.
// The panel shows it so two entries that share a spelling stay distinguishable.
func Homonym(lemma string) string {
	if i := strings.LastIndexByte(lemma, ' '); i > 0 {
		if _, err := strconv.ParseFloat(lemma[i+1:], 64); err == nil {
			return lemma[i+1:]
		}
	}
	return ""
}
