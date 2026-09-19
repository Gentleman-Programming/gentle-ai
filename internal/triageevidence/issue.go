package triageevidence

import (
	"regexp"
	"strconv"
	"strings"
)

// Issue is the trimmed GitHub issue record the classifier needs. The caller is
// responsible for fetching it; this package only reads.
type Issue struct {
	Number    int
	Title     string
	Body      string
	Labels    []string
	CreatedAt string // ISO date, quoted verbatim (never used for closure)
	HTMLURL   string
}

// HasLabel reports whether the issue carries the exact label name.
func (i Issue) HasLabel(name string) bool {
	for _, l := range i.Labels {
		if l == name {
			return true
		}
	}
	return false
}

// IsBugReport treats "bug" and "type:bug" as the same input signal, without
// ever proposing a label change (the caller owns labels).
func (i Issue) IsBugReport() bool {
	return i.HasLabel("bug") || i.HasLabel("type:bug")
}

// MatchesReference reports whether the title/body mention the exact issue
// number as "#N" (never as a substring of a larger "#NNNN"), which is the
// strongest cheap signal that a candidate change is actually related.
func (i Issue) MatchesReference(number int) bool {
	return hasExactRef(i.Title, number) || hasExactRef(i.Body, number)
}

// refToken matches a "#N" reference; numbers shorter than the target are
// rejected so "#4711" never counts as a reference to 47.
var refToken = regexp.MustCompile(`#\d+`)

func hasExactRef(s string, number int) bool {
	for _, tok := range refToken.FindAllString(s, -1) {
		n, err := strconv.Atoi(strings.TrimPrefix(tok, "#"))
		if err == nil && n == number {
			return true
		}
	}
	return false
}
