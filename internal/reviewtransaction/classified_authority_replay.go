package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

const authorityRepairMaxQuarantineRecords = authorityRepairMaxLineages

type classifiedAuthorityRepairRecordMatch struct {
	record CompactReclaimRecord
	path   string
}

// ClassifiedAuthorityRepairProgressError preserves the durable two-phase
// boundary when a classified repair stops after preparing or committing its
// audit record. Progress contains only path-free opaque identities.
type ClassifiedAuthorityRepairProgressError struct {
	Progress        ClassifiedAuthorityRepairExecution
	ExactReplaySafe bool
	Cause           error
}

func (err *ClassifiedAuthorityRepairProgressError) Error() string {
	return fmt.Sprintf("classified authority repair %s: %v", err.Progress.Status, err.Cause)
}

func (err *ClassifiedAuthorityRepairProgressError) Unwrap() error { return err.Cause }

func newClassifiedAuthorityRepairAudit(assessment AuthorityRepairAssessment, request ClassifiedAuthorityRepairRequest) (*ClassifiedAuthorityRepairAudit, error) {
	assessment = cloneAuthorityRepairAssessment(assessment)
	assessmentDigest, err := classifiedAuthorityRepairAssessmentDigest(assessment)
	if err != nil {
		return nil, err
	}
	requestDigest, err := classifiedAuthorityRepairRequestDigest(request)
	if err != nil {
		return nil, err
	}
	return &ClassifiedAuthorityRepairAudit{
		Schema: ClassifiedAuthorityRepairAuditSchema, Route: ClassifiedAuthorityRepairRoute,
		Assessment: assessment, AssessmentDigest: assessmentDigest, RequestDigest: requestDigest,
	}, nil
}

func cloneAuthorityRepairAssessment(assessment AuthorityRepairAssessment) AuthorityRepairAssessment {
	assessment.SupportedOperations = append([]string(nil), assessment.SupportedOperations...)
	if assessment.Candidate != nil {
		candidate := *assessment.Candidate
		candidate.Operations = append([]string(nil), assessment.Candidate.Operations...)
		assessment.Candidate = &candidate
	}
	return assessment
}

func classifiedAuthorityRepairAssessmentDigest(assessment AuthorityRepairAssessment) (string, error) {
	return classifiedAuthorityRepairDigest("gentle-ai.review-classified-assessment-digest/v1", assessment)
}

func classifiedAuthorityRepairRequestDigest(request ClassifiedAuthorityRepairRequest) (string, error) {
	canonical := struct {
		Class             AuthorityRepairClass       `json:"class"`
		LineageID         string                     `json:"lineage_id"`
		ExpectedRevision  string                     `json:"expected_revision"`
		Cause             AuthorityRepairCause       `json:"cause"`
		Disposition       AuthorityRepairDisposition `json:"disposition"`
		RepositoryBinding string                     `json:"repository_binding"`
		Actor             string                     `json:"actor"`
		Reason            string                     `json:"reason"`
	}{
		Class: request.Class, LineageID: request.LineageID, ExpectedRevision: request.ExpectedRevision,
		Cause: request.Cause, Disposition: request.Disposition, RepositoryBinding: request.RepositoryBinding,
		Actor: request.Actor, Reason: request.Reason,
	}
	return classifiedAuthorityRepairDigest("gentle-ai.review-classified-request-digest/v1", canonical)
}

func classifiedAuthorityRepairDigest(domain string, value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte(domain+"\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func validateClassifiedAuthorityRepairAudit(audit *ClassifiedAuthorityRepairAudit, request ClassifiedAuthorityRepairRequest) error {
	if audit == nil || audit.Schema != ClassifiedAuthorityRepairAuditSchema || audit.Route != ClassifiedAuthorityRepairRoute {
		return errors.New("classified authority repair audit route is invalid")
	}
	if err := audit.Assessment.Validate(); err != nil {
		return fmt.Errorf("classified authority repair audit assessment: %w", err)
	}
	assessmentDigest, err := classifiedAuthorityRepairAssessmentDigest(audit.Assessment)
	if err != nil || assessmentDigest != audit.AssessmentDigest {
		return errors.New("classified authority repair assessment digest is invalid")
	}
	requestDigest, err := classifiedAuthorityRepairRequestDigest(request)
	if err != nil || requestDigest != audit.RequestDigest {
		return errors.New("classified authority repair request digest is invalid")
	}
	if err := validateClassifiedAuthorityRepairRequest(request, audit.Assessment); err != nil {
		return fmt.Errorf("classified authority repair audit request: %w", err)
	}
	return nil
}

func classifiedAuthorityRepairRecordIdentity(record CompactReclaimRecord) (string, error) {
	canonical := struct {
		Schema                    string                          `json:"schema"`
		LineageID                 string                          `json:"lineage_id"`
		Reason                    string                          `json:"reason"`
		Actor                     string                          `json:"actor"`
		ReclaimedAt               string                          `json:"reclaimed_at"`
		Residue                   []string                        `json:"residue"`
		LegacyAliasRepair         *LegacyAliasRepairProof         `json:"legacy_alias_repair"`
		ClassifiedAuthorityRepair *ClassifiedAuthorityRepairAudit `json:"classified_authority_repair"`
	}{
		Schema: record.Schema, LineageID: record.LineageID, Reason: record.Reason, Actor: record.Actor,
		ReclaimedAt: record.ReclaimedAt.UTC().Format(time.RFC3339Nano),
		Residue:     append([]string(nil), record.Residue...), LegacyAliasRepair: record.LegacyAliasRepair,
		ClassifiedAuthorityRepair: record.ClassifiedAuthorityRepair,
	}
	return classifiedAuthorityRepairDigest("gentle-ai.review-classified-record-identity/v1", canonical)
}

func discoverClassifiedAuthorityRepairRecord(ctx context.Context, base string, request ClassifiedAuthorityRepairRequest) (classifiedAuthorityRepairRecordMatch, bool, error) {
	if err := ctx.Err(); err != nil {
		return classifiedAuthorityRepairRecordMatch{}, false, err
	}
	quarantineRoot := filepath.Join(base, "quarantine")
	if err := ensureCanonicalReviewQuarantineRoot(base, quarantineRoot); err != nil {
		return classifiedAuthorityRepairRecordMatch{}, false, err
	}
	entries, err := readAuthorityRepairDirectory(quarantineRoot, authorityRepairMaxQuarantineRecords)
	if os.IsNotExist(err) {
		return classifiedAuthorityRepairRecordMatch{}, false, nil
	}
	if err != nil {
		return classifiedAuthorityRepairRecordMatch{}, false, fmt.Errorf("inspect classified repair quarantine: %w", err)
	}
	var matched *classifiedAuthorityRepairRecordMatch
	totalBytes := int64(0)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return classifiedAuthorityRepairRecordMatch{}, false, err
		}
		if !strings.HasPrefix(entry.Name(), request.LineageID+"-") {
			continue
		}
		path := filepath.Join(quarantineRoot, entry.Name())
		info, statErr := os.Lstat(path)
		canonical, canonicalErr := filepath.EvalSymlinks(path)
		if statErr != nil || canonicalErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || filepath.Clean(canonical) != path {
			return classifiedAuthorityRepairRecordMatch{}, false, errors.New("classified authority repair refused unsafe quarantine directory")
		}
		recordPath := filepath.Join(path, "reclaim-record.json")
		recordInfo, statErr := os.Lstat(recordPath)
		if statErr != nil || recordInfo.Mode()&os.ModeSymlink != 0 || !recordInfo.Mode().IsRegular() || recordInfo.Size() < 0 || recordInfo.Size() > authorityRepairMaxEventBytes {
			return classifiedAuthorityRepairRecordMatch{}, false, errors.New("classified authority repair refused unsafe replay record")
		}
		totalBytes += recordInfo.Size()
		if totalBytes > authorityRepairMaxTotalBytes {
			return classifiedAuthorityRepairRecordMatch{}, false, errAuthorityRepairTruncated
		}
		payload, readErr := os.ReadFile(recordPath)
		if readErr != nil || int64(len(payload)) != recordInfo.Size() {
			return classifiedAuthorityRepairRecordMatch{}, false, errors.New("classified authority repair refused unreadable replay record")
		}
		record, decodeErr := decodeClassifiedAuthorityRepairRecord(payload)
		if decodeErr != nil {
			return classifiedAuthorityRepairRecordMatch{}, false, fmt.Errorf("classified authority repair refused corrupt replay record: %w", decodeErr)
		}
		if record.LineageID != request.LineageID {
			return classifiedAuthorityRepairRecordMatch{}, false, errors.New("classified authority repair refused mismatched replay lineage")
		}
		if record.ClassifiedAuthorityRepair == nil {
			// Compatibility records remain replayable only through the compatibility
			// command. They can never prove provider-owned classification.
			continue
		}
		if err := validateClassifiedAuthorityRepairRecordShape(record, path, base, request); err != nil {
			return classifiedAuthorityRepairRecordMatch{}, false, err
		}
		if matched != nil {
			return classifiedAuthorityRepairRecordMatch{}, false, errors.New("classified authority repair refused duplicate replay records")
		}
		candidate := classifiedAuthorityRepairRecordMatch{record: record, path: path}
		matched = &candidate
	}
	if matched == nil {
		return classifiedAuthorityRepairRecordMatch{}, false, nil
	}
	return *matched, true, nil
}

func decodeClassifiedAuthorityRepairRecord(payload []byte) (CompactReclaimRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var record CompactReclaimRecord
	if err := decoder.Decode(&record); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := decoder.Decode(new(struct{})); err != io.EOF {
		if err == nil {
			return CompactReclaimRecord{}, errors.New("multiple JSON values")
		}
		return CompactReclaimRecord{}, err
	}
	return record, nil
}

func validateClassifiedAuthorityRepairRecordShape(record CompactReclaimRecord, quarantinePath, base string, request ClassifiedAuthorityRepairRequest) error {
	proof := record.LegacyAliasRepair
	if record.Schema != CompactReclaimRecordSchema ||
		(record.Status != CompactReclaimPrepared && record.Status != CompactReclaimCommitted) ||
		record.LineageID != request.LineageID || record.ReclaimedAt.IsZero() || proof == nil ||
		record.InvalidRecoveryEdge != nil || record.MalformedRecoveryAuthorization != nil || record.PristineAbandonment != nil ||
		record.MalformedLegacyFreeze != nil || record.LegacyFixScopeQuarantine != nil {
		return errors.New("classified authority repair refused incomplete replay record")
	}
	if record.SourcePath != filepath.Join(base, "v1", request.LineageID) || record.QuarantinePath != quarantinePath ||
		record.Reason != request.Reason || record.Actor != request.Actor {
		return errors.New("classified authority repair refused mismatched replay audit")
	}
	if err := validateClassifiedAuthorityRepairAudit(record.ClassifiedAuthorityRepair, request); err != nil {
		return err
	}
	candidate := record.ClassifiedAuthorityRepair.Assessment.Candidate
	if candidate == nil || proof.LineageID != candidate.LineageID || proof.HeadRevision != candidate.Revision ||
		proof.ChainIdentity != candidate.ChainIdentity || len(proof.EventRevisions) != candidate.AliasEventCount ||
		len(proof.Operations) != candidate.AliasEventCount || !reflect.DeepEqual(uniqueClassifiedRepairOperations(proof.Operations), candidate.Operations) ||
		proof.Diagnostic != legacyAliasRepairDiagnostic ||
		proof.Disposition != string(request.Disposition) {
		return errors.New("classified authority repair refused mismatched replay proof")
	}
	previous := ""
	for _, name := range record.Residue {
		if name == "" || name <= previous || filepath.Base(name) != name || name == "." || name == ".." || strings.ContainsAny(name, `/\\`) {
			return errors.New("classified authority repair refused noncanonical residue inventory")
		}
		previous = name
	}
	return nil
}

func uniqueClassifiedRepairOperations(operations []string) []string {
	unique := map[string]struct{}{}
	for _, operation := range operations {
		if _, approved := approvedHistoricalAliasCanonicalOperation(operation); !approved {
			return nil
		}
		unique[operation] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for operation := range unique {
		result = append(result, operation)
	}
	sort.Strings(result)
	return result
}

func reconcileClassifiedAuthorityRepairRecord(ctx context.Context, base string, request ClassifiedAuthorityRepairRequest, live AuthorityRepairAssessment, match classifiedAuthorityRepairRecordMatch) (CompactReclaimRecord, error) {
	record := match.record
	sourceExists, sourceErr := canonicalClassifiedRepairDirectory(record.SourcePath)
	if sourceErr != nil {
		return record, sourceErr
	}
	residuePath := filepath.Join(match.path, "residue")
	residueExists, residueErr := canonicalClassifiedRepairDirectory(residuePath)
	if residueErr != nil {
		return record, residueErr
	}
	if record.Status == CompactReclaimCommitted && (sourceExists || !residueExists) {
		return record, errors.New("classified authority repair refused incomplete committed residue")
	}
	if record.Status == CompactReclaimPrepared && sourceExists == residueExists {
		return record, errors.New("classified authority repair refused ambiguous prepared residue")
	}
	proofRoot := residuePath
	if sourceExists {
		proofRoot = record.SourcePath
	}
	if err := validateClassifiedRepairResidue(ctx, request, record, proofRoot); err != nil {
		return record, err
	}
	if sourceExists {
		currentDigest, err := classifiedAuthorityRepairAssessmentDigest(live)
		if err != nil || live.Status != AuthorityRepairEligible || currentDigest != record.ClassifiedAuthorityRepair.AssessmentDigest ||
			validateClassifiedAuthorityRepairRequest(request, live) != nil {
			return record, errors.New("classified authority repair prepared source no longer matches its full assessment")
		}
	} else if live.Status != AuthorityRepairUnsupported || live.Counts.EligibleCandidates != 0 ||
		live.Counts.UnsupportedLineages != 0 || live.Counts.Conflicts != 0 {
		return record, errors.New("classified authority repair replay no longer has unique full-inventory proof")
	}
	if record.Status == CompactReclaimPrepared {
		var err error
		record, err = resumePreparedClassifiedAuthorityRepair(ctx, record, sourceExists)
		if err != nil {
			return record, err
		}
	}
	return record, nil
}

func canonicalClassifiedRepairDirectory(path string) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	canonical, canonicalErr := filepath.EvalSymlinks(path)
	if canonicalErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || filepath.Clean(canonical) != path {
		return false, errors.New("classified authority repair refused unsafe source or residue directory")
	}
	return true, nil
}

func validateClassifiedRepairResidue(ctx context.Context, request ClassifiedAuthorityRepairRequest, record CompactReclaimRecord, dir string) error {
	items, err := readAuthorityRepairDirectory(dir, authorityRepairMaxOperations)
	if err != nil {
		return fmt.Errorf("classified authority repair residue inventory: %w", err)
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(dir, item.Name())
		info, statErr := os.Lstat(path)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("classified authority repair refused unsafe replay residue entry")
		}
		if info.IsDir() {
			canonical, canonicalErr := filepath.EvalSymlinks(path)
			if canonicalErr != nil || filepath.Clean(canonical) != path {
				return errors.New("classified authority repair refused noncanonical replay residue directory")
			}
		}
		residue = append(residue, item.Name())
	}
	sort.Strings(residue)
	if !reflect.DeepEqual(record.Residue, residue) {
		return errors.New("classified authority repair refused missing, extra, or replaced residue inventory")
	}
	counts := AuthorityRepairCounts{}
	budget := &authorityRepairBudget{
		ctx: ctx, counts: &counts, uniqueFiles: map[string]struct{}{}, uniqueEvents: map[string]struct{}{},
	}
	candidate, final, aliases, aliasOperations, err := scanLegacyAuthorityRepair(ctx, dir, request.LineageID, budget)
	if err != nil {
		return fmt.Errorf("classified authority repair refused invalid replay residue: %w", err)
	}
	if candidate == nil {
		return errors.New("classified authority repair refused replay residue without its classified alias candidate")
	}
	scan := authorityRepairScan{}
	if !scan.validLegacyReceipt(dir, final, budget) {
		return errors.New("classified authority repair refused invalid replay receipt")
	}
	if !reflect.DeepEqual(*candidate, *record.ClassifiedAuthorityRepair.Assessment.Candidate) {
		return errors.New("classified authority repair refused changed replay candidate")
	}
	derived := LegacyAliasRepairProof{
		LineageID: candidate.LineageID, HeadRevision: candidate.Revision, ChainIdentity: candidate.ChainIdentity,
		EventRevisions: aliases, Operations: aliasOperations, Diagnostic: legacyAliasRepairDiagnostic,
		Disposition: string(request.Disposition),
	}
	if !reflect.DeepEqual(derived, *record.LegacyAliasRepair) {
		return errors.New("classified authority repair refused forged replay chain proof")
	}
	return nil
}

func resumePreparedClassifiedAuthorityRepair(ctx context.Context, record CompactReclaimRecord, sourceExists bool) (CompactReclaimRecord, error) {
	residuePath := filepath.Join(record.QuarantinePath, "residue")
	if sourceExists {
		if err := reclaimQuarantineResidue(record.SourcePath, residuePath); err != nil {
			return record, fmt.Errorf("resume classified repair residue quarantine: %w", err)
		}
		if err := SyncReviewDirectory(filepath.Dir(record.SourcePath)); err != nil {
			return record, err
		}
		if err := SyncReviewDirectory(record.QuarantinePath); err != nil {
			return record, err
		}
		if err := compactReclaimPhaseHook(ctx, compactReclaimPhaseRenamed, record); err != nil {
			return record, err
		}
	}
	committed := record
	committed.Status = CompactReclaimCommitted
	if err := persistReclaimRecord(committed); err != nil {
		return record, err
	}
	if err := compactReclaimPhaseHook(ctx, compactReclaimPhaseCommitted, committed); err != nil {
		return committed, err
	}
	return committed, nil
}
