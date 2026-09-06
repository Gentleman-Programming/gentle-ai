package sddstatus

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestBeginAttemptRequestDigestIsIndependentOfMaxChangedLinesExplicit pins
// the exact property a refuter escalated on: MaxChangedLinesExplicit
// (#2589 provenance) must never change BeginAttemptRequest's request digest.
// It is excluded entirely (json:"-"), not merely omitted when false, so an
// in-flight ledger holding explicit-budget begins written by a binary that
// predates this field replays against the identical digest after an
// upgrade, whichever value this field happens to hold.
func TestBeginAttemptRequestDigestIsIndependentOfMaxChangedLinesExplicit(t *testing.T) {
	base := BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "digest-parity", WorkUnit: "unit", EvidenceGoal: "goal",
		MaxAttempts: 2, MaxChangedLines: 40, IntendedUntracked: []string{},
	}
	explicit := base
	explicit.MaxChangedLinesExplicit = true
	defaulted := base
	defaulted.MaxChangedLinesExplicit = false

	explicitDigest := runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", explicit)
	defaultedDigest := runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", defaulted)
	if explicitDigest != defaultedDigest {
		t.Fatalf("digest depends on MaxChangedLinesExplicit: explicit=%s defaulted=%s", explicitDigest, defaultedDigest)
	}
}

// TestRuntimeLedgerLegacyShapedBeginEventReplaysAndDigestMatches is the
// end-to-end counterpart: a begin whose budget was NOT explicit persists an
// event with no max_changed_lines_explicit key at all (omitempty), which is
// byte-for-byte the same shape a binary written before this field existed
// would have produced. A fresh store re-reading that record from disk must
// replay it without begin_request_digest_match, and must report the
// provenance as "default" rather than asserting an explicit value it was
// never told.
func TestRuntimeLedgerLegacyShapedBeginEventReplaysAndDigestMatches(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "legacy-shaped-begin")
	if err != nil {
		t.Fatal(err)
	}
	began, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "legacy-begin-1", WorkUnit: "legacy-unit",
		EvidenceGoal: "prove legacy replay", MaxAttempts: 2, MaxChangedLines: DefaultRuntimeChangedLines,
		MaxChangedLinesExplicit: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(store.recordPath(began.Revision))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "max_changed_lines_explicit") {
		t.Fatalf("a non-explicit begin persisted the provenance key, which a legacy record could never have had:\n%s", raw)
	}

	reopened, err := OpenRuntimeStore(context.Background(), repo, "legacy-shaped-begin")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := reopened.Status()
	if err != nil {
		t.Fatalf("replay a legacy-shaped begin event: %v", err)
	}
	if replayed.Objective == nil || replayed.Objective.MaxChangedLines != DefaultRuntimeChangedLines ||
		replayed.Objective.MaxChangedLinesSource != runtimeChangedLinesSourceDefault || replayed.Revision != began.Revision {
		t.Fatalf("replayed legacy-shaped objective = %#v", replayed.Objective)
	}
}

// TestRuntimeLedgerExplicitBeginRoundTrips is the explicit counterpart: an
// explicit budget persists the marker and survives a fresh replay with its
// provenance intact.
func TestRuntimeLedgerExplicitBeginRoundTrips(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "explicit-begin-roundtrip")
	if err != nil {
		t.Fatal(err)
	}
	began, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "explicit-begin-1", WorkUnit: "explicit-unit",
		EvidenceGoal: "prove explicit round trip", MaxAttempts: 2, MaxChangedLines: 40,
		MaxChangedLinesExplicit: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if began.Objective == nil || began.Objective.MaxChangedLinesSource != runtimeChangedLinesSourceExplicit {
		t.Fatalf("began objective = %#v, want source %q", began.Objective, runtimeChangedLinesSourceExplicit)
	}

	raw, err := os.ReadFile(store.recordPath(began.Revision))
	if err != nil {
		t.Fatal(err)
	}
	var onDisk struct {
		Begin struct {
			MaxChangedLinesExplicit bool `json:"max_changed_lines_explicit"`
		} `json:"begin"`
	}
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatal(err)
	}
	if !onDisk.Begin.MaxChangedLinesExplicit {
		t.Fatalf("persisted record did not carry the explicit marker:\n%s", raw)
	}

	reopened, err := OpenRuntimeStore(context.Background(), repo, "explicit-begin-roundtrip")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := reopened.Status()
	if err != nil {
		t.Fatalf("replay an explicit begin event: %v", err)
	}
	if replayed.Objective == nil || replayed.Objective.MaxChangedLines != 40 ||
		replayed.Objective.MaxChangedLinesSource != runtimeChangedLinesSourceExplicit || replayed.Revision != began.Revision {
		t.Fatalf("replayed explicit objective = %#v", replayed.Objective)
	}
}
