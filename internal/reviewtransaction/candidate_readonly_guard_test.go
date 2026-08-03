package reviewtransaction

import (
	"testing"
)

// This file is the AST guard for Wave 3 Slice 1's promotion rename (design
// decision 2): shadow_relation.go and shadow_identity.go were promoted to
// candidate_relation.go and candidate_identity.go so one implementation
// serves both the shadow observer and the live ReviewCore/AuthorityStore
// (Wave 3 Slice 2+). Promotion narrows the read-only guard's coverage
// rather than retiring it — the promoted files must still never mutate
// CompactState, Store, or CompactStore; only ReviewCore/AuthorityStore are
// ever allowed to (and neither exists yet in this slice). It reuses
// scanShadowReadOnlyTree (via scanShadowReadOnlySourceFile,
// shadow_readonly_guard_test.go) verbatim — the AST scan itself is already
// proven correct there against synthetic sources; this file only changes
// which files it walks.
//
// This intentionally names the exact two promoted files rather than
// globbing "candidate_*.go": this package already has unrelated
// candidate_decline.go (Wave 2) that a glob would silently sweep in,
// masking a genuine RED state before the Slice 1 rename lands.

// promotedCandidateFiles is the exact, closed set of files this guard
// covers — Wave 3 Slice 1's promotion of the Wave 1 shadow relation/
// identity resolver. A new promoted file must be added here explicitly,
// never picked up implicitly by a glob.
var promotedCandidateFiles = []string{
	"candidate_relation.go",
	"candidate_identity.go",
}

// TestCandidateReadOnlyGuardHoldsForPromotedFiles runs the same scanner
// against every promoted production file and fails with the exact
// violation evidence if any is found. Before the Slice 1 rename lands,
// this fails outright (the named files do not exist yet) — the guard
// itself proves it is watching, not merely finding nothing to complain
// about.
func TestCandidateReadOnlyGuardHoldsForPromotedFiles(t *testing.T) {
	for _, file := range promotedCandidateFiles {
		t.Run(file, func(t *testing.T) {
			violations, err := scanShadowReadOnlySourceFile(file)
			if err != nil {
				t.Fatalf("scanShadowReadOnlySourceFile(%s): %v", file, err)
			}
			if len(violations) > 0 {
				t.Fatalf("%s references mutation shapes: %v", file, violations)
			}
		})
	}
}
