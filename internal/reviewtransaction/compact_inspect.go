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
)

type CompactRecoveryInspectionReport struct {
	Complete         bool                             `json:"complete"`
	Valid            bool                             `json:"valid"`
	Totals           CompactRecoveryInspectionTotals  `json:"totals"`
	Edges            []CompactRecoveryEdgeInspection  `json:"edges"`
	EntryDiagnostics []CompactRecoveryEntryDiagnostic `json:"entry_diagnostics"`
}
type CompactRecoveryInspectionTotals struct {
	CompactEntries   int `json:"compact_entries"`
	LoadedEntries    int `json:"loaded_entries"`
	Edges            int `json:"edges"`
	ValidEdges       int `json:"valid_edges"`
	InvalidEdges     int `json:"invalid_edges"`
	EntryDiagnostics int `json:"entry_diagnostics"`
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
}
type CompactRecoveryEntryDiagnostic struct {
	LineageID string `json:"lineage_id"`
	Problem   string `json:"problem"`
}

// InspectCompactRecoveryEdges is lock-free and read-only; Complete covers the initial directory pass, not an atomic snapshot, so mutating consumers must re-read revisions under lock/CAS.
func InspectCompactRecoveryEdges(ctx context.Context, repo string) (CompactRecoveryInspectionReport, error) {
	report := CompactRecoveryInspectionReport{Complete: true, Valid: true, Edges: []CompactRecoveryEdgeInspection{}, EntryDiagnostics: []CompactRecoveryEntryDiagnostic{}}
	base, root, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return report, err
	}
	versionRoot := filepath.Join(base, "v2")
	entries, err := os.ReadDir(versionRoot)
	if os.IsNotExist(err) {
		return report, nil
	}
	if err != nil {
		return report, fmt.Errorf("inspect compact authority v2 root: %w", err)
	}
	records := make(map[string]CompactRecord, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if !entry.IsDir() {
			continue
		}
		report.Totals.CompactEntries++
		store := CompactStore{Dir: filepath.Join(versionRoot, entry.Name()), lineageID: entry.Name(), repo: root,
			lockPath: filepath.Join(versionRoot, "LOCK"), maintenanceLockPath: compactMaintenanceLockPath(base)}
		record, loadErr := store.Load()
		if loadErr != nil {
			report.EntryDiagnostics = append(report.EntryDiagnostics, CompactRecoveryEntryDiagnostic{
				LineageID: entry.Name(), Problem: compactRecoveryEntryProblem(loadErr),
			})
			continue
		}
		report.Totals.LoadedEntries++
		records[record.State.LineageID] = record
	}
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
