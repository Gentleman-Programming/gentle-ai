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

const CompactBatchReconcileJournalSchema = "gentle-ai.review-batch-reconcile-journal/v1"

const (
	CompactBatchReconcilePrepared  = "prepared"
	CompactBatchReconcileCommitted = "committed"
	compactBatchReconcileMarker    = "batch-reconcile-journal.json"
	compactBatchReconcileAudit     = "batch-reconcile-journal.json"
)

var ErrCompactBatchReconcilePrepared = errors.New("compact batch reconciliation is prepared; exact replay is required")

var writeCompactBatchReconcileJournalAtomic = writeAtomic
var renameCompactBatchReconcileEntry = os.Rename
var syncCompactBatchReconcileDirectory = SyncReviewDirectory
var removeCompactBatchReconcileMarker = os.Remove

// CompactBatchReconcileMove binds one whole compact-v2 entry move by relative
// canonical paths, content-addressed state identity, and exact residue names.
type CompactBatchReconcileMove struct {
	LineageID              string   `json:"lineage_id"`
	Revision               string   `json:"revision"`
	InitialTargetIdentity  string   `json:"initial_target_identity"`
	CurrentTargetIdentity  string   `json:"current_target_identity"`
	SourceRelativePath     string   `json:"source_relative_path"`
	QuarantineRelativePath string   `json:"quarantine_relative_path"`
	Residue                []string `json:"residue"`
}

// CompactBatchReconcileJournal is both the global fail-closed marker while an
// operation is prepared and the durable audit record after it commits.
type CompactBatchReconcileJournal struct {
	Schema                        string                          `json:"schema"`
	Status                        string                          `json:"status"`
	RequestSHA256                 string                          `json:"request_sha256"`
	RepositorySHA256              string                          `json:"repository_sha256"`
	PlanSHA256                    string                          `json:"plan_sha256"`
	InvalidEdgesSHA256            string                          `json:"invalid_edges_sha256"`
	ValidDescendantSuffixesSHA256 string                          `json:"valid_descendant_suffixes_sha256"`
	QuarantineEntriesSHA256       string                          `json:"quarantine_entries_sha256"`
	MaintainerAuthorizationSHA256 string                          `json:"maintainer_authorization_sha256"`
	Actor                         string                          `json:"actor"`
	Reason                        string                          `json:"reason"`
	ReconciledAt                  time.Time                       `json:"reconciled_at"`
	AuditRelativePath             string                          `json:"audit_relative_path"`
	DeclaredInvalidEdges          []CompactRecoveryEdgeInspection `json:"declared_invalid_edges"`
	Plan                          CompactBatchReconcilePlan       `json:"plan"`
	Moves                         []CompactBatchReconcileMove     `json:"moves"`
}

// ReconcileInvalidRecoveryEdges executes or exactly replays one prepared
// all-or-nothing batch while holding the authority maintenance lock exclusively
// and the compact-v2 lock. A non-nil error may return the prepared journal;
// callers must preserve it because it identifies the only admissible replay.
func ReconcileInvalidRecoveryEdges(ctx context.Context, repo string, request CompactBatchReconcileRequest) (CompactBatchReconcileJournal, error) {
	if err := validateCompactBatchReconcileRequestShape(request); err != nil {
		return CompactBatchReconcileJournal{}, err
	}
	locks, err := acquireCompactBatchReconcileLocks(ctx, repo, true)
	if err != nil {
		return CompactBatchReconcileJournal{}, err
	}
	defer locks.release()

	expected, err := newCompactBatchReconcileJournal(locks, request)
	if err != nil {
		return CompactBatchReconcileJournal{}, err
	}
	markerPath := compactBatchReconcileMarkerPath(locks.base)
	marker, markerErr := readCompactBatchReconcileJournal(markerPath)
	switch {
	case markerErr == nil:
		if !compactBatchJournalMatchesRequest(marker, expected) {
			return CompactBatchReconcileJournal{}, fmt.Errorf("%w: a different prepared batch owns authority maintenance", ErrCompactBatchReconcilePrepared)
		}
		return replayCompactBatchReconcileJournalLocked(ctx, locks, marker)
	case !os.IsNotExist(markerErr):
		return CompactBatchReconcileJournal{}, fmt.Errorf("%w: global marker is unreadable", ErrCompactBatchReconcilePrepared)
	}

	auditPath := filepath.Join(locks.base, filepath.FromSlash(expected.AuditRelativePath))
	audit, auditErr := readCompactBatchReconcileJournal(auditPath)
	if auditErr == nil {
		if !compactBatchJournalMatchesRequest(audit, expected) {
			return CompactBatchReconcileJournal{}, errors.New("review reconcile-authority-batch refused a conflicting deterministic audit directory")
		}
		if audit.Status == CompactBatchReconcileCommitted {
			if err := verifyCompactBatchMovesAndRetainedGraphLocked(ctx, locks, audit); err != nil {
				return CompactBatchReconcileJournal{}, err
			}
			return audit, nil
		}
		if err := persistCompactBatchReconcileJournal(markerPath, audit); err != nil {
			return audit, err
		}
		return replayCompactBatchReconcileJournalLocked(ctx, locks, audit)
	}
	if !os.IsNotExist(auditErr) {
		return CompactBatchReconcileJournal{}, fmt.Errorf("inspect deterministic compact batch audit: %w", auditErr)
	}

	plan, err := validateCompactBatchReconcileRequestLocked(ctx, locks, request)
	if err != nil {
		return CompactBatchReconcileJournal{}, err
	}
	if !reflect.DeepEqual(plan, expected.Plan) {
		return CompactBatchReconcileJournal{}, fmt.Errorf("%w: prepared plan changed before journal creation", ErrConcurrentUpdate)
	}
	journal, err := bindCompactBatchReconcileMovesLocked(locks, expected)
	if err != nil {
		return CompactBatchReconcileJournal{}, err
	}
	if err := persistCompactBatchReconcileJournal(markerPath, journal); err != nil {
		return journal, err
	}
	return replayCompactBatchReconcileJournalLocked(ctx, locks, journal)
}

func newCompactBatchReconcileJournal(locks *compactBatchReconcileLocks, request CompactBatchReconcileRequest) (CompactBatchReconcileJournal, error) {
	actor, reason, err := validateCompactBatchReconcileIdentity(request.Actor, request.Reason)
	if err != nil {
		return CompactBatchReconcileJournal{}, err
	}
	preparation, err := newCompactBatchReconcilePreparation(locks.repository, request.ExpectedPlan, actor, reason)
	if err != nil {
		return CompactBatchReconcileJournal{}, err
	}
	authorizationSHA256, err := compactBatchReconcileDigest("maintainer-authorization", request.MaintainerAuthorization)
	if err != nil {
		return CompactBatchReconcileJournal{}, err
	}
	identity := compactBatchReconcileRequestIdentity{
		Schema:           CompactBatchReconcileRequestSchema,
		RepositorySHA256: preparation.RepositorySHA256, PlanSHA256: preparation.PlanSHA256,
		InvalidEdgesSHA256:            preparation.InvalidEdgesSHA256,
		ValidDescendantSuffixesSHA256: preparation.ValidDescendantSuffixesSHA256,
		QuarantineEntriesSHA256:       preparation.QuarantineEntriesSHA256,
		MaintainerAuthorizationSHA256: authorizationSHA256,
		Actor:                         actor, Reason: reason,
	}
	requestSHA256, err := compactBatchReconcileDigest("request", identity)
	if err != nil {
		return CompactBatchReconcileJournal{}, err
	}
	plan, err := cloneCompactBatchReconcilePlan(request.ExpectedPlan)
	if err != nil {
		return CompactBatchReconcileJournal{}, err
	}
	batchName := "batch-" + strings.TrimPrefix(requestSHA256, "sha256:")
	return CompactBatchReconcileJournal{
		Schema: CompactBatchReconcileJournalSchema, Status: CompactBatchReconcilePrepared,
		RequestSHA256: requestSHA256, RepositorySHA256: preparation.RepositorySHA256,
		PlanSHA256: preparation.PlanSHA256, InvalidEdgesSHA256: preparation.InvalidEdgesSHA256,
		ValidDescendantSuffixesSHA256: preparation.ValidDescendantSuffixesSHA256,
		QuarantineEntriesSHA256:       preparation.QuarantineEntriesSHA256,
		MaintainerAuthorizationSHA256: authorizationSHA256,
		Actor:                         actor, Reason: reason, ReconciledAt: time.Now().UTC(),
		AuditRelativePath:    filepath.ToSlash(filepath.Join("quarantine", batchName, compactBatchReconcileAudit)),
		DeclaredInvalidEdges: cloneCompactRecoveryEdges(plan.InvalidEdges), Plan: plan,
		Moves: []CompactBatchReconcileMove{},
	}, nil
}

type compactBatchReconcileRequestIdentity struct {
	Schema                        string `json:"schema"`
	RepositorySHA256              string `json:"repository_sha256"`
	PlanSHA256                    string `json:"plan_sha256"`
	InvalidEdgesSHA256            string `json:"invalid_edges_sha256"`
	ValidDescendantSuffixesSHA256 string `json:"valid_descendant_suffixes_sha256"`
	QuarantineEntriesSHA256       string `json:"quarantine_entries_sha256"`
	MaintainerAuthorizationSHA256 string `json:"maintainer_authorization_sha256"`
	Actor                         string `json:"actor"`
	Reason                        string `json:"reason"`
}

func bindCompactBatchReconcileMovesLocked(locks *compactBatchReconcileLocks, journal CompactBatchReconcileJournal) (CompactBatchReconcileJournal, error) {
	batchRoot := filepath.Dir(filepath.Join(locks.base, filepath.FromSlash(journal.AuditRelativePath)))
	entriesRoot := filepath.Join(batchRoot, "entries")
	journal.Moves = make([]CompactBatchReconcileMove, 0, len(journal.Plan.QuarantineEntries))
	for _, entry := range journal.Plan.QuarantineEntries {
		source := filepath.Join(locks.versionRoot, entry.LineageID)
		if err := ensureCompactBatchSourceIdentity(source, entry); err != nil {
			return CompactBatchReconcileJournal{}, err
		}
		items, err := os.ReadDir(source)
		if err != nil {
			return CompactBatchReconcileJournal{}, fmt.Errorf("inspect compact batch source %q: %w", entry.LineageID, err)
		}
		residue := make([]string, len(items))
		for index, item := range items {
			residue[index] = item.Name()
		}
		sort.Strings(residue)
		journal.Moves = append(journal.Moves, CompactBatchReconcileMove{
			LineageID: entry.LineageID, Revision: entry.Revision,
			InitialTargetIdentity: entry.InitialTargetIdentity, CurrentTargetIdentity: entry.CurrentTargetIdentity,
			SourceRelativePath:     filepath.ToSlash(filepath.Join("v2", entry.LineageID)),
			QuarantineRelativePath: filepath.ToSlash(filepath.Join(strings.TrimSuffix(journal.AuditRelativePath, "/"+compactBatchReconcileAudit), "entries", entry.LineageID)),
			Residue:                residue,
		})
	}
	if err := ensureCompactBatchJournalDirectories(locks.base, batchRoot, entriesRoot); err != nil {
		return journal, err
	}
	return journal, nil
}

func replayCompactBatchReconcileJournalLocked(ctx context.Context, locks *compactBatchReconcileLocks, journal CompactBatchReconcileJournal) (CompactBatchReconcileJournal, error) {
	batchRoot := filepath.Dir(filepath.Join(locks.base, filepath.FromSlash(journal.AuditRelativePath)))
	entriesRoot := filepath.Join(batchRoot, "entries")
	if err := ensureCompactBatchJournalDirectories(locks.base, batchRoot, entriesRoot); err != nil {
		return journal, err
	}
	auditPath := filepath.Join(locks.base, filepath.FromSlash(journal.AuditRelativePath))
	if existing, err := readCompactBatchReconcileJournal(auditPath); err == nil {
		if !compactBatchJournalMatchesRequest(existing, journal) {
			return journal, errors.New("review reconcile-authority-batch found a conflicting audit record")
		}
		if existing.Status == CompactBatchReconcileCommitted {
			if err := verifyCompactBatchMovesAndRetainedGraphLocked(ctx, locks, existing); err != nil {
				return journal, err
			}
			if err := clearCompactBatchReconcileMarker(locks.base); err != nil {
				return journal, err
			}
			return existing, nil
		}
	} else if !os.IsNotExist(err) {
		return journal, err
	} else if err := persistCompactBatchReconcileJournal(auditPath, journal); err != nil {
		return journal, err
	}

	for _, move := range journal.Moves {
		if err := ctx.Err(); err != nil {
			return journal, err
		}
		if err := executeCompactBatchReconcileMove(locks, move); err != nil {
			return journal, err
		}
	}
	if err := verifyCompactBatchMovesAndRetainedGraphLocked(ctx, locks, journal); err != nil {
		return journal, err
	}
	committed := journal
	committed.Status = CompactBatchReconcileCommitted
	if err := persistCompactBatchReconcileJournal(auditPath, committed); err != nil {
		return journal, err
	}
	if err := clearCompactBatchReconcileMarker(locks.base); err != nil {
		return journal, err
	}
	return committed, nil
}

func executeCompactBatchReconcileMove(locks *compactBatchReconcileLocks, move CompactBatchReconcileMove) error {
	source := filepath.Join(locks.base, filepath.FromSlash(move.SourceRelativePath))
	destination := filepath.Join(locks.base, filepath.FromSlash(move.QuarantineRelativePath))
	sourceInfo, sourceErr := os.Lstat(source)
	destinationInfo, destinationErr := os.Lstat(destination)
	sourceExists, destinationExists := sourceErr == nil, destinationErr == nil
	if sourceErr != nil && !os.IsNotExist(sourceErr) {
		return sourceErr
	}
	if destinationErr != nil && !os.IsNotExist(destinationErr) {
		return destinationErr
	}
	if sourceExists && destinationExists {
		return fmt.Errorf("compact batch move %q has both source and quarantine destination", move.LineageID)
	}
	if !sourceExists && !destinationExists {
		return fmt.Errorf("compact batch move %q has neither source nor quarantine destination", move.LineageID)
	}
	identity := CompactBatchReconcileEntryIdentity{
		LineageID: move.LineageID, Revision: move.Revision,
		InitialTargetIdentity: move.InitialTargetIdentity, CurrentTargetIdentity: move.CurrentTargetIdentity,
	}
	if destinationExists {
		if !destinationInfo.IsDir() || destinationInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("compact batch destination %q is unsafe", move.LineageID)
		}
		return verifyCompactBatchMovedEntry(destination, identity, move.Residue)
	}
	if !sourceInfo.IsDir() || sourceInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("compact batch source %q is unsafe", move.LineageID)
	}
	if err := ensureCompactBatchSourceIdentity(source, identity); err != nil {
		return err
	}
	if err := verifyCompactBatchResidue(source, move.Residue); err != nil {
		return err
	}
	if err := renameCompactBatchReconcileEntry(source, destination); err != nil {
		return fmt.Errorf("move compact batch source %q into quarantine: %w", move.LineageID, err)
	}
	if err := syncCompactBatchReconcileDirectory(locks.versionRoot); err != nil {
		return fmt.Errorf("sync compact-v2 after moving %q: %w", move.LineageID, err)
	}
	if err := syncCompactBatchReconcileDirectory(filepath.Dir(destination)); err != nil {
		return fmt.Errorf("sync compact batch quarantine after moving %q: %w", move.LineageID, err)
	}
	return verifyCompactBatchMovedEntry(destination, identity, move.Residue)
}

func verifyCompactBatchMovesAndRetainedGraphLocked(ctx context.Context, locks *compactBatchReconcileLocks, journal CompactBatchReconcileJournal) error {
	for _, move := range journal.Moves {
		if err := ctx.Err(); err != nil {
			return err
		}
		source := filepath.Join(locks.base, filepath.FromSlash(move.SourceRelativePath))
		if _, err := os.Lstat(source); !os.IsNotExist(err) {
			return fmt.Errorf("compact batch source %q still exists after planned moves", move.LineageID)
		}
		destination := filepath.Join(locks.base, filepath.FromSlash(move.QuarantineRelativePath))
		identity := CompactBatchReconcileEntryIdentity{
			LineageID: move.LineageID, Revision: move.Revision,
			InitialTargetIdentity: move.InitialTargetIdentity, CurrentTargetIdentity: move.CurrentTargetIdentity,
		}
		if err := verifyCompactBatchMovedEntry(destination, identity, move.Residue); err != nil {
			return err
		}
	}
	snapshot, err := loadCompactBatchReconcileSnapshotLocked(ctx, locks)
	if err != nil {
		return err
	}
	if !snapshot.Inspection.Complete || !snapshot.Inspection.Valid {
		return errors.New("review reconcile-authority-batch retained authority graph is incomplete or invalid")
	}
	records := make(map[string]CompactRecord, len(snapshot.Records))
	for _, record := range snapshot.Records {
		records[record.State.LineageID] = record
	}
	if err := validateCompactBatchRetainedGraph(records); err != nil {
		return err
	}
	chains, err := orderCompactBatchChains(records)
	if err != nil {
		return err
	}
	retained := []CompactBatchReconcileEntryIdentity{}
	for _, chain := range chains {
		for _, lineage := range chain {
			retained = append(retained, compactBatchEntryIdentity(records[lineage]))
		}
	}
	if !reflect.DeepEqual(retained, journal.Plan.RetainedEntries) {
		return errors.New("review reconcile-authority-batch retained graph identity differs from the prepared plan")
	}
	return nil
}

func ensureCompactBatchSourceIdentity(path string, expected CompactBatchReconcileEntryIdentity) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("compact batch source %q is not a canonical directory", expected.LineageID)
	}
	store := CompactStore{Dir: path, lineageID: expected.LineageID}
	record, err := store.loadCompactRecordLocked()
	if err != nil {
		return fmt.Errorf("load compact batch source %q: %w", expected.LineageID, err)
	}
	identity := compactBatchEntryIdentity(record)
	if identity.LineageID != expected.LineageID || identity.Revision != expected.Revision ||
		identity.InitialTargetIdentity != expected.InitialTargetIdentity || identity.CurrentTargetIdentity != expected.CurrentTargetIdentity {
		return fmt.Errorf("%w: compact batch source %q changed revision or target identity", ErrConcurrentUpdate, expected.LineageID)
	}
	return nil
}

func verifyCompactBatchMovedEntry(path string, expected CompactBatchReconcileEntryIdentity, residue []string) error {
	if err := ensureCompactBatchSourceIdentity(path, expected); err != nil {
		return fmt.Errorf("verify quarantined compact batch source: %w", err)
	}
	return verifyCompactBatchResidue(path, residue)
}

func verifyCompactBatchResidue(path string, expected []string) error {
	items, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	actual := make([]string, len(items))
	for index, item := range items {
		actual[index] = item.Name()
	}
	sort.Strings(actual)
	if !reflect.DeepEqual(actual, expected) {
		return errors.New("compact batch residue changed from the prepared audit")
	}
	return nil
}

func ensureCompactBatchJournalDirectories(base, batchRoot, entriesRoot string) error {
	quarantineRoot := filepath.Join(base, "quarantine")
	if err := ensureCanonicalReviewQuarantineRoot(base, quarantineRoot); err != nil {
		return err
	}
	wantPrefix := filepath.Join(quarantineRoot, "batch-")
	if !strings.HasPrefix(filepath.Clean(batchRoot), wantPrefix) || filepath.Dir(batchRoot) != quarantineRoot || filepath.Dir(entriesRoot) != batchRoot {
		return errors.New("review reconcile-authority-batch refused noncanonical quarantine paths")
	}
	for _, path := range []string{quarantineRoot, batchRoot, entriesRoot} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("review reconcile-authority-batch quarantine path is unsafe")
		}
		if err := syncCompactBatchReconcileDirectory(filepath.Dir(path)); err != nil {
			return fmt.Errorf("sync compact batch quarantine parent: %w", err)
		}
	}
	return nil
}

func clearCompactBatchReconcileMarker(base string) error {
	path := compactBatchReconcileMarkerPath(base)
	if err := removeCompactBatchReconcileMarker(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove committed compact batch marker: %w", err)
	}
	if err := syncCompactBatchReconcileDirectory(base); err != nil {
		return fmt.Errorf("sync authority root after compact batch marker removal: %w", err)
	}
	return nil
}

func compactBatchReconcileMarkerPath(base string) string {
	return filepath.Join(base, compactBatchReconcileMarker)
}

func persistCompactBatchReconcileJournal(path string, journal CompactBatchReconcileJournal) error {
	payload, err := marshalCompactBatchReconcileJournal(journal)
	if err != nil {
		return err
	}
	writeErr := writeCompactBatchReconcileJournalAtomic(path, payload, 0o600)
	var syncErr *directorySyncError
	if errors.As(writeErr, &syncErr) {
		return writeErr
	}
	reloaded, err := readCompactBatchReconcileJournal(path)
	if err != nil {
		if writeErr != nil {
			return writeErr
		}
		return fmt.Errorf("reread compact batch reconciliation journal: %w", err)
	}
	if !reflect.DeepEqual(reloaded, journal) {
		if writeErr != nil {
			return writeErr
		}
		return errors.New("compact batch reconciliation journal replacement is ambiguous")
	}
	return nil
}

func marshalCompactBatchReconcileJournal(journal CompactBatchReconcileJournal) ([]byte, error) {
	if err := validateCompactBatchReconcileJournal(journal); err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func readCompactBatchReconcileJournal(path string) (CompactBatchReconcileJournal, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return CompactBatchReconcileJournal{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var journal CompactBatchReconcileJournal
	if err := decoder.Decode(&journal); err != nil {
		return CompactBatchReconcileJournal{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return CompactBatchReconcileJournal{}, errors.New("multiple JSON values in compact batch reconciliation journal")
	}
	if err := validateCompactBatchReconcileJournal(journal); err != nil {
		return CompactBatchReconcileJournal{}, err
	}
	return journal, nil
}

func validateCompactBatchReconcileJournal(journal CompactBatchReconcileJournal) error {
	if journal.Schema != CompactBatchReconcileJournalSchema ||
		(journal.Status != CompactBatchReconcilePrepared && journal.Status != CompactBatchReconcileCommitted) ||
		!validSHA256(journal.RequestSHA256) || !validSHA256(journal.RepositorySHA256) || !validSHA256(journal.PlanSHA256) ||
		!validSHA256(journal.InvalidEdgesSHA256) || !validSHA256(journal.ValidDescendantSuffixesSHA256) ||
		!validSHA256(journal.QuarantineEntriesSHA256) || !validSHA256(journal.MaintainerAuthorizationSHA256) ||
		journal.ReconciledAt.IsZero() || journal.DeclaredInvalidEdges == nil || journal.Plan.InvalidEdges == nil || journal.Moves == nil {
		return errors.New("invalid compact batch reconciliation journal")
	}
	planSHA256, err := compactBatchReconcileDigest("plan", journal.Plan)
	if err != nil || planSHA256 != journal.PlanSHA256 || !reflect.DeepEqual(journal.DeclaredInvalidEdges, journal.Plan.InvalidEdges) {
		return errors.New("compact batch reconciliation journal plan binding is invalid")
	}
	invalidSHA256, _ := compactBatchReconcileDigest("invalid-edges", journal.Plan.InvalidEdges)
	suffixesSHA256, _ := compactBatchReconcileDigest("valid-descendant-suffixes", journal.Plan.ValidDescendantSuffixes)
	quarantineSHA256, _ := compactBatchReconcileDigest("quarantine-entries", journal.Plan.QuarantineEntries)
	if invalidSHA256 != journal.InvalidEdgesSHA256 || suffixesSHA256 != journal.ValidDescendantSuffixesSHA256 || quarantineSHA256 != journal.QuarantineEntriesSHA256 {
		return errors.New("compact batch reconciliation journal guard digest is invalid")
	}
	identity := compactBatchReconcileRequestIdentity{
		Schema:           CompactBatchReconcileRequestSchema,
		RepositorySHA256: journal.RepositorySHA256, PlanSHA256: journal.PlanSHA256,
		InvalidEdgesSHA256:            journal.InvalidEdgesSHA256,
		ValidDescendantSuffixesSHA256: journal.ValidDescendantSuffixesSHA256,
		QuarantineEntriesSHA256:       journal.QuarantineEntriesSHA256,
		MaintainerAuthorizationSHA256: journal.MaintainerAuthorizationSHA256,
		Actor:                         journal.Actor, Reason: journal.Reason,
	}
	requestSHA256, _ := compactBatchReconcileDigest("request", identity)
	batchName := "batch-" + strings.TrimPrefix(journal.RequestSHA256, "sha256:")
	wantAudit := filepath.ToSlash(filepath.Join("quarantine", batchName, compactBatchReconcileAudit))
	if requestSHA256 != journal.RequestSHA256 || journal.AuditRelativePath != wantAudit ||
		len(journal.Moves) != 0 && len(journal.Moves) != len(journal.Plan.QuarantineEntries) {
		return errors.New("compact batch reconciliation journal request identity is invalid")
	}
	for index, move := range journal.Moves {
		entry := journal.Plan.QuarantineEntries[index]
		if move.LineageID != entry.LineageID || move.Revision != entry.Revision ||
			move.InitialTargetIdentity != entry.InitialTargetIdentity || move.CurrentTargetIdentity != entry.CurrentTargetIdentity ||
			move.SourceRelativePath != filepath.ToSlash(filepath.Join("v2", entry.LineageID)) ||
			move.QuarantineRelativePath != filepath.ToSlash(filepath.Join("quarantine", batchName, "entries", entry.LineageID)) ||
			move.Residue == nil || !sort.StringsAreSorted(move.Residue) {
			return errors.New("compact batch reconciliation journal move binding is invalid")
		}
		for _, name := range move.Residue {
			if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
				return errors.New("compact batch reconciliation journal residue name is invalid")
			}
		}
	}
	return nil
}

func compactBatchJournalMatchesRequest(left, right CompactBatchReconcileJournal) bool {
	return left.RequestSHA256 == right.RequestSHA256 && left.RepositorySHA256 == right.RepositorySHA256 &&
		left.PlanSHA256 == right.PlanSHA256 && left.InvalidEdgesSHA256 == right.InvalidEdgesSHA256 &&
		left.ValidDescendantSuffixesSHA256 == right.ValidDescendantSuffixesSHA256 &&
		left.QuarantineEntriesSHA256 == right.QuarantineEntriesSHA256 &&
		left.MaintainerAuthorizationSHA256 == right.MaintainerAuthorizationSHA256 &&
		left.Actor == right.Actor && left.Reason == right.Reason &&
		left.AuditRelativePath == right.AuditRelativePath && reflect.DeepEqual(left.Plan, right.Plan)
}

func cloneCompactBatchReconcilePlan(plan CompactBatchReconcilePlan) (CompactBatchReconcilePlan, error) {
	payload, err := json.Marshal(plan)
	if err != nil {
		return CompactBatchReconcilePlan{}, err
	}
	var cloned CompactBatchReconcilePlan
	if err := json.Unmarshal(payload, &cloned); err != nil {
		return CompactBatchReconcilePlan{}, err
	}
	return cloned, nil
}

func ensureNoPreparedCompactBatchReconciliation(base string) error {
	if _, err := os.Lstat(compactBatchReconcileMarkerPath(base)); err == nil {
		return ErrCompactBatchReconcilePrepared
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("%w: inspect global marker", ErrCompactBatchReconcilePrepared)
	}
	return nil
}
