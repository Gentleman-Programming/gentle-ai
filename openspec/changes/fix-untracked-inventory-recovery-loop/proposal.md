# Proposal: Fix Untracked Inventory Recovery Loop

## Intent

Issue #4040: refusals from `intendedUntrackedScopeForTarget` and `ValidateIntendedUntrackedSelection` demand `--expected-untracked-inventory` and name `gentle-ai review status --next-transition` as the recovery route. That route never publishes the digest they demand. Reproduced by four reporters on 2.5.0-rc.3 and 2.5.0 across macOS/Codex, Linux/Pi, and Windows Git Bash.

Two causally independent invariants produce one symptom:

- **A — unobtainable value.** The canonical digest exists on every call (`review_intended_untracked.go:79`) but is emitted only inside `next_transition.collect.inputs[].arguments`, which fires only when `!Declared`. Following the refusal sets `Declared = true` and disables publication. `rdd_disabled` stops and the compact-reviewing scope replacement (`review_facade.go:917-923`, which zeroes `Digest`) suppress it independently.
- **B — the worse message wins.** The CLI finish/settle preflight (`sdd_attempt.go:133-139`) recomputes the same freshness check and fails before the ledger's better-informed `runtimeBornDuringUntrackedRefusal` (`runtime_ledger.go:1061-1067`).

Both fixes delete conditions. Neither weakens the safety invariant.

## Scope

### In Scope
- Publish `eligible_untracked_inventory` as a top-level status projection field, unconditional on `Declared`, `NeedsSelection`, RDD mode, and the compact-reviewing path — which must preserve or recompute a real digest.
- `omitempty`: **absent** for the `staged` projection. Absent means "not applicable"; an empty digest would falsely assert "checked, found none".
- Add `status-v7.schema.json` with `ReviewIntegrationStatusSchemaV7`/`SchemaIDV7`, a `Schemas` entry (`review_capabilities.go` ~314), and an updated pinned SHA-256 guard.
- Delete the redundant finish/settle CLI precondition so the ledger refusal surfaces.
- Regression tests: field presence across undeclared/declared/RDD-disabled/compact-reviewing, staged-absent, end-to-end refusal→STATUS→retry, and finish/settle message provenance.

### Out of Scope
- Relaxing, bypassing, or making optional the untracked declaration or its freshness check.
- Redesigning the untracked scope contract, flags, or selection semantics.
- Auto-declaring, auto-selecting, or auto-excluding on the caller's behalf.
- Any new authority store, lineage, or lifecycle state.
- `begin`/`acquire` preflight changes beyond what A requires — it has no ledger-side equivalent.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `review-findings-ledger`: status projection publishes the eligible-untracked-inventory digest unconditionally; staged omits it; schema v7 advertisement.
- `rdd-sdd-receipt-consumption`: the SDD ledger, not the CLI, owns finish/settle untracked-freshness refusal.

## Approach

Thread the digest already returned at `review_intended_untracked.go:79` into `ReviewTargetStatusProjection` and keep it across every scope-replacement path. Wire schemas are strict closed shapes (`additionalProperties: false` six times in v6), so an optional field still needs a new version file per this repo's v1–v6 precedent. Then delete the duplicate CLI check. Two work units, separable at task level, one PR.

## Affected Areas

| Area | Impact |
|------|--------|
| `internal/cli/review_status_contract.go` | New top-level field + validation |
| `internal/cli/review_facade.go` | Unconditional publication; fix 917-923 zeroing |
| `internal/cli/review_capabilities.go` | Advertise v7 |
| `contracts/review-integration/v2/schemas/status-v7.schema.json` | New schema |
| `internal/cli/sdd_attempt.go` | Delete finish/settle preflight |
| `internal/cli/review_provider_artifact_contract_test.go` | Repin SHA-256 |
| `internal/sddstatus`, `internal/reviewtransaction` | Read/depended-on only; **no production change expected** — report explicitly if apply proves otherwise |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Empty string shipped where absent is required | Medium | Dedicated staged-absent test asserting key absence, not value |
| Compact-reviewing path regresses to zero digest | Medium | Explicit compact-reviewing presence test |
| Bundling A and B produces oversized scope | Medium | Separable task units; forecast 250-370 vs 400 budget |
| Consumer breaks on unknown field | Low | `AdditiveMinorPolicy: optional-fields-only`; v7 is additive |

## Rollback Plan

Revert the PR and drop the v7 advertisement; consumers ignore unknown optional fields per `AdditiveMinorPolicy`, so no stored state or migration is affected. v6 remains the advertised schema and the loop returns.

Partial rollback leaves a known-bad state, so prefer full revert:
- **A without B**: the digest is obtainable, but finish/settle still emits the generic context-free message instead of the ledger's.
- **B without A**: the ledger's better message surfaces but still names a recovery route that publishes nothing — the loop persists with better prose.

## Dependencies

None. No overlap with `sdd-compact-authority-recovery`, `rdd-root-simplification-wave7`, `fix-review-lifecycle-loop`, `organic-rdd-recovery`, `review-lifecycle-hardening`, or `complete-native-review-lifecycle`.

## Success Criteria

- [ ] `gentle-ai review status --next-transition` publishes `eligible_untracked_inventory` when declared, undeclared, RDD-disabled, and compact-reviewing.
- [ ] The `staged` projection omits the field entirely.
- [ ] An end-to-end test reproduces #4040 and proves refusal → STATUS → retry now succeeds.
- [ ] Finish/settle refusal carries the ledger message, not the CLI generic one.
- [ ] The declaration requirement and freshness check are unchanged in strictness.
- [ ] `go test ./...`, `go vet ./...`, and `go run ./internal/gofmtcheck` pass; authored change stays under 400 lines.
