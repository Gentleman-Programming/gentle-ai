package reviewtransaction

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const (
	VerificationEvidenceRecordSchema  = "gentle-ai.review-verification-evidence/v2"
	VerificationEvidenceRecordVersion = 2
	CompactFinalEvidenceRecordFile    = "record.json"
	verificationEvidenceRecordLimit   = 1 << 20
	verificationEvidencePayloadLimit  = 4 << 20
)

type VerificationOutcome string

const (
	VerificationOutcomePassed            VerificationOutcome = "passed"
	VerificationOutcomeFailed            VerificationOutcome = "verification_failed"
	VerificationOutcomeProceduralFailure VerificationOutcome = "procedural_tooling_failed"
)

var (
	ErrCapturedVerificationEvidenceMissing         = errors.New("captured verification evidence is missing")                        // refusal:by-design operator-knowledge: STATUS supplies the exact candidate-bound capture transition when evidence is absent
	ErrCapturedVerificationEvidenceMetadataMissing = errors.New("captured verification evidence metadata is missing")               // refusal:by-design operator-knowledge: legacy bytes require an exact outcome-bearing recapture before they can be trusted
	ErrCapturedVerificationEvidenceInvalid         = errors.New("captured verification evidence is invalid")                        // refusal:by-design world-action: malformed immutable evidence cannot authorize a transition and requires code or storage repair
	ErrCapturedVerificationEvidenceConflict        = errors.New("captured verification evidence conflicts with immutable evidence") // refusal:by-design world-action: conflicting bytes or outcomes cannot overwrite the candidate's immutable evidence
)

// VerificationEvidenceRecord binds opaque verification bytes to one immutable
// candidate and the authority revision that requested their collection.
type VerificationEvidenceRecord struct {
	Schema            string              `json:"schema"`
	Version           int                 `json:"version"`
	LineageID         string              `json:"lineage_id"`
	AuthorityRevision string              `json:"authority_revision"`
	TargetIdentity    string              `json:"target_identity"`
	CandidateTree     string              `json:"candidate_tree"`
	PathsDigest       string              `json:"paths_digest"`
	Paths             []string            `json:"paths"`
	LedgerIDs         []string            `json:"ledger_ids"`
	RawPayloadSHA256  string              `json:"raw_payload_sha256"`
	RawPayloadBytes   int64               `json:"raw_payload_bytes"`
	Outcome           VerificationOutcome `json:"outcome"`
	RecordDigest      string              `json:"record_digest"`
}

type CapturedVerificationEvidence struct {
	Record  VerificationEvidenceRecord
	Payload []byte
}

type CaptureVerificationEvidenceRequest struct {
	StoreDir          string
	LineageID         string
	AuthorityRevision string
	Target            Snapshot
	Payload           []byte
	Outcome           VerificationOutcome
}

func validVerificationOutcome(outcome VerificationOutcome) bool {
	switch outcome {
	case VerificationOutcomePassed, VerificationOutcomeFailed, VerificationOutcomeProceduralFailure:
		return true
	default:
		return false
	}
}

func NewVerificationEvidenceRecord(lineageID, authorityRevision string, target Snapshot, payload []byte, outcome VerificationOutcome) (VerificationEvidenceRecord, error) {
	if err := validateCompactSnapshot(target); err != nil {
		return VerificationEvidenceRecord{}, fmt.Errorf("verification evidence target: %w", err)
	}
	record := VerificationEvidenceRecord{
		Schema: VerificationEvidenceRecordSchema, Version: VerificationEvidenceRecordVersion,
		LineageID: lineageID, AuthorityRevision: authorityRevision, TargetIdentity: target.Identity,
		CandidateTree: target.CandidateTree, PathsDigest: target.PathsDigest,
		Paths: append([]string(nil), target.Paths...), LedgerIDs: append([]string(nil), target.LedgerIDs...),
		RawPayloadSHA256: finalVerificationPayloadDigest(payload), RawPayloadBytes: int64(len(payload)), Outcome: outcome,
	}
	record.RecordDigest = verificationEvidenceRecordDigest(record)
	if err := record.ValidatePayload(payload); err != nil {
		return VerificationEvidenceRecord{}, err
	}
	return record, nil
}

func verificationEvidenceRecordDigest(record VerificationEvidenceRecord) string {
	record.RecordDigest = ""
	payload, err := json.Marshal(record)
	if err != nil {
		return ""
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("gentle-ai.review-verification-evidence-record/v2"))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(payload)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func (record VerificationEvidenceRecord) Validate() error {
	if record.Schema != VerificationEvidenceRecordSchema || record.Version != VerificationEvidenceRecordVersion ||
		validateLineageID(record.LineageID) != nil || !validSHA256(record.AuthorityRevision) ||
		!validSHA256(record.TargetIdentity) || !validGitTree(record.CandidateTree) || !validSHA256(record.PathsDigest) ||
		!validSHA256(record.RawPayloadSHA256) || record.RawPayloadBytes <= 0 || record.RawPayloadBytes > verificationEvidencePayloadLimit ||
		!validVerificationOutcome(record.Outcome) || !validSHA256(record.RecordDigest) || record.RecordDigest != verificationEvidenceRecordDigest(record) {
		return errors.New("invalid verification evidence record identity or digest") // refusal:by-design world-action: an invalid immutable record cannot be repaired without trustworthy source evidence
	}
	paths, err := canonicalPaths(record.Paths)
	if err != nil || !equalStrings(paths, record.Paths) {
		return errors.New("verification evidence paths are not canonical") // refusal:by-design world-action: noncanonical immutable metadata cannot be interpreted without changing its signed identity
	}
	ledgerIDs, err := canonicalStrings(record.LedgerIDs, "ledger id")
	if err != nil || !equalStrings(ledgerIDs, record.LedgerIDs) {
		return errors.New("verification evidence ledger IDs are not canonical") // refusal:by-design world-action: noncanonical immutable metadata cannot be interpreted without changing its signed identity
	}
	return nil
}

func (record VerificationEvidenceRecord) ValidatePayload(payload []byte) error {
	if err := record.Validate(); err != nil {
		return err
	}
	if int64(len(payload)) != record.RawPayloadBytes || len(payload) == 0 || len(payload) > verificationEvidencePayloadLimit ||
		finalVerificationPayloadDigest(payload) != record.RawPayloadSHA256 {
		return errors.New("verification evidence raw payload does not match its record") // refusal:by-design world-action: immutable raw bytes and metadata conflict, so neither can safely authorize continuation
	}
	return nil
}

func (record VerificationEvidenceRecord) ValidateBinding(lineageID, authorityRevision string, target Snapshot) error {
	if err := record.Validate(); err != nil {
		return err
	}
	if record.LineageID != lineageID || record.AuthorityRevision != authorityRevision ||
		record.TargetIdentity != target.Identity || record.CandidateTree != target.CandidateTree ||
		record.PathsDigest != target.PathsDigest || !equalStrings(record.Paths, target.Paths) || !equalStrings(record.LedgerIDs, target.LedgerIDs) {
		return errors.New("verification evidence record does not match the requested authority and candidate") // refusal:by-design world-action: evidence captured for another immutable target cannot be rebound to this authority
	}
	return nil
}

func CanonicalVerificationEvidenceRecord(record VerificationEvidenceRecord) ([]byte, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func ParseVerificationEvidenceRecord(payload []byte) (VerificationEvidenceRecord, error) {
	if bytes.Contains(payload, []byte{'\r'}) {
		return VerificationEvidenceRecord{}, errors.New("verification evidence record must use LF-only canonical JSON") // refusal:by-design world-action: changing immutable record bytes would change their authority-bound digest
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var record VerificationEvidenceRecord
	if err := decoder.Decode(&record); err != nil {
		return VerificationEvidenceRecord{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return VerificationEvidenceRecord{}, errors.New("verification evidence record contains multiple JSON values") // refusal:by-design world-action: ambiguous immutable record bytes cannot be normalized after capture
	}
	canonical, err := CanonicalVerificationEvidenceRecord(record)
	if err != nil {
		return VerificationEvidenceRecord{}, err
	}
	if !bytes.Equal(payload, canonical) {
		return VerificationEvidenceRecord{}, errors.New("verification evidence record is not canonical JSON") // refusal:by-design world-action: normalizing immutable record bytes would change their authority-bound digest
	}
	return record, nil
}

func compactFinalEvidenceCandidateDir(storeDir, targetIdentity string) (string, error) {
	if !validSHA256(targetIdentity) {
		return "", errors.New("verification evidence target identity is invalid") // refusal:by-design world-action: an invalid identity cannot define a safe candidate-addressed storage directory
	}
	return filepath.Join(storeDir, CompactFinalEvidenceDir, strings.TrimPrefix(targetIdentity, "sha256:")), nil
}

func ensureCompactEvidenceDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !compactPrivateArtifactMode(info.Mode(), true) {
		return errors.New("verification evidence directory is unavailable or unsafe") // refusal:by-design world-action: unsafe filesystem ownership or shape must be repaired before immutable evidence can be trusted
	}
	return nil
}

func readCompactEvidenceFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || !compactPrivateArtifactMode(info.Mode(), false) {
		return nil, errors.New("verification evidence artifact is unavailable or unsafe") // refusal:by-design world-action: unsafe filesystem ownership or shape must be repaired before immutable evidence can be trusted
	}
	finalVerificationRetryEvidenceAfterLstat()
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !compactPrivateArtifactMode(opened.Mode(), false) || !os.SameFile(info, opened) {
		return nil, errors.New("verification evidence artifact changed before read") // refusal:by-design world-action: concurrent filesystem mutation prevents a trustworthy immutable read
	}
	payload, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(payload) == 0 || int64(len(payload)) > limit {
		return nil, errors.New("verification evidence artifact is empty or exceeds its native limit") // refusal:by-design world-action: invalid persisted bytes cannot be truncated or invented without falsifying evidence
	}
	after, err := os.Lstat(path)
	if err != nil || !after.Mode().IsRegular() || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, after) {
		return nil, errors.New("verification evidence artifact changed during read") // refusal:by-design world-action: concurrent filesystem mutation prevents a trustworthy immutable read
	}
	return payload, nil
}

func ReadCapturedVerificationEvidence(storeDir, lineageID, authorityRevision string, target Snapshot) (CapturedVerificationEvidence, error) {
	dir, err := compactFinalEvidenceCandidateDir(storeDir, target.Identity)
	if err != nil {
		return CapturedVerificationEvidence{}, fmt.Errorf("%w: %v", ErrCapturedVerificationEvidenceInvalid, err)
	}
	info, dirErr := os.Lstat(dir)
	if errors.Is(dirErr, os.ErrNotExist) {
		legacyPath := filepath.Join(storeDir, CompactFinalEvidenceDir, CompactFinalEvidenceFile)
		if _, legacyErr := os.Lstat(legacyPath); legacyErr == nil {
			return CapturedVerificationEvidence{}, ErrCapturedVerificationEvidenceMetadataMissing
		} else if !errors.Is(legacyErr, os.ErrNotExist) {
			return CapturedVerificationEvidence{}, fmt.Errorf("%w: %v", ErrCapturedVerificationEvidenceInvalid, legacyErr)
		}
		return CapturedVerificationEvidence{}, ErrCapturedVerificationEvidenceMissing
	}
	if dirErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !compactPrivateArtifactMode(info.Mode(), true) {
		return CapturedVerificationEvidence{}, ErrCapturedVerificationEvidenceInvalid
	}
	rawPath := filepath.Join(dir, CompactFinalEvidenceFile)
	recordPath := filepath.Join(dir, CompactFinalEvidenceRecordFile)
	raw, rawErr := readCompactEvidenceFile(rawPath, verificationEvidencePayloadLimit)
	recordPayload, recordErr := readCompactEvidenceFile(recordPath, verificationEvidenceRecordLimit)
	if errors.Is(rawErr, os.ErrNotExist) && errors.Is(recordErr, os.ErrNotExist) {
		return CapturedVerificationEvidence{}, ErrCapturedVerificationEvidenceMissing
	}
	if rawErr == nil && errors.Is(recordErr, os.ErrNotExist) {
		return CapturedVerificationEvidence{}, ErrCapturedVerificationEvidenceMetadataMissing
	}
	if rawErr != nil || recordErr != nil {
		return CapturedVerificationEvidence{}, fmt.Errorf("%w: raw=%v metadata=%v", ErrCapturedVerificationEvidenceInvalid, rawErr, recordErr)
	}
	record, err := ParseVerificationEvidenceRecord(recordPayload)
	if err != nil || record.ValidatePayload(raw) != nil || record.ValidateBinding(lineageID, authorityRevision, target) != nil {
		return CapturedVerificationEvidence{}, ErrCapturedVerificationEvidenceInvalid
	}
	dirAfter, err := os.Lstat(dir)
	if err != nil || !dirAfter.IsDir() || dirAfter.Mode()&os.ModeSymlink != 0 ||
		!compactPrivateArtifactMode(dirAfter.Mode(), true) || !os.SameFile(info, dirAfter) {
		return CapturedVerificationEvidence{}, ErrCapturedVerificationEvidenceInvalid
	}
	return CapturedVerificationEvidence{Record: record, Payload: raw}, nil
}

func PublishCapturedVerificationEvidence(request CaptureVerificationEvidenceRequest) (CapturedVerificationEvidence, error) {
	record, err := NewVerificationEvidenceRecord(request.LineageID, request.AuthorityRevision, request.Target, request.Payload, request.Outcome)
	if err != nil {
		return CapturedVerificationEvidence{}, err
	}
	dir, err := compactFinalEvidenceCandidateDir(request.StoreDir, request.Target.Identity)
	if err != nil {
		return CapturedVerificationEvidence{}, err
	}
	root := filepath.Join(request.StoreDir, CompactFinalEvidenceDir)
	if err := ensureCompactEvidenceDirectory(root); err != nil {
		return CapturedVerificationEvidence{}, err
	}
	if _, statErr := os.Lstat(dir); errors.Is(statErr, os.ErrNotExist) {
		legacyPath := filepath.Join(root, CompactFinalEvidenceFile)
		if legacy, legacyErr := readCompactEvidenceFile(legacyPath, verificationEvidencePayloadLimit); legacyErr == nil && !bytes.Equal(legacy, request.Payload) {
			return CapturedVerificationEvidence{}, ErrCapturedVerificationEvidenceConflict
		} else if legacyErr != nil && !errors.Is(legacyErr, os.ErrNotExist) {
			return CapturedVerificationEvidence{}, fmt.Errorf("inspect legacy verification evidence: %w", legacyErr)
		}
	}
	if err := ensureCompactEvidenceDirectory(dir); err != nil {
		return CapturedVerificationEvidence{}, err
	}
	recordPayload, err := CanonicalVerificationEvidenceRecord(record)
	if err != nil {
		return CapturedVerificationEvidence{}, err
	}
	if err := publishImmutable(filepath.Join(dir, CompactFinalEvidenceFile), request.Payload, 0o600); err != nil {
		var conflict *ImmutablePublicationConflictError
		if errors.As(err, &conflict) {
			return CapturedVerificationEvidence{}, ErrCapturedVerificationEvidenceConflict
		}
		return CapturedVerificationEvidence{}, err
	}
	if err := publishImmutable(filepath.Join(dir, CompactFinalEvidenceRecordFile), recordPayload, 0o600); err != nil {
		var conflict *ImmutablePublicationConflictError
		if errors.As(err, &conflict) {
			return CapturedVerificationEvidence{}, ErrCapturedVerificationEvidenceConflict
		}
		return CapturedVerificationEvidence{}, err
	}
	captured, err := ReadCapturedVerificationEvidence(request.StoreDir, request.LineageID, request.AuthorityRevision, request.Target)
	if err != nil || !reflect.DeepEqual(captured.Record, record) || !bytes.Equal(captured.Payload, request.Payload) {
		if err != nil {
			return CapturedVerificationEvidence{}, err
		}
		return CapturedVerificationEvidence{}, ErrCapturedVerificationEvidenceConflict
	}
	return captured, nil
}
