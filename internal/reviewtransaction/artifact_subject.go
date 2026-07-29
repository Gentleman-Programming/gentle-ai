package reviewtransaction

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
)

const ArtifactSubjectSchema = "gentle-ai.review-artifact-subject/v1"

// ArtifactSubject is the provider-owned identity of one reviewer execution.
// It binds the exact authority revision, immutable candidate bytes and path
// manifest, selected lens slot, and (when present) correction target.
type ArtifactSubject struct {
	Schema                    string `json:"schema"`
	SubjectHash               string `json:"subject_hash"`
	LineageID                 string `json:"lineage_id"`
	AuthorityRevision         string `json:"authority_revision"`
	TargetIdentity            string `json:"target_identity"`
	CandidateDiffSHA256       string `json:"candidate_diff_sha256"`
	ChangedPathManifestSHA256 string `json:"changed_path_manifest_sha256"`
	Lens                      string `json:"lens"`
	SelectedOrder             int    `json:"selected_order"`
	CorrectionTargetIdentity  string `json:"correction_target_identity,omitempty"`
}

// ChangedPathManifestDigest returns the canonical provider-owned identity of
// an ordered immutable changed-path manifest.
func ChangedPathManifestDigest(entries []ChangedPathManifestEntry) (string, error) {
	if err := ValidateChangedPathManifest(entries); err != nil {
		return "", err
	}
	payload, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte("gentle-ai.review-changed-path-manifest/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// NewArtifactSubject derives one slot identity from native compact authority
// and the exact frozen context already rendered for that authority.
func NewArtifactSubject(state CompactState, revision string, frozen FrozenCandidateContext, lens string, order int, correctionTargetIdentity string) (ArtifactSubject, error) {
	if validateLineageID(state.LineageID) != nil || !validSHA256(revision) || !validSHA256(state.InitialSnapshot.Identity) {
		return ArtifactSubject{}, errors.New("artifact subject authority binding is incomplete")
	}
	if order < 0 || order >= len(state.SelectedLenses) || state.SelectedLenses[order] != lens || !isSupportedLens(lens) {
		return ArtifactSubject{}, errors.New("artifact subject does not bind a selected lens slot")
	}
	if correctionTargetIdentity != "" && !validSHA256(correctionTargetIdentity) {
		return ArtifactSubject{}, errors.New("artifact subject correction identity is invalid")
	}
	if _, err := frozen.CandidateDiff.Bytes(); err != nil {
		return ArtifactSubject{}, err
	}
	manifestDigest, err := ChangedPathManifestDigest(frozen.ChangedPathManifest)
	if err != nil {
		return ArtifactSubject{}, err
	}
	paths := make([]string, len(frozen.ChangedPathManifest))
	for index, entry := range frozen.ChangedPathManifest {
		paths[index] = entry.Path
	}
	if !reflect.DeepEqual(paths, state.InitialSnapshot.Paths) {
		return ArtifactSubject{}, errors.New("artifact subject manifest does not match the frozen snapshot paths")
	}
	subject := ArtifactSubject{
		Schema: ArtifactSubjectSchema, LineageID: state.LineageID, AuthorityRevision: revision,
		TargetIdentity: state.InitialSnapshot.Identity, CandidateDiffSHA256: frozen.CandidateDiff.SHA256,
		ChangedPathManifestSHA256: manifestDigest, Lens: lens, SelectedOrder: order,
		CorrectionTargetIdentity: correctionTargetIdentity,
	}
	subject.SubjectHash = artifactSubjectHash(subject)
	return subject, ValidateArtifactSubject(subject)
}

// ValidateArtifactSubject verifies the canonical self-hash and every identity
// field without consulting mutable repository state.
func ValidateArtifactSubject(subject ArtifactSubject) error {
	if subject.Schema != ArtifactSubjectSchema || validateLineageID(subject.LineageID) != nil ||
		!validSHA256(subject.AuthorityRevision) || !validSHA256(subject.TargetIdentity) ||
		!validSHA256(subject.CandidateDiffSHA256) || !validSHA256(subject.ChangedPathManifestSHA256) ||
		!isSupportedLens(subject.Lens) || subject.SelectedOrder < 0 ||
		(subject.CorrectionTargetIdentity != "" && !validSHA256(subject.CorrectionTargetIdentity)) {
		return errors.New("artifact subject identity is incomplete")
	}
	if !validSHA256(subject.SubjectHash) || artifactSubjectHash(subject) != subject.SubjectHash {
		return errors.New("artifact subject hash does not match its binding")
	}
	return nil
}

func artifactSubjectHash(subject ArtifactSubject) string {
	preimage := struct {
		Schema                    string `json:"schema"`
		LineageID                 string `json:"lineage_id"`
		AuthorityRevision         string `json:"authority_revision"`
		TargetIdentity            string `json:"target_identity"`
		CandidateDiffSHA256       string `json:"candidate_diff_sha256"`
		ChangedPathManifestSHA256 string `json:"changed_path_manifest_sha256"`
		Lens                      string `json:"lens"`
		SelectedOrder             int    `json:"selected_order"`
		CorrectionTargetIdentity  string `json:"correction_target_identity,omitempty"`
	}{
		Schema: subject.Schema, LineageID: subject.LineageID, AuthorityRevision: subject.AuthorityRevision,
		TargetIdentity: subject.TargetIdentity, CandidateDiffSHA256: subject.CandidateDiffSHA256,
		ChangedPathManifestSHA256: subject.ChangedPathManifestSHA256, Lens: subject.Lens,
		SelectedOrder: subject.SelectedOrder, CorrectionTargetIdentity: subject.CorrectionTargetIdentity,
	}
	payload, _ := json.Marshal(preimage)
	sum := sha256.Sum256(append([]byte("gentle-ai.review-artifact-subject/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}
