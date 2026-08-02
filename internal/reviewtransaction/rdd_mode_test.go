package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	status, err := ResolveRDDMode(ctx, repo, RDDGlobalMode{})
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
		{name: "unconfigured stays enabled", global: "", cloneLocal: RDDModeUnset, effective: RDDModeOn, source: RDDModeSourceDefault},
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

	overridePath := filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "rar-authority", "v1", "rdd-mode")
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
	if status.Effective != RDDModeOn || status.Source != RDDModeSourceDefault {
		t.Fatalf("unconfigured status = %#v", status)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".git", "gentle-ai")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only resolution created repository state: %v", err)
	}
}

func TestDisabledRDDRejectsStartsAndFreezesActiveAuthority(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "candidate")
	_, _, receipt := approvedCompactRevisionFixture(t, repo, "rdd-mode-frozen")

	global := RDDGlobalMode{Value: "off", RecordedAt: time.Now().UTC()}
	if _, err := AuthorizeRDDOperation(context.Background(), repo, global, RDDOperationStart); !errors.Is(err, ErrRDDDisabled) {
		t.Fatalf("disabled start error = %v, want ErrRDDDisabled", err)
	}
	var disabled *RDDDisabledError
	_, err := AuthorizeRDDOperation(context.Background(), repo, global, RDDOperationMutate)
	if !errors.As(err, &disabled) || disabled.Operation != RDDOperationMutate {
		t.Fatalf("disabled mutation error = %v, want typed RDDDisabledError", err)
	}
	if _, err := AuthorizeRDDOperation(context.Background(), repo, global, RDDOperationRead); err != nil {
		t.Fatalf("disabled mode broke read-only authority: %v", err)
	}
	if err := receipt.Validate(); err != nil {
		t.Fatalf("disabled mode broke receipt validation: %v", err)
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
	// While disabled, starting a review is still a typed stop.
	var stop *RDDDisabledError
	err = AuthorizeRDDCandidate(disabled)
	if !errors.As(err, &stop) || !errors.Is(err, ErrRDDDisabled) || stop.Operation != RDDOperationStart {
		t.Fatalf("disabled candidate error = %v, want a typed RDDDisabledError start stop", err)
	}

	// The user keeps working with the kill switch off.
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

	// The recovery path must actually reach a receipt, and that receipt must be
	// bound to the bytes the review froze rather than to any earlier approval.
	state, _, receipt := approvedCompactRevisionFixture(t, repo, "rdd-recovery")
	if err := receipt.Validate(); err != nil {
		t.Fatalf("recovery receipt is invalid: %v", err)
	}
	subject, err := VerificationSubjectFromSnapshot(state.CurrentSnapshot)
	if err != nil {
		t.Fatalf("VerificationSubjectFromSnapshot error = %v", err)
	}
	if receipt.FinalCandidateTree != subject.CandidateTree {
		t.Fatalf("recovery receipt tree = %q, want the reviewed candidate tree %q",
			receipt.FinalCandidateTree, subject.CandidateTree)
	}
}

// The invariant that survives is content binding, not authorship time. A
// receipt issued before the disabled window may never approve the bytes that
// exist after re-enabling, and nothing approves without a review having run.
// The binding itself is enforced by the native receipt authority path
// (rar_native_receipt.go plus the compact store), so this test asserts the
// delegation instead of restating the rule in the kill switch.
func TestReEnabledRDDNeverInheritsAPreDisableApproval(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed before the kill switch\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "reviewed before the kill switch")
	staleState, _, staleReceipt := approvedCompactRevisionFixture(t, repo, "rdd-stale-approval")
	stalePayload, err := canonicalRARReceiptPayload(staleReceipt)
	if err != nil {
		t.Fatalf("canonicalRARReceiptPayload error = %v", err)
	}
	staleRef := sha256Ref(stalePayload)

	global := RDDGlobalMode{Value: "on", RecordedAt: time.Now().UTC().Add(-time.Hour)}
	disabled, err := SetCloneLocalRDDMode(context.Background(), repo, RDDModeOff, "", global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "changed while review was disabled\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "changed while review was disabled")
	enabled, err := SetCloneLocalRDDMode(context.Background(), repo, RDDModeUnset, disabled.Revision, global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(clear) error = %v", err)
	}
	if err := AuthorizeRDDCandidate(enabled); err != nil {
		t.Fatalf("re-enable stranded the changed candidate: %v", err)
	}

	repository, err := OpenRARAuthorityRepository(context.Background(), repo)
	if err != nil {
		t.Fatalf("OpenRARAuthorityRepository error = %v", err)
	}

	// Nothing approves without an actual review: a started-but-unreviewed
	// lineage over the current bytes derives no receipt and locks no authority.
	started := newCompactRevisionState(t, repo, "rdd-unreviewed")
	startedStore, err := CompactAuthoritativeStore(context.Background(), repo, "rdd-unreviewed")
	if err != nil {
		t.Fatalf("CompactAuthoritativeStore error = %v", err)
	}
	if _, err := startedStore.Replace("", "review/start", started); err != nil {
		t.Fatalf("start fresh review error = %v", err)
	}
	if _, err := started.Receipt(); err == nil {
		t.Fatal("a started-but-unreviewed candidate derived a receipt")
	}
	if _, _, release, err := repository.lockNativeReceipt(context.Background(), "rdd-unreviewed", staleRef); err == nil {
		release()
		t.Fatal("an unreviewed candidate locked native receipt authority")
	}

	// A genuine fresh review of the current bytes reaches its own receipt, and
	// the pre-disable receipt is refused for it because it binds other bytes.
	freshState, _, freshReceipt := approvedCompactRevisionFixture(t, repo, "rdd-recovery")
	freshSubject, err := VerificationSubjectFromSnapshot(freshState.CurrentSnapshot)
	if err != nil {
		t.Fatalf("VerificationSubjectFromSnapshot error = %v", err)
	}
	staleSubject, err := VerificationSubjectFromSnapshot(staleState.CurrentSnapshot)
	if err != nil {
		t.Fatalf("VerificationSubjectFromSnapshot(stale) error = %v", err)
	}
	if staleSubject.CandidateTree == freshSubject.CandidateTree {
		t.Fatal("the disabled window did not change the candidate bytes")
	}
	if staleReceipt.FinalCandidateTree == freshSubject.CandidateTree {
		t.Fatal("the pre-disable receipt is bound to the post-re-enable bytes")
	}
	if _, _, release, err := repository.lockNativeReceipt(context.Background(), "rdd-recovery", staleRef); err == nil {
		release()
		t.Fatal("a pre-disable receipt approved the post-re-enable candidate")
	}
	freshPayload, err := canonicalRARReceiptPayload(freshReceipt)
	if err != nil {
		t.Fatalf("canonicalRARReceiptPayload(fresh) error = %v", err)
	}
	native, boundSubject, release, err := repository.lockNativeReceipt(
		context.Background(), "rdd-recovery", sha256Ref(freshPayload))
	if err != nil {
		t.Fatalf("fresh receipt did not govern the reviewed candidate: %v", err)
	}
	defer release()
	if boundSubject.CandidateTree != freshSubject.CandidateTree || native.Compact == nil {
		t.Fatalf("fresh native authority = %#v bound to %#v", native, boundSubject)
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
	corrupt := filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "rar-authority", "v1", "rdd-mode", "gen-0000000001.json")
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

func TestRDDDeliveryDispositionNeverFabricatesApproval(t *testing.T) {
	disabled := RDDModeStatus{Effective: RDDModeOff}
	if got := RDDDeliveryDisposition(disabled, false); got != RDDDeliveryDisabledUnmanaged {
		t.Fatalf("disabled delivery = %q, want %q", got, RDDDeliveryDisabledUnmanaged)
	}
	// A receipt issued before the kill switch remains real authority: disabling
	// freezes it read-only, it does not retroactively unmake it.
	if got := RDDDeliveryDisposition(disabled, true); got != RDDDeliveryReceiptGoverned {
		t.Fatalf("disabled delivery with an existing receipt = %q, want %q", got, RDDDeliveryReceiptGoverned)
	}
	enabled := RDDModeStatus{Effective: RDDModeOn}
	if got := RDDDeliveryDisposition(enabled, false); got != RDDDeliveryUnmanaged {
		t.Fatalf("enabled delivery without a receipt = %q, want %q", got, RDDDeliveryUnmanaged)
	}
	if got := RDDDeliveryDisposition(enabled, true); got != RDDDeliveryReceiptGoverned {
		t.Fatalf("enabled delivery with a receipt = %q, want %q", got, RDDDeliveryReceiptGoverned)
	}
}
