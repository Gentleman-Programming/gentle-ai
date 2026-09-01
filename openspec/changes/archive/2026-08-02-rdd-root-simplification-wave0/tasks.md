# Tasks: RDD Root Simplification — Wave 0 (Freeze Expansion)

Organized by the design's four-slice PR chain (design.md:155-166). Docs-only; no `internal/**` or `contracts/**` change in any slice.

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines (total, 4 PRs) | ~1,570 (PR0 ~450 + PR1 ~570 + PR2 ~250 + PR3 ~300) |
| Effective per-PR review budget | 1000 changed lines/PR — session param `review_budget_lines: 1000`, sourced from design.md:157 "PR slicing preview (input to sdd-tasks)" itself sourced from proposal.md deliverable #5 (chain plan); **not** an `openspec/config.yaml` rule |
| 1000-line budget risk per slice | PR0 Low (~45%) · PR1 Low-Medium (~57%, largest slice, sole normative artifact) · PR2 Low (~25%) · PR3 Low (~30%) |
| Chained PRs recommended | Yes (already selected) |
| Suggested split | PR 0 → PR 1 → PR 2 → PR 3 (feature-branch-chain) |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain — tracker `feature/rdd-root-simplification` off `main@ece470dacd0041f394e7f6f3877a6a9fcb3482af`; PR#1 targets tracker, PR#n targets PR#(n-1) branch; only tracker merges to main |

```text
Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High (generic skill default; superseded here by the session's explicit 1000-line/PR budget above, under which all four slices are Low-Medium)
```

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Land SDD artifacts (proposal, specs, design, tasks) | PR 0, base=tracker | `git diff --name-only main...HEAD \| grep -v '^openspec/'` → empty | N/A — docs-only artifact set, no runtime surface (design Threat Matrix: N/A) | Revert `openspec/changes/rdd-root-simplification-wave0/**` on tracker branch |
| 2 | Amended design + chain-plan section + amendment log | PR 1, base=PR0 branch | RED/GREEN rg recipe from task 2.1/2.13 + `sha256sum docs/audits/2026-07-21-rdd-system-audit.md` | N/A — no code path exercised | Delete `docs/architecture/rdd-root-simplification-design.md` |
| 3 | Ownership inventory | PR 2, base=PR1 branch | RED/GREEN rg set-difference recipe from task 3.1/3.13 | N/A — CodeGraph read-only tracing only, no mutation | Delete `docs/architecture/rdd-ownership-inventory.md` |
| 4 | Freeze policy + backlog disposition | PR 3, base=PR2 branch | RED/GREEN rg recipe from task 4.1/4.9 | N/A — no CI enforcement introduced | Delete both Phase 4 files |

## Phase 1: PR 0 — SDD Artifacts (base: tracker `feature/rdd-root-simplification`)

- [x] 1.1 Confirm `proposal.md`, `specs/{rdd-simplification-design,rdd-ownership-inventory,rdd-freeze-expansion-policy,rdd-backlog-disposition}/spec.md`, and `design.md` are present and unmodified under `openspec/changes/rdd-root-simplification-wave0/`. Req: all four spec domains.
- [x] 1.2 Commit this `tasks.md`. Req: Migration Chain Plan Documented, Change Scope Boundary.
- [x] 1.3 Create tracker branch `feature/rdd-root-simplification` off `main@ece470dacd0041f394e7f6f3877a6a9fcb3482af`; open it as a draft/no-merge tracker PR to `main`.
- [x] 1.4 Verify diff scope: every changed path is under `openspec/`. Req: Change Scope Boundary.

## Phase 2: PR 1 — Amended Design (base: PR 0 branch)

- [x] 2.1 **[RED]** Author the rg-based structural assertion recipe before any content: presence of Amendment sections A–E, the closing `Amendment log` table, in-repo evidence paths (`docs/audits/2026-07-21-rdd-system-audit.md`, `internal/reviewtransaction/prepr.go:73`), and a recorded sha256 line. Run it against the current empty `docs/architecture/` state and confirm every assertion fails. Do not commit the recipe; record commands + failing output ad hoc for `verify-report.md`. Req: Design Document Source and Evidence Paths, Amendments A–E.
- [x] 2.2 Locate the sibling-worktree source design (proposal.md:53: "Copy the design from the sibling worktree"; exact path not recorded in-repo — resolve at apply time) and copy its ~515 lines unchanged in structure/register into `docs/architecture/rdd-root-simplification-design.md`. Req: Design Document Source and Evidence Paths.
- [x] 2.3 State proposal decisions 1–5 as adopted verbatim, no re-derivation. Req: Adopted Next-Step Decisions Preserved Verbatim.
- [x] 2.4 Apply Amendment A: `compatible_base_advance` row + new subsection citing `internal/reviewtransaction/prepr.go` `deriveBaseAdvanceCompatibility` (line 73, all seven conditions) + `Adversarial safety analysis` "Incompatible base advance" row + `Evidence and scope` proof-source row. Req: Amendment A.
- [x] 2.5 Apply Amendment B: `provable_contraction` row + same subsection + new "Unsound contraction" row + one `Acceptance criteria` bullet (degrade to `changed` when admitted findings touch an excluded path). Req: Amendment B / Decision 6.
- [x] 2.6 Apply Amendment C: `Migration waves` Wave 3 `Rollback boundary` cell + precedence note under the wave table + new "Coexistence precedence" row (legacy readable authority never authorizes delivery of a new-lineage candidate). Req: Amendment C / Decision 7.
- [x] 2.7 Apply Amendment D: two new `Unresolved maintainer and product decisions` rows — decision 8 (external evidence retention horizon) and decision 9 (SDD attempt-ledger ownership). Req: Amendment D / Decisions 8–9.
- [x] 2.8 Apply Amendment E: new "Cross-lineage receipt contamination" row in `Adversarial safety analysis` + add `#1379` to the existing `Receipt-only delivery validation` row in `Issue and PR coverage map` (one row only, no duplicate). Req: Amendment E.
- [x] 2.9 Correct the `Evidence and scope` "Prior system audit" row path from the external `/home/gentleman/work/rdd-system-audit.md` to `docs/audits/2026-07-21-rdd-system-audit.md`. Req: Design Document Source and Evidence Paths.
- [x] 2.10 **[sha256]** Run `sha256sum docs/audits/2026-07-21-rdd-system-audit.md`; record the measured digest in the evidence row. If it differs from `4b41d15a…`, add a discrepancy note stating only the measured value — never assert the unverified expected value. Req: Design Document Source and Evidence Paths.
- [x] 2.11 Add the `Migration chain and delivery plan` section (after `Migration waves`, before `Issue and PR coverage map`): tracker `feature/rdd-root-simplification` off `main@ece470dacd0041f394e7f6f3877a6a9fcb3482af`; PR#1 targets tracker, PR#n targets PR#(n-1) branch; only tracker merges to `main`; ~1000 changed-line budget/PR (cite proposal.md deliverable #5, not `openspec/config.yaml`); retarget/rebase if a child diff shows a prior slice. Req: Migration Chain Plan Documented.
- [x] 2.12 Add the closing `Amendment log` table mapping IDs A–E to touched sections and edit shape (design.md:25-33).
- [x] 2.13 **[GREEN]** Re-run the 2.1 recipe; confirm every assertion now passes; record pass output for `verify-report.md`.
- [x] 2.14 Verify diff scope: only `docs/architecture/rdd-root-simplification-design.md` changed in this PR.

## Phase 3: PR 2 — Ownership Inventory (base: PR 1 branch)

- [x] 3.1 **[RED]** Author the rg-based set-difference completeness recipe before content (each enumeration point below vs. inventory rows); confirm it fails against the current nonexistent `docs/architecture/rdd-ownership-inventory.md`. Record ad hoc, not committed. Req: Inventory Row Completeness and Single Ownership.
- [x] 3.2 Enumerate lifecycle transitions (`TRN-nn`) from `internal/reviewtransaction/transaction.go` state vocabulary + `internal/cli/review_next_transition.go` `next_transition` producer via CodeGraph read-only tracing.
- [x] 3.3 Enumerate public operation forms (`TRN-nn`) from `internal/app/app.go` dispatch (`review`, `review-resume`, `review-step`, bundle/validate legacy forms), cross-checked against `internal/app/help_test.go`. Record the confirmed discrepancy as inventory evidence: `internal/app/app.go:111` dispatches `case "review-step"`, but `internal/app/help_test.go:14`'s `commands` list omits `review-step`. Do not modify `help_test.go` — fixing it is out of docs-only scope.
- [x] 3.4 Enumerate persisted artifacts (`ART-nn`) from `internal/reviewtransaction` store writers (authority record, receipt, journal, bundle, sidecar, quarantine residue).
- [x] 3.5 Enumerate contract surfaces (`CTR-nn`) from `contracts/review-integration/v1/**` and `v2/**` schema+fixtures, cross-checked against schema-ID constants in `internal/cli/review_*.go`.
- [x] 3.6 Enumerate delivery gates (`CON-nn`) from the `GateKind` constants at `internal/reviewtransaction/receipt.go:133-137` (`GatePostApply`, `GatePreCommit`, `GatePrePush`, `GatePrePR`, `GateRelease`) and `reviewIntegrationGatesInOrder` at `internal/cli/review_operation_contract.go:1440-1444`.
- [x] 3.7 Enumerate consumers (`CON-nn`) from `internal/sddstatus/review_gate.go`, `internal/sddstatus/review_binding.go`, adapter/plugin assets, CI hook definitions; record out-of-repo consumers (OpenCode, Pi, Claude plugin assets) as `CON-nn` rows with `evidence: out-of-repo`, flagged in `Inventory gaps`.
- [x] 3.8 Assign observed `current owner(s)` and exactly one `target owner` (closed set: `ReviewCore`/`AuthorityStore`/`CandidateResolver`/`ReviewAdapter`/`DeliveryGate`/`SDD`) per row; classify row status (clean / `unowned` / `split-ownership` / `undesignated-target`).
- [x] 3.9 Write `Findings` section listing unowned/split-ownership/undesignated-target rows by ID; leave unresolved. Req: Inventory Row Completeness and Single Ownership.
- [x] 3.10 Write `Inventory gaps` section for unanchored rows and out-of-repo consumers.
- [x] 3.11 Record the SDD attempt-ledger ownership row per decision 9's conditional default (SDD-owned only with durable cumulative CAS-like properties; else native authority) — not an unconditional single owner. Req: SDD Attempt-Ledger Ownership Default.
- [x] 3.12 State the snapshot pin `ece470dacd0041f394e7f6f3877a6a9fcb3482af` explicitly in the header/preamble; state the document is not live authority. Req: Snapshot-Pinned, Non-Live Enumeration.
- [x] 3.13 **[GREEN]** Re-run the 3.1 recipe; confirm every enumerated point appears exactly once; record pass output.
- [x] 3.14 Verify diff scope: only `docs/architecture/rdd-ownership-inventory.md` changed in this PR.

## Phase 4: PR 3 — Freeze Policy + Backlog Disposition (base: PR 2 branch)

- [x] 4.1 **[RED]** Author the structural assertion recipe for both documents before content (freeze policy: 7 sections + 4-criterion test present; disposition: every seeded number classified exactly once). Confirm failure against the current nonexistent files. Record ad hoc, not committed.
- [x] 4.2 Write `docs/architecture/rdd-freeze-expansion-policy.md`: `Decision` → `Quick path` (4 steps) → `Scope` (frozen rows/globs by inventory row ID; explicit non-frozen: docs, behavior-pinning tests, tracker wave work) → `Proven security defect` → `Required evidence` → `Escalation path` → `Status and expiry`. Req: Policy Scope, Criteria, Evidence, and Escalation.
- [x] 4.3 Encode the four-criterion conjunctive exemption test: (1) concrete confidentiality/integrity/authorization impact on a named non-negotiable-invariant row, (2) reproduction on current `main` per `skills/rdd-defect-workflow`, (3) minimal fix inside the frozen surface — no new state/verb/reason-code/contract-version/artifact, (4) declared rollback boundary. Three-of-four escalates, never exempts. Req: Security-Defect Reproduction Criterion.
- [x] 4.4 State maintainer-internal, non-CI-binding scope explicitly; no CI enforcement introduced; policy expires at Wave 7 completion or tracker abandonment. Req: Maintainer-Internal, Non-CI-Binding Scope.
- [x] 4.5 Seed `docs/architecture/rdd-backlog-disposition.md` from the union of the amended design's `Issue and PR coverage map` (including `#1379`) and the audit's issue map, deduplicated by number; one read-only `gh` pass records title/state for seeded numbers only. Req: Coverage-Map-Bound Baseline Completeness.
- [x] 4.6 Classify each seeded item into exactly one of `superseded-by-design` / `absorbed-into-wave-N` / `still-valid-fix-now` / `orthogonal` / `unclassified — needs triage` (equal support for two classes → `unclassified`). No closure/relabel proposals. Req: Fixed Classification Vocabulary, Classification-Only, No Closure Proposals.
- [x] 4.7 Record header `snapshot: ece470da` + `github state read at: <ISO-8601 UTC>`; per-row state @ read; first-paragraph advisory/non-closing statement.
- [x] 4.8 Write the closure audit protocol section (5 steps, ending in: verify obsoleted issues/PRs closed as outdated, each with a wave-PR + design-section comment and a recorded closure date).
- [x] 4.9 **[GREEN]** Re-run the 4.1 recipe; confirm both documents pass; record output.
- [x] 4.10 Verify diff scope: only the two Phase 4 files changed in this PR.

## Phase 5: Verification handoff (sdd-verify — not a PR of its own)

- [x] 5.1 Compile the ad hoc RED/GREEN outputs from 2.1/2.13, 3.1/3.13, 4.1/4.9 into `verify-report.md`; do not commit the recipes themselves.
- [x] 5.2 Run non-regression checks: `git diff --name-only` restricted to `docs/` + `openspec/`; `go build ./...`, `go vet ./...`, `go test ./...` unchanged.
