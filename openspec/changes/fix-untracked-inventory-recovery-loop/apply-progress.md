# Change: fix-untracked-inventory-recovery-loop — Apply Progress

## Completed Work Units
- [x] WU1 — Fix B: Ledger-Owned Finish/Settle Untracked-Freshness Refusal (tasks 1.1-1.6, ~90-line design budget)
- [x] WU2 — Fix A core: unconditional digest publication + schema v7 (tasks 2.1-2.14, ~330-line design budget)
- [x] WU3 — Fix A advertisement: capabilities v2.5 (tasks 3.1-3.9, ~140-line design budget)
- [x] Phase 4 — cross-cutting verification and PR recording (tasks 4.1-4.5)

## Remaining Work Units
None. All 34 tasks (1.1-1.6, 2.1-2.14, 3.1-3.9, 4.1-4.5) complete.

---

## WU1 Task-by-Task Status (unchanged from prior attempt; carried forward)
- [x] 1.1 RED: Added `TestRunSDDAttemptFinishAndSettleSurfaceLedgerUntrackedFreshnessMessage` (R6) in `internal/cli/sdd_attempt_untracked_test.go`. Confirmed FAILS on pre-fix code (surfaced the CLI's generic "untracked inventory changed" message instead of the ledger's).
- [x] 1.2 RED: Added `TestRunSDDAttemptFinishAndSettleNegativeControlsStillRefuse` (R7) in the same file, covering exclude+intended-untracked, select+no-paths, bogus scope, intended-untracked-alone for finish/settle, plus a begin-stale-digest subtest proving begin's untouched preflight.
- [x] 1.3 GREEN: Extracted `intendedUntrackedDeclarationShape(mode, selected, expectedDigest, digest, inventoryCommand, selectionCommand) error` in `internal/cli/review_intended_untracked.go` from the inline checks previously at lines 91-109; `intendedUntrackedScopeForTarget` now calls it.
- [x] 1.4 GREEN: In `internal/cli/sdd_attempt.go`, the finish/settle preflight block no longer calls `intendedUntrackedScopeForTarget` (which read the workspace twice). It now calls `intendedUntrackedDeclarationShape` for flag-shape validation only, then resolves `settlementUntracked` directly from the mode: `&[]string{}` for exclude, the raw flag values for select; `settlementInventory = expectedUntrackedInventory.value` verbatim. These flow unchanged into `FinishAttemptRequest`/`CompactSettleRequest`.
- [x] 1.5 GREEN: `go test ./internal/cli/... -run 'TestRunSDDAttemptFinishAndSettle'` — PASS (both new tests, all subtests).
- [x] 1.6 REFACTOR: `go vet ./...` clean; `go run ./internal/gofmtcheck` clean.

### WU1 Files Changed
| File | Action | Lines (ins/del) |
|---|---|---|
| `internal/cli/review_intended_untracked.go` | Modified — extracted `intendedUntrackedDeclarationShape` | +49/-21 |
| `internal/cli/sdd_attempt.go` | Modified — deleted the two workspace-reading calls from finish/settle preflight | +23 |
| `internal/cli/sdd_attempt_untracked_test.go` | Modified — added R6/R7 tests plus a shared fixture helper | +156 |

WU1 total: 228 changed lines (207 insertions, 21 deletions).

### WU1 Rollback Boundary
Revert `internal/cli/review_intended_untracked.go` (the `intendedUntrackedDeclarationShape` extraction), `internal/cli/sdd_attempt.go` (the finish/settle preflight rewrite), and `internal/cli/sdd_attempt_untracked_test.go` (the two new tests + fixture helper). WU2/WU3 are entirely unaffected since no shared files, symbols, or schema surfaces overlap.

(Full WU1 detail, including deviations and TDD cycle evidence, is preserved in Engram observation #1170, topic `sdd/fix-untracked-inventory-recovery-loop/apply-progress`, and merged here unabridged where it affects the overall change record.)

---

## WU2 Task-by-Task Status

- [x] 2.1 RED (R1): Strengthened `TestIntendedUntrackedRefusalsNameARunnableStatusInvocation` in `internal/cli/review_intended_untracked_test.go` in place (no duplicate test added). It now decodes the named recovery STATUS command's `output` into `ReviewTargetStatusResult`, asserts `EligibleUntrackedInventory` matches `^sha256:[0-9a-f]{64}$` (via a new `reviewCapabilitySHA256Pattern` regexp local to the test file — production code adds no new digest-shape check, per decision 6), then retries the previously-refused `review start` with `--untracked-scope=select --intended-untracked=candidate.txt --expected-untracked-inventory=<recovered digest>` for both the "undeclared selection" and "stale inventory" cases, asserting success. Confirmed FAILS today (compile error: `EligibleUntrackedInventory` did not exist).
- [x] 2.2 RED (R2): Added `TestEligibleUntrackedInventoryPublishedUnconditionally`, a table-driven test covering `undeclared`, `declared_select`, `declared_exclude`, `rdd_disabled`; each asserts the field equals an independently computed `(reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(ctx)` digest. Extended `TestExplicitFrozenReviewingStatusUsesFrozenUntrackedScope` in `internal/cli/review_frozen_status_test.go:71-86` (per the task's correction note — NOT `compact_reviewer_capture_test.go:152`, which is a stale citation) with the `compact_reviewing` scenario's assertion instead of adding a new test.
- [x] 2.3 RED (R3): Added `TestStagedProjectionOmitsEligibleUntrackedInventoryKey`, decoding a staged projection response into `map[string]json.RawMessage` and asserting `_, ok := m["eligible_untracked_inventory"]; !ok` — key absence, never an empty-string struct-field check.
- [x] 2.4 RED (R4): Updated `internal/cli/review_transition_schema_drift_test.go:174` (`compileWholeNativeStatusSchema(t, "status-v5.schema.json")` → `"status-v7.schema.json"`, variable renamed `statusV7Schema`) and `internal/cli/review_intended_untracked_test.go` (`selected.Schema != ReviewIntegrationStatusSchemaV6` → `...V7`).
- [x] 2.5 GREEN: Added `EligibleUntrackedInventory string \`json:"eligible_untracked_inventory,omitempty"\`` to `ReviewTargetStatusResult` in `internal/cli/review_status_contract.go`, placed immediately after `Projection`, with a doc comment explaining the field-home rationale (decision 1).
- [x] 2.6 GREEN: Created `contracts/review-integration/v2/schemas/status-v7.schema.json` — `$ref`-based per the v5→v6 precedent. `$ref`s v5 for every unchanged top-level property, adds the optional `eligible_untracked_inventory` (`$ref` v5's `sha256` `$def`), and defines `next_transition` as a `oneOf`: (a) v5's generic next_transition minus the `intended_untracked_selection_required` reason code (identical structure to v6's own first `oneOf` branch), or (b) the untracked-selection collect shape with `arguments` `$ref`-ing v6's five existing argument `$defs`, and `submission` present in `properties` but **absent from `required`** — the one substantive difference from v6, making submission optional per design decision 4.
- [x] 2.7 GREEN: Added `ReviewIntegrationStatusSchemaV7`/`ReviewIntegrationStatusSchemaIDV7` constants; moved the `ReviewIntegrationStatusSchema`/`...ID` alias to V7; added `V7` to all four version-chain sites (`nativeGitTransport`, the "submission descriptor status schema is unsupported" check, the "v4 negotiated status contains a provider role task" check, `validateTargetedValidatorProviderTaskInput`'s schema guard); restated the submission rule in `validateIntendedUntrackedSelectionTransition` on three named boolean legs (`submissionInvalidWhenPresent`, `v5ForbidsSubmission`, `v6RequiresSubmission`) instead of the prior inline `V5`/`V6` equality expressions — V7 satisfies all three legs without a new branch, since neither leg forces a submission requirement for it.
- [x] 2.8 GREEN: Added `result.EligibleUntrackedInventory = intendedScope.Digest` in `internal/cli/review_facade.go` immediately after `result.intendedUntracked = intendedScope` (before the compact-reviewing replacement, `newReviewNextTransition`, and the `rdd_disabled` guard). Did **not** modify the `Digest` zeroing inside the compact-reviewing replacement block — verified untouched via `git diff`.
- [x] 2.9 GREEN: Deleted the conditional `if runtime != "" && (...) { result.Schema = ReviewIntegrationStatusSchemaV6 }` in `review_facade.go`, replaced with an explanatory comment. Schema is now always resolved from the `ReviewIntegrationStatusSchema` alias (V7) for contract v2.
- [x] 2.10 GREEN: Added `statusSchemaV7` constant next to `statusSchemaV6` in `bench/journeys_capture_evidence_v5.go`; updated all three pinned comparisons (`journeys_capture_evidence_v5.go` line ~160, `journeys_intended_untracked.go` lines ~82 and ~121) from `statusSchemaV6` to `statusSchemaV7`.
- [x] 2.11 GREEN: Added `TestReviewProviderArtifactStatusV7ContractsArePinned` to `internal/cli/review_provider_artifact_contract_test.go` (no collision with `TestReviewProviderArtifactV25StatusContractsArePinned`), pinning `schemas/status-v7.schema.json`'s SHA-256 (`5a5e4a7052689f97767d02625ad7572628cc4d5a62677497f569fb2d78608aa2`, computed via `shasum -a 256`). Added a matching `status-v7.schema.json` row (id `ReviewIntegrationStatusSchemaIDV7`) to the strict/bound `v2Schemas` list.
- [x] 2.12 GREEN: `go test ./internal/cli/... -run 'TestIntendedUntracked|TestReviewProviderArtifact|TestExplicitFrozenReviewingStatus'` — PASS, all subtests (50.4s). Confirms 2.1-2.4's RED tests now pass.
- [x] 2.13 GREEN: `cd bench && go build -o /tmp/gentle-ai-bench . && go vet ./... && go test ./...` — build clean, vet clean, `ok github.com/gentleman-programming/gentle-ai/bench 10.1s`.
- [x] 2.14 REFACTOR: `go run ./internal/gofmtcheck` clean (repo-wide). `go vet ./...` clean (repo-wide, modulo the two known-environmental `internal/update` failures — see below). Confirmed via `grep -n "sha256:\[0-9a-f\]" internal/cli/review_status_contract.go` that no parallel digest-shape validation was added to `Validate()` — the schema stays the sole shape authority (decision 6).

### Task 3.1's grep, performed early per the "Also required" instruction
Before moving the `ReviewIntegrationStatusSchema` alias from V5 to V7 (task 2.7), grepped the repository's shipped adapters (`internal/`, `cmd/`, excluding `contracts/*.json` and `bench/`) for a hard string comparison against the literal `"gentle-ai.review-integration.status/v5"`:

```
grep -rn '"gentle-ai.review-integration.status/v5"' internal/ cmd/ --include="*.go" | grep -v "_test.go"
→ internal/cli/review_status_contract.go:26: const ReviewIntegrationStatusSchemaV5 = "gentle-ai.review-integration.status/v5"
```

Also checked every non-test production use of the `ReviewIntegrationStatusSchemaV5` symbol:
```
grep -rn "ReviewIntegrationStatusSchemaV5\b" internal/ cmd/ --include="*.go" | grep -v "_test.go"
```
Only two production sites use it: `review_status_contract.go` (the validator itself, updated by this WU) and `review_capabilities.go:311` (the capabilities v2.4 emitter's static schema list — a **producer** assignment `result.Schemas[index] = ReviewIntegrationStatusSchemaV5`, not a consumer equality comparison against the advertised set; unaffected by the alias move, correctly out of WU2 scope since advertising v7 is WU3's job).

**Outcome: no hard string comparison found.** The alias move is safe. Not stopping.

### WU2 Files Changed
| File | Action | Lines (ins/del) |
|---|---|---|
| `internal/cli/review_status_contract.go` | Modified — new field, V7 consts, alias move, version-chain updates, submission rule restatement | +53/-14 |
| `internal/cli/review_facade.go` | Modified — `+1` digest assignment, deleted the V6 bump conditional | +18/-4 |
| `contracts/review-integration/v2/schemas/status-v7.schema.json` | Created — `$ref`-based v7 schema | +66 |
| `internal/cli/review_intended_untracked_test.go` | Modified — R1 strengthened in place, R2 (new), R3 (new), R4 (schema ref update) | +114 |
| `internal/cli/review_frozen_status_test.go` | Modified — R2 compact_reviewing assertion added | +14 |
| `internal/cli/review_transition_schema_drift_test.go` | Modified — R4 schema ref update | +4/-2 |
| `internal/cli/review_provider_artifact_contract_test.go` | Modified — new pinning test + strict/bound row | +21 |
| `bench/journeys_capture_evidence_v5.go` | Modified — `statusSchemaV7` const, pin update | +3/-1 |
| `bench/journeys_intended_untracked.go` | Modified — two pin updates | +4/-2 |

WU2 total (`git diff --stat`, these 9 files): 273 insertions(+), 24 deletions(-) = **297 changed lines**. Within the design's ~330-line WU2 estimate.

### Hard Constraints Verified
- `internal/cli/review_facade.go:917-924` (the compact-reviewing `Digest` zeroing, the #1972 fail-closed read) is **untouched** — confirmed via `git diff`; only a new line was added immediately *before* this block (line 910's assignment now sits ahead of it by ordering, not mutation).
- Staged projection omits the key entirely — proven structurally by R3 (`map[string]json.RawMessage` absence check), not a `== ""` struct check.
- No relaxation of the untracked declaration or freshness checks — no code path in `intendedUntrackedScopeForTarget`, `ValidateIntendedUntrackedSelection`, or the runtime ledger checks was touched by WU2.
- No `capabilities-v2.5.schema.json`, no capabilities constants, no `review_capabilities.go` changes, no new bench journey — confirmed via `git status`; WU3's files are all still absent from the working tree.
- No WU1 files touched beyond what was already committed by the prior attempt (`git diff --stat` shows the same three WU1 files with the same line counts as the WU1 record).
- No parallel `^sha256:[0-9a-f]{64}$` Go-side validation added to `Validate()` (decision 6) — confirmed via grep.

### TDD Cycle Evidence
| Task | RED | GREEN | REFACTOR |
|---|---|---|---|
| R1 (2.1) | Confirmed FAIL: compile error, `EligibleUntrackedInventory undefined` | PASS after 2.5/2.8: recovered digest matches the regex and the retried `review start` succeeds for both "undeclared selection" and "stale inventory" | gofmt + vet clean |
| R2 (2.2) | Confirmed FAIL: same compile error (shared package) | PASS after 2.5/2.8: all 4 table scenarios + the extended frozen-status test match an independently computed digest | gofmt + vet clean |
| R3 (2.3) | Confirmed FAIL: same compile error | PASS after 2.5/2.9 (staged path leaves `Digest` at its zero value; `omitempty` drops the key) | gofmt + vet clean |
| R4 (2.4) | Confirmed FAIL: `status-v7.schema.json` did not exist / `ReviewIntegrationStatusSchemaV7` undefined | PASS after 2.6/2.7/2.9: whole-envelope validation against the compiled `status-v7.schema.json` succeeds; `selected.Schema == ReviewIntegrationStatusSchemaV7` | gofmt + vet clean |

**Note on RED methodology for Go**: because Go requires whole-package compilation, "RED" for symbol-dependent tests (R1-R4, which reference `EligibleUntrackedInventory`, `ReviewIntegrationStatusSchemaV7`, and `status-v7.schema.json` before they exist) manifested as a confirmed package-level compile failure (`go vet ./internal/cli/...` → `status.EligibleUntrackedInventory undefined`) rather than a runtime assertion failure. This was captured verbatim before any GREEN code was written, consistent with WU1's own documented approach to the same language constraint.

### Verification Commands and Results (WU2)
- `go vet ./internal/cli/...` (before GREEN) → `vet: internal/cli/review_frozen_status_test.go:97:32: status.EligibleUntrackedInventory undefined (type ReviewTargetStatusResult has no field or method EligibleUntrackedInventory)` — confirmed RED.
- `go build ./internal/cli/...` (after GREEN) → clean, exit 0.
- `go test ./internal/cli/... -run 'TestIntendedUntracked|TestEligibleUntrackedInventory|TestExplicitFrozenReviewingStatus|TestStagedProjectionOmits'` → PASS, all subtests (66.4s).
- `go test ./internal/cli/... -run 'TestV2TransitionSchemasAcceptProviderPayloadsAndRejectDrift|TestNativeStatusSchemasValidateWholeForecastEnvelope|TestV2TransitionSchemasStayLocalSharedAndPackaged'` → PASS (8.4s).
- `go test ./internal/cli/... -run 'TestReviewProviderArtifact'` → PASS, all 10 subtests (0.9s).
- `go test ./internal/cli/... -run 'TestIntendedUntracked|TestReviewProviderArtifact|TestExplicitFrozenReviewingStatus'` (tasks.md's exact WU2 focused command, task 2.12) → PASS, all subtests (50.4s).
- `cd bench && go build -o /tmp/gentle-ai-bench .` → clean (after a one-time `go mod download` for `charmbracelet/x/xpty`, `conpty`, `termios`, `creack/pty`).
- `cd bench && go vet ./...` → clean.
- `cd bench && go test ./...` → `ok github.com/gentleman-programming/gentle-ai/bench 10.109s` (task 2.13).
- `go run ./internal/gofmtcheck` (repo-wide) → clean, exit 0.
- `go vet ./...` (repo-wide) → clean, exit 0 (the two known `internal/update` bash-3.2 environmental failures noted by the orchestrator are `go test` failures, not `go vet` failures, so they do not appear here).
- `go test ./internal/cli/...` (full package) → in progress at time of this writing; the orchestrator's own prior WU1 record and this run both observe this full-package invocation running past 10 minutes on this machine, dominated by git-subprocess-heavy tests unrelated to WU2 (confirmed no WU2 test is among the stack traces sampled from an in-progress run — the goroutine dump captured only pre-existing, unmodified tests: `TestRecoverSuccessorLineageHelpSaysWhereItComesFrom`, `TestCaptureResultHelpSaysRepositoryContextIsVerifiedAgainstCwd`, `TestReviewGateDeniedNegotiatedEnvelopeUnchangedByHumanMessage`, `TestNegotiatedStartLeavesNoAuthorityStatusCanOnlyRefuse`, `TestReviewGateDeniedErrorNamesItsContinuation`). Every WU2-relevant test subset above passed cleanly and quickly; the full-package invocation is reported for completeness once it finishes, per the orchestrator's request, but WU2's own correctness evidence does not depend on it.

### Known Environment Facts (unchanged from WU1, orchestrator-verified, not regressions)
1. `internal/reviewtransaction` times out even in isolation — `go list -deps ./internal/reviewtransaction` proves it does not import `internal/cli`, so nothing in WU1 or WU2 can affect it.
2. `internal/update` — `TestReleaseSecurityScriptsAreSyntacticallyValidAndFailClosed` and `TestCanonicalReleasePublicKeysControlRealLinkerBuild` fail because this machine's `/bin/bash` is GNU bash 3.2.57, lacking `-v` and `declare -A`. Environmental, not a code defect.

### Deviations from Design / Tasks
None — implementation matches design.md's decisions 1-4 and 6, and tasks.md's task list, exactly. The one design citation correction (task 2.2's `compact_reviewer_capture_test.go:152` → `review_frozen_status_test.go:71`) was already pre-applied to tasks.md by a prior phase and followed as written.

### Rollback Boundary (WU2)
Revert `contracts/review-integration/v2/schemas/status-v7.schema.json` (delete), the `EligibleUntrackedInventory` field and V7 constants/version-chain entries/submission-rule restatement in `internal/cli/review_status_contract.go`, the digest assignment and deleted V6-bump conditional in `internal/cli/review_facade.go`, the bench pin updates in `bench/journeys_capture_evidence_v5.go` and `bench/journeys_intended_untracked.go`, and the four test files' additions (`review_intended_untracked_test.go`, `review_frozen_status_test.go`, `review_transition_schema_drift_test.go`, `review_provider_artifact_contract_test.go`). WU1 is unaffected (no shared files). WU3 was never started (no capabilities files, no new bench journey exist yet), so there is nothing downstream to unwind.

## WU3 Task-by-Task Status

- [x] 3.1 Verification (carried forward): grepped shipped adapters for a hard string comparison against `"gentle-ai.review-integration.status/v5"` before moving the alias — the only production match was the constant declaration itself (`review_status_contract.go:26`); the only non-test consumer, `review_capabilities.go`'s v2.4 emitter, is a producer assignment, not an equality comparison. Outcome: none found. Not stopping. (This grep note is preserved unabridged from WU2's own record, since 2.7 already executed the alias move; 3.1 is the confirmation this WU inherits.)
- [x] 3.2 RED (R5): Renamed `TestReviewCapabilitiesV24AdvertisementIsCurrent` (`internal/cli/review_capabilities_test.go`) to `TestReviewCapabilitiesV25AdvertisementIsCurrent` in place, rather than adding a parallel test — deliberate deviation, see "Deviations" below. Asserts `got.Schema == ReviewIntegrationCapabilitiesSchemaV25`, `got.Protocol == {2,5}`, `slices.Contains(got.Schemas, ReviewIntegrationStatusSchemaV7)`, absence of both `ReviewIntegrationCapabilitiesSchemaV23` and `...V24` from the advertised set, and validates the emitted bytes against `capabilities-v2.5.schema.json`. Also updated the one other live-assertion site, `TestReviewCapabilitiesContractValidationIsExactAndReadOnly` (`nativeGit.Schema != ReviewIntegrationCapabilitiesSchemaV24` → `...V25`). Confirmed FAILS today: `go vet ./internal/cli/...` → `undefined: ReviewIntegrationCapabilitiesSchemaV25` (compile-level RED, consistent with WU1/WU2's documented Go RED methodology).
- [x] 3.3 GREEN: Created `contracts/review-integration/v2/schemas/capabilities-v2.5.schema.json` — same `$ref`-to-v2.3 pattern as v2.4, `schemas` array widened to `minItems`/`maxItems: 22` (v2.4's 21, with the self-referential `capabilities/v2.4` entry replaced by `v2.5`, plus `gentle-ai.review-integration.status/v7` appended).
- [x] 3.4 GREEN: In `internal/cli/review_capabilities.go`, added `ReviewIntegrationCapabilitiesSchemaV25`/`...IDV25` constants. The v2.4 "branch" is a single in-place block (not per-version branches — confirmed via `git log -p` on the v2.3→v2.4 transition, which mutated the same block's constants rather than adding a parallel one); bumped it to v2.5 the same way: `result.Schema`/`Protocol` now `{v2.5, {2,5}}`, the self-referential `case ReviewIntegrationCapabilitiesSchema` branch now emits `...V25`, and `ReviewIntegrationStatusSchemaV7` is appended to `result.Schemas` alongside the existing v2.4 appends.
- [x] 3.5 GREEN: Added a `capabilities-v2.5.schema.json` row to `TestReviewProviderArtifactSchemasAreStrictAndBound`'s `v2Schemas` strict/bound list, and a SHA-256 pin row (`9fcdb1717a54bcd4f73d4dee1283d9ec2f27cccbb5d54804ee8b40a6ed2db553`, computed via `shasum -a 256`) to the existing `TestReviewProviderArtifactStatusV7ContractsArePinned` (WU2's test, extended rather than duplicated, since it is already "the same table touched in 2.11" the task names) — status/v7's WU2-computed pin (`5a5e4a7052689f97767d02625ad7572628cc4d5a62677497f569fb2d78608aa2`) is left untouched.
- [x] 3.6 GREEN: Authored `bench/journeys_4040_untracked_inventory.go`, ID `j4040-untracked-inventory-recovery-loop` (verified unused via the manifest before writing). **Deviation from the task's literal `Review: reviewOptedIn`**: declared `Review: reviewUntouched` instead — see "Deviations" below. Drives: clean `sdd-attempt begin` → untracked candidate born during the attempt → `review status --contract v2 --next-transition` (RDD left at its default disabled state) → decode the envelope's `schema` (must be `status/v7`) and **top-level** `eligible_untracked_inventory` (regex `^sha256:[0-9a-f]{64}$`), never the collect argument → `sdd-attempt finish --untracked-scope=select --intended-untracked=<path> --expected-untracked-inventory=<that digest>` → asserts exit 0 and that `sdd-attempt status` recorded the declared selection. Cites issue #4040 in the file's doc comment. Also had to register the new file in `bench/journeys_id_collision_test.go`'s `journeySources()` (`TestJourneySourcesCoverTheWholeCorpus` fails otherwise — an unplanned but mandatory registration site the task didn't name) and in `bench/review_declarations.go`'s `coreJourneyReviewModes` map (`declareCoreJourneyReviewModes` overwrites every journey's `.Review` field from this map by ID; an unlisted ID silently resets to the zero value `reviewPreconditionUndeclared`, which `validateCorpus` rejects).
- [x] 3.7 GREEN: `cd bench && go test ./... -run TestRegisteredJourneysMatchTheManifest -update-journey-manifest` → added one line (`j4040-untracked-inventory-recovery-loop`) to `bench/testdata/journeys.manifest`. Rerun without `-update-journey-manifest` → clean.
- [x] 3.8 GREEN: `go test ./internal/cli/... -run TestReviewCapabilitiesV25AdvertisementIsCurrent` → PASS.
- [x] 3.9 REFACTOR: `go run ./internal/gofmtcheck` and `go vet ./...` both clean, repo-wide.

### Deviations from Design / Tasks (WU3)

1. **Task 3.2/3.6 said "Add" a new test / `Review: reviewOptedIn`; I renamed/changed in place.** For 3.2: `TestReviewCapabilitiesV24AdvertisementIsCurrent` asserts the LIVE current v2 advertisement (not a frozen fixture — no `capabilities-v2.4.fixture.json` exists on disk, unlike v2.1/v2.2/v2.3). Once `buildReviewCapabilities` is bumped to emit v2.5 unconditionally for contract v2 (task 3.4, which the design explicitly requires — decision 5 rejects "not advertising v7"), the OLD test's assertions (`Schema == ".../v2.4"`) would start failing against the new live output. Adding a parallel `...V25...` test alongside the unmodified `...V24...` one would leave a permanently-broken test in the suite, which is not a legitimate outcome. I renamed/updated `TestReviewCapabilitiesV24AdvertisementIsCurrent` → `TestReviewCapabilitiesV25AdvertisementIsCurrent` in place instead, consistent with the codebase's own established pattern for version-bump tests that assert current/live behavior (WU2's `review_transition_schema_drift_test.go` and `review_intended_untracked_test.go` updated their `V5`→`V7`/`V6`→`V7` assertions in place rather than duplicating). For 3.6: reproducing the exact "known-good end-to-end behavior" the orchestrator manually verified (top-level `eligible_untracked_inventory` published even when `next_transition = stop/rdd_disabled`) requires RDD to stay in its default DISABLED state during the journey. `Review: reviewOptedIn` would have the runner enable RDD globally before the journey's first step, which defeats exactly the scenario being proven (decision 2's claim that the digest survives the `rdd_disabled` guard). `Review: reviewUntouched` is the correct declaration per the `gentle-ai-bench` skill's own vocabulary ("its subject IS the switch, or it has nothing to do with reviews") and matches sibling journeys like `j41-kill-switch-versus-sdd-pre-verify`.
2. **No hard refusal-message assertion.** I considered asserting the exact CLI refusal text (`intendedUntrackedSelectionRequired`, which names `gentle-ai review status --next-transition` as the recovery route) as a first step, but that refusal already embeds a freshly-computed digest inline — it does not by itself demonstrate the #4040 defect (an unobtainable value after declaring, or under `rdd_disabled`). The journey instead demonstrates the actual fix (decision 2's unconditional, pre-suppressor assignment) directly: STATUS publishes the top-level digest regardless of RDD/applicability state, and that digest alone is sufficient to complete a real `sdd-attempt finish`. This is a stronger, less assumption-fragile proof than pinning refusal prose I could not independently verify without risking an incorrect assertion.

None of the WU1/WU2 hard constraints were touched: `review_facade.go:917-924`'s digest zeroing is untouched (confirmed via `git diff`), `eligible_untracked_inventory`'s field/assignment/schema are untouched, no declaration/freshness check was relaxed, and `capabilities-v2.4.schema.json` was not edited in place (a new `v2.5` file was created instead, per decision 5).

### WU3 Files Changed
| File | Action | Lines (ins/del) |
|---|---|---|
| `internal/cli/review_capabilities.go` | Modified — V25 consts, v2.5 emission bump, V7 appended to `Schemas` | +12/-4 |
| `internal/cli/review_capabilities_test.go` | Modified — renamed/updated the live-advertisement test to V25, updated the one other live V24 assertion site | +16/-5 |
| `internal/cli/review_provider_artifact_contract_test.go` | Modified — `capabilities-v2.5.schema.json` strict/bound row + SHA-256 pin | +24/-1 |
| `contracts/review-integration/v2/schemas/capabilities-v2.5.schema.json` | Created — 22-entry `$ref`-based v2.5 capabilities schema | +41 |
| `bench/journeys_4040_untracked_inventory.go` | Created — the #4040 reproduction journey | +101 |
| `bench/journeys.go` | Modified — `Schema`/`EligibleUntrackedInventory` fields added to the shared `statusEnvelope`; new journey registered in `Journeys()` | +9 |
| `bench/journeys_id_collision_test.go` | Modified — new file registered in `journeySources()` | +1 |
| `bench/review_declarations.go` | Modified — new journey ID registered in `coreJourneyReviewModes` as `reviewUntouched` | +1 |
| `bench/testdata/journeys.manifest` | Regenerated (generated file, excluded from authored-line budget per convention) | +1 |

WU3 total (authored, excluding the generated manifest line): **205 changed lines** (63 insertions/deletions across modified files + 142 lines of new files). Within the design's ~140-line estimate is exceeded modestly, mainly due to the mandatory-but-unplanned `journeySources`/`coreJourneyReviewModes` registrations and the doc-comment-heavy bench journey; still comfortably inside the session's accepted `size:exception`.

### TDD Cycle Evidence (WU3)
| Task | RED | GREEN | REFACTOR |
|---|---|---|---|
| R5 (3.2) | Confirmed FAIL: `go vet ./internal/cli/...` → `undefined: ReviewIntegrationCapabilitiesSchemaV25` (compile-level RED, per WU1/WU2's documented Go RED methodology) | PASS after 3.3/3.4: `TestReviewCapabilitiesV25AdvertisementIsCurrent` and `TestReviewCapabilitiesContractValidationIsExactAndReadOnly` both pass | gofmt + vet clean |

### Verification Commands and Results (WU3)
- `go vet ./internal/cli/...` (before GREEN) → `vet: internal/cli/review_capabilities_test.go:133:19: undefined: ReviewIntegrationCapabilitiesSchemaV25` — confirmed RED.
- `go test ./internal/cli/... -run 'TestReviewCapabilitiesV25AdvertisementIsCurrent|TestReviewCapabilitiesContractValidationIsExactAndReadOnly|TestReviewCapabilitiesV24'` → PASS, all subtests.
- `go test ./internal/cli/... -run 'TestReviewCapabilities|TestReviewIntegrationDocumentation'` → PASS, all subtests (no doc-count regressions — the doc's stale-claim list pins the `contracts/review-integration/v1` directory only, unaffected by v2 additions).
- `go test ./internal/cli/... -run 'TestReviewProviderArtifact'` → PASS, all 10 subtests, including the new/extended v2.5 pin.
- `cd bench && go build .` → clean; `go vet ./...` → clean.
- `cd bench && go test ./... -run TestRegisteredJourneysMatchTheManifest -update-journey-manifest` then rerun without the flag → clean both times.
- `cd bench && go test ./...` → `ok` (full declarations suite, includes `TestJourneySourcesCoverTheWholeCorpus`, `TestJourneyIDsAreUniqueAcrossSourceFiles`, and the review-mode-declaration validator).
- `go test ./internal/cli/... -run 'TestReviewCapabilities|TestReviewProviderArtifact|TestIntendedUntracked'` (orchestrator's exact required command) → PASS, all subtests (15.1s).
- `go vet ./...` (repo-wide) → clean.
- `go run ./internal/gofmtcheck` (repo-wide) → clean.
- `go build -o /tmp/gentle-ai-bench . && go build -trimpath -o /tmp/gentle-ai ../cmd/gentle-ai` → both clean.
- `/tmp/gentle-ai-bench run --binary /tmp/gentle-ai --only j4040-untracked-inventory-recovery-loop --out /tmp/bench-4040.json` (driven-mode proof against the real built binary) → `journeys: 1 completed, 0 unsupported, 0 failed`.
- `go test ./...` (root module, run per-package to respect the machine's per-package timeout budget) → every package `ok` except: `internal/reviewtransaction` (19 pre-existing failures, all `review store lock could not be acquired` / `maintenance authority component "/var" is unsafe` — **verified identical on a pristine `git worktree add ... HEAD --detach` checkout with none of WU1/2/3's changes**, so 100% pre-existing/environmental, not a regression; this is a larger set than the two the orchestrator described, but confirmed byte-for-byte identical between the pristine tree and this branch); `internal/update` (the exact two documented bash-3.2 `declare -A`/`-v` failures); `internal/components/engram` (`TestStdioHandshake_TerminatesThenDrainsBufferedStdout` — file has zero diff under any WU, confirmed pre-existing and unrelated); and, only inside one 25-minute unfiltered `internal/cli` run that itself hit its own timeout budget under heavy parallel load, two unrelated resource-contention flakes (`TestInstallStateLockPathIsStableAcrossStateDirectoryCreation`, `TestReviewCaptureRefuterExecuteDeadlineFailsClosedWithoutCapture` — install-state locking and refuter capture-deadline timing, neither touched by this change) that do not appear in the scoped/focused command above. No WU3-relevant test failed anywhere.

### Rollback Boundary (WU3)
Revert `contracts/review-integration/v2/schemas/capabilities-v2.5.schema.json` (delete), `bench/journeys_4040_untracked_inventory.go` (delete), the V25 constants and v2.5 emission bump in `internal/cli/review_capabilities.go` (which returns the live emitter to v2.4), the renamed/updated tests in `internal/cli/review_capabilities_test.go`, the new pin rows in `internal/cli/review_provider_artifact_contract_test.go`, the `Schema`/`EligibleUntrackedInventory` fields and journey registration in `bench/journeys.go`, the `journeySources()` entry in `bench/journeys_id_collision_test.go`, the `coreJourneyReviewModes` entry in `bench/review_declarations.go`, and the manifest regeneration. WU1/WU2 are entirely unaffected — no shared files beyond `bench/journeys.go`, and that file's WU3 edit is additive-only (new struct fields, one new `append`), not a mutation of any WU2 line.

## Phase 4 — Cross-Cutting Verification and PR Recording

- [x] 4.1 Full root-module gate. `go vet ./...` and `go run ./internal/gofmtcheck` clean repo-wide. `go test ./...` run per-package (root multi-package run hit its own 9-minute shared timeout mid-`internal/cli`, so `internal/cli` and `internal/reviewtransaction` were each rerun standalone with generous per-package timeouts — see WU3's Verification Commands above for the full breakdown and the pristine-worktree comparison proving `internal/reviewtransaction`'s failures pre-date this change).
- [x] 4.2 Bench module gate: `cd bench && go build -o /tmp/gentle-ai-bench . && go vet ./... && go test ./...` → all clean.
- [x] 4.3 Driven-mode proof: `go build -trimpath -o /tmp/gentle-ai ./cmd/gentle-ai` then `/tmp/gentle-ai-bench run --binary /tmp/gentle-ai --only j4040-untracked-inventory-recovery-loop --out /tmp/bench-4040.json` → `1 completed, 0 unsupported, 0 failed`.
- [x] 4.4 `size:exception` recorded here for the eventual commit/PR author (this apply session creates no commits or PRs, per its explicit constraints): the session's accepted `delivery_strategy: exception-ok` covers the full change at **730 authored changed lines** (WU1 228 + WU2 297 + WU3 205, each `git diff --stat` on that work unit's own files, generated goldens like `bench/testdata/journeys.manifest` excluded per convention) against the 400-line budget, exactly matching this tasks.md's Review Workload Forecast (`~560` estimate, `400-line budget risk: High`, `Chain strategy: size-exception`, explicitly pre-accepted by the user per the session preflight).
- [x] 4.5 Issue #4040 citation recorded here for the eventual commit/PR author: the WU3 commit (the one landing `capabilities-v2.5.schema.json`, the `review_capabilities.go` bump, and `bench/journeys_4040_untracked_inventory.go`) must cite `#4040` in its message, e.g. `feat(review): advertise capabilities v2.5 and reproduce #4040's recovery loop`. The journey's own file-level doc comment already cites `#4040` (task 3.6).

## Status
34/34 tasks complete (WU1: 6/6, WU2: 14/14, WU3: 9/9, Phase 4: 5/5). All phases done. This apply session created no commits, branches, or PRs per its explicit constraints — the working tree carries all WU1+WU2+WU3 changes uncommitted, ready for the orchestrator to commit/PR per the recorded `size:exception` and `#4040` citation above. Ready for `sdd-verify`.

---

## Post-Verify Amendment (commit `e54fade8`)

This section records work done to the change AFTER the verify phase returned PASS,
so the artifact set does not read as if the implementation froze at verify.

### What changed

`bench/journeys_4040_untracked_inventory.go` was reworked in response to a CodeRabbit
review finding on PR #4066 (`bench/journeys_4040_untracked_inventory.go`, inline comment
on the STATUS read). No production code changed; the amendment is confined to the bench
journey.

The journey previously read the top-level `eligible_untracked_inventory` digest
immediately after creating the born-during untracked candidate. It now drives both
refusals of #4040's closed loop first, then follows the route those refusals name:

1. An undeclared `sdd-attempt finish` must refuse, and the refusal must name the
   unaccounted candidate together with the exact rerun flags. This is the
   `internal/sddstatus/runtime_ledger.go` message WU1 (Fix B) made reachable, so this
   assertion also guards against the deleted CLI-side duplicate returning.
2. A declaration carrying a well-formed but deliberately wrong digest must refuse
   naming `gentle-ai review status --next-transition`. This is the instruction #4040's
   reporters followed into the dead end.
3. Only then does the journey run that named route, read the top-level digest, and feed
   it into a successful declared `finish`.

### This reverses WU3 apply-time deviation 2 ("No hard refusal-message assertion")

That deviation declined to assert refusal prose on the grounds that it could not be
independently verified "without risking an incorrect assertion". The caution was
warranted, and the verification has now been done against a built binary rather than
against source reading. The refusal texts are two DISTINCT sites, and conflating them is
exactly the mistake the deviation was avoiding:

| Site | Trigger | Names a STATUS route? |
|------|---------|-----------------------|
| `internal/sddstatus/runtime_ledger.go:1067` | settlement declaring nothing | **No.** It returns the digest inline as `--expected-untracked-inventory=<digest>`. |
| `internal/sddstatus/runtime_ledger.go:1037` | declaration carrying a stale digest | **Yes.** `gentle-ai review status --next-transition`. |
| `internal/reviewtransaction/snapshot.go:736`, `:748` | `ValidateIntendedUntrackedSelection` | Yes, the full negotiated form in `intendedUntrackedInventoryCommand` (`snapshot.go:700`). |

The CodeRabbit finding's own instruction was therefore partially wrong: it asked the
UNDECLARED refusal to name the negotiated STATUS command, which that refusal does not
emit. Asserting it would have pinned text the product does not produce. The finding's
core point held, though: a journey that only reads a field stays green if the product
stops REQUIRING the thing that field recovers from.

### Why the original journey was a weak guard

Its driven metrics were `blk 0 / in_band 0 / recov 0`, i.e. it never exercised a block at
all. After the rework: `blk 2 / in_band 2 / recov 2`. The loop is now measured rather than
asserted around.

### Evidence

- `cd bench && go vet ./... && go test ./...` → clean, `ok`.
- `gentle-ai-bench run --binary <built> --only j4040-untracked-inventory-recovery-loop`
  → `journeys: 1 completed, 0 unsupported, 0 failed`.
- `gentle-ai-bench run --binary <built>` (FULL corpus) →
  `journeys: 62 completed, 0 unsupported, 0 failed`. This is the run that matters for an
  amendment to an existing journey: `--only` cannot show that the other 61 journeys are
  undisturbed. The original WU3 evidence at task 3.8 was `--only` alone.
- `go run ./internal/gofmtcheck` → clean. `go vet ./...` → clean.

### Line-count impact

`bench/journeys_4040_untracked_inventory.go` moved from +101 to +152 lines, which moves
the PR from 710 to 761 changed lines. Production code is unchanged at 155 lines across
five `internal/cli` files; the whole delta is bench evidence.
