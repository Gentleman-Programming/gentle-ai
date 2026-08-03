// Package reviewtransaction — shadow observer seam (Wave 1, Slice 5). This
// file is part of the read-only shadow of the target RDD relation model
// (docs/architecture/rdd-root-simplification-design.md). It is the ONLY
// production entry point that calls the shadow identity resolver
// (shadow_identity.go), the relation algebra (shadow_relation.go), and the
// authority graph health classifier (shadow_authority_health.go) from a
// live call site — see shadow_readonly_guard_test.go for the AST guard that
// keeps this whole package free of mutation.
//
// ObserveShadowRelation is the third and final symbol design decision 1
// exports for the whole wave (after Slice 2's CandidateIdentity and Slice
// 3's ShadowRelation). It satisfies spec.md's "Advisory-Only, Never
// Blocking" and "Disable Switch Is the Rollback Boundary" requirements: it
// returns nothing, its own error and panic paths are always swallowed, and
// GENTLE_AI_RDD_SHADOW unset or empty makes it a true no-op before any
// repository read, Git command, or in-memory record (design decision 2).
package reviewtransaction

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// shadowObservationEnvVar is the opt-in disable switch (design decision 2).
// Unset or empty (after trimming) means OFF; any other value means ON. It
// is read fresh on every observation rather than cached once, so a test —
// or an operator toggling it between runs — never needs a process restart.
const shadowObservationEnvVar = "GENTLE_AI_RDD_SHADOW"

func shadowObservationEnabled() bool {
	return strings.TrimSpace(os.Getenv(shadowObservationEnvVar)) != ""
}

// shadowObservationRow is one in-memory advisory record (design decision
// 3's "in-memory shadowObserver sink"). It is deliberately unexported and
// never persisted anywhere: spec.md's "No Persisted Divergence Artifact"
// requirement.
type shadowObservationRow struct {
	Gate              GateKind
	LiveResult        GateResult
	Relation          ShadowRelation
	HasRelation       bool
	NoLiveCounterpart bool
	AuthorityHealth   string
	Err               string
}

var (
	shadowObserverMu   sync.Mutex
	shadowObserverRows []shadowObservationRow
	// shadowObserverStderr defaults to the real stderr; tests substitute an
	// in-memory writer so the "stderr only, never stdout" property (design
	// decision 3 / spec.md scenario) is checked without capturing the real
	// process descriptor in every test.
	shadowObserverStderr io.Writer = os.Stderr
	// shadowObserverCallCount increments on every ObserveShadowRelation
	// invocation, ON or OFF. It is the cheapest possible proof that the
	// disable switch's OFF path executes zero shadow work — the hard safety
	// requirement's own suggested "test hook counter" — without inspecting
	// stderr or the sink.
	shadowObserverCallCount int
)

// shadowObserverRowsForTest and shadowResetObserverForTest exist only for
// this package's own tests (shadow_observer_test.go). Nothing in the
// production build ever reads shadowObserverRows back out.
func shadowObserverRowsForTest() []shadowObservationRow {
	shadowObserverMu.Lock()
	defer shadowObserverMu.Unlock()
	rows := make([]shadowObservationRow, len(shadowObserverRows))
	copy(rows, shadowObserverRows)
	return rows
}

func shadowResetObserverForTest() {
	shadowObserverMu.Lock()
	defer shadowObserverMu.Unlock()
	shadowObserverRows = nil
	shadowObserverCallCount = 0
}

func shadowObserverCallCountForTest() int {
	shadowObserverMu.Lock()
	defer shadowObserverMu.Unlock()
	return shadowObserverCallCount
}

// recordShadowObservation is the single write path into the in-memory sink
// and the single stderr writer. Both happen together so a divergence row
// and its stderr line can never drift out of sync.
func recordShadowObservation(row shadowObservationRow) {
	shadowObserverMu.Lock()
	shadowObserverRows = append(shadowObserverRows, row)
	writer := shadowObserverStderr
	shadowObserverMu.Unlock()
	fmt.Fprintf(writer,
		"gentle-ai.rdd-shadow/v1 gate=%s live_result=%s has_relation=%t shadow_relation=%s no_live_counterpart=%t authority_health=%s err=%q\n",
		row.Gate, row.LiveResult, row.HasRelation, row.Relation, row.NoLiveCounterpart, row.AuthorityHealth, row.Err)
}

// ObserveShadowRelation is Wave 1's sole observer seam. It is called once,
// outcome-neutrally, from each of gate.go, compact_gate.go,
// compact_recovery_binding.go, and internal/cli/review_facade.go
// (design.md "File Changes"; spec.md's seven observed call sites: start,
// status, post-apply, pre-commit, pre-push, pre-pr, release). It returns
// nothing and never panics out of this function: every error path is
// swallowed and, when possible, recorded as advisory evidence instead
// (design.md "Data Flow" — "an observation failure can never alter a gate
// outcome").
//
// gate, liveResult, pushRefs, and baseAdvance describe the live decision
// already reached by the caller; ObserveShadowRelation never influences it
// and is always invoked after that decision is final. frozenBaseTree and
// frozenCandidateTree are the authoritative receipt's frozen values; when
// both are empty (for example the "start" call site, which has no prior
// receipt to compare against) the shadow relation is skipped and only
// authority health is observed — no comparison is ever fabricated from
// absent evidence.
//
// When GENTLE_AI_RDD_SHADOW is unset or empty, this function returns
// before resolving a repository identity, running any Git command, or
// touching the in-memory sink (spec.md "Disabling removes all shadow
// execution"). When it is set, this call issues real Git reads through
// shadowCandidateIdentity and shadowAuthorityHealthAtRepo — the
// shared-lock coordination and merge-tree/patch-identity cost design
// decision 2 forbids on every gate call by default is accepted here only
// because the switch is opt-in: nothing on the human's default path pays
// for it.
func ObserveShadowRelation(
	ctx context.Context,
	repo string,
	gate GateKind,
	frozenBaseTree, frozenCandidateTree, frozenPathsDigest, frozenPolicyHash string,
	liveSnapshot Snapshot,
	livePolicyHash string,
	liveResult GateResult,
	pushRefs *resolvedPrePRRefs,
	baseAdvance *BaseAdvanceCompatibility,
) {
	shadowObserverMu.Lock()
	shadowObserverCallCount++
	shadowObserverMu.Unlock()

	if !shadowObservationEnabled() {
		return
	}
	defer func() {
		// A shadow-side panic must never surface as a live failure —
		// recover unconditionally, exactly like every other error path in
		// this function (spec.md "Shadow failure does not block the live
		// path").
		_ = recover()
	}()

	row := shadowObservationRow{Gate: gate, LiveResult: liveResult}

	if health, healthErr := shadowAuthorityHealthAtRepo(ctx, repo); healthErr != nil {
		row.Err = healthErr.Error()
	} else {
		row.AuthorityHealth = string(health)
	}

	if frozenBaseTree != "" || frozenCandidateTree != "" {
		identity, err := shadowCandidateIdentity(ctx, shadowSelector{
			Kind: shadowSelectorWorkspace, Repo: repo, IntendedUntracked: []string{},
		})
		if err != nil {
			if row.Err == "" {
				row.Err = err.Error()
			}
		} else {
			frozen := CandidateIdentity{
				RepositoryID: identity.RepositoryID, BaseTree: frozenBaseTree, CandidateTree: frozenCandidateTree,
				ChangedPathsModesDigest: frozenPathsDigest, PolicyHash: frozenPolicyHash,
			}
			live := CandidateIdentity{
				RepositoryID: identity.RepositoryID, BaseTree: liveSnapshot.BaseTree, CandidateTree: liveSnapshot.CandidateTree,
				ChangedPathsModesDigest: liveSnapshot.PathsDigest, PolicyHash: livePolicyHash,
			}
			unresolvable := false
			if gate == GatePrePush || gate == GatePrePR {
				unresolvable = shadowPushBoundaryUnresolvable(pushRefs, nil)
			}
			row.Relation = shadowRelate(shadowRelationInput{
				Frozen: frozen, Live: live, LiveSnapshot: liveSnapshot,
				BaseAdvance: baseAdvance, ApplicableAuthorities: 1, LiveUnresolvable: unresolvable,
			})
			row.HasRelation = true
			row.NoLiveCounterpart = shadowRelationHasNoLiveCounterpart(row.Relation)
		}
	}

	recordShadowObservation(row)
}
