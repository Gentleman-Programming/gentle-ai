package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

// Issue #4494: the consent answer re-enters negotiated START, which rebuilds
// the live workspace snapshot and refuses a moved candidate with an opaque
// identity mismatch whose recovery advice re-derives into an unbounded loop
// while a concurrent writer advances. The negotiated continuation therefore
// carries the candidate evidence components beside the identity hash, so the
// failing process can decompose the mismatch into a truthful cause and branch
// next_action between wait-and-rederive and stop.

// reviewTargetEvidenceTokenVersion guards the token layout below. The token is
// colon-joined ASCII: kind and projection are hyphenated enums, both trees are
// bare hex, and paths_digest is the one component that itself contains a
// colon (its sha256: prefix), which is why parsing splits at most six fields.
const reviewTargetEvidenceTokenVersion = "v1"

// reviewTargetEvidence carries the five components the negotiated identity
// hash is derived from (snapshotIdentityForProjection), so a later process can
// compare them against a live rebuild without any persisted negotiation state.
type reviewTargetEvidence struct {
	Kind          string
	Projection    string
	BaseTree      string
	CandidateTree string
	PathsDigest   string
}

var (
	reviewTargetEvidenceTreePattern   = regexp.MustCompile(`^[0-9a-f]{40}(?:[0-9a-f]{24})?$`)
	reviewTargetEvidenceDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// formatReviewTargetEvidence renders the self-describing evidence token for
// one negotiated snapshot. The projection is normalized the same way the
// continuation command spells it, so the token always carries an explicit
// canonical projection.
func formatReviewTargetEvidence(snapshot reviewtransaction.Snapshot) string {
	return strings.Join([]string{
		reviewTargetEvidenceTokenVersion,
		string(snapshot.Kind),
		string(facadeProjection(snapshot.Projection)),
		snapshot.BaseTree,
		snapshot.CandidateTree,
		snapshot.PathsDigest,
	}, ":")
}

// parseReviewTargetEvidenceToken decodes one evidence token. A malformed token
// is a caller request defect, never a staleness classification.
func parseReviewTargetEvidenceToken(token string) (reviewTargetEvidence, error) {
	fields := strings.SplitN(strings.TrimSpace(token), ":", 6)
	if len(fields) != 6 || fields[0] != reviewTargetEvidenceTokenVersion {
		return reviewTargetEvidence{}, fmt.Errorf("--target-evidence must be %s:kind:projection:base_tree:candidate_tree:paths_digest", reviewTargetEvidenceTokenVersion)
	}
	evidence := reviewTargetEvidence{
		Kind: fields[1], Projection: fields[2], BaseTree: fields[3], CandidateTree: fields[4], PathsDigest: fields[5],
	}
	if evidence.Kind == "" || evidence.Projection == "" {
		return reviewTargetEvidence{}, errors.New("--target-evidence carries an empty kind or projection")
	}
	if evidence.BaseTree != "" && !reviewTargetEvidenceTreePattern.MatchString(evidence.BaseTree) {
		return reviewTargetEvidence{}, errors.New("--target-evidence base_tree is not a Git tree hash")
	}
	if !reviewTargetEvidenceTreePattern.MatchString(evidence.CandidateTree) {
		return reviewTargetEvidence{}, errors.New("--target-evidence candidate_tree is not a Git tree hash")
	}
	if !reviewTargetEvidenceDigestPattern.MatchString(evidence.PathsDigest) {
		return reviewTargetEvidence{}, errors.New("--target-evidence paths_digest is not a sha256 digest")
	}
	return evidence, nil
}

// identity recomputes the content-addressed snapshot identity from the token
// components alone. proof, intended untracked, and ledger IDs never enter the
// identity hash (maintainer decision D1 on #2471), so no negotiation state is
// needed to verify that the token and the --target identity belong to the
// same negotiation.
func (e reviewTargetEvidence) identity() string {
	return reviewtransaction.IdentityForComponents(
		reviewtransaction.TargetKind(e.Kind), reviewtransaction.Projection(e.Projection),
		e.BaseTree, e.CandidateTree, e.PathsDigest)
}

// reviewTargetEvidenceDifferences names the components that differ between
// the negotiated evidence and the live rebuild, in a stable component order.
// An empty result means every component matches and an identity mismatch can
// only be a derivation defect, never workspace movement.
func reviewTargetEvidenceDifferences(expected, actual reviewTargetEvidence) []string {
	type component struct {
		name             string
		expected, actual string
	}
	components := []component{
		{"kind", expected.Kind, actual.Kind},
		{"projection", expected.Projection, actual.Projection},
		{"base_tree", expected.BaseTree, actual.BaseTree},
		{"candidate_tree", expected.CandidateTree, actual.CandidateTree},
		{"paths_digest", expected.PathsDigest, actual.PathsDigest},
	}
	var differing []string
	for _, entry := range components {
		if entry.expected != entry.actual {
			differing = append(differing, entry.name)
		}
	}
	return differing
}

// reviewConsentStaleMarkerSchema identifies the small per-repository record
// written when a relayed consent answer is spent on a moved candidate.
const reviewConsentStaleMarkerSchema = "gentle-ai.review-consent-stale-marker/v1"

// reviewConsentStaleMarkerWindow bounds how long the stop hook stays quiet
// after a consent answer was refused as stale because the candidate moved.
// The window dampens the #4494 loop to at most one renegotiation attempt per
// window instead of one per candidate change, without ever suppressing the
// reminder indefinitely.
const reviewConsentStaleMarkerWindow = 15 * time.Minute

// reviewConsentStaleMarker is the per-repository record the START refusal
// writes and the stop hook reads. It lives under the user's home directory,
// never inside the reviewed repository.
type reviewConsentStaleMarker struct {
	Schema                string    `json:"schema"`
	RefusedTargetIdentity string    `json:"refused_target_identity"`
	FirstRefusedAt        time.Time `json:"first_refused_at"`
	LastRefusedAt         time.Time `json:"last_refused_at"`
}

// reviewConsentStaleMarkerPath is the per-repository marker file path. The
// key canonicalizes the repository root so the START refusal and the stop
// hook agree on one file even when the caller spells the cwd differently.
func reviewConsentStaleMarkerPath(home, repo string) string {
	canonical, err := filepath.Abs(repo)
	if err == nil {
		if resolved, evalErr := filepath.EvalSymlinks(canonical); evalErr == nil {
			canonical = resolved
		}
	}
	digest := sha256.Sum256([]byte(filepath.Clean(canonical)))
	return filepath.Join(home, ".gentle-ai", "review-stop-hook", "v1", "consent-stale", hex.EncodeToString(digest[:])+".json")
}

// recordReviewConsentStaleRefusal records that a spent consent answer was
// refused because the negotiated candidate moved. Best-effort by design: the
// refusal itself is the operator-facing outcome, and a failed marker write
// fails open to today's reminder behavior.
func recordReviewConsentStaleRefusal(repo, targetIdentity string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := reviewConsentStaleMarkerPath(home, repo)
	now := time.Now()
	record := reviewConsentStaleMarker{Schema: reviewConsentStaleMarkerSchema, RefusedTargetIdentity: targetIdentity, FirstRefusedAt: now, LastRefusedAt: now}
	if existing, ok, readErr := readReviewConsentStaleMarker(repo); readErr == nil && ok {
		record.FirstRefusedAt = existing.FirstRefusedAt
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o600)
}

// readReviewConsentStaleMarker loads the per-repository marker. ok is false
// when no marker exists; a corrupt marker reads as absent so a damaged state
// file can never permanently silence the stop hook.
func readReviewConsentStaleMarker(repo string) (reviewConsentStaleMarker, bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return reviewConsentStaleMarker{}, false, err
	}
	payload, err := os.ReadFile(reviewConsentStaleMarkerPath(home, repo))
	if err != nil {
		if os.IsNotExist(err) {
			return reviewConsentStaleMarker{}, false, nil
		}
		return reviewConsentStaleMarker{}, false, err
	}
	var record reviewConsentStaleMarker
	if err := json.Unmarshal(payload, &record); err != nil {
		return reviewConsentStaleMarker{}, false, nil
	}
	if record.Schema != reviewConsentStaleMarkerSchema {
		return reviewConsentStaleMarker{}, false, nil
	}
	return record, true, nil
}

// reviewNegotiatedStaleTargetRefusal classifies one negotiated stale-target
// mismatch. Without an evidence token it reproduces the shipped refusal
// bytes exactly. With a token, the mismatch decomposes into a truthful
// cause: the moved class names the differing components and waits for the
// writer, while an all-components-equal mismatch is a derivation defect that
// must stop instead of retrying (#4494, #2700, #4353, #4412).
func reviewNegotiatedStaleTargetRefusal(targetIdentity, evidenceToken string, snapshot reviewtransaction.Snapshot, consentMode reviewStartConsentMode, repo string) error {
	if strings.TrimSpace(evidenceToken) == "" {
		return reviewPreflightRefusal(reviewPreflightStaleTargetReason,
			errors.New("review start target does not match the freshly built snapshot"))
	}
	token, err := parseReviewTargetEvidenceToken(evidenceToken)
	if err != nil || token.identity() != targetIdentity {
		// The negotiated binding validation already rejects malformed or
		// mismatched tokens; this defensive branch keeps the shipped refusal
		// for any caller that still reaches staleness with an unusable token.
		return reviewPreflightRefusal(reviewPreflightStaleTargetReason,
			errors.New("review start target does not match the freshly built snapshot"))
	}
	live := reviewTargetEvidence{
		Kind: string(snapshot.Kind), Projection: string(facadeProjection(snapshot.Projection)),
		BaseTree: snapshot.BaseTree, CandidateTree: snapshot.CandidateTree, PathsDigest: snapshot.PathsDigest,
	}
	diffs := reviewTargetEvidenceDifferences(token, live)
	if len(diffs) == 0 {
		reason := reviewPreflightStaleTargetReason
		reason.NextAction = "stop"
		reason.RetrySafeFalse = true
		return reviewPreflightRefusal(reason, errors.New("the negotiated identity hash drifted while every evidence component matches the live workspace; this is a review derivation defect, do not retry the same request; report it"))
	}
	reason := reviewPreflightStaleTargetReason
	reason.Context = &ReviewIntegrationFailureContext{TargetDrift: &ReviewIntegrationTargetDrift{
		Expected: negotiatedEvidenceComponents(token), Actual: negotiatedEvidenceComponents(live), DifferingComponents: diffs,
	}}
	if consentMode == reviewConsentModeGranted || consentMode == reviewConsentModeDeclined {
		_ = recordReviewConsentStaleRefusal(repo, targetIdentity)
	}
	return reviewPreflightRefusal(reason, fmt.Errorf("the negotiated candidate moved between negotiation and answer: %s changed; wait for concurrent writes to settle, then re-derive the next transition with review.status before spending another consent answer", strings.Join(diffs, ", ")))
}

// negotiatedEvidenceComponents projects one evidence value onto its
// schema-bounded JSON shape.
func negotiatedEvidenceComponents(evidence reviewTargetEvidence) ReviewIntegrationNegotiatedComponents {
	return ReviewIntegrationNegotiatedComponents{
		Kind: evidence.Kind, Projection: evidence.Projection, BaseTree: evidence.BaseTree,
		CandidateTree: evidence.CandidateTree, PathsDigest: evidence.PathsDigest,
	}
}

// reviewTargetDriftComponentNames is the closed component vocabulary the
// target_drift failure context may name.
var reviewTargetDriftComponentNames = map[string]bool{
	"kind": true, "projection": true, "base_tree": true, "candidate_tree": true, "paths_digest": true,
}

// validReviewTargetDriftComponents proves one component record is complete
// enough to publish: kind and projection are required, base_tree is optional
// (an unborn HEAD has none), and the tree/digest hashes must be well formed.
func validReviewTargetDriftComponents(components ReviewIntegrationNegotiatedComponents) bool {
	if components.Kind == "" || components.Projection == "" {
		return false
	}
	if components.BaseTree != "" && !reviewTargetEvidenceTreePattern.MatchString(components.BaseTree) {
		return false
	}
	return reviewTargetEvidenceTreePattern.MatchString(components.CandidateTree) &&
		reviewTargetEvidenceDigestPattern.MatchString(components.PathsDigest)
}
