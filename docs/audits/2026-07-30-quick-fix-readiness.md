# Quick-Fix Readiness: 10 Current Start Candidates

The corrected operational answer is: **10 issues are current start candidates, subject to one final live label and PR-collision check immediately before work begins.** A further 13 issues have technically bounded mechanisms but are blocked by workflow state, for **23 technically bounded issues** in total. The other 33 rows are either small but not technically ready, or not quick fixes.

## Executive Correction Summary

| Group | Count | Operational meaning |
|---|---:|---|
| `current-start-candidate` | **10** | Technically bounded and currently approved; no known active related PR collision. |
| `technically-bounded-workflow-blocked` | **13** | The mechanism is bounded, but approval, intake, or an active related PR blocks a contribution start. |
| `small-but-technically-blocked` | **20** | The work looks small, but a platform oracle, product decision, design prerequisite, localization, stale report, or active carrier prevents technical readiness. |
| `not-a-quick-fix` | **13** | The issue requires an architectural, contract, ownership, product, or decomposition track. |
| **Total** | **56** | **10 + 13 + 20 + 13 = 56**; groups are mutually exclusive and exhaustive for this handoff. |

The previous count of 25 technical quick fixes is corrected to 23 technically bounded issues: **10 + 13**. Issues [#1834](https://github.com/Gentleman-Programming/gentle-ai/issues/1834) and [#1839](https://github.com/Gentleman-Programming/gentle-ai/issues/1839) were removed from that technical set because both now have `status:needs-design` and explicit maintainer-required design prerequisites.

This is a point-in-time technical handoff, not implementation authorization, a priority ranking, closure evidence, or a merge/release decision. Table order is issue-number order, not priority.

## Quick Path

1. Select one `current-start-candidate` by ownership and available test environment.
2. Immediately run one final live check of its labels and related/open PRs; this document cannot clear a collision that appeared after the scan.
3. Use one issue and one worktree for the attempt, then add a failing regression first.
4. Keep the change inside the stated mechanism. Stop and reclassify if it exposes a shared contract, authority, identity, persistence, ownership, or runtime boundary.
5. Run the focused regression and owning-package tests, and leave issue closure to the normal evidence and authorization process.

## Readiness Definitions

| Term | Meaning |
|---|---|
| Technically bounded | Current evidence identifies a local mechanism and a plausible bounded correction without an unresolved design prerequisite. It says nothing about ownership, approval, or an active PR collision. |
| Contribution-ready | Technically bounded **and** currently approved/intake-complete, free of an active related-work collision, and ready for a failing regression in a suitable environment. |
| `current-start-candidate` | Contribution-ready at the live scan, but still requires the final check in the quick path. |
| Regression status | `executed and reproduced` means the stated regression was run and failed as expected; `earlier execution reported; not independently replayed` means an earlier local batch reported the result, but the v1 independent verification executed no tests, did not replay it, and has no repository-replayable execution receipt attached; `existing focused suite passed but exact regression missing` means related tests passed but the issue-specific test still must be added; `proposed/not executed` means the oracle is a planned test, not execution evidence. |

## Current Start Candidates

All rows below were `approved` in the live scan. [#1937](https://github.com/Gentleman-Programming/gentle-ai/issues/1937) was also `up-for-grabs`. “No active related PR found” is limited to the read-only scan; it is not proof that no semantically related work exists.

| Issue | Bounded mechanism and likely path | Evidence observed | Regression status | Current GitHub status | Collision/design prerequisite |
|---|---|---|---|---|---|
| [#1644](https://github.com/Gentleman-Programming/gentle-ai/issues/1644) | Local-design-flaw: attribute timeout to initialize versus `tools/list` in `internal/components/communitytool/pi_codegraph.go`. | An earlier local batch reported that the focused timeout regression reproduced the defect 10/10. The v1 independent verification executed no tests and did not replay it; no repository-replayable execution receipt is attached. | `earlier execution reported; not independently replayed`: write and run the failing regression before implementation. | Open; `approved` | No active related PR found; no design prerequisite recorded. |
| [#1664](https://github.com/Gentleman-Programming/gentle-ai/issues/1664) | Micro-bug: canonicalize equivalent `PATH` directories in `internal/cli/doctor.go`. | Equivalent directory paths need symlink-aware canonical comparison. | `proposed/not executed`: add a symlink-equivalent path regression. | Open; `approved` | No active related PR found. [PR #1709](https://github.com/Gentleman-Programming/gentle-ai/pull/1709) closes #1690 and changes Gemini/Antigravity identity; it is explicitly unrelated. |
| [#1666](https://github.com/Gentleman-Programming/gentle-ai/issues/1666) | Micro-bug: type-check an invalid `FINALIZE` flag during preflight, before authority handling in the review facade/operation contract. | Invalid flag reaches the wrong phase without typed preflight. | `proposed/not executed`: add the exact invalid-flag regression first. | Open; `approved` | No active related PR found; no design prerequisite recorded. |
| [#1732](https://github.com/Gentleman-Programming/gentle-ai/issues/1732) | Local-design-flaw: include content bytes in the skill-registry fingerprint. | Current fingerprint evidence identifies metadata-only invalidation. | `proposed/not executed`: same-size, restored-mtime, different-content fixture. | Open; `approved` | No active related PR found; no design prerequisite recorded. |
| [#1862](https://github.com/Gentleman-Programming/gentle-ai/issues/1862) | Local-design-flaw: canonicalize nested `--cwd` for selector-free status. | Root, nested path, and safe symlink inputs need one canonical status target. | `proposed/not executed`: root/nested/safe-symlink equivalence test. | Open; `approved` | No active related PR found; no design prerequisite recorded. |
| [#1883](https://github.com/Gentleman-Programming/gentle-ai/issues/1883) | Micro-bug: fail an explicit benchmark selector with zero matches in `bench/main.go`. | Current runner does not separately reject an empty explicit selection; the focused suite passed. | `existing focused suite passed but exact regression missing` | Open; `approved` | No active related PR found; no design prerequisite recorded. |
| [#1885](https://github.com/Gentleman-Programming/gentle-ai/issues/1885) | Local-design-flaw: derive Go preflight from the selected beta Engram strategy before mutation. | Strategy selection and Go preflight are ordered incorrectly for beta installation. | `proposed/not executed`: stable/beta by selected/not-selected-by-Go matrix. | Open; `approved` | No active related PR found; no design prerequisite recorded. |
| [#1903](https://github.com/Gentleman-Programming/gentle-ai/issues/1903) | Local-design-flaw: resolve the Engram project before memory search in `internal/assets/engram/protocol.md`. | Protocol ordering can search before a unique project is resolved. | `proposed/not executed`: unique, ambiguous, and no-project ordering tests. | Open; `approved` | No active related PR found; no design prerequisite recorded. |
| [#1937](https://github.com/Gentleman-Programming/gentle-ai/issues/1937) | Local-design-flaw: handle `--help`/`-h` before flags or store access in the SDD attempt/verify CLI. | Focused tests passed for the help path. | `existing focused suite passed but exact regression missing` | Open; `approved`; `up-for-grabs` | No active related PR found; no design prerequisite recorded. |
| [#1948](https://github.com/Gentleman-Programming/gentle-ai/issues/1948) | Micro-bug/test gap: assert that unstaged tracked bytes are excluded in the staged-recovery fixture. | Existing staged-recovery fixture has the required seam but does not assert this negative case. | `proposed/not executed`: extend the existing fixture. | Open; `approved` | No active related PR found; no design prerequisite recorded. |

## Technically Bounded, Workflow Blocked

These 13 rows have a bounded mechanism but are **not contribution-ready**. The listed PR is a collision prompt, not proof that it implements the stated mechanism.

| Issue | Bounded mechanism | Evidence observed | Regression status | Current GitHub status | Collision or workflow prerequisite |
|---|---|---|---|---|---|
| [#535](https://github.com/Gentleman-Programming/gentle-ai/issues/535) | Reject dash-prefixed upgrade arguments before side effects in `internal/app/app.go`. | Local argument-validation path is identified. | `proposed/not executed`: app unit regression. | Open; `approved` | [PR #1929](https://github.com/Gentleman-Programming/gentle-ai/pull/1929) is open; resolve collision before contribution. |
| [#721](https://github.com/Gentleman-Programming/gentle-ai/issues/721) | Move Executor Override before the imperative gate in `internal/assets/skills/sdd-apply/SKILL.md`. | Asset ordering mechanism is identified. | `proposed/not executed`: asset-ordering test. | Open; `approved` | [PR #1105](https://github.com/Gentleman-Programming/gentle-ai/pull/1105) is open; resolve collision before contribution. |
| [#848](https://github.com/Gentleman-Programming/gentle-ai/issues/848) | Parse TODO/PENDING contextually rather than treating compliant prose as pending in `internal/sddstatus/status.go`. | Parser behavior and compliant-prose distinction are identified. | `proposed/not executed`: compliant-prose versus explicit-pending tests. | Open; `approved` | [PR #979](https://github.com/Gentleman-Programming/gentle-ai/pull/979) is open; resolve collision before contribution. |
| [#1273](https://github.com/Gentleman-Programming/gentle-ai/issues/1273) | Persist backward-compatible last-sync metadata after complete sync in `internal/state/state.go` and `internal/cli/sync.go`. | State transition and preservation target are identified. | `proposed/not executed`: preserve-complete-state regression. | Open; `approved` | [PR #1978](https://github.com/Gentleman-Programming/gentle-ai/pull/1978) is open; resolve collision before contribution. |
| [#1485](https://github.com/Gentleman-Programming/gentle-ai/issues/1485) | Deploy omitted `review-ledger-contract.md` from `internal/components/sdd/inject.go`. | Shared-file inventory omits the contract asset. | `proposed/not executed`: embedded shared-file deployment test. | Open; `approved` | [PR #1805](https://github.com/Gentleman-Programming/gentle-ai/pull/1805) and [PR #1950](https://github.com/Gentleman-Programming/gentle-ai/pull/1950) are open; resolve both collisions. |
| [#1672](https://github.com/Gentleman-Programming/gentle-ai/issues/1672) | Preserve large JSON integers with `UseNumber` in `internal/components/filemerge/json_merge.go`. | JSON number-loss mechanism is localized. | `proposed/not executed`: exact `9007199254740993` regression. | Open; `approved` | [PR #1752](https://github.com/Gentleman-Programming/gentle-ai/pull/1752) is open; resolve collision before contribution. |
| [#1675](https://github.com/Gentleman-Programming/gentle-ai/issues/1675) | Collapse duplicate valid managed-persona sections in `internal/components/filemerge/section.go`. | Duplicate-section convergence path is identified. | `proposed/not executed`: two-pair convergence test. | Open; `approved` | [PR #1764](https://github.com/Gentleman-Programming/gentle-ai/pull/1764) is open; resolve collision before contribution. |
| [#1676](https://github.com/Gentleman-Programming/gentle-ai/issues/1676) | Mark and restore post-rename ambiguous write failures in `internal/components/mutationjournal/journal.go`. | Failure window and before-image restoration seam are identified. | `proposed/not executed`: injected post-rename failure regression. | Open; `approved` | [PR #1756](https://github.com/Gentleman-Programming/gentle-ai/pull/1756) is open; resolve collision before contribution. |
| [#1678](https://github.com/Gentleman-Programming/gentle-ai/issues/1678) | Compensate the source write when TUI plugin registration fails in `internal/components/opencodeplugin/plugin.go`. | Rollback seam after malformed `tui.json` is identified. | `proposed/not executed`: malformed-`tui.json` rollback test. | Open; `approved` | [PR #1753](https://github.com/Gentleman-Programming/gentle-ai/pull/1753) is open; resolve collision before contribution. |
| [#1770](https://github.com/Gentleman-Programming/gentle-ai/issues/1770) | Ignore HTML comments while extracting linked issues from the PR-body parser. | Raw `body.matchAll(pattern)` parsing remains current and does not strip HTML comments. | `proposed/not executed`: visible versus commented-reference regression. | Open; `status:needs-info` | [PR #1791](https://github.com/Gentleman-Programming/gentle-ai/pull/1791) is open; complete intake/current reproduction and resolve collision first. |
| [#1806](https://github.com/Gentleman-Programming/gentle-ai/issues/1806) | Derive shared assets from one embedded inventory rather than divergent lists. | Current inventories remain divergent. | `proposed/not executed`: inventory-parity tests. | Open; `status:needs-info` | [PR #1950](https://github.com/Gentleman-Programming/gentle-ai/pull/1950) is open; complete intake/current reproduction and resolve collision first. |
| [#1998](https://github.com/Gentleman-Programming/gentle-ai/issues/1998) | Surface `Sync`/`Close` failures and clean up a partial Engram download in `internal/components/engram/download.go`. | Bounded downloader error and cleanup seam is identified. | `proposed/not executed`: fault-seam regression. | Open; `status:needs-review` | Approval is required before a contribution start; no active related PR found in the scan. |
| [#2000](https://github.com/Gentleman-Programming/gentle-ai/issues/2000) | Make admission-rejection guidance neutral unless severe findings exist. | Guidance/recovery behavior is localized to the review plugin. | `proposed/not executed`: guidance and recovery tests. | Open; `approved` | [PR #2002](https://github.com/Gentleman-Programming/gentle-ai/pull/2002) is open; resolve collision before contribution. |

## Small but Technically Blocked

These 20 rows remain small in apparent mechanism but are not in either technically bounded group. The regression column records the state of the stated re-entry oracle, not a promise that satisfying it will be sufficient.

| Issue | Current GitHub status | Blocker and evidence observed | Regression status | Collision/design prerequisite |
|---|---|---|---|---|
| [#268](https://github.com/Gentleman-Programming/gentle-ai/issues/268) | Open; blocked | Active carrier [PR #1219](https://github.com/Gentleman-Programming/gentle-ai/pull/1219) and no current Windows/WSL no-rescan oracle. | `proposed/not executed`: Windows/WSL no-rescan oracle. | Resolve carrier disposition. |
| [#428](https://github.com/Gentleman-Programming/gentle-ai/issues/428) | Open; blocked | Archive Bash mismatch is small, but [PR #1488](https://github.com/Gentleman-Programming/gentle-ai/pull/1488) leaves destination-collision semantics unresolved. | `proposed/not executed`: acceptance test after semantics decision. | Explicit collision semantics required. |
| [#727](https://github.com/Gentleman-Programming/gentle-ai/issues/727) | Open; blocked | Homebrew cask fix lacks a Homebrew-capable runner or portable fixture. | `proposed/not executed`: native or portable Homebrew oracle. | Platform prerequisite. |
| [#735](https://github.com/Gentleman-Programming/gentle-ai/issues/735) | Open; blocked | Missing Conductor adapter [#730](https://github.com/Gentleman-Programming/gentle-ai/issues/730). | `proposed/not executed`: adapter acceptance oracle. | Adapter availability prerequisite. |
| [#896](https://github.com/Gentleman-Programming/gentle-ai/issues/896) | Open; blocked | Needs a real Claude theme-resolution oracle; [PR #1630](https://github.com/Gentleman-Programming/gentle-ai/pull/1630) is open. | `proposed/not executed`: native theme-resolution proof. | Resolve carrier collision. |
| [#910](https://github.com/Gentleman-Programming/gentle-ai/issues/910) | Open; blocked | No Windows/pwsh execution seam or oracle. | `proposed/not executed`: Windows/pwsh executable test seam. | Platform prerequisite. |
| [#968](https://github.com/Gentleman-Programming/gentle-ai/issues/968) | Open; blocked | Historical correction is partial; remaining proxy/docs semantics lack an oracle. | `proposed/not executed`: executable acceptance oracle. | No cleared prerequisite. |
| [#1374](https://github.com/Gentleman-Programming/gentle-ai/issues/1374) | Open; blocked | Dynamic discovery versus generic consumer template is undecided. | `proposed/not executed`: acceptance test after decision. | Product decision required. |
| [#1726](https://github.com/Gentleman-Programming/gentle-ai/issues/1726) | Open; blocked | Nested TOML behavior is already corrected while the issue remains open. | `proposed/not executed`: live reproduction of a remaining defect. | Re-scope or confirm a remaining defect. |
| [#1728](https://github.com/Gentleman-Programming/gentle-ai/issues/1728) | Open; blocked | Unix/WSL atomic-copy oracle is unavailable; [PR #1748](https://github.com/Gentleman-Programming/gentle-ai/pull/1748) is open. | `proposed/not executed`: native Unix/WSL atomic-copy proof. | Resolve carrier collision. |
| [#1792](https://github.com/Gentleman-Programming/gentle-ai/issues/1792) | Open; blocked | HEAD already emits `fresh_target_ready`/`review.start`. | `proposed/not executed`: live reproduction of an unmet remainder. | Re-scope or confirm a remaining defect. |
| [#1833](https://github.com/Gentleman-Programming/gentle-ai/issues/1833) | Open; blocked | Timeout is confirmed, but the costly `pre_native` suboperation is not localized. | `proposed/not executed`: targeted deadline oracle after localization. | Localized cause prerequisite. |
| [#1834](https://github.com/Gentleman-Programming/gentle-ai/issues/1834) | Open; `status:needs-design` | Current mechanism confirms empty and oversized evidence collapse to one cause, but a maintainer requires trust-model, ownership, and lifecycle design first. | `proposed/not executed`: typed-error tests only after design. | Explicit maintainer design prerequisite. |
| [#1839](https://github.com/Gentleman-Programming/gentle-ai/issues/1839) | Open; `status:needs-design` | Current mechanism confirms unsupported gates reach repository/receipt discovery, but a maintainer requires terminal-output, error-contract, and persistence design first. | `proposed/not executed`: unsupported-gate table only after design. | Explicit maintainer design prerequisite. |
| [#1841](https://github.com/Gentleman-Programming/gentle-ai/issues/1841) | Open; blocked | Workspace Codex path needs a native Codex load oracle and ownership decision. | `proposed/not executed`: native load proof. | Ownership decision required. |
| [#1902](https://github.com/Gentleman-Programming/gentle-ai/issues/1902) | Open; blocked | Product decision remains unresolved: omit optional unselected plugins or mark them optional. | `proposed/not executed`: acceptance matrix after decision. | Product decision required. |
| [#1906](https://github.com/Gentleman-Programming/gentle-ai/issues/1906) | Open; blocked | The fix already exists in open [PR #1981](https://github.com/Gentleman-Programming/gentle-ai/pull/1981). | `proposed/not executed`: do not duplicate the carrier's regression. | Carrier disposition required. |
| [#1944](https://github.com/Gentleman-Programming/gentle-ai/issues/1944) | Open; blocked | SUSE/SLES platform/package validation is missing; [PR #1946](https://github.com/Gentleman-Programming/gentle-ai/pull/1946) is open. | `proposed/not executed`: native platform/package validation. | Platform and carrier prerequisites. |
| [#1955](https://github.com/Gentleman-Programming/gentle-ai/issues/1955) | Open; blocked | Source release-matrix work cannot repair published v2.2.0 and needs external Windows artifact verification. | `proposed/not executed`: external artifact verification. | Separately scoped release remedy required. |
| [#2001](https://github.com/Gentleman-Programming/gentle-ai/issues/2001) | Open; blocked | Policy is unresolved: whether every webhook path is security-high. | `proposed/not executed`: acceptance coverage after policy. | Explicit policy decision required. |

## Not a Quick Fix

These 13 rows are complete for this handoff. Their current GitHub state does not make them candidates for the quick-fix workflow.

| Issue | Current GitHub status | Final depth | Reason |
|---|---|---|---|
| [#298](https://github.com/Gentleman-Programming/gentle-ai/issues/298) | Open; owning-track work | Architectural-boundary-failure | Safe Windows ACL rollback, ownership, and crash recovery cross a boundary. |
| [#445](https://github.com/Gentleman-Programming/gentle-ai/issues/445) | Open; owning-track work | Systemic-contract-failure | Missing no-cleanup scope spans enum, UI, and executor. |
| [#533](https://github.com/Gentleman-Programming/gentle-ai/issues/533) | Open; owning-track work | Systemic-contract-failure | Producer/consumer progress contract and visual oracle are shared. |
| [#782](https://github.com/Gentleman-Programming/gentle-ai/issues/782) | Open; owning-track work | Local-design-flaw | Adapter/install/registry compatibility is too broad for a bounded quick fix. |
| [#828](https://github.com/Gentleman-Programming/gentle-ai/issues/828) | Open; owning-track work | Systemic-contract-failure | Gentle AI catalog identity conflicts with external Engram vocabulary. |
| [#984](https://github.com/Gentleman-Programming/gentle-ai/issues/984) | Open; owning-track work | Product-design-gap | Latest/pin/force CodeGraph policy remains undecided. |
| [#1015](https://github.com/Gentleman-Programming/gentle-ai/issues/1015) | Open; owning-track work | Systemic-contract-failure | OpenCode effective data/runtime source contract is shared. |
| [#1074](https://github.com/Gentleman-Programming/gentle-ai/issues/1074) | Open; owning-track work | Architectural-boundary-failure | Workspace/global ownership crosses backup, plugins, and injections. |
| [#1546](https://github.com/Gentleman-Programming/gentle-ai/issues/1546) | Open; owning-track work | Architectural-boundary-failure | Reopens ownership and migration of user Codex configuration. |
| [#1562](https://github.com/Gentleman-Programming/gentle-ai/issues/1562) | Open; owning-track work | Product-design-gap | Source format, precedence, cache, and migration semantics are undecided. |
| [#1648](https://github.com/Gentleman-Programming/gentle-ai/issues/1648) | Open; owning-track work | Systemic-contract-failure | Plugin/cache/Go-provider alias contract is shared. |
| [#1983](https://github.com/Gentleman-Programming/gentle-ai/issues/1983) | Open; owning-track work | Local-design-flaw aggregate | Heterogeneous Windows failures must be split before bounded work is possible. |
| [#1996](https://github.com/Gentleman-Programming/gentle-ai/issues/1996) | Open; owning-track work | Systemic-contract-failure | Correction, failed-evidence, and terminal-review lifecycle is shared. |

## Revision and Independent Verification

The independent [verification report](2026-07-30-quick-fix-readiness-verification.md) audited the prior untracked source bytes with SHA-256 `9f02a3281b9a07f25d7e5d1c988b1b885defe14bc313e6e1791ed243ff95da8c`. Its line references and hash identify **v1**, not this corrected revision.

This revision incorporates that report's material findings and the current live corrections: #1770 and #1806 remain technically substantiated but are `status:needs-info`; #1834 and #1839 require design; #1998 is `status:needs-review`; #1664 is approved and has no #1709 collision; and #1937 is approved and `up-for-grabs`.

## GitHub Projects: Optional Projection

GitHub Projects can provide a filtered board with fields such as `Readiness`, `Causal depth`, `Blocker`, `Evidence observed`, `Regression status`, and `Work state`. It is optional and non-authoritative: it should project this document and the live issue record, not replace either as a source of truth.

## Validation, Provenance, and Limitations

| Check | Result |
|---|---|
| Partition accounting | `10 + 13 + 20 + 13 = 56`; the groups are mutually exclusive. |
| Technically bounded accounting | `10 + 13 = 23`; #1834 and #1839 are excluded. |
| Live-state interpretation | Statuses and PR relations are read-only observations from the 2026-07-30 UTC scan and can drift. |
| Evidence language | A proposed oracle is not described as executed. #1644 has an earlier local-batch 10/10 reproduction report, but the v1 independent verification did not replay it and no repository-replayable execution receipt is attached; write and run the failing regression before implementation. #1883 and #1937 record focused-suite evidence while retaining an exact-regression gap. |
| Taxonomy background | The 321-issue taxonomy is local analytical background, not a repository-replayable receipt. |
| Engram provenance | Engram observations are local analytical background, not a repository-replayable receipt or immutable snapshot. |

Historical documents are context only, not current authority:

- [Post-ratchet audit](2026-07-27-post-ratchet-audit.md)
- [Systemic remediation architecture](2026-07-23-systemic-remediation-architecture.md)
- [v2.2.0 closure ledger](../releases/v2.2.0-closure-ledger.md)

The document intentionally makes no immutable historical-worktree, runtime, receipt-driven-development, or review-transaction claim. It records the local analytical conclusions and live observations available for this handoff; contributors must perform the final live check and establish their own implementation evidence.
