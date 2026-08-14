package reviewtransaction

import (
	"errors"
	"strings"
	"testing"
)

// TestAuthorityFromANewerReleaseIsNamedAsSuch covers #2461's rc.6 report.
//
// Persisted compact authority is decoded with DisallowUnknownFields. That is
// correct for tamper resistance, but it makes authority non-forward-compatible
// by construction: every release that adds a state field produces bytes an
// older binary can never read. There is already a compatibility path for the
// mirror case, retiredCompactFieldError, and none for this one, because the
// binary that needs to change is the old one and it cannot be patched
// retroactively.
//
// What CAN be fixed is the story the refusal tells. The reporter received
// `json: unknown field "correction_budget_policy"` wrapped in "refresh the
// exact native next_transition before retrying", followed that advice exactly,
// and got an identical failure, because refreshing a transition cannot make an
// older binary parse newer bytes. The refusal has to name the real situation
// and the only action that resolves it.
func TestAuthorityFromANewerReleaseIsNamedAsSuch(t *testing.T) {
	// A record a later release wrote: valid in every way this build knows,
	// plus one state field it has never heard of.
	payload := []byte(`{
		"schema": "` + compactRecordSchema + `",
		"revision": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		"state": {"lineage_id": "review-0123456789abcdef", "field_a_later_release_added": "floor_two"}
	}`)

	_, err := parseCompactRecord(payload, "review-0123456789abcdef")
	if err == nil {
		t.Fatal("parseCompactRecord accepted a field this build does not know; strict decoding is the tamper guard and must stay")
	}
	if !errors.Is(err, ErrCompactAuthorityFromNewerRelease) {
		t.Fatalf("error = %v, want it to unwrap to ErrCompactAuthorityFromNewerRelease so callers can name the situation instead of guessing", err)
	}

	message := err.Error()
	// The unknown field is the one piece of evidence that identifies which
	// release wrote it, so it is preserved rather than swallowed.
	if !strings.Contains(message, "field_a_later_release_added") {
		t.Fatalf("refusal drops the field that identifies the writing release: %q", message)
	}
	for _, want := range []string{"newer", "upgrade"} {
		if !strings.Contains(strings.ToLower(message), want) {
			t.Fatalf("refusal does not name the situation or the action (%q missing): %q", want, message)
		}
	}
}

// TestRetiredFieldCompatibilityStillWins proves the new classification did not
// capture the mirror case. A retired field must still take the tolerant
// historical parse, because that authority IS readable and refusing it would
// strand records this build is explicitly meant to keep loading.
func TestRetiredFieldCompatibilityStillWins(t *testing.T) {
	if len(compactRetiredStateFieldPaths) == 0 {
		t.Skip("no retired compatibility fields to exercise")
	}
	var retired string
	for path := range compactRetiredStateFieldPaths {
		segments := strings.Split(path, ".")
		retired = segments[len(segments)-1]
		break
	}
	err := errors.New(`json: unknown field "` + retired + `"`)
	if compactAuthorityFromNewerRelease(err) {
		t.Fatalf("retired field %q was classified as authority from a newer release; it is the opposite case and has its own tolerant parse", retired)
	}
}
