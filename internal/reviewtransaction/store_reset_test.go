package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeStoreResetFile creates one fixture file and every parent directory it
// needs. Fixtures are written straight to disk on purpose: this command has to
// work against a store the normal writers can no longer open, so building the
// fixture through those writers would only ever prove the healthy case.
func writeStoreResetFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

// storeResetGentleAIRoot returns <git-common-dir>/gentle-ai for a fixture repo.
func storeResetGentleAIRoot(t *testing.T, repo string) string {
	t.Helper()
	authority, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(authority)
}

// seedStoreResetLineage writes one compact-v2 lineage holding the given state.
func seedStoreResetLineage(t *testing.T, repo, lineage string, state State) {
	t.Helper()
	seedStoreResetLineageAt(t, storeResetGentleAIRoot(t, repo), lineage, state)
}

// seedStoreResetLineageAt writes the same lineage against an already-resolved
// store root. Resolving the root shells out to Git, so a caller standing inside
// a timed window uses this one and pays that cost before the window opens.
func seedStoreResetLineageAt(t *testing.T, root, lineage string, state State) {
	t.Helper()
	writeStoreResetFile(t, filepath.Join(root, "review-transactions", "v2", lineage, "review-state.json"),
		`{"schema":"gentle-ai.review-state-record/v2","revision":"sha256:00","state":{"schema":"gentle-ai.review-state/v2","lineage_id":"`+lineage+`","state":"`+string(state)+`"}}`+"\n")
}

// seedStoreResetKillSwitch writes the clone-scoped kill switch into both the
// current location and the pre-#2882 mirror that still-installed builds read.
func seedStoreResetKillSwitch(t *testing.T, repo string) (current, legacy string) {
	t.Helper()
	root := storeResetGentleAIRoot(t, repo)
	current = filepath.Join(root, "review-mode", "rar-authority", "v1", "rdd-mode", "gen-0000000001.json")
	legacy = filepath.Join(root, "review-transactions", "rar-authority", "v1", "rdd-mode", "gen-0000000001.json")
	writeStoreResetFile(t, current, `{"schema":"gentle-ai.rdd-mode-override/v1","mode":"off"}`+"\n")
	writeStoreResetFile(t, legacy, `{"schema":"gentle-ai.rdd-mode-override/v1","mode":"off"}`+"\n")
	return current, legacy
}

// stubStoreResetRename swaps the rename seam for one test and restores it
// afterwards, mirroring reclaimQuarantineResidue in compact_reclaim.go.
func stubStoreResetRename(t *testing.T, rename func(oldpath, newpath string) error) {
	t.Helper()
	previous := storeResetRename
	storeResetRename = rename
	t.Cleanup(func() { storeResetRename = previous })
}

// stubStoreResetAcquireLease swaps the lease seam for one test and restores it
// afterwards, mirroring stubStoreResetRename.
func stubStoreResetAcquireLease(t *testing.T, acquire func(ctx context.Context, repo string) (*MaintenanceLock, error)) {
	t.Helper()
	previous := storeResetAcquireLease
	storeResetAcquireLease = acquire
	t.Cleanup(func() { storeResetAcquireLease = previous })
}

// seedStoreResetCandidateView writes one candidate view and the Git worktree
// administrative directory that registers it, linked the way Git links them.
func seedStoreResetCandidateView(t *testing.T, repo, name string) (view, admin string) {
	t.Helper()
	root := storeResetGentleAIRoot(t, repo)
	view = filepath.Join(root, "candidate-views", name)
	admin = filepath.Join(filepath.Dir(root), "worktrees", name)
	writeStoreResetFile(t, filepath.Join(view, "README.md"), "checkout\n")
	writeStoreResetFile(t, filepath.Join(view, ".git"), "gitdir: "+admin+"\n")
	writeStoreResetFile(t, filepath.Join(admin, "gitdir"), filepath.Join(view, ".git")+"\n")
	return view, admin
}

func storeResetEntryByName(t *testing.T, entries []StoreResetEntry, name string) StoreResetEntry {
	t.Helper()
	for _, entry := range entries {
		if entry.Name == name {
			return entry
		}
	}
	t.Fatalf("no entry named %q in %#v", name, entries)
	return StoreResetEntry{}
}

// TestSurveyReviewStoreReportsEveryCategoryWithoutMutating is the preview
// contract: it must name what it would remove, count it, and leave every byte
// exactly where it found it.
func TestSurveyReviewStoreReportsEveryCategoryWithoutMutating(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	seedStoreResetLineage(t, repo, "review-settled", StateApproved)
	writeStoreResetFile(t, filepath.Join(root, "candidate-views", "1a09d6f9", "README.md"), "checkout\n")
	writeStoreResetFile(t, filepath.Join(root, "reviews", "IDENTITY"), "{}\n")
	seedStoreResetKillSwitch(t, repo)

	report, err := SurveyReviewStore(context.Background(), repo, StoreResetRequest{})
	if err != nil {
		t.Fatalf("survey: %v", err)
	}
	if report.Operation != StoreResetPreview {
		t.Fatalf("operation = %q, want %q", report.Operation, StoreResetPreview)
	}
	if report.Schema != StoreResetReportSchema {
		t.Fatalf("schema = %q", report.Schema)
	}
	for _, name := range []string{"candidate-views", "review-transactions/v2"} {
		entry := storeResetEntryByName(t, report.Removable, name)
		if !entry.Present || entry.Files == 0 || entry.Bytes == 0 {
			t.Fatalf("removable %q = %#v, want a present non-empty category", name, entry)
		}
		if entry.Removed {
			t.Fatalf("survey reported %q as removed", name)
		}
	}
	if report.RemovedFiles != 0 || report.RemovedBytes != 0 {
		t.Fatalf("survey reported removals: %d files / %d bytes", report.RemovedFiles, report.RemovedBytes)
	}

	// Nothing moved.
	for _, path := range []string{
		filepath.Join(root, "candidate-views", "1a09d6f9", "README.md"),
		filepath.Join(root, "reviews", "IDENTITY"),
		filepath.Join(root, "review-transactions", "v2", "review-settled", "review-state.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("survey mutated the store: %v", err)
		}
	}
}

// TestSurveyReviewStoreCreatesNoStateOnACleanClone proves the preview is safe
// to run anywhere: a repository that never reviewed anything must not gain a
// gentle-ai directory just because someone asked what a reset would do.
func TestSurveyReviewStoreCreatesNoStateOnACleanClone(t *testing.T) {
	repo := initSnapshotRepo(t)

	report, err := SurveyReviewStore(context.Background(), repo, StoreResetRequest{})
	if err != nil {
		t.Fatalf("survey: %v", err)
	}
	if report.RemovedFiles != 0 || len(report.InFlight) != 0 {
		t.Fatalf("clean clone report = %#v", report)
	}
	for _, entry := range report.Removable {
		if entry.Present {
			t.Fatalf("clean clone reported %q present", entry.Name)
		}
	}
	if _, err := os.Lstat(storeResetGentleAIRoot(t, repo)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("survey created review state: %v", err)
	}
}

// TestResetReviewStoreRefusesWhileLineagesAreInFlight is the central safety
// rule. A lineage that is not approved, escalated, or invalidated is a review
// somebody is in the middle of, and the default answer is no.
func TestResetReviewStoreRefusesWhileLineagesAreInFlight(t *testing.T) {
	repo := initSnapshotRepo(t)
	seedStoreResetLineage(t, repo, "review-settled", StateApproved)
	seedStoreResetLineage(t, repo, "review-active", StateReviewing)
	seedStoreResetLineage(t, repo, "review-correcting", StateCorrectionRequired)
	root := storeResetGentleAIRoot(t, repo)

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{})
	var inFlight *StoreResetInFlightError
	if !errors.As(err, &inFlight) {
		t.Fatalf("reset with in-flight lineages = %v, want a typed refusal", err)
	}
	if len(inFlight.Lineages) != 2 {
		t.Fatalf("refusal listed %#v, want both in-flight lineages", inFlight.Lineages)
	}
	// The refusal has to name them; a count alone does not tell the user which
	// review they are about to lose.
	for _, lineage := range []string{"review-active", "review-correcting"} {
		if !strings.Contains(inFlight.Error(), lineage) {
			t.Fatalf("refusal does not name %q: %v", lineage, inFlight)
		}
	}
	if strings.Contains(inFlight.Error(), "review-settled") {
		t.Fatalf("refusal named a settled lineage: %v", inFlight)
	}
	if !strings.Contains(inFlight.Error(), "--include-in-flight") {
		t.Fatalf("refusal does not name the override flag: %v", inFlight)
	}
	if report.RemovedFiles != 0 || report.RemovedBytes != 0 {
		t.Fatalf("refused reset still removed %d files", report.RemovedFiles)
	}
	// A refusal removes nothing at all -- not even the settled lineages.
	for _, lineage := range []string{"review-settled", "review-active", "review-correcting"} {
		if _, err := os.Stat(filepath.Join(root, "review-transactions", "v2", lineage, "review-state.json")); err != nil {
			t.Fatalf("refused reset removed %q: %v", lineage, err)
		}
	}
}

// TestResetReviewStoreUnreadableLineageCountsAsInFlight fails closed. A record
// this command cannot parse might be an open review, and a degraded store is
// exactly when that guess gets made, so it guesses in the direction that keeps
// the work.
func TestResetReviewStoreUnreadableLineageCountsAsInFlight(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	writeStoreResetFile(t, filepath.Join(root, "review-transactions", "v2", "review-damaged", "review-state.json"), "{not json\n")

	_, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{})
	var inFlight *StoreResetInFlightError
	if !errors.As(err, &inFlight) {
		t.Fatalf("reset over an unreadable record = %v, want a typed refusal", err)
	}
	if len(inFlight.Lineages) != 1 || inFlight.Lineages[0].Unreadable == "" {
		t.Fatalf("unreadable lineage = %#v, want it reported as unreadable", inFlight.Lineages)
	}
}

// TestResetReviewStoreRemovesSettledLineagesWithoutAnOverride is the ordinary
// path: every lineage has reached a terminal state, so no explicit flag is
// required and the store empties.
func TestResetReviewStoreRemovesSettledLineagesWithoutAnOverride(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	seedStoreResetLineage(t, repo, "review-approved", StateApproved)
	seedStoreResetLineage(t, repo, "review-escalated", StateEscalated)
	seedStoreResetLineage(t, repo, "review-invalidated", StateInvalidated)
	writeStoreResetFile(t, filepath.Join(root, "review-transactions", "quarantine", "review-old-1", "reclaim-record.json"), "{}\n")
	writeStoreResetFile(t, filepath.Join(root, "review-transactions", "effect-markers", "v1", "marker.json"), "{}\n")
	writeStoreResetFile(t, filepath.Join(root, "candidate-views", "abc", "file.txt"), "checkout\n")

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{})
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if !report.Complete {
		t.Fatalf("reset reported incomplete: %#v", report)
	}
	if report.Operation != StoreResetApplied {
		t.Fatalf("operation = %q", report.Operation)
	}
	if report.RemovedFiles == 0 || report.RemovedBytes == 0 {
		t.Fatalf("reset removed nothing: %#v", report)
	}
	for _, name := range []string{"review-transactions/v2", "review-transactions/quarantine", "review-transactions/effect-markers", "candidate-views"} {
		if entry := storeResetEntryByName(t, report.Removable, name); !entry.Removed {
			t.Fatalf("category %q was not removed: %#v", name, entry)
		}
	}
	for _, path := range []string{
		filepath.Join(root, "review-transactions", "v2"),
		filepath.Join(root, "review-transactions", "quarantine"),
		filepath.Join(root, "candidate-views"),
	} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s survived the reset: %v", path, err)
		}
	}
}

// TestResetReviewStoreOverrideRemovesInFlightLineages proves the escape hatch
// works once the user asks for it explicitly.
func TestResetReviewStoreOverrideRemovesInFlightLineages(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	seedStoreResetLineage(t, repo, "review-active", StateReviewing)

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{IncludeInFlight: true})
	if err != nil {
		t.Fatalf("override reset: %v", err)
	}
	if len(report.InFlight) != 1 {
		t.Fatalf("override report still has to name what it destroyed: %#v", report.InFlight)
	}
	if _, err := os.Lstat(filepath.Join(root, "review-transactions", "v2")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("override reset kept the in-flight lineage: %v", err)
	}
}

// TestResetReviewStorePreservesTheKillSwitchInBothLocations is the regression
// this whole allowlist exists for. The pre-#2882 mirror lives *inside*
// review-transactions/, so a reset that removed that directory wholesale would
// silently switch reviews back on for a user who turned them off.
func TestResetReviewStorePreservesTheKillSwitchInBothLocations(t *testing.T) {
	repo := initSnapshotRepo(t)
	current, legacy := seedStoreResetKillSwitch(t, repo)
	seedStoreResetLineage(t, repo, "review-approved", StateApproved)

	if _, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{}); err != nil {
		t.Fatalf("reset: %v", err)
	}
	for _, path := range []string{current, legacy} {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reset destroyed the kill switch at %s: %v", path, err)
		}
		if !strings.Contains(string(payload), `"mode":"off"`) {
			t.Fatalf("kill switch at %s changed: %s", path, payload)
		}
	}
}

// TestResetReviewStorePreservesEverythingItDoesNotOwn covers the rest of the
// exclusion list plus the allowlist rule: an unrecognized directory is reported
// and left alone, never guessed at.
func TestResetReviewStorePreservesEverythingItDoesNotOwn(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	seedStoreResetLineage(t, repo, "review-approved", StateApproved)
	preserved := map[string]string{
		"sdd-runtime":      filepath.Join(root, "sdd-runtime", "v1", "some-change", "HEAD"),
		"defect-reports":   filepath.Join(root, "defect-reports", "operation-outcome-unknown-00ff.md"),
		"review-artifacts": filepath.Join(root, "review-artifacts", "issue-1107", "notes.md"),
		"incidents":        filepath.Join(root, "incidents", "1462", "notes.md"),
	}
	for _, path := range preserved {
		writeStoreResetFile(t, path, "keep me\n")
	}
	future := filepath.Join(root, "some-future-feature", "state.json")
	writeStoreResetFile(t, future, "{}\n")
	nestedFuture := filepath.Join(root, "review-transactions", "v3", "state.json")
	writeStoreResetFile(t, nestedFuture, "{}\n")

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{})
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	for name, path := range preserved {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("reset removed preserved category %q: %v", name, err)
		}
		if entry := storeResetEntryByName(t, report.Preserved, name); entry.Reason == "" {
			t.Fatalf("preserved category %q carries no reason", name)
		}
	}
	for _, path := range []string{future, nestedFuture} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("reset removed an unrecognized path %s: %v", path, err)
		}
	}
	for _, name := range []string{"some-future-feature", "review-transactions/v3"} {
		if entry := storeResetEntryByName(t, report.Unrecognized, name); !entry.Present {
			t.Fatalf("unrecognized path %q was not reported", name)
		}
	}
}

// TestResetReviewStoreRemovesCandidateViewWorktreeAdminDirs covers the one
// category that is not just files. Candidate views are registered Git
// worktrees, so removing the checkout without its admin directory would leave
// the repository's worktree list pointing at nothing.
func TestResetReviewStoreRemovesCandidateViewWorktreeAdminDirs(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	commonDir := filepath.Dir(root)
	view := filepath.Join(root, "candidate-views", "1a09d6f9")
	admin := filepath.Join(commonDir, "worktrees", "1a09d6f9")
	writeStoreResetFile(t, filepath.Join(view, "README.md"), "checkout\n")
	writeStoreResetFile(t, filepath.Join(view, ".git"), "gitdir: "+admin+"\n")
	writeStoreResetFile(t, filepath.Join(admin, "gitdir"), filepath.Join(view, ".git")+"\n")

	// A worktree that is not a candidate view must survive untouched, even
	// though it sits in the same admin directory.
	foreign := filepath.Join(commonDir, "worktrees", "alans-branch")
	writeStoreResetFile(t, filepath.Join(foreign, "gitdir"), filepath.Join(repo, "..", "alans-branch", ".git")+"\n")

	if _, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{}); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, err := os.Lstat(view); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("candidate view survived: %v", err)
	}
	if _, err := os.Lstat(admin); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("candidate view admin directory survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(foreign, "gitdir")); err != nil {
		t.Fatalf("reset pruned an unrelated worktree: %v", err)
	}
}

// TestResetReviewStoreRemovesReadOnlyCandidateViews covers the shape the
// adapter actually writes: the checkout is chmod'd read-only, which makes a
// plain RemoveAll fail.
func TestResetReviewStoreRemovesReadOnlyCandidateViews(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores the permission bits this test depends on")
	}
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	view := filepath.Join(root, "candidate-views", "read-only-view")
	writeStoreResetFile(t, filepath.Join(view, "nested", "file.txt"), "frozen\n")
	for _, path := range []string{filepath.Join(view, "nested"), view} {
		if err := os.Chmod(path, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o755) })
	}

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{})
	if err != nil {
		t.Fatalf("reset over a read-only candidate view: %v", err)
	}
	if !report.Complete {
		t.Fatalf("reset reported incomplete: %#v", report.Removable)
	}
	if _, err := os.Lstat(view); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only candidate view survived: %v", err)
	}
}

// TestResetReviewStoreReportsPartialFailureHonestly is the reporting contract.
// When a category cannot be moved aside, the command must say so, must not
// count it as removed, and must not claim it succeeded.
func TestResetReviewStoreReportsPartialFailureHonestly(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	seedStoreResetLineage(t, repo, "review-approved", StateApproved)
	writeStoreResetFile(t, filepath.Join(root, "reviews", "IDENTITY"), "{}\n")

	stubStoreResetRename(t, func(oldpath, newpath string) error {
		if filepath.Base(oldpath) == "reviews" {
			return errors.New("simulated rename failure")
		}
		return os.Rename(oldpath, newpath)
	})

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{IncludeAdapterReviews: true})
	if err == nil {
		t.Fatal("a partial reset reported success")
	}
	if report.Complete {
		t.Fatalf("a partial reset reported Complete: %#v", report)
	}
	failed := storeResetEntryByName(t, report.Removable, "reviews")
	if failed.Removed {
		t.Fatalf("the failed category counted as removed: %#v", failed)
	}
	if failed.Skipped == "" || !strings.Contains(failed.Skipped, "simulated rename failure") {
		t.Fatalf("the failed category does not carry its reason: %#v", failed)
	}
	// The category that did work is still reported as removed, and the byte
	// count covers only what actually went away.
	removed := storeResetEntryByName(t, report.Removable, "review-transactions/v2")
	if !removed.Removed || report.RemovedBytes != removed.Bytes {
		t.Fatalf("partial reset byte accounting = %d, want %d", report.RemovedBytes, removed.Bytes)
	}
	if _, err := os.Stat(filepath.Join(root, "reviews", "IDENTITY")); err != nil {
		t.Fatalf("the failed category was destroyed anyway: %v", err)
	}
}

// TestResetReviewStoreLeavesNoStagingResidue proves the staged removal cleans
// up after itself, so a reset never trades one accumulating directory for
// another.
func TestResetReviewStoreLeavesNoStagingResidue(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	seedStoreResetLineage(t, repo, "review-approved", StateApproved)

	if _, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{}); err != nil {
		t.Fatalf("reset: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), storeResetStagingPrefix) {
			t.Fatalf("reset left staging residue %q", entry.Name())
		}
	}
}

// TestResetReviewStoreClassifiesLineagesUnderTheMaintenanceLease is the
// serialization contract, and it is a real race rather than an assertion about
// where a line of code sits.
//
// The test holds the shared side of the maintenance lease, which is exactly
// what a live review holds: every review writer takes it before it opens a
// per-lineage store. A reset that decides what is in flight before it wins that
// contention is deciding from a picture another process is still free to
// change, and because removal renames whole categories rather than the
// lineages the survey enumerated, a review that starts inside that window is
// destroyed by a run that reported none open.
//
// The lease seam is used only as a barrier. It fires at the one point both a
// correct and an incorrect ordering pass through, so the new lineage is created
// strictly after anything the reset decided without mutual exclusion, and
// strictly before the lease it is waiting for is free.
func TestResetReviewStoreClassifiesLineagesUnderTheMaintenanceLease(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	seedStoreResetLineageAt(t, root, "review-approved", StateApproved)

	writer, err := acquireMaintenanceLock(context.Background(),
		filepath.Join(root, "REVIEW-MAINTENANCE.lock"), maintenanceShared)
	if err != nil {
		t.Fatalf("hold the shared maintenance lease: %v", err)
	}
	held := true
	defer func() {
		if held {
			_ = writer.Release()
		}
	}()

	asked := make(chan struct{})
	acquire := storeResetAcquireLease
	stubStoreResetAcquireLease(t, func(ctx context.Context, repo string) (*MaintenanceLock, error) {
		close(asked)
		return acquire(ctx, repo)
	})

	type outcome struct {
		report StoreResetReport
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{})
		done <- outcome{report: report, err: err}
	}()

	<-asked
	// The window: a review starts while the reset is still waiting for the
	// lease. Nothing about it is visible to a survey taken before this point.
	seedStoreResetLineageAt(t, root, "review-active", StateReviewing)
	held = false
	if err := writer.Release(); err != nil {
		t.Fatalf("release the shared maintenance lease: %v", err)
	}

	var result outcome
	select {
	case result = <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("reset never returned")
	}

	var inFlight *StoreResetInFlightError
	if !errors.As(result.err, &inFlight) {
		t.Fatalf("reset racing a review that started before the lease = %v, want a typed refusal", result.err)
	}
	if len(inFlight.Lineages) != 1 || inFlight.Lineages[0].LineageID != "review-active" {
		t.Fatalf("refusal named %#v, want the lineage created inside the window", inFlight.Lineages)
	}
	if result.report.RemovedFiles != 0 || result.report.RemovedBytes != 0 {
		t.Fatalf("the refused reset removed %d file(s)", result.report.RemovedFiles)
	}
	for _, lineage := range []string{"review-approved", "review-active"} {
		path := filepath.Join(root, "review-transactions", "v2", lineage, "review-state.json")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("the reset destroyed %q: %v", lineage, err)
		}
	}
}

// TestResetReviewStoreReleasesTheLeaseAfterRefusing covers the cost of holding
// the lease across the decision: the refusal now returns from inside the
// critical section, so it has to hand the lease back or the next review is
// blocked by a command that removed nothing.
func TestResetReviewStoreReleasesTheLeaseAfterRefusing(t *testing.T) {
	repo := initSnapshotRepo(t)
	seedStoreResetLineage(t, repo, "review-active", StateReviewing)

	if _, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{}); err == nil {
		t.Fatal("reset with an in-flight lineage reported success")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	lock, err := AcquireReviewMaintenanceExclusive(ctx, repo)
	if err != nil {
		t.Fatalf("the refusal kept the maintenance lease: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
}

// TestResetReviewStoreRestoresWorktreeAdminDirsWhenTheCategoryCannotMove is the
// all-or-nothing contract for the one category that is not merely files.
//
// Detaching a candidate view's worktree registration is not reversible by
// deletion, so it must not happen before the move that decides success. If it
// did, a failed rename would leave a checkout on disk whose .git file points at
// an administrative directory that no longer exists: Git commands inside it
// fail, the repository's worktree list no longer names it, and a later review
// that wants the same view finds a directory it can no longer register -- while
// the report calls the category SKIPPED, meaning untouched.
func TestResetReviewStoreRestoresWorktreeAdminDirsWhenTheCategoryCannotMove(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	view, admin := seedStoreResetCandidateView(t, repo, "1a09d6f9")

	stubStoreResetRename(t, func(oldpath, newpath string) error {
		if filepath.Base(oldpath) == "candidate-views" {
			return errors.New("simulated rename failure")
		}
		return os.Rename(oldpath, newpath)
	})

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{})
	var incomplete *StoreResetIncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("reset over an unmovable category = %v, want a typed incomplete run", err)
	}
	entry := storeResetEntryByName(t, report.Removable, "candidate-views")
	if entry.Removed {
		t.Fatalf("the unmovable category counted as removed: %#v", entry)
	}
	if !strings.Contains(entry.Skipped, "simulated rename failure") {
		t.Fatalf("the unmovable category does not carry its reason: %#v", entry)
	}
	if strings.Contains(entry.Skipped, "could not be restored") {
		t.Fatalf("the restore itself failed: %#v", entry)
	}

	// SKIPPED has to mean untouched, and for a registered worktree that covers
	// the registration as much as the checkout.
	if _, err := os.Stat(filepath.Join(view, "README.md")); err != nil {
		t.Fatalf("the skipped candidate view lost its contents: %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(admin, "gitdir"))
	if err != nil {
		t.Fatalf("the skipped candidate view lost its worktree registration: %v", err)
	}
	if got, want := strings.TrimSpace(string(payload)), filepath.Join(view, ".git"); got != want {
		t.Fatalf("restored gitdir = %q, want %q", got, want)
	}

	// The restore puts registrations back where they were rather than leaving
	// them parked in the store.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, staging := range entries {
		if strings.HasPrefix(staging.Name(), storeResetStagingPrefix) {
			t.Fatalf("the failed run left staging residue %q", staging.Name())
		}
	}
}

// TestResetReviewStoreRestoresWorktreeAdminDirsWhenStagingHalfSucceeds covers
// the same rule one step earlier: the preparation itself can fail partway, and
// the views it already detached have to go back too.
func TestResetReviewStoreRestoresWorktreeAdminDirsWhenStagingHalfSucceeds(t *testing.T) {
	repo := initSnapshotRepo(t)
	firstView, firstAdmin := seedStoreResetCandidateView(t, repo, "view-a")
	secondView, secondAdmin := seedStoreResetCandidateView(t, repo, "view-b")

	stubStoreResetRename(t, func(oldpath, newpath string) error {
		if filepath.Base(newpath) == "worktrees-view-b" {
			return errors.New("simulated staging failure")
		}
		return os.Rename(oldpath, newpath)
	})

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{})
	var incomplete *StoreResetIncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("reset over a half-staged category = %v, want a typed incomplete run", err)
	}
	entry := storeResetEntryByName(t, report.Removable, "candidate-views")
	if entry.Removed || !strings.Contains(entry.Skipped, "simulated staging failure") {
		t.Fatalf("the half-staged category = %#v", entry)
	}

	for view, admin := range map[string]string{firstView: firstAdmin, secondView: secondAdmin} {
		if _, err := os.Stat(filepath.Join(view, "README.md")); err != nil {
			t.Fatalf("candidate view %s lost its contents: %v", view, err)
		}
		payload, err := os.ReadFile(filepath.Join(admin, "gitdir"))
		if err != nil {
			t.Fatalf("candidate view %s lost its worktree registration: %v", view, err)
		}
		if got, want := strings.TrimSpace(string(payload)), filepath.Join(view, ".git"); got != want {
			t.Fatalf("restored gitdir for %s = %q, want %q", view, got, want)
		}
	}
}

// stubStoreResetRemoveTree swaps the deletion seam for one test and restores it
// afterwards, mirroring stubStoreResetRename.
func stubStoreResetRemoveTree(t *testing.T, remove func(path string) error) {
	t.Helper()
	previous := storeResetRemoveTree
	storeResetRemoveTree = remove
	t.Cleanup(func() { storeResetRemoveTree = previous })
}

// TestResetReviewStoreKeepsWorktreeAdminDirsItCouldNotRestore is the failure
// this command has no second chance at.
//
// The category is skipped, so the report calls it untouched. But the run had
// already moved its worktree administrative directories into the staging
// directory it is about to delete, and putting one back can fail on its own:
// something recreated the path in the window, the disk is full, or -- on
// Windows -- a process holds a handle inside the directory, which is also the
// most likely reason the first move failed, so this path is at its least
// reliable exactly when it is reached.
//
// Deleting staging then destroys a registration for a checkout that is still on
// disk and still reported as spared. Git commands inside that view fail, the
// repository's worktree list no longer names it, no later run can rebuild it,
// and nothing in the report ever said so. The bytes have to survive, and the
// report has to point at them.
func TestResetReviewStoreKeepsWorktreeAdminDirsItCouldNotRestore(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	firstView, firstAdmin := seedStoreResetCandidateView(t, repo, "view-a")
	_, secondAdmin := seedStoreResetCandidateView(t, repo, "view-b")

	stubStoreResetRename(t, func(oldpath, newpath string) error {
		if filepath.Base(newpath) == "worktrees-view-b" {
			return errors.New("simulated staging failure")
		}
		if filepath.Base(oldpath) == "worktrees-view-a" {
			return errors.New("simulated restore failure")
		}
		return os.Rename(oldpath, newpath)
	})

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{})
	var incomplete *StoreResetIncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("reset over an unrestorable registration = %v, want a typed incomplete run", err)
	}
	entry := storeResetEntryByName(t, report.Removable, "candidate-views")
	if entry.Removed || !strings.Contains(entry.Skipped, "could not be restored") {
		t.Fatalf("the skipped category does not admit the failed restore: %#v", entry)
	}

	// The report has to name the directory and where it went. A skip reason
	// alone is prose; the user needs the path to put back.
	if len(report.UnrestoredAdminDirs) != 1 {
		t.Fatalf("unrestored registrations = %#v, want exactly the one that failed", report.UnrestoredAdminDirs)
	}
	stranded := report.UnrestoredAdminDirs[0]
	if stranded.Original != firstAdmin {
		t.Fatalf("unrestored registration names %q, want %q", stranded.Original, firstAdmin)
	}
	if report.Residue == "" || !strings.HasPrefix(stranded.Staged, report.Residue) {
		t.Fatalf("staged registration %q is not reported under residue %q", stranded.Staged, report.Residue)
	}

	// The bytes are still there, and still usable: the gitdir file that proves
	// which view it registers has to survive with them.
	payload, err := os.ReadFile(filepath.Join(stranded.Staged, "gitdir"))
	if err != nil {
		t.Fatalf("the unrestored registration was destroyed with the staging directory: %v", err)
	}
	if got, want := strings.TrimSpace(string(payload)), filepath.Join(firstView, ".git"); got != want {
		t.Fatalf("stranded gitdir = %q, want %q", got, want)
	}

	// The one that did restore is back where it belongs, and is not reported
	// as stranded.
	if _, err := os.Stat(filepath.Join(secondAdmin, "gitdir")); err != nil {
		t.Fatalf("the restorable registration was not put back: %v", err)
	}
	if _, err := os.Stat(filepath.Join(firstView, "README.md")); err != nil {
		t.Fatalf("the skipped candidate view lost its contents: %v", err)
	}
	if report.Complete {
		t.Fatalf("a run that stranded a registration reported Complete: %#v", report)
	}
	if !strings.HasPrefix(report.Residue, filepath.Join(root, storeResetStagingPrefix)) {
		t.Fatalf("residue %q is not the staging directory under %q", report.Residue, root)
	}
}

// TestResetReviewStoreStillClearsStagingAroundAnUnrestorableRegistration is the
// other half of that rule: keeping what could not be restored must not turn
// into keeping everything. A category that really was removed still goes.
func TestResetReviewStoreStillClearsStagingAroundAnUnrestorableRegistration(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	seedStoreResetLineage(t, repo, "review-approved", StateApproved)
	seedStoreResetCandidateView(t, repo, "view-a")
	seedStoreResetCandidateView(t, repo, "view-b")

	stubStoreResetRename(t, func(oldpath, newpath string) error {
		if filepath.Base(newpath) == "worktrees-view-b" {
			return errors.New("simulated staging failure")
		}
		if filepath.Base(oldpath) == "worktrees-view-a" {
			return errors.New("simulated restore failure")
		}
		return os.Rename(oldpath, newpath)
	})

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{})
	if err == nil {
		t.Fatal("a run that stranded a registration reported success")
	}
	if _, err := os.Lstat(filepath.Join(root, "review-transactions", "v2")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the removable category survived: %v", err)
	}
	entries, err := os.ReadDir(report.Residue)
	if err != nil {
		t.Fatalf("read the kept staging directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "worktrees-view-a" {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("staging kept %v, want only the registration that could not be restored", names)
	}
}

// TestResetReviewStoreNeverReportsNegativeFreedBytes covers the accounting on
// the one path that subtracts.
//
// The administrative directories move into staging with their category, so
// their bytes are freed with it; counting them only on the residual side of the
// subtraction made a failed deletion print a size smaller than nothing. A
// negative freed size is not a small inaccuracy -- it is the report saying the
// command consumed disk, which is the opposite of what it does.
func TestResetReviewStoreNeverReportsNegativeFreedBytes(t *testing.T) {
	repo := initSnapshotRepo(t)
	seedStoreResetCandidateView(t, repo, "view-a")

	// A deletion that fails without unlinking anything: the tree survives, so
	// the residual is everything that was staged, administrative directories
	// included.
	stubStoreResetRemoveTree(t, func(string) error {
		return errors.New("simulated device failure")
	})

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{})
	var incomplete *StoreResetIncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("reset whose staging survived = %v, want a typed incomplete run", err)
	}
	if report.RemovedBytes < 0 {
		t.Fatalf("freed bytes = %d, want never less than nothing", report.RemovedBytes)
	}
	if report.RemovedBytes != 0 {
		t.Fatalf("freed bytes = %d, want 0: nothing left the disk", report.RemovedBytes)
	}
	if report.Residue == "" || report.Complete {
		t.Fatalf("a run whose staging survived = residue %q complete %v", report.Residue, report.Complete)
	}
}

// TestResetReviewStoreCountsWorktreeAdminBytesAsFreed is the same accounting
// from the successful side: the registrations really are deleted, so their
// bytes really were reclaimed and belong in the total.
func TestResetReviewStoreCountsWorktreeAdminBytesAsFreed(t *testing.T) {
	repo := initSnapshotRepo(t)
	view, admin := seedStoreResetCandidateView(t, repo, "view-a")

	_, viewBytes := storeResetUsage(view)
	_, adminBytes := storeResetUsage(admin)
	if adminBytes == 0 {
		t.Fatal("the fixture registration is empty; the assertion below would prove nothing")
	}

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{})
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if report.RemovedBytes != viewBytes+adminBytes {
		t.Fatalf("freed bytes = %d, want %d (candidate view %d + its registration %d)",
			report.RemovedBytes, viewBytes+adminBytes, viewBytes, adminBytes)
	}
}

// TestSurveyReviewStoreWithholdsTheAdapterReviewStore is the coverage rule.
//
// reviews/ is written by the gentle-pi adapter, and neither guarantee this
// command advertises reaches it: maintenanceLockPathForStoreLock returns no
// lease path for anything under it, so the exclusive lease excludes no writer
// there, and storeResetLineages reads only review-transactions/v2, so no review
// living there can ever be listed as in flight. A default run removing it would
// destroy a live review while reporting none open -- the exact failure the
// lease ordering exists to prevent, arrived at from the other direction.
func TestSurveyReviewStoreWithholdsTheAdapterReviewStore(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	writeStoreResetFile(t, filepath.Join(root, "reviews", "IDENTITY"), "{}\n")

	report, err := SurveyReviewStore(context.Background(), repo, StoreResetRequest{})
	if err != nil {
		t.Fatalf("survey: %v", err)
	}
	for _, entry := range report.Removable {
		if entry.Name == "reviews" {
			t.Fatalf("a default survey offers to remove the adapter review store: %#v", entry)
		}
	}
	spared := storeResetEntryByName(t, report.Preserved, "reviews")
	if !spared.Present || spared.Files == 0 {
		t.Fatalf("the withheld category was not measured: %#v", spared)
	}
	// The reason has to name the way out, or the category is simply
	// undeletable as far as the user can tell.
	if !strings.Contains(spared.Reason, "--include-adapter-reviews") {
		t.Fatalf("the withheld category does not name its override: %q", spared.Reason)
	}

	// The lease genuinely does not cover it. This is the claim the reason
	// makes, checked rather than asserted in prose.
	lease, err := maintenanceLockPathForStoreLock(filepath.Join(root, "reviews", "graph-v1", "objects", "aa"))
	if err != nil {
		t.Fatalf("resolve a lease path under the adapter review store: %v", err)
	}
	if lease != "" {
		t.Fatalf("the maintenance lease covers %q after all; the withholding reason is stale", lease)
	}
}

// TestResetReviewStoreSparesTheAdapterReviewStoreByDefault is the behavior that
// rule buys: the bytes stay, and the run is still complete, because a spared
// category is not a failed one.
func TestResetReviewStoreSparesTheAdapterReviewStoreByDefault(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	seedStoreResetLineage(t, repo, "review-approved", StateApproved)
	identity := filepath.Join(root, "reviews", "IDENTITY")
	writeStoreResetFile(t, identity, "{}\n")
	graph := filepath.Join(root, "reviews", "graph-v1", "objects", "aa", "record.json")
	writeStoreResetFile(t, graph, "{}\n")

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{})
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if !report.Complete {
		t.Fatalf("sparing an opt-in category made the run incomplete: %#v", report)
	}
	for _, path := range []string{identity, graph} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("a default reset destroyed the adapter review store at %s: %v", path, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "review-transactions", "v2")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sparing reviews/ also spared the categories this command does cover: %v", err)
	}
}

// TestResetReviewStoreRemovesTheAdapterReviewStoreWhenAsked keeps the exit
// real. Withholding a category is only defensible while there is a way to say
// yes to it.
func TestResetReviewStoreRemovesTheAdapterReviewStoreWhenAsked(t *testing.T) {
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	writeStoreResetFile(t, filepath.Join(root, "reviews", "IDENTITY"), "{}\n")

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{IncludeAdapterReviews: true})
	if err != nil {
		t.Fatalf("opted-in reset: %v", err)
	}
	if entry := storeResetEntryByName(t, report.Removable, "reviews"); !entry.Removed {
		t.Fatalf("the opted-in category was not removed: %#v", entry)
	}
	if _, err := os.Lstat(filepath.Join(root, "reviews")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the opted-in category survived: %v", err)
	}
}

// TestResetReviewStoreRefusalIsNotReportedAsAnAttempt covers the one field a
// machine reader routes on. The refusal returns before the staging directory
// exists, so nothing was opened and nothing was skipped; labelling that run
// "reset" describes a removal that tried every category and failed at all of
// them, which is a far worse outcome than the one that happened.
func TestResetReviewStoreRefusalIsNotReportedAsAnAttempt(t *testing.T) {
	repo := initSnapshotRepo(t)
	seedStoreResetLineage(t, repo, "review-active", StateReviewing)

	report, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{})
	var inFlight *StoreResetInFlightError
	if !errors.As(err, &inFlight) {
		t.Fatalf("reset with an in-flight lineage = %v, want a typed refusal", err)
	}
	if report.Operation != StoreResetPreview {
		t.Fatalf("refused run reported operation %q, want %q", report.Operation, StoreResetPreview)
	}
	for _, entry := range report.Removable {
		if entry.Skipped != "" || entry.Removed {
			t.Fatalf("a refusal reported per-category outcomes: %#v", entry)
		}
	}
}

// TestResetReviewStoreLeaseDeadlineTracksTheRealBound keeps the constant honest
// about what actually bounds the wait.
//
// storeResetLockTimeout is only the context deadline handed to the acquisition;
// the acquisition applies maintenanceLockTimeout internally, and that shorter
// bound is what a contended lease expires on. The previous 30s literal
// documented a wait no run has ever performed, and a comment justifying
// behavior that does not happen is worse than no comment: the next reader
// reasons about contention from a number that is off by an order of magnitude.
func TestResetReviewStoreLeaseDeadlineTracksTheRealBound(t *testing.T) {
	if storeResetLockTimeout != maintenanceLockTimeout+storeResetIdentitySlack {
		t.Fatalf("storeResetLockTimeout = %s, want the real lease bound %s plus its documented identity slack %s",
			storeResetLockTimeout, maintenanceLockTimeout, storeResetIdentitySlack)
	}
	if storeResetLockTimeout <= maintenanceLockTimeout {
		t.Fatalf("storeResetLockTimeout = %s must exceed the lease bound %s, or a slow identity probe cancels an uncontended lease",
			storeResetLockTimeout, maintenanceLockTimeout)
	}

	// And the bound the constant claims to track is the one that fires.
	repo := initSnapshotRepo(t)
	root := storeResetGentleAIRoot(t, repo)
	seedStoreResetLineageAt(t, root, "review-approved", StateApproved)

	ctx, cancel := context.WithTimeout(context.Background(), storeResetLockTimeout)
	defer cancel()
	holder, err := acquireMaintenanceLock(ctx, filepath.Join(root, "REVIEW-MAINTENANCE.lock"), maintenanceExclusive)
	if err != nil {
		t.Fatalf("hold the exclusive maintenance lease: %v", err)
	}
	defer func() { _ = holder.Release() }()

	start := time.Now()
	if _, err := ResetReviewStore(context.Background(), repo, StoreResetRequest{}); err == nil {
		t.Fatal("a reset that never won the lease reported success")
	}
	elapsed := time.Since(start)
	if elapsed >= storeResetLockTimeout {
		t.Fatalf("contended reset waited %s, so the context deadline bounded it, not the lease bound %s",
			elapsed, maintenanceLockTimeout)
	}
	if _, err := os.Stat(filepath.Join(root, "review-transactions", "v2", "review-approved", "review-state.json")); err != nil {
		t.Fatalf("a reset that never won the lease still removed state: %v", err)
	}
}

// TestReviewStoreResetFreedBytesNeverGoesNegative pins the clamp on its own.
//
// The two counts it subtracts are measured at different moments against a tree
// other processes can touch, so their difference can invert for reasons no
// accounting fix inside this command prevents. What the command controls is
// what it prints, and a negative freed size tells the user this command
// consumed disk -- the opposite of what it does, and a claim no reader can
// check.
func TestReviewStoreResetFreedBytesNeverGoesNegative(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		staged, residual int64
		want             int64
	}{
		{name: "everything went away", staged: 4096, residual: 0, want: 4096},
		{name: "some of it survived", staged: 4096, residual: 1024, want: 3072},
		{name: "all of it survived", staged: 4096, residual: 4096, want: 0},
		{name: "more survived than was staged", staged: 4096, residual: 8192, want: 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := storeResetFreedBytes(testCase.staged, testCase.residual); got != testCase.want {
				t.Fatalf("storeResetFreedBytes(%d, %d) = %d, want %d",
					testCase.staged, testCase.residual, got, testCase.want)
			}
		})
	}
}
