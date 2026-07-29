package reviewtransaction

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// CompactBatchReconcileSnapshot binds the operator-visible recovery inspection
// to the exact compact records re-read by the caller. Planning rejects the
// snapshot unless the inspection can be reproduced exactly from these records.
type CompactBatchReconcileSnapshot struct {
	Inspection CompactRecoveryInspectionReport `json:"inspection"`
	Records    []CompactRecord                 `json:"records"`
}

// CompactBatchReconcileEntryIdentity binds one source entry without retaining
// or aliasing its mutable CompactState. The content-addressed revision binds the
// full state, while both target identities make later mutation plans auditable.
type CompactBatchReconcileEntryIdentity struct {
	LineageID             string `json:"lineage_id"`
	Revision              string `json:"revision"`
	Generation            int    `json:"generation"`
	InitialTargetIdentity string `json:"initial_target_identity"`
	CurrentTargetIdentity string `json:"current_target_identity"`
}

// CompactBatchReconcileSuffix names the first invalid successor in one chain
// and the contiguous valid descendant suffix that must move with it. Consecutive
// invalid successors remain represented by InvalidEdges and QuarantineEntries.
type CompactBatchReconcileSuffix struct {
	InvalidRoot CompactBatchReconcileEntryIdentity   `json:"invalid_root"`
	Entries     []CompactBatchReconcileEntryIdentity `json:"entries"`
}

// CompactBatchReconcilePlan is an owned, deterministic plan over exact source
// revisions. Its slices do not alias either the snapshot or declarations.
type CompactBatchReconcilePlan struct {
	InvalidEdges            []CompactRecoveryEdgeInspection      `json:"invalid_edges"`
	InvalidRoots            []CompactBatchReconcileEntryIdentity `json:"invalid_roots"`
	ValidDescendantSuffixes []CompactBatchReconcileSuffix        `json:"valid_descendant_suffixes"`
	QuarantineEntries       []CompactBatchReconcileEntryIdentity `json:"quarantine_entries"`
	RetainedEntries         []CompactBatchReconcileEntryIdentity `json:"retained_entries"`
	RetainedEdges           []CompactRecoveryEdgeInspection      `json:"retained_edges"`
}

// PlanCompactBatchReconciliation derives an all-or-nothing batch plan without
// locks, filesystem access, or mutation. Mutating callers must obtain their
// locks and re-read the exact records before invoking this function.
func PlanCompactBatchReconciliation(snapshot CompactBatchReconcileSnapshot, declaredInvalid []CompactRecoveryEdgeInspection) (CompactBatchReconcilePlan, error) {
	records, inspection, err := validateCompactBatchSnapshot(snapshot)
	if err != nil {
		return CompactBatchReconcilePlan{}, err
	}
	invalidEdges, err := validateCompactBatchDeclarations(inspection, records, declaredInvalid)
	if err != nil {
		return CompactBatchReconcilePlan{}, err
	}
	if len(invalidEdges) == 0 {
		return CompactBatchReconcilePlan{}, compactBatchPlanErrorf("at least one currently invalid recovery edge must be declared")
	}

	chains, err := orderCompactBatchChains(records)
	if err != nil {
		return CompactBatchReconcilePlan{}, err
	}
	invalidSuccessors := make(map[string]bool, len(invalidEdges))
	for _, edge := range invalidEdges {
		invalidSuccessors[edge.SuccessorLineageID] = true
	}

	plan := CompactBatchReconcilePlan{
		InvalidEdges:            cloneCompactRecoveryEdges(invalidEdges),
		InvalidRoots:            []CompactBatchReconcileEntryIdentity{},
		ValidDescendantSuffixes: []CompactBatchReconcileSuffix{},
		QuarantineEntries:       []CompactBatchReconcileEntryIdentity{},
		RetainedEntries:         []CompactBatchReconcileEntryIdentity{},
		RetainedEdges:           []CompactRecoveryEdgeInspection{},
	}
	quarantine := make(map[string]bool)
	for _, chain := range chains {
		firstInvalid, lastInvalid := -1, -1
		seenValidDescendant := false
		for index, lineage := range chain {
			if invalidSuccessors[lineage] {
				if seenValidDescendant {
					return CompactBatchReconcilePlan{}, compactBatchPlanErrorf("chain rooted at %q has a non-contiguous invalid edge closure at %q", chain[0], lineage)
				}
				if firstInvalid < 0 {
					firstInvalid = index
				}
				lastInvalid = index
				continue
			}
			if firstInvalid >= 0 {
				seenValidDescendant = true
			}
		}
		if firstInvalid < 0 {
			for _, lineage := range chain {
				plan.RetainedEntries = append(plan.RetainedEntries, compactBatchEntryIdentity(records[lineage]))
			}
			continue
		}

		root := compactBatchEntryIdentity(records[chain[firstInvalid]])
		plan.InvalidRoots = append(plan.InvalidRoots, root)
		for _, lineage := range chain[:firstInvalid] {
			plan.RetainedEntries = append(plan.RetainedEntries, compactBatchEntryIdentity(records[lineage]))
		}
		for _, lineage := range chain[firstInvalid:] {
			quarantine[lineage] = true
			plan.QuarantineEntries = append(plan.QuarantineEntries, compactBatchEntryIdentity(records[lineage]))
		}
		if lastInvalid+1 < len(chain) {
			entries := make([]CompactBatchReconcileEntryIdentity, 0, len(chain)-lastInvalid-1)
			for _, lineage := range chain[lastInvalid+1:] {
				entries = append(entries, compactBatchEntryIdentity(records[lineage]))
			}
			plan.ValidDescendantSuffixes = append(plan.ValidDescendantSuffixes, CompactBatchReconcileSuffix{
				InvalidRoot: root,
				Entries:     entries,
			})
		}
	}

	retainedRecords := make(map[string]CompactRecord, len(records)-len(quarantine))
	for lineage, record := range records {
		if !quarantine[lineage] {
			retainedRecords[lineage] = record
		}
	}
	if err := validateCompactBatchRetainedGraph(retainedRecords); err != nil {
		return CompactBatchReconcilePlan{}, err
	}
	for _, edge := range inspection.Edges {
		if !quarantine[edge.SuccessorLineageID] {
			plan.RetainedEdges = append(plan.RetainedEdges, cloneCompactRecoveryEdge(edge))
		}
	}
	return plan, nil
}

func validateCompactBatchSnapshot(snapshot CompactBatchReconcileSnapshot) (map[string]CompactRecord, CompactRecoveryInspectionReport, error) {
	if !snapshot.Inspection.Complete || len(snapshot.Inspection.EntryDiagnostics) != 0 {
		return nil, CompactRecoveryInspectionReport{}, compactBatchPlanErrorf("inspection is incomplete")
	}
	records := make(map[string]CompactRecord, len(snapshot.Records))
	for _, record := range snapshot.Records {
		lineage := record.State.LineageID
		if err := validateLineageID(lineage); err != nil {
			return nil, CompactRecoveryInspectionReport{}, compactBatchPlanErrorf("record lineage %q is invalid: %v", lineage, err)
		}
		if _, duplicate := records[lineage]; duplicate {
			return nil, CompactRecoveryInspectionReport{}, compactBatchPlanErrorf("snapshot repeats compact record %q", lineage)
		}
		if record.Schema != compactRecordSchema || !validSHA256(record.Revision) {
			return nil, CompactRecoveryInspectionReport{}, compactBatchPlanErrorf("record %q has invalid schema or revision", lineage)
		}
		if err := record.State.Validate(); err != nil {
			return nil, CompactRecoveryInspectionReport{}, compactBatchPlanErrorf("record %q is invalid: %v", lineage, err)
		}
		if !record.HistoricalCompat {
			revision, err := CompactRevisionForState(record.State)
			if err != nil || revision != record.Revision {
				return nil, CompactRecoveryInspectionReport{}, compactBatchPlanErrorf("record %q revision does not bind its state", lineage)
			}
		}
		records[lineage] = record
	}
	inspection, err := inspectCompactRecoveryRecordSet(context.Background(), records, CompactRecoveryInspectionReport{
		Complete: true,
		Valid:    true,
		Totals: CompactRecoveryInspectionTotals{
			CompactEntries: len(records),
			LoadedEntries:  len(records),
		},
		Edges:            []CompactRecoveryEdgeInspection{},
		EntryDiagnostics: []CompactRecoveryEntryDiagnostic{},
	})
	if err != nil {
		return nil, CompactRecoveryInspectionReport{}, compactBatchPlanErrorf("re-inspect exact records: %v", err)
	}
	if !reflect.DeepEqual(snapshot.Inspection, inspection) {
		return nil, CompactRecoveryInspectionReport{}, compactBatchPlanErrorf("inspection does not match the exact records")
	}
	return records, inspection, nil
}

func validateCompactBatchDeclarations(inspection CompactRecoveryInspectionReport, records map[string]CompactRecord, declared []CompactRecoveryEdgeInspection) ([]CompactRecoveryEdgeInspection, error) {
	currentBySuccessor := make(map[string]CompactRecoveryEdgeInspection, len(inspection.Edges))
	invalid := make([]CompactRecoveryEdgeInspection, 0, inspection.Totals.InvalidEdges)
	for _, edge := range inspection.Edges {
		currentBySuccessor[edge.SuccessorLineageID] = edge
		if edge.Valid {
			continue
		}
		if err := validateSupportedCompactBatchEdge(edge, records); err != nil {
			return nil, err
		}
		invalid = append(invalid, edge)
	}

	seen := make(map[string]bool, len(declared))
	for _, edge := range declared {
		if seen[edge.SuccessorLineageID] {
			return nil, compactBatchPlanErrorf("duplicate declaration for successor %q", edge.SuccessorLineageID)
		}
		seen[edge.SuccessorLineageID] = true
		current, found := currentBySuccessor[edge.SuccessorLineageID]
		if !found || current.Valid {
			return nil, compactBatchPlanErrorf("declared successor %q is not currently invalid", edge.SuccessorLineageID)
		}
		if err := validateCompactBatchAnomalyClasses(edge.AnomalyClasses); err != nil {
			return nil, compactBatchPlanErrorf("declaration for %q: %v", edge.SuccessorLineageID, err)
		}
		if !reflect.DeepEqual(edge, current) {
			return nil, compactBatchPlanErrorf("declaration for successor %q does not match the current invalid edge", edge.SuccessorLineageID)
		}
	}
	if len(seen) < len(invalid) {
		return nil, compactBatchPlanErrorf("declaration set is incomplete: declared %d of %d invalid recovery edges", len(seen), len(invalid))
	}
	if len(seen) != len(invalid) {
		return nil, compactBatchPlanErrorf("declaration set does not exactly match the %d invalid recovery edges", len(invalid))
	}
	for _, edge := range invalid {
		if !seen[edge.SuccessorLineageID] {
			return nil, compactBatchPlanErrorf("declaration set is incomplete: missing successor %q", edge.SuccessorLineageID)
		}
	}
	return invalid, nil
}

func validateSupportedCompactBatchEdge(edge CompactRecoveryEdgeInspection, records map[string]CompactRecord) error {
	predecessor, predecessorFound := records[edge.PredecessorLineageID]
	successor, successorFound := records[edge.SuccessorLineageID]
	if !predecessorFound || !successorFound {
		return compactBatchPlanErrorf("invalid recovery edge %q is outside supported reconciliation classes: %s", edge.SuccessorLineageID, strings.Join(edge.Problems, "; "))
	}
	classification := classifyCompactRecoveryEdgeAnomalies(predecessor, successor)
	if classification.Valid || classification.NonReconcilableError != nil || validateCompactBatchAnomalyClasses(classification.Anomalies) != nil {
		problem := strings.Join(edge.Problems, "; ")
		if problem == "" && classification.NonReconcilableError != nil {
			problem = classification.NonReconcilableError.Error()
		}
		return compactBatchPlanErrorf("invalid recovery edge %q is outside supported reconciliation classes: %s", edge.SuccessorLineageID, problem)
	}
	wantProblems := []string{classification.ValidationError.Error()}
	sort.Strings(wantProblems)
	if !reflect.DeepEqual(edge.Problems, wantProblems) || !reflect.DeepEqual(edge.AnomalyClasses, classification.Anomalies) {
		return compactBatchPlanErrorf("invalid recovery edge %q is outside supported reconciliation classes: %s", edge.SuccessorLineageID, strings.Join(edge.Problems, "; "))
	}
	return nil
}

func validateCompactBatchAnomalyClasses(classes []string) error {
	if len(classes) == 0 {
		return fmt.Errorf("unsupported anomaly class set")
	}
	seen := make(map[string]bool, len(classes))
	for _, class := range classes {
		if class != compactRecoveryEdgeUnchangedTarget && class != compactRecoveryEdgeMalformedAuthorization {
			return fmt.Errorf("unsupported anomaly class %q", class)
		}
		if seen[class] {
			return fmt.Errorf("duplicate anomaly class %q", class)
		}
		seen[class] = true
	}
	return nil
}

func orderCompactBatchChains(records map[string]CompactRecord) ([][]string, error) {
	children := make(map[string]string, len(records))
	roots := make([]string, 0, len(records))
	for lineage, record := range records {
		if record.State.Recovery == nil {
			roots = append(roots, lineage)
			continue
		}
		predecessor := record.State.Recovery.PredecessorLineageID
		if _, found := records[predecessor]; !found {
			return nil, compactBatchPlanErrorf("recovery graph has missing predecessor %q for %q", predecessor, lineage)
		}
		if previous, duplicate := children[predecessor]; duplicate {
			return nil, compactBatchPlanErrorf("recovery graph is ambiguous: predecessor %q has successors %q and %q", predecessor, previous, lineage)
		}
		children[predecessor] = lineage
	}
	sort.Strings(roots)
	chains := make([][]string, 0, len(roots))
	visited := make(map[string]bool, len(records))
	for _, root := range roots {
		chain := []string{}
		for lineage := root; lineage != ""; lineage = children[lineage] {
			if visited[lineage] {
				return nil, compactBatchPlanErrorf("recovery graph has a cycle at %q", lineage)
			}
			visited[lineage] = true
			chain = append(chain, lineage)
		}
		chains = append(chains, chain)
	}
	if len(visited) != len(records) {
		return nil, compactBatchPlanErrorf("recovery graph has an ambiguous or cyclic descendant closure")
	}
	return chains, nil
}

func validateCompactBatchRetainedGraph(records map[string]CompactRecord) error {
	stores := make(map[string]CompactStore, len(records))
	for lineage := range records {
		stores[lineage] = CompactStore{lineageID: lineage}
	}
	if _, err := compactAuthorityLeaves(records, stores); err != nil {
		return compactBatchPlanErrorf("retained graph remains invalid: %v", err)
	}
	return nil
}

func compactBatchEntryIdentity(record CompactRecord) CompactBatchReconcileEntryIdentity {
	return CompactBatchReconcileEntryIdentity{
		LineageID:             record.State.LineageID,
		Revision:              record.Revision,
		Generation:            record.State.Generation,
		InitialTargetIdentity: record.State.InitialSnapshot.Identity,
		CurrentTargetIdentity: record.State.CurrentSnapshot.Identity,
	}
}

func cloneCompactRecoveryEdges(edges []CompactRecoveryEdgeInspection) []CompactRecoveryEdgeInspection {
	cloned := make([]CompactRecoveryEdgeInspection, len(edges))
	for index, edge := range edges {
		cloned[index] = cloneCompactRecoveryEdge(edge)
	}
	return cloned
}

func cloneCompactRecoveryEdge(edge CompactRecoveryEdgeInspection) CompactRecoveryEdgeInspection {
	edge.AnomalyClasses = append([]string(nil), edge.AnomalyClasses...)
	edge.Problems = append([]string(nil), edge.Problems...)
	return edge
}

func compactBatchPlanErrorf(format string, args ...any) error {
	return fmt.Errorf("plan compact batch reconciliation refused: "+format, args...)
}
