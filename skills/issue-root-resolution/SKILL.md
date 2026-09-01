---
name: issue-root-resolution
description: "Trigger: root audit, atacar la raíz, issue roots, backlog roots, mechanism map, deletion-driven fix, resolver issues de raíz, close outdated issues. Audit and resolve issue clusters by verified root cause."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Load when auditing a defect backlog for shared root causes, proposing a fix for an issue cluster, or closing issues as resolved/outdated. Complements `rdd-defect-workflow` (single-defect flow) — this skill governs the cluster-level method.

## Hard Rules

- Read full bodies and comments, never titles. Classify against the actual code or PR diff, not descriptions of it.
- Verify every "fixed" or "broken" claim against current `origin/main` the day you act. Compare report dates to fix merge dates: a repro filed before the fix landed is evidence about old builds, not current code.
- Cluster defects by causal root, not surface. Each root gets a measured row in the meta-issue (#2471 style): issues attached, fix shape, state.
- Before proposing any fix, produce a read-only mechanism map: one file:line anchor per claim, mechanism explained as implemented today. Report claim-vs-code mismatches explicitly; never force evidence to fit the hypothesis. Let the map shrink the proposal.
- Rank solutions by what they DELETE: (1) removes a mechanism so the class becomes impossible, (2) static guard-ratchet making reintroduction a test failure, (3) localized predicate fix behind a failing repro test. New surface (verbs, flags, mechanisms) is last resort — defer until re-verified evidence demands it.
- Extract maintainer choices as named D-items with recommended defaults. A maintainer condition recorded in an issue thread outranks any plan table, including yours.
- Close only with evidence, one rule per closure: (A) fixed on main, cite commit AND proving test; (B) superseded by recorded maintainer decision; (C) surface no longer exists; (D) duplicate of a fixed issue. Comment before closing: what resolved it, verified today, reopen invitation. When in doubt, do not close — list as borderline.
- Clean break, never compatibility. When a format or identity changes, bump its versioned tag as a REPLACEMENT: old records become outdated and fail closed with an actionable refusal naming the rerun. Never write dual recognition, legacy fallbacks, or compat shims; that residue is what the deletion criterion exists to prevent. Stored bytes are never rewritten and stay readable for forensics, but carry no gate or lifecycle validity.
- Implementation follows the waves recipe: repro as failing test first, byte-stable goldens as defect signal, independently revertible slices, one writer per slice.
- A universal guard's verification matrix is `go test ./...` at the repository root, never a curated package list. A guard that forbids a shape breaks every fixture that relied on it, including in packages the change never touched.
- One worktree per writer, always, including the orchestrator. Before editing inline, check whether a delegated worker holds that path; if so, create a separate worktree rather than reusing it.
- Audit every worker report yourself: re-run its key verification, decoy-test any new guard, spot-check its diffs. A self-report is a claim, not evidence.

## Decision Gates

| Condition | Action |
| --- | --- |
| Issue's failure mechanism absent from current main | Re-verify on latest build; close by rule A/C or mark stale — never fix ghost code. |
| Fix would add a mechanism, flag, or verb | Defer with written reason; re-rank for a deletion-shaped alternative first. |
| Map contradicts the issue or your hypothesis | The map wins. Revise the plan and correct the public record (meta-issue) before coding. |
| Blocker's fix lives in an unmerged PR | Leave open; it closes on merge or by its own thread condition. |
| Closure evidence incomplete | Borderline list, not closure. |

## Execution Steps

1. Measure: read the cluster fully; partition by root; record counts in the meta-issue.
2. Classify each issue against the real diff/code; separate closes-via-X, improved-not-closed, unrelated, blocker.
3. Map mechanisms read-only with anchors; flag mismatches.
4. Rank fixes deletion-first; name D-items; get maintainer answers.
5. Implement in slices (issue-first: every PR links a `status:approved` issue), audit each worker report.
6. Hygiene pass: evidence-gated closures, stale-repro re-verification requests, meta-issue update with what changed and why.

## Output Contract

Per pass, report: roots table (issues, fix shape, state), closures with rule+evidence, borderline list with reasons, D-items and their answers, mechanism-map mismatches found, and the meta-issue comment link.

## References

- `../rdd-advisory-transport/references/shared-advisory-transport-proposal.md` — exemplar proposal shape produced by this method.
- `../rdd-advisory-transport/references/issue-impact-matrix.md` — exemplar per-issue disposition matrix.
