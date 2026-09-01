# Tasks: RDD Root Simplification — Wave 3 (New-Lineage Facade)

## Gate

Wave 3 chains AFTER Wave 2 lands on `feature/rdd-root-simplification`. Do not open Wave 3 PR #1 until Wave 2's final slice merges to the tracker branch (`openspec/changes/rdd-root-simplification-wave2` is currently unarchived — confirm merge before S1 starts).

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | S1 ~350, S2 ~800, S3 ~900, S4 ~700, S5 ~600 (total ~3350) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR0 → S1 → S2 → S3 → S4 → S5 |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | PR | Focused test | Harness | Rollback |
|---|---|---|---|---|---|
| PR0 | Land Wave 3 SDD artifacts + Wave 2 archive move (its turn) | tracker base | N/A (docs) | N/A — SDD artifacts only | Revert `openspec/changes/rdd-root-simplification-wave3/**` |
| S1 | Promotion rename + alias + guard split | PR0-base | `go test ./internal/reviewtransaction/... -run Guard` | N/A — pure rename, no runtime scenario | Revert rename commit; `shadow_*.go` restored |
| S2 | v3 AuthorityStore: artifacts, CAS, lock, replay, receipt | S1-base | `go test ./internal/reviewtransaction/... -run AuthorityStore` | `t.TempDir()` crash/replay integration | Delete `authority_store.go` + tests |
| S3 | ReviewCore start/finalize/validate + consent reuse | S2-base | `go test ./internal/reviewtransaction/... -run ReviewCore` | tier 0/1/4 bench journey | Delete `review_core*.go`; facade start branch reverts |
| S4 | Governing-authority branch, Amendment C, switch-off goldens | S3-base | `go test ./internal/cli/... -run GoverningAuthority` | 5-gate golden regeneration `-update` then verify | Revert `review_facade.go`:2872 branch |
| S5 | Reason-taxonomy regression, unwired offer API, bench | S4-base | `go test ./internal/cli/... -run ReasonTaxonomy` | full tier bench suite | Remove `review_offer.go`; drop baseline entry |

## Phase 1: PR0 — SDD Artifacts

- [x] 1.1 Land `openspec/changes/rdd-root-simplification-wave3/{proposal,specs,design,tasks}.md` (already written).
- [x] 1.2 Archive Wave 2 (`openspec/changes/rdd-root-simplification-wave2/**` → `openspec/specs/`) when its turn comes, mirroring prior wave pattern. **Completed out-of-band**: Wave 2 was archived to `openspec/changes/archive/2026-08-02-rdd-root-simplification-wave2/` with its own `archive-report.md` before this Wave 3 archive ran; the checkbox in this file was never updated to reflect it. See this archive's own report for the filesystem proof and reconciliation record.

## Phase 2 (S1): Promotion Rename

- [x] 2.1 RED: `candidate_readonly_guard_test.go` reusing `scanShadowReadOnlyTree` verbatim over the promoted files — fails (files don't exist yet). Targets the exact two promoted files by name rather than a `candidate_*.go` glob, since this package already has an unrelated `candidate_decline.go` (Wave 2) that a glob would silently sweep in and mask the RED state.
- [x] 2.2 Rename `shadow_relation.go`→`candidate_relation.go` (`shadowRelate`→`relateCandidates`, `ShadowRelation`→`CandidateRelation`); rename `shadow_identity.go`→`candidate_identity.go`.
- [x] 2.3 Add `type ShadowRelation = CandidateRelation` alias so `shadow_observer.go` compiles unchanged.
- [x] 2.4 GREEN: 2.1 passes; existing `shadow_readonly_guard_test.go` still covers `shadow_observer.go`/`shadow_authority_health.go`; full `go test ./internal/reviewtransaction/...` passes.
- [x] 2.5 `scripts/deadcode-ratchet.sh --update` for renamed symbols — 2 pre-existing baselined test-helper entries moved from `shadow_relation.go` to `candidate_relation.go` (reachability-neutral file-path rename only, no new dead code).

## Phase 3 (S2): AuthorityStore

- [x] 3.1 RED: two-artifact-only test — inspect `v3/<lineage>/` dir, assert exactly `review-state.json` + `review-receipt.json`, no sidecar.
- [x] 3.2 RED: CAS stale-revision refuse test; replay-identity equal ⇒ return stored transition, consume nothing; different in `correcting` ⇒ refuse.
- [x] 3.3 Create `internal/reviewtransaction/authority_store.go`: `v3/LOCK` store-scoped lock, `acquireStoreLock` + `.atomic-`/rename publish reuse, `Mutate(ctx, expectedRevision, apply)`.
- [x] 3.4 GREEN: 3.1, 3.2 pass. Crash/replay integration test with `t.TempDir()`.
- [x] 3.5 Receipt immutability test: mutate after issuance refuses.
- [x] 3.6 `scripts/deadcode-ratchet.sh --update` for `authority_store.go` exports.

## Phase 4 (S3): ReviewCore

- [x] 4.1 RED: consent-before-authority-freeze ordering test — tier 1/4 freeze blocked until v2 consent envelope granted; tier 0 carve-out proceeds without a consent question straight to freeze.
- [x] 4.2 Create `internal/reviewtransaction/review_core.go`, `review_core_transition.go`: `Next(ctx, authority, CoreRequest) (CoreTransition, error)`.
- [x] 4.3 Reuse `authorizeReviewStart`, `AssessSnapshotRisk`→`ClassifyRisk`, `SelectReviewLenses`, `CorrectionBudget` — no re-derivation.
- [x] 4.4 GREEN: 4.1 passes for all three tiers.
- [x] 4.5 Modify `review_facade.go`:1618 start branch: `GENTLE_AI_RDD_NEW_LINEAGE` OFF (default) → legacy `NewCompactState`; ON → `ReviewCore.Next`/`AuthorityStore.Create`.
- [x] 4.6 Kill-switch-off structural-unfailure test: OFF creates zero `v3/` entries.
- [x] 4.7 `scripts/deadcode-ratchet.sh --update` for `review_core*.go` exports.

## Phase 5 (S4): Amendment C + Switch-Off Equivalence

- [x] 5.1 RED: full 2×2 matrix table test — new{related|unrelated|absent} × legacy{present|absent}.
- [x] 5.2 RED (mandatory): new-lineage marker present, v3 record removed, legacy receipt present ⇒ **deny** (discovery-integrity corruption path, never falls through to legacy).
- [x] 5.3 Decision task: confirm discovery-integrity denial routes to existing `authority_corrupted` constant, or document why a new reason constant is required — no silent default.
- [x] 5.4 Implement `resolveGoverningAuthority(new, legacy)` in `runReviewFacadeValidate`, single shared path for all five gates.
- [x] 5.5 GREEN: 5.1, 5.2 pass.
- [x] 5.6 Switch-off byte-equivalence golden — post-apply: capture, diff against pre-change golden, byte-identical.
- [x] 5.7 Switch-off byte-equivalence golden — pre-commit: same.
- [x] 5.8 Switch-off byte-equivalence golden — pre-push: same.
- [x] 5.9 Switch-off byte-equivalence golden — pre-pr: same.
- [x] 5.10 Switch-off byte-equivalence golden — release: same.
- [x] 5.11 Rollback-safety test: switch OFF after in-flight new lineage exists — lineage stays readable and finalizable.
- [x] 5.12 `scripts/deadcode-ratchet.sh --update` for `resolveGoverningAuthority` and golden fixtures.

## Phase 6 (S5): Reason Taxonomy, Offer API, Bench

- [x] 6.1 RED: `TestNewLineageReasonTaxonomyCoversLegacyRefusals` — regression table, six `ReviewReceiptDiscoveryKind` × four `GateResult` closed vocabulary, table-driven, no default fallback. Implemented as `TestNewLineageReasonTaxonomyCoversLegacyRefusalsClosedMatrix` (the literal 6×4=24-cell closed matrix) + `TestNewLineageReasonTaxonomyCoversLegacyRefusals` (per-kind coverage against real production functions) in `internal/cli/review_reason_taxonomy_test.go`.
- [x] 6.2 GREEN: `newLineageGateEvaluation` (`internal/cli/review_governing_authority.go`) now has an explicit case per `CoreTransitionKind` value (continue→allow, collect→scope-changed, escalate→escalated [a documented strengthening vs. legacy's generic invalidated for `receipt_ambiguous`], approve/repair/stop→invalidated, each explicitly named rather than sharing S4's `default` branch). `receipt_missing`/`target_unresolvable` defer byte-identical to legacy (Amendment C's own "new absent" construction); `receipt_unrelated`/`authority_corrupted` reach the same `GateInvalidated` legacy also reaches.
- [x] 6.3 Decision task: **DECIDED** — admitted-finding path references belong in `review-state.json`, not the receipt (matches `AdmittedFindingIDs`'s existing S2 precedent). Recorded in `openspec/changes/rdd-root-simplification-wave3/design.md`'s Open Questions with full rationale; not yet wired (no S5 `validate` call site constructs `CoreValidateEvidence.AdmittedFindingPaths` from persisted authority — documented, not silent).
- [x] 6.4 Decision task: **DECIDED** — ship `OfferReviewAfterVerify` unwired with the `.deadcode-baseline.txt` entry, per design decision 8. Recorded in `openspec/changes/rdd-root-simplification-wave3/design.md`'s Open Questions with full rationale.
- [x] 6.5 Created `internal/reviewtransaction/review_offer.go`: `OfferReviewAfterVerify` unwired; kill-switch OFF returns `Offer{Available:false}, nil` before any repository read (proven in `review_offer_test.go` via a nonexistent repo path).
- [x] 6.6 Added two unwired-offer entries to `.deadcode-baseline.txt` (`OfferReviewAfterVerify`, `readGlobalRDDModeForOffer`) with reason documented in `review_offer.go`'s own doc comment. Also added a Wave 3 exit-evidence pointer to `docs/architecture/rdd-root-simplification-design.md`'s migration-waves section, naming the concrete test files proving each piece and the documented finalize-not-CLI-wired scope gap.
- [x] 6.7 Bench journeys: scoped to what is CLI-reachable this wave (documented deviation — `review finalize` is not yet CLI-wired for new lineages, so a full tier 0/1/4 receipt-issuance flow cannot run through the CLI regardless of bench harness capability). Added `bench/journeys_wave3.go` (j59, j60) exercising the new-lineage `start`→`validate` lifecycle across gates, plus a bench-harness extension (`Sandbox.NewLineageActivation`/`Journey.NewLineageActivation`, `bench/runner.go`) to opt a journey into `GENTLE_AI_RDD_NEW_LINEAGE`. Also widened `governingAuthorityLiveEvidence` (S4's documented scope cut) to use the STAGED projection at pre-commit (`internal/cli/review_governing_authority.go`), with regression coverage in `internal/cli/review_new_lineage_gate_selector_test.go`; discovered and fixed a latent policy-hash mismatch bug in the same function (never exercised until this slice's tests reached `ReviewCore.validate`'s exact-match path for the first time).
- [x] 6.8 `scripts/deadcode-ratchet.sh --update` — 234→236 entries (+2, exactly `OfferReviewAfterVerify`/`readGlobalRDDModeForOffer`); re-run confirms clean.
