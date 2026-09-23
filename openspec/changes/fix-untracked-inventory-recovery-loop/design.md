# Design: Fix Untracked Inventory Recovery Loop

## Technical Approach

Fix A publishes the digest `intendedUntrackedScopeForTarget` already computes as one new top-level optional field on the STATUS envelope, assigned once, before every path that could suppress it. Fix B deletes the two workspace-reading operations from the finish/settle CLI preflight so the ledger's better-informed refusal is the one callers see, while keeping the flag-shape checks the ledger cannot perform. Both are additive-optional on the wire; neither relaxes the declaration or freshness invariant.

## Where the Exploration and the Code Disagree

The exploration and proposal say the compact-reviewing replacement (`review_facade.go:917-924`) "must preserve or recompute a real digest". **The code refutes that.** `review_next_transition.go:855-858` reads the same `intendedScope`: when `Declared` is true and `Digest` is empty, the RECOVER selector builder returns `nil, false` and fails closed, deliberately (#1972 — "a declared selection without its validated inventory digest cannot be replayed"). The frozen `IntendedUntracked` paths that block replays are *not* declared against today's live digest, so preserving or recomputing `Digest` there would turn a fail-closed into a rendered selector carrying a mismatched pair. The zeroing at 920-922 is load-bearing and stays. The digest is instead read **before** the replacement, so nothing needs preserving. Code wins; the proposal's "In Scope" line about that path is satisfied by ordering, not mutation.

## Architecture Decisions

| # | Decision | Choice and rationale | Rejected alternative |
|---|---|---|---|
| 1 | Field home | `EligibleUntrackedInventory string \`json:"eligible_untracked_inventory,omitempty"\`` on **`ReviewTargetStatusResult`** (`review_status_contract.go:54-91`), placed after `Projection`. `ReviewTargetStatusProjection` serializes through the **shared** `contracts/review-integration/v1/schemas/projection.schema.json`, which `status.schema.json` (v3), `status-v4`, `status-v5` and `status-v6` all `$ref`; adding a property there retroactively mutates four published status versions and forces a `projection/v2` anyway. Semantically the projection describes the *frozen candidate* — `intended_untracked_proof` proves what the candidate **did** include. This value describes the live workspace population the candidate **could** have included and explicitly does not. Different subject, different home. | Projection field: one strict shared schema mutated for four live versions, and a top-level fact filed under a frozen-candidate record. |
| 2 | Threading | Assign `result.EligibleUntrackedInventory = intendedScope.Digest` **once**, at `review_facade.go:910`, adjacent to the existing `result.intendedUntracked = intendedScope`. This is before the compact-reviewing replacement (917-924), before `newReviewNextTransition` (1165), and before the `rdd_disabled` guard (1186-1190) — so all three suppressors are bypassed by ordering, not by new branches. `intendedUntrackedInventoryDigest` (`snapshot.go:798-805`) hashes a domain tag even for an empty population, so the workspace digest is always non-empty; staged leaves the zero value at `:805` and `omitempty` omits it with **zero new code**. | Preserve/recompute inside 917-924: breaks the #1972 recover fail-closed (see above). A second `IntendedUntrackedInventory` call: a redundant `git ls-files` per reviewing STATUS for a value already in hand, which could disagree with the digest published in the same envelope. |
| 3 | Collect argument | **Keep both.** The argument is load-bearing, not redundant: `validateIntendedUntrackedSelectionTransition` (`:784, :789-790`) requires exactly 6 arguments with `Arguments[5].Name == "expected_untracked_inventory"` and a non-empty value, and `status-v6.schema.json:46-50` pins `minItems/maxItems: 6` with `prefixItems` and `items: false`. Removing it breaks the validator, the published schema, and the negotiated submission operand set. The two carry different jobs: the top-level field is **discoverable** on every envelope; the argument is **executable** for the one negotiated submission. | Removing the argument: contract break with no benefit. |
| 4 | Schema v7 | New `contracts/review-integration/v2/schemas/status-v7.schema.json`, `$id` `.../status-v7.schema.json`, **`$ref`-based, not a copy** — that is the actual v5→v6 precedent (v6 is 91 lines that `$ref` v5 for every unchanged property and redefine only `next_transition`). v7 `$ref`s v5/v6 for all top-level properties, adds the one optional `eligible_untracked_inventory` (`$ref` v5's `sha256`), and defines a `next_transition` admitting v5's generic collect **and** v6's untracked collect with `submission` optional. Emitted **unconditionally for contract v2**: move `ReviewIntegrationStatusSchema`/`...ID` (`:30-31`) to V7, and **delete** the conditional at `review_facade.go:1183-1184`. v7 is a strict structural superset of v5 and v6 (both new properties optional), so a staged envelope validates unchanged. | Conditional v7 (emit only when the field is present): the field is on every workspace projection, so it flips nearly all traffic anyway while leaving three live shapes on one route and **adding** a condition. Loosening v5/v6 in place: retroactive published-contract mutation. |
| 5 | Advertisement | Precedent forces a capabilities minor bump: `capabilities-v2.3.schema.json` pins `schemas` at `minItems/maxItems: 19`, `v2.4` at `21`, both with closed enums, and `review_capabilities_test.go:134` compiles the emitted bytes against `capabilities-v2.4.schema.json` with a real JSON-Schema validator. Growing the advertised set is exactly what produced v2.4. So: new `capabilities-v2.5.schema.json` (22 items, `+status/v7`), `ReviewIntegrationCapabilitiesSchemaV25`/`...IDV25`, `Protocol{Major: 2, Minor: 5}`, and `ReviewIntegrationStatusSchemaV7` appended at `review_capabilities.go:314`. | Editing v2.4 in place: breaks the repo's own versioning discipline for a published advertisement. Not advertising v7: the emitter would publish a schema capabilities denies. |
| 6 | No new Go-side digest validation | The v7 schema is the shape authority; `TestReviewProviderArtifactSchemasAreStrictAndBound` guards its strictness and `review_transition_schema_drift_test.go:174` validates live envelopes against it. A parallel `Validate()` assertion would be a second representation of the same truth. The real `review_status_contract.go` work is version plumbing, not a new invariant. | A `^sha256:[0-9a-f]{64}$` check in `Validate()`: duplicated truth, per the over-engineering test. |
| 7 | Fix B deletion boundary | Delete only the **two workspace-reading operations** from `sdd_attempt.go:133-139`: `builder.IntendedUntrackedInventory` and `ValidateIntendedUntrackedSelection`. Extract lines `review_intended_untracked.go:91-109` (mode present, mode ∈ {exclude, select}, exclude-takes-no-paths, select-takes-≥1-path) into a pure `intendedUntrackedDeclarationShape(...)` that both `intendedUntrackedScopeForTarget` and the settlement path call — same conditions, one place, no duplication. **The ledger cannot recover these**: it only receives `IntendedUntracked *[]string`, so `--untracked-scope=exclude --intended-untracked=x` would silently become a select and `--untracked-scope=bogus` would be accepted. | Deleting the block outright and passing raw flags: silently drops mode semantics — worse than the bug. Adding a bool parameter to `intendedUntrackedScopeForTarget`: a flag, rejected. |

### What supplies `settlementUntracked` / `settlementInventory` after Fix B

They flow to `FinishAttemptRequest{IntendedUntracked, ExpectedUntrackedInventory}` (`sdd_attempt.go:175, :212`) and land in `settlementUntrackedSelection` (`runtime_ledger.go:1007`). After the deletion: `settlementUntracked` = the extracted helper's mode-resolved selection (`[]string{}` for `exclude`, the flag values for `select`); `settlementInventory` = `expectedUntrackedInventory.value` verbatim. Nothing is dropped — the ledger already performs every check the deleted operations did, and does them better:

| Check | Deleted CLI site | Surviving ledger site |
|---|---|---|
| pairing (selection ↔ digest) | `:91-93` | `runtime_ledger.go:2880` |
| `sha256:<64hex>` shape | — | `:2883` |
| canonicalization | `snapshot.go:738` | `:2886-2891` (`canonicalRuntimeIntendedUntracked`) |
| freshness (stale digest) | `snapshot.go:735-736`, generic | `:1036-1037`, names both digests and the exact rerun |
| eligibility membership | `snapshot.go:746-749` | `:1040-1043` |
| non-narrowing vs. begin | — | `:1048-1051` |
| born-during-attempt | — | `:1027-1034` → `runtimeBornDuringUntrackedRefusal` |

`begin`/`acquire` (`sdd_attempt.go:105-126`) is **untouched**: it has no ledger-side equivalent, so its check is not redundant.

Precision on the symptom: today the CLI pre-empts only when the caller *has* declared. The undeclared case already reaches `runtimeBornDuringUntrackedRefusal`. Fix B's payoff is the **second round** — after the caller follows that refusal's instructions, a stale digest currently returns the generic CLI message instead of the ledger's digest-naming one.

## Data Flow

```text
builder.IntendedUntrackedInventory        (review_intended_untracked.go:79 — every invocation)
    -> intendedScope.Digest
    -> result.EligibleUntrackedInventory  (review_facade.go:910 — ONE assignment, no condition)
         |                                  bypasses by ordering:
         |                                    917-924 compact-reviewing replacement (Digest stays zeroed)
         |                                    1165    next_transition builder
         |                                    1186    rdd_disabled guard
         '-> encodeReviewJSON              (staged: zero value -> omitempty -> key absent)
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/cli/review_status_contract.go` | Modify | New top-level field; `V7`/`IDV7` consts; alias `:30-31` → V7; add V7 to the version chains at `:299, :526, :536, :656`; restate `:785-786` submission rule on submission validity, not schema version |
| `internal/cli/review_facade.go` | Modify | `+1` assignment at `:910`; **delete** `:1183-1184` |
| `internal/cli/review_capabilities.go` | Modify | v2.5 branch; append `ReviewIntegrationStatusSchemaV7` at `:314` |
| `contracts/review-integration/v2/schemas/status-v7.schema.json` | Create | `$ref`-based v7 (v5→v6 pattern) |
| `contracts/review-integration/v2/schemas/capabilities-v2.5.schema.json` | Create | 22-entry `schemas` set incl. `status/v7` |
| `internal/cli/review_intended_untracked.go` | Modify | Extract `intendedUntrackedDeclarationShape` |
| `internal/cli/sdd_attempt.go` | Modify | Delete the two workspace-reading calls at `:133-139` |
| `internal/cli/review_provider_artifact_contract_test.go` | Modify | New `TestReviewProviderArtifactStatusV7ContractsArePinned` (avoid collision with the release-named `...V25StatusContracts...` at `:109`); v7 + capabilities-v2.5 rows in the strict/bound list at `:305-332` |
| `internal/sddstatus`, `internal/reviewtransaction` | None | Read-only, as the proposal predicted — confirmed |

## Testing Strategy (strict TDD — RED first, in this order)

| # | Test | Assertion |
|---|---|---|
| R1 | **Strengthen `TestIntendedUntrackedRefusalsNameARunnableStatusInvocation` in place** (`review_intended_untracked_test.go:360-384`) — do **not** duplicate | It already executes the printed command into `output` and then discards it, asserting only `runErr == nil`. That is how this defect shipped: runnability proven, usefulness assumed. Decode `output`, assert `eligible_untracked_inventory` matches `^sha256:[0-9a-f]{64}$`, feed it back as `--expected-untracked-inventory`, and assert the previously-refused `review start` now succeeds. A parallel new test would leave the false assurance standing. |
| R2 | Presence table (new) | `undeclared`, `declared_select`, `declared_exclude`, `rdd_disabled`, `compact_reviewing`: field present and equal to an independently computed `SnapshotBuilder{Repo}.IntendedUntrackedInventory` digest |
| R3 | Staged absence (new) | Decode into `map[string]json.RawMessage`; assert `_, ok := m["eligible_untracked_inventory"]; !ok`. **Never** `status.EligibleUntrackedInventory != ""` — a struct check cannot tell absent from empty and passes on the exact bug |
| R4 | Schema conformance | Live workspace and staged STATUS both validate against the whole compiled `status-v7.schema.json`; `schema == "gentle-ai.review-integration.status/v7"`. Update `review_transition_schema_drift_test.go:174`, `review_next_transition_test.go:569`, `compact_reviewer_capture_test.go:152`, `review_intended_untracked_test.go:169` |
| R5 | Capabilities | `Schemas` contains v7; emitted bytes validate against `capabilities-v2.5.schema.json`; `Protocol{2,5}` |
| R6 | Fix B provenance | `finish`/`settle` with a stale digest returns the **ledger** text (`this declaration was made against untracked inventory … but the workspace now holds …`), not the CLI's (`untracked inventory changed; rerun …`), wrapping `ErrRuntimeUndeclaredUntracked` |
| R7 | Fix B negative controls | `exclude` + `--intended-untracked`, `select` with no paths, `--untracked-scope=bogus`, and `--intended-untracked` alone all still refused; `begin` with a stale digest still fails at the CLI |

## Bench Corpus — required, not optional

Three journey sites pin `status/v6` and **will fail** on the v7 move: `bench/journeys_intended_untracked.go:82, :121` and `bench/journeys_capture_evidence_v5.go:160` (whose `statusSchemaV6` const at `:14` gains a `statusSchemaV7` sibling). Per `gentle-ai-bench`, add one **new** `bench/journeys_4040_untracked_inventory.go` (`Review: reviewOptedIn`) driving the black-box loop: STATUS → read `eligible_untracked_inventory` from the **top level**, not from the collect argument → `sdd-attempt finish` succeeds. Regenerate `bench/testdata/journeys.manifest` via `go test ./bench -run TestRegisteredJourneysMatchTheManifest -update-journey-manifest`. `go test ./bench` proves declarations only; the driven harness from `.github/workflows/ci.yml` is the execution proof.

## Threat Matrix

| Boundary | Applicability | Behavior and RED test |
|---|---|---|
| Executable-file classification | **Applicable** — the eligible inventory can name executable untracked paths (bench `unbornUntrackedExecutableCandidate`) | Publishing the *digest* names no bytes and changes no classification; the eligible-paths list already ships in the collect argument. R2 asserts the field is a digest only, never a path list |
| VCS/repository selection | **Applicable** — `DiscoverUnignoredUntracked` runs `git ls-files` at the resolved root | Unchanged: Fix A reuses the existing call; Fix B **removes** two git invocations from finish/settle. R7 pins the surviving refusals |
| Shell / subprocess / PR automation | N/A | No command composition changes |
| Routing | N/A | `next_transition` selection logic is untouched apart from deleting `:1183-1184` |

## Migration / Rollout

No data migration, no feature flag. v7 is additive-optional; `AdditiveMinorPolicy: optional-fields-only` covers consumers. Rollback = full revert (partial rollback states are known-bad per the proposal).

## Review Workload Forecast

**Honest revision of the proposal's 250-370 estimate.** It did not account for the capabilities-v2.5 bump (decision 5) or the bench work, both of which the code proves mandatory.

| Slice | Scope | Lines |
|---|---|---:|
| Fix B | shape extraction + preflight deletion + R6/R7 | ~90 |
| Fix A core | v7 schema + field + emitter + R1-R4 + bench v6→v7 pins | ~330 |
| Fix A advertisement | capabilities-v2.5 + pins + new #4040 journey + manifest | ~140 |

Total ≈ 560.

```
Decision needed before apply: Yes
Chained PRs recommended: Yes
400-line budget risk: High
```

The cached `delivery_strategy: single-pr` **conflicts with this forecast**. Escalate to the orchestrator: chain the three slices (Fix B first — it is an independent invariant with its own rollback boundary), or record an explicit `size:exception`. Fix A core and Fix A advertisement must land in that order; Fix A core without the advertisement is an under-advertisement, which is the safe direction.

## Open Questions

None blocking. One item for `sdd-tasks` to verify during apply: whether any consumer adapter string-compares `schema == "gentle-ai.review-integration.status/v5"` rather than accepting the advertised set — grep the repo's shipped adapters before the alias move.
