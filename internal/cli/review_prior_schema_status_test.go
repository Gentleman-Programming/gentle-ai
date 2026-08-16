package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// These tests pin the negotiated contract surface for prior-schema authority:
// a stored lineage whose snapshot identities were minted by the retired
// pre-fbb55080 formula is inert history, so STATUS naming it must route to
// the ordinary fresh-start transition for the live candidate — and that START
// must actually succeed, creating a new lineage alongside the untouched
// historical record — while a record matching neither formula keeps the
// terminal corrupted_or_unverifiable_authority stop.

// retiredSnapshotIdentityForCLITest reproduces the exact pre-fbb55080
// snapshot-identity formula (verbatim from fbb55080^): v1/v2/
// base-workspace-overlay/v1 domain tags, the untracked-replay proof folded
// into the hashed values, and the intended-untracked paths hashed after them.
// Fixture-only; the fixture below additionally proves the production forensic
// classifier accepts its output before any assertion runs.
func retiredSnapshotIdentityForCLITest(snapshot reviewtransaction.Snapshot) string {
	digest := sha256.New()
	writeLengthPrefixed := func(value string) {
		_, _ = digest.Write([]byte(strconv.Itoa(len(value))))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write([]byte(value))
		_, _ = digest.Write([]byte{0})
	}
	if snapshot.Kind == reviewtransaction.TargetBaseWorkspaceOverlay {
		digest.Write([]byte("gentle-ai.review-snapshot/base-workspace-overlay/v1\x00"))
	} else if snapshot.Projection == reviewtransaction.ProjectionStaged {
		digest.Write([]byte("gentle-ai.review-snapshot/v2\x00"))
	} else {
		digest.Write([]byte("gentle-ai.review-snapshot/v1\x00"))
	}
	values := []string{string(snapshot.Kind), snapshot.BaseTree, snapshot.CandidateTree, snapshot.PathsDigest, snapshot.IntendedUntrackedProof}
	if snapshot.Projection == reviewtransaction.ProjectionStaged {
		values = []string{string(snapshot.Kind), string(snapshot.Projection), snapshot.BaseTree, snapshot.CandidateTree, snapshot.PathsDigest, snapshot.IntendedUntrackedProof}
	}
	for _, value := range values {
		writeLengthPrefixed(value)
	}
	for _, value := range snapshot.IntendedUntracked {
		writeLengthPrefixed(value)
	}
	for _, value := range snapshot.LedgerIDs {
		writeLengthPrefixed(value)
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil))
}

// relocateCompactRecordWithIdentities rewrites a persisted compact record
// under a NEW lineage directory with re-minted snapshot identities, exactly
// like a store written by another release: the historical directory name was
// derived from the historical identity, so it never collides with the lineage
// a fresh START of the same candidate derives today.
func relocateCompactRecordWithIdentities(t *testing.T, repo, fromLineage, toLineage string, mint func(reviewtransaction.Snapshot) string) reviewtransaction.CompactStore {
	t.Helper()
	root := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2")
	payload, err := os.ReadFile(filepath.Join(root, fromLineage, "review-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record reviewtransaction.CompactRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	record.State.LineageID = toLineage
	record.State.InitialSnapshot.Identity = mint(record.State.InitialSnapshot)
	record.State.CurrentSnapshot.Identity = mint(record.State.CurrentSnapshot)
	revision, err := reviewtransaction.CompactRevisionForState(record.State)
	if err != nil {
		t.Fatal(err)
	}
	record.Revision = revision
	rewritten, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, toLineage), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, toLineage, "review-state.json"), rewritten, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, fromLineage)); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, toLineage)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestStatusOffersFreshStartForPriorSchemaLineageAndStartSucceeds(t *testing.T) {
	repo, started, _, _ := newArtifactReview(t, false)
	store := relocateCompactRecordWithIdentities(t, repo, started.LineageID, "prior-schema-history", retiredSnapshotIdentityForCLITest)

	// The fixture must be what the production forensic classifier calls
	// prior-schema; anything else would test the wrong record class.
	_, loadErr := store.Load()
	var semantic *reviewtransaction.CompactSemanticStateError
	if loadErr == nil || !errors.As(loadErr, &semantic) || !semantic.OutdatedIdentity {
		t.Fatalf("prior-schema fixture load error = %v, want an outdated-identity semantic failure", loadErr)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", "prior-schema-history"}, &output); err != nil {
		t.Fatalf("status naming a prior-schema lineage: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.Applicability != reviewtransaction.TargetApplicabilityUnrelated {
		t.Fatalf("prior-schema lineage applicability = %q, want unrelated (fresh start)", status.Applicability)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
		status.NextTransition.ReasonCode != "fresh_target_ready" || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("prior-schema lineage transition = %#v, want the fresh review.start execute", status.NextTransition)
	}

	// An explicit lineage argument appears only when the derived name is
	// occupied; the fixture freed the derived name, so the transition may
	// legitimately omit it and let START derive the fresh lineage itself.
	freshLineage := ""
	for _, argument := range status.NextTransition.Execute.Arguments {
		if argument.Name == "lineage" {
			freshLineage = argument.Value
		}
	}
	if freshLineage == "prior-schema-history" {
		t.Fatalf("fresh start transition renamed the prior-schema lineage itself: %#v", status.NextTransition.Execute.Arguments)
	}
	startArgs := []string{"--cwd", repo}
	if freshLineage != "" {
		startArgs = append(startArgs, "--lineage", freshLineage)
	}
	var startOutput bytes.Buffer
	if err := RunReviewFacadeStart(startArgs, &startOutput); err != nil {
		t.Fatalf("fresh start alongside prior-schema history: %v\n%s", err, startOutput.String())
	}
	var restarted ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startOutput.Bytes(), &restarted)
	if restarted.LineageID == "" || restarted.LineageID == "prior-schema-history" {
		t.Fatalf("fresh start lineage = %q", restarted.LineageID)
	}
	if freshLineage != "" && restarted.LineageID != freshLineage {
		t.Fatalf("fresh start created lineage %q, want the rendered %q", restarted.LineageID, freshLineage)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("fresh start mutated the prior-schema historical record")
	}
	freshStore, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, restarted.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := freshStore.Load(); err != nil {
		t.Fatalf("fresh lineage alongside prior-schema history does not load: %v", err)
	}
}

func TestStatusKeepsTerminalStopForUnrecognizedIdentityLineage(t *testing.T) {
	repo, started, _, _ := newArtifactReview(t, false)
	store := relocateCompactRecordWithIdentities(t, repo, started.LineageID, "unrecognized-history", func(snapshot reviewtransaction.Snapshot) string {
		sum := sha256.Sum256([]byte("unrecognized identity domain " + snapshot.Identity))
		return "sha256:" + hex.EncodeToString(sum[:])
	})

	_, loadErr := store.Load()
	var semantic *reviewtransaction.CompactSemanticStateError
	if loadErr == nil || !errors.As(loadErr, &semantic) || semantic.OutdatedIdentity {
		t.Fatalf("unrecognized-identity fixture load error = %v, want a non-outdated semantic failure", loadErr)
	}

	var output bytes.Buffer
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", "unrecognized-history"}, &output); err != nil {
		t.Fatalf("status naming a corrupted lineage: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.Applicability != reviewtransaction.TargetApplicabilityCorrupted {
		t.Fatalf("unrecognized-identity lineage applicability = %q, want corrupted", status.Applicability)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionStop ||
		status.NextTransition.ReasonCode != "corrupted_or_unverifiable_authority" {
		t.Fatalf("unrecognized-identity lineage transition = %#v, want the terminal corrupted stop", status.NextTransition)
	}
}
