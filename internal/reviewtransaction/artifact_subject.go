package reviewtransaction

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
)

const ArtifactSubjectSchemaV1 = "gentle-ai.review-artifact-subject/v1"
const ArtifactSubjectSchema = "gentle-ai.review-artifact-subject/v2"

// ArtifactSubject is the provider-owned identity of one reviewer execution.
// It binds the exact authority revision, immutable Git trees and path manifest,
// selected lens slot, and (when present) correction target.
type ArtifactSubject struct {
	Schema                    string `json:"schema"`
	SubjectHash               string `json:"subject_hash"`
	LineageID                 string `json:"lineage_id"`
	AuthorityRevision         string `json:"authority_revision"`
	TargetIdentity            string `json:"target_identity"`
	CandidateDiffSHA256       string `json:"candidate_diff_sha256,omitempty"`
	BaseTree                  string `json:"base_tree,omitempty"`
	CandidateTree             string `json:"candidate_tree,omitempty"`
	ChangedPathManifestSHA256 string `json:"changed_path_manifest_sha256"`
	Lens                      string `json:"lens"`
	SelectedOrder             int    `json:"selected_order"`
	CorrectionTargetIdentity  string `json:"correction_target_identity,omitempty"`
}

// NewLegacyArtifactSubject preserves the published v1 subject preimage for
// consumers that explicitly negotiate review-integration/v1.
func NewLegacyArtifactSubject(state CompactState, revision string, frozen FrozenCandidateContext, lens string, order int, correctionTargetIdentity string) (ArtifactSubject, error) {
	if frozen.LegacyCandidateDiff == nil {
		return ArtifactSubject{}, errors.New("legacy artifact subject requires candidate diff") // refusal:by-design world-action: provider construction omitted required immutable v1 transport and must be fixed before retry
	}
	if _, err := frozen.LegacyCandidateDiff.Bytes(); err != nil {
		return ArtifactSubject{}, err
	}
	subject, err := newArtifactSubjectIdentity(state, revision, frozen, lens, order, correctionTargetIdentity)
	if err != nil {
		return ArtifactSubject{}, err
	}
	subject.Schema = ArtifactSubjectSchemaV1
	subject.CandidateDiffSHA256 = frozen.LegacyCandidateDiff.SHA256
	subject.BaseTree, subject.CandidateTree = "", ""
	subject.SubjectHash = artifactSubjectHashV1(subject)
	return subject, ValidateArtifactSubject(subject)
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
	subject, err := newArtifactSubjectIdentity(state, revision, frozen, lens, order, correctionTargetIdentity)
	if err != nil {
		return ArtifactSubject{}, err
	}
	subject.Schema = ArtifactSubjectSchema
	subject.SubjectHash = artifactSubjectHash(subject)
	return subject, ValidateArtifactSubject(subject)
}

func newArtifactSubjectIdentity(state CompactState, revision string, frozen FrozenCandidateContext, lens string, order int, correctionTargetIdentity string) (ArtifactSubject, error) {
	if validateLineageID(state.LineageID) != nil || !validSHA256(revision) || !validSHA256(state.InitialSnapshot.Identity) {
		return ArtifactSubject{}, errors.New("artifact subject authority binding is incomplete")
	}
	if order < 0 || order >= len(state.SelectedLenses) || state.SelectedLenses[order] != lens || !isSupportedLens(lens) {
		return ArtifactSubject{}, errors.New("artifact subject does not bind a selected lens slot")
	}
	if correctionTargetIdentity != "" && !validSHA256(correctionTargetIdentity) {
		return ArtifactSubject{}, errors.New("artifact subject correction identity is invalid")
	}
	if !validGitTree(frozen.BaseTree) || !validGitTree(frozen.CandidateTree) ||
		frozen.BaseTree != state.InitialSnapshot.BaseTree || frozen.CandidateTree != state.InitialSnapshot.CandidateTree {
		return ArtifactSubject{}, errors.New("artifact subject trees do not match the frozen snapshot") // refusal:by-design world-action: this provider-built subject is inconsistent; the exit is a code fix, not a command
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
	return ArtifactSubject{
		LineageID: state.LineageID, AuthorityRevision: revision,
		TargetIdentity: state.InitialSnapshot.Identity, BaseTree: frozen.BaseTree, CandidateTree: frozen.CandidateTree,
		ChangedPathManifestSHA256: manifestDigest, Lens: lens, SelectedOrder: order,
		CorrectionTargetIdentity: correctionTargetIdentity,
	}, nil
}

// ValidateArtifactSubject verifies the canonical self-hash and every identity
// field without consulting mutable repository state.
func ValidateArtifactSubject(subject ArtifactSubject) error {
	if validateLineageID(subject.LineageID) != nil ||
		!validSHA256(subject.AuthorityRevision) || !validSHA256(subject.TargetIdentity) ||
		!validSHA256(subject.ChangedPathManifestSHA256) ||
		!isSupportedLens(subject.Lens) || subject.SelectedOrder < 0 ||
		(subject.CorrectionTargetIdentity != "" && !validSHA256(subject.CorrectionTargetIdentity)) {
		return errors.New("artifact subject identity is incomplete")
	}
	var wantHash string
	switch subject.Schema {
	case ArtifactSubjectSchemaV1:
		if !validSHA256(subject.CandidateDiffSHA256) || subject.BaseTree != "" || subject.CandidateTree != "" {
			return errors.New("legacy artifact subject identity is incomplete") // refusal:by-design world-action: the immutable provider-owned v1 identity must be replaced by its producer
		}
		wantHash = artifactSubjectHashV1(subject)
	case ArtifactSubjectSchema:
		if subject.CandidateDiffSHA256 != "" || !validGitTree(subject.BaseTree) || !validGitTree(subject.CandidateTree) {
			return errors.New("artifact subject tree identity is incomplete") // refusal:by-design world-action: the immutable provider-owned native Git identity must be replaced by its producer
		}
		wantHash = artifactSubjectHash(subject)
	default:
		return errors.New("artifact subject schema is unsupported") // refusal:by-design world-action: the immutable provider-owned subject must be replaced with a supported contract identity
	}
	if !validSHA256(subject.SubjectHash) || wantHash != subject.SubjectHash {
		return errors.New("artifact subject hash does not match its binding")
	}
	return nil
}

func artifactSubjectHashV1(subject ArtifactSubject) string {
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

func artifactSubjectHash(subject ArtifactSubject) string {
	preimage := struct {
		Schema                    string `json:"schema"`
		LineageID                 string `json:"lineage_id"`
		AuthorityRevision         string `json:"authority_revision"`
		TargetIdentity            string `json:"target_identity"`
		BaseTree                  string `json:"base_tree"`
		CandidateTree             string `json:"candidate_tree"`
		ChangedPathManifestSHA256 string `json:"changed_path_manifest_sha256"`
		Lens                      string `json:"lens"`
		SelectedOrder             int    `json:"selected_order"`
		CorrectionTargetIdentity  string `json:"correction_target_identity,omitempty"`
	}{
		Schema: subject.Schema, LineageID: subject.LineageID, AuthorityRevision: subject.AuthorityRevision,
		TargetIdentity: subject.TargetIdentity, BaseTree: subject.BaseTree, CandidateTree: subject.CandidateTree,
		ChangedPathManifestSHA256: subject.ChangedPathManifestSHA256, Lens: subject.Lens,
		SelectedOrder: subject.SelectedOrder, CorrectionTargetIdentity: subject.CorrectionTargetIdentity,
	}
	payload, _ := json.Marshal(preimage)
	sum := sha256.Sum256(append([]byte("gentle-ai.review-artifact-subject/v2\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}
