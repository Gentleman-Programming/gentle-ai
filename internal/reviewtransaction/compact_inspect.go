package reviewtransaction

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	compactInspectionEntryMissing    = "missing_compact_state"
	compactInspectionEntryUnreadable = "unreadable_compact_state"
	compactInspectionEntryMalformed  = "malformed_compact_state"
	compactInspectionEntryUnexpected = "unexpected_authority_root_entry"
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

// The closed vocabulary of operations that admit one invalid recovery edge.
// An empty operation means no advertised surface accepts the edge, and the
// exit then carries Blocked instead.
const (
	CompactRecoveryEdgeExitReconcile = "review reconcile-authority"
	CompactRecoveryEdgeExitAbandon   = "review abandon"
	// CompactRecoveryEdgeExitRepair names the existing `review repair` verb
	// (Wave 2 Slice S3, rdd-authority-disposition-plan) as the sanctioned exit
	// for a closed content_mismatched_recovery_authorization leaf: derivation
	// (deriveAuthorityDispositionPlanAtRepo) and leaf admission
	// (admitLeafDisposition) both accept the edge, so the operation this names
	// will actually run. Unlike CompactRecoveryEdgeExitAbandon, this is never
	// gated on the successor's pristine state — quarantine byte-preserves the
	// whole entry, so nothing captured is discarded.
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
	report := CompactRecoveryInspectionReport{Complete: true, Valid: true, Edges: []CompactRecoveryEdgeInspection{}, EntryDiagnostics: []CompactRecoveryEntryDiagnostic{}}
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
// An edge reconciliation already classified into an anomaly class is
// reconcilable by construction. Everything else — corrupt authorizations,
// forks, cycles, dangling predecessors — is offered `review abandon` only when
// the abandonment gate's own read-only prediction accepts the successor, so
// this surface can never advertise a continuation that would then refuse.
// When neither answers, the exit says so and names the diagnostic instead of a
// command that would not resolve the block.
func SanctionedCompactRecoveryExits(ctx context.Context, repo string, report CompactRecoveryInspectionReport) ([]CompactRecoverySanctionedExit, error) {
	exits := []CompactRecoverySanctionedExit{}
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return nil, err
	}
	// dispositionSeed names the one leaf successor (if any) whose closed
	// content-mismatch classification both derives an AuthorityDispositionPlan
	// and admits as a cardinality-one leaf. A derivation refusal (no eligible
	// edge, more than one, or an incomplete inspection) is not propagated —
	// it just means no edge advertises review repair this round, exactly like
	// InspectCompactPristineAbandonment's per-edge eligibility below never
	// aborts the whole exit computation.
	dispositionSeed := ""
	if plan, planErr := deriveAuthorityDispositionPlanAtRepo(ctx, repo, "", ""); planErr == nil && admitLeafDisposition(plan) == nil {
		dispositionSeed = plan.SeedSet[0]
	}
	for _, edge := range report.Edges {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if edge.Valid {
			continue
		}
		exit := CompactRecoverySanctionedExit{SuccessorLineageID: edge.SuccessorLineageID}
		switch {
		case len(edge.AnomalyClasses) > 0:
			exit.Operation = CompactRecoveryEdgeExitReconcile
		default:
			eligibility, err := InspectCompactPristineAbandonment(ctx, root, edge.SuccessorLineageID)
			if err != nil {
				return nil, err
			}
			switch {
			case eligibility.Eligible:
				// A pristine successor already has a sanctioned exit today —
				// review repair's disposition-plan quarantine is byte-preserving
				// like abandon, so this ordering does not change what a pristine
				// forged edge advertises (`otherwise existing Blocked prose
				// stands unchanged`; abandon is not Blocked prose).
				exit.Operation = CompactRecoveryEdgeExitAbandon
			case dispositionSeed != "" && edge.SuccessorLineageID == dispositionSeed:
				// Only reached once abandon's own prediction refuses — a
				// non-pristine (captured review or correction data) successor
				// whose recovery edge closes on the content-mismatch class.
				// This is #2014's "nothing applies" gap: neither reconcile nor
				// abandon accepted the edge before Wave 2's disposition plan.
				exit.Operation = CompactRecoveryEdgeExitRepair
			default:
				exit.Blocked = "no advertised operation admits this edge: reconciliation does not classify it into a supported anomaly class, and `review abandon` does not accept the successor. " +
					"Nothing quarantines this shape today, so no command clears it and the entry stays exactly as persisted. " +
					"This report, with non_reconcilable_reason, is the artifact to escalate"
			}
		}
		exits = append(exits, exit)
	}
	return exits, nil
}

// CompactAuthorityDamageKinds names, one clause per damaged lineage, the KIND
// of damage one inspection found: an unreadable entry, a dangling predecessor,
// a forged authorization binding, a reconcilable anomaly class. A denial that
// collapses all of them into one generic sentence leaves the operator unable
// to distinguish a truncated file from a missing entry from a forged binding
// while the product holds the exact diagnosis; this is that diagnosis, in the
// same words the inspection publishes machine-readably.
func CompactAuthorityDamageKinds(report CompactRecoveryInspectionReport) []string {
	kinds := []string{}
	for _, diagnostic := range report.EntryDiagnostics {
		switch diagnostic.Problem {
		case compactInspectionEntryMalformed:
			kinds = append(kinds, fmt.Sprintf("store entry %q holds a record that does not parse (%s), which an interrupted write leaves behind", diagnostic.LineageID, diagnostic.Problem))
		case compactInspectionEntryMissing:
			kinds = append(kinds, fmt.Sprintf("store entry %q holds no compact state (%s)", diagnostic.LineageID, diagnostic.Problem))
		case compactInspectionEntryUnreadable:
			kinds = append(kinds, fmt.Sprintf("store entry %q cannot be read (%s)", diagnostic.LineageID, diagnostic.Problem))
		default:
			kinds = append(kinds, fmt.Sprintf("authority root entry %q does not belong there (%s)", diagnostic.LineageID, diagnostic.Problem))
		}
	}
	for _, edge := range report.Edges {
		if edge.Valid {
			continue
		}
		switch {
		case len(edge.AnomalyClasses) > 0:
			kinds = append(kinds, fmt.Sprintf("the recovery edge onto successor %q classifies as %s, which review reconcile-authority admits", edge.SuccessorLineageID, strings.Join(edge.AnomalyClasses, ",")))
		case edge.NonReconcilableReason != "":
			kinds = append(kinds, edge.NonReconcilableReason)
		case anyCompactProblemContains(edge.Problems, "missing predecessor"):
			kinds = append(kinds, fmt.Sprintf("recovery successor %q names predecessor %q, which is missing from the store", edge.SuccessorLineageID, edge.PredecessorLineageID))
		default:
			kinds = append(kinds, fmt.Sprintf("the recovery edge onto successor %q does not re-derive: %s", edge.SuccessorLineageID, strings.Join(edge.Problems, "; ")))
		}
	}
	return kinds
}

func anyCompactProblemContains(problems []string, fragment string) bool {
	for _, problem := range problems {
		if strings.Contains(problem, fragment) {
			return true
		}
	}
	return false
}

// compactStartInvalidGraphRefusal turns the bare invalid-graph refusal REVIEW
// START used to emit into one that names the sanctioned exit the read-only
// inspection proves, following the same rule the reconcile refusal follows: a
// specific operation is named only when the operation's own prediction accepts
// it, so the command printed never turns around and refuses. When no edge has
// a sanctioned exit, only the diagnosis is named — naming a dead end is worse
// than naming nothing.
func compactStartInvalidGraphRefusal(ctx context.Context, repo string, records map[string]CompactRecord, cause error) error {
	report, inspectErr := inspectCompactRecoveryRecordSet(ctx, records, CompactRecoveryInspectionReport{
		Complete: true, Valid: true, Edges: []CompactRecoveryEdgeInspection{}, EntryDiagnostics: []CompactRecoveryEntryDiagnostic{},
	})
	if inspectErr != nil {
		return cause
	}
	exits, exitsErr := SanctionedCompactRecoveryExits(ctx, repo, report)
	if exitsErr != nil {
		return cause
	}
	var continuation strings.Builder
	for _, exit := range exits {
		switch exit.Operation {
		case CompactRecoveryEdgeExitAbandon:
			eligibility, eligibilityErr := InspectCompactPristineAbandonment(ctx, repo, exit.SuccessorLineageID)
			if eligibilityErr != nil || !eligibility.Eligible {
				continue
			}
			fmt.Fprintf(&continuation, " Successor %q is pristine, so quarantining it whole clears its graph defect: %s",
				exit.SuccessorLineageID, compactAbandonCommandText(repo, exit.SuccessorLineageID, eligibility))
		case CompactRecoveryEdgeExitReconcile:
			for _, edge := range report.Edges {
				if edge.SuccessorLineageID != exit.SuccessorLineageID || len(edge.AnomalyClasses) == 0 {
					continue
				}
				fmt.Fprintf(&continuation, " The recovery edge onto successor %q classifies as %s, which reconciliation admits: %s",
					edge.SuccessorLineageID, strings.Join(edge.AnomalyClasses, ","),
					compactReconcileCommandText(repo, edge.PredecessorLineageID, edge.ObservedPredecessorRevision, edge.SuccessorLineageID, edge.SuccessorRevision, edge.AnomalyClasses))
				break
			}
		case CompactRecoveryEdgeExitRepair:
			// Re-derive with the same empty actor/reason `review repair
			// --preflight` always uses (SanctionedCompactRecoveryExits'
			// dispositionSeed check above), and re-confirm admission before
			// naming the command — the same rule the abandon/reconcile cases
			// above already follow, so this never advertises a plan whose own
			// re-derivation would then refuse.
			plan, planErr := deriveAuthorityDispositionPlanAtRepo(ctx, repo, "", "")
			if planErr != nil || admitLeafDisposition(plan) != nil || len(plan.SeedSet) != 1 || plan.SeedSet[0] != exit.SuccessorLineageID {
				continue
			}
			fmt.Fprintf(&continuation, " Successor %q closes the content-mismatched-recovery-authorization class, so `review repair` quarantines it whole without discarding its captured review data: %s",
				exit.SuccessorLineageID, compactRepairCommandText(repo, plan))
		}
	}
	return fmt.Errorf("%v.%s Capture the complete machine-readable diagnosis for every affected lineage with `gentle-ai review inspect-authority --cwd %q`",
		cause, continuation.String(), repo)
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
