# Design: RDD Root Simplification Wave 1 (Shadow Algebra)

## Technical Approach

Wave 1 adds a read-only shadow of the target relation model **inside** `package reviewtransaction` as `shadow_*.go` files. It reuses the live proof functions rather than restating them: `deriveBaseAdvanceCompatibility` (prepr.go:73) is called, never reimplemented (Amendment A), and `classifyCompactTargetRelation` (compact_target_relation.go:45) is the live decision the shadow is measured against. Live behavior is untouched; the product is the differential matrix.

Grounding facts established by code reading:

- `deriveBaseAdvanceCompatibility` takes `refs *resolvedPrePRRefs` and `preimages gateArtifactPreimages` — **both types are unexported**. No sibling package can call it without exporting them.
- Live relation vocabulary is 5 values (`same`, `compatible-advance`, `provable-contraction`, `changed-scope`, `unsafe`); the target is 7. `unrelated`, `ambiguous`, and `unknown` all collapse into `unsafe` today.
- `RepositoryIdentity.RepositoryRef` (repository_locator.go:50, via `OpenRepositoryIdentityLease`) is already the worktree-aware, common-dir-derived repository identity.
- Gate kinds are `GatePostApply|GatePreCommit|GatePrePush|GatePrePR|GateRelease` (receipt.go:133-137).

## Architecture Decisions

| # | Decision | Choice | Rejected alternative | Rationale |
|---|---|---|---|---|
| 1 | Delegation seam (Amendment A) | Shadow lives **inside** `package reviewtransaction` as `shadow_*.go`; calls the unexported function directly; adds **one exported function** (`ObserveShadowRelation`) plus two exported data types (`CandidateIdentity`, `ShadowRelation`) and nothing else | Exported read-only wrapper + sibling `internal/reviewshadow` package | The wrapper is not constructible: two parameter types are unexported. Exporting them grows exactly the surface Wave 0 froze. An interface/DTO seam would force re-encoding the seven conditions — forbidden. Read-only-ness is enforced by an AST guard test instead of by package boundary. |
| 2 | Harness activation | **Opt-in** env switch `GENTLE_AI_RDD_SHADOW`; unset/empty = OFF. Matrix produced fixture-driven in tests, not inline per gate | Inline evaluation on every gate call | `merge-tree --write-tree` plus two `patchIdentity` runs per gate would put shadow Git work on the human's blocking path, breaching the freeze policy's Blocking budget. Unsetting the variable is the wave's rollback boundary. |
| 3 | Divergence destination | In-memory `shadowObserver` sink + deterministic golden matrix under `testdata/`; when the switch is ON, one structured divergence line to **stderr** only | Sidecar divergence journal under `.git/` | A persisted record is precisely the artifact proliferation the root design forbids and would become an operational dependency. stdout is reserved for contract JSON; writing there would break adapters. |
| 4 | `CandidateIdentity` resolver | `{repository_id, base_tree, candidate_tree, changed_paths_modes_digest, policy_hash}`. `repository_id` = `RepositoryRef` from the identity lease. `policy_hash` = `CompactState.PolicyHash`/`Receipt.PolicyHash`; absent ⇒ `unknown`, never fabricated | New repository-id derivation; reuse of `digestPaths` alone | The lease already resolves linked worktrees through the common dir. `digestPaths` is paths-only; modes come from `parseRawDiffModes`/`CandidatePathStatus`. Both digests are recorded so mode-only drift is a measurable divergence class. |
| 5 | Relation function | Ordered evaluation, fail-closed first: ambiguity → unknown → exact → compatible_base_advance (delegated) → provable_contraction → changed → unrelated | Scoring or best-match selection | Ambiguity must be resolved before identity, or recency silently picks a lineage. **No-input degradation (Amendment B):** when admitted-finding paths are unavailable (shadow mode with no live review state), the relation degrades to `changed`, never `provable_contraction` and never `unknown` — absent evidence is not proof, and `changed` is the amendment's literal safe fallback. |
| 6 | Differential matrix | Covering array (~40-60 rows), not the ~540 full cross product: every dimension value plus every semantically reachable pair | Full cross product; live-traffic sampling as primary source | A covering array is provable exit evidence at reviewable size. Row verdicts are `agreement`, `divergence`, `no-live-decision`, `no-shadow-decision` — the last is kept separate because a shadow `unknown` where live decided is the dangerous class. |
| 7 | PR slicing | Six work-unit slices, feature-branch chain on `feature/rdd-root-simplification`, ≤1000 authored lines each | One PR with `size:exception` | Characterization tests must land before the function is elevated to normative. Each slice adding unwired functions runs `scripts/deadcode-ratchet.sh --update` **in that same slice**; slice 5 wires them and re-runs `--update` to drop the entries. |

### Selector → canonical tuple mapping (Decision 4)

| Selector variant | Live target | Note |
|---|---|---|
| workspace | `TargetCurrentChanges` / `ProjectionWorkspace` | |
| staged | `TargetCurrentChanges`, `TargetBaseDiff`, `TargetFixDiff` / `ProjectionStaged` | plus recovery-only `TargetBaseWorkspaceOverlay`/staged |
| committed-range | `TargetBaseDiff`, `TargetExactRevision` | |
| workspace-overlay | `TargetBaseWorkspaceOverlay` / `ProjectionWorkspace` | |

### Shadow (7) → live (5) comparison map (Decision 6)

| Shadow | Live | Comparable? |
|---|---|---|
| `exact` | `same` | Yes — agreement assertable |
| `compatible_base_advance` | `compatible-advance` | Yes |
| `provable_contraction` | `provable-contraction` | Yes |
| `changed` | `changed-scope` | Yes |
| `unrelated` / `ambiguous` / `unknown` | `unsafe` (all three) | **No** — structurally `no-live-decision`; documented, not counted as agreement |

Exit bar: any *unexplained* divergence on the first three rows blocks Wave 2.

## Data Flow

```
selector ──► shadowCandidateIdentity ──┐
                                       ├──► shadowRelate ──► ShadowRelation (7)
frozen authority ──► CandidateIdentity ┘        │
                                                ├─ delegates ─► deriveBaseAdvanceCompatibility
                                                └─ needs ─────► admitted finding paths (may be absent ⇒ changed)

live gate ──► classifyCompactTargetRelation ──► live relation (5)
                         │
                         └──► shadowObserver (OFF by default) ──► in-memory rows ──► golden matrix
```

The observer receives values only, returns nothing, and its error paths are swallowed — an observation failure can never alter a gate outcome.

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/reviewtransaction/prepr_base_advance_characterization_test.go` | Create | Characterization tests for `deriveBaseAdvanceCompatibility` — all seven conditions, success and each failure (SUGGESTION-5 gap: 4 callers, no covering tests) |
| `internal/reviewtransaction/shadow_identity.go` | Create | `CandidateIdentity` + selector-variant resolver |
| `internal/reviewtransaction/shadow_relation.go` | Create | 7-value enum, ordered relation function, Amendment A delegation, Amendment B degradation |
| `internal/reviewtransaction/shadow_authority_health.go` | Create | Read-only `healthy \| repairable \| blocked` graph classifier |
| `internal/reviewtransaction/shadow_observer.go` | Create | `ObserveShadowRelation`, env switch, in-memory sink, stderr divergence line |
| `internal/reviewtransaction/shadow_readonly_guard_test.go` | Create | AST guard: no `shadow_*.go` file references store mutation, `*CompactState` pointer receivers, or write paths |
| `internal/reviewtransaction/shadow_matrix_test.go` | Create | Covering-array corpus + golden emission |
| `internal/reviewtransaction/testdata/shadow-differential-matrix.golden` | Create | Deterministic matrix (generated; excluded from authored line count, included in snapshot identity) |
| `internal/reviewtransaction/gate.go`, `compact_gate.go`, `compact_recovery_binding.go` | Modify | One outcome-neutral observer call each |
| `internal/cli/review_facade.go` | Modify | One outcome-neutral observer call at status/start |
| `.deadcode-baseline.txt` | Modify | Ratchet update in each slice that adds unwired functions |
| `docs/architecture/rdd-root-simplification-design.md` | Modify | Wave 1 exit-evidence pointer only |
| `openspec/specs/rdd-simplification-design/spec.md` | Modify | Correct the Amendment A paraphrase (trust root belongs inside condition 6; condition 7 is base/HEAD non-advance revalidation) — Wave 0 verify follow-up |

## Interfaces / Contracts

```go
type CandidateIdentity struct {
	RepositoryID            string // RepositoryIdentity.RepositoryRef
	BaseTree                string
	CandidateTree           string
	ChangedPathsModesDigest string
	PolicyHash              string
}

type ShadowRelation string // exact | compatible_base_advance | provable_contraction |
                           // changed | unrelated | ambiguous | unknown

type shadowRelationInput struct {
	Frozen, Live                 CandidateIdentity
	FrozenSnapshot, LiveSnapshot Snapshot
	GenesisPaths                 []string
	BaseAdvance                  *BaseAdvanceCompatibility // nil when not derivable
	AdmittedFindingPaths         []string
	AdmittedPathsKnown           bool // false ⇒ contraction degrades to changed
	ApplicableAuthorities        int  // >1 ⇒ ambiguous
}
```

No new persisted schema, contract version, public operation, or state value.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | Seven `deriveBaseAdvanceCompatibility` conditions | Table-driven, `t.TempDir()` + real `git`, one failure case per condition |
| Unit | Relation ordering, Amendment B degradation, ambiguity precedence | Table-driven on `shadowRelationInput` |
| Unit | Read-only property | AST guard test over `shadow_*.go` |
| Integration | Shadow observation is outcome-neutral | Run each of the five gates with switch ON and OFF; assert byte-identical results |
| Golden | Differential matrix | Covering-array corpus → golden; `-update` then re-run without it |

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | **N/A** — no executable-file classification; shadow reads Git trees, never executes repository content | — | — |
| Git repository selection | **Applicable** — resolver runs `git` in a repo directory; linked worktrees and separate git dirs must resolve identically | Repository identity comes only from `OpenRepositoryIdentityLease`; never from a caller-supplied relative path | Linked worktree, separate git dir, and absolute vs relative `repo` argument all yield the same `RepositoryID` |
| Commit state | **Applicable** — staged, workspace, empty index, and unborn HEAD change the candidate tuple | Reuse `SnapshotBuilder`; unborn HEAD and empty index resolve to `unknown`, never to an accidental `exact` | One test per state at `pre-commit` and `post-apply` |
| Push state | **Applicable** — `pre-push`/`pre-pr` resolve a delivery range and remote boundary | Reuse `prePushTargetForRequest`/`prePRBoundaryForRequest`; an unresolvable boundary is `unknown`, fail-closed | First push, tracking branch, explicit refspec |
| PR commands | **N/A** — Wave 1 issues no PR command and composes no `gh` invocation | — | — |

Shadow Git calls are read-only (`merge-tree --write-tree` writes an object, not a ref) and inherit the existing `runGit` environment discipline. They run only behind the opt-in switch or inside tests.

## Migration / Rollout

Off by default. Rollback = unset `GENTLE_AI_RDD_SHADOW` (behavioral) or revert slices 5-6 (structural). No data migration; no persisted state is created or read for writing.

## Open Questions

- [ ] Q1-Q4 from the proposal are recorded assumptions, not maintainer-confirmed. Design proceeds on them; a maintainer amendment on Q3 (exit bar) would change Wave 2 gating only.
- [ ] Whether the paths-only vs paths+modes digest divergence is a real defect class or a benign redundancy (candidate tree already encodes modes) is a matrix output, not a design assumption.
