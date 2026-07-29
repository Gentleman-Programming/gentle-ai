# Design: Restore Organic Post-Candidate RDD

## Approach and Authority

Selectively extract; never reset. The systemic audit owns history, boundaries, edge cases, security, kill switches, and cross-domain authority. Owner-approved deviations constrain SDD; silence blocks. Preserve touched invariants, not contexts. SDD governs this work unit, never shipped guidance.

```text
configured agent -> baseline organic guidance -> direct | delegated | optional SDD
candidate -> normalize/freeze -> proportional local review -> receipt -> flexible delivery
disable -> reject/freeze RDD mutation -> disabled/unmanaged delivery + read-only authority
```

## Decisions

- ACI owns `agentguidance.InjectRouting`; `installRuntime.stagePlan` (`run.go`) and `syncRuntime.stagePlan` (`sync.go`) schedule it for every agent outside `model.ComponentSDD`. `internal/components/agentguidance/` already exists (`inject.go`, `routing.go` plus tests, 18 green) and covers all three adapter delivery strategies: Jinja module (Kimi), always-loaded orchestrator prompt (OpenCode/Kilocode), markdown section (remaining 13). Read-only digest inspection (`status.go`) is still unwritten.
- `sdd.Inject` must still drop its trigger-rules step: `internal/components/sdd/triggerrules.go` still exists, still emits 3 WorkRun references, and is still called from `sdd.Inject`. That call plus the file are what remains; only then does `sdd.Inject` keep optional SDD assets alone.
- Global mode persists in uncommitted user state; clone-local off-only override uses Git-common-dir CAS. Any off wins; repositories cannot force on/share policy. `review mode enable|disable|status` and Doctor expose source/effective mode. Disabled freezes authority read-only, keeps tests/hooks/CI, and emits `disabled/unmanaged`; re-enable applies after the candidate cutoff.
- Review tier is post-freeze: passive/trivial has structural readback and zero reviewers; standard has one consolidated review; authorization/data-loss/mutation/release/security risk has focused 4R. Size alone never escalates. Ordinary review has one correction; explicit Judgment Day’s two-round budget is unchanged.
- Freeze binds repository/common-dir, base, bytes, paths, modes, policy, and subject; immutable admission, CAS, replay, and one receipt serve delivery gates.

## File Actions

| Action | Paths and responsibility |
|---|---|
| CREATE (partly landed) | LANDED: `internal/components/agentguidance/{inject.go,routing.go,inject_test.go,routing_test.go}`; baseline rendering, transactional sync, three adapter delivery strategies. REMAINING: `{status.go,status_test.go}` read-only digest inspection. |
| MODIFY | `internal/cli/{run.go,sync.go,doctor.go,run_component_paths_test.go,sync_test.go,doctor_test.go}`; `internal/doctor/{doctor.go,doctor_test.go}` adds `routing:sync-required`; `internal/components/sdd/{inject.go,boundedreview.go,inject_test.go,bounded_review_contract_test.go}` removes routing coupling. |
| DELETE | `internal/components/sdd/{triggerrules.go,triggerrules_test.go}` plus their `sdd.Inject` call site, after corrected regeneration. Both still exist on the branch and still emit 3 WorkRun references. |
| CREATE | `internal/agents/capabilitymanifest/{manifest.go,manifest_test.go}` (branch-new, absent from `origin/main`) truthful per-adapter facts/equal organic semantics; unknown agents reject. |
| MODIFY | `internal/state/{state.go,state_test.go}` global mode; `internal/reviewtransaction/risk.go` (the only one of these on the base; `risk_test.go` is not in the diff); `internal/cli/{review.go,review_facade.go,review_status_contract.go,review_next_transition.go}`; `internal/app/{app.go,app_test.go,help.go,help_test.go}`. RED: retired WorkRun commands no longer dispatch. |
| CREATE | `internal/reviewtransaction/{verification_contract.go,verification_contract_test.go,rar_plan_authority.go,rar_plan_authority_test.go}`; branch-new, absent from `origin/main`, so they are authored, not edited. |
| CREATE | `internal/reviewtransaction/{rdd_mode.go,rdd_mode_test.go,verification_consent.go,verification_consent_test.go}` and `internal/cli/{review_mode.go,review_mode_test.go,review_verification_decide.go,review_verification_decide_test.go}`. |
| MODIFY / KEEP | Remove WorkRun binding/reservation branches from `internal/sddstatus/{runtime_ledger.go,runtime_ledger_test.go}`; preserve attempt authority and `internal/cli/sdd_attempt.go`. DELETE `internal/sddstatus/{workrun_binding.go,workrun_binding_test.go}`. KEEP `internal/reviewtransaction/compact_reviewer_capture{,_test}.go`. |
| TRANSPLANT | The authoritative transplant set is the invariant ledger, not this row; 37 workprovider/workrun sources carry generic invariants. Each needs a named destination plus RED proof before its DELETE lands. Baseline consent sources: `internal/workrun/verification_consent.go`, `internal/workprovider/productive_verification_consent.go`, `internal/cli/work_verification_decide.go`. Highest-loss sources, called out explicitly: `productive_advance_recovery_test.go` (14 distinct crash and kill-switch failure classes); `workrun/store.go` (CAS revision guard, no-clobber publication with read-back, atomic HEAD replace); `pad_git_object_authority.go` (bounded git execution, scrubbed `GIT_*`/`LANG`/`LC_*` env, output caps, full-OID-only); `workrun/path_safety*.go` (directory-identity replacement detection across the lock→publish window — `rar_path_safety.go` checks per-open only and does NOT cover this); `productive_advance_path_{unix,windows}.go` (hardlink-count guard, no equivalent in any surviving package); `runtime_policy_provisioner.go` (write-ahead snapshot intent, all-or-nothing slot rotation); `pad_candidate_catalog.go` (immutable candidate identity, append-only slot, never rebindable). |
| TRANSPLANT then DELETE | `internal/deliveryadmission/` (23 files, branch-new): Windows use-handle token-owner rebind before DACL hardening; canonical private-directory ACE split; advisory-lock inode truth; read-only delivery replay; trusted-repository identity lease. Imported only by workprovider/workrun/evidence/e2e, so it dies with the control plane, but the Threat Matrix depends on these invariants. |
| TRANSPLANT then DELETE | `internal/evidence/` (18 files, branch-new): immutable append-only execution admission, replay-safe; owner semantic action tickets require execution proof; Windows evidence path safety. |
| TRANSPLANT then DELETE | `internal/hostruntime/` (10 files, branch-new): bounded host execution with process-tree kill on Unix and Windows; request→launch binding; toolchain identity. |
| TRANSPLANT then DELETE | `internal/mutationintegrity/` (5 files, branch-new): cross-platform directory fsync before a mutation counts as complete; structural completion evidence. |
| DELETE | After transplant: `internal/{workrun,workprovider,deliveryadmission,evidence,hostruntime,mutationintegrity}/`, `internal/cli/work_{advance,capabilities,reconcile,route,start,status,transition,verification_decide}.go`, `internal/cli/work_flags.go` (its `validateExactWorkFlags`/`encodeWorkJSON` helpers are consumed only by those eight files), `internal/cli/work_{advance,command,reconcile,route,runtime,status,transition,verification_decide}_test.go`, `internal/app/work_provider_test.go` (the only importer of `internal/workprovider` outside `internal/cli` and `e2e/`), and `contracts/work-routing/v1/`. Closure condition, stated exactly: after the six packages above join the set, the only remaining importer outside it is `e2e/organicruntime/organic_runtime_test.go`, which imports `deliveryadmission`, `evidence`, and `hostruntime` on top of workprovider/workrun. That file is a REWRITE, not a DELETE, so the set closes only once its rewrite has dropped those imports. |
| UNAUTHORIZED DELETION — RECORD | The branch already deleted these PUBLISHED paths, which this design authorizes nowhere: `internal/catalog/{triggers.go,triggers_test.go,bounded_review_triggers_test.go,release_event_test.go}`, `internal/testdata/golden/trigger-rules-default.golden`, and the `Trigger*` type removals in `internal/model/types.go`. They are published, so they cannot take the early-removal deviation. Each requires an explicit deletion-ledger entry before the work unit closes. |
| REWRITE | `e2e/organicruntime/organic_runtime_test.go`; replace TLS/runtime-fixture proof with real configured-agent journeys: direct implementation, delegated implementation, optional-SDD propose/decline/accept, tier-0 docs-only with zero reviewer ceremony, tier-1 one consolidated review, tier-2 focused 4R, one bounded correction, and flexible delivery routes. |
| REGENERATE | `internal/assets/{antigravity,claude,codex,cursor,gemini,generic,hermes,kimi,kiro,opencode,qwen,windsurf}/sdd-orchestrator.md` and published `docs/trigger-rules.md` under systemic order; modify `internal/assets/{assets_test.go,skills/_shared/review-ledger-contract.md}` and `internal/components/golden_test.go`. Embedded key stays `skills/_shared/review-ledger-contract.md`. Released assets had no WorkRun; explicit sync replaces managed blocks, preserves user content, and converges. ORDERING HAZARD: `internal/assets/assets_test.go` currently holds 13 assertions that REQUIRE the WorkRun ceremony (`gentle-ai.work-status/v1`, `gentle-ai.work-advance/v2`, `Capability stop rule`, …) and therefore FAILS when the ceremony is removed — the inverse of task 4.1. Rewrite that test BEFORE asset regeneration, never after. |
| CREATE | `testdata/golden/organic-routing-manifest.json`; golden fixture for the regenerated organic routing projection. |

## Verification Contract

Keep `VerificationApplicability` and `VerificationCost`. Add independent `VerificationMutationEffect = read_only|destructive` and `VerificationPermissionEffect = ordinary|permission_sensitive`; `RARPlanAuthority` binds all dimensions. N/A does no work; unknown records a gap; quick read-only auto-runs; long/very-long uses frozen-plan consent; sensitive effects require immediate authorization. RED-test every cross-product, replay, and pre-authorization effect.

## Traceability

CREATE `docs/audits/data/organic-rdd-recovery/{snapshot,change,invariant,contribution,test,deletion,release}-ledger.json` plus matching schemas. CREATE repository-only `internal/recoverytrace/{model.go,import.go,validate.go,generate.go,validate_test.go}` with `ValidateLedgers`; CI runs its generated-current and backlog-close tests—no shipped CLI. Import systemic Appendix B; require 241/92, 74 collisions, 499 overlaps, 16 decompositions, credit, and release-bound SHA.

Early deletion requires per-path `git cat-file -e` evidence for `origin/main`, local `main`, and latest tag. `internal/workprovider`/`internal/workrun` are absent from those refs and `internal/workprovider/runtime_connector.go` from recent v2 tags; they may deviate early only after transplant/no-invariant proof. 81 paths in the branch diff exist on `origin/main` (86 counting the 5 deleted published paths); all 81 are published, barred from the early-removal deviation, and retain systemic order.

## Downstream: Gentle Pi

`contracts/work-routing/` does not exist on `origin/main`; gentle-ai HEAD carries 37 files under `contracts/work-routing/v1/`. Pi vendors 15 of them: 4 byte-identical, 11 matching an earlier branch commit, 22 never vendored. Vendor drift does not soften the impact — deleting the eight `work-*` commands still breaks Pi's live consumers `lib/native-work-cli.ts`, `lib/work-routing-v1.ts`, `lib/work-routing-consumer.ts`, and `extensions/gentle-ai.ts`. Treat it as a breaking change and record it in the release ledger.

## Threat Matrix and RED Proof

| Boundary | Safe/failure behavior and RED test |
|---|---|
| Documentation-like paths | Content/mode classifies `requirements.txt`, `CMakeLists.txt`, executable MD/MDX, `README.sh`; ambiguity fails closed. |
| Git repository selection | Canonical relative/absolute/`git -C` common-dir identity; alias/escape/mismatch rejects. |
| Commit state | Exact staged/`commit -a`/empty-index projection; mutation invalidates. |
| Push state | Tracking/first-push/refspec binds exact SHA; ambiguity rejects. |
| PR commands | Structured `--head`; environment-prefix/composed shell rejects. |
| Invariant transplantation | `DELETE` fails without tested destination proof or explicit no-retained-invariant proof. |

The five boundary rows receive table-driven RED tests using `t.TempDir()`/fake remotes; invariant transplantation is proven instead by `ValidateLedgers` RED tests over ledger fixtures. Exact-SHA applies to every touched invariant; cross-OS only to platform-sensitive behavior and real-agent only to adapter-visible behavior.

## Migration and Rollback

Freeze ledgers → prove publication/deviations → RED/transplant → delete remote surfaces → implement proportional RDD/kill switch → regenerate → applicable unit/integration/cross-OS/real-agent/exact-SHA checks. Use append-only conventional commits and corrective commits or `git revert`; never force-push.

## Open Questions

None. Three owner decisions are closed:

- Kill switch: global plus uncommitted clone-local off-only override; status/Doctor observable; future-only re-enable.
- Authority: systemic wins cross-domain; silence blocks; SDD order requires owner-approved deviations.
- Early removal: per-path unpublished evidence permits deviation; published paths retain systemic order.
