package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// This file is issue #3342: read-only delivery gates collapsed EVERY
// assessment or inventory failure into `authority_corrupted` /
// "complete review authority inventory is unavailable or corrupted" /
// explicit-maintainer-action, even when the underlying cause was retry-safe
// and self-describing (a publication remote the local clone has not fetched,
// or shared store-lock contention). The denial stays a denial (allowed=false,
// fail-closed), but it must name the actual cause and its runnable
// continuation instead of routing every transient condition to a maintainer.

const reviewSharedStoreLockToken = "review-transactions/v2/LOCK"

func reviewCLICommonDir(t *testing.T, repo string) string {
	t.Helper()
	return filepath.Clean(strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")))
}

func reviewCLISharedStoreLockPath(t *testing.T, repo string) string {
	t.Helper()
	return filepath.Join(reviewCLICommonDir(t, repo), "gentle-ai", "review-transactions", "v2", "LOCK")
}

func decodeDeniedGateResult(t *testing.T, gateErr error, payload []byte) ReviewValidateResult {
	t.Helper()
	if gateErr == nil {
		t.Fatalf("gate unexpectedly allowed:\n%s", payload)
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, payload, &result)
	if result.Allowed || result.Result != reviewtransaction.GateInvalidated {
		t.Fatalf("denied gate result = %#v", result)
	}
	return result
}

// TestPrePRGateNamesFetchStalenessAsRetrySafeDenial reproduces occurrence 3 of
// issue #3342: with the local clone behind the advertised remote tip, pre-PR
// boundary selection fails every terminal lineage with "advertised base commit
// is not available locally; fetch before validation", and discovery collapsed
// that into authority_corrupted although nothing in the store is damaged --
// `git fetch origin` alone flips the identical gate to allow.
func TestPrePRGateNamesFetchStalenessAsRetrySafeDenial(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "-q", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	approveDiscoveryMarkdown(t, repo, "review-fetch-staleness", "docs/single.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "deliver reviewed change")

	// Advance the remote past the local clone's fetched state, exactly as a
	// teammate's merge does: the advertised tip is now absent locally.
	seed := filepath.Join(t.TempDir(), "seed")
	runReviewCLIGit(t, repo, "clone", "-q", remote, seed)
	runReviewCLIGit(t, seed, "config", "user.email", "test@example.com")
	runReviewCLIGit(t, seed, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(seed, "remote-advance.txt"), []byte("remote advance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, seed, "add", "remote-advance.txt")
	runReviewCLIGit(t, seed, "commit", "-qm", "advance remote")
	runReviewCLIGit(t, seed, "push", "-q", "origin", "HEAD:refs/heads/"+branch)

	validate := func() (*bytes.Buffer, error) {
		var output bytes.Buffer
		err := RunReviewFacadeValidate([]string{
			"--cwd", repo, "--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/" + branch,
		}, &output)
		return &output, err
	}

	output, gateErr := validate()
	result := decodeDeniedGateResult(t, gateErr, output.Bytes())
	if result.Context.Denial == nil || result.Context.Denial.Stage != "receipt-discovery" || result.Context.Denial.Code != "remote_fetch_required" {
		t.Fatalf("fetch-staleness denial = %#v, want receipt-discovery/remote_fetch_required", result.Context.Denial)
	}
	if result.Action != "retry" {
		t.Fatalf("fetch-staleness action = %q, want retry", result.Action)
	}
	if !strings.Contains(result.Reason, "git fetch origin") {
		t.Fatalf("fetch-staleness reason does not name the runnable continuation: %q", result.Reason)
	}
	if strings.Contains(result.Reason, "unavailable or corrupted") {
		t.Fatalf("fetch-staleness reason still claims corruption: %q", result.Reason)
	}

	var negotiated bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/" + branch,
	}, &negotiated); err == nil {
		t.Fatalf("negotiated fetch-staleness gate unexpectedly allowed:\n%s", negotiated.String())
	}
	failure := decodeReviewIntegrationFailure(t, negotiated.Bytes())
	if failure.Code != "remote_fetch_required" || !failure.RetrySafe || failure.NextAction != "retry" {
		t.Fatalf("negotiated fetch-staleness failure = %#v, want remote_fetch_required/retry_safe/retry", failure)
	}
	if failure.AuthorityApplicability == "corrupted" {
		t.Fatalf("negotiated fetch-staleness failure still classifies authority as corrupted: %#v", failure)
	}
	if !strings.Contains(failure.Cause, "git fetch origin") {
		t.Fatalf("negotiated fetch-staleness cause does not name the runnable continuation: %q", failure.Cause)
	}

	// The named continuation, and nothing else, flips the identical gate.
	runReviewCLIGit(t, repo, "fetch", "-q", "origin")
	output, gateErr = validate()
	if gateErr != nil {
		t.Fatalf("gate still denied after the named continuation: %v\n%s", gateErr, output.String())
	}
	var allowed ReviewValidateResult
	decodeStrictReviewJSON(t, output.Bytes(), &allowed)
	if !allowed.Allowed {
		t.Fatalf("gate after fetch = %#v", allowed)
	}
}

// TestGateDiscoveryNamesLiveLockContentionAsInventoryBusy reproduces
// occurrence 1 of issue #3342: while a concurrent review operation holds the
// shared compact store lock, a transient per-lineage read anomaly (here: a
// terminal record whose receipt is not yet published) was reported as
// authority corruption requiring explicit maintainer action, although the
// identical gate allows once the concurrent writer finishes.
func TestGateDiscoveryNamesLiveLockContentionAsInventoryBusy(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	_, store := approveDiscoveryMarkdown(t, repo, "review-lock-live", "docs/live.md", "reviewed\n")
	lockPath := reviewCLISharedStoreLockPath(t, repo)

	held, err := reviewtransaction.AcquireAuthorityFileLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			_ = held.Release()
		}
	}()

	receipt := store.ReceiptPath()
	hidden := receipt + ".hidden"
	if err := os.Rename(receipt, hidden); err != nil {
		t.Fatal(err)
	}
	restored := false
	defer func() {
		if !restored {
			_ = os.Rename(hidden, receipt)
		}
	}()

	var output bytes.Buffer
	gateErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &output)
	result := decodeDeniedGateResult(t, gateErr, output.Bytes())
	if result.Context.Denial == nil || result.Context.Denial.Stage != "receipt-discovery" || result.Context.Denial.Code != "inventory_busy" {
		t.Fatalf("live-contention denial = %#v, want receipt-discovery/inventory_busy", result.Context.Denial)
	}
	if result.Action != "retry" {
		t.Fatalf("live-contention action = %q, want retry", result.Action)
	}
	if !strings.Contains(result.Reason, reviewSharedStoreLockToken) {
		t.Fatalf("live-contention reason does not name the lock: %q", result.Reason)
	}
	if !strings.Contains(result.Reason, strconv.Itoa(os.Getpid())) {
		t.Fatalf("live-contention reason does not name the recorded holder pid %d: %q", os.Getpid(), result.Reason)
	}
	if strings.Contains(result.Reason, "remove the stale") {
		t.Fatalf("live-contention reason suggests removing a lock whose holder is alive: %q", result.Reason)
	}
	if strings.Contains(result.Reason, "unavailable or corrupted") {
		t.Fatalf("live-contention reason still claims corruption: %q", result.Reason)
	}

	var negotiated bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &negotiated); err == nil {
		t.Fatalf("negotiated live-contention gate unexpectedly allowed:\n%s", negotiated.String())
	}
	failure := decodeReviewIntegrationFailure(t, negotiated.Bytes())
	if failure.Code != "inventory_busy" || !failure.RetrySafe || failure.NextAction != "retry" {
		t.Fatalf("negotiated live-contention failure = %#v, want inventory_busy/retry_safe/retry", failure)
	}

	// A receipt that EXISTS but fails to parse is integrity damage, never
	// inventory_busy, even while the shared lock is verifiably held.
	if err := os.WriteFile(receipt, []byte("{tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	gateErr = RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &output)
	tampered := decodeDeniedGateResult(t, gateErr, output.Bytes())
	if tampered.Context.Denial == nil || tampered.Context.Denial.Code != "authority_corrupted" || tampered.Action == "retry" {
		t.Fatalf("held-lock tampered-receipt result = %#v, want authority_corrupted without a retry promise", tampered)
	}
	if err := os.Remove(receipt); err != nil {
		t.Fatal(err)
	}

	// The same anomaly WITHOUT a live holder is genuine damage and keeps
	// today's fail-closed corruption verdict, byte for byte.
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	released = true
	output.Reset()
	gateErr = RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &output)
	control := decodeDeniedGateResult(t, gateErr, output.Bytes())
	if control.Context.Denial == nil || control.Context.Denial.Code != "authority_corrupted" {
		t.Fatalf("uncontended missing-receipt control = %#v, want authority_corrupted", control.Context.Denial)
	}
	if control.Reason != "complete review authority inventory is unavailable or corrupted" {
		t.Fatalf("uncontended missing-receipt control reason changed: %q", control.Reason)
	}

	// Restoring the receipt (the concurrent writer finishing) allows.
	if err := os.Rename(hidden, receipt); err != nil {
		t.Fatal(err)
	}
	restored = true
	output.Reset()
	if gateErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &output); gateErr != nil {
		t.Fatalf("gate still denied after contention cleared: %v\n%s", gateErr, output.String())
	}
}

// reviewCLIDeadPID returns a pid that verifiably belonged to a process that
// has already exited on this host.
func reviewCLIDeadPID(t *testing.T) int {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run", "^TestNothingMatchesThisNamePattern$", "-test.count=1")
	command.Stdout, command.Stderr = nil, nil
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	return command.Process.Pid
}

func writeReviewCLIStoreLockResidue(t *testing.T, lockPath string, pid int, host string) {
	t.Helper()
	owner := fmt.Sprintf(
		`{"schema":"gentle-ai.review-store-lock/v1","owner_id":"deadbeefdeadbeefdeadbeefdeadbeef","pid":%d,"host":%q,"acquired_at":"2026-08-16T00:00:00Z"}`,
		pid, host,
	)
	// The trailing partial record is the interrupted-rewrite residue an
	// abruptly terminated holder leaves behind: the recorded owner is intact,
	// but the lock file no longer parses as exactly one owner document.
	if err := os.WriteFile(lockPath, []byte(owner+"\n{\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGateDiscoveryNamesDeadOwnerStaleLockResidue reproduces occurrence 2 of
// issue #3342: a process killed mid-operation left the shared store LOCK
// carrying its owner record, and every subsequent gate reported authority
// corruption until the stale lock was removed by hand. The lock schema records
// owner pid and host precisely so this situation is decidable: when the
// recorded pid is verifiably dead on this host, the denial must say so and
// name the removal continuation.
func TestGateDiscoveryNamesDeadOwnerStaleLockResidue(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, repo, "review-lock-dead", "docs/dead.md", "reviewed\n")
	lockPath := reviewCLISharedStoreLockPath(t, repo)
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}

	validate := func() (*bytes.Buffer, error) {
		var output bytes.Buffer
		err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &output)
		return &output, err
	}

	t.Run("dead recorded holder on this host names the removal continuation", func(t *testing.T) {
		deadPID := reviewCLIDeadPID(t)
		writeReviewCLIStoreLockResidue(t, lockPath, deadPID, host)
		output, gateErr := validate()
		result := decodeDeniedGateResult(t, gateErr, output.Bytes())
		if result.Context.Denial == nil || result.Context.Denial.Code != "inventory_busy" {
			t.Fatalf("dead-owner denial = %#v, want inventory_busy", result.Context.Denial)
		}
		if result.Action != "retry" {
			t.Fatalf("dead-owner action = %q, want retry", result.Action)
		}
		for _, token := range []string{reviewSharedStoreLockToken, strconv.Itoa(deadPID), "no longer running", "remove the stale"} {
			if !strings.Contains(result.Reason, token) {
				t.Fatalf("dead-owner reason misses %q: %q", token, result.Reason)
			}
		}

		// The named continuation, and nothing else, flips the identical gate.
		if err := os.Remove(lockPath); err != nil {
			t.Fatal(err)
		}
		output, gateErr = validate()
		if gateErr != nil {
			t.Fatalf("gate still denied after removing the stale lock: %v\n%s", gateErr, output.String())
		}
	})

	t.Run("recorded holder without the flock is unverifiable even when its pid runs here", func(t *testing.T) {
		writeReviewCLIStoreLockResidue(t, lockPath, os.Getpid(), host)
		t.Cleanup(func() { _ = os.Remove(lockPath) })
		output, gateErr := validate()
		result := decodeDeniedGateResult(t, gateErr, output.Bytes())
		if result.Context.Denial == nil || result.Context.Denial.Code != "lock_holder_unverifiable" || result.Action == "retry" {
			t.Fatalf("live-pid result = %#v, want lock_holder_unverifiable without a retry promise", result)
		}
		if !strings.Contains(result.Reason, strconv.Itoa(os.Getpid())) {
			t.Fatalf("live-pid reason does not name the recorded holder pid: %q", result.Reason)
		}
		if strings.Contains(result.Reason, "remove the stale") || strings.Contains(result.Reason, "retry once") {
			t.Fatalf("live-pid reason promises removal or a terminating retry: %q", result.Reason)
		}
	})

	t.Run("foreign-host recorded holder is unverifiable, never a retry promise", func(t *testing.T) {
		pid := reviewCLIDeadPID(t)
		writeReviewCLIStoreLockResidue(t, lockPath, pid, host+"-elsewhere")
		t.Cleanup(func() { _ = os.Remove(lockPath) })
		output, gateErr := validate()
		result := decodeDeniedGateResult(t, gateErr, output.Bytes())
		if result.Context.Denial == nil || result.Context.Denial.Code != "lock_holder_unverifiable" || result.Action == "retry" {
			t.Fatalf("foreign-host result = %#v, want lock_holder_unverifiable without a retry promise", result)
		}
		for _, token := range []string{reviewSharedStoreLockToken, strconv.Itoa(pid), host + "-elsewhere", "known idle"} {
			if !strings.Contains(result.Reason, token) {
				t.Fatalf("foreign-host reason misses %q: %q", token, result.Reason)
			}
		}
		if strings.Contains(result.Reason, "remove the stale") || strings.Contains(result.Reason, "retry once") {
			t.Fatalf("foreign-host reason promises removal or a terminating retry: %q", result.Reason)
		}

		var negotiated bytes.Buffer
		if err := RunReview([]string{"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &negotiated); err == nil {
			t.Fatalf("negotiated foreign-host gate unexpectedly allowed:\n%s", negotiated.String())
		}
		failure := decodeReviewIntegrationFailure(t, negotiated.Bytes())
		if failure.Code != "lock_holder_unverifiable" || failure.RetrySafe || failure.NextAction == "retry" {
			t.Fatalf("negotiated foreign-host failure = %#v, want lock_holder_unverifiable and not retry-safe", failure)
		}
	})

	t.Run("unreadable lock residue keeps the fail-closed corruption verdict", func(t *testing.T) {
		if err := os.WriteFile(lockPath, []byte("not-json\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(lockPath) })
		output, gateErr := validate()
		result := decodeDeniedGateResult(t, gateErr, output.Bytes())
		if result.Context.Denial == nil || result.Context.Denial.Code != "authority_corrupted" {
			t.Fatalf("unreadable-lock control = %#v, want authority_corrupted", result.Context.Denial)
		}
		if result.Reason != "complete review authority inventory is unavailable or corrupted" {
			t.Fatalf("unreadable-lock control reason changed: %q", result.Reason)
		}
	})
}

// TestInventoryBusyProducersRequireProvablySelfClearingContention pins the
// producer set behind reviewInventoryBusyError after the narrowing: (1) a
// typed in-flight lock-contention cause, here and in the all-contended
// aggregate (pinned below); (2) a read anomaly under a verifiably held flock
// (pinned end-to-end above); (3) residue with a held flock or a holder
// verifiably dead on this host (pinned above). Nothing else is ever busy.
func TestInventoryBusyProducersRequireProvablySelfClearingContention(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, repo, "review-busy-producers", "docs/busy.md", "reviewed\n")
	ctx := context.Background()
	contention := fmt.Errorf("read authority: %w", reviewtransaction.ErrStoreLockContended)
	if busy := reviewInventoryBusyFromStoreRead(ctx, repo, contention); busy == nil || busy.Kind != ReviewAuthorityInventoryBusy {
		t.Fatalf("typed in-flight contention = %#v, want inventory_busy", busy)
	}
	if busy := reviewInventoryBusyFromStoreRead(ctx, repo, errors.New("parse compact receipt: unexpected end of JSON input")); busy != nil {
		t.Fatalf("anomaly without a held flock = %#v, want nil (fail-closed corruption)", busy)
	}
}

// TestGateDiscoveryMixedOrUnrecognizedUnknownCausesStayCorrupted pins the
// control of issue #3342: the retry-safe classification exists only when
// EVERY unknown assessment shares one recognized retry-safe cause class.
// Mixed or unrecognized causes keep today's fail-closed corruption verdict.
func TestGateDiscoveryMixedOrUnrecognizedUnknownCausesStayCorrupted(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, repo, "review-mixed-first", "docs/mixed-first.md", "first\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "deliver first")
	approveDiscoveryMarkdown(t, repo, "review-mixed-second", "docs/mixed-second.md", "second\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "deliver second")

	original := assessCompactGateTargetForDiscovery
	t.Cleanup(func() { assessCompactGateTargetForDiscovery = original })
	stub := func(errs map[string]error) {
		assessCompactGateTargetForDiscovery = func(ctx context.Context, repo string, state reviewtransaction.CompactState, input reviewtransaction.NativeGateRequestInput) (reviewtransaction.CompactGateTargetAssessment, error) {
			if err, stubbed := errs[state.LineageID]; stubbed {
				return reviewtransaction.CompactGateTargetAssessment{}, err
			}
			return original(ctx, repo, state, input)
		}
	}
	discover := func() error {
		_, _, err := discoverCompactFacadeGateReview(context.Background(), repo, "", reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply})
		return err
	}
	discoveryKind := func(t *testing.T, err error) ReviewReceiptDiscoveryKind {
		t.Helper()
		var discovery *ReviewReceiptDiscoveryError
		if !errors.As(err, &discovery) {
			t.Fatalf("discovery error = %v", err)
		}
		return discovery.Kind
	}

	t.Run("all fetch-stale assessments classify retry-safe", func(t *testing.T) {
		stub(map[string]error{
			"review-mixed-first":  fmt.Errorf("derive compact gate target: %w", &reviewtransaction.GateRemoteFetchRequiredError{Remote: "origin"}),
			"review-mixed-second": fmt.Errorf("derive compact gate target: %w", &reviewtransaction.GateRemoteFetchRequiredError{Remote: "origin"}),
		})
		if kind := discoveryKind(t, discover()); kind != ReviewGateRemoteFetchRequired {
			t.Fatalf("all-fetch-stale discovery kind = %q, want %q", kind, ReviewGateRemoteFetchRequired)
		}
	})

	t.Run("all lock-contended assessments classify retry-safe", func(t *testing.T) {
		stub(map[string]error{
			"review-mixed-first":  fmt.Errorf("read authority: %w", reviewtransaction.ErrStoreLockContended),
			"review-mixed-second": &reviewtransaction.AuthorityLockTimeoutError{Timeout: 0},
		})
		if kind := discoveryKind(t, discover()); kind != ReviewAuthorityInventoryBusy {
			t.Fatalf("all-lock-contended discovery kind = %q, want %q", kind, ReviewAuthorityInventoryBusy)
		}
	})

	t.Run("mixed retry-safe causes stay corrupted", func(t *testing.T) {
		stub(map[string]error{
			"review-mixed-first":  fmt.Errorf("derive compact gate target: %w", &reviewtransaction.GateRemoteFetchRequiredError{Remote: "origin"}),
			"review-mixed-second": fmt.Errorf("read authority: %w", reviewtransaction.ErrStoreLockContended),
		})
		if kind := discoveryKind(t, discover()); kind != ReviewAuthorityCorrupted {
			t.Fatalf("mixed-cause discovery kind = %q, want %q", kind, ReviewAuthorityCorrupted)
		}
	})

	t.Run("unrecognized cause stays corrupted", func(t *testing.T) {
		stub(map[string]error{
			"review-mixed-first":  fmt.Errorf("derive compact gate target: %w", &reviewtransaction.GateRemoteFetchRequiredError{Remote: "origin"}),
			"review-mixed-second": errors.New("assessment exploded"),
		})
		if kind := discoveryKind(t, discover()); kind != ReviewAuthorityCorrupted {
			t.Fatalf("unrecognized-cause discovery kind = %q, want %q", kind, ReviewAuthorityCorrupted)
		}
	})
}
