package reviewtransaction

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

const (
	compactInspectionEntryMissing    = "missing_compact_state"
	compactInspectionEntryUnreadable = "unreadable_compact_state"
	compactInspectionEntryMalformed  = "malformed_compact_state"
	// compactInspectionEntryOutdated classifies a record whose bytes parse
	// intact but whose frozen snapshot identity was computed by an earlier
	// release's retired formula (#2743). It is historical, not damaged:
	// gate-invalid under the clean-break policy, preserved unrewritten for
	// forensics, and never a verdict on any other lineage.
	compactInspectionEntryOutdated   = "outdated_compact_state"
	compactInspectionEntryUnexpected = "unexpected_authority_root_entry"
)

type CompactRecoveryInspectionReport struct {
	Complete         bool                             `json:"complete"`
	Valid            bool                             `json:"valid"`
	Totals           CompactRecoveryInspectionTotals  `json:"totals"`
	Edges            []CompactRecoveryEdgeInspection  `json:"edges"`
	EntryDiagnostics []CompactRecoveryEntryDiagnostic `json:"entry_diagnostics"`
	historical       map[string]historicalCompactForensicRecord
}
type CompactRecoveryInspectionTotals struct {
	CompactEntries   int `json:"compact_entries"`
	LoadedEntries    int `json:"loaded_entries"`
	Edges            int `json:"edges"`
	ValidEdges       int `json:"valid_edges"`
	InvalidEdges     int `json:"invalid_edges"`
	EntryDiagnostics int `json:"entry_diagnostics"`
}

// The closed vocabulary of operations that admit one invalid recovery edge.
// An empty operation means no advertised surface accepts the edge, and the
// exit then carries Blocked instead.
//
// CompactRecoveryEdgeExitReconcile ("review reconcile-authority") retired in
// Wave 7 S3a along with the verb it named: reconciliation's two anomaly
// classes (unchanged_target, malformed_recovery_authorization) have no
// surviving admitting operation — `review repair`'s disposition machinery
// covers only the disjoint content_mismatched_recovery_authorization class
// (authority_disposition_plan.go), never reconciliation's own two. An edge
// classified into either of reconciliation's classes now falls through to
// Blocked, honestly: the operation that used to admit it no longer exists,
// and naming a dead command is worse than naming none (design decision 2,
// rdd-legacy-retirement).
const (
	// CompactRecoveryEdgeExitAbandon is the one operation that admits an
	// invalid recovery edge: `review abandon` accepts any lineage, whatever its
	// state or readability, and cascades to its recovery successors so no
	// orphan edge remains.
	CompactRecoveryEdgeExitAbandon = "review abandon"
	// CompactRecoveryEdgeExitRepair names the existing `review repair` verb
	// for a historical (outdated) entry's selector-scoped disposition. `review
	// abandon` also covers those entries; `repair` is retained only until its
	// own deletion slice.
	CompactRecoveryEdgeExitRepair = "review repair"
)

// CompactRecoverySanctionedExit names, for one invalid recovery edge, the
// operation an operator can actually run right now.
//
// It is deliberately NOT a field of CompactRecoveryEdgeInspection. That struct
// is a binding identity: batch reconciliation re-derives it from the exact
// records under lock and compares it byte-for-byte against the inspection a
// planner declared, so any value that depends on the live store rather than on
// the records alone would make an honest plan look stale. This carries the
// live, operator-facing half separately.
type CompactRecoverySanctionedExit struct {
	SuccessorLineageID string `json:"successor_lineage_id"`
	Operation          string `json:"operation,omitempty"`
	Blocked            string `json:"blocked,omitempty"`
}

type CompactRecoveryEdgeInspection struct {
	PredecessorLineageID        string   `json:"predecessor_lineage_id"`
	RecordedPredecessorRevision string   `json:"recorded_predecessor_revision"`
	ObservedPredecessorRevision string   `json:"observed_predecessor_revision"`
	SuccessorLineageID          string   `json:"successor_lineage_id"`
	SuccessorRevision           string   `json:"successor_revision"`
	Valid                       bool     `json:"valid"`
	AnomalyClasses              []string `json:"anomaly_classes"`
	Problems                    []string `json:"problems"`
	// NonReconcilableReason carries the diagnosis reconciliation already
	// derived when it declined to classify the edge into a reconcilable
	// anomaly class. Without it an operator reads anomaly_classes: [] and is
	// told nothing, while the product holds the exact reason internally and
	// `review reconcile-authority` prints it verbatim on refusal.
	NonReconcilableReason string `json:"non_reconcilable_reason,omitempty"`
}
type CompactRecoveryEntryDiagnostic struct {
	LineageID string `json:"lineage_id"`
	Problem   string `json:"problem"`
}

// InspectCompactRecoveryEdges is read-only and coordinates each record read
// against authority maintenance. Complete covers the initial directory pass,
// not an atomic snapshot, so mutating consumers must re-read under lock/CAS.
func InspectCompactRecoveryEdges(ctx context.Context, repo string) (CompactRecoveryInspectionReport, error) {
	report, _, err := loadCompactRecoveryRecords(ctx, repo)
	return report, err
}

// loadCompactRecoveryRecords is the ONE read-only pass that loads every
// compact-v2 record from repo's authority root AND classifies every recovery
// edge over that exact record set, returning the fully inspected report
// alongside the loaded records. It is a package-level var, not a plain func,
// so a test can prove — by counting calls through a swapped implementation —
// that it is the ONLY function InspectCompactRecoveryEdges and
// deriveAuthorityDispositionPlanAtRepo (authority_disposition_plan.go) call
// for their report/records inputs: no second, independent record-loading
// path ever feeds graph inspection or plan derivation with a different view
// of the store (Wave 2 tasks.md mandatory obligation (a)).
//
// This is InspectCompactRecoveryEdges's former body verbatim, only moved
// down one level with InspectCompactRecoveryEdges left as a thin wrapper: no
// inspection semantics or JSON output changed by this extraction.
var loadCompactRecoveryRecords = func(ctx context.Context, repo string) (CompactRecoveryInspectionReport, map[string]CompactRecord, error) {
	report := CompactRecoveryInspectionReport{Complete: true, Valid: true, Edges: []CompactRecoveryEdgeInspection{}, EntryDiagnostics: []CompactRecoveryEntryDiagnostic{}, historical: map[string]historicalCompactForensicRecord{}}
	base, root, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return report, nil, err
	}
	if err := ensureNoPreparedCompactBatchReconciliation(base); err != nil {
		return report, nil, err
	}
	versionRoot := filepath.Join(base, "v2")
	entries, err := os.ReadDir(versionRoot)
	if os.IsNotExist(err) {
		return report, map[string]CompactRecord{}, nil
	}
	if err != nil {
		return report, nil, fmt.Errorf("inspect compact authority v2 root: %w", err)
	}
	records := make(map[string]CompactRecord, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return report, nil, err
		}
		if !entry.IsDir() {
			if entry.Name() != "LOCK" {
				report.EntryDiagnostics = append(report.EntryDiagnostics, CompactRecoveryEntryDiagnostic{
					LineageID: entry.Name(), Problem: compactInspectionEntryUnexpected,
				})
			}
			continue
		}
		report.Totals.CompactEntries++
		store := CompactStore{Dir: filepath.Join(versionRoot, entry.Name()), lineageID: entry.Name(), repo: root,
			lockPath: filepath.Join(versionRoot, "LOCK"), maintenanceLockPath: compactMaintenanceLockPath(base)}
		record, loadErr := store.Load()
		if loadErr != nil {
			if payload, err := os.ReadFile(store.StatePath()); err == nil {
				if historical, ok := forensicHistoricalCompactRecord(payload, entry.Name()); ok {
					report.historical[entry.Name()] = historical
				}
			}
			report.EntryDiagnostics = append(report.EntryDiagnostics, CompactRecoveryEntryDiagnostic{
				LineageID: entry.Name(), Problem: compactRecoveryEntryProblem(loadErr),
			})
			continue
		}
		report.Totals.LoadedEntries++
		records[record.State.LineageID] = record
	}
	report, err = inspectCompactRecoveryRecordSet(ctx, records, report)
	return report, records, err
}

// SanctionedCompactRecoveryExits resolves, for every invalid edge in one
// inspection, which advertised operation would accept it right now. It is
// read-only, takes no lock, and is the operator-facing companion to the
// inspection rather than part of its binding identity.
//
// Every invalid edge's successor exits through `review abandon`, which accepts
// any lineage regardless of state or readability. The only other exits are the
// retained `review repair` selectors for historical entries, which abandonment
// also covers and which go away with repair's own deletion slice.
func SanctionedCompactRecoveryExits(ctx context.Context, repo string, report CompactRecoveryInspectionReport) ([]CompactRecoverySanctionedExit, error) {
	exits := []CompactRecoverySanctionedExit{}
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return nil, err
	}
	// historicalExits (#2995) lists every historical (outdated) entry when the
	// store's ONLY diagnostics are historical: each entry then has a reachable
	// selector-scoped repair, so inspection surfaces one exit per entry —
	// unless a retained edge still references the entry, which the
	// exact-selector derivation refuses, so the exit is withheld individually
	// (unreferenced siblings stay advertised). The reference is reachable:
	// edges are built from loaded successors, so a loaded successor naming a
	// now-historical predecessor produces it. A
	// store with ANY non-historical diagnostic keeps today's stricter posture —
	// no historical exit is advertised unless the selectorless derivation
	// itself closes (the N=1 j92 case, unchanged below) — because a mixed
	// store's selectorless preflight still refuses and this surface must never
	// advertise a continuation the very next command would refuse. Malformed,
	// unreadable, missing, and unexpected entries never gain an exit.
	historicalExits := historicalDispositionExitLineages(report)
	for _, lineage := range historicalExits {
		exits = append(exits, CompactRecoverySanctionedExit{SuccessorLineageID: lineage, Operation: CompactRecoveryEdgeExitRepair})
	}
	// The selectorless historical-snapshot-identity plan is advertised only
	// when no per-entry historical exit already covers it. A derivation
	// refusal is not propagated: it just means no extra plan exists this round.
	if plan, planErr := deriveAuthorityDispositionPlanAtRepo(ctx, repo, "", ""); planErr == nil && admitClosureDisposition(plan) == nil {
		if plan.AnomalyClass == compactHistoricalSnapshotIdentityClass {
			if len(historicalExits) == 0 {
				exits = append(exits, CompactRecoverySanctionedExit{SuccessorLineageID: plan.SeedSet[0], Operation: CompactRecoveryEdgeExitRepair})
			}
		}
	}
	for _, edge := range report.Edges {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if edge.Valid {
			continue
		}
		exit := CompactRecoverySanctionedExit{SuccessorLineageID: edge.SuccessorLineageID}
		eligibility, err := InspectCompactPristineAbandonment(ctx, root, edge.SuccessorLineageID)
		if err != nil {
			return nil, err
		}
		if eligibility.Eligible {
			// Abandonment accepts every state, so an invalid edge's successor
			// always has this exit; abandoning the successor's own
			// predecessor would cascade through it instead.
			exit.Operation = CompactRecoveryEdgeExitAbandon
		} else {
			exit.Blocked = "the successor entry no longer exists in the review authority store, so there is nothing for `review abandon` to quarantine; re-run the inspection for the current state"
		}
		exits = append(exits, exit)
	}
	return exits, nil
}

// historicalDispositionExitLineages returns every historical (outdated)
// entry lineage, sorted, when the report's diagnostics are ALL historical —
// the one store shape where every diagnostic has a reachable selector-scoped
// repair (#2995). A diagnostic lineage any retained report edge still
// references (on either side) is excluded individually: the exact-selector
// derivation refuses that repair, and this surface must never advertise a
// continuation the very next command refuses — unreferenced siblings stay
// advertised. Any other shape returns nil: malformed, unreadable,
// missing, and unexpected entries never gain an exit, and a store that mixes
// them with historical entries keeps its existing exits unchanged.
func historicalDispositionExitLineages(report CompactRecoveryInspectionReport) []string {
	if len(report.historical) == 0 || len(report.historical) != len(report.EntryDiagnostics) {
		return nil
	}
	lineages := make([]string, 0, len(report.EntryDiagnostics))
	for _, diagnostic := range report.EntryDiagnostics {
		if diagnostic.Problem != compactInspectionEntryOutdated {
			return nil
		}
		if _, found := report.historical[diagnostic.LineageID]; !found {
			return nil
		}
		if historicalLineageEdgeReferenced(report, diagnostic.LineageID) {
			continue
		}
		lineages = append(lineages, diagnostic.LineageID)
	}
	slices.Sort(lineages)
	return lineages
}

// historicalLineageEdgeReferenced reports whether any retained report edge
// still names lineage on either side. The predecessor side of the reference
// is reachable in real stores — edges are built from loaded successors, so a
// loaded successor naming a now-historical predecessor yields the edge
// (#2995) — while the successor side stays drift hardening: a historical
// entry never loads and can never be an edge successor.
func historicalLineageEdgeReferenced(report CompactRecoveryInspectionReport, lineage string) bool {
	return slices.ContainsFunc(report.Edges, func(edge CompactRecoveryEdgeInspection) bool {
		return edge.PredecessorLineageID == lineage || edge.SuccessorLineageID == lineage
	})
}

// inspectCompactRecoveryRecordSet applies the canonical all-edge inspection to
// an already loaded record set. Read-only consumers use it to prove that an
// inspection still describes the exact records they hold.
func inspectCompactRecoveryRecordSet(ctx context.Context, records map[string]CompactRecord, report CompactRecoveryInspectionReport) (CompactRecoveryInspectionReport, error) {
	for lineage, successor := range records {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		recovery := successor.State.Recovery
		if recovery == nil {
			continue
		}
		edge := CompactRecoveryEdgeInspection{
			PredecessorLineageID: recovery.PredecessorLineageID, RecordedPredecessorRevision: recovery.PredecessorRevision,
			SuccessorLineageID: lineage, SuccessorRevision: successor.Revision,
			Valid: true, AnomalyClasses: []string{}, Problems: []string{},
		}
		predecessor, found := records[recovery.PredecessorLineageID]
		if !found {
			edge.Valid = false
			edge.Problems = append(edge.Problems, "missing predecessor")
		} else {
			edge.ObservedPredecessorRevision = predecessor.Revision
			classification := classifyCompactRecoveryEdgeAnomalies(predecessor, successor)
			edge.Valid = classification.Valid
			edge.AnomalyClasses = append(edge.AnomalyClasses, classification.Anomalies...)
			if classification.ValidationError != nil {
				edge.Problems = append(edge.Problems, classification.ValidationError.Error())
			}
			if classification.NonReconcilableError != nil {
				edge.NonReconcilableReason = classification.NonReconcilableError.Error()
			}
		}
		report.Edges = append(report.Edges, edge)
	}
	if err := sortCompactInspection(ctx, report.Edges, func(left, right CompactRecoveryEdgeInspection) int {
		return cmp.Or(cmp.Compare(left.PredecessorLineageID, right.PredecessorLineageID),
			cmp.Compare(left.RecordedPredecessorRevision, right.RecordedPredecessorRevision),
			cmp.Compare(left.SuccessorLineageID, right.SuccessorLineageID), cmp.Compare(left.SuccessorRevision, right.SuccessorRevision))
	}); err != nil {
		return report, err
	}
	if err := markCompactRecoveryForks(ctx, report.Edges); err != nil {
		return report, err
	}
	if err := markCompactRecoveryCycles(ctx, report.Edges); err != nil {
		return report, err
	}
	if err := sortCompactInspection(ctx, report.EntryDiagnostics, func(left, right CompactRecoveryEntryDiagnostic) int {
		return cmp.Or(cmp.Compare(left.LineageID, right.LineageID), cmp.Compare(left.Problem, right.Problem))
	}); err != nil {
		return report, err
	}
	report.Complete = len(report.EntryDiagnostics) == 0
	report.Valid = report.Complete
	report.Totals.Edges = len(report.Edges)
	report.Totals.EntryDiagnostics = len(report.EntryDiagnostics)
	for index := range report.Edges {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if err := sortCompactInspection(ctx, report.Edges[index].Problems, cmp.Compare[string]); err != nil {
			return report, err
		}
		if report.Edges[index].Valid {
			report.Totals.ValidEdges++
		} else {
			report.Totals.InvalidEdges++
			report.Valid = false
		}
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	return report, nil
}
func compactRecoveryEntryProblem(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return compactInspectionEntryMissing
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return compactInspectionEntryUnreadable
	}
	var semantic *CompactSemanticStateError
	if errors.As(err, &semantic) && semantic.OutdatedIdentity {
		return compactInspectionEntryOutdated
	}
	return compactInspectionEntryMalformed
}
func sortCompactInspection[T any](ctx context.Context, values []T, compare func(T, T) int) error {
	var canceled error
	slices.SortFunc(values, func(left, right T) int {
		canceled = ctx.Err()
		if canceled != nil {
			return 0
		}
		return compare(left, right)
	})
	if canceled != nil {
		return canceled
	}
	return ctx.Err()
}
func markCompactRecoveryForks(ctx context.Context, edges []CompactRecoveryEdgeInspection) error {
	children := make(map[string][]int, len(edges))
	for index, edge := range edges {
		if err := ctx.Err(); err != nil {
			return err
		}
		children[edge.PredecessorLineageID] = append(children[edge.PredecessorLineageID], index)
	}
	for _, siblings := range children {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(siblings) < 2 {
			continue
		}
		for cursor := 0; cursor < len(siblings) && ctx.Err() == nil; cursor++ {
			index := siblings[cursor]
			edges[index].Valid = false
			edges[index].Problems = append(edges[index].Problems, "recovery fork")
		}
	}
	return ctx.Err()
}
func markCompactRecoveryCycles(ctx context.Context, edges []CompactRecoveryEdgeInspection) error {
	bySuccessor := make(map[string]int, len(edges))
	for index, edge := range edges {
		if err := ctx.Err(); err != nil {
			return err
		}
		bySuccessor[edge.SuccessorLineageID] = index
	}
	visited := make(map[int]bool, len(edges))
	for start := range edges {
		if err := ctx.Err(); err != nil {
			return err
		}
		path, positions := []int{}, map[int]int{}
		for cursor := start; !visited[cursor]; {
			if err := ctx.Err(); err != nil {
				return err
			}
			if position, cycle := positions[cursor]; cycle {
				for offset := position; offset < len(path) && ctx.Err() == nil; offset++ {
					index := path[offset]
					edges[index].Valid = false
					edges[index].Problems = append(edges[index].Problems, "recovery cycle")
				}
				break
			}
			positions[cursor], path = len(path), append(path, cursor)
			next, found := bySuccessor[edges[cursor].PredecessorLineageID]
			if !found {
				break
			}
			cursor = next
		}
		for cursor := 0; cursor < len(path) && ctx.Err() == nil; cursor++ {
			index := path[cursor]
			visited[index] = true
		}
	}
	return ctx.Err()
}
