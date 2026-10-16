package triageevidence

import (
	"regexp"
	"strings"
)

// Keywords derives the bounded, deterministic search terms used to find related
// issues, PRs, and releases for one report. Terms come from the title and the
// affected area: alphabetic tokens of at least minLen runes, stopwords removed,
// deduplicated, capped at max terms, in first-appearance order. No LLM, no
// randomness, and identical input always yields identical terms.
func Keywords(title, area string, max, minLen int) []string {
	if max <= 0 {
		return nil
	}
	if minLen <= 0 {
		minLen = 4
	}
	var out []string
	seen := map[string]bool{}
	for _, token := range strings.Fields(strings.ToLower(title + " " + area)) {
		clean := tokenPattern.FindString(token)
		if len(clean) < minLen || stopwords[clean] || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
		if len(out) >= max {
			break
		}
	}
	return out
}

// tokenPattern keeps only alphabetic runs so "3.4.0", "R3-001", backticks, and
// punctuation never leak into a search query.
var tokenPattern = regexp.MustCompile(`[a-z]+`)

// stopwords are closed-class English words plus the template's own section
// words, which carry no discriminating signal for a related search. Kept as a
// small conservative set: a false exclusion only costs one term.
var stopwords = map[string]bool{
	"the": true, "this": true, "that": true, "with": true, "from": true,
	"into": true, "when": true, "what": true, "your": true, "after": true,
	"before": true, "about": true, "does": true, "will": true, "should": true,
	"still": true, "there": true, "here": true, "then": true, "than": true,
	"also": true, "only": true, "some": true, "more": true, "most": true,
	"other": true, "such": true, "very": true, "just": true, "have": true,
	"been": true, "were": true, "being": true, "they": true, "them": true,
	"which": true, "would": true, "could": true, "because": true, "issue": true,
	"issues": true, "error": true, "errors": true, "report": true, "reports": true,
	"fix": true, "fixes": true, "fixed": true, "bug": true, "bugs": true,
	"work": true, "works": true, "working": true, "get": true, "gets": true,
	"make": true, "makes": true, "made": true, "see": true, "seen": true,
	"using": true, "used": true, "use": true, "via": true, "per": true,
}
