# Archive Report: RDD Root Simplification — Wave 6 (Descendant Closure)

**Change**: rdd-root-simplification-wave6
**Archived**: 2026-08-04
**Archived to**: `openspec/changes/archive/2026-08-04-rdd-root-simplification-wave6/`
**Store mode**: hybrid (filesystem + Engram)

## Traceability — Engram Observation IDs

| Artifact | Observation ID | Title |
|---|---|---|
| Proposal | #10150 | sdd/rdd-root-simplification-wave6/proposal |
| Delta specs | #10157 | sdd/rdd-root-simplification-wave6/spec |
| Design | #10156 | sdd/rdd-root-simplification-wave6/design |
| Tasks (final revision 9) | #10159 | sdd/rdd-root-simplification-wave6/tasks |
| Verify report (cycle 3, final) | #10201 | sdd/rdd-root-simplification-wave6/verify-report |
| This archive report | (assigned on save below) | sdd/rdd-root-simplification-wave6/archive-report |

## What Shipped

Wave 6 relaxed Wave 2's `admitLeafDisposition` cardinality-one gate to admit N≥1 for closed, evidence-backed anomaly classes, closing #2014's multi-node case and the classifiable half of #1656. It shipped as a feature-branch chain on the `feature/rdd-root-simplification` tracker:

- Chain: `bb3c22a9` (base) → S1 `41a471ff` → S2 `8768b7cf` → S3 `d17b088d` → S4 `48a70562` → S5 `40176a8f` → F1 `bba17974` (`feat/rdd-wave6-f1-resume-authorization`) → F2 `e174bc2b` (`feat/rdd-wave6-f2-journey-positions`).
- PR chain #2401-#2407, boundary #2408 (this wave's slices as reviewed/merged PRs against the tracker).
- Landed to `main` at commit `e6ac4176` — confirmed as the archive launch prompt's HEAD ("main WITH wave 6 merged"), which is the authoritative final-state fact per the Final-State Authority hierarchy and outranks any earlier intermediate snapshot claim.

Delivered capabilities: N-node admission for closed anomaly classes; descendant-first ordered disposition (deepest descendant first, seed last, so every interruption prefix leaves a valid retained graph); a plan-scoped closure manifest binding all N per-entry two-phase quarantine records to one `plan_digest`, with success/retained-graph revalidation reported only after the last node commits; forward-only resume via digest + `residue/` discriminator; cross-lineage closure with unrelated-lineage byte-identity; and a negotiated `review status --next-transition` route for closure disposition (D7), replacing the raw `--plan-digest/--inventory-revision/--authorization` flag triad. Zero new plan shape, zero new digest domain, zero new public verbs, consistent with the proposal's Success Criteria.

## Resolved Issues

- **#2014** (content-mismatched binding whose closure spans 2+ nodes): closed. The multi-node case is now admitted end-to-end, byte-preserving, per the "N-Node Admission for Closed Anomaly Classes" requirement and bench `ds09`. Wave 2's leaf case (`ds06`) already closed the cardinality-one shape.
- **#1656** (multi-lineage repair dead-end): closed for its **classifiable** half only, by design. The unclassifiable half of #1656 stays `blocked` permanently (no generic fallback quarantine) — this is an explicit, intentional boundary, not a deferral, per the proposal's Coverage section and the "Unclassifiable multi-lineage shape still blocks" scenario.
- **#1529** (stale-but-healthy lineage cleanup): assessed and explicitly **not absorbed** (D3) — it is retention/GC policy, not a classified anomaly, and absorbing it would require exactly the generic quarantine fallback the design forbids.

## Three-Cycle Verification Summary

### Cycle 1 — candidate `40176a8f` (end of S1-S5 chain): FAIL, 2 CRITICAL, 2 blockers, 8/10 requirements, 11/13 scenarios

**CRITICAL-1 — authorization bypass on resume (security).** `lockedAuthorityDispositionMutation` gated `validateAuthorityDispositionAuthorization` — its only non-test call site — inside an `if freshExecution` branch. Root cause: **the authorization check was gated inside a fresh-execution-only branch**, so on the resume path the guard simply never ran; every resumed closure disposition executed *unauthorized*. Reproduced concretely: a forged `--authorization` and a different actor (`attacker`) were silently admitted on resume, both through the Go public entrypoint and through the real CLI (exit 0, all three closure members quarantined). CAS-all-N validation was skipped on the same path for the same reason. Closed on branch `feat/rdd-wave6-f1-resume-authorization`, commit `bba17974`: authorization and CAS-all-N were moved out of the fresh-execution-only branch so both validate on every execution path (fresh or resumed); only the plan re-derivation comparison correctly stays fresh-only, since it cannot work mid-closure by definition. New RED test: `TestAuthorityDispositionResumeRefusesForgedAuthorization` plus a new ds11 bench step reproducing the identical forged-authorization repro through the real binary.

**CRITICAL-2 — by-design refusals surfaced as defect reports.** Slice S5 removed the CLI pre-check and routed all execution failures through `reviewRepairOperationError`, whose `Error()` returned only `message` (dropping cause), and the type was unrecognized by the negotiated classification cascade — so every disposition-execution refusal, including a plain stale `--plan-digest`, fell through to `operation_outcome_unknown` and appended a spurious saved-defect-report clause, a direct regression against base `bb3c22a9`. Closed in the same `bba17974` commit: a refusal whose returned record proves nothing mutated now routes through `reviewPreflightError`, restoring base classification.

Both CRITICALs closed by `bba17974`. This alone did not authorize archiving — `sdd-verify` was re-run.

### Cycle 2 — candidate `bba17974` (fix cycle 1 result): FAIL, 0 CRITICAL, 0 blockers, 9/10 requirements, 12/13 scenarios

Both cycle-1 CRITICALs stayed CLOSED with fresh, independently reproduced evidence. Verdict stayed `fail` solely because "Multi-chain and crash-recovery journeys pass" remained PARTIAL: `ds11` **authored** a pre-broken on-disk state rather than genuinely interrupting the real binary, and covered only 1 of the 3-node closure's 6 ordered positions (`TestAuthorityDispositionResumeCrashPositionMatrix` already proved 6/6 at the Go layer, but the journey/CLI layer did not). Coordinator decision: extend the journey, do not amend the spec — this wave's own CRITICAL-1 was a real public-entrypoint defect that Go-level unit tests structurally could not see, so "interrupt at each ordered position" at the journey layer is load-bearing, not ceremony.

### Cycle 3 — candidate `e174bc2b` (fix cycle 2 result, FINAL): PASS WITH WARNINGS, 0 CRITICAL, 0 blockers, 10/10 requirements, 13/13 scenarios

Fixed on branch `feat/rdd-wave6-f2-journey-positions`, commit `e174bc2b`. **Six-position journey coverage**: the single hand-authored `ds11-crash-recovery-mid-closure` was replaced with six generated journeys (`ds11-crash-recovery-<phase>-<role>` for prepared/committed × grandchild/child/seed), each genuinely interrupting the real `review repair` binary via a new build-tag-gated product hook (`internal/reviewtransaction/bench_fixture.go`, `-tags bench_fixture`, mirroring the pre-existing `internal/sddstatus/bench_fixture.go` j57 pattern) that overrides `compactReclaimPhaseHook` — the identical seam the in-process Go-level crash-position matrix already used — reading `GENTLE_AI_BENCH_CRASH_AT_PHASE="<phase>:<lineage>"`. All six completed against a fresh tagged binary (0 failed); genuineness was verified two ways: (a) the fault text carries production's own phase prefixes from `quarantineCompactStoreEntry`, proving the process really stopped at that phase boundary inside the real mutation, not a fixture-authored state; (b) the resulting on-disk state is written only by production code, with convergence proved per position by `requireCrashPositionConvergedByteIdentical` (post-resume `residue/` digest equals each member's own pre-disposition digest). Coverage moved from 6/6 (Go layer) + 1/6 (journey layer) in cycle 2 to 6/6 + 6/6 in cycle 3.

**Build-tag seam audit.** The `bench_fixture` hook was independently audited as NOT a production surface: `strings -a` and `go tool nm` found zero occurrences of the crash-injection env var, its marker text, and its symbol in a plain `go build ./...` binary (all present under `-tags bench_fixture`), validated against a control (`GENTLE_AI_RDD_NEW_LINEAGE`, present in both, proving the detection method itself works). Runtime proof: the plain binary was driven through a complete N=3 disposition with the crash env var explicitly set at each of the six positions — every run exited 0, no marker fired, closure fully disposed; no plain binary can be induced to crash mid-disposition by any env var. The release workflow builds with `go build -trimpath` only, and the sole `-tags bench_fixture` build in the repo is the pre-existing CI evidence step.

Fresh, isolated synchronous foreground verification on `e174bc2b`: `go test ./... -count=1` — 63/63 packages ok, 0 FAIL; `go build ./...` and `go build -tags bench_fixture ./...` both exit 0; `go vet` clean (both tags); gofmt clean; deadcode ratchet clean; bench `--axis all` (89 journeys): tagged binary 89/89 completed 0 unsupported 0 failed; plain binary 82 completed + 7 unsupported (the 6 new + pre-existing j57) + 0 failed. Both closed CRITICALs regression-verified against genuinely interrupted closures this cycle (not cycle-1's authored fixture) and still held.

**Verdict**: PASS WITH WARNINGS — 10/10 requirements, 13/13 scenarios COMPLIANT, 0 criticals, 0 blockers, 5 non-blocking warnings (see Carry-Forwards below).

## Six-Position Journey Coverage and Build-Tag Seam — Summary Table

| Position | Role | Journey | Result |
|---|---|---|---|
| prepared | grandchild | `ds11-crash-recovery-prepared-grandchild` | completed, genuine |
| prepared | child | `ds11-crash-recovery-prepared-child` | completed, genuine |
| prepared | seed | `ds11-crash-recovery-prepared-seed` | completed, genuine |
| committed | grandchild | `ds11-crash-recovery-committed-grandchild` | completed, genuine (also carries forged-authorization proof from F1) |
| committed | child | `ds11-crash-recovery-committed-child` | completed, genuine |
| committed | seed | `ds11-crash-recovery-committed-seed` | completed, genuine |

Seam audit checks (plain build vs `-tags bench_fixture`): env var string 0 vs 1; marker text 0 vs 1; exported symbol 0 vs 1; control env var 1 vs 1 (proves the method). Plain-binary runtime probe across all six positions: 6/6 exit 0, no crash, no marker — inert.

## Carry-Forwards (not blocking this archive)

(a) **CI never runs the damaged-store axis.** `.github/workflows/ci.yml` runs the portable core plus `--axis source-coupled --only j57` only; `ds01`-`ds12`, including all six new crash-position journeys, execute on no CI path. The wave's exit evidence passes locally but is unenforced against future changes. Pre-existing gap, materially larger now that the spec's exit-evidence requirement rests on these journeys. **Recommended as a pre-rc item.**

(b) **Undisclosed `bench_fixture` tag requirement.** The damaged-store axis's own `Properties` declaration does not disclose that six of its journeys require a `bench_fixture`-tagged binary, and `bench/README.md` still frames the tag as source-coupled/j57-only. A contributor following the README gets six silent `unsupported` results with no stated reason.

(c) **SUGGESTION-1 — mixed actor provenance across a resumed closure.** A resume may be attested by a different maintainer than the interrupted attempt (`plan_digest` excludes actor/reason), producing mixed actor provenance across a closure's quarantine records. Needs design attention, not a code fix. **Recorded as Wave 7 backlog.**

(d) **ShadowRelation alias rename pass.** Deferred to Wave 7, consistent with the proposal's Out of Scope boundary (legacy verb deletion / public-surface retirement is Wave 7's job, not this wave's admission relaxation).

Additional non-blocking warnings from verify-report cycle 3 (not separately called out above): a forged-authorization resume still completes an already-in-flight PREPARED move before authorization validation (pre-existing, base `bb3c22a9` is strictly worse); the `record.LineageID == ""` heuristic still misclassifies one rare skip-then-mismatch refusal as `operation_outcome_unknown` (cosmetic, cause still printed correctly); `TestAuthorityDispositionPlanDigestN1ByteStability` still never calls `authorityDispositionClosure` directly (cross-version proof supplied out of band across all three cycles instead).

## Spec Sync (openspec/hybrid)

| Domain | Action | Details |
|---|---|---|
| `rdd-closure-disposition-execution` | Created | New capability, full spec copied verbatim (7 requirements, 8 scenarios) |
| `rdd-leaf-disposition-execution` | Modified | 1 requirement replaced ("Cardinality-One Admission" narrowed to the N=1 base case; retired multi-node refusal scenario dropped); all other 9 requirements preserved unchanged |
| `rdd-authority-disposition-plan` | Modified | 2 requirements replaced ("Deterministic Closure Derivation..." gained normative descendant-first ordering + new scenario; "Cardinality Is an Executor Admission Policy...") gained the shipped-executor cross-reference); all other 5 requirements preserved unchanged |

Source of truth now reflects Wave 6 behavior at:
- `openspec/specs/rdd-closure-disposition-execution/spec.md`
- `openspec/specs/rdd-leaf-disposition-execution/spec.md`
- `openspec/specs/rdd-authority-disposition-plan/spec.md`

## Task Completion Gate — Exceptional Reconciliation

The persisted `tasks.md` (and its Engram mirror #10159) carried 44 stale unchecked implementation checkboxes (Gate 0.0, all of Phase 1-5, Deletion Deferral D.1/D.2, and Fix-Cycle-1 F1.4) despite `verify-report.md` cycle 3 explicitly reporting `Tasks total 44 / Tasks complete 44 / Tasks incomplete 0` and every slice/fix commit hash already being recorded as committed in the same document's Chain summary. Per the Task Completion Gate's exceptional-reconciliation path, and the explicit final-state fact in the archive launch prompt (HEAD `e6ac4176` = `main` WITH Wave 6 merged, which outranks intermediate unchecked snapshot state under the Final-State Authority hierarchy), all 44 were reconciled to `[x]` at archive time, each with an inline note where the reconciliation needed evidence beyond the chain-summary commit hashes (e.g. F1.4's resolution via F2.1, D.2's honored-by-omission compliance). One item, F1.5, was deliberately left unchecked: it is a genuinely open, non-blocking item tracked as WARNING-5 in the final verify-report, not stale bookkeeping. Full rationale is recorded in `tasks.md`'s "Archive Reconciliation Note" section, preserved in the archived copy.

## Archive Contents

- `proposal.md` ✅ (118 lines, byte-identical to source)
- `design.md` ✅ (95 lines, byte-identical to source)
- `specs/rdd-leaf-disposition-execution/spec.md` ✅ (16 lines, byte-identical to source)
- `specs/rdd-authority-disposition-plan/spec.md` ✅ (34 lines, byte-identical to source)
- `specs/rdd-closure-disposition-execution/spec.md` ✅ (90 lines, byte-identical to source)
- `verify-report.md` ✅ (280 lines, byte-identical to source — all 3 cycle histories preserved intact)
- `tasks.md` ✅ (139 → 144 lines; intentionally modified for checkbox reconciliation only, per the Task Completion Gate exception above; no content otherwise altered)
- `archive-report.md` ✅ (this file, new)

## SDD Cycle Complete

The change has been fully planned, implemented, verified (3 cycles, 0 criticals at close), and archived. Ready for Wave 7.
