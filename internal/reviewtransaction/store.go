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
	"regexp"
	"runtime"
	"strings"
	"syscall"
)

const RecordSchema = "gentle-ai.review-record/v1"

var ErrConcurrentUpdate = errors.New("review transaction changed concurrently")
var ErrInvalidSuccessor = errors.New("review transaction successor is invalid")

var reviewRuntimeGOOS = func() string { return runtime.GOOS }
var syncReviewDirectory = func(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}

type directorySyncError struct {
	path  string
	cause error
}

func (err *directorySyncError) Error() string {
	return fmt.Sprintf("sync parent directory for %q: %v", err.path, err.cause)
}

func (err *directorySyncError) Unwrap() error { return err.cause }

// ImmutablePublicationConflictError requires maintainer action because replay
// cannot replace a conflicting immutable artifact.
type ImmutablePublicationConflictError struct{ Cause error }

func (err *ImmutablePublicationConflictError) Error() string {
	return fmt.Sprintf("immutable review publication conflict: %v", err.Cause)
}

func (err *ImmutablePublicationConflictError) Unwrap() error { return err.Cause }

// SyncReviewDirectory persists a directory entry when the platform supports it.
// Windows filesystems may reject directory handles; in that case the file rename
// remains atomic, but power-loss durability of the directory entry is not claimed.
func SyncReviewDirectory(path string) error {
	if err := syncReviewDirectory(path); err != nil {
		unsupported := errors.Is(err, syscall.EINVAL) || errors.Is(err, errors.ErrUnsupported) ||
			reviewRuntimeGOOS() == "windows" && errors.Is(err, os.ErrPermission)
		if !unsupported {
			return err
		}
	}
	return nil
}

type Record struct {
	Schema           string      `json:"schema"`
	Operation        string      `json:"operation"`
	PreviousRevision string      `json:"previous_revision"`
	Transaction      Transaction `json:"transaction"`
}

type Store struct {
	Dir       string
	lineageID string
	repo      string
	readOnly  bool
}

type ValidatedChain struct {
	Records         []Record `json:"records"`
	Revisions       []string `json:"revisions"`
	GenesisRevision string   `json:"genesis_revision"`
	HeadRevision    string   `json:"head_revision"`
	Identity        string   `json:"identity"`
}

var lineageIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func AuthoritativeStore(ctx context.Context, repo, lineageID string) (Store, error) {
	if err := validateLineageID(lineageID); err != nil {
		return Store{}, err
	}
	authorityRoot, root, err := authoritativeStoreRoot(ctx, repo)
	if err != nil {
		return Store{}, err
	}
	dir := filepath.Join(authorityRoot, lineageID)
	relative, err := filepath.Rel(authorityRoot, dir)
	if err != nil || relative != lineageID || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return Store{}, errors.New("lineage_id escapes the repository review store")
	}
	_, statErr := os.Stat(filepath.Join(dir, "HEAD"))
	return Store{Dir: dir, lineageID: lineageID, repo: root, readOnly: statErr == nil}, nil
}

// DiscoverAuthoritativeStores returns every canonical lineage rooted in the
// repository Git common directory. Callers still validate each chain before
// treating it as review authority.
func DiscoverAuthoritativeStores(ctx context.Context, repo string) ([]Store, error) {
	authorityRoot, root, err := authoritativeStoreRoot(ctx, repo)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(authorityRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []Store{}, nil
		}
		return nil, err
	}
	stores := make([]Store, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || validateLineageID(entry.Name()) != nil {
			continue
		}
		stores = append(stores, Store{
			Dir: filepath.Join(authorityRoot, entry.Name()), lineageID: entry.Name(), repo: root, readOnly: true,
		})
	}
	return stores, nil
}

func authoritativeStoreRoot(ctx context.Context, repo string) (string, string, error) {
	base, root, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(base, "v1"), root, nil
}

func reviewAuthorityRoot(ctx context.Context, repo string) (string, string, error) {
	root, err := (SnapshotBuilder{Repo: repo}).repositoryRoot(ctx)
	if err != nil {
		return "", "", fmt.Errorf("resolve authoritative review repository: %w", err)
	}
	output, err := runGit(ctx, root, nil, nil, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", "", fmt.Errorf("resolve repository Git common directory: %w", err)
	}
	commonDir := strings.TrimSpace(string(output))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(root, commonDir)
	}
	commonDir, err = filepath.Abs(commonDir)
	if err != nil {
		return "", "", err
	}
	authorityRoot := filepath.Join(filepath.Clean(commonDir), "gentle-ai", "review-transactions")
	return authorityRoot, root, nil
}

func validateLineageID(lineageID string) error {
	if len(lineageID) == 0 || len(lineageID) > 128 || !lineageIDPattern.MatchString(lineageID) {
		return errors.New("lineage_id must be a canonical lowercase kebab-case identifier of at most 128 bytes")
	}
	return nil
}

func (store Store) Append(expectedRevision string, record Record) (string, error) {
	return store.append(expectedRevision, record)
}

func (store Store) append(expectedRevision string, record Record) (string, error) {
	if store.readOnly {
		operation := strings.TrimSpace(record.Operation)
		if operation == "" {
			operation = "review/append"
		}
		return "", NewLegacyReadOnlyError(operation, store.lineageID)
	}
	if strings.TrimSpace(store.Dir) == "" {
		return "", errors.New("review store directory is required")
	}
	if strings.TrimSpace(record.Operation) == "" {
		return "", errors.New("record operation is required")
	}
	if err := record.Transaction.validate(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidSuccessor, err)
	}
	if store.lineageID != "" && record.Transaction.LineageID != store.lineageID {
		return "", fmt.Errorf("%w: transaction lineage does not match authoritative store lineage", ErrInvalidSuccessor)
	}
	if err := os.MkdirAll(filepath.Join(store.Dir, "events"), 0o755); err != nil {
		return "", err
	}
	lockPath := filepath.Join(store.Dir, "LOCK")
	lock, err := acquireStoreLock(lockPath)
	if err != nil {
		return "", err
	}
	defer lock.release()
	record.Schema = RecordSchema
	record.PreviousRevision = expectedRevision
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", err
	}
	payload = append(payload, '\n')
	sum := sha256.Sum256(payload)
	revision := "sha256:" + hex.EncodeToString(sum[:])

	current, err := readRevision(filepath.Join(store.Dir, "HEAD"))
	if err != nil {
		return "", err
	}
	if current == revision {
		if _, err := store.loadChain(current); err != nil {
			return "", err
		}
		existing, err := os.ReadFile(filepath.Join(store.Dir, "events", strings.TrimPrefix(revision, "sha256:")+".json"))
		if err != nil {
			return "", err
		}
		if !bytes.Equal(existing, payload) {
			return "", ErrConcurrentUpdate
		}
		return revision, nil
	}
	if current != expectedRevision {
		return "", fmt.Errorf("%w: expected predecessor %q, current HEAD %q, candidate revision %q", ErrConcurrentUpdate, expectedRevision, current, revision)
	}
	if current == "" {
		if !validInitialStoreRecord(record) {
			return "", fmt.Errorf("%w: first event must be review/start in reviewing state", ErrInvalidSuccessor)
		}
		if record.Transaction.Mode == ModeOrdinaryBounded && !record.Transaction.hasCorrectionBudget() {
			return "", fmt.Errorf("%w: new ordinary_bounded review/start requires correction budget fields", ErrInvalidSuccessor)
		}
		if store.repo != "" {
			builder := SnapshotBuilder{Repo: store.repo}
			if err := builder.ValidateEvidence(context.Background(), record.Transaction.Snapshot); err != nil {
				return "", fmt.Errorf("%w: initial snapshot is not repository-derived: %v", ErrInvalidSuccessor, err)
			}
			if record.Transaction.hasCorrectionBudget() {
				risk, changedLines, err := builder.ClassifySnapshotRisk(context.Background(), record.Transaction.Snapshot)
				if err != nil || changedLines != *record.Transaction.OriginalChangedLines || risk != record.Transaction.RiskLevel {
					return "", fmt.Errorf("%w: original risk inputs do not match repository tree evidence", ErrInvalidSuccessor)
				}
			}
		}
	} else {
		chain, err := store.loadChain(current)
		if err != nil {
			return "", err
		}
		previous := chain.Records[len(chain.Records)-1]
		if previous.Transaction.State == StateFixing && record.Transaction.State == StateFixValidating && store.repo != "" {
			builder := SnapshotBuilder{Repo: store.repo}
			if err := builder.ValidateEvidence(context.Background(), record.Transaction.Snapshot); err != nil {
				return "", fmt.Errorf("%w: correction snapshot is not repository-derived: %v", ErrInvalidSuccessor, err)
			}
			if record.Transaction.hasCorrectionBudget() {
				actualChangedLines, err := builder.ChangedLines(context.Background(), record.Transaction.Snapshot)
				if err != nil || record.Transaction.ActualCorrectionLines == nil || actualChangedLines != *record.Transaction.ActualCorrectionLines {
					return "", fmt.Errorf("%w: actual correction lines do not match repository tree evidence", ErrInvalidSuccessor)
				}
			}
		}
		if err := validateSuccessor(previous.Transaction, record.Transaction, record.Operation); err != nil {
			return "", err
		}
	}
	temp, err := os.CreateTemp(filepath.Join(store.Dir, "events"), ".event-*")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(payload); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	eventPath := filepath.Join(store.Dir, "events", strings.TrimPrefix(revision, "sha256:")+".json")
	if err := publishNoReplace(tempPath, eventPath); err != nil {
		if os.IsExist(err) {
			existing, readErr := os.ReadFile(eventPath)
			if readErr != nil {
				return "", readErr
			}
			if !reflect.DeepEqual(existing, payload) {
				return "", errors.New("existing content-addressed review event does not match its revision")
			}
		} else {
			return "", err
		}
	}
	if err := writeAtomic(filepath.Join(store.Dir, "HEAD"), []byte(revision+"\n"), 0o644); err != nil {
		return "", err
	}
	return revision, nil
}

// InvalidatePristine is retained as a compatibility entry point, but ordinary
// legacy-v1 authority is immutable once created.
func (store Store) InvalidatePristine(expectedRevision, reason string, snapshot Snapshot) (string, error) {
	return "", NewLegacyReadOnlyError("review/invalidate", store.lineageID)
}

func (store Store) Load() (Record, string, error) {
	chain, err := store.LoadChain()
	if err != nil {
		return Record{}, "", err
	}
	return chain.Records[len(chain.Records)-1], chain.HeadRevision, nil
}

func (store Store) LoadChain() (ValidatedChain, error) {
	revision, err := readRevision(filepath.Join(store.Dir, "HEAD"))
	if err != nil {
		return ValidatedChain{}, err
	}
	if revision == "" {
		return ValidatedChain{}, os.ErrNotExist
	}
	return store.loadChain(revision)
}

func (store Store) loadChain(headRevision string) (ValidatedChain, error) {
	if !validSHA256(headRevision) {
		return ValidatedChain{}, errors.New("review chain HEAD revision is invalid")
	}
	visited := map[string]struct{}{}
	reverseRecords := []Record{}
	reverseRevisions := []string{}
	for revision := headRevision; revision != ""; {
		if _, seen := visited[revision]; seen {
			return ValidatedChain{}, errors.New("review record predecessor cycle detected")
		}
		visited[revision] = struct{}{}
		record, loadedRevision, err := store.loadRevision(revision)
		if err != nil {
			return ValidatedChain{}, fmt.Errorf("load review chain revision %s: %w", revision, err)
		}
		reverseRecords = append(reverseRecords, record)
		reverseRevisions = append(reverseRevisions, loadedRevision)
		if record.PreviousRevision != "" && !validSHA256(record.PreviousRevision) {
			return ValidatedChain{}, errors.New("review record predecessor revision is invalid")
		}
		revision = record.PreviousRevision
	}

	records := make([]Record, len(reverseRecords))
	revisions := make([]string, len(reverseRevisions))
	for index := range reverseRecords {
		reverseIndex := len(reverseRecords) - 1 - index
		records[index] = reverseRecords[reverseIndex]
		revisions[index] = reverseRevisions[reverseIndex]
	}
	if len(records) == 0 || !validInitialStoreRecord(records[0]) {
		return ValidatedChain{}, errors.New("review chain must have exactly one valid review/start genesis")
	}
	if store.lineageID != "" && records[0].Transaction.LineageID != store.lineageID {
		return ValidatedChain{}, errors.New("review chain genesis lineage does not match authoritative store lineage")
	}
	for index := 1; index < len(records); index++ {
		if records[index].PreviousRevision != revisions[index-1] {
			return ValidatedChain{}, errors.New("review chain predecessor revision is discontinuous")
		}
		if err := validatePersistedV1Successor(records[index-1].Transaction, records[index].Transaction, records[index].Operation, index); err != nil {
			return ValidatedChain{}, fmt.Errorf("review chain successor %s: %w", revisions[index], err)
		}
	}
	if store.repo != "" {
		if err := validateRepositoryBudgetEvidence(context.Background(), store.repo, records); err != nil {
			return ValidatedChain{}, err
		}
	}

	chain := ValidatedChain{
		Records: records, Revisions: revisions,
		GenesisRevision: revisions[0], HeadRevision: headRevision,
	}
	chain.Identity = chainIdentity(revisions)
	return chain, nil
}

func validateRepositoryBudgetEvidence(ctx context.Context, repo string, records []Record) error {
	if len(records) == 0 || !records[0].Transaction.hasCorrectionBudget() {
		return nil
	}
	builder := SnapshotBuilder{Repo: repo}
	genesis := records[0].Transaction
	risk, changedLines, err := builder.ClassifySnapshotRisk(ctx, genesis.Snapshot)
	if err != nil || changedLines != *genesis.OriginalChangedLines || risk != genesis.RiskLevel {
		return errors.New("original risk inputs do not match repository tree evidence")
	}
	for index := 1; index < len(records); index++ {
		previous, next := records[index-1].Transaction, records[index].Transaction
		if previous.State != StateFixing || next.State != StateFixValidating {
			continue
		}
		actual, countErr := builder.ChangedLines(ctx, next.Snapshot)
		if countErr != nil || next.ActualCorrectionLines == nil || actual != *next.ActualCorrectionLines {
			return errors.New("actual correction lines do not match repository tree evidence")
		}
	}
	return nil
}

func (store Store) loadRevision(revision string) (Record, string, error) {
	if !validSHA256(revision) {
		return Record{}, "", errors.New("review record revision is invalid")
	}
	path := filepath.Join(store.Dir, "events", strings.TrimPrefix(revision, "sha256:")+".json")
	payload, err := os.ReadFile(path)
	if err != nil {
		return Record{}, "", err
	}
	sum := sha256.Sum256(payload)
	if got := "sha256:" + hex.EncodeToString(sum[:]); got != revision {
		return Record{}, "", errors.New("review record hash mismatch")
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	var record Record
	if err := decoder.Decode(&record); err != nil {
		return Record{}, "", err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Record{}, "", errors.New("multiple JSON values in review record")
	}
	if record.Schema != RecordSchema || strings.TrimSpace(record.Operation) == "" {
		return Record{}, "", errors.New("invalid review record")
	}
	if err := record.Transaction.validate(); err != nil {
		return Record{}, "", err
	}
	return record, revision, nil
}

func validateSuccessor(previous, next Transaction, operation string) error {
	if operation == "review/invalidate" {
		expected := previous
		if err := expected.Invalidate(next.InvalidationReason); err != nil || !transactionsEqual(expected, next) {
			return fmt.Errorf("%w: invalidation must retain a pristine reviewing authority", ErrInvalidSuccessor)
		}
		return nil
	}
	if previous.LineageID != next.LineageID || previous.Generation != next.Generation || previous.Mode != next.Mode {
		return fmt.Errorf("%w: lineage, generation, and mode are immutable", ErrInvalidSuccessor)
	}
	if previous.BaseTree != next.BaseTree || previous.InitialReviewTree != next.InitialReviewTree || previous.PathsDigest != next.PathsDigest || previous.PolicyHash != next.PolicyHash {
		return fmt.Errorf("%w: initial target and policy are immutable", ErrInvalidSuccessor)
	}
	if !equalStrings(previous.GenesisPaths, next.GenesisPaths) {
		return fmt.Errorf("%w: immutable genesis paths changed", ErrInvalidSuccessor)
	}
	if !equalStrings(previous.SelectedLenses, next.SelectedLenses) {
		return fmt.Errorf("%w: selected lenses are immutable", ErrInvalidSuccessor)
	}
	if previous.RiskLevel != next.RiskLevel {
		return fmt.Errorf("%w: native risk classification is immutable", ErrInvalidSuccessor)
	}
	if !equalOptionalInt(previous.OriginalChangedLines, next.OriginalChangedLines) || !equalOptionalInt(previous.CorrectionBudget, next.CorrectionBudget) {
		return fmt.Errorf("%w: original changed lines and correction budget are immutable", ErrInvalidSuccessor)
	}
	lensStateChanged := !reflect.DeepEqual(previous.LensResults, next.LensResults) ||
		previous.Counters.RiskExecutions != next.Counters.RiskExecutions ||
		previous.Counters.ResilienceExecutions != next.Counters.ResilienceExecutions ||
		previous.Counters.ReadabilityExecutions != next.Counters.ReadabilityExecutions ||
		previous.Counters.ReliabilityExecutions != next.Counters.ReliabilityExecutions
	lensTransition := operation == "review/record-lens-result" && previous.Mode == ModeOrdinaryBounded && previous.State == StateReviewing && next.State == StateReviewing
	if lensTransition {
		if len(next.LensResults) != len(previous.LensResults)+1 {
			return fmt.Errorf("%w: lens result transition must append exactly one result", ErrInvalidSuccessor)
		}
		expected := previous
		if err := expected.RecordLensResult(next.LensResults[len(next.LensResults)-1]); err != nil || !transactionsEqual(expected, next) {
			return fmt.Errorf("%w: lens result transition changed unrelated transaction state", ErrInvalidSuccessor)
		}
	} else if lensStateChanged || operation == "review/record-lens-result" {
		return fmt.Errorf("%w: lens state changed outside the native lens result transition", ErrInvalidSuccessor)
	}
	freezeTransition := (previous.State == StateReviewing || previous.State == StateJudgesConfirmed) && next.State == StateFindingsFrozen
	if freezeTransition {
		expected := previous
		ledger, err := CanonicalLedger(next.Findings)
		if operation != "review/freeze-findings" || err != nil || expected.FreezeFindings(next.Findings, ledger, next.LedgerHash) != nil || !transactionsEqual(expected, next) {
			return fmt.Errorf("%w: findings freeze must replay the exact native transition", ErrInvalidSuccessor)
		}
	}
	fixCompletion := previous.State == StateFixing && next.State == StateFixValidating
	if !fixCompletion && !snapshotsEqual(previous.Snapshot, next.Snapshot) {
		return fmt.Errorf("%w: immutable review snapshot changed", ErrInvalidSuccessor)
	}
	if fixCompletion && (next.Snapshot.Kind != TargetFixDiff || next.Snapshot.BaseTree != previous.FinalCandidateTree || next.Snapshot.CandidateTree != next.FinalCandidateTree || !equalStrings(next.Snapshot.LedgerIDs, next.FixFindingIDs)) {
		return fmt.Errorf("%w: fix completion is not bound to the prior candidate and frozen ledger IDs", ErrInvalidSuccessor)
	}
	if fixCompletion {
		genesisPaths := previous.GenesisPaths
		if genesisPaths == nil {
			genesisPaths = previous.Snapshot.Paths
		}
		if err := pathsAreSubset(next.Snapshot.Paths, genesisPaths); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidSuccessor, err)
		}
		expected := previous
		var err error
		if next.ActualCorrectionLines == nil {
			err = expected.CompleteFix(next.Snapshot, next.FixDeltaHash, next.FixFindingIDs)
		} else {
			err = expected.CompleteFix(next.Snapshot, next.FixDeltaHash, next.FixFindingIDs, *next.ActualCorrectionLines)
		}
		if err != nil || !transactionsEqual(expected, next) {
			return fmt.Errorf("%w: fix completion must replay the exact budgeted transition", ErrInvalidSuccessor)
		}
	}
	fixStart := previous.State == StateFixRequired && (next.State == StateFixing || previous.hasCorrectionBudget() && next.State == StateEscalated)
	if fixStart {
		expected := previous
		var err error
		if next.ProposedCorrectionLines == nil {
			err = expected.BeginFix(next.FailedEvidenceRevision)
		} else {
			err = expected.BeginFix(next.FailedEvidenceRevision, *next.ProposedCorrectionLines)
		}
		if operation != "review/begin-fix" || err != nil || !transactionsEqual(expected, next) {
			return fmt.Errorf("%w: fix start must replay the exact budgeted transition", ErrInvalidSuccessor)
		}
	} else if !equalOptionalInt(previous.ProposedCorrectionLines, next.ProposedCorrectionLines) {
		return fmt.Errorf("%w: proposed correction lines changed outside fix start", ErrInvalidSuccessor)
	}
	if !fixCompletion && !equalOptionalInt(previous.ActualCorrectionLines, next.ActualCorrectionLines) {
		return fmt.Errorf("%w: actual correction lines changed outside fix completion", ErrInvalidSuccessor)
	}
	releaseBinding := previous.Release == nil && next.Release != nil
	if previous.Release != nil && !reflect.DeepEqual(previous.Release, next.Release) {
		return fmt.Errorf("%w: bound release evidence is immutable", ErrInvalidSuccessor)
	}
	if releaseBinding {
		withoutRelease := next
		withoutRelease.Release = nil
		if operation != "review/bind-release-evidence" || previous.State != StateReadyFinalVerification || next.State != StateReadyFinalVerification || !transactionsEqual(previous, withoutRelease) {
			return fmt.Errorf("%w: release evidence must be bound as the only change while ready for final verification", ErrInvalidSuccessor)
		}
	}
	if !releaseBinding && !lensTransition && !legalStateTransition(previous.State, next.State) {
		return fmt.Errorf("%w: illegal state transition %q -> %q", ErrInvalidSuccessor, previous.State, next.State)
	}
	if !countersMonotonic(previous.Counters, next.Counters) {
		return fmt.Errorf("%w: counters cannot decrease", ErrInvalidSuccessor)
	}
	if err := validateSuccessorCounters(previous, next); err != nil {
		return err
	}
	if previous.LedgerHash != "" && (previous.LedgerHash != next.LedgerHash || previous.LedgerFindingsHash != next.LedgerFindingsHash) {
		return fmt.Errorf("%w: frozen ledger hash changed", ErrInvalidSuccessor)
	}
	if previous.JudgeProofHash != "" && (previous.JudgeProofHash != next.JudgeProofHash || previous.JudgeAgreementHash != next.JudgeAgreementHash || !reflect.DeepEqual(previous.JudgeProofs, next.JudgeProofs)) {
		return fmt.Errorf("%w: Judgment Day proof changed", ErrInvalidSuccessor)
	}
	if previous.State != StateReviewing && previous.State != StateJudgesConfirmed && !reflect.DeepEqual(previous.Findings, next.Findings) {
		return fmt.Errorf("%w: frozen findings changed", ErrInvalidSuccessor)
	}
	if !mapIsMonotonic(previous.Classifications, next.Classifications) || !mapIsMonotonic(previous.Outcomes, next.Outcomes) {
		return fmt.Errorf("%w: evidence classifications or outcomes regressed", ErrInvalidSuccessor)
	}
	if !sliceIsSubset(previous.FixFindingIDs, next.FixFindingIDs) || !findingSliceIsPrefix(previous.FixCausedFindings, next.FixCausedFindings) {
		return fmt.Errorf("%w: correction findings regressed", ErrInvalidSuccessor)
	}
	if !followUpSliceIsPrefix(previous.FollowUps, next.FollowUps) {
		return fmt.Errorf("%w: follow-ups regressed", ErrInvalidSuccessor)
	}
	if !equalStrings(previous.PendingRefuterIDs, next.PendingRefuterIDs) {
		classifiedPending := previous.State == StateFindingsFrozen && next.State == StateEvidenceClassified && len(previous.PendingRefuterIDs) == 0 && len(next.PendingRefuterIDs) > 0 && next.Counters.RefuterBatches == previous.Counters.RefuterBatches
		completedBatch := previous.State == StateEvidenceClassified && len(previous.PendingRefuterIDs) > 0 && len(next.PendingRefuterIDs) == 0 && next.Counters.RefuterBatches == previous.Counters.RefuterBatches+1
		if !classifiedPending && !completedBatch {
			return fmt.Errorf("%w: pending refuter IDs changed without one complete consumed batch", ErrInvalidSuccessor)
		}
	}
	if previous.FinalCandidateTree != next.FinalCandidateTree && !fixCompletion {
		return fmt.Errorf("%w: final candidate changed outside fix completion", ErrInvalidSuccessor)
	}
	if previous.FixDeltaHash != next.FixDeltaHash && !fixCompletion {
		return fmt.Errorf("%w: fix delta changed outside fix completion", ErrInvalidSuccessor)
	}
	validationCompletion := isOrdinaryMode(previous.Mode) && previous.State == StateFixValidating && (next.State == StateReadyFinalVerification || next.State == StateEscalated)
	if validationCompletion {
		legacy := next.OriginalCriteria == nil && next.CorrectionRegression == nil
		if legacy {
			if operation != "review/validate-fix-delta" {
				return fmt.Errorf("%w: legacy validation requires its v1 operation", ErrInvalidSuccessor)
			}
		} else if operation != "review/validate-targeted-fix" || next.OriginalCriteria == nil || next.CorrectionRegression == nil {
			return fmt.Errorf("%w: targeted validation requires persisted checks", ErrInvalidSuccessor)
		}
	} else if !reflect.DeepEqual(previous.OriginalCriteria, next.OriginalCriteria) || !reflect.DeepEqual(previous.CorrectionRegression, next.CorrectionRegression) {
		return fmt.Errorf("%w: targeted validation checks changed outside exact validation transition", ErrInvalidSuccessor)
	}
	return nil
}

// validatePersistedV1Successor accepts only the two v1 encodings documented
// in the historical lifecycle. It validates decoded semantics without
// rewriting the content-addressed event payload.
func validatePersistedV1Successor(previous, next Transaction, operation string, index int) error {
	if err := validateSuccessor(previous, next, operation); err == nil {
		return nil
	}
	if index < 1 {
		return fmt.Errorf("%w: historical aliases cannot be genesis records", ErrInvalidSuccessor)
	}
	if operation == "review/validate-targeted-fix" &&
		previous.Mode == ModeOrdinary4R && previous.State == StateFixValidating &&
		(next.State == StateReadyFinalVerification || next.State == StateEscalated) &&
		next.OriginalCriteria == nil && next.CorrectionRegression == nil {
		return validateSuccessor(previous, next, "review/validate-fix-delta")
	}
	if operation == "review/freeze-findings" &&
		(previous.Mode == ModeOrdinary4R || previous.Mode == ModeJudgmentDay) &&
		(previous.State == StateReviewing || previous.State == StateJudgesConfirmed) &&
		next.State == StateFindingsFrozen {
		return validateHistoricalFreezeFindings(previous, next)
	}
	return fmt.Errorf("%w: unsupported historical v1 operation alias", ErrInvalidSuccessor)
}

// validateHistoricalFreezeFindings reproduces v1.49 findings-freeze semantics:
// the event retained an external ledger hash and derived only the findings hash.
func validateHistoricalFreezeFindings(previous, next Transaction) error {
	if !validSHA256(next.LedgerHash) {
		return fmt.Errorf("%w: historical findings freeze requires a ledger hash", ErrInvalidSuccessor)
	}
	expected := previous
	expected.Findings = nil
	if next.Findings != nil {
		// v1.49 hashed an explicit empty findings array differently from null.
		// Preserve that published representation while deriving the hash.
		expected.Findings = make([]Finding, len(next.Findings))
		copy(expected.Findings, next.Findings)
	}
	expected.Outcomes = cloneOutcomes(previous.Outcomes)
	for _, finding := range expected.Findings {
		if !isSevereSeverity(finding.Severity) {
			expected.Outcomes[finding.ID] = OutcomeInfo
		}
	}
	expected.LedgerHash = next.LedgerHash
	expected.LedgerFindingsHash = findingsHash(expected.Findings)
	expected.State = StateFindingsFrozen
	if !transactionsEqual(expected, next) {
		return fmt.Errorf("%w: historical findings freeze changed unrelated transaction state", ErrInvalidSuccessor)
	}
	return nil
}

// transactionsEqual compares persisted transaction state. JSON omits empty
// optional arrays, so a local empty slice and its nil decoded form represent
// the same immutable release-binding state.
func transactionsEqual(left, right Transaction) bool {
	normalize := func(transaction *Transaction) {
		if len(transaction.Snapshot.IntendedUntracked) == 0 {
			transaction.Snapshot.IntendedUntracked = nil
		}
		if len(transaction.Snapshot.LedgerIDs) == 0 {
			transaction.Snapshot.LedgerIDs = nil
		}
		if len(transaction.Snapshot.Paths) == 0 {
			transaction.Snapshot.Paths = nil
		}
		if len(transaction.Findings) == 0 {
			transaction.Findings = nil
		}
		if len(transaction.FixFindingIDs) == 0 {
			transaction.FixFindingIDs = nil
		}
		if len(transaction.PendingRefuterIDs) == 0 {
			transaction.PendingRefuterIDs = nil
		}
		if len(transaction.FixCausedFindings) == 0 {
			transaction.FixCausedFindings = nil
		}
		if len(transaction.FollowUps) == 0 {
			transaction.FollowUps = nil
		}
		if len(transaction.JudgeProofs) == 0 {
			transaction.JudgeProofs = nil
		}
		if len(transaction.SelectedLenses) == 0 {
			transaction.SelectedLenses = nil
		}
		if len(transaction.LensResults) == 0 {
			transaction.LensResults = nil
		}
		for index := range transaction.Findings {
			if len(transaction.Findings[index].ProofRefs) == 0 {
				transaction.Findings[index].ProofRefs = nil
			}
		}
		for index := range transaction.FixCausedFindings {
			if len(transaction.FixCausedFindings[index].ProofRefs) == 0 {
				transaction.FixCausedFindings[index].ProofRefs = nil
			}
		}
	}
	normalize(&left)
	normalize(&right)
	return reflect.DeepEqual(left, right)
}

func followUpSliceIsPrefix(previous, next []FollowUp) bool {
	if len(previous) > len(next) {
		return false
	}
	return reflect.DeepEqual(previous, next[:len(previous)])
}

func equalOptionalInt(left, right *int) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func validateSuccessorCounters(previous, next Transaction) error {
	expected := previous.Counters
	switch {
	case previous.Mode == ModeOrdinaryBounded && previous.State == StateReviewing && next.State == StateReviewing:
		if len(next.LensResults) != len(previous.LensResults)+1 {
			return fmt.Errorf("%w: lens result transition must consume exactly one execution", ErrInvalidSuccessor)
		}
		setLensCounter(&expected, next.LensResults[len(next.LensResults)-1].Lens, 1)
	case previous.State == StateReviewing && next.State == StateJudgesConfirmed:
		expected.JudgeExecutions = 2
	case previous.State == StateEvidenceClassified && next.State != StateEvidenceClassified:
		expected.RefuterBatches++
	case previous.State == StateFixRequired && next.State == StateFixing:
		if isOrdinaryMode(previous.Mode) {
			expected.FixBatches++
		} else {
			expected.FixRounds++
		}
	case previous.State == StateFixValidating && (next.State == StateReadyFinalVerification || next.State == StateFixRequired || next.State == StateEscalated):
		if isOrdinaryMode(previous.Mode) {
			expected.ScopedFixValidations++
		} else {
			expected.ScopedRejudgments++
		}
	case previous.State == StateReadyFinalVerification && next.State == StateFinalVerifying:
		expected.FinalVerifications++
	}
	if expected != next.Counters {
		return fmt.Errorf("%w: counters do not match the semantic state transition", ErrInvalidSuccessor)
	}
	return nil
}

func snapshotsEqual(previous, next Snapshot) bool {
	return previous.Kind == next.Kind &&
		previous.Projection == next.Projection &&
		previous.BaseTree == next.BaseTree &&
		previous.CandidateTree == next.CandidateTree &&
		previous.PathsDigest == next.PathsDigest &&
		previous.IntendedUntrackedProof == next.IntendedUntrackedProof &&
		previous.Identity == next.Identity &&
		equalStrings(previous.IntendedUntracked, next.IntendedUntracked) &&
		equalStrings(previous.LedgerIDs, next.LedgerIDs) &&
		equalStrings(previous.Paths, next.Paths)
}

func validInitialStoreRecord(record Record) bool {
	if record.Operation != "review/start" || record.PreviousRevision != "" {
		return false
	}
	transaction := record.Transaction
	if transaction.State != StateReviewing ||
		transaction.BaseTree != transaction.Snapshot.BaseTree ||
		transaction.PathsDigest != transaction.Snapshot.PathsDigest ||
		transaction.InitialReviewTree != transaction.Snapshot.CandidateTree ||
		transaction.FinalCandidateTree != transaction.InitialReviewTree ||
		transaction.FixDeltaHash != EmptyFixDeltaHash ||
		transaction.LedgerHash != "" || transaction.EvidenceHash != "" ||
		transaction.JudgeProofHash != "" || transaction.JudgeAgreementHash != "" ||
		transaction.Release != nil || transaction.FailedEvidenceRevision != "" ||
		len(transaction.Findings) != 0 || len(transaction.Classifications) != 0 ||
		len(transaction.Outcomes) != 0 || len(transaction.FixFindingIDs) != 0 ||
		len(transaction.PendingRefuterIDs) != 0 || len(transaction.FixCausedFindings) != 0 ||
		len(transaction.JudgeProofs) != 0 {
		return false
	}
	switch transaction.Mode {
	case ModeOrdinary4R:
		return transaction.Counters == (Counters{FullReviews: 1})
	case ModeOrdinaryBounded:
		return transaction.Counters == (Counters{}) && len(transaction.LensResults) == 0
	case ModeJudgmentDay:
		return transaction.Counters == (Counters{})
	default:
		return false
	}
}

func chainIdentity(revisions []string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("gentle-ai.review-chain/v1\x00"))
	for _, revision := range revisions {
		writeLengthPrefixed(hash, []byte(revision))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func legalStateTransition(previous, next State) bool {
	allowed := map[State][]State{
		StateReviewing:              {StateJudgesConfirmed, StateFindingsFrozen, StateEscalated},
		StateJudgesConfirmed:        {StateFindingsFrozen, StateEscalated},
		StateFindingsFrozen:         {StateEvidenceClassified, StateFixRequired, StateReadyFinalVerification, StateEscalated},
		StateEvidenceClassified:     {StateFixRequired, StateReadyFinalVerification, StateEscalated},
		StateFixRequired:            {StateFixing, StateEscalated},
		StateFixing:                 {StateFixValidating, StateEscalated},
		StateFixValidating:          {StateReadyFinalVerification, StateFixRequired, StateEscalated},
		StateReadyFinalVerification: {StateFinalVerifying, StateEscalated},
		StateFinalVerifying:         {StateApproved, StateEscalated},
	}
	for _, candidate := range allowed[previous] {
		if candidate == next {
			return true
		}
	}
	return false
}

func countersMonotonic(previous, next Counters) bool {
	return next.FullReviews >= previous.FullReviews &&
		next.RefuterBatches >= previous.RefuterBatches &&
		next.FixBatches >= previous.FixBatches &&
		next.ScopedFixValidations >= previous.ScopedFixValidations &&
		next.FinalVerifications >= previous.FinalVerifications &&
		next.FixRounds >= previous.FixRounds &&
		next.ScopedRejudgments >= previous.ScopedRejudgments &&
		next.JudgeExecutions >= previous.JudgeExecutions &&
		next.RiskExecutions >= previous.RiskExecutions &&
		next.ResilienceExecutions >= previous.ResilienceExecutions &&
		next.ReadabilityExecutions >= previous.ReadabilityExecutions &&
		next.ReliabilityExecutions >= previous.ReliabilityExecutions
}

func mapIsMonotonic[K comparable, V comparable](previous, next map[K]V) bool {
	for key, value := range previous {
		if candidate, ok := next[key]; !ok || candidate != value {
			return false
		}
	}
	return true
}

func sliceIsSubset(previous, next []string) bool {
	set := make(map[string]struct{}, len(next))
	for _, value := range next {
		set[value] = struct{}{}
	}
	for _, value := range previous {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func findingSliceIsPrefix(previous, next []Finding) bool {
	return len(previous) <= len(next) && reflect.DeepEqual(previous, next[:len(previous)])
}

func WriteReceiptAtomic(path string, receipt Receipt) error {
	if err := validateReceiptStructure(receipt); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(payload, '\n'), 0o644)
}

func WriteTransactionAtomic(path string, transaction Transaction) error {
	if err := transaction.validate(); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(transaction, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(payload, '\n'), 0o644)
}

// PublishFileNoReplace atomically publishes source only when destination is absent.
func PublishFileNoReplace(source, destination string) error {
	return publishNoReplace(source, destination)
}

// ReplaceFileAtomic atomically replaces destination with source.
func ReplaceFileAtomic(source, destination string) error {
	return replaceFileAtomic(source, destination)
}

func readRevision(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	revision := strings.TrimSpace(string(payload))
	if revision != "" && !validSHA256(revision) {
		return "", fmt.Errorf("invalid review store HEAD %q", revision)
	}
	return revision, nil
}

func writeAtomic(path string, payload []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".atomic-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(payload); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := replaceFileAtomic(tempPath, path); err != nil {
		return err
	}
	if err := SyncReviewDirectory(filepath.Dir(path)); err != nil {
		return &directorySyncError{path: path, cause: err}
	}
	return nil
}

func publishImmutable(path string, payload []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".publish-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(payload); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := publishNoReplace(tempPath, path); err != nil {
		if !os.IsExist(err) {
			return err
		}
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return &ImmutablePublicationConflictError{Cause: errors.New("non-regular existing path")}
		}
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(existing, payload) {
			return &ImmutablePublicationConflictError{Cause: errors.New("existing content differs")}
		}
		if err := SyncReviewDirectory(dir); err != nil {
			return &directorySyncError{path: path, cause: err}
		}
		return nil
	}
	if err := SyncReviewDirectory(dir); err != nil {
		return &directorySyncError{path: path, cause: err}
	}
	return nil
}
