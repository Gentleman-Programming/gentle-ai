package reviewtransaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	legacyAliasRepairAuthorizationSchema = "gentle-ai.review-legacy-alias-repair-authorization/v1"
	LegacyAliasRepairDisposition         = "quarantine-approved-historical-alias"
	legacyAliasRepairDiagnostic          = "unsupported historical v1 operation alias"
)

// LegacyAliasRepairRequest identifies one exact invalid legacy-v1 lineage.
// It is intentionally a quarantine operation, never a rewrite or reset.
type LegacyAliasRepairRequest struct {
	LineageID               string
	ExpectedRevision        string
	ExpectedDiagnostic      string
	Disposition             string
	Reason                  string
	Actor                   string
	MaintainerAuthorization string
	RepairedAt              time.Time
}

// LegacyAliasRepairProof captures the re-derived chain identity and only the
// approved historical aliases found while replaying its immutable history.
type LegacyAliasRepairProof struct {
	LineageID      string   `json:"lineage_id"`
	HeadRevision   string   `json:"head_revision"`
	ChainIdentity  string   `json:"chain_identity"`
	EventRevisions []string `json:"event_revisions"`
	Operations     []string `json:"operations"`
	Diagnostic     string   `json:"diagnostic"`
	Disposition    string   `json:"disposition"`
}

func legacyAliasRepairAuthorizationBinding(repository, lineage, revision, diagnostic, disposition, actor, reason string) string {
	return legacyAliasRepairAuthorizationSchema + "\nrepository=" + repository +
		"\nlineage=" + lineage + "\nrevision=" + revision +
		"\ndiagnostic=" + diagnostic + "\ndisposition=" + disposition +
		"\nactor=" + strings.TrimSpace(actor) + "\nreason=" + strings.TrimSpace(reason)
}

// RepairHistoricalLegacyAlias quarantines one historical legacy-v1 lineage
// rejected solely by an approved operation alias. It requires the exact HEAD,
// a complete LF authorization binding, a re-derived chain proof, and the
// authority-wide maintenance lease. It never makes historical events valid.
func RepairHistoricalLegacyAlias(ctx context.Context, repo string, request LegacyAliasRepairRequest) (CompactReclaimRecord, error) {
	return repairHistoricalLegacyAlias(ctx, repo, request, legacyAliasRepairOptions{})
}

type legacyAliasRepairOptions struct {
	waitForCooperativeRepair bool
	maintenanceAlreadyHeld   bool
	classifiedAudit          *ClassifiedAuthorityRepairAudit
}

// repairHistoricalLegacyAlias lets the provider-owned classified operation
// wait behind another cooperative repair at the common maintenance lock. The
// compatibility command retains its historical immediate active-owner refusal.
func repairHistoricalLegacyAlias(ctx context.Context, repo string, request LegacyAliasRepairRequest, options legacyAliasRepairOptions) (CompactReclaimRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := validateLineageID(request.LineageID); err != nil {
		return CompactReclaimRecord{}, err
	}
	if !validSHA256(request.ExpectedRevision) {
		return CompactReclaimRecord{}, errors.New("review repair-legacy-alias requires an exact current HEAD revision")
	}
	if strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.Actor) == "" {
		return CompactReclaimRecord{}, errors.New("review repair-legacy-alias requires a non-empty reason and actor")
	}
	if request.ExpectedDiagnostic != legacyAliasRepairDiagnostic || request.Disposition != LegacyAliasRepairDisposition {
		return CompactReclaimRecord{}, errors.New("review repair-legacy-alias supports only the approved unsupported historical v1 operation alias diagnostic and disposition")
	}
	base, repository, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	dir := filepath.Join(base, "v1", request.LineageID)
	if _, statErr := os.Stat(dir); statErr == nil {
		if !options.waitForCooperativeRepair {
			if status := probeAuthorityRepairLock(filepath.Join(dir, "LOCK")); status == authorityRepairLockHeld || status == authorityRepairLockUnknown {
				return CompactReclaimRecord{}, fmt.Errorf("review repair-legacy-alias refused active ownership: %w", ErrConcurrentUpdate)
			}
		}
	} else if !os.IsNotExist(statErr) {
		return CompactReclaimRecord{}, fmt.Errorf("inspect legacy alias repair target: %w", statErr)
	}
	if !options.maintenanceAlreadyHeld {
		maintenance, err := acquireMaintenanceLock(ctx, compactMaintenanceLockPath(base), maintenanceExclusive)
		if err != nil {
			return CompactReclaimRecord{}, err
		}
		defer maintenance.Release()
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return replayCommittedLegacyAliasRepair(base, repository, request)
		}
		return CompactReclaimRecord{}, fmt.Errorf("inspect legacy alias repair target: %w", err)
	}
	if _, err := os.Stat(filepath.Join(base, "v2", request.LineageID)); err == nil {
		return CompactReclaimRecord{}, fmt.Errorf("review repair-legacy-alias refused: lineage %q also exists in compact-v2 authority", request.LineageID)
	} else if !os.IsNotExist(err) {
		return CompactReclaimRecord{}, fmt.Errorf("inspect same-lineage compact authority: %w", err)
	}

	localLock, err := acquireLocalStoreLock(filepath.Join(dir, "LOCK"))
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("review repair-legacy-alias refused active ownership: %w", err)
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

	store := Store{Dir: dir, lineageID: request.LineageID, repo: repository, readOnly: true}
	proof, err := inspectHistoricalLegacyAlias(store, request.ExpectedRevision)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	proof.Disposition = request.Disposition
	if request.MaintainerAuthorization != legacyAliasRepairAuthorizationBinding(
		repository, request.LineageID, request.ExpectedRevision, request.ExpectedDiagnostic,
		request.Disposition, request.Actor, request.Reason,
	) {
		return CompactReclaimRecord{}, fmt.Errorf("review repair-legacy-alias requires an exact maintainer authorization binding (schema %s over repository, lineage, revision, diagnostic, disposition, actor, and reason)", legacyAliasRepairAuthorizationSchema)
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("inspect legacy alias repair target: %w", err)
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		residue = append(residue, item.Name())
	}
	sort.Strings(residue)
	if request.RepairedAt.IsZero() {
		request.RepairedAt = time.Now().UTC()
	}
	if err := releaseLocalLock(); err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("release legacy lineage lock before alias repair: %w", err)
	}
	record, err := quarantineCompactStoreEntry(ctx, base, dir, CompactReclaimRecord{
		Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared,
		LineageID: request.LineageID, Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
		ReclaimedAt: request.RepairedAt.UTC(), SourcePath: dir, Residue: residue, LegacyAliasRepair: &proof,
		ClassifiedAuthorityRepair: options.classifiedAudit,
	})
	if err != nil {
		return record, err
	}
	if err := verifyHistoricalLegacyAliasRepairReadback(ctx, repo, request.LineageID, record); err != nil {
		return record, err
	}
	return record, nil
}

func inspectHistoricalLegacyAlias(store Store, expectedHead string) (LegacyAliasRepairProof, error) {
	head, err := readRevision(filepath.Join(store.Dir, "HEAD"))
	if err != nil {
		return LegacyAliasRepairProof{}, err
	}
	if head != expectedHead {
		return LegacyAliasRepairProof{}, fmt.Errorf("%w: expected legacy HEAD %q, current %q", ErrConcurrentUpdate, expectedHead, head)
	}
	visited := map[string]bool{}
	reverseRecords := []Record{}
	reverseRevisions := []string{}
	for revision := head; revision != ""; {
		if visited[revision] {
			return LegacyAliasRepairProof{}, errors.New("review repair-legacy-alias refused: predecessor cycle")
		}
		visited[revision] = true
		record, loaded, err := store.loadRevision(revision)
		if err != nil {
			return LegacyAliasRepairProof{}, fmt.Errorf("review repair-legacy-alias refused: load revision %s: %w", revision, err)
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
		return LegacyAliasRepairProof{}, errors.New("review repair-legacy-alias refused: chain genesis is invalid")
	}
	proof := LegacyAliasRepairProof{
		LineageID: store.lineageID, HeadRevision: head, ChainIdentity: chainIdentity(revisions),
		EventRevisions: []string{}, Operations: []string{}, Diagnostic: legacyAliasRepairDiagnostic,
	}
	for index := 1; index < len(records); index++ {
		if records[index].PreviousRevision != revisions[index-1] {
			return LegacyAliasRepairProof{}, errors.New("review repair-legacy-alias refused: predecessor revision is discontinuous")
		}
		operation := records[index].Operation
		err := validatePersistedV1Successor(records[index-1].Transaction, records[index].Transaction, operation, index)
		if err == nil {
			continue
		}
		canonical, approved := approvedHistoricalAliasCanonicalOperation(operation)
		if !approved || err.Error() != ErrInvalidSuccessor.Error()+": "+legacyAliasRepairDiagnostic ||
			validateSuccessor(records[index-1].Transaction, records[index].Transaction, canonical) != nil {
			return LegacyAliasRepairProof{}, fmt.Errorf("review repair-legacy-alias refused unsupported successor anomaly at %s: %w", revisions[index], err)
		}
		proof.EventRevisions = append(proof.EventRevisions, revisions[index])
		proof.Operations = append(proof.Operations, operation)
	}
	if len(proof.EventRevisions) == 0 {
		return LegacyAliasRepairProof{}, errors.New("review repair-legacy-alias refused: lineage has no approved unsupported historical operation alias")
	}
	return proof, nil
}

func approvedHistoricalAliasCanonicalOperation(operation string) (string, bool) {
	switch operation {
	case "review/validate-fix":
		return "review/validate-fix-delta", true
	case "review/complete-fix":
		return "review/validate-fix-delta", true
	default:
		return "", false
	}
}

func verifyHistoricalLegacyAliasRepairReadback(ctx context.Context, repo, lineage string, record CompactReclaimRecord) error {
	if record.Status != CompactReclaimCommitted || record.LegacyAliasRepair == nil {
		return errors.New("review repair-legacy-alias did not commit an auditable quarantine record")
	}
	report, err := InventoryAuthority(ctx, repo)
	if err != nil {
		return fmt.Errorf("read back authority inventory after alias repair: %w", err)
	}
	for _, entry := range report.Entries {
		if entry.LineageID == lineage {
			return fmt.Errorf("read back authority inventory still contains repaired lineage %q", lineage)
		}
	}
	return nil
}

func replayCommittedLegacyAliasRepair(base, repository string, request LegacyAliasRepairRequest) (CompactReclaimRecord, error) {
	entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil && !os.IsNotExist(err) {
		return CompactReclaimRecord{}, fmt.Errorf("inspect quarantine for legacy alias replay: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(base, "quarantine", entry.Name(), "reclaim-record.json"))
		if readErr != nil {
			continue
		}
		var record CompactReclaimRecord
		if json.Unmarshal(payload, &record) != nil || record.Status != CompactReclaimCommitted || record.LegacyAliasRepair == nil {
			continue
		}
		proof := record.LegacyAliasRepair
		if proof.LineageID != request.LineageID || proof.HeadRevision != request.ExpectedRevision ||
			proof.Diagnostic != request.ExpectedDiagnostic || proof.Disposition != request.Disposition ||
			record.Reason != strings.TrimSpace(request.Reason) || record.Actor != strings.TrimSpace(request.Actor) {
			continue
		}
		if request.MaintainerAuthorization != legacyAliasRepairAuthorizationBinding(
			repository, proof.LineageID, proof.HeadRevision, proof.Diagnostic, proof.Disposition, request.Actor, request.Reason,
		) {
			continue
		}
		return record, nil
	}
	return CompactReclaimRecord{}, fmt.Errorf("review repair-legacy-alias refused: lineage %q is absent and no committed repair record matches; inspect prepared records under %s", request.LineageID, filepath.Join(base, "quarantine"))
}
