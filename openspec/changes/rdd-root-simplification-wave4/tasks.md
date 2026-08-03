# Tasks: RDD Root Simplification — Wave 4 (Thin Consumers)

## Gate

HARD-GATED: chains on Wave 3's final slice (feature-branch-chain, tracker
`feature/rdd-root-simplification`). Wave 3 is not yet on `main`
(`review_offer.go`/`review_core.go`/`authority_store.go` absent at `d591f4cf`).
Do not open Wave 4 PR0 until Wave 3's last slice merges to the tracker branch.

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | S1 ~300, S2 ~350, S3 ~850, S4 ~650, S5 ~900, S6 ~500, S7 ~750 (total ~4300) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR0 → S1 → S2 → S3 → S4 → S5 → S6 → S7 |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | PR | Focused test | Harness | Rollback |
|---|---|---|---|---|---|
| PR0 | Land W4 SDD artifacts + Wave 3 archive move | tracker base | N/A (docs) | N/A — SDD artifacts only | Revert `openspec/changes/rdd-root-simplification-wave4/**` |
| S1 | Adapter behavioral-depth trace, inventory, forbidden-construction guard, pin fix | PR0-base | `go test ./internal/agents/... -run ForbiddenConstruction` | N/A — static grep/trace, no runtime scenario | Revert inventory edits + guard test; pin fix reverts independently |
| S2 | Decision-9 single work-unit owner (`RuntimeObjective`) | S1-base | `go test ./internal/sddstatus/... -run RuntimeObjective` | `t.TempDir()` CAS/replay integration | Revert `BeginAttemptRequest` field collapse |
| S3 | Offer call site FIRST + pre-verify routing removal + decline + AST/call-graph absence guard | S2-base | `go test ./internal/sddstatus/... ./internal/cli/... -run OfferAbsence` | OFF-mode bench apply→verify→archive | Revert offer call site; restore `applyPreVerify*Routing` |
| S4 | Transport capability admission (second) | S3-base | `go test ./internal/agents/capabilitymanifest/... -run ReviewTransport` | unsupported-transport bench denial journey | Revert `ContractReviewTransportV1` claim + admission check |
| S5 | `ReceiptRef` record + legacy read-only projection | S4-base | `go test ./internal/reviewtransaction/... -run ReceiptRef` | `t.TempDir()` golden byte comparison of `binding.json` | Revert `receipt_ref.go`/`legacy_binding_read.go`; ledger reverts to `BindingRevision` |
| S6 | Targeted re-verify three-branch scoping | S5-base | `go test ./internal/cli/... -run TargetedReverify` | staged/`commit -a`/empty-index/unborn-HEAD bench matrix | Revert re-verify branch logic |
| S7 | Mirror writer deletion + adapter/asset/golden updates (lands last) | S6-base | `go test ./internal/sddstatus/... -run BindingWriterRemoved` | 5-gate golden regeneration `-update` then verify | Restore `review_binding.go` writer half from git history |

## Phase 1 (PR0): SDD Artifacts

- [ ] 1.1 Land `openspec/changes/rdd-root-simplification-wave4/{proposal,specs,design,tasks}.md` (already written).
- [ ] 1.2 Archive Wave 3 (`openspec/changes/rdd-root-simplification-wave3/**` → `openspec/specs/`) when its turn comes, mirroring prior wave pattern.

## Phase 2 (S1): Adapter Trace + Guard + Pin Fix

- [ ] 2.1 Run the Wave-0 CON-09/10/11 trace per adapter: `codegraph_explore` blast radius, grep Go package + bundled asset tree for the four forbidden constructions (CLI flag literals, revision/`expected-revision`, target/lineage identity, binding JSON).
- [ ] 2.2 Record verdicts in `docs/architecture/rdd-ownership-inventory.md`: amend CON-09/10/11 evidence columns, delete the "behavioral depth" gap bullet at line 87.
- [ ] 2.3 RED: forbidden-construction guard test encoding the CON-09/10/11 grep set so the trace cannot silently rot.
- [ ] 2.4 GREEN: 2.3 passes against current adapters (`internal/agents/opencode/adapter.go`, `pi/adapter.go`, `claude/adapter.go` — all clean; violations live in the bundled assets, tracked by S3/S7).
- [ ] 2.5 Cross-slice fix (rides S1 per CI-exact-head rule): correct `gentle-ai.review-integration/v1` → `/v2` pin in `internal/assets/claude/commands/sdd-apply.md` and `internal/assets/opencode/commands/sdd-apply.md`; resolves design's open pin-placement question.
- [ ] 2.6 `scripts/deadcode-ratchet.sh --update` for inventory/guard changes.

## Phase 3 (S2): Decision-9 Single Owner

- [ ] 3.1 RED: work-unit-owner uniqueness test — `RuntimeObjective` is the sole scope owner; no parallel struct exists.
- [ ] 3.2 Collapse `CompactAcquireRequest`'s `WorkUnit`/`EvidenceGoal`/`MaxAttempts`/`MaxChangedLines` into `BeginAttemptRequest` fields (`runtime_ledger.go`); `runtime_compact.go` becomes projection-only through `normalizeBeginAttemptRequest`.
- [ ] 3.3 GREEN: 3.1 passes; advance-vs-reset distinction (#2133/#2151) defined once at the objective.
- [ ] 3.4 Rewrite the conditional CON-08 owner row at `docs/architecture/rdd-ownership-inventory.md:93` to name `RuntimeObjective` unconditionally (drop "only if" wording) per maintainer-confirmed decision 9.
- [ ] 3.5 `scripts/deadcode-ratchet.sh --update` for removed `CompactAcquireRequest` fields.

## Phase 4 (S3): Offer Call Site + Absence Guard (FIRST per spec sequencing)

- [ ] 4.1 RED: static AST/call-graph guard asserting zero call edges into offer/`ReviewCore` symbols from any SDD apply/verify/archive path — must fail while pre-verify routing still exists.
- [ ] 4.2 Create `internal/sddstatus/review_door.go` (`reviewEntryHook`) and `internal/cli/review_offer_door.go` (`offerEntryHook`, scoped to the verify-success-exit path only; the ~30 other non-test `internal/cli` files importing `reviewtransaction` for explicit `review` subcommands stay out of scope).
- [ ] 4.3 Remove `applyPreVerifyReviewRouting` (`status.go:829`), `applyPreVerifyCompactBridgeRouting` (`status.go:857`), and all four call sites (`:487`, `:490`, `:770`, `:772`).
- [ ] 4.4 Wire the offer call site in `internal/cli`'s SDD verify-success exit (after verify-report admission, before archive eligibility) — `sddstatus.Resolve` stays a pure read.
- [ ] 4.5 Implement decline = nothing recorded: unmanaged proceed, `reviewGate.delivery: disabled/unmanaged`, byte-identical to kill-switch-OFF.
- [ ] 4.6 GREEN: 4.1 passes; remove the `.deadcode-baseline.txt` entry for `review_offer.go` now that it is wired.
- [ ] 4.7 Corroborating: OFF-mode bench call-absence counter — `reviewEntryHook`/`offerEntryHook` fire zero times across apply→verify→archive.
- [ ] 4.8 Integration test: decline output byte-identical to OFF golden.
- [ ] 4.9 `scripts/deadcode-ratchet.sh --update` for door symbols and removed routing functions.

## Phase 5 (S4): Transport Capability Admission (second per spec sequencing)

- [ ] 5.1 RED: unsupported-transport denial before any authority/tier/lens/budget/collection state exists (store inspection asserting empty authority root).
- [ ] 5.2 RED (threat matrix — Process integration/adapters): absent claim, unrecognised claim, advertised-but-failing transport — three distinct table cases.
- [ ] 5.3 Add `ContractReviewTransportV1` to `ContractClaims` in `internal/agents/capabilitymanifest/manifest.go`, reusing `dormant|advertised` vocabulary, canonical registry, `Validate()`, digest.
- [ ] 5.4 Wire capability admission in `internal/cli/review_facade.go`'s `review start`, BEFORE risk/tier/lens/budget/consent/freeze.
- [ ] 5.5 Per-adapter unavailable mode: Pi adapter (declares only `AutoInstall|SystemPrompt|MCP`) enters unavailable, never a self-constructed transition.
- [ ] 5.6 GREEN: 5.1, 5.2 pass.
- [ ] 5.7 `scripts/deadcode-ratchet.sh --update` for the new claim.

## Phase 6 (S5): ReceiptRef Record

- [ ] 6.1 RED: `ReceiptRef{Lineage, ReceiptHash}` two-field shape test, rejects extra fields.
- [ ] 6.2 RED: legacy binding.json read-only compat test — parses to an in-memory `ReceiptRef`, never rewritten, never unlinked; golden byte comparison of the untouched file.
- [ ] 6.3 Create `internal/reviewtransaction/receipt_ref.go`: `ReceiptRef` + `ValidateReceiptRef(ctx, repo, ref) (GateResult, string, error)`.
- [ ] 6.4 Create `internal/sddstatus/legacy_binding_read.go`: `parseBinding` → `parseLegacyBinding`, read-only projection.
- [ ] 6.5 Modify `runtime_ledger.go`: `RuntimeStatus.Receipt *ReceiptRef`, new `runtimeReceiptEvent` (operation `receipt`), replacing `BindingRevision`/`Binding *ReviewBinding`.
- [ ] 6.6 Modify `review_gate.go`: `applyReviewGateEvaluation` keeps only the provider verdict; disabled/unmanaged branch becomes structurally unreachable when OFF.
- [ ] 6.7 Collapse `resolveReviewAuthority` + `resolveCompactRemediationAuthority` into the single `ValidateReceiptRef` call; `sddstatus` stores `result`/`reason` verbatim.
- [ ] 6.8 GREEN: 6.1, 6.2 pass.
- [ ] 6.9 RED (threat matrix — Git repository selection): relative `--cwd`, linked worktree, symlinked common dir, foreign-repo refusal.
- [ ] 6.10 GREEN: 6.9 via existing `SnapshotBuilder.ResolveRepositoryRoot`; OFF path returns before any repository read.
- [ ] 6.11 `scripts/deadcode-ratchet.sh --update` for `receipt_ref.go`/`legacy_binding_read.go` exports.

## Phase 7 (S6): Targeted Re-Verify (Three Branches)

- [ ] 7.1 RED: empty-intersection branch — correction changed paths ∩ verify evidence scope is empty ⇒ re-run the objective's evidence goal.
- [ ] 7.2 RED: unprovable/non-derivable scoping branch — changed-path set not reliably derivable ⇒ FULL re-verify of the objective's evidence goal (distinct from 7.1, never conflated).
- [ ] 7.3 RED: empty-index/unborn-HEAD branch ⇒ fail closed.
- [ ] 7.4 Implement the three-branch scoping logic; record a new `RuntimeAttempt` using the existing `RemediatesEvidenceRevision` field.
- [ ] 7.5 GREEN: 7.1–7.3 pass.
- [ ] 7.6 RED (threat matrix — Commit state, remaining cases): staged, `commit -a`.
- [ ] 7.7 GREEN: 7.6 passes.
- [ ] 7.8 `scripts/deadcode-ratchet.sh --update` for re-verify branch exports.

## Phase 8 (S7): Mirror Deletion + Asset Updates (lands LAST — only destructive step)

- [ ] 8.1 Delete writer half of `internal/sddstatus/review_binding.go`: `BindApprovedReview`, `prepareApproved*Binding`, `bindingBytes`/`bindingDigest`/`bindingPath`, `bindingExists`, `validateBoundReview`, `verifyBindingLedger`, `runtimeSelfSuccessorAvailable`, `RuntimeStrandedSuccessor`.
- [ ] 8.2 RED: `BindingWriterRemoved` guard confirming no writer symbol remains reachable; legacy read path (S5) still functions.
- [ ] 8.3 GREEN: 8.2 passes.
- [ ] 8.4 Thin `internal/assets/opencode/plugins/review-result-artifacts.ts`: remove its `ReviewBinding` type, `admissionRecoveryKey` composition, session-scoped recovery budget (`claimAdmissionRecovery`, `MAX_ADMISSION_RECOVERIES_PER_SESSION`); consume opaque transitions only.
- [ ] 8.5 Decision task: the relaunch-bound loss from 8.4 needs a provider-side replacement — record whether it lands in this wave or is deferred, do not resolve silently by omission.
- [ ] 8.6 Update `internal/assets/claude/commands/sdd-apply.md`, `opencode/commands/sdd-apply.md`, `skills/_shared/review-ledger-contract.md`: post-apply routing text → post-verify offer.
- [ ] 8.7 Regenerate `testdata/golden/*` (`-update` flag), verify byte-diff is only the expected routing/pin changes.
- [ ] 8.8 `scripts/deadcode-ratchet.sh --update` for deleted writer symbols and asset changes.
