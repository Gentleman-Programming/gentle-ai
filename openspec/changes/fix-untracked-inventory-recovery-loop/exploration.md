## Exploration: fix-untracked-inventory-recovery-loop

Source: GitHub issue #4040 — `bug(sdd-attempt): untracked-inventory recovery loop blocks continuation`.
Reproduced independently on 2.5.0-rc.3 and 2.5.0 stable, on macOS/ARM64 (Codex CLI), Linux x86_64 (Pi), and Windows 11 Git Bash.

### Current State

Three digest-shaped values coexist in the review status projection. Two are published unconditionally at top level; neither is the one `--expected-untracked-inventory` accepts. The third — the only usable one — has exactly one publication site, buried inside a conditional transition argument.

| Value | Source | What it hashes | Published |
| --- | --- | --- | --- |
| `paths_digest` | `internal/cli/review_status_contract.go:172` | `digestPaths(candidatePaths)` — the resolved review candidate | top level, always |
| `intended_untracked_proof` | `internal/cli/review_status_contract.go:175` | the already-declared/selected untracked subset | top level, always |
| eligible-untracked-inventory digest | `internal/reviewtransaction/snapshot.go:798-805`, domain `gentle-ai.intended-untracked-inventory/v1`, computed by `SnapshotBuilder.IntendedUntrackedInventory` (`snapshot.go:689-695`) | the eligible-but-undecided untracked population | only inside `next_transition.collect.inputs[].arguments.expected_untracked_inventory` |

The refusals that callers hit are emitted by `ValidateIntendedUntrackedSelection` (`internal/reviewtransaction/snapshot.go:730-736`) and `intendedUntrackedScopeForTarget` (`internal/cli/review_intended_untracked.go:91-93`). Both name `gentle-ai review status --next-transition` as the recovery route. That route does not publish the value they demand.

### Verified Mechanism Map

Every claim below was confirmed against the code at the cited lines.

1. **Confusable siblings.** Three digest-shaped values, not two. The two visible ones are both wrong for this flag; the correct one is invisible. This is the structural source of the reported confusion — a caller reading the envelope has no way to reach the right value.
2. **Single publication site.** `reviewIntendedUntrackedCollection` (`internal/cli/review_intended_untracked.go:127-135`) is the only code in the repository that emits the canonical digest to a caller. It is never a top-level status field.
3. **The declaration paradox.** The collect transition fires only when `intendedScope.NeedsSelection` (`internal/cli/review_facade.go:1186-1190`), and `NeedsSelection` is set only when `!scope.Declared` (`internal/cli/review_intended_untracked.go:83-89`). Following the refusal's own instruction — rerun STATUS with the selection — sets `Declared = true`, which is the exact condition that disables publication. Literal dead loop.
4. **RDD-disabled suppression.** `rdd_disabled` is emitted from one line (`internal/cli/review_next_transition.go:157-158`), reachable only under `TargetApplicabilityUnrelated` — a fresh target with no live authority, i.e. the common first-attempt case. `review_facade.go:1186-1190` explicitly declines to override a `{stop, rdd_disabled}` transition, so the digest is never published. Meanwhile `internal/cli/sdd_attempt.go:105-126` demands the flag without ever consulting RDD mode. This is an independent trigger of the same symptom: fixing claim 3 alone does not fix it.
5. **Compact reviewing override.** `internal/cli/review_facade.go:917-923` replaces the scope with `{Intended: ..., Declared: true}`, dropping `Digest` to `""`. A third independent trigger.
6. **Duplicate finish/settle validation.** `runtimeBornDuringUntrackedRefusal` (`internal/sddstatus/runtime_ledger.go:1061-1067`) does emit a fresh, usable digest, but it lives inside `settlementUntrackedSelection`, reached only from `store.Finish`/`Settle`. The CLI preflight (`internal/cli/sdd_attempt.go:133-139`, finish/settle only) independently recomputes the same live inventory and fails first with a strictly worse, context-free message. Same freshness check, two messages, the worse one always wins. `begin`/`acquire` has no ledger-side equivalent, so its CLI check is not redundant — only finish/settle is.

### Root Class

Two causally independent invariants, not one defect.

**Fix A — unconditional publication (claims 1-5).** Publish the canonical eligible-untracked-inventory digest as a top-level status field, sourced from the `builder.IntendedUntrackedInventory` call `intendedUntrackedScopeForTarget` already makes on every invocation (`internal/cli/review_intended_untracked.go:79`). Independent of `Declared`, `NeedsSelection`, RDD mode, and the compact-reviewing override — which must be corrected to preserve or recompute a real digest rather than zeroing it. This deletes conditions rather than adding machinery: the value already exists on every call and is currently discarded.

**Fix B — remove duplicate finish/settle validation (claim 6).** Remove the redundant CLI-side digest-equality precondition for finish/settle, so `runtimeBornDuringUntrackedRefusal`'s better-informed message is what callers actually see. Deletion, not a second machinery layer.

### Affected Areas

- `internal/cli/review_status_contract.go` — new top-level projection field and its validation.
- `internal/cli/review_facade.go` — publish unconditionally; decouple from the `rdd_disabled` stop path and from the compact-reviewing scope replacement at lines 917-923.
- `internal/cli/review_capabilities.go` (~line 314) — advertise the new schema version.
- `contracts/review-integration/v2/schemas/status-v7.schema.json` — new schema file.
- `internal/cli/sdd_attempt.go` — remove the redundant finish/settle preflight (Fix B).
- `internal/cli/review_provider_artifact_contract_test.go` — update the pinned schema SHA-256 guard.
- Tests: `internal/cli/review_intended_untracked_test.go`, `internal/cli/sdd_attempt_untracked_test.go`, `internal/sddstatus/runtime_ledger_untracked_test.go`.

### Blast Radius

`ReviewTargetStatusProjection` and its wire schemas are strict closed shapes: `additionalProperties: false` appears six times in `status-v6.schema.json`. Despite the protocol's `AdditiveMinorPolicy: "optional-fields-only"` (`internal/cli/review_capabilities.go:281,325`) instructing consumers to ignore unknown optional fields, the schema files themselves cannot silently gain one. This repository's own precedent (v1 through v6) is one new schema file per shape change, so the field requires `status-v7.schema.json` with a new `$id` referencing v6 defs, new `ReviewIntegrationStatusSchemaV7`/`SchemaIDV7` constants, and a `Schemas` entry.

`internal/cli/review_provider_artifact_contract_test.go` pins exact SHA-256 hashes of every schema file (verified: `sha256.Sum256` at lines 47, 69, 102, 134). Updating that guard is by design, not incidental.

For compliant consumers (Claude Code, Codex, OpenCode, Pi), a new optional field is non-breaking. The bench journey corpus (`bench/journeys*.go`) already reads `paths_digest`, but no journey pins the current missing shape as intended behavior.

### The `staged` Projection Edge Case

`internal/cli/review_facade.go:806-809` rejects untracked flags for the staged projection and leaves `intendedScope` at its zero value (`Digest: ""`). Untracked files may still exist on disk, but staged never inspects them by design.

The new field MUST be **absent** (`omitempty`) for staged, not an empty-population digest. Absent says "not applicable here". An empty-population digest would falsely assert "checked, found none".

### Naming

`eligible_untracked_inventory`. It is already the exact name for this identical value in the SDD ledger record (`internal/sddstatus/runtime_ledger.go:231,586`), recorded at `begin` time. The "eligible" versus "intended" contrast is load-bearing vocabulary in this codebase, and the name is clearly distinct from both siblings in the same projection struct. Verified unreserved: zero hits for `eligible_untracked_inventory` across `openspec/`.

### Prior Art and Conflicts

Every change below was verified by listing its directory and reading its files directly.

| Change | State | Verdict |
| --- | --- | --- |
| `sdd-compact-authority-recovery` | live, implemented and verified (8/8 tasks, PASS WITH WARNINGS) | No overlap. Scope is the `sdd-status` pre-final-verification authority bridge in `internal/sddstatus`. Never references `review_facade.go`. Does not own or constrain the claim-5 override. |
| `rdd-root-simplification-wave7` | live, mid-flight | No overlap. Deletes the legacy pre-v3 RDD mutation lifecycle at disjoint `review_facade.go` line ranges. Its switch-removal slice was deferred, not merged. No `untracked` or `next_transition` references. |
| `fix-review-lifecycle-loop` | live, not applied | No overlap. A different loop: `CompactState.Receipt()` binding `PathsDigest` from the wrong snapshot generation, producing a `scope-changed` gate loop. |
| `organic-rdd-recovery` | live | No overlap. Control-plane deletion (WorkRun/productive-runtime). No scope item touches untracked-inventory publication. |
| `review-lifecycle-hardening` | live | No overlap. A closed, pre-enumerated list of eighteen defects (#1699-#1818). Issue #4040 is not among them. |
| `complete-native-review-lifecycle` | live, implemented | No overlap. Genesis-path immutability and one-discovery/one-correction bounds; touches intended-untracked paths only for correction-scope subset validation. |

No existing live or archived change fixes, reserves, or contradicts this fix scope.

### Test Surface

`TestIntendedUntrackedRefusalsNameARunnableStatusInvocation` (`internal/cli/review_intended_untracked_test.go`) proves the named recovery command *parses*, but never asserts it *produces a usable digest*. That is precisely the gap the defect fell through: verifying runnability while assuming usefulness.

`internal/cli/sdd_attempt_untracked_test.go` has no end-to-end reproduction of the reported loop. `internal/sddstatus/runtime_ledger_untracked_test.go` and `runtime_undeclared_untracked_reason_test.go` cover ledger-side finish/settle reconciliation but never the CLI preflight layer that currently pre-empts it.

A TDD-first implementation needs:
1. Status-field presence tests: undeclared, declared, RDD-disabled, compact-reviewing, and staged-absent.
2. An end-to-end `sdd-attempt` refusal → STATUS → retry regression proving the loop is closed.
3. A finish/settle test proving the ledger's informative message surfaces, not the generic one.

### Forecast

Fix A approximately 80-150 lines; Fix B approximately 15-40 lines, mostly deletion; tests approximately 100-180 lines. Total roughly 250-370 authored changed lines — within the 400-line budget, but close enough that task slicing must keep the two invariants separable.

### Risks

- Fix A and Fix B are independent root causes on different call paths. Bundling them without explicit task-level separation risks an oversized or improperly scoped change.
- The compact-reviewing override (claim 5) and the staged-absent semantics are easy to regress silently: empty string versus absent field is the whole distinction, and a wrong choice reintroduces the defect in a quieter form.
- Prior-art checks in downstream phases must list `openspec/changes/` directly rather than trusting a single glob pattern's negative result. A glob scoped to `archive/` returned empty for two changes that exist, live and readable, one directory up.

### Ready for Proposal

Yes.
