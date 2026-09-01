package reviewtransaction

// Wave 5 (Gate Cutover) Slice 6, design decision 6: candidate decline no
// longer authorizes delivery at any gate, and nothing is ever durably
// recorded (RecordCandidateDecline, the writer, and ResolveCandidateDeclineForGate,
// the gate-side resolver, are both deleted -- their only production callers
// were internal/cli/review_facade.go's consent-decline branch and the
// funnel's own decline check, both removed in the same slice). Consistent
// with Wave 4 decision 4 ("decline = unmanaged proceed: nothing recorded"),
// an inert authority-shaped file is exactly the mirror pattern that
// decision already removed on the SDD side.
//
// parseCandidateDeclineAuthorization (and its own dependencies:
// canonicalCandidateDeclinePayload, candidateDeclineDigest, and
// CandidateDeclineAuthorization.validate) survive READ-ONLY: pre-existing
// decline records written before this slice may still be present on disk
// (Wave 7 deletes the files), and the ability to parse and structurally
// validate one — never to write, resolve, or authorize from one — is a
// forensic guarantee this package keeps until then. No production caller
// exists yet (no gate or facade command reads a historical decline record
// today), so this parser is exercised only by
// candidate_decline_parser_test.go's direct round-trip coverage; it is
// intentionally baselined in the deadcode ratchet rather than deleted,
// mirroring Wave 5 Slice 5's AssessTargetStatus precedent (a legitimate,
// forensic-facing primitive whose sole production caller was removed, kept
// rather than deleted for a disproportionate reason).

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	// CandidateDeclineAuthorizationSchema is the schema every pre-existing
	// (pre-Slice-6) decline record on disk carries.
	CandidateDeclineAuthorizationSchema = "gentle-ai.rdd-candidate-decline/v1"
	candidateDeclineDigestDomain        = "gentle-ai.rdd-candidate-decline-digest/v1"
)

// ErrCandidateDeclineCorrupt rejects a candidate-scoped decline whose durable
// bytes cannot prove the exact snapshot the operator declined to review.
// refusal:by-design world-action: corrupted private authority bytes must be repaired or removed before they can govern delivery
var ErrCandidateDeclineCorrupt = errors.New("candidate decline authorization is corrupt")

// CandidateDeclineAuthorization is a durable, content-addressed opt-out for
// one frozen candidate, as written by pre-Slice-6 code. It is intentionally
// not review authority: it contains no lineage, receipt, verdict, or
// approval. No production code writes one anymore; this type exists so a
// pre-existing record can still be parsed.
type CandidateDeclineAuthorization struct {
	Schema           string   `json:"schema"`
	Snapshot         Snapshot `json:"snapshot"`
	AuthorizationRef string   `json:"authorization_ref"`
}

func (authorization CandidateDeclineAuthorization) validate() error {
	if authorization.Schema != CandidateDeclineAuthorizationSchema {
		// refusal:by-design world-action: an unknown durable schema cannot safely authorize delivery
		return errors.New("invalid candidate decline authorization schema")
	}
	if err := validateCompactSnapshot(authorization.Snapshot); err != nil {
		return fmt.Errorf("invalid candidate decline snapshot: %w", err)
	}
	if !validSHA256(authorization.AuthorizationRef) {
		// refusal:by-design world-action: an invalid content address cannot bind durable authorization bytes
		return errors.New("invalid candidate decline authorization ref")
	}
	want, err := candidateDeclineDigest(authorization)
	if err != nil {
		return err
	}
	if authorization.AuthorizationRef != want {
		// refusal:by-design world-action: modified durable bytes cannot retain authority from their previous content address
		return errors.New("candidate decline authorization ref does not match canonical content")
	}
	return nil
}

func candidateDeclineDigest(authorization CandidateDeclineAuthorization) (string, error) {
	authorization.AuthorizationRef = ""
	payload, err := json.Marshal(authorization)
	if err != nil {
		return "", err
	}
	sum := sha256.New()
	_, _ = sum.Write([]byte(candidateDeclineDigestDomain))
	_, _ = sum.Write([]byte{0})
	_, _ = sum.Write(payload)
	return "sha256:" + hex.EncodeToString(sum.Sum(nil)), nil
}

func canonicalCandidateDeclinePayload(authorization CandidateDeclineAuthorization) ([]byte, error) {
	if err := authorization.validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(authorization)
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

// parseCandidateDeclineAuthorization structurally validates and canonically
// re-verifies a pre-existing decline record's bytes. It is read-only: it
// never writes, resolves a gate against, or authorizes delivery from the
// result.
func parseCandidateDeclineAuthorization(payload []byte) (CandidateDeclineAuthorization, error) {
	var authorization CandidateDeclineAuthorization
	if err := decodeStrictRARJSON(payload, &authorization); err != nil {
		return CandidateDeclineAuthorization{}, err
	}
	if err := authorization.validate(); err != nil {
		return CandidateDeclineAuthorization{}, err
	}
	canonical, err := canonicalCandidateDeclinePayload(authorization)
	if err != nil || !bytes.Equal(payload, canonical) {
		// refusal:by-design world-action: non-canonical durable bytes must be repaired or removed before delivery
		return CandidateDeclineAuthorization{}, errors.New("candidate decline authorization is not canonical")
	}
	return authorization, nil
}
