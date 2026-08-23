package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// storeResetSeedLineage writes one compact-v2 lineage record straight to disk.
// Going around the normal writers is the point: this command has to work on a
// store the normal writers can no longer open.
func storeResetSeedLineage(t *testing.T, repo, lineage string, state reviewtransaction.State) {
	t.Helper()
	dir := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", lineage)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := `{"schema":"gentle-ai.review-state-record/v2","revision":"sha256:00","state":{"schema":"gentle-ai.review-state/v2","lineage_id":"` +
		lineage + `","state":"` + string(state) + `"}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "review-state.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
}

func storeResetStoreRoot(t *testing.T, repo string) string {
	t.Helper()
	return filepath.Dir(reviewCLIAuthorityRoot(t, repo))
}

// TestReviewStoreResetPreviewsByDefault is the safety default. The command is
// irreversible and clone-wide, so the bare invocation reports and stops.
func TestReviewStoreResetPreviewsByDefault(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	storeResetSeedLineage(t, repo, "review-approved", reviewtransaction.StateApproved)

	var output bytes.Buffer
	if err := RunReview([]string{"store-reset", "--cwd", repo}, &output); err != nil {
		t.Fatalf("review store-reset preview: %v\n%s", err, output.String())
	}
	rendered := output.String()
	if !strings.Contains(rendered, "--confirm") {
		t.Fatalf("preview does not say how to apply it:\n%s", rendered)
	}
	if !strings.Contains(rendered, "review-transactions/v2") {
		t.Fatalf("preview does not name the category it would remove:\n%s", rendered)
	}
	if _, err := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "review-approved", "review-state.json")); err != nil {
		t.Fatalf("preview removed lineage state: %v", err)
	}
}

// TestReviewStoreResetPreviewReportsBothLists proves the preview names what it
// spares and why, not only what it takes.
func TestReviewStoreResetPreviewReportsBothLists(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	storeResetSeedLineage(t, repo, "review-approved", reviewtransaction.StateApproved)
	disableReviewForClone(t, repo)

	var output bytes.Buffer
	if err := RunReview([]string{"store-reset", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatalf("review store-reset preview: %v\n%s", err, output.String())
	}
	var result ReviewStoreResetResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Operation != "review/store-reset" {
		t.Fatalf("operation = %q", result.Operation)
	}
	if result.Report.Operation != reviewtransaction.StoreResetPreview {
		t.Fatalf("report operation = %q, want a preview", result.Report.Operation)
	}
	if len(result.Report.Preserved) == 0 || len(result.Report.Removable) == 0 {
		t.Fatalf("preview report = %#v", result.Report)
	}
	for _, entry := range append(result.Report.Removable, result.Report.Preserved...) {
		if strings.TrimSpace(entry.Reason) == "" {
			t.Fatalf("category %q carries no reason", entry.Name)
		}
	}
}

// TestReviewStoreResetRefusesInFlightAndNamesTheOverride is the refusal the
// maintainer asked for: it lists the open reviews and does not make the
// dangerous path reachable by accident.
func TestReviewStoreResetRefusesInFlightAndNamesTheOverride(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	storeResetSeedLineage(t, repo, "review-approved", reviewtransaction.StateApproved)
	storeResetSeedLineage(t, repo, "review-open", reviewtransaction.StateReviewing)

	var output bytes.Buffer
	err := RunReview([]string{"store-reset", "--cwd", repo, "--confirm"}, &output)
	if err == nil {
		t.Fatalf("review store-reset destroyed an open review\n%s", output.String())
	}
	var inFlight *reviewtransaction.StoreResetInFlightError
	if !errors.As(err, &inFlight) {
		t.Fatalf("refusal is not typed: %v", err)
	}
	if !strings.Contains(err.Error(), "review-open") || !strings.Contains(err.Error(), "--include-in-flight") {
		t.Fatalf("refusal does not name the lineage and the override: %v", err)
	}
	// A refusal returns before opening the store, so it must not be rendered as
	// a removal that skipped everything: that reads as a half-processed store.
	rendered := output.String()
	if strings.Contains(rendered, "SKIPPED") || strings.Contains(rendered, "removed while still open") {
		t.Fatalf("the refusal is rendered as an attempted removal:\n%s", rendered)
	}
	if !strings.Contains(rendered, "would remove") || !strings.Contains(rendered, "still open") {
		t.Fatalf("the refusal does not read as a preview:\n%s", rendered)
	}
	for _, lineage := range []string{"review-approved", "review-open"} {
		if _, statErr := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", lineage, "review-state.json")); statErr != nil {
			t.Fatalf("a refused reset removed %q: %v", lineage, statErr)
		}
	}
}

// TestReviewStoreResetConfirmRemovesSettledState is the ordinary success path.
func TestReviewStoreResetConfirmRemovesSettledState(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	storeResetSeedLineage(t, repo, "review-approved", reviewtransaction.StateApproved)
	root := storeResetStoreRoot(t, repo)
	views := filepath.Join(root, "candidate-views", "1a09d6f9")
	if err := os.MkdirAll(views, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(views, "README.md"), []byte("checkout\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{"store-reset", "--cwd", repo, "--confirm", "--json"}, &output); err != nil {
		t.Fatalf("review store-reset --confirm: %v\n%s", err, output.String())
	}
	var result ReviewStoreResetResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if !result.Report.Complete || result.Report.RemovedFiles == 0 || result.Report.RemovedBytes == 0 {
		t.Fatalf("confirmed reset report = %#v", result.Report)
	}
	for _, path := range []string{
		filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2"),
		filepath.Join(root, "candidate-views"),
	} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s survived a confirmed reset: %v", path, err)
		}
	}
}

// TestReviewStoreResetOverrideRemovesInFlightState proves the escape hatch is
// reachable once the user asks for it in as many words.
func TestReviewStoreResetOverrideRemovesInFlightState(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	storeResetSeedLineage(t, repo, "review-open", reviewtransaction.StateReviewing)

	var output bytes.Buffer
	if err := RunReview([]string{"store-reset", "--cwd", repo, "--confirm", "--include-in-flight"}, &output); err != nil {
		t.Fatalf("override reset: %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), "review-open") {
		t.Fatalf("override output does not name what it destroyed:\n%s", output.String())
	}
	if _, err := os.Lstat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("override reset kept the open lineage: %v", err)
	}
}

// TestReviewStoreResetKeepsTheKillSwitchOff is the regression guard for the
// worst outcome this command could produce: silently turning reviews back on
// for someone who turned them off.
func TestReviewStoreResetKeepsTheKillSwitchOff(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	storeResetSeedLineage(t, repo, "review-approved", reviewtransaction.StateApproved)
	disableReviewForClone(t, repo)

	var output bytes.Buffer
	if err := RunReview([]string{"store-reset", "--cwd", repo, "--confirm"}, &output); err != nil {
		t.Fatalf("reset while reviews are off: %v\n%s", err, output.String())
	}

	var mode bytes.Buffer
	if err := RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &mode); err != nil {
		t.Fatalf("review mode status: %v\n%s", err, mode.String())
	}
	if effective := decodeReviewModeResult(t, mode.Bytes()).Status.Effective; effective != reviewtransaction.RDDModeOff {
		t.Fatalf("the reset re-enabled reviews: effective mode = %q", effective)
	}
}

// TestReviewStoreResetRunsWhileReviewsAreDisabled states the deliberate gap in
// the kill-switch gate. A user whose store degraded and who then turned reviews
// off must still be able to clean up; refusing here would rebuild the dead end
// this command exists to remove.
func TestReviewStoreResetRunsWhileReviewsAreDisabled(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	storeResetSeedLineage(t, repo, "review-approved", reviewtransaction.StateApproved)
	disableReviewForClone(t, repo)

	var output bytes.Buffer
	err := RunReview([]string{"store-reset", "--cwd", repo, "--confirm"}, &output)
	if errors.Is(err, reviewtransaction.ErrRDDDisabled) {
		t.Fatalf("store-reset refused because reviews are off: %v", err)
	}
	if err != nil {
		t.Fatalf("store-reset while reviews are off: %v\n%s", err, output.String())
	}
	if _, statErr := os.Lstat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the reset did nothing while reviews were off: %v", statErr)
	}
}

// TestReviewStoreResetReportsPartialFailureWithoutClaimingSuccess covers the
// honesty requirement end to end: the CLI still prints the report, still exits
// non-zero, and never shows a category it failed to remove as removed.
func TestReviewStoreResetReportsPartialFailureWithoutClaimingSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the permission bit this test withholds is not enforced on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root ignores the permission bit this test withholds")
	}
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	storeResetSeedLineage(t, repo, "review-approved", reviewtransaction.StateApproved)
	root := storeResetStoreRoot(t, repo)
	reviews := filepath.Join(root, "reviews")
	if err := os.MkdirAll(reviews, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reviews, "IDENTITY"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Moving a directory to a new parent needs write permission on the
	// directory itself, so clearing that bit makes this one category
	// genuinely unmovable while every other category still works.
	if err := os.Chmod(reviews, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(reviews, 0o755) })

	var output bytes.Buffer
	err := RunReview([]string{"store-reset", "--cwd", repo, "--confirm", "--include-adapter-reviews", "--json"}, &output)
	if err == nil {
		t.Fatalf("a partial reset exited zero\n%s", output.String())
	}
	// The report is still emitted, because a caller that cannot see what was
	// removed cannot reconcile the store.
	var result ReviewStoreResetResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Report.Complete {
		t.Fatalf("a partial reset reported Complete: %#v", result.Report)
	}
	var failed reviewtransaction.StoreResetEntry
	for _, entry := range result.Report.Removable {
		if entry.Name == "reviews" {
			failed = entry
		}
	}
	if failed.Removed || failed.Skipped == "" {
		t.Fatalf("the failed category = %#v", failed)
	}
	if _, statErr := os.Stat(filepath.Join(root, "reviews", "IDENTITY")); statErr != nil {
		t.Fatalf("the failed category was destroyed anyway: %v", statErr)
	}
}

// TestReviewStoreResetRejectsUnexpectedArguments keeps the verb aligned with
// the parse-rejection shape the documented-invocation guard recognizes.
func TestReviewStoreResetRejectsUnexpectedArguments(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)

	var output bytes.Buffer
	err := RunReview([]string{"store-reset", "--cwd", repo, "stray"}, &output)
	if err == nil || !strings.Contains(err.Error(), "unexpected review store-reset argument") {
		t.Fatalf("unexpected argument error = %v", err)
	}
}

// TestReviewStoreResetIsNotOnTheNegotiatedRoute keeps the verb user-initiated.
// The negotiated contract is the machine surface; a destructive clone-wide
// reset has no business being reachable from it, exactly as abandon and reclaim
// are not.
func TestReviewStoreResetIsNotOnTheNegotiatedRoute(t *testing.T) {
	for _, operation := range reviewIntegrationOperationRegistry {
		if operation.Command == "store-reset" || strings.Contains(string(operation.Operation), "store-reset") {
			t.Fatalf("store-reset is reachable on the negotiated machine route: %#v", operation)
		}
	}
}

// TestReviewStoreResetWithdrawsTheUntouchedPromiseWhenARestoreFailed is the
// line that must not survive a stranded worktree registration.
//
// "nothing above marked SKIPPED was removed" is the whole reassurance a partial
// run offers. When the run moved a candidate view's worktree administrative
// directory aside and could not put it back, that sentence is false: the
// checkout is where it was, but the registration that makes it a worktree is
// not, and printing the reassurance anyway leaves the user with no reason to go
// looking.
func TestReviewStoreResetWithdrawsTheUntouchedPromiseWhenARestoreFailed(t *testing.T) {
	report := reviewtransaction.StoreResetReport{
		Schema: reviewtransaction.StoreResetReportSchema, Operation: reviewtransaction.StoreResetApplied,
		Repository: "/repo", StoreRoot: "/repo/.git/gentle-ai",
		Removable: []reviewtransaction.StoreResetEntry{{
			Name: "candidate-views", Present: true, Files: 4, Bytes: 2048,
			Skipped: "simulated staging failure; and the worktree administrative directories moved aside for it could not be restored: simulated restore failure",
		}},
		Residue: "/repo/.git/gentle-ai/.store-reset-42",
		UnrestoredAdminDirs: []reviewtransaction.StoreResetUnrestoredAdminDir{{
			Original: "/repo/.git/worktrees/view-a",
			Staged:   "/repo/.git/gentle-ai/.store-reset-42/worktrees-view-a",
		}},
		Complete: false,
	}

	var output bytes.Buffer
	if err := renderReviewStoreReset(&output, report, true, true); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	if strings.Contains(rendered, "nothing above marked SKIPPED was removed") {
		t.Fatalf("a run that stranded a registration still promised nothing was removed:\n%s", rendered)
	}
	for _, want := range []string{
		"/repo/.git/worktrees/view-a",
		"/repo/.git/gentle-ai/.store-reset-42/worktrees-view-a",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("the output does not name %q:\n%s", want, rendered)
		}
	}
}

// TestReviewStoreResetKeepsTheUntouchedPromiseForAnOrdinaryPartialRun is the
// other side: withdrawing the reassurance whenever anything failed would drop
// it from the case it was written for.
func TestReviewStoreResetKeepsTheUntouchedPromiseForAnOrdinaryPartialRun(t *testing.T) {
	report := reviewtransaction.StoreResetReport{
		Schema: reviewtransaction.StoreResetReportSchema, Operation: reviewtransaction.StoreResetApplied,
		Repository: "/repo", StoreRoot: "/repo/.git/gentle-ai",
		Removable: []reviewtransaction.StoreResetEntry{{
			Name: "review-transactions/v2", Present: true, Files: 4, Bytes: 2048, Skipped: "permission denied",
		}},
		Complete: false,
	}

	var output bytes.Buffer
	if err := renderReviewStoreReset(&output, report, true, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "nothing above marked SKIPPED was removed") {
		t.Fatalf("an ordinary partial run lost its reassurance:\n%s", output.String())
	}
}

// TestReviewStoreResetSparesTheAdapterReviewStoreByDefault is the coverage rule
// at the surface the user types. reviews/ is written by the gentle-pi adapter,
// which this command's maintenance lease does not exclude and whose reviews its
// in-flight refusal cannot see, so a default run has to leave it alone and say
// why.
func TestReviewStoreResetSparesTheAdapterReviewStoreByDefault(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	storeResetSeedLineage(t, repo, "review-approved", reviewtransaction.StateApproved)
	root := storeResetStoreRoot(t, repo)
	identity := filepath.Join(root, "reviews", "IDENTITY")
	if err := os.MkdirAll(filepath.Dir(identity), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(identity, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{"store-reset", "--cwd", repo, "--confirm", "--json"}, &output); err != nil {
		t.Fatalf("reset: %v\n%s", err, output.String())
	}
	var result ReviewStoreResetResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	for _, entry := range result.Report.Removable {
		if entry.Name == "reviews" {
			t.Fatalf("a default run offered to remove the adapter review store: %#v", entry)
		}
	}
	if _, err := os.Stat(identity); err != nil {
		t.Fatalf("a default run destroyed the adapter review store: %v", err)
	}

	// And the override actually removes it, so the withholding is a decision
	// rather than a dead end.
	output.Reset()
	if err := RunReview([]string{"store-reset", "--cwd", repo, "--confirm", "--include-adapter-reviews"}, &output); err != nil {
		t.Fatalf("opted-in reset: %v\n%s", err, output.String())
	}
	if _, err := os.Lstat(filepath.Join(root, "reviews")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the override did not remove the adapter review store: %v", err)
	}
}
