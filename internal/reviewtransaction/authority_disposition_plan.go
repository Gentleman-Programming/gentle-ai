package reviewtransaction

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// AuthorityDispositionPlanSchema identifies AuthorityDispositionPlan's shape.
const AuthorityDispositionPlanSchema = "gentle-ai.review-authority-disposition-plan/v1"

// AuthorityDispositionPlan is the generic, deterministically-derived
// disposition plan for a closed-classified authority graph anomaly
// (rdd-authority-disposition-plan). It is the one reusable shape for every
// future disposition wave: Wave 2's leaf-only executor
// (authority_disposition_execute.go, Slice S2) admitted only a cardinality-one
// closure; Wave 6 (#2014, #1656) reuses this exact shape for a larger closure
// by relaxing admission only (admitClosureDisposition) — never the plan shape
// or digest domains (design decision 5).
//
// Schema is an explicitly-permitted eleventh serialization field beyond the
// spec's ten (rdd-authority-disposition-plan / "Plan Field Set"). Authorization
// is populated at execution time from the maintainer input and is EXCLUDED
// from the plan_digest pre-image, so plan_digest is derivable pre-
// authorization; the executor validates the populated Authorization against
// the digest-bound plan at execution time (mandatory obligation (b), Wave 2
// Slice S2). Actor and Reason are likewise EXCLUDED from the plan_digest
// pre-image: they are execution-time provenance a maintainer supplies (or,
// for `review repair --preflight`, cannot yet know), not plan identity —
// mirroring Authorization's treatment. This makes plan_digest fully
// derivable read-only, before any actor/reason/authorization exists, and
// actor/reason-independent, so the digest a read-only preflight publishes is
// exactly the digest a later execution (with the real actor/reason) validates
// against, for the same graph state.
type AuthorityDispositionPlan struct {
	Schema                     string                        `json:"schema"`
	RepositoryBinding          string                        `json:"repository_id"`
	AuthorityInventoryRevision string                        `json:"authority_inventory_revision"`
	AnomalyClass               string                        `json:"anomaly_class"`
	Selector                   *AuthorityDispositionSelector `json:"selector,omitempty"`
	SeedSet                    []string                      `json:"ordered_seed_set"`
	Closure                    []string                      `json:"ordered_closure"`
	ExpectedRevisions          map[string]string             `json:"expected_revisions"`
	PlanDigest                 string                        `json:"plan_digest"`
	Actor                      string                        `json:"actor"`
	Reason                     string                        `json:"reason"`
	Authorization              string                        `json:"authorization"`
}

type AuthorityDispositionSelector struct {
	PredecessorLineageID        string `json:"predecessor_lineage_id"`
	PredecessorExpectedRevision string `json:"predecessor_expected_revision"`
	SuccessorLineageID          string `json:"successor_lineage_id"`
	SuccessorExpectedRevision   string `json:"successor_expected_revision"`
}

// compactContentMismatchedRecoveryAuthorizationClass is the one closed
// anomaly class Wave 2 derives a plan for: a recovery successor whose
// persisted maintainer authorization carries the exact
// gentle-ai.review-recovery-authorization/v1 schema prefix but binds
// different content than the successor's own recorded fields — corruption
// rather than a pre-contract legacy authorization
// (classifyCompactRecoveryEdgeAnomalies, compact_reconcile.go). It is
// deliberately not one of CompactRecoveryEdgeInspection's AnomalyClasses:
// that vocabulary makes reconciliation and SanctionedCompactRecoveryExits
// advertise `review reconcile-authority`, which would then refuse this shape
// — a dead end (design decision 2).
const compactContentMismatchedRecoveryAuthorizationClass = "content_mismatched_recovery_authorization"

const compactHistoricalSnapshotIdentityClass = "retired_compact_snapshot_identity"

// errAuthorityDispositionPlanNotDerivable is returned, always wrapped with a
// specific cause, whenever derivation refuses to produce a plan: an
// unclassifiable shape, a mixed/ambiguous set of eligible edges, or a
// diagnostic affecting the selected closure. There is never a generic
// fallback plan.
var errAuthorityDispositionPlanNotDerivable = errors.New("authority disposition plan refused: anomaly classification is not closed") // refusal:by-design human-authority: an unclassifiable or ambiguous graph shape needs a maintainer's diagnosis before any plan can be derived, not a command this refusal can name

func authorityDispositionSelectors(report CompactRecoveryInspectionReport, records map[string]CompactRecord) ([]AuthorityDispositionSelector, error) {
	selectors := []AuthorityDispositionSelector{}
	for _, edge := range report.Edges {
		if edge.Valid {
			continue
		}
		predecessor, foundPredecessor := records[edge.PredecessorLineageID]
		successor, foundSuccessor := records[edge.SuccessorLineageID]
		if !foundPredecessor || !foundSuccessor ||
			classifyCompactRecoveryEdgeAnomalies(predecessor, successor).DispositionClass != compactContentMismatchedRecoveryAuthorizationClass {
			continue
		}
		selectors = append(selectors, AuthorityDispositionSelector{
			PredecessorLineageID: edge.PredecessorLineageID, PredecessorExpectedRevision: predecessor.Revision,
			SuccessorLineageID: edge.SuccessorLineageID, SuccessorExpectedRevision: successor.Revision,
		})
	}
	slices.SortFunc(selectors, func(left, right AuthorityDispositionSelector) int {
		return cmp.Or(cmp.Compare(left.PredecessorLineageID, right.PredecessorLineageID), cmp.Compare(left.SuccessorLineageID, right.SuccessorLineageID))
	})
	return selectors, nil
}

// historicalAuthorityDispositionPlanRecord returns the one released authority
// record that can be planned for quarantine. Its own outdated diagnostic is the
// sole admissible diagnostic: any malformed, unreadable, missing, or unexpected
// additional authority entry prevents a historical plan from being published.
func historicalAuthorityDispositionPlanRecord(report CompactRecoveryInspectionReport) (string, historicalCompactForensicRecord, bool) {
	if len(report.historical) != 1 || len(report.EntryDiagnostics) != 1 {
		return "", historicalCompactForensicRecord{}, false
	}
	for lineage, historical := range report.historical {
		diagnostic := report.EntryDiagnostics[0]
		if diagnostic.LineageID == lineage && diagnostic.Problem == compactInspectionEntryOutdated {
			return lineage, historical, true
		}
	}
	return "", historicalCompactForensicRecord{}, false
}

// deriveAuthorityDispositionPlan derives a generic AuthorityDispositionPlan
// deterministically from report and records — both of which MUST come from
// the single loadCompactRecoveryRecords seam (compact_inspect.go), so no
// second, independent record-loading path ever feeds derivation (mandatory
// obligation (a)). It refuses (no plan) unless the inspection that produced
// selected closure carries no entry diagnostics and exactly one report edge
// re-derives into the one closed content_mismatched_recovery_authorization
// class, except one forensic historical entry whose only diagnostic is outdated.
func deriveAuthorityDispositionPlan(report CompactRecoveryInspectionReport, records map[string]CompactRecord, binding, actor, reason string, requested ...AuthorityDispositionSelector) (AuthorityDispositionPlan, error) {
	if len(requested) > 1 {
		return AuthorityDispositionPlan{}, fmt.Errorf("%w: multiple exact content-mismatch selectors supplied", errAuthorityDispositionPlanNotDerivable)
	}
	selectors, err := authorityDispositionSelectors(report, records)
	if err != nil {
		return AuthorityDispositionPlan{}, err
	}
	if len(requested) == 0 && len(selectors) == 0 {
		if lineage, historical, eligible := historicalAuthorityDispositionPlanRecord(report); eligible {
			inventory, err := authorityInventoryRevision(records, report.historical)
			if err != nil {
				return AuthorityDispositionPlan{}, err
			}
			plan := AuthorityDispositionPlan{Schema: AuthorityDispositionPlanSchema, RepositoryBinding: binding, AuthorityInventoryRevision: inventory, AnomalyClass: compactHistoricalSnapshotIdentityClass, SeedSet: []string{lineage}, Closure: []string{lineage}, ExpectedRevisions: map[string]string{lineage: historical.RawDigest}, Actor: strings.TrimSpace(actor), Reason: strings.TrimSpace(reason)}
			plan.PlanDigest, err = authorityDispositionPlanDigest(plan)
			return plan, err
		}
	}
	var selector *AuthorityDispositionSelector
	if len(requested) == 1 {
		if index := slices.Index(selectors, requested[0]); index >= 0 {
			selector = &selectors[index]
		}
		if selector == nil {
			return AuthorityDispositionPlan{}, fmt.Errorf("%w: exact content-mismatch selector no longer matches the inspected graph", ErrConcurrentUpdate)
		}
	} else if len(selectors) == 1 {
		selector = &selectors[0]
	} else {
		return AuthorityDispositionPlan{}, fmt.Errorf("%w: found %d closed content-mismatch edge(s), want exactly 1 or an exact selector", errAuthorityDispositionPlanNotDerivable, len(selectors))
	}
	seed := selector.SuccessorLineageID
	closure := authorityDispositionClosure(report, seed)
	closureMembers := make(map[string]struct{}, len(closure))
	for _, lineage := range closure {
		closureMembers[lineage] = struct{}{}
	}
	for _, diagnostic := range report.EntryDiagnostics {
		if _, found := closureMembers[diagnostic.LineageID]; found {
			return AuthorityDispositionPlan{}, fmt.Errorf("%w: inspection carries %q diagnostic for closure member %q", errAuthorityDispositionPlanNotDerivable, diagnostic.Problem, diagnostic.LineageID)
		}
	}
	expectedRevisions := make(map[string]string, len(closure))
	for _, lineage := range closure {
		record, found := records[lineage]
		if !found {
			return AuthorityDispositionPlan{}, fmt.Errorf("%w: closure member %q has no loaded record", errAuthorityDispositionPlanNotDerivable, lineage)
		}
		expectedRevisions[lineage] = record.Revision
	}
	inventoryRevision, err := authorityInventoryRevision(records, report.historical)
	if err != nil {
		return AuthorityDispositionPlan{}, err
	}
	plan := AuthorityDispositionPlan{
		Schema: AuthorityDispositionPlanSchema, RepositoryBinding: binding,
		AuthorityInventoryRevision: inventoryRevision, AnomalyClass: compactContentMismatchedRecoveryAuthorizationClass,
		SeedSet: []string{seed}, Closure: closure, ExpectedRevisions: expectedRevisions,
		Actor: strings.TrimSpace(actor), Reason: strings.TrimSpace(reason),
	}
	if len(requested) == 1 {
		plan.Selector = selector
	}
	digest, err := authorityDispositionPlanDigest(plan)
	if err != nil {
		return AuthorityDispositionPlan{}, err
	}
	plan.PlanDigest = digest
	return plan, nil
}

// deriveAuthorityDispositionPlanAtRepo is the production seam that ties
// derivation to the exact same read-only record load InspectCompactRecoveryEdges
// uses: both call loadCompactRecoveryRecords (compact_inspect.go), and
// nothing else loads compact-v2 records for either purpose (mandatory
// obligation (a), Wave 2 tasks.md 1.1). It stays unexported: plan derivation
// has no CLI entrypoint of its own in this slice (rdd-authority-disposition-plan
// / "No New Public Repair Verb") — Slice S3 wires it behind the existing
// `review repair` verb.
func deriveAuthorityDispositionPlanAtRepo(ctx context.Context, repo, actor, reason string, requested ...AuthorityDispositionSelector) (AuthorityDispositionPlan, error) {
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return AuthorityDispositionPlan{}, err
	}
	_, binding, err := authorityRepairRoot(root)
	if err != nil {
		return AuthorityDispositionPlan{}, err
	}
	report, records, err := loadCompactRecoveryRecords(ctx, root)
	if err != nil {
		return AuthorityDispositionPlan{}, err
	}
	return deriveAuthorityDispositionPlan(report, records, binding, actor, reason, requested...)
}

// authorityDispositionClosure derives ordered_closure for one seed by
// following the same report-only successor→descendant edge the Wave 1 leaf
// predicate already proved read-only (shadow_authority_health.go): a
// lineage's descendants are every edge in the same report whose
// PredecessorLineageID names it. It never re-reads authority state or
// consults a cache — only the already-loaded report.
//
// As of Wave 6 (rdd-authority-disposition-plan / "Deterministic Closure
// Derivation From the Graph Source of Record"), ordering is normative, not
// just deterministic: entries emit deepest-descendant-first with the seed
// last via a post-order depth-first walk over the same children map,
// visiting each node's children in lexicographic order before appending the
// node itself — so every prefix of the returned slice is a set of nodes with
// no not-yet-emitted ancestor, which is exactly what makes each ordered
// disposition prefix a valid retained graph (rdd-closure-disposition-execution
// / "Descendant-First Ordered Disposition"). The visited guard doubles as
// cycle protection, matching the old BFS's guarantee. A leaf seed (no report
// edge names it as predecessor) derives closure = {seed} exactly — the
// identity of the pre-Wave-6 lexicographic sort for N=1 — which was Wave 2's
// whole disposition scope; a seed with descendants derives their full
// transitive closure so Wave 6's executor can reuse this exact function
// unchanged (design decision 5).
func authorityDispositionClosure(report CompactRecoveryInspectionReport, seed string) []string {
	children := make(map[string][]string, len(report.Edges))
	for _, edge := range report.Edges {
		children[edge.PredecessorLineageID] = append(children[edge.PredecessorLineageID], edge.SuccessorLineageID)
	}
	for lineage := range children {
		slices.SortFunc(children[lineage], func(left, right string) int { return cmp.Compare(left, right) })
	}

	closure := make([]string, 0, len(children)+1)
	visited := make(map[string]bool, len(children)+1)
	var visitDescendantsFirst func(node string)
	visitDescendantsFirst = func(node string) {
		if visited[node] {
			return
		}
		visited[node] = true
		for _, child := range children[node] {
			visitDescendantsFirst(child)
		}
		closure = append(closure, node)
	}
	visitDescendantsFirst(seed)
	return closure
}

// authorityInventoryRevision digests every loaded record's lineage and
// revision — the whole authority store's revision state at derivation time,
// not just the plan's closure — so Authorization's CAS-only staleness guard
// (rdd-authority-disposition-plan / "Authorization Binds to Digest and
// Revision, No Wall-Clock Expiry") can detect drift anywhere in the store,
// including outside the closure. encoding/json sorts map keys, so this is
// already deterministic (the same idiom classifiedAuthorityRepairDigest's own
// doc comment relies on).
func authorityInventoryRevision(records map[string]CompactRecord, historical map[string]historicalCompactForensicRecord) (string, error) {
	revisions := make(map[string]string, len(records)+len(historical))
	for lineage, record := range records {
		revisions[lineage] = record.Revision
	}
	for lineage, record := range historical {
		revisions[lineage] = record.RawDigest
	}
	return classifiedAuthorityRepairDigest("gentle-ai.review-authority-inventory-revision/v1", revisions)
}

// authorityDispositionPlanDigest computes plan_digest over the seven derived
// IDENTITY fields only (schema, repository_id, authority_inventory_revision,
// anomaly_class, ordered_seed_set, ordered_closure, expected_revisions) —
// never plan_digest itself, never authorization, and never actor/reason. Actor
// and Reason are execution-time PROVENANCE, not plan identity: they are
// carried on the plan (Plan Field Set, spec's ten-field carry requirement) and
// still required non-empty at execution time, but they do not participate in
// what makes two plans "the same plan". Excluding them means plan_digest is
// derivable before a maintainer authorizes anything AND before actor/reason
// are known at all (e.g. `review repair --preflight`, which always derives
// with empty actor/reason) — the executor validates the populated
// Authorization against the digest-bound plan at execution time (design
// decision 1; rdd-authority-disposition-plan / "Plan Digest Binds Exact
// Content").
func authorityDispositionPlanDigest(plan AuthorityDispositionPlan) (string, error) {
	canonical := struct {
		Schema                     string                        `json:"schema"`
		RepositoryBinding          string                        `json:"repository_id"`
		AuthorityInventoryRevision string                        `json:"authority_inventory_revision"`
		AnomalyClass               string                        `json:"anomaly_class"`
		Selector                   *AuthorityDispositionSelector `json:"selector,omitempty"`
		SeedSet                    []string                      `json:"ordered_seed_set"`
		Closure                    []string                      `json:"ordered_closure"`
		ExpectedRevisions          map[string]string             `json:"expected_revisions"`
	}{
		Schema: plan.Schema, RepositoryBinding: plan.RepositoryBinding,
		AuthorityInventoryRevision: plan.AuthorityInventoryRevision, AnomalyClass: plan.AnomalyClass,
		Selector: plan.Selector, SeedSet: plan.SeedSet, Closure: plan.Closure, ExpectedRevisions: plan.ExpectedRevisions,
	}
	return classifiedAuthorityRepairDigest("gentle-ai.review-disposition-plan-digest/v1", canonical)
}

// authorityDispositionAuthorizationSchema is the first line of the exact
// seven-line gentle-ai.review-disposition-authorization/v1 binding a
// maintainer must supply verbatim (rdd-authority-disposition-plan /
// "Authorization Binds to Digest and Revision, No Wall-Clock Expiry",
// pending-confirmation assumption 1).
const authorityDispositionAuthorizationSchema = "gentle-ai.review-disposition-authorization/v1"

// AuthorityDispositionAuthorizationSchema is the exported form of
// authorityDispositionAuthorizationSchema for Wave 6 Slice S4's negotiated-
// transition wiring (internal/cli), which needs to publish the disposition
// collect{}'s schema without duplicating the literal.
const AuthorityDispositionAuthorizationSchema = authorityDispositionAuthorizationSchema

// authorityDispositionAuthorizationBinding renders the exact authorization
// text a maintainer must supply for plan to be admitted at execution time,
// shaped like authorityRepairAuthorizationBinding: schema, repository,
// class, plan_digest, inventory revision, actor, reason. There is
// deliberately no expiry timestamp field anywhere in this binding.
func authorityDispositionAuthorizationBinding(plan AuthorityDispositionPlan) string {
	return authorityDispositionAuthorizationSchema +
		"\nschema=" + plan.Schema +
		"\nrepository=" + plan.RepositoryBinding +
		"\nclass=" + plan.AnomalyClass +
		"\nplan_digest=" + plan.PlanDigest +
		"\ninventory_revision=" + plan.AuthorityInventoryRevision +
		"\nactor=" + plan.Actor +
		"\nreason=" + plan.Reason
}

// validateAuthorityDispositionAuthorization proves an authorized plan's
// Authorization binds to its own plan_digest AND the
// authority_inventory_revision named by callerAuthorityInventoryRevision.
//
// Fix cycle 2 (WARNING-2, sdd-verify cycle-2): callerAuthorityInventoryRevision
// is NOT always the live, freshly re-derived revision — its meaning is
// caller-supplied and depends on the execution path. On a FRESH execution
// (lockedAuthorityDispositionMutation, authority_disposition_execute.go) the
// caller passes the just-re-derived current revision, so this genuinely
// checks the plan against the live store. On a RESUME the caller passes
// plan.AuthorityInventoryRevision — the plan's own FROZEN value, compared to
// itself — which makes the drift half of this check a deliberate no-op
// (a narrowing re-derivation mid-closure is exactly what forward-only resume
// forbids); CAS-all-N is the live guard for the members a resume actually
// disposes. Only the binding half (Authorization matching its own frozen
// plan_digest/inventory_revision) is unconditional on every path — which is
// what makes a forged or mismatched Authorization refuse regardless of
// fresh-vs-resume. No elapsed-time expiry check exists anywhere in this
// function or its caller — CAS on ExpectedRevisions (Slice S2) plus the
// drift comparison (on the paths where it is live) is the entire staleness
// guard (rdd-authority-disposition-plan / "Authorization Binds to Digest and
// Revision, No Wall-Clock Expiry", pending-confirmation assumption 1).
func validateAuthorityDispositionAuthorization(plan AuthorityDispositionPlan, callerAuthorityInventoryRevision string) error {
	if plan.AuthorityInventoryRevision != callerAuthorityInventoryRevision {
		return fmt.Errorf("%w: authority inventory revision drifted from %q to %q", ErrConcurrentUpdate, plan.AuthorityInventoryRevision, callerAuthorityInventoryRevision)
	}
	if plan.Authorization != authorityDispositionAuthorizationBinding(plan) {
		// refusal:by-design human-authority: only a maintainer can supply a correct authorization binding; there is no command that fixes a forged one
		return errors.New("authority disposition plan refused: authorization does not bind to plan_digest and authority_inventory_revision")
	}
	return nil
}

// DeriveAuthorityDispositionPlanAtRepo is the exported form of
// deriveAuthorityDispositionPlanAtRepo for Slice S3's `review repair` CLI
// wiring — a read-only plan derivation with no CLI entrypoint of its own
// (rdd-authority-disposition-plan / "No New Public Repair Verb": this is a Go
// API surface behind the existing verb, not a new command).
func DeriveAuthorityDispositionPlanAtRepo(ctx context.Context, repo, actor, reason string, requested ...AuthorityDispositionSelector) (AuthorityDispositionPlan, error) {
	return deriveAuthorityDispositionPlanAtRepo(ctx, repo, actor, reason, requested...)
}

// ListAuthorityDispositionSelectorsAtRepo exposes exact choices for multi-edge content mismatch.
func ListAuthorityDispositionSelectorsAtRepo(ctx context.Context, repo string) ([]AuthorityDispositionSelector, error) {
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return nil, err
	}
	report, records, err := loadCompactRecoveryRecords(ctx, root)
	if err != nil {
		return nil, err
	}
	return authorityDispositionSelectors(report, records)
}
