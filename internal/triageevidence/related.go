package triageevidence

// RelatedKind is what kind of candidate a related-change search hit.
type RelatedKind string

const (
	// RelatedIssue is another issue found by the bounded related search.
	RelatedIssue RelatedKind = "issue"
	// RelatedPR is a pull request found by the bounded related search.
	RelatedPR RelatedKind = "pull-request"
	// RelatedRelease is a release whose notes match the report's area/terms.
	RelatedRelease RelatedKind = "release"
	// RelatedKnownChange is an explicitly corroborated change (e.g. a merged
	// commit the maintainer already knows touches this area).
	RelatedKnownChange RelatedKind = "known-change"
)

// RelatedChange is one bounded candidate produced by the related search. The
// classifier never fetches anything itself: the caller assembles these and the
// report cites the URLs verbatim (escaped).
type RelatedChange struct {
	Kind    RelatedKind
	Number  int // issue/PR number; 0 for releases and known changes
	Title   string
	URL     string
	State   string          // "open", "merged", "closed", or a release tag
	Version ReportedVersion // release version, when Kind is RelatedRelease
}

// IsReleasedChange reports whether this candidate proves a change already
// landed: a merged PR, a release, or a known-change corroboration.
func (r RelatedChange) IsReleasedChange() bool {
	switch r.Kind {
	case RelatedRelease, RelatedKnownChange:
		return true
	case RelatedPR:
		return r.State == "merged"
	default:
		return false
	}
}

// IsInFlight reports whether the candidate is work that exists but has not
// landed: an open PR, or an open issue referencing this report.
func (r RelatedChange) IsInFlight() bool {
	switch r.Kind {
	case RelatedIssue:
		return r.State == "open"
	case RelatedPR:
		return r.State == "open"
	default:
		return false
	}
}
