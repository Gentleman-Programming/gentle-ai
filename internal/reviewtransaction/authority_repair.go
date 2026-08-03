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
)

const (
	AuthorityRepairAssessmentSchema    = "gentle-ai.review-authority-repair-assessment/v1"
	AuthorityRepairAssessmentSchemaID  = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/authority-repair-assessment.schema.json"
	AuthorityRepairAuthorizationSchema = "gentle-ai.review-repair-authorization/v1"

	AuthorityRepairClassLegacyV1HistoricalAlias               AuthorityRepairClass       = "legacy_v1_historical_alias"
	AuthorityRepairCauseUnsupportedHistoricalV1OperationAlias AuthorityRepairCause       = "unsupported_historical_v1_operation_alias"
	AuthorityRepairDispositionQuarantineHistoricalAlias       AuthorityRepairDisposition = "quarantine-approved-historical-alias"
	AuthorityRepairEligible                                   AuthorityRepairStatus      = "eligible"
	AuthorityRepairUnsupported                                AuthorityRepairStatus      = "unsupported"
	AuthorityRepairAmbiguous                                  AuthorityRepairStatus      = "ambiguous"
	AuthorityRepairConflicting                                AuthorityRepairStatus      = "conflicting"
	AuthorityRepairTruncated                                  AuthorityRepairStatus      = "truncated"
	authorityRepairMaxLineages                                                           = 256
	authorityRepairMaxEvents                                                             = 1024
	authorityRepairMaxEventBytes                                                         = 1 << 20
	authorityRepairMaxTotalBytes                                                         = 8 << 20
	authorityRepairMaxOperations                                                         = 1024
	authorityRepairMaxProofReads                                                         = 4*authorityRepairMaxEvents + 4*authorityRepairMaxLineages
	authorityRepairMaxProofReadBytes                                                     = 3 * authorityRepairMaxTotalBytes
)

var authorityRepairSupportedOperations = []string{"review/complete-fix", "review/validate-fix"}

type AuthorityRepairStatus string
type AuthorityRepairClass string
type AuthorityRepairCause string
type AuthorityRepairDisposition string

type AuthorityRepairAssessment struct {
	Schema              string                     `json:"schema"`
	Status              AuthorityRepairStatus      `json:"status"`
	Class               AuthorityRepairClass       `json:"class,omitempty"`
	Cause               AuthorityRepairCause       `json:"cause,omitempty"`
	Disposition         AuthorityRepairDisposition `json:"disposition,omitempty"`
	RepositoryBinding   string                     `json:"repository_binding,omitempty"`
	Candidate           *AuthorityRepairCandidate  `json:"candidate,omitempty"`
	Counts              AuthorityRepairCounts      `json:"counts"`
	SupportedOperations []string                   `json:"supported_operations"`
	AuthorizationSchema string                     `json:"authorization_schema"`
}

type AuthorityRepairCandidate struct {
	LineageID       string   `json:"lineage_id"`
	Revision        string   `json:"revision"`
	ChainIdentity   string   `json:"chain_identity"`
	EventCount      int      `json:"event_count"`
	AliasEventCount int      `json:"alias_event_count"`
	Operations      []string `json:"operations"`
}

type AuthorityRepairCounts struct {
	Lineages            int `json:"lineages"`
	CompactLineages     int `json:"compact_lineages"`
	LegacyLineages      int `json:"legacy_lineages"`
	Events              int `json:"events"`
	Bytes               int `json:"bytes"`
	EligibleCandidates  int `json:"eligible_candidates"`
	UnsupportedLineages int `json:"unsupported_lineages"`
	Conflicts           int `json:"conflicts"`
}

type ClassifiedAuthorityRepairRequest struct {
	Class                   AuthorityRepairClass
	LineageID               string
	ExpectedRevision        string
	Cause                   AuthorityRepairCause
	Disposition             AuthorityRepairDisposition
	RepositoryBinding       string
	Actor                   string
	Reason                  string
	MaintainerAuthorization string
}

type ClassifiedAuthorityRepairExecution struct {
	Status           string                     `json:"status"`
	Class            AuthorityRepairClass       `json:"class"`
	LineageID        string                     `json:"lineage_id"`
	Revision         string                     `json:"revision"`
	ChainIdentity    string                     `json:"chain_identity"`
	Cause            AuthorityRepairCause       `json:"cause"`
	Disposition      AuthorityRepairDisposition `json:"disposition"`
	AssessmentDigest string                     `json:"assessment_digest"`
	RequestDigest    string                     `json:"request_digest"`
	RecordIdentity   string                     `json:"record_identity"`
}

var classifiedAuthorityRepairAfterAssessmentHook = func() {}

type authorityRepairBudget struct {
	ctx          context.Context
	counts       *AuthorityRepairCounts
	entries      int
	proofReads   int
	proofBytes   int
	uniqueFiles  map[string]struct{}
	uniqueEvents map[string]struct{}
	truncated    bool
}

type authorityRepairScan struct {
	assessment       AuthorityRepairAssessment
	base             string
	binding          string
	maintenanceOwned bool
	candidates       []AuthorityRepairCandidate
	unsupported      int
	conflicts        int
	truncated        bool
	compact          map[string]CompactRecord
	legacy           map[string]struct{}
}

var errAuthorityRepairTruncated = errors.New("authority repair assessment limit exceeded")
var errAuthorityRepairConcurrent = errors.New("authority repair assessment changed during proof")

// AssessAuthorityRepair inspects the complete authority inventory without
// creating files or mutating authority bytes. It exposes only a bounded,
// path-free classification.
func AssessAuthorityRepair(ctx context.Context, repo string) (AuthorityRepairAssessment, error) {
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return AuthorityRepairAssessment{}, err
	}
	return AssessAuthorityRepairAtRepositoryRoot(ctx, root)
}

// AssessAuthorityRepairAtRepositoryRoot is the STATUS/preflight/execution
// shared classifier when the canonical repository root is already resolved.
func AssessAuthorityRepairAtRepositoryRoot(ctx context.Context, root string) (AuthorityRepairAssessment, error) {
	return assessAuthorityRepairAtRepositoryRoot(ctx, root, false)
}

func assessAuthorityRepairAtRepositoryRoot(ctx context.Context, root string, maintenanceOwned bool) (AuthorityRepairAssessment, error) {
	base, binding, err := authorityRepairRoot(root)
	if err != nil {
		return AuthorityRepairAssessment{}, err
	}
	scan := authorityRepairScan{
		assessment: AuthorityRepairAssessment{
			Schema: AuthorityRepairAssessmentSchema, Status: AuthorityRepairUnsupported,
			Counts: AuthorityRepairCounts{}, SupportedOperations: append([]string(nil), authorityRepairSupportedOperations...),
			AuthorizationSchema: AuthorityRepairAuthorizationSchema,
		},
		base: base, binding: binding, maintenanceOwned: maintenanceOwned,
		compact: map[string]CompactRecord{}, legacy: map[string]struct{}{},
	}
	if err := scan.run(ctx); err != nil {
		return AuthorityRepairAssessment{}, err
	}
	scan.finish()
	if err := scan.assessment.Validate(); err != nil {
		return AuthorityRepairAssessment{}, err
	}
	return scan.assessment, nil
}

func UnsupportedAuthorityRepairAssessment() AuthorityRepairAssessment {
	return AuthorityRepairAssessment{
		Schema: AuthorityRepairAssessmentSchema, Status: AuthorityRepairUnsupported,
		Counts: AuthorityRepairCounts{}, SupportedOperations: append([]string(nil), authorityRepairSupportedOperations...),
		AuthorizationSchema: AuthorityRepairAuthorizationSchema,
	}
}

func (scan *authorityRepairScan) run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Stat(scan.base); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	budget := &authorityRepairBudget{
		ctx: ctx, counts: &scan.assessment.Counts,
		uniqueFiles: map[string]struct{}{}, uniqueEvents: map[string]struct{}{},
	}
	if !scan.maintenanceOwned {
		if status := probeAuthorityRepairLock(compactMaintenanceLockPath(scan.base)); status != authorityRepairLockReleased {
			if status != authorityRepairLockAbsent {
				scan.conflicts++
			}
		}
	}
	if _, err := os.Lstat(compactBatchReconcileMarkerPath(scan.base)); err == nil {
		scan.conflicts++
	} else if !os.IsNotExist(err) {
		scan.conflicts++
	}
	if err := scan.scanCompact(ctx, budget); errors.Is(err, errAuthorityRepairTruncated) {
		scan.truncated = true
	} else if err != nil {
		return err
	}
	if !scan.truncated {
		if err := scan.scanLegacy(ctx, budget); errors.Is(err, errAuthorityRepairTruncated) {
			scan.truncated = true
		} else if err != nil {
			return err
		}
	}
	for lineage := range scan.legacy {
		if _, exists := scan.compact[lineage]; exists {
			scan.conflicts++
		}
	}
	return nil
}

func (scan *authorityRepairScan) scanCompact(ctx context.Context, budget *authorityRepairBudget) error {
	root := filepath.Join(scan.base, "v2")
	entries, err := readAuthorityRepairDirectory(root, authorityRepairMaxLineages)
	if err != nil {
		if errors.Is(err, errAuthorityRepairTruncated) {
			return err
		}
		if os.IsNotExist(err) {
			return nil
		}
		scan.unsupported++
		return nil
	}
	if status := probeAuthorityRepairLock(filepath.Join(root, "LOCK")); status != authorityRepairLockReleased && status != authorityRepairLockAbsent {
		scan.conflicts++
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Name() == "LOCK" && !entry.IsDir() {
			continue
		}
		if err := budget.addEntry(); err != nil {
			return err
		}
		if !entry.IsDir() || validateLineageID(entry.Name()) != nil {
			scan.unsupported++
			continue
		}
		budget.addLineage(true)
		lineage := entry.Name()
		dir := filepath.Join(root, lineage)
		residue, residueErr := authorityRepairHasAtomicResidue(dir)
		if errors.Is(residueErr, errAuthorityRepairTruncated) {
			return residueErr
		}
		if residueErr != nil || residue {
			scan.unsupported++
			continue
		}
		payload, readErr := budget.read(filepath.Join(dir, compactStateFileName), authorityRepairMaxEventBytes)
		if readErr != nil {
			if errors.Is(readErr, errAuthorityRepairTruncated) {
				return readErr
			}
			scan.unsupported++
			continue
		}
		record, parseErr := parseCompactRecord(payload, lineage)
		if parseErr != nil {
			scan.unsupported++
			continue
		}
		if !scan.validCompactReceipt(dir, record, budget) {
			if budget.truncated {
				return errAuthorityRepairTruncated
			}
			scan.unsupported++
			continue
		}
		scan.compact[lineage] = record
	}
	// One count per lineage that actually carries a graph defect. Collapsing
	// the whole graph into a single increment reported `unsupported_lineages:
	// 1` while two independent edges were invalid, which understated the work
	// an operator still had to do and made the assessment disagree with
	// `review inspect-authority`.
	violations, _ := compactAuthorityGraphViolations(scan.compact)
	scan.unsupported += len(violations)
	return nil
}

func (scan *authorityRepairScan) validCompactReceipt(dir string, record CompactRecord, budget *authorityRepairBudget) bool {
	path := filepath.Join(dir, compactReceiptFileName)
	payload, err := budget.readOptional(path, authorityRepairMaxEventBytes)
	if err != nil {
		return false
	}
	if payload == nil {
		return record.State.State != StateApproved && record.State.State != StateEscalated
	}
	receipt, parseErr := ParseCompactReceipt(payload)
	authoritative, authorityErr := record.State.Receipt()
	return parseErr == nil && authorityErr == nil && compactReceiptEqual(receipt, authoritative)
}

func (scan *authorityRepairScan) scanLegacy(ctx context.Context, budget *authorityRepairBudget) error {
	root := filepath.Join(scan.base, "v1")
	entries, err := readAuthorityRepairDirectory(root, authorityRepairMaxLineages)
	if err != nil {
		if errors.Is(err, errAuthorityRepairTruncated) {
			return err
		}
		if os.IsNotExist(err) {
			return nil
		}
		scan.unsupported++
		return nil
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := budget.addEntry(); err != nil {
			return err
		}
		if !entry.IsDir() || validateLineageID(entry.Name()) != nil {
			scan.unsupported++
			continue
		}
		budget.addLineage(false)
		lineage := entry.Name()
		dir := filepath.Join(root, lineage)
		scan.legacy[lineage] = struct{}{}
		residue, residueErr := authorityRepairHasAtomicResidue(dir)
		if errors.Is(residueErr, errAuthorityRepairTruncated) {
			return residueErr
		}
		if residueErr != nil || residue {
			scan.unsupported++
			continue
		}
		if status := probeAuthorityRepairLock(filepath.Join(dir, "LOCK")); status != authorityRepairLockReleased && status != authorityRepairLockAbsent {
			scan.conflicts++
		}
		candidate, final, _, _, classifyErr := scanLegacyAuthorityRepair(ctx, dir, lineage, budget)
		if errors.Is(classifyErr, errAuthorityRepairTruncated) {
			return classifyErr
		}
		if errors.Is(classifyErr, errAuthorityRepairConcurrent) {
			scan.conflicts++
			continue
		}
		if classifyErr != nil || !scan.validLegacyReceipt(dir, final, budget) {
			if budget.truncated {
				return errAuthorityRepairTruncated
			}
			scan.unsupported++
			continue
		}
		if candidate != nil {
			scan.candidates = append(scan.candidates, *candidate)
		}
	}
	return nil
}

func authorityRepairHasAtomicResidue(dir string) (bool, error) {
	entries, err := readAuthorityRepairDirectory(dir, authorityRepairMaxOperations)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".atomic-") {
			return true, nil
		}
	}
	return false, nil
}

func readAuthorityRepairDirectory(dir string, limit int) ([]os.DirEntry, error) {
	file, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	entries, err := file.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > limit {
		return nil, errAuthorityRepairTruncated
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func (scan *authorityRepairScan) validLegacyReceipt(dir string, transaction Transaction, budget *authorityRepairBudget) bool {
	path := filepath.Join(dir, "artifacts", "receipt.json")
	payload, err := budget.readOptional(path, authorityRepairMaxEventBytes)
	if err != nil {
		return false
	}
	if payload == nil {
		return true
	}
	receipt, parseErr := ParseReceipt(payload)
	authoritative, authorityErr := transaction.Receipt()
	return parseErr == nil && authorityErr == nil && reflect.DeepEqual(receipt, authoritative)
}

func (scan *authorityRepairScan) finish() {
	sort.Slice(scan.candidates, func(i, j int) bool { return scan.candidates[i].LineageID < scan.candidates[j].LineageID })
	scan.assessment.Counts.EligibleCandidates = len(scan.candidates)
	scan.assessment.Counts.UnsupportedLineages = scan.unsupported
	scan.assessment.Counts.Conflicts = scan.conflicts
	switch {
	case scan.truncated:
		scan.assessment.Status = AuthorityRepairTruncated
	case scan.conflicts > 0:
		scan.assessment.Status = AuthorityRepairConflicting
	case len(scan.candidates) > 1 || len(scan.candidates) == 1 && scan.unsupported > 0:
		scan.assessment.Status = AuthorityRepairAmbiguous
	case len(scan.candidates) == 1:
		scan.assessment.Status = AuthorityRepairEligible
		scan.assessment.Class = AuthorityRepairClassLegacyV1HistoricalAlias
		scan.assessment.Cause = AuthorityRepairCauseUnsupportedHistoricalV1OperationAlias
		scan.assessment.Disposition = AuthorityRepairDispositionQuarantineHistoricalAlias
		scan.assessment.RepositoryBinding = scan.binding
		candidate := scan.candidates[0]
		scan.assessment.Candidate = &candidate
	default:
		scan.assessment.Status = AuthorityRepairUnsupported
	}
}

func scanLegacyAuthorityRepair(ctx context.Context, dir, lineage string, budget *authorityRepairBudget) (*AuthorityRepairCandidate, Transaction, []string, []string, error) {
	headOne, err := readAuthorityRepairHead(dir, budget)
	if err != nil {
		return nil, Transaction{}, nil, nil, err
	}
	first, firstFinal, firstAliases, firstAliasOperations, err := scanLegacyAuthorityRepairOnce(ctx, dir, lineage, headOne, budget)
	if err != nil {
		return nil, Transaction{}, nil, nil, err
	}
	headTwo, err := readAuthorityRepairHead(dir, budget)
	if err != nil {
		return nil, Transaction{}, nil, nil, err
	}
	second, secondFinal, secondAliases, secondAliasOperations, err := scanLegacyAuthorityRepairOnce(ctx, dir, lineage, headTwo, budget)
	if err != nil {
		return nil, Transaction{}, nil, nil, err
	}
	headThree, err := readAuthorityRepairHead(dir, budget)
	if err != nil {
		return nil, Transaction{}, nil, nil, err
	}
	if headOne != headTwo || headTwo != headThree || !reflect.DeepEqual(first, second) ||
		!reflect.DeepEqual(firstAliases, secondAliases) || !reflect.DeepEqual(firstAliasOperations, secondAliasOperations) ||
		!transactionsEqual(firstFinal, secondFinal) {
		return nil, Transaction{}, nil, nil, errAuthorityRepairConcurrent
	}
	if first.AliasEventCount == 0 {
		return nil, firstFinal, nil, nil, nil
	}
	return &first, firstFinal, firstAliases, firstAliasOperations, nil
}

func scanLegacyAuthorityRepairOnce(ctx context.Context, dir, lineage, head string, budget *authorityRepairBudget) (AuthorityRepairCandidate, Transaction, []string, []string, error) {
	visited := map[string]bool{}
	reverseRecords := []Record{}
	reverseRevisions := []string{}
	for revision := head; revision != ""; {
		if err := ctx.Err(); err != nil {
			return AuthorityRepairCandidate{}, Transaction{}, nil, nil, err
		}
		if visited[revision] {
			return AuthorityRepairCandidate{}, Transaction{}, nil, nil, errors.New("legacy repair chain cycle")
		}
		visited[revision] = true
		if len(visited) > authorityRepairMaxOperations {
			return AuthorityRepairCandidate{}, Transaction{}, nil, nil, errAuthorityRepairTruncated
		}
		record, err := readAuthorityRepairRecord(dir, revision, budget)
		if err != nil {
			return AuthorityRepairCandidate{}, Transaction{}, nil, nil, err
		}
		reverseRecords = append(reverseRecords, record)
		reverseRevisions = append(reverseRevisions, revision)
		revision = record.PreviousRevision
	}
	records := make([]Record, len(reverseRecords))
	revisions := make([]string, len(reverseRevisions))
	for index := range reverseRecords {
		reverse := len(reverseRecords) - 1 - index
		records[index], revisions[index] = reverseRecords[reverse], reverseRevisions[reverse]
	}
	if len(records) == 0 || !validInitialStoreRecord(records[0]) || records[0].Transaction.LineageID != lineage {
		return AuthorityRepairCandidate{}, Transaction{}, nil, nil, errors.New("legacy repair chain genesis invalid")
	}
	operations := map[string]struct{}{}
	aliasRevisions := []string{}
	aliasOperations := []string{}
	aliasEvents := 0
	for index := 1; index < len(records); index++ {
		if records[index].PreviousRevision != revisions[index-1] {
			return AuthorityRepairCandidate{}, Transaction{}, nil, nil, errors.New("legacy repair chain discontinuous")
		}
		operation := records[index].Operation
		if validatePersistedV1Successor(records[index-1].Transaction, records[index].Transaction, operation, index) == nil {
			continue
		}
		canonical, approved := approvedHistoricalAliasCanonicalOperation(operation)
		if !approved || validateSuccessor(records[index-1].Transaction, records[index].Transaction, canonical) != nil {
			return AuthorityRepairCandidate{}, Transaction{}, nil, nil, errors.New("legacy repair chain unsupported")
		}
		operations[operation] = struct{}{}
		aliasRevisions = append(aliasRevisions, revisions[index])
		aliasOperations = append(aliasOperations, operation)
		aliasEvents++
	}
	operationSet := make([]string, 0, len(operations))
	for operation := range operations {
		operationSet = append(operationSet, operation)
	}
	sort.Strings(operationSet)
	return AuthorityRepairCandidate{
		LineageID: lineage, Revision: head, ChainIdentity: chainIdentity(revisions), EventCount: len(records),
		AliasEventCount: aliasEvents, Operations: operationSet,
	}, records[len(records)-1].Transaction, aliasRevisions, aliasOperations, nil
}

func readAuthorityRepairRecord(dir, revision string, budget *authorityRepairBudget) (Record, error) {
	if !validSHA256(revision) {
		return Record{}, errors.New("legacy repair revision invalid")
	}
	payload, err := budget.readEvent(filepath.Join(dir, "events", strings.TrimPrefix(revision, "sha256:")+".json"), authorityRepairMaxEventBytes)
	if err != nil {
		return Record{}, err
	}
	sum := sha256.Sum256(payload)
	if "sha256:"+hex.EncodeToString(sum[:]) != revision {
		return Record{}, errors.New("legacy repair record hash mismatch")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var record Record
	if err := decoder.Decode(&record); err != nil {
		return Record{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Record{}, errors.New("legacy repair record has multiple values")
	}
	if record.Schema != RecordSchema || strings.TrimSpace(record.Operation) == "" || record.Transaction.validate() != nil {
		return Record{}, errors.New("legacy repair record invalid")
	}
	return record, nil
}

func readAuthorityRepairHead(dir string, budget *authorityRepairBudget) (string, error) {
	payload, err := budget.read(filepath.Join(dir, "HEAD"), 128)
	if err != nil {
		return "", err
	}
	if bytes.Contains(payload, []byte{'\r'}) || bytes.Count(payload, []byte{'\n'}) > 1 {
		return "", errors.New("legacy repair HEAD invalid")
	}
	revision := strings.TrimSuffix(string(payload), "\n")
	if !validSHA256(revision) {
		return "", errors.New("legacy repair HEAD invalid")
	}
	return revision, nil
}

func (budget *authorityRepairBudget) addEntry() error {
	budget.entries++
	if budget.entries > authorityRepairMaxLineages {
		budget.truncated = true
		return errAuthorityRepairTruncated
	}
	return nil
}

func (budget *authorityRepairBudget) addLineage(compact bool) {
	budget.counts.Lineages++
	if compact {
		budget.counts.CompactLineages++
	} else {
		budget.counts.LegacyLineages++
	}
}

func (budget *authorityRepairBudget) read(path string, limit int64) ([]byte, error) {
	return budget.readKind(path, limit, false)
}

func (budget *authorityRepairBudget) readEvent(path string, limit int64) ([]byte, error) {
	return budget.readKind(path, limit, true)
}

func (budget *authorityRepairBudget) readKind(path string, limit int64, event bool) ([]byte, error) {
	if err := budget.ctx.Err(); err != nil {
		return nil, err
	}
	budget.proofReads++
	if budget.proofReads > authorityRepairMaxProofReads {
		budget.truncated = true
		return nil, errAuthorityRepairTruncated
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	_, uniqueFile := budget.uniqueFiles[path]
	_, uniqueEvent := budget.uniqueEvents[path]
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit ||
		budget.proofBytes+int(info.Size()) > authorityRepairMaxProofReadBytes ||
		!uniqueFile && int64(budget.counts.Bytes)+info.Size() > authorityRepairMaxTotalBytes ||
		event && !uniqueEvent && budget.counts.Events+1 > authorityRepairMaxEvents {
		budget.truncated = true
		return nil, errAuthorityRepairTruncated
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) != info.Size() {
		return nil, errAuthorityRepairConcurrent
	}
	budget.proofBytes += len(payload)
	if !uniqueFile {
		budget.uniqueFiles[path] = struct{}{}
		budget.counts.Bytes += len(payload)
	}
	if event && !uniqueEvent {
		budget.uniqueEvents[path] = struct{}{}
		budget.counts.Events++
	}
	return payload, nil
}

func (budget *authorityRepairBudget) readOptional(path string, limit int64) ([]byte, error) {
	payload, err := budget.read(path, limit)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return payload, err
}

type authorityRepairLockStatus uint8

const (
	authorityRepairLockAbsent authorityRepairLockStatus = iota
	authorityRepairLockReleased
	authorityRepairLockHeld
	authorityRepairLockUnknown
)

func probeAuthorityRepairLock(path string) authorityRepairLockStatus {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return authorityRepairLockAbsent
		}
		return authorityRepairLockUnknown
	}
	defer file.Close()
	locked, err := tryLockFile(file)
	if err != nil {
		return authorityRepairLockUnknown
	}
	if !locked {
		return authorityRepairLockHeld
	}
	if err := unlockFile(file); err != nil {
		return authorityRepairLockUnknown
	}
	return authorityRepairLockReleased
}

func authorityRepairRoot(root string) (string, string, error) {
	repository, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	repository, err = filepath.EvalSymlinks(repository)
	if err != nil {
		return "", "", err
	}
	dotGit := filepath.Join(repository, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return "", "", err
	}
	common := dotGit
	if !info.IsDir() {
		payload, err := os.ReadFile(dotGit)
		if err != nil || len(payload) > 4096 || bytes.Contains(payload, []byte{'\r'}) {
			return "", "", errors.New("repository worktree metadata is invalid")
		}
		line := strings.TrimSuffix(string(payload), "\n")
		if !strings.HasPrefix(line, "gitdir: ") || strings.Contains(line, "\n") {
			return "", "", errors.New("repository worktree metadata is invalid")
		}
		gitDir := strings.TrimPrefix(line, "gitdir: ")
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(repository, gitDir)
		}
		gitDir, err = filepath.EvalSymlinks(gitDir)
		if err != nil {
			return "", "", err
		}
		common = gitDir
		if payload, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
			if len(payload) > 4096 || bytes.Contains(payload, []byte{'\r'}) {
				return "", "", errors.New("repository common-directory metadata is invalid")
			}
			value := strings.TrimSuffix(string(payload), "\n")
			if value == "" || strings.Contains(value, "\n") {
				return "", "", errors.New("repository common-directory metadata is invalid")
			}
			common = value
			if !filepath.IsAbs(common) {
				common = filepath.Join(gitDir, common)
			}
			common, err = filepath.EvalSymlinks(common)
			if err != nil {
				return "", "", err
			}
		} else if !os.IsNotExist(err) {
			return "", "", err
		}
	}
	common, err = filepath.Abs(common)
	if err != nil {
		return "", "", err
	}
	common = filepath.Clean(common)
	info, err = os.Stat(common)
	if err != nil || !info.IsDir() {
		return "", "", errors.New("repository common directory is invalid")
	}
	sum := sha256.Sum256([]byte("gentle-ai.review-repository-binding/v1\n" + common))
	binding := "sha256:" + hex.EncodeToString(sum[:])
	return filepath.Join(common, "gentle-ai", "review-transactions"), binding, nil
}

func (assessment AuthorityRepairAssessment) Validate() error {
	if assessment.Schema != AuthorityRepairAssessmentSchema || assessment.AuthorizationSchema != AuthorityRepairAuthorizationSchema ||
		!reflect.DeepEqual(assessment.SupportedOperations, authorityRepairSupportedOperations) {
		return errors.New("authority repair assessment identity is invalid")
	}
	counts := assessment.Counts
	for _, value := range []int{counts.Lineages, counts.CompactLineages, counts.LegacyLineages, counts.Events, counts.Bytes, counts.EligibleCandidates, counts.UnsupportedLineages, counts.Conflicts} {
		if value < 0 {
			return errors.New("authority repair assessment count is invalid")
		}
	}
	if counts.Lineages > authorityRepairMaxLineages || counts.Events > authorityRepairMaxEvents || counts.Bytes > authorityRepairMaxTotalBytes ||
		counts.UnsupportedLineages > authorityRepairMaxOperations || counts.Conflicts > authorityRepairMaxOperations ||
		counts.CompactLineages+counts.LegacyLineages != counts.Lineages || counts.EligibleCandidates > counts.LegacyLineages {
		return errors.New("authority repair assessment count exceeds its bound")
	}
	if assessment.Status == AuthorityRepairEligible {
		candidate := assessment.Candidate
		if assessment.Class != AuthorityRepairClassLegacyV1HistoricalAlias || assessment.Cause != AuthorityRepairCauseUnsupportedHistoricalV1OperationAlias ||
			assessment.Disposition != AuthorityRepairDispositionQuarantineHistoricalAlias || !validSHA256(assessment.RepositoryBinding) ||
			candidate == nil || validateLineageID(candidate.LineageID) != nil || !validSHA256(candidate.Revision) || !validSHA256(candidate.ChainIdentity) ||
			candidate.EventCount < 2 || candidate.EventCount > authorityRepairMaxEvents || candidate.AliasEventCount < 1 || candidate.AliasEventCount > candidate.EventCount ||
			len(candidate.Operations) == 0 || !repairOperationsSupported(candidate.Operations) || counts.EligibleCandidates != 1 || counts.UnsupportedLineages != 0 || counts.Conflicts != 0 {
			return errors.New("eligible authority repair assessment is incomplete")
		}
		return nil
	}
	if assessment.Status != AuthorityRepairUnsupported && assessment.Status != AuthorityRepairAmbiguous && assessment.Status != AuthorityRepairConflicting && assessment.Status != AuthorityRepairTruncated {
		return errors.New("authority repair assessment status is invalid")
	}
	if assessment.Class != "" || assessment.Cause != "" || assessment.Disposition != "" || assessment.RepositoryBinding != "" || assessment.Candidate != nil {
		return errors.New("stopped authority repair assessment contains an executable candidate")
	}
	return nil
}

func repairOperationsSupported(operations []string) bool {
	if len(operations) == 0 {
		return false
	}
	previous := ""
	for _, operation := range operations {
		if operation <= previous || !stringInSlice(authorityRepairSupportedOperations, operation) {
			return false
		}
		previous = operation
	}
	return true
}

func authorityRepairAuthorizationBinding(request ClassifiedAuthorityRepairRequest) string {
	return AuthorityRepairAuthorizationSchema +
		"\nrepository=" + request.RepositoryBinding +
		"\nclass=" + string(request.Class) +
		"\nlineage=" + request.LineageID +
		"\nrevision=" + request.ExpectedRevision +
		"\ncause=" + string(request.Cause) +
		"\ndisposition=" + string(request.Disposition) +
		"\nactor=" + request.Actor +
		"\nreason=" + request.Reason
}

func validateClassifiedAuthorityRepairRequest(request ClassifiedAuthorityRepairRequest, assessment AuthorityRepairAssessment) error {
	if assessment.Status != AuthorityRepairEligible || assessment.Candidate == nil {
		return errors.New("classified authority repair is not eligible")
	}
	if request.Class != assessment.Class || request.LineageID != assessment.Candidate.LineageID || request.ExpectedRevision != assessment.Candidate.Revision ||
		request.Cause != assessment.Cause || request.Disposition != assessment.Disposition || request.RepositoryBinding != assessment.RepositoryBinding {
		return errors.New("classified authority repair inputs do not match the current assessment")
	}
	if strings.TrimSpace(request.Actor) != request.Actor || strings.TrimSpace(request.Reason) != request.Reason ||
		request.Actor == "" || request.Reason == "" || len(request.Actor) > 256 || len(request.Reason) > 512 ||
		strings.ContainsAny(request.Actor, "\r\n") || strings.ContainsAny(request.Reason, "\r\n") || strings.Contains(request.MaintainerAuthorization, "\r") ||
		request.MaintainerAuthorization != authorityRepairAuthorizationBinding(request) {
		return errors.New("classified authority repair authorization is not exact")
	}
	return nil
}

func ValidateClassifiedAuthorityRepairRequest(request ClassifiedAuthorityRepairRequest, assessment AuthorityRepairAssessment) error {
	return validateClassifiedAuthorityRepairRequest(request, assessment)
}

// RepairClassifiedAuthority executes only the provider-owned repair class
// returned by the shared assessment. Exact committed retries replay the same
// path-free audit result without reopening or rewriting quarantined history.
func RepairClassifiedAuthority(ctx context.Context, repo string, request ClassifiedAuthorityRepairRequest) (ClassifiedAuthorityRepairExecution, error) {
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return ClassifiedAuthorityRepairExecution{}, err
	}
	base, repositoryBinding, err := authorityRepairRoot(root)
	if err != nil {
		return ClassifiedAuthorityRepairExecution{}, err
	}
	if err := validateClassifiedAuthorityRepairRequestStatic(request, repositoryBinding); err != nil {
		return ClassifiedAuthorityRepairExecution{}, err
	}
	assessment, err := AssessAuthorityRepairAtRepositoryRoot(ctx, root)
	if err != nil {
		return ClassifiedAuthorityRepairExecution{}, err
	}
	initiallyMatched := validateClassifiedAuthorityRepairRequest(request, assessment) == nil
	if _, _, err := discoverClassifiedAuthorityRepairRecord(ctx, base, request); err != nil {
		return ClassifiedAuthorityRepairExecution{}, err
	}
	classifiedAuthorityRepairAfterAssessmentHook()
	maintenance, err := acquireMaintenanceLock(ctx, compactMaintenanceLockPath(base), maintenanceExclusive)
	if err != nil {
		return ClassifiedAuthorityRepairExecution{}, err
	}
	defer maintenance.Release()
	assessment, err = assessAuthorityRepairAtRepositoryRoot(ctx, root, true)
	if err != nil {
		return ClassifiedAuthorityRepairExecution{}, err
	}
	if replay, found, replayErr := discoverClassifiedAuthorityRepairRecord(ctx, base, request); replayErr != nil {
		return ClassifiedAuthorityRepairExecution{}, replayErr
	} else if found {
		record, reconcileErr := reconcileClassifiedAuthorityRepairRecord(ctx, base, request, assessment, replay)
		execution, progressErr := classifiedAuthorityRepairExecution(request, record)
		if progressErr != nil {
			return ClassifiedAuthorityRepairExecution{}, errors.Join(reconcileErr, progressErr)
		}
		if reconcileErr != nil {
			return execution, &ClassifiedAuthorityRepairProgressError{Progress: execution, ExactReplaySafe: true, Cause: reconcileErr}
		}
		return execution, nil
	}
	legacyRequest := LegacyAliasRepairRequest{
		LineageID: request.LineageID, ExpectedRevision: request.ExpectedRevision,
		ExpectedDiagnostic: legacyAliasRepairDiagnostic, Disposition: LegacyAliasRepairDisposition,
		Reason: request.Reason, Actor: request.Actor,
	}
	legacyRequest.MaintainerAuthorization = legacyAliasRepairAuthorizationBinding(
		root, legacyRequest.LineageID, legacyRequest.ExpectedRevision, legacyRequest.ExpectedDiagnostic,
		legacyRequest.Disposition, legacyRequest.Actor, legacyRequest.Reason,
	)
	if assessment.Status != AuthorityRepairEligible {
		if initiallyMatched {
			return ClassifiedAuthorityRepairExecution{}, fmt.Errorf("classified authority repair changed before exclusive maintenance ownership: %w", ErrConcurrentUpdate)
		}
		return ClassifiedAuthorityRepairExecution{}, errors.New("classified authority repair is not eligible")
	}
	if err := validateClassifiedAuthorityRepairRequest(request, assessment); err != nil {
		if initiallyMatched {
			return ClassifiedAuthorityRepairExecution{}, fmt.Errorf("classified authority repair changed before exclusive maintenance ownership: %w", ErrConcurrentUpdate)
		}
		return ClassifiedAuthorityRepairExecution{}, err
	}
	audit, err := newClassifiedAuthorityRepairAudit(assessment, request)
	if err != nil {
		return ClassifiedAuthorityRepairExecution{}, err
	}
	record, repairErr := repairHistoricalLegacyAlias(ctx, root, legacyRequest, legacyAliasRepairOptions{
		waitForCooperativeRepair: true, maintenanceAlreadyHeld: true, classifiedAudit: audit,
	})
	execution, progressErr := classifiedAuthorityRepairExecution(request, record)
	if repairErr != nil {
		if progressErr == nil {
			if execution.Status == CompactReclaimCommitted {
				if verified, verifyErr := readBackClassifiedAuthorityRepair(ctx, root, base, request); verifyErr == nil {
					execution = verified
				} else {
					repairErr = errors.Join(repairErr, verifyErr)
				}
			}
			return execution, &ClassifiedAuthorityRepairProgressError{Progress: execution, ExactReplaySafe: true, Cause: repairErr}
		}
		return ClassifiedAuthorityRepairExecution{}, repairErr
	}
	if progressErr != nil {
		return ClassifiedAuthorityRepairExecution{}, progressErr
	}
	verified, err := readBackClassifiedAuthorityRepair(ctx, root, base, request)
	if err != nil {
		return execution, &ClassifiedAuthorityRepairProgressError{Progress: execution, ExactReplaySafe: true, Cause: err}
	}
	return verified, nil
}

func readBackClassifiedAuthorityRepair(ctx context.Context, root, base string, request ClassifiedAuthorityRepairRequest) (ClassifiedAuthorityRepairExecution, error) {
	readback, err := assessAuthorityRepairAtRepositoryRoot(ctx, root, true)
	if err != nil {
		return ClassifiedAuthorityRepairExecution{}, fmt.Errorf("read back classified authority repair inventory: %w", err)
	}
	replay, found, err := discoverClassifiedAuthorityRepairRecord(ctx, base, request)
	if err != nil {
		return ClassifiedAuthorityRepairExecution{}, err
	}
	if !found {
		return ClassifiedAuthorityRepairExecution{}, errors.New("classified authority repair audit readback is missing")
	}
	record, reconcileErr := reconcileClassifiedAuthorityRepairRecord(ctx, base, request, readback, replay)
	verified, verifyErr := classifiedAuthorityRepairExecution(request, record)
	if verifyErr != nil {
		return ClassifiedAuthorityRepairExecution{}, errors.Join(reconcileErr, verifyErr)
	}
	if reconcileErr != nil {
		return verified, reconcileErr
	}
	if verified.Status != CompactReclaimCommitted {
		return verified, errors.New("classified authority repair audit readback is not committed")
	}
	return verified, nil
}

func classifiedAuthorityRepairExecution(request ClassifiedAuthorityRepairRequest, record CompactReclaimRecord) (ClassifiedAuthorityRepairExecution, error) {
	proof := record.LegacyAliasRepair
	if (record.Status != CompactReclaimPrepared && record.Status != CompactReclaimCommitted) || proof == nil ||
		proof.LineageID != request.LineageID || proof.HeadRevision != request.ExpectedRevision ||
		proof.Disposition != string(request.Disposition) || !validSHA256(proof.ChainIdentity) ||
		validateClassifiedAuthorityRepairAudit(record.ClassifiedAuthorityRepair, request) != nil {
		return ClassifiedAuthorityRepairExecution{}, errors.New("classified authority repair audit readback is invalid")
	}
	recordIdentity, err := classifiedAuthorityRepairRecordIdentity(record)
	if err != nil {
		return ClassifiedAuthorityRepairExecution{}, err
	}
	return ClassifiedAuthorityRepairExecution{
		Status: record.Status, Class: request.Class, LineageID: request.LineageID, Revision: request.ExpectedRevision,
		ChainIdentity: proof.ChainIdentity, Cause: request.Cause, Disposition: request.Disposition,
		AssessmentDigest: record.ClassifiedAuthorityRepair.AssessmentDigest,
		RequestDigest:    record.ClassifiedAuthorityRepair.RequestDigest, RecordIdentity: recordIdentity,
	}, nil
}

func validateClassifiedAuthorityRepairRequestStatic(request ClassifiedAuthorityRepairRequest, repositoryBinding string) error {
	if request.Class != AuthorityRepairClassLegacyV1HistoricalAlias || request.Cause != AuthorityRepairCauseUnsupportedHistoricalV1OperationAlias ||
		request.Disposition != AuthorityRepairDispositionQuarantineHistoricalAlias || validateLineageID(request.LineageID) != nil ||
		!validSHA256(request.ExpectedRevision) || request.RepositoryBinding != repositoryBinding ||
		strings.TrimSpace(request.Actor) != request.Actor || strings.TrimSpace(request.Reason) != request.Reason ||
		request.Actor == "" || request.Reason == "" || len(request.Actor) > 256 || len(request.Reason) > 512 ||
		strings.ContainsAny(request.Actor, "\r\n") || strings.ContainsAny(request.Reason, "\r\n") || strings.Contains(request.MaintainerAuthorization, "\r") ||
		request.MaintainerAuthorization != authorityRepairAuthorizationBinding(request) {
		return errors.New("classified authority repair request is invalid")
	}
	return nil
}

func stringInSlice(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// repairAuthorityDispositionAtRepo is the production seam that ties
// disposition plan derivation (authority_disposition_plan.go) to leaf
// executor admission and mutation (authority_disposition_execute.go) behind
// the shape a future `review repair` CLI wiring (Slice S3) calls: derive the
// current plan, stamp the caller-supplied maintainer authorization, and
// execute it. It stays unexported and unwired from any CLI entrypoint this
// slice — no new public repair verb (rdd-authority-disposition-plan / "No
// New Public Repair Verb") — Slice S3 wires it behind the existing
// `review repair` verb.
func repairAuthorityDispositionAtRepo(ctx context.Context, repo, actor, reason, authorization string) (CompactReclaimRecord, error) {
	plan, err := deriveAuthorityDispositionPlanAtRepo(ctx, repo, actor, reason)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	plan.Authorization = authorization
	return executeAuthorityDisposition(ctx, repo, plan)
}

// RepairAuthorityDisposition is the exported form of
// repairAuthorityDispositionAtRepo — the one public entrypoint Slice S3's
// `review repair` CLI wiring calls to execute a leaf authority disposition
// plan (rdd-authority-disposition-plan / "No New Public Repair Verb": an
// exported Go seam behind the existing verb, not a new CLI command).
func RepairAuthorityDisposition(ctx context.Context, repo, actor, reason, authorization string) (CompactReclaimRecord, error) {
	return repairAuthorityDispositionAtRepo(ctx, repo, actor, reason, authorization)
}
