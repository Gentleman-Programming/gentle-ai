package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestRDDConsentLatchIsAbsentUntilRecordedAndThenIdempotent locks the property
// the latch relies on: recording the same answer twice must not raise an
// immutable-slot conflict, and the latch must not disturb the override head.
func TestRDDConsentLatchIsAbsentUntilRecordedAndThenIdempotent(t *testing.T) {
	repo := initSnapshotRepo(t)
	ctx := context.Background()

	if asked, err := RDDConsentAsked(ctx, repo); err != nil || asked {
		t.Fatalf("fresh clone latch = %v, %v", asked, err)
	}
	if err := RecordRDDConsentAsked(ctx, repo); err != nil {
		t.Fatalf("record consent: %v", err)
	}
	if err := RecordRDDConsentAsked(ctx, repo); err != nil {
		t.Fatalf("repeated record consent: %v", err)
	}
	if asked, err := RDDConsentAsked(ctx, repo); err != nil || !asked {
		t.Fatalf("latched consent = %v, %v", asked, err)
	}
	// Resolved against an explicit opt-in, because this asserts the latch left
	// the override head alone -- not what an unconfigured clone resolves to.
	// Receipt-driven development is off by default, so passing no opinion here
	// would make the check pass for the wrong reason.
	status, err := ResolveRDDMode(ctx, repo, RDDGlobalMode{Value: string(RDDModeOn)})
	if err != nil {
		t.Fatalf("ResolveRDDMode after latching: %v", err)
	}
	if status.Effective != RDDModeOn || status.Revision != "" {
		t.Fatalf("consent latch disturbed the override head: %#v", status)
	}
}

func TestResolveRDDModeLetsAnyOffWin(t *testing.T) {
	for _, test := range []struct {
		name       string
		global     string
		cloneLocal RDDMode
		effective  RDDMode
		source     RDDModeSource
	}{
		{name: "unconfigured stays off", global: "", cloneLocal: RDDModeUnset, effective: RDDModeOff, source: RDDModeSourceDefault},
		{name: "global off with no override", global: "off", cloneLocal: RDDModeUnset, effective: RDDModeOff, source: RDDModeSourceGlobal},
		{name: "global on with clone off", global: "on", cloneLocal: RDDModeOff, effective: RDDModeOff, source: RDDModeSourceCloneLocal},
		{name: "global off with cleared override", global: "off", cloneLocal: RDDModeUnset, effective: RDDModeOff, source: RDDModeSourceGlobal},
		{name: "global on with no override", global: "on", cloneLocal: RDDModeUnset, effective: RDDModeOn, source: RDDModeSourceGlobal},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			if test.cloneLocal == RDDModeOff {
				if _, err := SetCloneLocalRDDMode(context.Background(), repo, RDDModeOff, "", RDDGlobalMode{Value: test.global}); err != nil {
					t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
				}
			}
			status, err := ResolveRDDMode(context.Background(), repo, RDDGlobalMode{Value: test.global})
			if err != nil {
				t.Fatalf("ResolveRDDMode error = %v", err)
			}
			if status.Effective != test.effective || status.Source != test.source {
				t.Fatalf("effective/source = %q/%q, want %q/%q", status.Effective, status.Source, test.effective, test.source)
			}
			if status.Enabled() != (test.effective == RDDModeOn) {
				t.Fatalf("Enabled() = %v for effective %q", status.Enabled(), status.Effective)
			}
		})
	}
}

// TestResolveRDDModeStaysOffUntilExplicitlyEnabled pins the product default:
// receipt-driven development is opt-in. A fresh install where no source
// expressed an opinion must resolve to off, and it must say the default is why,
// so nothing about the resolution looks like a choice somebody made. The other
// two cases are the reason that flip is safe to ship: an explicit global "on"
// survives an upgrade untouched, and a clone-local "off" still beats it.
func TestResolveRDDModeStaysOffUntilExplicitlyEnabled(t *testing.T) {
	t.Run("nobody chose anything so reviews stay off", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		status, err := ResolveRDDMode(context.Background(), repo, RDDGlobalMode{})
		if err != nil {
			t.Fatalf("ResolveRDDMode error = %v", err)
		}
		if status.Effective != RDDModeOff || status.Source != RDDModeSourceDefault {
			t.Fatalf("unconfigured effective/source = %q/%q, want %q/%q", status.Effective, status.Source, RDDModeOff, RDDModeSourceDefault)
		}
		if status.Enabled() {
			t.Fatalf("an unconfigured clone reported reviews enabled: %#v", status)
		}
		if status.Global != RDDModeUnset || status.CloneLocal != RDDModeUnset {
			t.Fatalf("the default must not invent an opinion for either source: %#v", status)
		}
	})

	t.Run("an explicit global enable survives the new default", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		status, err := ResolveRDDMode(context.Background(), repo, RDDGlobalMode{Value: string(RDDModeOn)})
		if err != nil {
			t.Fatalf("ResolveRDDMode error = %v", err)
		}
		if status.Effective != RDDModeOn || status.Source != RDDModeSourceGlobal {
			t.Fatalf("explicit global on = %q/%q, want %q/%q", status.Effective, status.Source, RDDModeOn, RDDModeSourceGlobal)
		}
		if !status.Enabled() {
			t.Fatalf("a user who deliberately enabled reviews lost them: %#v", status)
		}
	})

	t.Run("a clone-local off still beats an explicit global on", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		global := RDDGlobalMode{Value: string(RDDModeOn)}
		if _, err := SetCloneLocalRDDMode(context.Background(), repo, RDDModeOff, "", global); err != nil {
			t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
		}
		status, err := ResolveRDDMode(context.Background(), repo, global)
		if err != nil {
			t.Fatalf("ResolveRDDMode error = %v", err)
		}
		if status.Effective != RDDModeOff || status.Source != RDDModeSourceCloneLocal {
			t.Fatalf("clone-local off = %q/%q, want %q/%q", status.Effective, status.Source, RDDModeOff, RDDModeSourceCloneLocal)
		}
	})
}

func TestCloneLocalRDDOverrideCannotForceOn(t *testing.T) {
	repo := initSnapshotRepo(t)
	if _, err := SetCloneLocalRDDMode(context.Background(), repo, RDDModeOn, "", RDDGlobalMode{Value: "off"}); !errors.Is(err, ErrRDDModeRepositoryForcedOn) {
		t.Fatalf("SetCloneLocalRDDMode(on) error = %v, want ErrRDDModeRepositoryForcedOn", err)
	}
	status, err := ResolveRDDMode(context.Background(), repo, RDDGlobalMode{Value: "off"})
	if err != nil {
		t.Fatalf("ResolveRDDMode error = %v", err)
	}
	if status.Effective != RDDModeOff || status.Revision != "" {
		t.Fatalf("rejected force-on left state %#v", status)
	}
}

func TestCloneLocalRDDOverrideStaysInsideItsClone(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "clone source\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "clone source")
	if _, err := SetCloneLocalRDDMode(context.Background(), repo, RDDModeOff, "", RDDGlobalMode{Value: "on"}); err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}

	overridePath := filepath.Join(repo, ".git", "gentle-ai", "review-mode", "rar-authority", "v1", "rdd-mode")
	if _, err := os.Stat(overridePath); err != nil {
		t.Fatalf("clone-local override is not stored under the Git common directory: %v", err)
	}

	clone := filepath.Join(t.TempDir(), "clone")
	gitSnapshot(t, repo, "clone", repo, clone)
	status, err := ResolveRDDMode(context.Background(), clone, RDDGlobalMode{Value: "on"})
	if err != nil {
		t.Fatalf("ResolveRDDMode(clone) error = %v", err)
	}
	if status.Effective != RDDModeOn || status.CloneLocal != RDDModeUnset {
		t.Fatalf("second clone inherited the override: %#v", status)
	}
}

func TestResolveRDDModeNeverCreatesState(t *testing.T) {
	repo := initSnapshotRepo(t)
	status, err := ResolveRDDMode(context.Background(), repo, RDDGlobalMode{})
	if err != nil {
		t.Fatalf("ResolveRDDMode error = %v", err)
	}
	if status.Effective != RDDModeOff || status.Source != RDDModeSourceDefault {
		t.Fatalf("unconfigured status = %#v", status)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".git", "gentle-ai")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only resolution created repository state: %v", err)
	}
}

func TestDisabledRDDRejectsStartsAndLeavesCurrentOpenAuthorityIntact(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "rdd-mode-frozen")
	_, store := startReviewingCompactAuthority(t, repo, state)
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	global := RDDGlobalMode{Value: "off", RecordedAt: time.Now().UTC()}
	if _, err := AuthorizeRDDOperation(context.Background(), repo, global, RDDOperationStart); !errors.Is(err, ErrRDDDisabled) {
		t.Fatalf("disabled start error = %v, want ErrRDDDisabled", err)
	}
	var disabled *RDDDisabledError
	_, err = AuthorizeRDDOperation(context.Background(), repo, global, RDDOperationMutate)
	if !errors.As(err, &disabled) || disabled.Operation != RDDOperationMutate {
		t.Fatalf("disabled mutation error = %v, want typed RDDDisabledError", err)
	}
	if _, err := AuthorizeRDDOperation(context.Background(), repo, global, RDDOperationRead); err != nil {
		t.Fatalf("disabled mode broke read-only authority: %v", err)
	}
	if _, err := AuthorizeRDDOperation(context.Background(), repo, global, RDDOperationAbandon); err != nil {
		t.Fatalf("disabled mode rejected sanctioned abandonment: %v", err)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || after.State.State != StateReviewing ||
		after.State.CurrentSnapshot.Identity != before.State.CurrentSnapshot.Identity {
		t.Fatalf("disabled mode changed current open authority: before=%#v after=%#v", before, after)
	}
}

// Re-enabling must leave a recovery path. RDD is post-candidate by design: the
// review freezes a snapshot now and reviews those exact bytes, so when the work
// was authored is irrelevant. Stranding work authored during a disabled window
// would force the user to discard and redo it, which is not a safety property.
func TestReEnabledRDDAuthorizesAFreshReviewOfTheCurrentCandidate(t *testing.T) {
	repo := initSnapshotRepo(t)
	global := RDDGlobalMode{Value: "on", RecordedAt: time.Now().UTC().Add(-time.Hour)}
	disabled, err := SetCloneLocalRDDMode(context.Background(), repo, RDDModeOff, "", global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}
	if disabled.Enabled() {
		t.Fatalf("clone-local disable did not take effect: %#v", disabled)
	}
	var stop *RDDDisabledError
	err = AuthorizeRDDCandidate(disabled)
	if !errors.As(err, &stop) || !errors.Is(err, ErrRDDDisabled) || stop.Operation != RDDOperationStart {
		t.Fatalf("disabled candidate error = %v, want a typed RDDDisabledError start stop", err)
	}

	writeSnapshotFile(t, repo, "recovered.txt", "work authored while review was disabled\n")
	gitSnapshot(t, repo, "add", "recovered.txt")
	gitSnapshot(t, repo, "commit", "-m", "work authored while review was disabled")

	enabled, err := SetCloneLocalRDDMode(context.Background(), repo, RDDModeUnset, disabled.Revision, global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(clear) error = %v", err)
	}
	if !enabled.Enabled() {
		t.Fatalf("cleared override did not re-enable: %#v", enabled)
	}
	if err := AuthorizeRDDCandidate(enabled); err != nil {
		t.Fatalf("re-enable stranded the current candidate: %v", err)
	}

	state := newCompactTestState(t, repo, "rdd-recovery")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID,
	})
	if err != nil || status.Applicability != TargetApplicabilityCurrent || status.State != StateReviewing ||
		status.Revision != revision || status.Projection.CurrentCandidateTree != state.CurrentSnapshot.CandidateTree {
		t.Fatalf("re-enabled current open candidate status = %#v, %v", status, err)
	}
}

func TestCloneLocalRDDOverrideRejectsStaleExpectedRevision(t *testing.T) {
	repo := initSnapshotRepo(t)
	global := RDDGlobalMode{Value: "on"}
	first, err := SetCloneLocalRDDMode(context.Background(), repo, RDDModeOff, "", global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}
	if _, err := SetCloneLocalRDDMode(context.Background(), repo, RDDModeUnset, "", global); !errors.Is(err, ErrRDDModeRevisionMismatch) {
		t.Fatalf("stale expected revision error = %v, want ErrRDDModeRevisionMismatch", err)
	}
	current, err := ResolveRDDMode(context.Background(), repo, global)
	if err != nil {
		t.Fatalf("ResolveRDDMode error = %v", err)
	}
	if current.Revision != first.Revision || current.Effective != RDDModeOff {
		t.Fatalf("losing writer corrupted the record: %#v", current)
	}
}

func TestCloneLocalRDDModeEnableRejectsGlobalOffWithoutChangingExplicitOff(t *testing.T) {
	repo := initSnapshotRepo(t)
	ctx := context.Background()
	disabled, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", RDDGlobalMode{Value: "on"})
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}
	record, err := CloneLocalRDDModeRecordPath(ctx, repo)
	if err != nil {
		t.Fatalf("CloneLocalRDDModeRecordPath error = %v", err)
	}
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("read explicit-off record: %v", err)
	}

	status, err := SetCloneLocalRDDMode(ctx, repo, RDDModeUnset, disabled.Revision, RDDGlobalMode{Value: "off"})
	var rejected *RDDDisabledError
	if !errors.As(err, &rejected) || !errors.Is(err, ErrRDDDisabled) || rejected.Source != RDDModeSourceGlobal {
		t.Fatalf("clear explicit-off override error = %v, want global typed disabled error", err)
	}
	if !strings.Contains(err.Error(), "gentle-ai review mode enable --scope=global") {
		t.Fatalf("clear explicit-off override error does not name the global continuation: %v", err)
	}
	if status.CloneLocal != RDDModeOff || status.Revision != disabled.Revision || status.Effective != RDDModeOff {
		t.Fatalf("rejected clear changed the clone-local status: %#v", status)
	}
	recordAfter, err := CloneLocalRDDModeRecordPath(ctx, repo)
	if err != nil {
		t.Fatalf("CloneLocalRDDModeRecordPath after rejected clear error = %v", err)
	}
	after, err := os.ReadFile(recordAfter)
	if err != nil {
		t.Fatalf("read explicit-off record after rejected clear: %v", err)
	}
	if recordAfter != record || !bytes.Equal(after, before) {
		t.Fatalf("rejected clear published a new generation")
	}
}

func TestCloneLocalRDDModeEnableValidatesStaleRevisionBeforeGlobalOffRefusal(t *testing.T) {
	repo := initSnapshotRepo(t)
	ctx := context.Background()
	disabled, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", RDDGlobalMode{Value: "on"})
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}
	record, err := CloneLocalRDDModeRecordPath(ctx, repo)
	if err != nil {
		t.Fatalf("CloneLocalRDDModeRecordPath error = %v", err)
	}
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("read explicit-off record: %v", err)
	}
	dir, err := cloneLocalRDDModeRoot(ctx, repo, false)
	if err != nil {
		t.Fatalf("cloneLocalRDDModeRoot error = %v", err)
	}
	generation, err := cloneLocalRDDOverrideHeadGeneration(dir)
	if err != nil {
		t.Fatalf("cloneLocalRDDOverrideHeadGeneration error = %v", err)
	}
	assertUnchanged := func() {
		t.Helper()
		recordAfter, pathErr := CloneLocalRDDModeRecordPath(ctx, repo)
		if pathErr != nil {
			t.Fatalf("CloneLocalRDDModeRecordPath after refusal error = %v", pathErr)
		}
		after, readErr := os.ReadFile(recordAfter)
		if readErr != nil {
			t.Fatalf("read explicit-off record after refusal: %v", readErr)
		}
		generationAfter, generationErr := cloneLocalRDDOverrideHeadGeneration(dir)
		if generationErr != nil {
			t.Fatalf("cloneLocalRDDOverrideHeadGeneration after refusal error = %v", generationErr)
		}
		if recordAfter != record || !bytes.Equal(after, before) || generationAfter != generation {
			t.Fatalf("refusal published a new generation: record %q, generation %d", recordAfter, generationAfter)
		}
	}

	_, err = SetCloneLocalRDDMode(ctx, repo, RDDModeUnset, "stale-revision", RDDGlobalMode{Value: "off"})
	if !errors.Is(err, ErrRDDModeRevisionMismatch) {
		t.Fatalf("stale clear error = %v, want ErrRDDModeRevisionMismatch", err)
	}
	if errors.Is(err, ErrRDDDisabled) {
		t.Fatalf("stale clear error = %v, must not report ErrRDDDisabled", err)
	}
	assertUnchanged()

	_, err = SetCloneLocalRDDMode(ctx, repo, RDDModeUnset, disabled.Revision, RDDGlobalMode{Value: "off"})
	var rejected *RDDDisabledError
	if !errors.As(err, &rejected) || !errors.Is(err, ErrRDDDisabled) || rejected.Source != RDDModeSourceGlobal {
		t.Fatalf("current clear error = %v, want global typed disabled error", err)
	}
	assertUnchanged()
}

func TestCloneLocalRDDModeTransitionsPublishExactlyOneGeneration(t *testing.T) {
	repo := initSnapshotRepo(t)
	ctx := context.Background()
	global := RDDGlobalMode{Value: "on"}

	disabled, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}
	dir, err := cloneLocalRDDModeRoot(ctx, repo, false)
	if err != nil {
		t.Fatalf("cloneLocalRDDModeRoot error = %v", err)
	}
	if generation, err := cloneLocalRDDOverrideHeadGeneration(dir); err != nil || generation != 1 {
		t.Fatalf("off transition generation = %d, %v, want 1", generation, err)
	}

	inherited, err := SetCloneLocalRDDMode(ctx, repo, RDDModeUnset, disabled.Revision, global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(inherit) error = %v", err)
	}
	if inherited.Source != RDDModeSourceGlobal || inherited.CloneLocal != RDDModeUnset {
		t.Fatalf("off-to-inherit status = %#v", inherited)
	}
	if generation, err := cloneLocalRDDOverrideHeadGeneration(dir); err != nil || generation != 2 {
		t.Fatalf("off-to-inherit generation = %d, %v, want 2", generation, err)
	}

	if _, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, inherited.Revision, global); err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off after inherit) error = %v", err)
	}
	if generation, err := cloneLocalRDDOverrideHeadGeneration(dir); err != nil || generation != 3 {
		t.Fatalf("inherit-to-off generation = %d, %v, want 3", generation, err)
	}
}

func TestCloneLocalRDDOverrideConcurrentWritersKeepOneWinner(t *testing.T) {
	repo := initSnapshotRepo(t)
	global := RDDGlobalMode{Value: "on"}
	var (
		group   sync.WaitGroup
		mutex   sync.Mutex
		winners int
		failed  []error
	)
	for range 4 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := SetCloneLocalRDDMode(context.Background(), repo, RDDModeOff, "", global)
			mutex.Lock()
			defer mutex.Unlock()
			if err == nil {
				winners++
				return
			}
			failed = append(failed, err)
		}()
	}
	group.Wait()
	if winners != 1 {
		t.Fatalf("concurrent writers = %d winners, want exactly 1 (errors: %v)", winners, failed)
	}
	for _, err := range failed {
		if !errors.Is(err, ErrRDDModeRevisionMismatch) && !errors.Is(err, ErrRARAuthorityConflict) {
			t.Fatalf("losing writer error = %v, want a CAS rejection", err)
		}
	}
	status, err := ResolveRDDMode(context.Background(), repo, global)
	if err != nil {
		t.Fatalf("ResolveRDDMode error = %v", err)
	}
	if status.Effective != RDDModeOff || status.Revision == "" {
		t.Fatalf("concurrent writers corrupted the record: %#v", status)
	}
}

func TestUnknownRDDModeFailsClosedAsDisabled(t *testing.T) {
	repo := initSnapshotRepo(t)
	status, err := ResolveRDDMode(context.Background(), repo, RDDGlobalMode{Value: "maybe"})
	if !errors.Is(err, ErrRDDModeUnknown) {
		t.Fatalf("unknown global mode error = %v, want ErrRDDModeUnknown", err)
	}
	if status.Effective != RDDModeOff || status.Enabled() {
		t.Fatalf("unknown global mode did not fail closed: %#v", status)
	}

	if _, err := SetCloneLocalRDDMode(context.Background(), repo, RDDModeOff, "", RDDGlobalMode{Value: "on"}); err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}
	corrupt := filepath.Join(repo, ".git", "gentle-ai", "review-mode", "rar-authority", "v1", "rdd-mode", "gen-0000000001.json")
	if err := os.WriteFile(corrupt, []byte("{not json}\n"), 0o600); err != nil {
		t.Fatalf("corrupt override: %v", err)
	}
	status, err = ResolveRDDMode(context.Background(), repo, RDDGlobalMode{Value: "on"})
	if !errors.Is(err, ErrRDDModeCorrupt) {
		t.Fatalf("corrupt override error = %v, want ErrRDDModeCorrupt", err)
	}
	if status.Effective != RDDModeOff || status.Enabled() {
		t.Fatalf("corrupt override did not fail closed: %#v", status)
	}
}

func TestResolveRDDModeUnsafePrivatePathIsNotACorruptHead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions are covered by the Windows ACL test")
	}
	repo := initSnapshotRepo(t)
	if _, err := SetCloneLocalRDDMode(context.Background(), repo, RDDModeOff, "", RDDGlobalMode{}); err != nil {
		t.Fatalf("disable clone-local mode: %v", err)
	}
	modeRecord := filepath.Join(repo, ".git", "gentle-ai", "review-mode", "rar-authority", "v1", "rdd-mode", "gen-0000000001.json")
	if err := os.Chmod(modeRecord, 0o644); err != nil {
		t.Fatalf("make private RAR file unsafe: %v", err)
	}
	defer os.Chmod(modeRecord, 0o600)

	status, err := ResolveRDDMode(context.Background(), repo, RDDGlobalMode{})
	if err == nil {
		t.Fatal("unsafe private RAR path resolved without an error")
	}
	if errors.Is(err, ErrRDDModeCorrupt) {
		t.Fatalf("unsafe private RAR path entered corrupt-head recovery: %v", err)
	}
	var unsafePath *UnsafeRARPathError
	if !errors.As(err, &unsafePath) || unsafePath.Path != modeRecord || unsafePath.Directory {
		t.Fatalf("unsafe private RAR path lost its typed cause: %#v", err)
	}
	if status.Enabled() {
		t.Fatalf("unsafe private RAR path did not fail closed: %#v", status)
	}
}
