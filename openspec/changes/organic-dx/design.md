# Design: Organic DX

## Technical Approach

Four seams, no new frameworks, no wire-contract change. Stream 1 turns absence of
`--maintainer-authorization` into CLI-side derivation at the one site that already
re-derives the binding. Stream 2 converts the unrouted-stop class into a table-driven
structural test reusing the proven source-extraction harness. Stream 3 derives
escalation accounting instead of persisting it. Stream 4 registers the emission points
that stream 2 finalizes. Stream 5 is asset text pinned by existing anchors.

## Duty 1 — Auto-applicability of the 10 authorization-gated commands

The authorization *string* is derivable for all 10 (audit #8458). The real gate is
whether the *operation* is a deterministic continuation. Verdict per command:

| # | Command | Trigger state | Every input derivable? | Today's routing | Verdict |
|---|---|---|---|---|---|
| 1 | `review recover` | `StateInvalidated`; `CorrectionRequired`+`ActionRecover`; `Approved`+`ActionRecover`; `Escalated` (changed target or accounting-only) | Yes — binding + `ActionDisposition` come from `ReviewTargetStatusResult`; successor is a fresh ID; actor from git; reason is a per-shape constant | `collect recovery_authorization_required` (`review_next_transition.go:409`) | **AUTO — A1** |
| 2 | `review repair` | `TargetApplicabilityCorrupted` + `Repair.Status == eligible` | Yes — class/lineage/revision/cause/disposition/repository-binding are equality-checked against the assessment (`authority_repair.go:823-826`); only actor+reason are free | `reviewRepairTransition` collect (`:103`) | **AUTO — A2** |
| 3 | `review retry-final-verification` | `TargetStatusActionRetryFinalVerification` + `InspectCompactFinalVerificationRetrySource` ok | Yes — every ingredient is already published (`review_next_transition.go:463-472`) | `collect final_verification_retry_authorization_required` | **AUTO — A3** |
| 4 | `review reopen-results` | Reviewer slots whose own evidence proves repository inspection was unavailable | Yes — `PrepareCompactResultReopen` derives quarantined/retained slots *and* the exact authorization | `--prepare` emits it; **not routed by next-transition** | **AUTO — A4** (routing is the new part) |
| 5 | `review reconcile-authority` | Requires an already-existing successor revision | Inputs derivable, but no state-machine edge emits it | none | **DEFER** — decided by the stream-2 sweep; A5 if an edge is proven, else Tier C |
| 6 | `review abandon` | A human decides to stop | No — no state implies "the user wants to quit"; abandon vs recover is a real fork | none | **NO — Tier C** |
| 7 | `review dispose-result` | Unusable preserved artifact | No — `--class` ∈ {`transport_syntax`, `wrong_target`} plus free-text `--diagnostic` carry intent; unrouted | none | **NO — Tier C** (`wrong_target` is subject-hash-provable: follow-up candidate) |
| 8 | `review repair-legacy-alias` | Legacy alias inventory | Compatibility-only, superseded by `review repair` | none | **NO — frozen** |
| 9 | `review quarantine-legacy` | Legacy chain inventory | Operator-initiated | none | **NO — frozen** |
| 10 | `review quarantine-legacy-fix-scope` | Legacy fix-scope anomaly | Has a machine proof (`legacyFixScopeAuthorizationMatchesProof`) but is a frozen compatibility surface | none | **NO — frozen** |

**Honest boundary: 4 auto-applicable, 1 conditional, 5 not.** The proposal's "all 10
self-emit their binding" holds for emission; auto-*consumption* is A1–A4 only.

## Duty 2 — The auto-application seam

`RunReviewRecover` already calls `reviewTransitionRecoveryAuthorization` twice at
`review_facade.go:806-807` with every input in scope. The seam is that exact site.

```go
// absence, not emptiness: reviewFlagWasProvided is a flags.Visit presence check
// (review_operation_contract.go:957-965), so an explicitly-supplied wrong binding
// never reaches derivation and still refuses. Memory #8462 proves the mechanism.
if !reviewFlagWasProvided(flags, "maintainer-authorization") {
    *actor, *reason = reviewAuditActor(ctx, root), reviewSelfRecoveryReason(shape)
    *authorization = reviewDerivedMaintainerAuthorization(shape, binding, ...)
}
```

- **Actor** — new `reviewAuditActor`: `git -C <root> config --get user.name`/`user.email`
  → `"Name <email>"`. Fallback chain: email alone → `GIT_AUTHOR_EMAIL` →
  `GIT_COMMITTER_EMAIL` → literal `gentle-ai-self-recovery@localhost`. It never returns
  empty and never fails the operation; an unset git identity becoming a deadlock would
  reintroduce the exact failure class this change removes.
- **Reason** — a closed enum of per-shape constants naming the triggering state
  (e.g. `self-recovery: escalated authority, accounting-only budget crossing`), asserted
  by test. Free text is banned so the CAS trail stays greppable.
- **CAS entry** — structurally identical to today; only actor/reason provenance shifts.
- **Surfaces** — derivation happens in the CLI, so the negotiated envelope naturally
  reports an `execute` transition instead of the `collect`. TTY prints nothing (Tier B).

Negative tests per shape (A1–A4): corrupted authority still refuses; an explicit wrong
binding still refuses; an active attempt still refuses; the drift gate still holds.

## Duty 3 — No-stop-with-successor as a structural test

**Decision: table-driven binding, not a state-machine sweep.** A sweep would need a
reachability model the code does not have — `Applicability × State × Action × Selector ×
Receipt × Replayability` are six independent dimensions, so a sweep generates unreachable
combinations and forces exemptions, which the proposal explicitly bans.

New `internal/cli/review_stop_invariant_test.go` reuses the proven extraction harness
from `review_next_transition_docs_test.go:15` (regexp over the source, fails closed in
both directions) and binds each of the **15 distinct codes across 17 call sites** to
either a terminal proof or a routing function. A new stop code with no row fails the test.

| Stop code | Final disposition |
|---|---|
| `corrupted_or_unverifiable_authority` (×2) | terminal — the eligible-repair branch already precedes it |
| `missing_authority_binding` | terminal — no authority to continue from |
| `captured_artifacts_unverifiable` | terminal — inspection failure, not a state |
| `captured_result_selection_unavailable` | terminal |
| `staged_workspace_overlay_recovery_unavailable` | terminal for a fresh target only; the row still names its own precondition (`--lineage <id>` or drop `--workspace-overlay`) |
| `final_verification_retry_unavailable` | terminal |
| `native_stop_required` (`:128`) | terminal — investigated (SUSPECT); only reachable for an `Escalated` authority where target-changed, accounting-only, and final-verification-retry are all ineligible (`target_status.go:176-199`) — genuinely stuck |
| `unchanged_or_unverified_authority` (`:126`) | terminal — investigated (SUSPECT); the accounting-only template does not apply here (that relief is specific to `StateEscalated`'s budget-only crossing) — `classifyCompactCorrectionTarget`'s `Blocked` claim already exhausts every scope-expansion/contraction/resume edge before reaching this Stop, matching the docs table's own "further work requires a new lineage" |
| `manual_intervention_required` (`:217`) | terminal — investigated (SUSPECT); the `default:` branch of the `Authority.State` switch is structurally unreachable given every state the compact authority (or a legacy chain, routed via `TargetStatusActionStop`) can actually produce is handled by an earlier explicit case — a defensive fallback, correctly terminal if a future unmodeled state ever reaches it |
| `pre_pr_selector_unrepresentable` (`:180`) | **caller-continuable** — investigated (SUSPECT); no in-process routing substitutes a different gate, because the caller explicitly chose the pre-pr gate (silently validating against a different gate would answer a different question); the fix is passing a symbolic `--base-ref`, a caller-side flag change |
| `original_finalize_request_required` (×2) | **caller-continuable** — corrected by the discoverability audit (Phase 3 task 3.10): the docs table (`docs/review-integration.md:212`) proves `gentle-ai review finalize --lineage <id>` with the exact original payload is a concrete, flag-driven continuation; this design doc previously called it terminal, which was wrong |
| `recovery_scope_unchanged` / `recovery_target_unrepresentable` | **caller-continuable** — corrected by the discoverability audit (Phase 3 task 3.10): each docs row (`docs/review-integration.md:214-215`) names a concrete, flag-driven continuation (change the target, or use a representable selector shape); this design doc previously called them terminal by "native rule", which was wrong |
| `corrected_candidate_unavailable` | **caller-continuable** — corrected during the Phase 3 structural sweep (beyond the three the audit named explicitly): its docs row (`docs/review-integration.md:205`) names `gentle-ai review status --next-transition` (or `review finalize`) after changing the candidate content — a concrete, flag-driven command, not a maintainer-only action; this design doc previously called it terminal ("a human must write the fix"), which conflated "a human edits code" with "no command exists" — the docs row's own convention (only "Terminal"-prefixed rows lack a flag-driven continuation) says otherwise |
| `escalated_recovery_requires_changed_target` (`:209`) | **routed, code removed** — investigated (SUSPECT "verify no third case remains"); the third case is real: `target_status.go:176-199` only ever sets `Action == TargetStatusActionRecover` for `StateEscalated` when either the target changed or the escalation is accounting-only-eligible (`compactAccountingOnlyEscalation`), and `RecoverCompactAuthority` already derives the evidence-bound edge (`deriveCompactRecoveredEvidence`) for the accounting-only case without requiring a further target change. The CLI's redundant target-identity re-check in `newReviewNextTransition`'s `StateEscalated` case forced a false stop on that already-legal edge. Fix: removed the re-check entirely (status.Action is trusted, matching every other case in the switch); the reason code and its docs row no longer exist. The pre-existing generic `recovery_scope_unchanged` guard inside `reviewRecoveryCollection` (fires only when a `Selector` is supplied) is unrelated and untouched. |

Each failing suspect became a fix (routing) or a corrected classification, never a test
exemption. `escalated_recovery_requires_changed_target`'s routing follows the
`target_status.go:183-191` template's *spirit* (trust the native `Action` a `validate*Edge`
predicate already governs) rather than adding a new branch, since the existing switch
case already had everything it needed once the redundant check was removed.

## Duty 4 — Tier enforcement

New `internal/cli/review_narration.go` holds `reviewNarrationTier`, a registry keyed by
stable emission ID. **Enforced subset for this change** (full `internal/cli` refusal
coverage is a separate change — ~200 `errors.New` sites):

1. The stop reason codes → Tier C, each with one rendered human statement.
2. The consent-prompt constants (`review_mode.go:379-407`) → Tier A.
3. Stream 3's escalation statement → Tier A.

- `TestReviewNarrationRegistryCoversEveryStopReasonCode` — fails on any unclassified code.
- `TestNarrationTierAAndCBanInternalVocabulary` — runs over the registry's *rendered
  strings*, banning `lineage`, `ordinal`, `CAS`, `facade`, `receipt`, `revision`,
  `digest`, `sha256:` and the uncertainty phrases `I'll figure`, `I don't know`,
  `look into`, `not sure`, `try to`.
- **Growth rule**: adding an emission to any of the three registered sources without a
  tier row fails. `review_refusal_wording_test.go` stays as the per-case complement.

The three tests interlock: every terminal stop code needs a docs row (existing test), a
tier row, and a compliant rendering.

## Duty 5 — Asset changes

Two mechanically distinct edits, identical text per group:

- **10 runtimes**, the sentence `If the user doesn't specify, default to **Interactive**…`
  → **Automatic** plus one prompt-budget sentence: `antigravity:154`, `hermes:156`,
  `gemini:137`, `codex:238`, `qwen:137`, `kimi:136`, `kiro:207`, `opencode:210`,
  `generic:139`, `cursor:157`, `windsurf:140`.
- **Preflight lists**: `claude/sdd-orchestrator-workflow.md:60,77` and
  `opencode/sdd-orchestrator.md:137,154`. The option order `Interactive, Automatic`
  stays (anchored); only the default changes.

Anchor tests updated in the same commit: `assets_test.go:799` (opencode preflight
wording), `:864` (claude workflow `1. Pace: Interactive, Automatic.`), `:935`
`TestSDDFFCommandsHonorInteractiveMode`. New anchor
`TestSDDOrchestratorAssetsDefaultToAutomatic` asserts all 12 carry the byte-identical
default sentence — that is what enforces "minimal and identical across runtimes".

## Duty 6 — Visible accounting

`compact.go:1171` sets `StateEscalated` for three distinct causes and records none.
**Derive, do not persist**: all three discriminants are already persisted
(`CumulativeCorrectionLines`, `OriginalCriteria.Passed`, `CorrectionRegression.Passed`),
so a pure method avoids touching persisted authority and its `Validate()` invariants
(`compact.go:650-690`).

```go
type CompactEscalationAccounting struct {
    Cause     string // budget_exceeded | original_criteria_failed | correction_regression_failed
    Spent, Remaining, Total int
}
func (state CompactState) EscalationAccounting() CompactEscalationAccounting
```

Naming follows `sddstatus/status.go:162-172`. `Remaining` clamps at 0 — on escalation
`Spent > Total`, and `sddstatus/review_gate.go:190` does *not* clamp today, so a negative
`CorrectionBudgetRemaining` is reachable. That gets its own RED test.

## Decisions

| Decision | Chosen | Rejected | Rationale |
|---|---|---|---|
| Auto-recovery trigger | Flag *absence* via `flags.Visit` | Empty-value check | Presence-check keeps explicit-wrong-binding refusal intact (#8462) |
| Scope of auto-consumption | A1–A4 only | All 10 | Abandon/dispose carry real human intent; legacy surfaces are frozen |
| Stop invariant | Table-driven source extraction | State-machine sweep | No reachability model exists; a sweep forces the exemptions the proposal bans |
| Escalation cause | Derived method | New persisted field | Zero schema change, no migration, no `Validate()` churn |
| Tier registry scope | 3 registered sources + growth rule | All `internal/cli` refusals | Full coverage is a separate change; a partial registry with a closing rule beats an aspirational one |
| Actor fallback | Never fails, ends at a literal | Refuse when git identity is unset | A refusal here would create a new deadlock class |
| Delivery order | 5 → 3 → 2 → 4 → 1 | Proposal's 1 → 2 → 3 → 4 → 5 | Authority-touching stream last (risk); and stream 2 changes the stop-code set stream 4 registers, so 4 must follow 2 |

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | **N/A** — no classification change; risk classification is untouched | — | — |
| Git repository selection | **Applicable** — `reviewAuditActor` adds a `git -C <root> config` subprocess | Always `git -C` against the already-resolved `ResolveRepositoryRoot` result, never cwd-relative; read-only subcommand only | Actor derived under a relative `--cwd`, an absolute `--cwd`, and a nested subdirectory all yield the same identity |
| Commit state | **Applicable** — auto-recovery must not change index/worktree semantics | Derivation is read-only; the frozen base-tree pin and projection are unchanged | Auto-recovery from a staged projection and from a dirty worktree produces the same successor as the manual path |
| Push state | **N/A** — no ref resolution or remote interaction added | — | — |
| PR commands | **N/A** — no PR argument composition added | — | — |

Additional applicable case: git identity containing shell metacharacters or newlines.
`reviewAuditActor` must reject `\r\n` (the binding is LF-delimited; a newline in actor
would forge binding lines) and fall through to the next fallback. RED test required.

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit | Actor fallback chain, reason enum, `EscalationAccounting` clamp, tier vocabulary | Table-driven, `t.TempDir()` git repos |
| Contract | Stop-code table completeness, tier-registry coverage, asset anchors | Source-extraction tests failing closed both directions |
| Negative | Corrupted authority, explicit wrong binding, active attempt, drift gate — per A1–A4 | One test per shape per guard (16 minimum) |
| E2E | Per-stream scenario in `e2e/organicruntime`, plus the paired narration scenario: one machinery failure, (a) recoverable → uninterrupted Tier-A, zero Tier-B; (b) terminal → exactly one Tier-C statement | Extend the `organic_lifecycle_hardening_test.go` per-issue subtest pattern |

## Migration / Rollout

No migration — no persisted schema changes. Each stream is an independently revertible
commit. Stream 1 degrades safely: reverting the facade routing leaves emitters in place
and restores today's manual flow. The kill switch remains the runtime escape.

## Open Questions

- [x] Stream 2's sweep decides whether `reconcile-authority` gains a routed edge (A5) or
      stays Tier C. **Resolved (Phase 3 task 3.7): Tier C, no A5 edge.**
      `review reconcile-authority` quarantines an already-created, already-invalid recovery
      successor (an unchanged target or a historical pre-contract free-form authorization on
      an otherwise structurally consistent edge) — a maintainer cleanup tool operating on a
      corrupted record discovered out-of-band (e.g. via `review inspect-authority`), not part
      of the single-target status/next-transition state graph `newReviewNextTransition`
      models. No `TargetStatusResult` state naturally implies "the caller should reconcile a
      bad successor"; unlike `abandon`, it is not even a live human decision point during an
      ordinary run. It stays Tier C for the Phase 4 narration registry, alongside `abandon`
      and `dispose-result`.
