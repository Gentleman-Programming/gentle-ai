package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// The OpenCode relay is spawned in the host session directory, which names
// nothing about the review. These tests pin that the provider-issued rctx3
// handle alone selects the repository: the session may be a superproject, a
// git-less parent, or an unrelated repository, and dead registrations on the
// host never participate.

func TestOpenCodeReviewTransportResolvesASubmoduleReviewFromItsSuperprojectSession(t *testing.T) {
	if testing.Short() {
		t.Skip("requires git submodules and relay subprocesses")
	}
	reviewEnabledHome(t)
	module := initReviewCLIRepo(t)
	superproject := initReviewCLIRepo(t)
	runReviewCLIGit(t, superproject, "-c", "protocol.file.allow=always", "submodule", "add", "-q", module, "module")
	runReviewCLIGit(t, superproject, "commit", "-qm", "register module")
	target := filepath.Join(superproject, "module")
	writeReviewStartCandidate(t, target, "tracked.txt", "candidate\n", 0o644)
	started := startFacadeReview(t, target)
	store, record := loadOpenCodeRelayAuthority(t, target, started.LineageID)

	completeOpenCodeRelayFromSession(t, superproject, target, record)
	// A submodule keeps its authority under the superproject's
	// .git/modules, so acknowledge from the module itself.
	current, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	acknowledgement, pending := reviewtransaction.PendingApprovedCompactAcknowledgement(current)
	if !pending {
		t.Fatalf("submodule relay did not close approved: %#v", current.State)
	}
	if err := RunReview([]string{
		"acknowledge-approved", "--cwd", target, "--lineage", acknowledgement.LineageID,
		"--target", acknowledgement.TargetIdentity, "--expected-revision", acknowledgement.ExpectedRevision, "--token", acknowledgement.Token,
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(store.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("approved submodule authority survived: %v", err)
	}
	if _, _, err := discoverCompactFacadeReview(t.Context(), superproject, started.LineageID, false); err == nil {
		t.Fatal("superproject-session relay selected authority in the superproject")
	}
}

func TestOpenCodeReviewTransportResolvesFromAGitlessParentSession(t *testing.T) {
	if testing.Short() {
		t.Skip("requires relay subprocesses")
	}
	reviewEnabledHome(t)
	target, _, store, record := newArtifactReview(t, false)
	parent := filepath.Dir(target)
	if _, err := os.Stat(filepath.Join(parent, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fixture parent %q must not be a Git repository: %v", parent, err)
	}

	completeOpenCodeRelayFromSession(t, parent, target, record)
	assertApprovedCompactAuthorityBurned(t, store, record.State.LineageID)
}

func TestOpenCodeReviewTransportResolvesFromAnUnrelatedRepositorySession(t *testing.T) {
	if testing.Short() {
		t.Skip("requires relay subprocesses")
	}
	reviewEnabledHome(t)
	target, _, store, record := newArtifactReview(t, false)
	host := initReviewCLIRepo(t)

	completeOpenCodeRelayFromSession(t, host, target, record)
	assertApprovedCompactAuthorityBurned(t, store, record.State.LineageID)
	if _, _, err := discoverCompactFacadeReview(t.Context(), host, record.State.LineageID, false); err == nil {
		t.Fatal("unrelated-session relay created or selected authority in the session repository")
	}
}

func TestOpenCodeReviewTransportIgnoresDeadRegisteredWorktreesOnTheHost(t *testing.T) {
	if testing.Short() {
		t.Skip("requires git worktrees and relay subprocesses")
	}
	reviewEnabledHome(t)
	target, _, store, record := newArtifactReview(t, false)
	dead := filepath.Join(canonicalReviewCLITempDir(t), "aaa-dead-worktree")
	runReviewCLIGit(t, target, "worktree", "add", "-q", "-b", "dead-registration", dead, "HEAD")
	t.Cleanup(func() { _ = runReviewCLIGitAllowFailure(target, "worktree", "remove", "--force", dead) })
	if err := os.RemoveAll(filepath.Join(dead, ".git")); err != nil {
		t.Fatal(err)
	}

	completeOpenCodeRelayFromSession(t, target, target, record)
	assertApprovedCompactAuthorityBurned(t, store, record.State.LineageID)
}

func TestOpenCodeReviewTransportRefusesATamperedHandleWithoutAuthorityMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("requires relay subprocesses")
	}
	reviewEnabledHome(t)
	target, started, store, record := newArtifactReview(t, false)
	start := openCodeLensTransportStart(t, target, record, record.State.SelectedLenses[0])
	handle := openCodeTransportStartHandle(t, start)
	// Flip one character inside the sealed payload; the base64url alphabet
	// keeps the shape valid so only authentication can catch it.
	index := len(handle) - 5
	replacement := byte('A')
	if handle[index] == 'A' {
		replacement = 'B'
	}
	tampered := handle[:index] + string(replacement) + handle[index+1:]
	start.Prompt = strings.Replace(start.Prompt, handle, tampered, 1)

	err := runOpenCodeRelayStartFrom(t, initReviewCLIRepo(t), start)
	var bindingErr *openCodeTransportBindingError
	if !errors.As(err, &bindingErr) || openCodeTransportRefusalReason(err) != openCodeRefusalBindingMismatch {
		t.Fatalf("tampered handle relay error = %v", err)
	}
	if strings.Contains(err.Error(), target) {
		t.Fatalf("tampered handle refusal leaked the repository path: %v", err)
	}
	assertOpenCodeRelayAuthorityUnchanged(t, target, started.LineageID, store, record)
}

// The relay resolves only the sealed handle OpenCode STATUS issues. An rctx2
// digest -- what every other runtime still receives -- names no root, so the
// relay refuses it with the STATUS that reissues the Task instead of searching
// the host session.
func TestOpenCodeReviewTransportRefusesAnRctx2HandleWithAFreshStatusContinuation(t *testing.T) {
	// Inheritance applies only without the Pi relay handshake; a Pi host
	// running this test must not turn the STATUS into a Pi-driven one.
	t.Setenv(reviewPiHostRelayContractEnvironment, "")
	if testing.Short() {
		t.Skip("requires relay subprocesses")
	}
	reviewEnabledHome(t)
	target, started, store, record := openCodeSealedLineage(t, "opencode-rctx2-remedy")
	_, status := openCodeSealedStatus(t, target, started.LineageID, "")
	issued := openCodeSealedCollectInput(t, status)
	if issued.ProviderTask == nil {
		t.Fatalf("OpenCode collect input carries no provider task: %#v", issued)
	}
	digest, err := reviewtransaction.DeriveReviewRepositoryContextHandle(t.Context(), target, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	})
	if err != nil || !strings.HasPrefix(digest, "rctx2_") {
		t.Fatalf("rctx2 derivation = %q, %v", digest, err)
	}
	start := openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "start", Prompt: issued.ProviderTask.Prompt}
	start.Prompt = strings.Replace(start.Prompt, openCodeTransportStartHandle(t, start), digest, 1)

	// Even from the repository itself: the relay never resolves rctx2.
	err = runOpenCodeRelayStartFrom(t, target, start)
	var bindingErr *openCodeTransportBindingError
	if !errors.As(err, &bindingErr) || openCodeTransportRefusalReason(err) != openCodeRefusalStaleAuthority {
		t.Fatalf("rctx2 relay error = %v, want a typed stale refusal", err)
	}
	assertOpenCodeRelayAuthorityUnchanged(t, target, started.LineageID, store, record)

	// The remedy is the exact bound continuation: running it reissues the
	// current provider Task, not a selectorless START offer.
	_, remedy, found := strings.Cut(err.Error(), "`")
	remedy, _, closed := strings.Cut(remedy, "`")
	if !found || !closed || !strings.Contains(remedy, "--lineage "+started.LineageID) {
		t.Fatalf("rctx2 relay remedy %q does not name the bound lineage", err.Error())
	}
	args := strings.Fields(strings.ReplaceAll(remedy, "<repo>", target))
	if len(args) < 3 || args[0] != "gentle-ai" {
		t.Fatalf("rctx2 relay remedy is not a gentle-ai command: %q", remedy)
	}
	var output bytes.Buffer
	if err := RunReview(args[2:], &output); err != nil || args[1] != "review" {
		t.Fatalf("rctx2 relay remedy %q failed: %v\n%s", remedy, err, output.String())
	}
	var reissued ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &reissued)
	if again := openCodeSealedCollectInput(t, reissued); again.ProviderTask == nil || again.ProviderTask.Prompt != issued.ProviderTask.Prompt {
		t.Fatalf("rctx2 relay remedy did not reissue the current Task: %#v", reissued.NextTransition)
	}
}

func loadOpenCodeRelayAuthority(t *testing.T, repo, lineage string) (reviewtransaction.CompactStore, reviewtransaction.CompactRecord) {
	t.Helper()
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return store, record
}

// completeOpenCodeRelayFromSession runs one relay with its process cwd at the
// host session directory and completes the first selected lens.
func completeOpenCodeRelayFromSession(t *testing.T, session, target string, record reviewtransaction.CompactRecord) {
	t.Helper()
	lens := record.State.SelectedLenses[0]
	start := openCodeLensTransportStart(t, target, record, lens)
	hostOutput := string(admittedReviewerPayloadForTest(t, target, record, lens, 0))
	relay := startOpenCodeTransportRelay(t, session, start)
	if _, err := relay.complete(openCodeTransportEnvelope{
		Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: relay.prompt.Nonce, Output: &hostOutput,
	}); err != nil {
		t.Fatal(err)
	}
}

func runOpenCodeRelayStartFrom(t *testing.T, session string, start openCodeTransportEnvelope) error {
	t.Helper()
	t.Chdir(session)
	payload, err := json.Marshal(start)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = runReviewOpenCodeTransport(nil, bytes.NewReader(payload), &output)
	if err == nil {
		t.Fatalf("relay unexpectedly started: %q", output.String())
	}
	if output.Len() != 0 {
		t.Fatalf("refused relay released output: %q", output.String())
	}
	return err
}

func openCodeTransportStartHandle(t *testing.T, start openCodeTransportEnvelope) string {
	t.Helper()
	binding, err := decodeOpenCodeTransportBinding(start.Prompt)
	if err != nil {
		t.Fatal(err)
	}
	return binding.RepositoryContext
}

func runReviewCLIGitAllowFailure(repo string, args ...string) error {
	return exec.Command("git", append([]string{"-C", repo}, args...)...).Run()
}
