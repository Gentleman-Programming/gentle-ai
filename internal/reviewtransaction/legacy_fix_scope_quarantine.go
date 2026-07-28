package reviewtransaction

import (
	"bytes"
	"context"
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

const (
	legacyFixScopeQuarantineAuthorizationSchema = "gentle-ai.review-legacy-fix-scope-quarantine-authorization/v2"
	LegacyFixScopeQuarantineDisposition         = "quarantine-historical-complete-fix-scope-expansion"
	legacyFixScopeScopeOnlyAnomalySet           = "complete_fix_scope_expansion"
	legacyFixScopeCombinedAnomalySet            = "complete_fix_scope_expansion,validate_fix_alias"
)

var legacyFixScopeBeforeMaintenance = func() {}
var legacyFixScopeAfterMaintenance = func() {}

// LegacyFixScopeQuarantineRequest names an exact immutable legacy-v1 source.
type LegacyFixScopeQuarantineRequest struct {
	LineageID, ExpectedRevision, ExpectedDiagnostic, Disposition string
	ExpectedAnomalySet, ExpectedValidateFixAliasRevision         string
	ExpectedValidateFixAliasOperation, Reason, Actor             string
	MaintainerAuthorization                                      string
	QuarantinedAt                                                time.Time
}

// LegacyFixScopeQuarantineProof is fully re-derived from the immutable chain.
type LegacyFixScopeQuarantineProof struct {
	LineageID               string                  `json:"lineage_id"`
	HeadRevision            string                  `json:"head_revision"`
	ChainIdentity           string                  `json:"chain_identity"`
	EventRevision           string                  `json:"event_revision"`
	PreviousRevision        string                  `json:"previous_revision"`
	Operation               string                  `json:"operation"`
	FrozenGenesisPaths      []string                `json:"frozen_genesis_paths"`
	ExpandedCorrectionPaths []string                `json:"expanded_correction_paths"`
	FixDeltaHash            string                  `json:"fix_delta_hash"`
	Diagnostic              string                  `json:"diagnostic"`
	Disposition             string                  `json:"disposition"`
	AnomalySet              string                  `json:"anomaly_set"`
	Anomalies               []LegacyFixScopeAnomaly `json:"anomalies"`
}

type LegacyFixScopeAnomaly struct {
	Kind               string `json:"kind"`
	EventRevision      string `json:"event_revision"`
	Operation          string `json:"operation"`
	CanonicalOperation string `json:"canonical_operation"`
}

func legacyFixScopeQuarantineAuthorizationBinding(repository, lineage, revision, diagnostic, disposition, anomalySet, aliasRevision, aliasOperation, actor, reason string) string {
	return legacyFixScopeQuarantineAuthorizationSchema + "\nrepository=" + repository +
		"\nlineage=" + lineage + "\nrevision=" + revision + "\ndiagnostic=" + diagnostic +
		"\ndisposition=" + disposition + "\nanomaly_set=" + anomalySet +
		"\nvalidate_fix_alias_revision=" + aliasRevision + "\nvalidate_fix_alias_operation=" + aliasOperation +
		"\nactor=" + strings.TrimSpace(actor) + "\nreason=" + strings.TrimSpace(reason)
}

// QuarantineHistoricalLegacyFixScope quarantines, but never validates, the
// only approved complete-fix scope-expansion classes.
func QuarantineHistoricalLegacyFixScope(ctx context.Context, repo string, request LegacyFixScopeQuarantineRequest) (CompactReclaimRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := validateLineageID(request.LineageID); err != nil {
		return CompactReclaimRecord{}, err
	}
	if !validSHA256(request.ExpectedRevision) {
		return CompactReclaimRecord{}, errors.New("review quarantine-legacy-fix-scope requires an exact current HEAD revision")
	}
	if strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.Actor) == "" || strings.ContainsAny(request.Reason, "\r\n") || strings.ContainsAny(request.Actor, "\r\n") {
		return CompactReclaimRecord{}, errors.New("review quarantine-legacy-fix-scope requires non-empty LF-only single-line actor and reason authorization fields")
	}
	if request.Disposition != LegacyFixScopeQuarantineDisposition {
		return CompactReclaimRecord{}, errors.New("review quarantine-legacy-fix-scope supports only the historical complete-fix scope-expansion diagnostic and disposition")
	}
	if err := validateLegacyFixScopeAuthorizationClass(request); err != nil {
		return CompactReclaimRecord{}, err
	}
	base, repository, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := canonicalLegacyFixScopeAuthorityRoots(base); err != nil {
		return CompactReclaimRecord{}, err
	}
	legacyFixScopeBeforeMaintenance()
	maintenance, err := acquireMaintenanceLock(ctx, compactMaintenanceLockPath(base), maintenanceExclusive)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	defer maintenance.Release()
	// The exclusive authority lock covers both the source/replay decision and the
	// prepared-to-committed publication window.
	legacyFixScopeAfterMaintenance()
	if err := canonicalLegacyFixScopeAuthorityRoots(base); err != nil {
		return CompactReclaimRecord{}, err
	}
	dir, exists, err := canonicalLegacyFixScopeDir(base, request.LineageID)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := rejectLegacyFixScopeCompactAuthority(base, request.LineageID); err != nil {
		return CompactReclaimRecord{}, err
	}
	if !exists {
		record, err := replayCommittedLegacyFixScopeQuarantine(base, repository, request)
		if err != nil {
			return CompactReclaimRecord{}, err
		}
		return record, verifyHistoricalLegacyFixScopeReadback(ctx, repo, request.LineageID, record)
	}
	if err := validateLegacyFixScopePhysicalStore(dir); err != nil {
		return CompactReclaimRecord{}, err
	}
	localLock, err := acquireLocalStoreLock(filepath.Join(dir, "LOCK"))
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("review quarantine-legacy-fix-scope refused active ownership: %w", err)
	}
	locked := true
	defer func() {
		if locked {
			_ = localLock.release()
		}
	}()
	if err := validateLegacyFixScopePhysicalStore(dir); err != nil {
		return CompactReclaimRecord{}, err
	}
	proof, err := inspectHistoricalLegacyFixScope(Store{Dir: dir, lineageID: request.LineageID, repo: repository, readOnly: true}, request.ExpectedRevision)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	proof.Disposition = request.Disposition
	if request.ExpectedDiagnostic != proof.Diagnostic {
		return CompactReclaimRecord{}, errors.New("review quarantine-legacy-fix-scope supports only the exact inventory diagnostic for the historical complete-fix scope-expansion")
	}
	if !legacyFixScopeAuthorizationMatchesProof(request, proof) {
		return CompactReclaimRecord{}, errors.New("review quarantine-legacy-fix-scope requires an exact anomaly set and validate-fix alias event binding")
	}
	if request.MaintainerAuthorization != legacyFixScopeQuarantineAuthorizationBinding(repository, request.LineageID, request.ExpectedRevision, request.ExpectedDiagnostic, request.Disposition, request.ExpectedAnomalySet, request.ExpectedValidateFixAliasRevision, request.ExpectedValidateFixAliasOperation, request.Actor, request.Reason) {
		return CompactReclaimRecord{}, errors.New("review quarantine-legacy-fix-scope requires an exact maintainer authorization binding")
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		residue = append(residue, item.Name())
	}
	sort.Strings(residue)
	if request.QuarantinedAt.IsZero() {
		request.QuarantinedAt = time.Now().UTC()
	}
	if err := localLock.release(); err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("release legacy lineage lock before quarantine: %w", err)
	}
	locked = false
	if err := canonicalLegacyFixScopeAuthorityRoots(base); err != nil {
		return CompactReclaimRecord{}, err
	}
	record, err := quarantineCompactStoreEntry(ctx, base, dir, CompactReclaimRecord{Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: request.LineageID, Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor), ReclaimedAt: request.QuarantinedAt.UTC(), SourcePath: dir, Residue: residue, LegacyFixScopeQuarantine: &proof})
	if err != nil {
		return record, err
	}
	if err := validateCommittedLegacyFixScopeReplay(record, record.QuarantinePath, filepath.Join(base, "v1"), repository, request); err != nil {
		return record, err
	}
	if err := verifyHistoricalLegacyFixScopeReadback(ctx, repo, request.LineageID, record); err != nil {
		return record, err
	}
	return record, nil
}

func rejectLegacyFixScopeCompactAuthority(base, lineage string) error {
	if _, err := os.Lstat(filepath.Join(base, "v2", lineage)); err == nil {
		return fmt.Errorf("review quarantine-legacy-fix-scope refused: lineage %q also exists in compact-v2 authority", lineage)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect same-lineage compact authority: %w", err)
	}
	return nil
}

func canonicalLegacyFixScopeAuthorityRoots(base string) error {
	if err := ensureCanonicalReviewQuarantineRoot(base, filepath.Join(base, "quarantine")); err != nil {
		return err
	}
	for _, component := range []string{"v1"} {
		path := filepath.Join(base, component)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("review quarantine-legacy-fix-scope refused unsafe authority root component %q", component)
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil || filepath.Clean(canonical) != path {
			return fmt.Errorf("review quarantine-legacy-fix-scope refused noncanonical authority root component %q", component)
		}
	}
	return nil
}

func canonicalLegacyFixScopeDir(base, lineage string) (string, bool, error) {
	root := filepath.Join(base, "v1")
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return filepath.Join(root, lineage), false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", false, errors.New("review quarantine-legacy-fix-scope refused unsafe authority root component \"v1\"")
	}
	dir := filepath.Join(root, lineage)
	info, err = os.Lstat(dir)
	if os.IsNotExist(err) {
		return dir, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", false, errors.New("review quarantine-legacy-fix-scope refused symlinked or non-directory lineage component")
	}
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil || filepath.Clean(canonical) != dir {
		return "", false, errors.New("review quarantine-legacy-fix-scope refused noncanonical lineage component")
	}
	return dir, true, nil
}

func validateLegacyFixScopePhysicalStore(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("review quarantine-legacy-fix-scope refused unsafe physical lineage directory")
	}
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil || filepath.Clean(canonical) != dir {
		return errors.New("review quarantine-legacy-fix-scope refused noncanonical physical lineage directory")
	}
	for _, name := range []string{"HEAD", "events", "LOCK"} {
		directory := name == "events"
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) && name == "LOCK" {
			continue
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory || !directory && !info.Mode().IsRegular() {
			return fmt.Errorf("review quarantine-legacy-fix-scope refused unsafe physical lineage child %q", name)
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil || filepath.Clean(canonical) != path {
			return fmt.Errorf("review quarantine-legacy-fix-scope refused noncanonical physical lineage child %q", name)
		}
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Name() != "HEAD" && item.Name() != "events" && item.Name() != "LOCK" {
			return fmt.Errorf("review quarantine-legacy-fix-scope refused unexpected physical lineage child %q", item.Name())
		}
	}
	return nil
}

func validateLegacyFixScopePhysicalEvent(store Store, revision string) error {
	path := filepath.Join(store.Dir, "events", strings.TrimPrefix(revision, "sha256:")+".json")
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("review quarantine-legacy-fix-scope refused unsafe physical event %q", revision)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || filepath.Clean(canonical) != path {
		return fmt.Errorf("review quarantine-legacy-fix-scope refused noncanonical physical event %q", revision)
	}
	return nil
}

func inspectHistoricalLegacyFixScope(store Store, expectedHead string) (LegacyFixScopeQuarantineProof, error) {
	if err := validateLegacyFixScopePhysicalStore(store.Dir); err != nil {
		return LegacyFixScopeQuarantineProof{}, err
	}
	head, err := readRevision(filepath.Join(store.Dir, "HEAD"))
	if err != nil {
		return LegacyFixScopeQuarantineProof{}, err
	}
	if head != expectedHead {
		return LegacyFixScopeQuarantineProof{}, fmt.Errorf("%w: expected legacy HEAD %q, current %q", ErrConcurrentUpdate, expectedHead, head)
	}
	var reverseRecords []Record
	var reverseRevisions []string
	visited := map[string]bool{}
	for revision := head; revision != ""; {
		if visited[revision] {
			return LegacyFixScopeQuarantineProof{}, errors.New("review quarantine-legacy-fix-scope refused: predecessor cycle")
		}
		visited[revision] = true
		if err := validateLegacyFixScopePhysicalEvent(store, revision); err != nil {
			return LegacyFixScopeQuarantineProof{}, err
		}
		record, loaded, err := store.loadRevision(revision)
		if err != nil {
			return LegacyFixScopeQuarantineProof{}, fmt.Errorf("review quarantine-legacy-fix-scope refused: load revision %s: %w", revision, err)
		}
		reverseRecords, reverseRevisions, revision = append(reverseRecords, record), append(reverseRevisions, loaded), record.PreviousRevision
	}
	if err := validateLegacyFixScopePhysicalEvents(store, reverseRevisions); err != nil {
		return LegacyFixScopeQuarantineProof{}, err
	}
	records, revisions := make([]Record, len(reverseRecords)), make([]string, len(reverseRevisions))
	for i := range reverseRecords {
		n := len(reverseRecords) - 1 - i
		records[i], revisions[i] = reverseRecords[n], reverseRevisions[n]
	}
	if len(records) == 0 || !validInitialStoreRecord(records[0]) || records[0].Transaction.LineageID != store.lineageID {
		return LegacyFixScopeQuarantineProof{}, errors.New("review quarantine-legacy-fix-scope refused: chain genesis is invalid")
	}
	var proof *LegacyFixScopeQuarantineProof
	for i := 1; i < len(records); i++ {
		if records[i].PreviousRevision != revisions[i-1] {
			return LegacyFixScopeQuarantineProof{}, errors.New("review quarantine-legacy-fix-scope refused: predecessor revision is discontinuous")
		}
		err := validatePersistedV1Successor(records[i-1].Transaction, records[i].Transaction, records[i].Operation, i)
		if err == nil {
			continue
		}
		if proof != nil && proof.AnomalySet == legacyFixScopeScopeOnlyAnomalySet && validateHistoricalFixScopeSuccessor(records[i-1].Transaction, records[i].Transaction, records[i].Operation) == nil {
			proof.AnomalySet = legacyFixScopeCombinedAnomalySet
			proof.Anomalies = append(proof.Anomalies, LegacyFixScopeAnomaly{"validate_fix_alias", revisions[i], records[i].Operation, "review/validate-fix-delta"})
			continue
		}
		expanded, frozen, scopeErr := historicalCompleteFixScopeExpansion(records[i-1].Transaction, records[i].Transaction, records[i].Operation)
		if scopeErr != nil || proof != nil {
			return LegacyFixScopeQuarantineProof{}, fmt.Errorf("review quarantine-legacy-fix-scope refused unsupported successor anomaly at %s: %w", revisions[i], err)
		}
		proof = &LegacyFixScopeQuarantineProof{LineageID: store.lineageID, HeadRevision: head, ChainIdentity: chainIdentity(revisions), EventRevision: revisions[i], PreviousRevision: revisions[i-1], Operation: records[i].Operation, FrozenGenesisPaths: frozen, ExpandedCorrectionPaths: expanded, FixDeltaHash: records[i].Transaction.FixDeltaHash, Diagnostic: legacyFixScopeInventoryDiagnostic(revisions[i]), AnomalySet: legacyFixScopeScopeOnlyAnomalySet, Anomalies: []LegacyFixScopeAnomaly{{"complete_fix_scope_expansion", revisions[i], records[i].Operation, "review/complete-fix"}}}
	}
	if proof == nil {
		return LegacyFixScopeQuarantineProof{}, errors.New("review quarantine-legacy-fix-scope refused: lineage has no supported historical complete-fix scope expansion")
	}
	return *proof, nil
}

func validateLegacyFixScopePhysicalEvents(store Store, revisions []string) error {
	expected := make(map[string]bool, len(revisions))
	for _, revision := range revisions {
		expected[strings.TrimPrefix(revision, "sha256:")+".json"] = true
	}
	items, err := os.ReadDir(filepath.Join(store.Dir, "events"))
	if err != nil {
		return err
	}
	for _, item := range items {
		path := filepath.Join(store.Dir, "events", item.Name())
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("review quarantine-legacy-fix-scope refused unsafe physical event component %q", item.Name())
		}
		if !expected[item.Name()] {
			return fmt.Errorf("review quarantine-legacy-fix-scope refused non-HEAD-chain physical event %q", item.Name())
		}
		delete(expected, item.Name())
	}
	if len(expected) != 0 {
		return errors.New("review quarantine-legacy-fix-scope refused missing physical chain event")
	}
	return nil
}

func legacyFixScopeInventoryDiagnostic(revision string) string {
	return "review chain successor " + revision + ": " + ErrInvalidSuccessor.Error() + ": unsupported historical v1 operation alias"
}

func validateHistoricalFixScopeSuccessor(previous, next Transaction, operation string) error {
	if operation != "review/validate-fix" || previous.Mode != ModeOrdinary4R || previous.State != StateFixValidating || (next.State != StateReadyFinalVerification && next.State != StateEscalated) || next.OriginalCriteria != nil || next.CorrectionRegression != nil {
		return errors.New("not a historical fix-scope successor")
	}
	return validateSuccessor(previous, next, "review/validate-fix-delta")
}

func historicalCompleteFixScopeExpansion(previous, next Transaction, operation string) ([]string, []string, error) {
	if operation != "review/complete-fix" || previous.Mode != ModeOrdinary4R || previous.State != StateFixing || next.State != StateFixValidating || !equalStrings(previous.GenesisPaths, next.GenesisPaths) {
		return nil, nil, errors.New("not an ordinary_4r complete-fix scope expansion")
	}
	frozen := previous.GenesisPaths
	if frozen == nil {
		frozen = previous.Snapshot.Paths
	}
	canonicalFrozen, err := canonicalPaths(frozen)
	if err != nil || !equalStrings(canonicalFrozen, frozen) {
		return nil, nil, errors.New("frozen genesis paths are malformed")
	}
	canonicalNext, err := canonicalPaths(next.Snapshot.Paths)
	if err != nil || !equalStrings(canonicalNext, next.Snapshot.Paths) {
		return nil, nil, errors.New("correction paths are malformed")
	}
	allowed := map[string]struct{}{}
	for _, path := range frozen {
		allowed[path] = struct{}{}
	}
	expanded := []string{}
	for _, path := range next.Snapshot.Paths {
		if _, ok := allowed[path]; !ok {
			expanded = append(expanded, path)
		}
	}
	if len(expanded) == 0 {
		return nil, nil, errors.New("correction does not expand frozen genesis scope")
	}
	derived, err := canonicalPaths(append(append([]string{}, frozen...), expanded...))
	if err != nil {
		return nil, nil, err
	}
	left, right := previous, next
	left.GenesisPaths, right.GenesisPaths = derived, derived
	if err := validateSuccessor(left, right, operation); err != nil {
		return nil, nil, err
	}
	return expanded, append([]string{}, frozen...), nil
}

func validateLegacyFixScopeAuthorizationClass(request LegacyFixScopeQuarantineRequest) error {
	switch request.ExpectedAnomalySet {
	case legacyFixScopeScopeOnlyAnomalySet:
		if request.ExpectedValidateFixAliasRevision != "" || request.ExpectedValidateFixAliasOperation != "" {
			return errors.New("review quarantine-legacy-fix-scope scope-only anomaly set forbids validate-fix alias fields")
		}
	case legacyFixScopeCombinedAnomalySet:
		if !validSHA256(request.ExpectedValidateFixAliasRevision) || request.ExpectedValidateFixAliasOperation != "review/validate-fix" {
			return errors.New("review quarantine-legacy-fix-scope combined anomaly set requires the exact validate-fix alias revision and operation")
		}
	default:
		return errors.New("review quarantine-legacy-fix-scope supports only canonical scope-only or scope-plus-validate-fix anomaly sets")
	}
	return nil
}

func legacyFixScopeAuthorizationMatchesProof(request LegacyFixScopeQuarantineRequest, proof LegacyFixScopeQuarantineProof) bool {
	if request.ExpectedAnomalySet != proof.AnomalySet || len(proof.Anomalies) == 0 || proof.Anomalies[0].Kind != "complete_fix_scope_expansion" {
		return false
	}
	if proof.AnomalySet == legacyFixScopeScopeOnlyAnomalySet {
		return len(proof.Anomalies) == 1
	}
	return len(proof.Anomalies) == 2 && proof.Anomalies[1].Kind == "validate_fix_alias" && request.ExpectedValidateFixAliasRevision == proof.Anomalies[1].EventRevision && request.ExpectedValidateFixAliasOperation == proof.Anomalies[1].Operation
}

func verifyHistoricalLegacyFixScopeReadback(ctx context.Context, repo, lineage string, record CompactReclaimRecord) error {
	if record.Status != CompactReclaimCommitted || record.LegacyFixScopeQuarantine == nil {
		return errors.New("review quarantine-legacy-fix-scope did not commit an auditable quarantine record")
	}
	report, err := InventoryAuthority(ctx, repo)
	if err != nil {
		return fmt.Errorf("read back authority inventory after fix-scope quarantine: %w", err)
	}
	if !report.Complete || !report.Authoritative {
		return errors.New("read back authority inventory after fix-scope quarantine is incomplete or non-authoritative")
	}
	for _, entry := range report.Entries {
		if entry.LineageID == lineage {
			return fmt.Errorf("read back authority inventory still contains quarantined lineage %q", lineage)
		}
	}
	return nil
}

func replayCommittedLegacyFixScopeQuarantine(base, repository string, request LegacyFixScopeQuarantineRequest) (CompactReclaimRecord, error) {
	if err := canonicalLegacyFixScopeAuthorityRoots(base); err != nil {
		return CompactReclaimRecord{}, err
	}
	versionRoot, quarantineRoot := filepath.Join(base, "v1"), filepath.Join(base, "quarantine")
	entries, err := os.ReadDir(quarantineRoot)
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("inspect quarantine for legacy fix-scope replay: %w", err)
	}
	var matched *CompactReclaimRecord
	stalePrepared := false
	for _, entry := range entries {
		path := filepath.Join(quarantineRoot, entry.Name())
		info, err := os.Lstat(path)
		candidate := strings.HasPrefix(entry.Name(), request.LineageID+"-")
		canonical, canonicalErr := filepath.EvalSymlinks(path)
		if err != nil || canonicalErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || filepath.Clean(canonical) != path {
			if candidate {
				return CompactReclaimRecord{}, errors.New("review quarantine-legacy-fix-scope refused unsafe quarantine record directory")
			}
			continue
		}
		recordPath := filepath.Join(path, "reclaim-record.json")
		recordInfo, recordErr := os.Lstat(recordPath)
		if recordErr != nil || recordInfo.Mode()&os.ModeSymlink != 0 || !recordInfo.Mode().IsRegular() {
			if candidate {
				return CompactReclaimRecord{}, errors.New("review quarantine-legacy-fix-scope refused unsafe committed replay record")
			}
			continue
		}
		payload, err := os.ReadFile(recordPath)
		if err != nil {
			if candidate {
				return CompactReclaimRecord{}, fmt.Errorf("review quarantine-legacy-fix-scope refused unreadable committed replay record: %w", err)
			}
			continue
		}
		record, err := decodeLegacyFixScopeReclaimRecord(payload)
		if err != nil {
			if candidate {
				return CompactReclaimRecord{}, fmt.Errorf("review quarantine-legacy-fix-scope refused corrupt replay record: %w", err)
			}
			continue
		}
		if record.LineageID != request.LineageID && (record.LegacyFixScopeQuarantine == nil || record.LegacyFixScopeQuarantine.LineageID != request.LineageID) {
			continue
		}
		if record.Status == CompactReclaimPrepared {
			// Under the caller's exclusive maintenance lock, this is an orphan,
			// not an in-flight replay, and cannot prove a successful quarantine.
			stalePrepared = true
			continue
		}
		if err := validateCommittedLegacyFixScopeReplay(record, path, versionRoot, repository, request); err != nil {
			return CompactReclaimRecord{}, err
		}
		if matched != nil {
			return CompactReclaimRecord{}, errors.New("review quarantine-legacy-fix-scope refused duplicate committed replay records")
		}
		copy := record
		matched = &copy
	}
	if matched == nil {
		if stalePrepared {
			return CompactReclaimRecord{}, errors.New("review quarantine-legacy-fix-scope refused: stale or orphan prepared quarantine record has no committed replay")
		}
		return CompactReclaimRecord{}, fmt.Errorf("review quarantine-legacy-fix-scope refused: lineage %q is absent and no committed quarantine record matches", request.LineageID)
	}
	return *matched, nil
}

func decodeLegacyFixScopeReclaimRecord(payload []byte) (CompactReclaimRecord, error) {
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

func validateCommittedLegacyFixScopeReplay(record CompactReclaimRecord, quarantinePath, versionRoot, repository string, request LegacyFixScopeQuarantineRequest) error {
	proof := record.LegacyFixScopeQuarantine
	if record.Schema != CompactReclaimRecordSchema || record.Status != CompactReclaimCommitted || record.LineageID != request.LineageID || record.ReclaimedAt.IsZero() || proof == nil || record.InvalidRecoveryEdge != nil || record.MalformedRecoveryAuthorization != nil || record.PristineAbandonment != nil || record.MalformedLegacyFreeze != nil || record.LegacyAliasRepair != nil {
		return errors.New("review quarantine-legacy-fix-scope refused incomplete or mismatched committed replay record")
	}
	if record.SourcePath != filepath.Join(versionRoot, request.LineageID) || record.QuarantinePath != quarantinePath || record.Reason != strings.TrimSpace(request.Reason) || record.Actor != strings.TrimSpace(request.Actor) {
		return errors.New("review quarantine-legacy-fix-scope refused mismatched committed replay audit path or authorization")
	}
	residuePath := filepath.Join(quarantinePath, "residue")
	info, err := os.Lstat(residuePath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("review quarantine-legacy-fix-scope refused missing or unsafe committed replay residue")
	}
	items, err := os.ReadDir(residuePath)
	if err != nil {
		return err
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		residue = append(residue, item.Name())
	}
	sort.Strings(residue)
	if !reflect.DeepEqual(record.Residue, residue) {
		return errors.New("review quarantine-legacy-fix-scope refused mismatched committed replay residue")
	}
	derived, err := inspectHistoricalLegacyFixScope(Store{Dir: residuePath, lineageID: request.LineageID, repo: repository, readOnly: true}, request.ExpectedRevision)
	if err != nil {
		return fmt.Errorf("review quarantine-legacy-fix-scope refused invalid committed replay residue: %w", err)
	}
	derived.Disposition = request.Disposition
	if !reflect.DeepEqual(*proof, derived) || proof.Diagnostic != request.ExpectedDiagnostic || proof.Disposition != request.Disposition || !legacyFixScopeAuthorizationMatchesProof(request, *proof) || request.MaintainerAuthorization != legacyFixScopeQuarantineAuthorizationBinding(repository, proof.LineageID, proof.HeadRevision, proof.Diagnostic, proof.Disposition, request.ExpectedAnomalySet, request.ExpectedValidateFixAliasRevision, request.ExpectedValidateFixAliasOperation, request.Actor, request.Reason) {
		return errors.New("review quarantine-legacy-fix-scope refused mismatched committed replay proof or authorization")
	}
	return nil
}
