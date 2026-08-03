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
// (authority_disposition_execute.go, Slice S2) admits only a cardinality-one
// closure; a future wave (#2014, #1656) reuses this exact shape for a larger
// closure by replacing admission only — never the plan shape or digest
// domains (design decision 5).
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
	Schema                     string            `json:"schema"`
	RepositoryBinding          string            `json:"repository_id"`
	AuthorityInventoryRevision string            `json:"authority_inventory_revision"`
	AnomalyClass               string            `json:"anomaly_class"`
	SeedSet                    []string          `json:"ordered_seed_set"`
	Closure                    []string          `json:"ordered_closure"`
	ExpectedRevisions          map[string]string `json:"expected_revisions"`
	PlanDigest                 string            `json:"plan_digest"`
	Actor                      string            `json:"actor"`
	Reason                     string            `json:"reason"`
	Authorization              string            `json:"authorization"`
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

// errAuthorityDispositionPlanNotDerivable is returned, always wrapped with a
// specific cause, whenever derivation refuses to produce a plan: an
// unclassifiable shape, a mixed/ambiguous set of eligible edges, or an
// incomplete inspection. There is never a generic fallback plan.
var errAuthorityDispositionPlanNotDerivable = errors.New("authority disposition plan refused: anomaly classification is not closed") // refusal:by-design human-authority: an unclassifiable or ambiguous graph shape needs a maintainer's diagnosis before any plan can be derived, not a command this refusal can name

// deriveAuthorityDispositionPlan derives a generic AuthorityDispositionPlan
// deterministically from report and records — both of which MUST come from
// the single loadCompactRecoveryRecords seam (compact_inspect.go), so no
// second, independent record-loading path ever feeds derivation (mandatory
// obligation (a)). It refuses (no plan) unless the inspection that produced
// report carried no entry diagnostics and exactly one report edge re-derives
// into the one closed content_mismatched_recovery_authorization class.
func deriveAuthorityDispositionPlan(report CompactRecoveryInspectionReport, records map[string]CompactRecord, binding, actor, reason string) (AuthorityDispositionPlan, error) {
	if !report.Complete || len(report.EntryDiagnostics) > 0 {
		return AuthorityDispositionPlan{}, fmt.Errorf("%w: inspection carries %d entry diagnostic(s)", errAuthorityDispositionPlanNotDerivable, len(report.EntryDiagnostics))
	}
	seed, seedCount := "", 0
	for _, edge := range report.Edges {
		if edge.Valid {
			continue
		}
		predecessor, foundPredecessor := records[edge.PredecessorLineageID]
		successor, foundSuccessor := records[edge.SuccessorLineageID]
		if !foundPredecessor || !foundSuccessor {
			continue
		}
		if classifyCompactRecoveryEdgeAnomalies(predecessor, successor).DispositionClass == "" {
			continue
		}
		seed = edge.SuccessorLineageID
		seedCount++
	}
	if seedCount != 1 {
		return AuthorityDispositionPlan{}, fmt.Errorf("%w: found %d closed content-mismatch edge(s), want exactly 1", errAuthorityDispositionPlanNotDerivable, seedCount)
	}
	closure := authorityDispositionClosure(report, seed)
	expectedRevisions := make(map[string]string, len(closure))
	for _, lineage := range closure {
		record, found := records[lineage]
		if !found {
			return AuthorityDispositionPlan{}, fmt.Errorf("%w: closure member %q has no loaded record", errAuthorityDispositionPlanNotDerivable, lineage)
		}
		expectedRevisions[lineage] = record.Revision
	}
	inventoryRevision, err := authorityInventoryRevision(records)
	if err != nil {
		return AuthorityDispositionPlan{}, err
	}
	plan := AuthorityDispositionPlan{
		Schema: AuthorityDispositionPlanSchema, RepositoryBinding: binding,
		AuthorityInventoryRevision: inventoryRevision, AnomalyClass: compactContentMismatchedRecoveryAuthorizationClass,
		SeedSet: []string{seed}, Closure: closure, ExpectedRevisions: expectedRevisions,
		Actor: strings.TrimSpace(actor), Reason: strings.TrimSpace(reason),
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
func deriveAuthorityDispositionPlanAtRepo(ctx context.Context, repo, actor, reason string) (AuthorityDispositionPlan, error) {
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
	return deriveAuthorityDispositionPlan(report, records, binding, actor, reason)
}

// authorityDispositionClosure derives ordered_closure for one seed by
// following the same report-only successor→descendant edge the Wave 1 leaf
// predicate already proved read-only (shadow_authority_health.go): a
// lineage's descendants are every edge in the same report whose
// PredecessorLineageID names it. It never re-reads authority state or
// consults a cache — only the already-loaded report. A leaf seed (no report
// edge names it as predecessor) derives closure = {seed} exactly, which is
// Wave 2's whole disposition scope; a seed with descendants derives their
// full transitive closure so a future wave can reuse this exact function
// unchanged (design decision 5).
func authorityDispositionClosure(report CompactRecoveryInspectionReport, seed string) []string {
	children := make(map[string][]string, len(report.Edges))
	for _, edge := range report.Edges {
		children[edge.PredecessorLineageID] = append(children[edge.PredecessorLineageID], edge.SuccessorLineageID)
	}
	visited := map[string]bool{seed: true}
	queue := []string{seed}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, child := range children[current] {
			if visited[child] {
				continue
			}
			visited[child] = true
			queue = append(queue, child)
		}
	}
	closure := make([]string, 0, len(visited))
	for lineage := range visited {
		closure = append(closure, lineage)
	}
	slices.SortFunc(closure, func(left, right string) int { return cmp.Compare(left, right) })
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
func authorityInventoryRevision(records map[string]CompactRecord) (string, error) {
	revisions := make(map[string]string, len(records))
	for lineage, record := range records {
		revisions[lineage] = record.Revision
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
		Schema                     string            `json:"schema"`
		RepositoryBinding          string            `json:"repository_id"`
		AuthorityInventoryRevision string            `json:"authority_inventory_revision"`
		AnomalyClass               string            `json:"anomaly_class"`
		SeedSet                    []string          `json:"ordered_seed_set"`
		Closure                    []string          `json:"ordered_closure"`
		ExpectedRevisions          map[string]string `json:"expected_revisions"`
	}{
		Schema: plan.Schema, RepositoryBinding: plan.RepositoryBinding,
		AuthorityInventoryRevision: plan.AuthorityInventoryRevision, AnomalyClass: plan.AnomalyClass,
		SeedSet: plan.SeedSet, Closure: plan.Closure, ExpectedRevisions: plan.ExpectedRevisions,
	}
	return classifiedAuthorityRepairDigest("gentle-ai.review-disposition-plan-digest/v1", canonical)
}

// authorityDispositionAuthorizationSchema is the first line of the exact
// seven-line gentle-ai.review-disposition-authorization/v1 binding a
// maintainer must supply verbatim (rdd-authority-disposition-plan /
// "Authorization Binds to Digest and Revision, No Wall-Clock Expiry",
// pending-confirmation assumption 1).
const authorityDispositionAuthorizationSchema = "gentle-ai.review-disposition-authorization/v1"

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
// Authorization binds to its own plan_digest AND the CURRENT
// authority_inventory_revision. No elapsed-time expiry check exists anywhere
// in this function or its caller — CAS on ExpectedRevisions (Slice S2) plus
// this revision comparison is the entire staleness guard (rdd-authority-
// disposition-plan / "Authorization Binds to Digest and Revision, No
// Wall-Clock Expiry", pending-confirmation assumption 1).
func validateAuthorityDispositionAuthorization(plan AuthorityDispositionPlan, currentAuthorityInventoryRevision string) error {
	if plan.AuthorityInventoryRevision != currentAuthorityInventoryRevision {
		return fmt.Errorf("%w: authority inventory revision drifted from %q to %q", ErrConcurrentUpdate, plan.AuthorityInventoryRevision, currentAuthorityInventoryRevision)
	}
	if plan.Authorization != authorityDispositionAuthorizationBinding(plan) {
		// refusal:by-design human-authority: only a maintainer can supply a correct authorization binding; there is no command that fixes a forged one
		return errors.New("authority disposition plan refused: authorization does not bind to plan_digest and authority_inventory_revision")
	}
	return nil
}

// compactRepairCommandText renders the exact runnable `review repair`
// invocation for one leaf authority disposition plan a caller already
// confirmed derives and admits (deriveAuthorityDispositionPlanAtRepo +
// admitLeafDisposition — the same read-only prediction `review repair
// --preflight` runs), with the persisted plan_digest and
// authority_inventory_revision preflight would publish and the authorization
// template execution verifies. plan_digest is actor/reason-independent
// (authorityDispositionPlanDigest excludes both from its pre-image), so the
// value rendered here is exactly the value execution re-derives and compares
// against, for whichever actor/reason a maintainer later supplies — the
// command printed is the command that runs. Only a caller whose own
// read-only prediction accepted the plan may render this, mirroring
// compactAbandonCommandText and compactReconcileCommandText
// (compact_reconcile.go).
func compactRepairCommandText(repo string, plan AuthorityDispositionPlan) string {
	template := plan
	template.Actor, template.Reason = "<actor>", "<why-it-is-repaired>"
	return fmt.Sprintf("`gentle-ai review repair --cwd %q --plan-digest %q --inventory-revision %q --actor \"<actor>\" --reason \"<why-it-is-repaired>\" --authorization \"<maintainer-authorization>\"`"+
		" (`gentle-ai review repair --preflight --cwd %q` re-confirms these are still current);"+
		" the repair quarantines the entry whole and rewrites nothing, so the recorded authorization bytes survive exactly as persisted."+
		" --authorization is exactly these seven lines, joined by LF, with no trailing newline, using the same --actor and --reason with surrounding whitespace trimmed:\n%s",
		repo, plan.PlanDigest, plan.AuthorityInventoryRevision, repo, authorityDispositionAuthorizationBinding(template))
}

// DeriveAuthorityDispositionPlanAtRepo is the exported form of
// deriveAuthorityDispositionPlanAtRepo for Slice S3's `review repair` CLI
// wiring — a read-only plan derivation with no CLI entrypoint of its own
// (rdd-authority-disposition-plan / "No New Public Repair Verb": this is a Go
// API surface behind the existing verb, not a new command).
func DeriveAuthorityDispositionPlanAtRepo(ctx context.Context, repo, actor, reason string) (AuthorityDispositionPlan, error) {
	return deriveAuthorityDispositionPlanAtRepo(ctx, repo, actor, reason)
}
