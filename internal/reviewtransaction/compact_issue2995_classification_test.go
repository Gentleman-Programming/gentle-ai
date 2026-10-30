package reviewtransaction

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Issue #2995 work-unit 1: classification honesty for released-v2.2.x compact
// records. A genuine v2.2.0 binary wrote the two released fixtures under
// testdata/issue2995 (see PROVENANCE.md there for the tarball, tag, and
// per-record SHA-256 pins). The reader must classify them as outdated
// compact authority -- never as damaged bytes, and never as authority from a
// newer release -- without making any of them loadable as operational state.

const (
	issue2995ApprovedLineage  = "review-a39b858db5f00bbb"
	issue2995EscalatedLineage = "review-a3118da0f4a4c425"
)

func issue2995FixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("testdata", "issue2995", name))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	t.Logf("fixture %s sha256:%s", name, hex.EncodeToString(sum[:]))
	return payload
}

// issue2995Classify runs the exact classification consumers derive from a
// store load failure: compactRecoveryEntryProblem maps the typed error to the
// entry class the inspection surfaces report.
func issue2995Classify(t *testing.T, payload []byte, lineageID string) (string, *CompactSemanticStateError) {
	t.Helper()
	_, err := parseCompactRecord(payload, lineageID)
	if err == nil {
		t.Fatalf("record %s unexpectedly loaded as operational state; the #2743 clean break must keep retired-identity records unloadable", lineageID)
	}
	var semantic *CompactSemanticStateError
	if errors.As(err, &semantic) {
		if semantic.OutdatedIdentity {
			return compactInspectionEntryOutdated, semantic
		}
		return compactInspectionEntryMalformed, semantic
	}
	if errors.Is(err, ErrCompactAuthorityFromNewerRelease) {
		return "newer_release", nil
	}
	return compactInspectionEntryMalformed, nil
}

// issue2995OutdateSnapshotIdentities rewrites both frozen snapshot identities
// in an already checksum-valid record payload to the retired pre-rc.2
// formula's own values -- exactly what a 2.2.x binary persisted -- and
// rebinds the record revision over the rewritten bytes. It returns the
// updated payload plus the retired identity now carried by the initial
// snapshot.
func issue2995OutdateSnapshotIdentities(t *testing.T, payload []byte) ([]byte, string) {
	t.Helper()
	var record CompactRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	outdated := payload
	retiredInitial := ""
	for _, snapshot := range []*Snapshot{&record.State.InitialSnapshot, &record.State.CurrentSnapshot} {
		retired := retiredCompactSnapshotIdentity(*snapshot)
		if retired == snapshot.Identity {
			t.Fatalf("retired identity formula reproduced the current identity %q; the fixture would prove nothing", snapshot.Identity)
		}
		if snapshot == &record.State.InitialSnapshot {
			retiredInitial = retired
		}
		outdated = bytes.ReplaceAll(outdated, []byte(`"identity": "`+snapshot.Identity+`"`), []byte(`"identity": "`+retired+`"`))
		snapshot.Identity = retired
	}
	if bytes.Equal(outdated, payload) {
		t.Fatal("record did not contain the expected snapshot identity markers")
	}
	_, updated, err := makeCompactRecord(record.State)
	if err != nil {
		t.Fatal(err)
	}
	return updated, retiredInitial
}

// issue2995InjectStateFields injects raw state fields into a record payload
// and rebinds the revision over the injected state bytes under the exact
// formula the historical tolerant parse verifies (the historical writer's own
// binding), so the injected record stays self-consistent.
func issue2995InjectStateFields(t *testing.T, payload []byte, fields map[string]any) []byte {
	t.Helper()
	var record map[string]any
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	state, ok := record["state"].(map[string]any)
	if !ok {
		t.Fatal("compact record has no state object")
	}
	for name, value := range fields {
		state[name] = value
	}
	statePayload, err := json.Marshal(record["state"])
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(append([]byte(CompactStateSchema+"\x00"), statePayload...))
	record["revision"] = "sha256:" + hex.EncodeToString(sum[:])
	updated, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(updated, '\n')
}

func issue2995SHA(t *testing.T, seed string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(seed))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// TestIssue2995ReleasedApprovedRecordClassifiesOutdatedNotMalformed pins the
// approved fixture: a released v2.2.0 approved record whose retired
// lens_results projection is NON-empty must classify as outdated compact
// authority through the OutdatedIdentity proof, not as malformed damage, and
// it must never load as operational state.
func TestIssue2995ReleasedApprovedRecordClassifiesOutdatedNotMalformed(t *testing.T) {
	payload := issue2995FixtureBytes(t, "released-v2.2.0-approved-review-state.json")
	classification, semantic := issue2995Classify(t, payload, issue2995ApprovedLineage)
	if classification != compactInspectionEntryOutdated {
		problem := ""
		if semantic != nil {
			problem = semantic.Problem
		}
		t.Fatalf("released v2.2.0 approved record classified %q (typed semantic error: %v) with problem %q, want %q", classification, semantic != nil, problem, compactInspectionEntryOutdated)
	}
	if semantic == nil || !semantic.OutdatedIdentity {
		t.Fatalf("released approved record lacks the OutdatedIdentity proof: %+v", semantic)
	}
	if !strings.Contains(semantic.Problem, "identity does not match its metadata") {
		t.Fatalf("outdated classification problem %q does not name the retired snapshot identity", semantic.Problem)
	}
}

// TestIssue2995ReleasedEscalatedRecordClassifiesOutdatedWithoutNewerReleaseWrap
// pins the escalated fixture: a released v2.2.0 escalated record carrying the
// v2.5.0-deleted top-level result_dispositions array must classify as
// outdated compact authority. Misattributing those bytes to a newer release
// (ErrCompactAuthorityFromNewerRelease) is exactly the dishonesty #2995
// removes.
func TestIssue2995ReleasedEscalatedRecordClassifiesOutdatedWithoutNewerReleaseWrap(t *testing.T) {
	payload := issue2995FixtureBytes(t, "released-v2.2.0-escalated-review-state.json")
	_, err := parseCompactRecord(payload, issue2995EscalatedLineage)
	if err == nil {
		t.Fatal("released escalated record loaded as operational state; the #2743 clean break must keep it unloadable")
	}
	if errors.Is(err, ErrCompactAuthorityFromNewerRelease) {
		t.Fatalf("released v2.2.0 bytes were misattributed to a newer release: %v", err)
	}
	classification, semantic := issue2995Classify(t, payload, issue2995EscalatedLineage)
	if classification != compactInspectionEntryOutdated {
		problem := ""
		if semantic != nil {
			problem = semantic.Problem
		}
		t.Fatalf("released v2.2.0 escalated record classified %q (typed semantic error: %v) with problem %q, want %q", classification, semantic != nil, problem, compactInspectionEntryOutdated)
	}
}

// TestIssue2995DamagedBytesRemainMalformed proves the tolerance is not a
// blanket downgrade: genuinely damaged or forged bytes must keep failing as
// malformed compact state, never as outdated history.
func TestIssue2995DamagedBytesRemainMalformed(t *testing.T) {
	t.Run("truncated json", func(t *testing.T) {
		payload := issue2995FixtureBytes(t, "released-v2.2.0-approved-review-state.json")
		classification, _ := issue2995Classify(t, payload[:len(payload)/2], issue2995ApprovedLineage)
		if classification != compactInspectionEntryMalformed {
			t.Fatalf("truncated record classified %q, want %q", classification, compactInspectionEntryMalformed)
		}
	})
	t.Run("corrupted retired-field record checksum", func(t *testing.T) {
		payload := issue2995FixtureBytes(t, "released-v2.2.0-approved-review-state.json")
		marker := []byte(`"policy_hash": "sha256:`)
		at := bytes.Index(payload, marker)
		if at < 0 {
			t.Fatal("fixture has no policy_hash marker to corrupt")
		}
		payload[at+len(marker)] = '0' // flip one digest nibble without rebinding the revision
		classification, _ := issue2995Classify(t, payload, issue2995ApprovedLineage)
		if classification != compactInspectionEntryMalformed {
			t.Fatalf("checksum-corrupted record classified %q, want %q", classification, compactInspectionEntryMalformed)
		}
	})
	t.Run("forged identity with empty arrays", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		lineage := "issue2995-forged-identity"
		state := newCompactTestState(t, repo, lineage)
		// Empty retired projections make the record strict-decodable exactly
		// like a genuine strict-decodable historical record; the forged
		// identity values are arbitrary sha256 strings that match neither the
		// current nor the retired identity formula.
		forged := state
		forged.HistoricalLensResults = &compactHistoricalEmptyArray{}
		forged.HistoricalFindings = &compactHistoricalEmptyArray{}
		forged.HistoricalClassifications = &compactHistoricalEmptyObject{}
		forged.HistoricalOutcomes = &compactHistoricalEmptyObject{}
		forged.HistoricalFollowUps = &compactHistoricalEmptyArray{}
		forged.InitialSnapshot.Identity = issue2995SHA(t, "forged-initial")
		forged.CurrentSnapshot.Identity = issue2995SHA(t, "forged-current")
		_, payload, err := makeCompactRecord(forged)
		if err != nil {
			t.Fatal(err)
		}
		classification, semantic := issue2995Classify(t, payload, lineage)
		if classification != compactInspectionEntryMalformed {
			t.Fatalf("forged-identity record classified %q (typed semantic error: %v), want %q", classification, semantic != nil, compactInspectionEntryMalformed)
		}
	})
}

// TestIssue2995NonEmptyRetiredLensResultsWithCurrentIdentitiesStillLoads
// proves the tolerance does not loosen the live path: a record whose retired
// lens_results projection is non-empty but whose snapshot identities follow
// the CURRENT formula and whose revision binds its bytes still loads
// read-only through the retired-field compatibility path, exactly as it did
// before this change.
func TestIssue2995NonEmptyRetiredLensResultsWithCurrentIdentitiesStillLoads(t *testing.T) {
	repo := initSnapshotRepo(t)
	lineage := "issue2995-tolerant-load"
	state := newCompactTestState(t, repo, lineage)
	_, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	payload = issue2995InjectStateFields(t, payload, map[string]any{
		"lens_results": []any{map[string]any{"lens": "review-reliability", "results": []any{map[string]any{"verdict": "approve"}}}},
	})
	record, err := parseCompactRecord(payload, lineage)
	if err != nil {
		t.Fatalf("non-empty retired projection with current-formula identities must still load read-only: %v", err)
	}
	if !record.HistoricalCompat {
		t.Fatal("record loaded through the retired-field path must be marked read-only historical compatibility authority")
	}
	if len(record.State.AdmittedRoleResults) != 0 {
		t.Fatalf("retired projection content leaked into operational state: %+v", record.State.AdmittedRoleResults)
	}
	if view, marshalErr := json.Marshal(record.State); marshalErr != nil || bytes.Contains(view, []byte(`"lens_results"`)) {
		t.Fatalf("loaded operational state still carries the retired projection: %s", view)
	}
}

// TestIssue2995RetiredResultReopenSlotsClassifyOutdatedNotNewerRelease pins
// the pre-v2.5 result_reopens slot shape: entries carrying the deleted
// quarantined/retained/authorized_lenses fields must take the retired-field
// tolerance and classify as outdated history -- not strict-decode-fail as
// authority from a newer release.
func TestIssue2995RetiredResultReopenSlotsClassifyOutdatedNotNewerRelease(t *testing.T) {
	repo := initSnapshotRepo(t)
	lineage := "issue2995-reopen-slots"
	state := newCompactTestState(t, repo, lineage)
	record, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	payload, retiredInitial := issue2995OutdateSnapshotIdentities(t, payload)
	payload = issue2995InjectStateFields(t, payload, map[string]any{
		"result_reopens": []any{map[string]any{
			"previous_revision":        record.Revision,
			"target_identity":          retiredInitial,
			"quarantined":              []any{map[string]any{"lens": "review-reliability", "selected_order": 0, "artifact_digest": issue2995SHA(t, "artifact"), "result_hash": issue2995SHA(t, "result")}},
			"retained":                 []any{},
			"authorized_lenses":        []any{},
			"reason":                   "historical quarantine replay",
			"actor":                    "maintainer@example.com",
			"reopened_at":              time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
			"maintainer_authorization": issue2995SHA(t, "authorization"),
		}},
	})
	_, err = parseCompactRecord(payload, lineage)
	if err == nil {
		t.Fatal("record with retired reopen slots loaded as operational state; the #2743 clean break must keep it unloadable")
	}
	if errors.Is(err, ErrCompactAuthorityFromNewerRelease) {
		t.Fatalf("pre-v2.5 result_reopens slot fields were misattributed to a newer release: %v", err)
	}
	classification, semantic := issue2995Classify(t, payload, lineage)
	if classification != compactInspectionEntryOutdated {
		problem := ""
		if semantic != nil {
			problem = semantic.Problem
		}
		t.Fatalf("retired-slot record classified %q (typed semantic error: %v) with problem %q, want %q", classification, semantic != nil, problem, compactInspectionEntryOutdated)
	}
}

// TestIssue2995RetiredFinalVerificationRetryRecoveryClassifiesOutdatedNotNewerRelease
// pins the retired recovery domain: the v2.5.0-deleted final_verification_retry
// disposition with its recovery.final_verification_retry proof field must take
// the retired-field tolerance and classify as outdated history -- not
// strict-decode-fail as authority from a newer release.
func TestIssue2995RetiredFinalVerificationRetryRecoveryClassifiesOutdatedNotNewerRelease(t *testing.T) {
	repo := initSnapshotRepo(t)
	lineage := "issue2995-retry-recovery"
	state := newCompactTestState(t, repo, lineage)
	_, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ = issue2995OutdateSnapshotIdentities(t, payload)
	payload = issue2995InjectStateFields(t, payload, map[string]any{
		"recovery": map[string]any{
			"predecessor_lineage_id":   "issue2995-retry-predecessor",
			"predecessor_revision":     issue2995SHA(t, "predecessor-revision"),
			"disposition":              "final_verification_retry",
			"reason":                   "historical final verification retry",
			"actor":                    "maintainer@example.com",
			"recovered_at":             time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
			"maintainer_authorization": issue2995SHA(t, "retry-authorization"),
			"final_verification_retry": map[string]any{"attempt_digest": issue2995SHA(t, "retry-attempt")},
		},
	})
	_, err = parseCompactRecord(payload, lineage)
	if err == nil {
		t.Fatal("record with a retired recovery disposition loaded as operational state; the #2743 clean break must keep it unloadable")
	}
	if errors.Is(err, ErrCompactAuthorityFromNewerRelease) {
		t.Fatalf("retired final_verification_retry recovery was misattributed to a newer release: %v", err)
	}
	classification, semantic := issue2995Classify(t, payload, lineage)
	if classification != compactInspectionEntryOutdated {
		problem := ""
		if semantic != nil {
			problem = semantic.Problem
		}
		t.Fatalf("retired-recovery record classified %q (typed semantic error: %v) with problem %q, want %q", classification, semantic != nil, problem, compactInspectionEntryOutdated)
	}
}
