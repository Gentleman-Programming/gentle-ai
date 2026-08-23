package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// preRelocationCloneMode reports the clone-local mode exactly as a gentle-ai
// installed before the switch moved out of the review authority tree reads it.
// Those builds look in one place only, so this deliberately never consults the
// switch's own root: it is the other reader on the machine, not this one.
func preRelocationCloneMode(t *testing.T, ctx context.Context, repo string) RDDModeStatus {
	t.Helper()
	dir, err := cloneLocalRDDModeLegacyRoot(ctx, repo, false)
	if err != nil {
		// An absent legacy tree is how those builds spell "this clone holds no
		// override", which leaves the global source deciding.
		return rddModeStatus(RDDModeOn, rddModeOverrideRecord{}, false)
	}
	record, present, err := readCloneLocalRDDOverrideHead(dir)
	if err != nil {
		t.Fatalf("pre-relocation clone-local read: %v", err)
	}
	return rddModeStatus(RDDModeOn, record, present)
}

// TestCloneScopeDisableAppliesToPreRelocationBuilds is the #3284 guard.
//
// The kill switch is machine state, not build state. A clone-scope disable that
// only this build can see reports success while every older gentle-ai on the
// same machine keeps enforcing review, which is the one thing a safety switch
// may never do.
func TestCloneScopeDisableAppliesToPreRelocationBuilds(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)

	if _, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", RDDGlobalMode{Value: "on"}); err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}

	if status := preRelocationCloneMode(t, ctx, repo); status.Enabled() {
		t.Fatalf("a pre-relocation gentle-ai still reports reviews on: %#v; the clone-scope disable reported success without applying", status)
	}
	assertCloneModeLocationsAgree(t, ctx, repo)
}

// TestCloneScopeEnableAlsoReachesPreRelocationBuilds keeps the switch
// symmetric. A disable that reaches every build and an enable that does not
// would strand the operator with reviews permanently off for half the machine.
func TestCloneScopeEnableAlsoReachesPreRelocationBuilds(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	global := RDDGlobalMode{Value: "on"}

	disabled, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}
	if _, err := SetCloneLocalRDDMode(ctx, repo, RDDModeUnset, disabled.Revision, global); err != nil {
		t.Fatalf("SetCloneLocalRDDMode(inherit) error = %v", err)
	}

	if status := preRelocationCloneMode(t, ctx, repo); !status.Enabled() {
		t.Fatalf("a pre-relocation gentle-ai still reports reviews off: %#v; the clone-scope enable never reached it", status)
	}
	assertCloneModeLocationsAgree(t, ctx, repo)
}

// TestCloneScopeWriteReportsMachineWideReach pins the claim the write is
// allowed to make. Reporting reach is the whole difference between a switch
// that applied and a switch that merely says so.
func TestCloneScopeWriteReportsMachineWideReach(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)

	status, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", RDDGlobalMode{Value: "on"})
	if err != nil || status.Reach != RDDModeReachMachine {
		t.Fatalf("clone-scope disable reach = %q, %v; want %q", status.Reach, err, RDDModeReachMachine)
	}
	// Resolution never probes the other location, so it must claim nothing
	// rather than repeat a reach it did not verify.
	resolved, err := ResolveRDDMode(ctx, repo, RDDGlobalMode{Value: "on"})
	if err != nil || resolved.Reach != RDDModeReachUnreported {
		t.Fatalf("read-only projection reach = %q, %v; want no claim", resolved.Reach, err)
	}
}

// TestCloneScopeRerunRepairsABuildLocalDisable is the upgrade path out of
// #3284. Operators who already disabled with an affected build hold a decision
// only that build can see; rerunning the documented command has to finish the
// job rather than report the same half-applied state as success.
func TestCloneScopeRerunRepairsABuildLocalDisable(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	global := RDDGlobalMode{Value: "on"}

	disabled, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}
	// Strip the mirrored copy to reproduce exactly what an affected build left
	// behind: this clone's own root decides off, the other location says nothing.
	legacy := legacyCloneModeRootForTest(t, ctx, repo)
	if err := os.Remove(filepath.Join(legacy, rddModeGenerationName(1))); err != nil {
		t.Fatalf("stage the build-local disable: %v", err)
	}
	if status := preRelocationCloneMode(t, ctx, repo); !status.Enabled() {
		t.Fatalf("staging failed: the pre-relocation location still decides %#v", status)
	}

	repaired, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, disabled.Revision, global)
	if err != nil {
		t.Fatalf("rerunning the disable failed: %v", err)
	}
	if repaired.Reach != RDDModeReachMachine || repaired.Revision == disabled.Revision {
		t.Fatalf("rerun status = %#v; want a fresh revision published with machine-wide reach", repaired)
	}
	if status := preRelocationCloneMode(t, ctx, repo); status.Enabled() {
		t.Fatalf("rerunning the disable left a pre-relocation gentle-ai enforcing review: %#v", status)
	}
	assertCloneModeLocationsAgree(t, ctx, repo)

	// Once both locations agree the same command is inert again: reconciling
	// must not turn every repeat into another generation.
	settled, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, repaired.Revision, global)
	if err != nil || settled.Revision != repaired.Revision {
		t.Fatalf("redundant disable = %#v, %v; want the same head", settled, err)
	}
}

// TestCloneScopeWriteSupersedesHigherPreRelocationGenerations covers the slot
// arithmetic across two locations. A gentle-ai from before the relocation
// publishes generations of its own, so this build's next slot has to clear
// whichever location has gone further: colliding would refuse the write, and
// publishing underneath would leave that build reading its own older record.
func TestCloneScopeWriteSupersedesHigherPreRelocationGenerations(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	global := RDDGlobalMode{Value: "on"}

	disabled, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}
	legacy := legacyCloneModeRootForTest(t, ctx, repo)
	foreign := publishCurrentModeRecordForTest(t, legacy, rddModeOverrideRecord{Generation: 4}, string(RDDModeOff))
	foreignPath := filepath.Join(legacy, rddModeGenerationName(foreign.Generation))
	foreignBytes, err := os.ReadFile(foreignPath)
	if err != nil {
		t.Fatalf("read the pre-relocation record: %v", err)
	}

	cleared, err := SetCloneLocalRDDMode(ctx, repo, RDDModeUnset, disabled.Revision, global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(inherit) error = %v", err)
	}
	if cleared.Revision == "" || cleared.Reach != RDDModeReachMachine {
		t.Fatalf("clear across two locations = %#v", cleared)
	}
	if after, err := os.ReadFile(foreignPath); err != nil || !bytes.Equal(after, foreignBytes) {
		t.Fatalf("the pre-relocation record was overwritten: err=%v", err)
	}
	if head, err := cloneLocalRDDOverrideHeadGeneration(legacy); err != nil || head != foreign.Generation+1 {
		t.Fatalf("pre-relocation head = %d, %v; want %d", head, err, foreign.Generation+1)
	}
	if status := preRelocationCloneMode(t, ctx, repo); !status.Enabled() {
		t.Fatalf("a pre-relocation gentle-ai kept its own older record: %#v", status)
	}
	assertCloneModeLocationsAgree(t, ctx, repo)
}

// TestCloneScopeWriteRefusesToClaimSuccessOnAPartialPublish is the partial
// failure this dual write introduces: the other location opened, so a
// pre-relocation gentle-ai can read it, and the publish there failed anyway.
// The decision applied here and did not apply there, and saying so is the whole
// point -- a switch reported as working while half the machine still enforces
// review is the defect, not the recovery.
func TestCloneScopeWriteRefusesToClaimSuccessOnAPartialPublish(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	global := RDDGlobalMode{Value: "on"}

	disabled, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}
	// Occupy the next slot at the other location with something that is not a
	// publishable record. The head scan ignores directories, so the write still
	// targets this slot and the immutable publish there refuses it.
	legacy := legacyCloneModeRootForTest(t, ctx, repo)
	blocked := filepath.Join(legacy, rddModeGenerationName(2))
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatalf("block the pre-relocation slot: %v", err)
	}

	partial, err := SetCloneLocalRDDMode(ctx, repo, RDDModeUnset, disabled.Revision, global)
	var reported *RDDModePartialApplyError
	if !errors.Is(err, ErrRDDModePartiallyApplied) || !errors.As(err, &reported) {
		t.Fatalf("partial publish error = %v, want ErrRDDModePartiallyApplied", err)
	}
	if reported.Mode != RDDModeUnset || !strings.Contains(err.Error(), "gentle-ai review mode enable --scope clone") {
		t.Fatalf("partial publish refusal names no rerun: %v", err)
	}
	// The decision is real for this build; reporting it as reached-nowhere
	// would be its own lie, and it must not claim the machine either.
	if partial.CloneLocal != RDDModeUnset || !partial.Enabled() || partial.Reach != RDDModeReachThisBuild {
		t.Fatalf("partial publish status = %#v; want this build's decision with this_build reach", partial)
	}
	if head, err := cloneLocalRDDOverrideHeadGeneration(cloneModeRootForTest(t, ctx, repo)); err != nil || head != 2 {
		t.Fatalf("this build's head = %d, %v; want the decision recorded here", head, err)
	}
	if status := preRelocationCloneMode(t, ctx, repo); status.Enabled() {
		t.Fatalf("the refusal was unnecessary: the pre-relocation location already agrees: %#v", status)
	}

	// Clearing the obstruction and rerunning converges both locations, so the
	// refusal names a continuation that actually finishes the job.
	if err := os.Remove(blocked); err != nil {
		t.Fatalf("unblock the pre-relocation slot: %v", err)
	}
	healed, err := SetCloneLocalRDDMode(ctx, repo, RDDModeUnset, partial.Revision, global)
	if err != nil || healed.Reach != RDDModeReachMachine {
		t.Fatalf("rerun after the partial publish = %#v, %v", healed, err)
	}
	if status := preRelocationCloneMode(t, ctx, repo); !status.Enabled() {
		t.Fatalf("rerun did not converge the two locations: %#v", status)
	}
	assertCloneModeLocationsAgree(t, ctx, repo)
}

// TestCloneScopeWriteStaysReachableWhenTheOtherLocationIsNot keeps #2882's exit
// open. `review mode disable --scope clone` is the continuation the stop-reason
// table names for unrecoverable review states, so a damaged review authority
// tree may not make it refuse. It may, and must, stop it claiming the machine.
func TestCloneScopeWriteStaysReachableWhenTheOtherLocationIsNot(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	global := RDDGlobalMode{Value: "on"}
	damageCloneModeLegacyTreeForTest(t, repo)

	disabled, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", global)
	if err != nil {
		t.Fatalf("clone-scope disable refused while the other location was damaged: %v", err)
	}
	if disabled.Effective != RDDModeOff || disabled.Source != RDDModeSourceCloneLocal {
		t.Fatalf("damaged-tree disable = %#v; want the clone-local source deciding off", disabled)
	}
	if disabled.Reach != RDDModeReachThisBuild {
		t.Fatalf("damaged-tree disable reach = %q; want %q, because a location nothing can open is a location nothing was published to",
			disabled.Reach, RDDModeReachThisBuild)
	}
	current := cloneModeRootForTest(t, ctx, repo)
	if head, err := cloneLocalRDDOverrideHeadGeneration(current); err != nil || head != 1 {
		t.Fatalf("damaged-tree disable head = %d, %v; want the decision persisted here", head, err)
	}

	// Repeating it must not churn generations: an unreachable location cannot
	// be reconciled, so there is nothing left for the second run to do.
	repeated, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, disabled.Revision, global)
	if err != nil || repeated.Revision != disabled.Revision || repeated.Reach != RDDModeReachThisBuild {
		t.Fatalf("repeated damaged-tree disable = %#v, %v", repeated, err)
	}
	if head, err := cloneLocalRDDOverrideHeadGeneration(current); err != nil || head != 1 {
		t.Fatalf("repeated damaged-tree disable published generation %d, %v", head, err)
	}
}

// TestCloneScopeWriteNeverClaimsTheMachineOverAnUnscannableMirror is the
// reproduction for R4-mirror-head-zero.
//
// The pre-relocation location can open, lock, and validate and still refuse to
// enumerate its published slots. Its head is then unknown, not zero, and the
// difference decides everything: publishing this build's next slot number into
// a location whose head is higher lands the decision UNDERNEATH the record a
// pre-relocation gentle-ai actually reads. That build keeps enforcing the
// mode the operator just changed while the write reports it reached the whole
// machine, which is #3284 wearing a success message.
func TestCloneScopeWriteNeverClaimsTheMachineOverAnUnscannableMirror(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	global := RDDGlobalMode{Value: "on"}

	disabled, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}
	// A pre-relocation gentle-ai then published generations of its own, so the
	// other location has gone further than this build's root. Its slot 2 is
	// free, which is exactly what makes the mis-publish silent instead of loud.
	legacy := legacyCloneModeRootForTest(t, ctx, repo)
	foreign := publishCurrentModeRecordForTest(t, legacy, rddModeOverrideRecord{Generation: 4}, string(RDDModeOff))
	if status := preRelocationCloneMode(t, ctx, repo); status.Enabled() {
		t.Fatalf("staging failed: the pre-relocation location must decide off, got %#v", status)
	}
	failCloneModeMirrorSlotScanForTest(t, legacy)

	status, err := SetCloneLocalRDDMode(ctx, repo, RDDModeUnset, disabled.Revision, global)
	if err != nil {
		t.Fatalf("clone-scope enable refused while the other location could not be enumerated: %v", err)
	}
	// The #2882 exit stays open: this build's own decision is recorded.
	if !status.Enabled() || status.CloneLocal != RDDModeUnset {
		t.Fatalf("clone-scope enable status = %#v; want this build's override cleared", status)
	}
	if status.Reach == RDDModeReachMachine {
		if pre := preRelocationCloneMode(t, ctx, repo); !pre.Enabled() {
			t.Fatalf("the write claimed machine-wide reach while a pre-relocation gentle-ai still reads reviews off: %#v", pre)
		}
	}
	if status.Reach != RDDModeReachThisBuild {
		t.Fatalf("reach over a mirror whose slots cannot be read = %q, want %q", status.Reach, RDDModeReachThisBuild)
	}
	// Nothing may be published into a location whose layout this build could
	// not read: a record under its head is invisible to the only reader it was
	// written for.
	stray := filepath.Join(legacy, rddModeGenerationName(2))
	if _, err := os.Lstat(stray); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a record was published at slot 2, underneath the pre-relocation head %d: %v", foreign.Generation, err)
	}
	if head, err := cloneLocalRDDOverrideHeadGeneration(legacy); err != nil || head != foreign.Generation {
		t.Fatalf("pre-relocation head = %d, %v; want the untouched %d", head, err, foreign.Generation)
	}
}

// failCloneModeMirrorSlotScanForTest reproduces a mirror directory that opens,
// validates, and then refuses to enumerate. The scan is a variable for the
// same reason the private-directory primitives are: no test on ext4 or tmpfs
// can produce a directory that passes the no-follow owner-only walk and then
// fails readdir(2).
func failCloneModeMirrorSlotScanForTest(t *testing.T, dir string) {
	t.Helper()
	real := cloneLocalRDDModeMirrorSlotScan
	t.Cleanup(func() { cloneLocalRDDModeMirrorSlotScan = real })
	cloneLocalRDDModeMirrorSlotScan = func(scanned string) (int, error) {
		if scanned == dir {
			return 0, fmt.Errorf("enumerate %q: %w", scanned, os.ErrPermission)
		}
		return real(scanned)
	}
}

// TestCloneScopeConcurrentWritesLeaveBothLocationsAgreeing pins the invariant
// that makes the dual write safe to reason about: one winner, and the two
// locations hold the same decision afterwards rather than one each.
func TestCloneScopeConcurrentWritesLeaveBothLocationsAgreeing(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	global := RDDGlobalMode{Value: "on"}

	var (
		group   sync.WaitGroup
		mutex   sync.Mutex
		winners int
	)
	for range 4 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", global)
			mutex.Lock()
			defer mutex.Unlock()
			if err == nil {
				winners++
				return
			}
			if !errors.Is(err, ErrRDDModeRevisionMismatch) && !errors.Is(err, ErrRARAuthorityConflict) {
				t.Errorf("losing writer error = %v, want a CAS rejection", err)
			}
		}()
	}
	group.Wait()
	if winners != 1 {
		t.Fatalf("concurrent clone-scope writers = %d winners, want 1", winners)
	}
	if status := preRelocationCloneMode(t, ctx, repo); status.Enabled() {
		t.Fatalf("concurrent writers left a pre-relocation gentle-ai enforcing review: %#v", status)
	}
	assertCloneModeLocationsAgree(t, ctx, repo)
}

// assertCloneModeLocationsAgree checks the property the two locations exist to
// hold jointly: the same head record, byte for byte. A pre-relocation gentle-ai
// re-derives the digest and rejects anything that is not canonical, so equal
// bytes is the only form of agreement that survives its parser.
func assertCloneModeLocationsAgree(t *testing.T, ctx context.Context, repo string) {
	t.Helper()
	current := cloneModeRootForTest(t, ctx, repo)
	legacy := legacyCloneModeRootForTest(t, ctx, repo)
	head, err := cloneLocalRDDOverrideHeadGeneration(current)
	if err != nil || head == 0 {
		t.Fatalf("this build's head = %d, %v", head, err)
	}
	legacyHead, err := cloneLocalRDDOverrideHeadGeneration(legacy)
	if err != nil || legacyHead != head {
		t.Fatalf("pre-relocation head = %d, %v; want %d", legacyHead, err, head)
	}
	name := rddModeGenerationName(head)
	mine, err := os.ReadFile(filepath.Join(current, name))
	if err != nil {
		t.Fatalf("read this build's head: %v", err)
	}
	theirs, err := os.ReadFile(filepath.Join(legacy, name))
	if err != nil {
		t.Fatalf("read the pre-relocation head: %v", err)
	}
	if !bytes.Equal(mine, theirs) {
		t.Fatalf("the two locations hold different decisions:\n  this build: %s  pre-relocation: %s", mine, theirs)
	}
}

func cloneModeRootForTest(t *testing.T, ctx context.Context, repo string) string {
	t.Helper()
	dir, err := cloneLocalRDDModeRoot(ctx, repo, false)
	if err != nil {
		t.Fatalf("this build's clone-local root: %v", err)
	}
	return dir
}

func legacyCloneModeRootForTest(t *testing.T, ctx context.Context, repo string) string {
	t.Helper()
	dir, err := cloneLocalRDDModeLegacyRoot(ctx, repo, false)
	if err != nil {
		t.Fatalf("pre-relocation clone-local root: %v", err)
	}
	return dir
}

// damageCloneModeLegacyTreeForTest makes the pre-relocation tree fail RAR path
// safety the way #2882's reporter hit it, without needing symlink support: the
// shared ancestor is a regular file, so every walk over it refuses.
func damageCloneModeLegacyTreeForTest(t *testing.T, repo string) {
	t.Helper()
	gentle := filepath.Join(repo, ".git", "gentle-ai")
	if err := os.MkdirAll(gentle, 0o700); err != nil {
		t.Fatal(err)
	}
	authority := filepath.Join(gentle, rddModeLegacySwitchDirectory)
	if err := os.RemoveAll(authority); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authority, []byte("not a directory\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
