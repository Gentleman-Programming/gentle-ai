package reviewtransaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

const (
	legacyQuarantineAuthorizationSchema        = "gentle-ai.review-legacy-quarantine-authorization/v1"
	LegacyMalformedFreezeQuarantineDisposition = "quarantine-malformed-freeze-event"
	legacyMalformedFreezeDiagnostic            = "historical findings freeze changed unrelated transaction state"
)

// LegacyQuarantineRequest identifies one exact legacy-v1 lineage revision
// whose malformed findings-freeze event must be quarantined.
type LegacyQuarantineRequest struct {
	LineageID               string
	ExpectedRevision        string
	ExpectedDiagnostic      string
	Disposition             string
	Reason                  string
	Actor                   string
	MaintainerAuthorization string
	QuarantinedAt           time.Time
}

// LegacyMalformedFreezeProof records the exact structurally valid event whose
// historical findings-freeze replay changed unrelated transaction fields.
type LegacyMalformedFreezeProof struct {
	LineageID        string   `json:"lineage_id"`
	HeadRevision     string   `json:"head_revision"`
	ChainIdentity    string   `json:"chain_identity"`
	EventRevision    string   `json:"event_revision"`
	PreviousRevision string   `json:"previous_revision"`
	Operation        string   `json:"operation"`
	Diagnostic       string   `json:"diagnostic"`
	Disposition      string   `json:"disposition"`
	ChangedFields    []string `json:"changed_fields"`
}

func legacyQuarantineAuthorizationBinding(repository, lineage, revision, diagnostic, disposition, actor, reason string) string {
	return legacyQuarantineAuthorizationSchema + "\nrepository=" + repository +
		"\nlineage=" + lineage + "\nrevision=" + revision +
		"\ndiagnostic=" + diagnostic + "\ndisposition=" + disposition +
		"\nactor=" + strings.TrimSpace(actor) + "\nreason=" + strings.TrimSpace(reason)
}

// QuarantineMalformedLegacyFreeze moves one legacy-v1 lineage whose exact
// content-addressed chain contains one supported malformed findings-freeze
// event into audited quarantine. It never accepts the event as valid or
// rewrites its bytes. Exact committed retries return the original record.
func QuarantineMalformedLegacyFreeze(ctx context.Context, repo string, request LegacyQuarantineRequest) (CompactReclaimRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := validateLineageID(request.LineageID); err != nil {
		return CompactReclaimRecord{}, err
	}
	if !validSHA256(request.ExpectedRevision) {
		return CompactReclaimRecord{}, errors.New("review quarantine-legacy requires an exact current HEAD revision")
	}
	if strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.Actor) == "" {
		return CompactReclaimRecord{}, errors.New("review quarantine-legacy requires a non-empty reason and actor")
	}
	if request.ExpectedDiagnostic != legacyMalformedFreezeDiagnostic || request.Disposition != LegacyMalformedFreezeQuarantineDisposition {
		return CompactReclaimRecord{}, errors.New("review quarantine-legacy supports only the malformed historical findings-freeze diagnostic and disposition")
	}
	base, repository, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	dir := filepath.Join(base, "v1", request.LineageID)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return replayCommittedLegacyQuarantine(base, repository, request)
		}
		return CompactReclaimRecord{}, fmt.Errorf("inspect legacy quarantine target: %w", err)
	}
	// Detect active lineage ownership with the per-lineage local lock only. Its
	// handle lives inside dir, which is renamed into quarantine below; on Windows
	// an open handle anywhere inside a directory makes the directory rename fail
	// with "Access is denied", so this handle must be closed before the move.
	localLock, err := acquireLocalStoreLock(filepath.Join(dir, "LOCK"))
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("review quarantine-legacy refused active ownership: %w", err)
	}
	localLockReleased := false
	releaseLocalLock := func() error {
		if localLockReleased {
			return nil
		}
		localLockReleased = true
		return localLock.release()
	}
	defer releaseLocalLock()
	// Hold the authority-wide maintenance lock exclusively across the rename so
	// mutual exclusion is still guaranteed once the per-lineage local lock is
	// released. The maintenance lock lives above the lineage directory, so it is
	// never an open handle inside the directory being moved.
	maintenance, err := acquireMaintenanceLock(ctx, compactMaintenanceLockPath(base), maintenanceExclusive)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	defer maintenance.Release()
	if _, err := os.Stat(filepath.Join(base, "v2", request.LineageID)); err == nil {
		return CompactReclaimRecord{}, fmt.Errorf("review quarantine-legacy refused: lineage %q also exists in compact-v2 authority", request.LineageID)
	} else if !os.IsNotExist(err) {
		return CompactReclaimRecord{}, fmt.Errorf("inspect same-lineage compact authority: %w", err)
	}
	store := Store{Dir: dir, lineageID: request.LineageID, repo: repository, readOnly: true}
	proof, err := inspectMalformedLegacyFreeze(ctx, store, request.ExpectedRevision)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	proof.Disposition = request.Disposition
	if request.MaintainerAuthorization != legacyQuarantineAuthorizationBinding(
		repository, request.LineageID, request.ExpectedRevision, request.ExpectedDiagnostic,
		request.Disposition, request.Actor, request.Reason,
	) {
		return CompactReclaimRecord{}, fmt.Errorf("review quarantine-legacy requires an exact maintainer authorization binding (schema %s over repository, lineage, revision, diagnostic, disposition, actor, and reason)", legacyQuarantineAuthorizationSchema)
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("inspect legacy quarantine target: %w", err)
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		residue = append(residue, item.Name())
	}
	sort.Strings(residue)
	if request.QuarantinedAt.IsZero() {
		request.QuarantinedAt = time.Now().UTC()
	}
	// Close the per-lineage lock handle before renaming dir into quarantine so
	// no open handle remains inside the directory on Windows. The exclusive
	// maintenance lock, held until this function returns, preserves mutual
	// exclusion across the rename.
	if err := releaseLocalLock(); err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("release legacy lineage lock before quarantine: %w", err)
	}
	return quarantineCompactStoreEntry(base, dir, CompactReclaimRecord{
		Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared,
		LineageID: request.LineageID, Reason: strings.TrimSpace(request.Reason),
		Actor: strings.TrimSpace(request.Actor), ReclaimedAt: request.QuarantinedAt.UTC(),
		SourcePath: dir, Residue: residue, MalformedLegacyFreeze: &proof,
	})
}

func inspectMalformedLegacyFreeze(ctx context.Context, store Store, expectedHead string) (LegacyMalformedFreezeProof, error) {
	head, err := readRevision(filepath.Join(store.Dir, "HEAD"))
	if err != nil {
		return LegacyMalformedFreezeProof{}, err
	}
	if head != expectedHead {
		return LegacyMalformedFreezeProof{}, fmt.Errorf("%w: expected legacy HEAD %q, current %q", ErrConcurrentUpdate, expectedHead, head)
	}
	visited := map[string]bool{}
	reverseRecords := []Record{}
	reverseRevisions := []string{}
	for revision := head; revision != ""; {
		if visited[revision] {
			return LegacyMalformedFreezeProof{}, errors.New("review quarantine-legacy refused: predecessor cycle")
		}
		visited[revision] = true
		record, loaded, err := store.loadRevision(revision)
		if err != nil {
			return LegacyMalformedFreezeProof{}, fmt.Errorf("review quarantine-legacy refused: load revision %s: %w", revision, err)
		}
		reverseRecords = append(reverseRecords, record)
		reverseRevisions = append(reverseRevisions, loaded)
		revision = record.PreviousRevision
	}
	records := make([]Record, len(reverseRecords))
	revisions := make([]string, len(reverseRevisions))
	for index := range reverseRecords {
		reverse := len(reverseRecords) - 1 - index
		records[index], revisions[index] = reverseRecords[reverse], reverseRevisions[reverse]
	}
	if len(records) == 0 || !validInitialStoreRecord(records[0]) || records[0].Transaction.LineageID != store.lineageID {
		return LegacyMalformedFreezeProof{}, errors.New("review quarantine-legacy refused: chain genesis is invalid")
	}
	var proof *LegacyMalformedFreezeProof
	for index := 1; index < len(records); index++ {
		if records[index].PreviousRevision != revisions[index-1] {
			return LegacyMalformedFreezeProof{}, errors.New("review quarantine-legacy refused: predecessor revision is discontinuous")
		}
		err := validatePersistedV1Successor(records[index-1].Transaction, records[index].Transaction, records[index].Operation, index)
		if err == nil {
			continue
		}
		candidate := records[index]
		if proof != nil || candidate.Operation != "review/freeze-findings" ||
			err.Error() != ErrInvalidSuccessor.Error()+": "+legacyMalformedFreezeDiagnostic {
			return LegacyMalformedFreezeProof{}, fmt.Errorf("review quarantine-legacy refused unsupported successor anomaly at %s: %w", revisions[index], err)
		}
		expected := historicalFreezeFindingsExpected(records[index-1].Transaction, candidate.Transaction)
		changed, err := changedTransactionFields(expected, candidate.Transaction)
		if err != nil || len(changed) == 0 {
			return LegacyMalformedFreezeProof{}, fmt.Errorf("review quarantine-legacy could not derive changed fields: %w", err)
		}
		proof = &LegacyMalformedFreezeProof{
			LineageID: store.lineageID, HeadRevision: head, ChainIdentity: chainIdentity(revisions),
			EventRevision: revisions[index], PreviousRevision: revisions[index-1],
			Operation: candidate.Operation, Diagnostic: legacyMalformedFreezeDiagnostic,
			ChangedFields: changed,
		}
	}
	if proof == nil {
		return LegacyMalformedFreezeProof{}, errors.New("review quarantine-legacy refused: lineage has no supported malformed findings-freeze event")
	}
	if err := validateRepositoryBudgetEvidence(ctx, store.repo, records); err != nil {
		return LegacyMalformedFreezeProof{}, fmt.Errorf("review quarantine-legacy refused repository evidence: %w", err)
	}
	return *proof, nil
}

func changedTransactionFields(expected, actual Transaction) ([]string, error) {
	var left, right map[string]json.RawMessage
	leftPayload, err := json.Marshal(expected)
	if err != nil {
		return nil, err
	}
	rightPayload, err := json.Marshal(actual)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(leftPayload, &left); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rightPayload, &right); err != nil {
		return nil, err
	}
	fields := make([]string, 0)
	for field, expectedValue := range left {
		if !reflect.DeepEqual(expectedValue, right[field]) {
			fields = append(fields, field)
		}
	}
	for field := range right {
		if _, ok := left[field]; !ok {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	return fields, nil
}

func replayCommittedLegacyQuarantine(base, repository string, request LegacyQuarantineRequest) (CompactReclaimRecord, error) {
	quarantineRoot := filepath.Join(base, "quarantine")
	entries, err := os.ReadDir(quarantineRoot)
	if err != nil && !os.IsNotExist(err) {
		return CompactReclaimRecord{}, fmt.Errorf("inspect quarantine for legacy replay: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(quarantineRoot, entry.Name(), "reclaim-record.json"))
		if readErr != nil {
			continue
		}
		var record CompactReclaimRecord
		if json.Unmarshal(payload, &record) != nil || record.Status != CompactReclaimCommitted || record.MalformedLegacyFreeze == nil {
			continue
		}
		proof := record.MalformedLegacyFreeze
		if proof.LineageID != request.LineageID || proof.HeadRevision != request.ExpectedRevision ||
			proof.Diagnostic != request.ExpectedDiagnostic || proof.Disposition != request.Disposition ||
			record.Reason != strings.TrimSpace(request.Reason) || record.Actor != strings.TrimSpace(request.Actor) {
			continue
		}
		if request.MaintainerAuthorization != legacyQuarantineAuthorizationBinding(
			repository, proof.LineageID, proof.HeadRevision, proof.Diagnostic,
			proof.Disposition, request.Actor, request.Reason,
		) {
			continue
		}
		return record, nil
	}
	return CompactReclaimRecord{}, fmt.Errorf("review quarantine-legacy refused: lineage %q is absent and no committed quarantine record matches; inspect prepared records under %s", request.LineageID, quarantineRoot)
}
