package reviewtransaction

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// LensContextEmissionSchema identifies one durable record that the provider
	// itself produced the reviewer lens context for one selected lens slot.
	LensContextEmissionSchema = "gentle-ai.review-lens-context-emission/v1"
	// LensContextEmissionDir sits beside the captured reviewer results in the
	// same authority store, and is append-only for exactly the same reason: it
	// is audit history, not authority. Publishing here costs no authority
	// revision, so emitting a lens context never invalidates the revision the
	// caller is still holding for its capture.
	LensContextEmissionDir = "lens-contexts"

	lensContextEmissionMaxBytes = 8 << 10
)

// ErrLensContextEmissionConflict reports that this exact lens slot already
// recorded a different reviewer context emission.
// refusal:by-design world-action: this is a sentinel identity, not an operator-facing message; the CLI translates it into the refusal that names the exit
var ErrLensContextEmissionConflict = errors.New("reviewer lens context emission already exists with different bytes")

// LensContextEmission records that the provider produced the immutable
// candidate evidence for one lens slot, and by which mechanism. It is bound to
// the artifact subject, so a record can never be read against a different
// candidate, revision, or lens position than the one it was written for.
type LensContextEmission struct {
	Schema            string               `json:"schema"`
	LineageID         string               `json:"lineage_id"`
	TargetIdentity    string               `json:"target_identity"`
	AuthorityRevision string               `json:"authority_revision"`
	Lens              string               `json:"lens"`
	SelectedOrder     int                  `json:"selected_order"`
	SubjectHash       string               `json:"subject_hash"`
	Level             ReviewerContextLevel `json:"level"`
}

// Validate proves the record is structurally complete. Level is checked for
// shape only, never for membership: a record written by a later release naming
// a mechanism this binary has never heard of stays readable.
func (emission LensContextEmission) Validate() error {
	if emission.Schema != LensContextEmissionSchema || validateLineageID(emission.LineageID) != nil ||
		!validSHA256(emission.TargetIdentity) || !validSHA256(emission.AuthorityRevision) ||
		emission.Lens == "" || emission.SelectedOrder < 0 || !validSHA256(emission.SubjectHash) ||
		!ReviewerContextLevelWellFormed(emission.Level) {
		return errors.New("reviewer lens context emission is invalid") // refusal:by-design world-action: a malformed record in provider-owned audit storage requires storage repair, not an operator command
	}
	return nil
}

// PublishLensContextEmission durably records one provider-produced reviewer
// lens context. Re-emitting the identical context for the same slot converges;
// recording a different mechanism for the same frozen slot is a conflict rather
// than an overwrite, because audit history that can be rewritten records
// nothing.
func PublishLensContextEmission(storeDir string, emission LensContextEmission) error {
	if err := emission.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(emission)
	if err != nil {
		return err
	}
	path, err := lensContextEmissionPath(storeDir, emission.AuthorityRevision, emission.Lens, emission.SelectedOrder)
	if err != nil {
		return err
	}
	if err := publishImmutable(path, append(payload, '\n'), 0o600); err != nil {
		var conflict *ImmutablePublicationConflictError
		if errors.As(err, &conflict) {
			return ErrLensContextEmissionConflict
		}
		return err
	}
	return nil
}

// ReadLensContextEmission returns the recorded emission for one lens slot, or
// false when nothing recorded it. A record that does not bind this exact
// lineage, target, revision, lens, order, and subject is treated as absent
// rather than as evidence: it describes a different candidate.
func ReadLensContextEmission(storeDir, lineageID, targetIdentity, revision, lens string, order int, subjectHash string) (LensContextEmission, bool) {
	path, err := lensContextEmissionPath(storeDir, revision, lens, order)
	if err != nil {
		return LensContextEmission{}, false
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		legacy, legacyErr := lensContextEmissionLegacyPath(storeDir, lens, order)
		if legacyErr != nil {
			return LensContextEmission{}, false
		}
		file, err = os.Open(legacy)
	}
	if err != nil {
		return LensContextEmission{}, false
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, lensContextEmissionMaxBytes+1))
	if err != nil || len(payload) > lensContextEmissionMaxBytes {
		return LensContextEmission{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var emission LensContextEmission
	if err := decoder.Decode(&emission); err != nil {
		return LensContextEmission{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return LensContextEmission{}, false
	}
	if emission.Validate() != nil || emission.LineageID != lineageID || emission.TargetIdentity != targetIdentity ||
		emission.AuthorityRevision != revision || emission.Lens != lens || emission.SelectedOrder != order ||
		emission.SubjectHash != subjectHash {
		return LensContextEmission{}, false
	}
	return emission, true
}

// DiscoverReviewerContextLevel resolves the single level to record on a
// receipt from the per-lens emissions.
//
// It records a level only when every selected lens has a bound emission and
// they all name the same mechanism. Anything else — a lens whose context was
// assembled some other way, a review with no lenses at all, or a mixture —
// records nothing. Absence therefore always means "not established", never a
// guess and never the weakest mechanism, which is what keeps the field
// meaningful without anyone gating on it.
func DiscoverReviewerContextLevel(storeDir, lineageID, targetIdentity, revision string, lenses []string, subjectHashes []string) ReviewerContextLevel {
	if len(lenses) == 0 || len(lenses) != len(subjectHashes) {
		return ""
	}
	var agreed ReviewerContextLevel
	for order, lens := range lenses {
		emission, found := ReadLensContextEmission(storeDir, lineageID, targetIdentity, revision, lens, order, subjectHashes[order])
		if !found || (order > 0 && emission.Level != agreed) {
			return ""
		}
		agreed = emission.Level
	}
	return agreed
}

// lensContextEmissionPath resolves the immutable file one lens slot's emission
// is published to.
//
// The authority revision is part of the key because it is part of the record's
// identity: ReadLensContextEmission already treats a record naming a different
// revision as absent ("it describes a different candidate"), so a path keyed by
// lens and order alone contradicted the reader. A maintainer-authorized
// reopen-results moves the same lineage back to `reviewing` under a new
// revision, and that legitimately-new record used to collide with the old one
// at the shared path — permanently, because publication is immutable (issue
// #2850). Scoping the path restores the invariant the refusal claims: audit
// history is never rewritten, and now it is never blocked either.
func lensContextEmissionPath(storeDir, revision, lens string, order int) (string, error) {
	if storeDir == "" || order < 0 || lens == "" || lens != filepath.Base(lens) ||
		lens == "." || lens == ".." || !validCanonicalLensName(lens) || !validSHA256(revision) {
		return "", errors.New("reviewer lens context emission slot is invalid") // refusal:by-design world-action: reaching this means provider code bypassed the validated native CLI contract, which is a code fix rather than an operator command
	}
	return filepath.Join(storeDir, LensContextEmissionDir, strings.TrimPrefix(revision, "sha256:"),
		fmt.Sprintf("%02d-%s.json", order, lens)), nil
}

// lensContextEmissionLegacyPath is the pre-#2850 flat location. It is read
// from, never written to: a store recorded by an older binary keeps answering
// for the revision it was written under, and the binding check in
// ReadLensContextEmission still rejects it for every other revision.
func lensContextEmissionLegacyPath(storeDir, lens string, order int) (string, error) {
	if storeDir == "" || order < 0 || lens == "" || lens != filepath.Base(lens) ||
		lens == "." || lens == ".." || !validCanonicalLensName(lens) {
		return "", errors.New("reviewer lens context emission slot is invalid") // refusal:by-design world-action: reaching this means provider code bypassed the validated native CLI contract, which is a code fix rather than an operator command
	}
	return filepath.Join(storeDir, LensContextEmissionDir, fmt.Sprintf("%02d-%s.json", order, lens)), nil
}

func validCanonicalLensName(lens string) bool {
	for _, supported := range supportedLenses {
		if lens == supported {
			return true
		}
	}
	return false
}
