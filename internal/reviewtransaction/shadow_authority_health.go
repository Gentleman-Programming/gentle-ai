package reviewtransaction

import "context"

// shadowAuthorityHealth is Wave 1's read-only three-value classification of
// the authority graph, satisfying rdd-authority-graph-classification. Wave 1
// delivers classification only — no disposition plan derivation, quarantine,
// or repair execution. Those are Wave 2 (#1892 leaf-only) and Wave 6 (#2014
// descendant closure) scope; nothing in this file executes either.
type shadowAuthorityHealth string

const (
	shadowAuthorityHealthy    shadowAuthorityHealth = "healthy"
	shadowAuthorityRepairable shadowAuthorityHealth = "repairable"
	shadowAuthorityBlocked    shadowAuthorityHealth = "blocked"
)

// shadowClassifyAuthorityHealth derives healthy|repairable|blocked from an
// already-computed CompactRecoveryInspectionReport (compact_inspect.go) — the
// exact read-only observation `review inspect-authority` and
// `review repair --preflight` (authority_repair.go) already produce. It never
// re-derives the graph, re-reads authority state, or re-parses a record: it
// classifies only over the report's own fields.
//
// Wave 1's repairable class is deliberately narrow: a successor's invalid
// edge must classify into a closed, evidence-backed anomaly class
// (compact_reconcile.go's classifyCompactRecoveryEdgeAnomalies, surfaced here
// as a non-empty AnomalyClasses) AND that successor must be a leaf — no other
// lineage in the same report recovers from it. A successor with descendants
// is a real anomaly the closed class still fits, but repairing it would need
// to reason about what depends on it, which is Wave 6's descendant-closure
// scope, not Wave 1's. Any edge this report could not classify into a closed
// anomaly class, or any incomplete inspection (entry diagnostics present),
// fails closed to blocked — never a generic repairable fallback.
func shadowClassifyAuthorityHealth(report CompactRecoveryInspectionReport) shadowAuthorityHealth {
	if !report.Complete || len(report.EntryDiagnostics) > 0 {
		return shadowAuthorityBlocked
	}
	if report.Valid {
		return shadowAuthorityHealthy
	}
	hasDescendant := make(map[string]bool, len(report.Edges))
	for _, edge := range report.Edges {
		hasDescendant[edge.PredecessorLineageID] = true
	}
	for _, edge := range report.Edges {
		if edge.Valid {
			continue
		}
		if len(edge.AnomalyClasses) == 0 {
			return shadowAuthorityBlocked
		}
		if hasDescendant[edge.SuccessorLineageID] {
			return shadowAuthorityBlocked
		}
	}
	return shadowAuthorityRepairable
}

// shadowAuthorityHealthAtRepo is the thin, read-only production seam: it
// delegates the entire graph inspection to InspectCompactRecoveryEdges
// (compact_inspect.go) — never re-implementing that parsing — and classifies
// only over its output. It issues no Git or filesystem call of its own, takes
// no lock, and creates nothing.
func shadowAuthorityHealthAtRepo(ctx context.Context, repo string) (shadowAuthorityHealth, error) {
	report, err := InspectCompactRecoveryEdges(ctx, repo)
	if err != nil {
		return "", err
	}
	return shadowClassifyAuthorityHealth(report), nil
}
