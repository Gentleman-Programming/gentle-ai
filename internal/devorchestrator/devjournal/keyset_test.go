package devjournal

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"
)

// forbiddenKeyPattern guards against a future field that would let devjournal
// author phase, next-recommended, apply-state, or dependency truth — all of
// which must be read from sddstatus.StatusV1Projection and never persisted
// here (design D2, binding decision Q5).
var forbiddenKeyPattern = regexp.MustCompile(`(?i)phase|nextrecommended|applystate|dependenc`)

// allowedJournalKeys is the exact, frozen top-level key set of a persisted
// Record. Adding a key here is a deliberate schema revision; a key matching
// forbiddenKeyPattern is never allowed, regardless.
var allowedJournalKeys = map[string]bool{
	"schema": true, "change": true, "updated_at": true,
	"status_digest": true, "cursor": true, "dispatches": true,
}

// TestRecordJSONKeySetIsFrozen marshals a fully-populated Record and asserts
// its top-level JSON key set equals the exact allowlist above and that no key
// matches forbiddenKeyPattern. This catches a phase-like field added under
// ANY name, not just literally "phase" — a build-time guard the AST import
// check (TestPackageImportsExcludeSDDStatus) cannot provide on its own.
func TestRecordJSONKeySetIsFrozen(t *testing.T) {
	rec := Record{
		Schema:       SchemaV1,
		Change:       "example-change",
		UpdatedAt:    time.Now().UTC(),
		StatusDigest: "sha256:deadbeefcafef00d",
		Cursor:       Cursor{BatchIndex: 1, RepoSlug: "repo-a"},
		Dispatches: []Dispatch{{
			RepoSlug: "repo-a", Agent: "backend-implementer", Attempt: 1,
			Outcome: OutcomeDispatched, StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(),
		}},
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal into raw map: %v", err)
	}

	if len(raw) != len(allowedJournalKeys) {
		t.Fatalf("Record has %d top-level JSON keys %v, want exactly the %d-key allowlist %v", len(raw), raw, len(allowedJournalKeys), allowedJournalKeys)
	}
	for key := range raw {
		if !allowedJournalKeys[key] {
			t.Fatalf("Record persists unexpected top-level key %q; the frozen allowlist is %v", key, allowedJournalKeys)
		}
		if forbiddenKeyPattern.MatchString(key) {
			t.Fatalf("Record persists key %q, matching the forbidden phase/next-recommended/apply-state/dependency pattern", key)
		}
	}
}
