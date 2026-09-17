package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

// Issue #4494: the consent answer re-enters negotiated START, which rebuilds
// the live snapshot and refuses a moved candidate with an opaque identity
// mismatch. These tests pin the self-describing continuation: the negotiated
// evidence travels beside the identity hash, the stale failure decomposes the
// mismatch into a truthful cause, and next_action branches between
// wait-and-rederive and stop.

// reviewTargetEvidenceRelaySetup prepares one RDD-enabled repository with a
// medium-risk candidate and returns the repo plus the relayed consent
// question for its negotiated candidate.
func reviewTargetEvidenceRelaySetup(t *testing.T) (string, ReviewIntegrationConsentResult) {
	t.Helper()
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o755)
	output := runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "review-target-evidence", "--consent", "relay",
	}))
	question := decodeConsentQuestion(t, output.Bytes())
	return repo, question
}

// testTargetEvidenceFromInvocation extracts the --target-evidence token from a
// runnable continuation invocation. It stays string-level so the primary
// behavioral tests compile against today's shipped contract.
func testTargetEvidenceFromInvocation(t *testing.T, invocation string) string {
	t.Helper()
	const flag = " --target-evidence "
	index := strings.Index(invocation, flag)
	if index < 0 {
		t.Fatalf("continuation invocation carries no --target-evidence token: %q", invocation)
	}
	rest := invocation[index+len(flag):]
	if end := strings.Index(rest, " "); end >= 0 {
		rest = rest[:end]
	}
	if rest == "" {
		t.Fatalf("continuation invocation carries an empty --target-evidence token: %q", invocation)
	}
	return rest
}

// testStripFlagFromInvocation removes one flag and its value from a runnable
// invocation. When the flag is absent the invocation is returned unchanged,
// so the legacy no-evidence path stays covered before and after the fix.
func testStripFlagFromInvocation(t *testing.T, invocation, flag string) string {
	t.Helper()
	needle := " " + flag + " "
	index := strings.Index(invocation, needle)
	if index < 0 {
		return invocation
	}
	rest := invocation[index+len(needle):]
	if end := strings.Index(rest, " "); end >= 0 {
		rest = rest[end:]
	} else {
		rest = ""
	}
	return invocation[:index] + rest
}

func decodeReviewFailureMap(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	var failure map[string]any
	if err := json.Unmarshal(payload, &failure); err != nil {
		t.Fatalf("decode failure envelope: %v\n%s", err, payload)
	}
	return failure
}

// TestReviewConsentAnswerNamesMovedCandidate reproduces issue #4494: a
// concurrent write between negotiation and the human answer must fail with a
// truthful moved-candidate cause instead of an opaque identity mismatch whose
// recovery advice re-derives into an unbounded loop.
func TestReviewConsentAnswerNamesMovedCandidate(t *testing.T) {
	repo, question := reviewTargetEvidenceRelaySetup(t)
	granted := question.Choices[0].Invocation
	token := testTargetEvidenceFromInvocation(t, granted)
	declined := question.Choices[1].Invocation
	if !strings.Contains(declined, " --target-evidence "+token+" ") {
		t.Fatalf("declined continuation lost the evidence token: %q", declined)
	}

	// The concurrent writer advances the candidate after negotiation and
	// before the human answer lands.
	writeReviewStartCandidate(t, repo, "scripts/second.sh", "echo more\n", 0o755)

	var stdout bytes.Buffer
	err := RunReview(invocationArgs(t, granted), &stdout)
	if err == nil {
		t.Fatalf("granted answer on a moved candidate must fail: %s", stdout.String())
	}
	failure := decodeReviewFailureMap(t, stdout.Bytes())
	if failure["code"] != reviewPreflightStaleTargetCode {
		t.Fatalf("moved-candidate code = %v, want %q\n%s", failure["code"], reviewPreflightStaleTargetCode, stdout.String())
	}
	if failure["next_action"] != "review.status" {
		t.Fatalf("moved-candidate next_action = %v, want review.status\n%s", failure["next_action"], stdout.String())
	}
	if failure["retry_safe"] != true {
		t.Fatalf("moved-candidate retry_safe = %v, want true\n%s", failure["retry_safe"], stdout.String())
	}
	cause, _ := failure["cause"].(string)
	if !strings.Contains(cause, "moved between negotiation and answer") {
		t.Fatalf("moved-candidate cause does not name the concurrent movement: %q", cause)
	}
	contextMap, _ := failure["context"].(map[string]any)
	if contextMap == nil {
		t.Fatalf("moved-candidate failure carries no context: %s", stdout.String())
	}
	drift, _ := contextMap["target_drift"].(map[string]any)
	if drift == nil {
		t.Fatalf("moved-candidate context carries no target_drift: %s", stdout.String())
	}
	expected, _ := drift["expected"].(map[string]any)
	actual, _ := drift["actual"].(map[string]any)
	if expected == nil || actual == nil {
		t.Fatalf("target_drift carries no expected/actual components: %s", stdout.String())
	}
	parts := strings.SplitN(token, ":", 6)
	if len(parts) != 6 {
		t.Fatalf("evidence token %q is not v1:kind:projection:base:candidate:digest", token)
	}
	if expected["candidate_tree"] != parts[4] {
		t.Fatalf("expected candidate_tree = %v, want negotiated %q", expected["candidate_tree"], parts[4])
	}
	if actual["candidate_tree"] == expected["candidate_tree"] {
		t.Fatalf("actual candidate_tree equals the negotiated one; the concurrent write is missing")
	}
	differing, _ := drift["differing_components"].([]any)
	foundCandidate := false
	for _, name := range differing {
		if name == "candidate_tree" {
			foundCandidate = true
		}
	}
	if !foundCandidate {
		t.Fatalf("differing_components = %v, want it to name candidate_tree", differing)
	}
}

// TestReviewStaleWithoutTargetEvidenceKeepsLegacyEnvelope guards the shipped
// failure bytes: a stale negotiated START whose continuation carries no
// evidence token must fail exactly as today, with no context and no new cause.
func TestReviewStaleWithoutTargetEvidenceKeepsLegacyEnvelope(t *testing.T) {
	repo, question := reviewTargetEvidenceRelaySetup(t)
	granted := testStripFlagFromInvocation(t, question.Choices[0].Invocation, "--target-evidence")
	writeReviewStartCandidate(t, repo, "scripts/second.sh", "echo more\n", 0o755)

	var stdout bytes.Buffer
	err := RunReview(invocationArgs(t, granted), &stdout)
	if err == nil {
		t.Fatalf("stale answer without evidence must still fail: %s", stdout.String())
	}
	failure := decodeReviewFailureMap(t, stdout.Bytes())
	if failure["code"] != reviewPreflightStaleTargetCode {
		t.Fatalf("legacy stale code = %v, want %q", failure["code"], reviewPreflightStaleTargetCode)
	}
	if failure["cause"] != "review start target does not match the freshly built snapshot" {
		t.Fatalf("legacy stale cause = %v, want the shipped cause", failure["cause"])
	}
	if _, has := failure["context"]; has {
		t.Fatalf("legacy stale failure must not carry context: %s", stdout.String())
	}
}

// TestReviewStartRejectsTargetEvidenceIdentityMismatch proves the evidence
// token binds to the negotiated identity: a token that does not hash to
// --target is a caller request defect, classified invalid_request before any
// staleness classification.
func TestReviewStartRejectsTargetEvidenceIdentityMismatch(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o755)
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	token := formatReviewTargetEvidence(snapshot)
	tampered := "sha256:" + strings.Repeat("b", 64)

	args := []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "review-target-evidence", "--target", tampered,
		"--projection", "workspace", "--target-evidence", token, "--consent", "relay",
	}
	var stdout bytes.Buffer
	if err := RunReview(args, &stdout); err == nil {
		t.Fatalf("mismatched evidence token must fail: %s", stdout.String())
	}
	failure := decodeReviewFailureMap(t, stdout.Bytes())
	if failure["code"] != reviewIntegrationInvalidRequestCode {
		t.Fatalf("mismatched evidence code = %v, want %q\n%s", failure["code"], reviewIntegrationInvalidRequestCode, stdout.String())
	}
	if cause, _ := failure["cause"].(string); !strings.Contains(cause, "--target-evidence") {
		t.Fatalf("mismatched evidence cause must name the token: %q", cause)
	}
}

// TestReviewStartRejectsMalformedTargetEvidence proves a garbage token is a
// request defect, never a staleness classification.
func TestReviewStartRejectsMalformedTargetEvidence(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o755)
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	args := []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "review-target-evidence", "--target", snapshot.Identity,
		"--projection", "workspace", "--target-evidence", "garbage", "--consent", "relay",
	}
	var stdout bytes.Buffer
	if err := RunReview(args, &stdout); err == nil {
		t.Fatalf("malformed evidence token must fail: %s", stdout.String())
	}
	failure := decodeReviewFailureMap(t, stdout.Bytes())
	if failure["code"] != reviewIntegrationInvalidRequestCode {
		t.Fatalf("malformed evidence code = %v, want %q\n%s", failure["code"], reviewIntegrationInvalidRequestCode, stdout.String())
	}
}

// TestReviewNegotiatedStartCommandCarriesTargetEvidence proves both
// continuation factories publish the evidence token beside the identity hash.
func TestReviewNegotiatedStartCommandCarriesTargetEvidence(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o755)
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := reviewNegotiatedStartCommand(snapshot, "claude-code")
	index := strings.Index(command, " --target-evidence ")
	if index < 0 {
		t.Fatalf("negotiated START command carries no evidence token: %q", command)
	}
	rest := command[index+len(" --target-evidence "):]
	if end := strings.Index(rest, " "); end >= 0 {
		rest = rest[:end]
	}
	token, err := parseReviewTargetEvidenceToken(rest)
	if err != nil {
		t.Fatalf("parse negotiated command evidence token %q: %v", rest, err)
	}
	if token.Kind != string(snapshot.Kind) || token.Projection != string(facadeProjection(snapshot.Projection)) ||
		token.BaseTree != snapshot.BaseTree || token.CandidateTree != snapshot.CandidateTree ||
		token.PathsDigest != snapshot.PathsDigest {
		t.Fatalf("evidence token %+v does not describe the snapshot %+v", token, snapshot)
	}
	if recomputed := token.identity(); recomputed != snapshot.Identity {
		t.Fatalf("evidence token identity %q does not recompute the negotiated identity %q", recomputed, snapshot.Identity)
	}
}

// TestReviewTargetEvidenceTokenRoundTrip pins the token format at the unit
// boundary: colon-free components, deterministic formatting, and identity
// recomputation from components alone.
func TestReviewTargetEvidenceTokenRoundTrip(t *testing.T) {
	snapshot := reviewtransaction.Snapshot{
		Kind:          reviewtransaction.TargetCurrentChanges,
		Projection:    reviewtransaction.ProjectionWorkspace,
		BaseTree:      strings.Repeat("a", 40),
		CandidateTree: strings.Repeat("b", 40),
		PathsDigest:   "sha256:" + strings.Repeat("c", 64),
		Identity:      "sha256:" + strings.Repeat("d", 64),
	}
	token := formatReviewTargetEvidence(snapshot)
	if !strings.HasPrefix(token, "v1:") {
		t.Fatalf("token %q is not versioned", token)
	}
	if strings.Contains(strings.TrimPrefix(token, "v1:"), "::") {
		t.Fatalf("token %q carries an empty component; format must stay explicit", token)
	}
	parsed, err := parseReviewTargetEvidenceToken(token)
	if err != nil {
		t.Fatalf("parse %q: %v", token, err)
	}
	if parsed.Kind != string(snapshot.Kind) || parsed.Projection != string(snapshot.Projection) ||
		parsed.BaseTree != snapshot.BaseTree || parsed.CandidateTree != snapshot.CandidateTree ||
		parsed.PathsDigest != snapshot.PathsDigest {
		t.Fatalf("round trip mismatch: %+v vs %+v", parsed, snapshot)
	}
	if _, err := parseReviewTargetEvidenceToken("v1:wrong"); err == nil {
		t.Fatal("expected a malformed token to be rejected")
	}
}

// TestReviewTargetEvidenceDifferences pins the decomposition unit: the
// differing component names that become the truthful cause.
func TestReviewTargetEvidenceDifferences(t *testing.T) {
	negotiated := reviewTargetEvidence{
		Kind: "current-changes", Projection: "workspace",
		BaseTree: strings.Repeat("a", 40), CandidateTree: strings.Repeat("b", 40), PathsDigest: strings.Repeat("c", 64),
	}
	if diffs := reviewTargetEvidenceDifferences(negotiated, negotiated); len(diffs) != 0 {
		t.Fatalf("identical evidence differs: %v", diffs)
	}
	moved := negotiated
	moved.CandidateTree = strings.Repeat("e", 40)
	moved.PathsDigest = strings.Repeat("f", 64)
	diffs := reviewTargetEvidenceDifferences(negotiated, moved)
	if len(diffs) != 2 || diffs[0] != "candidate_tree" || diffs[1] != "paths_digest" {
		t.Fatalf("moved differences = %v, want [candidate_tree paths_digest]", diffs)
	}
	rebased := negotiated
	rebased.BaseTree = strings.Repeat("1", 40)
	diffs = reviewTargetEvidenceDifferences(negotiated, rebased)
	if len(diffs) != 1 || diffs[0] != "base_tree" {
		t.Fatalf("rebased differences = %v, want [base_tree]", diffs)
	}
}

// TestReviewConsentStaleMarkerRecordsMovedConsentRefusal proves a spent human
// answer on a moved candidate records the per-repository consent-stale marker
// the stop hook reads to stay quiet while the writer advances.
func TestReviewConsentStaleMarkerRecordsMovedConsentRefusal(t *testing.T) {
	repo, question := reviewTargetEvidenceRelaySetup(t)
	granted := question.Choices[0].Invocation
	writeReviewStartCandidate(t, repo, "scripts/second.sh", "echo more\n", 0o755)

	var stdout bytes.Buffer
	if err := RunReview(invocationArgs(t, granted), &stdout); err == nil {
		t.Fatalf("granted answer on a moved candidate must fail: %s", stdout.String())
	}
	marker, ok, err := readReviewConsentStaleMarker(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("a spent consent answer on a moved candidate must record the consent-stale marker")
	}
	if marker.Schema != reviewConsentStaleMarkerSchema {
		t.Fatalf("marker schema = %q, want %q", marker.Schema, reviewConsentStaleMarkerSchema)
	}
	if marker.RefusedTargetIdentity != question.TargetIdentity {
		t.Fatalf("marker refused identity = %q, want the negotiated %q", marker.RefusedTargetIdentity, question.TargetIdentity)
	}
	if marker.LastRefusedAt.IsZero() {
		t.Fatal("marker must carry a refusal timestamp")
	}
}

// TestReviewConsentStaleMarkerSkipsNonConsentStale proves the marker records
// only spent human answers: a negotiated START that goes stale without any
// consent answer must not silence the stop hook.
func TestReviewConsentStaleMarkerSkipsNonConsentStale(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o755)
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	token := formatReviewTargetEvidence(snapshot)
	writeReviewStartCandidate(t, repo, "scripts/second.sh", "echo more\n", 0o755)

	args := []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "review-target-evidence", "--target", snapshot.Identity,
		"--projection", "workspace", "--target-evidence", token,
	}
	var stdout bytes.Buffer
	if err := RunReview(args, &stdout); err == nil {
		t.Fatalf("stale negotiated START without consent must fail: %s", stdout.String())
	}
	if _, ok, err := readReviewConsentStaleMarker(repo); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("a stale refusal without a consent answer must not record the marker")
	}
}

// TestReviewNegotiatedStaleIdentityOnlyDriftStopsInsteadOfRetrying is the
// quiet-workspace triangle of issue #4494 (the #2700/#4353/#4412 class): when
// every evidence component still matches the live workspace but the identity
// hash alone drifted, the refusal must stop the caller instead of advising
// another re-derivation, and it must never record the consent-stale marker
// (nothing moved; the stop hook must keep working).
func TestReviewNegotiatedStaleIdentityOnlyDriftStopsInsteadOfRetrying(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o755)
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	// The negotiated answer binds the honest identity recomputed from the
	// frozen components; the live rebuild's own Identity field drifted from
	// those components, which is the derivation defect this branch exists for.
	honest := reviewtransaction.IdentityForComponents(
		snapshot.Kind, facadeProjection(snapshot.Projection), snapshot.BaseTree, snapshot.CandidateTree, snapshot.PathsDigest)
	corrupted := snapshot
	corrupted.Identity = "sha256:" + strings.Repeat("9", 64)
	token := formatReviewTargetEvidence(corrupted)

	err = reviewNegotiatedStaleTargetRefusal(honest, token, corrupted, reviewConsentModeGranted, repo)
	if err == nil {
		t.Fatal("an identity-only drift must refuse")
	}
	var preflight *reviewIntegrationPreflightError
	if !errors.As(err, &preflight) {
		t.Fatalf("identity-only drift is not a typed preflight refusal: %v", err)
	}
	reason := preflight.classification()
	if reason.Code != reviewPreflightStaleTargetCode || reason.NextAction != "stop" || !reason.RetrySafeFalse {
		t.Fatalf("identity-only classification = %+v, want stale code, next_action stop, retry refused", reason)
	}
	if cause := err.Error(); !strings.Contains(cause, "derivation defect") {
		t.Fatalf("identity-only cause = %q, want it to name the derivation defect", cause)
	}
	if _, ok, err := readReviewConsentStaleMarker(repo); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("an identity-only drift must not record the consent-stale marker")
	}
}
