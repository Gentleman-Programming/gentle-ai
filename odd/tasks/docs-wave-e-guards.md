# docs-wave-e-guards

Issue: #5234 (tracker #4556). PR 1 branch: `docs/5234-retired-term-guard` from upstream/main 7af7eef8. PR 2 gets its own branch from main; the two PRs are independent, not stacked.
Goal: Go-test guards so retired terms and hardcoded inventories cannot drift back into living docs.

## Specs

- S1 Retired-term guard: "Extend the existing test to walk every living doc: `README.md`, `CONTRIBUTING.md` and `docs/**`. Exclude historical material: `docs/audits/`, `docs/releases/`, `docs/architecture/rdd-*`, and pages flagged as historical." Existing guard: `internal/app/documented_invocation_test.go:409-414` (`/sdd-`, `/gentle-sdd-`, `gentle-ai sdd-`, `SDD phases`, `SDD agents`, `OpenSpec`).
- S2 "Allow only explicit retirement or legacy notes, through a small, reviewed allowlist. Examples are the "retired in v4.0.0" notes and the legacy `sdd-*` telemetry schema values (`docs/telemetry.md`)."
- S3 "Add `--strict-tdd` to the retired list." Do not ban "Strict TDD" (current, `agentguidance/strict_tdd.go`) or "shadow" (ambiguous).
- S4 "Clean the leftovers listed above so the guard starts green." — `docs/engram.md:159`, `docs/testing-agents-deterministically.md:70,71,251`.
- S5 "Reword the template checkbox: "If behavior changed, docs in `docs/` are updated in the same PR (reference docs track `main`).""
- S6 "Add a test that fails when `docs/agents.md` and `allAgents` disagree. The failure lists missing and extra agents."
- S7 "Add a test that keeps the `README.md` agents badge equal to `len(allAgents)`."
- S8 "Add a test that fails when the component trees in `docs/architecture.md` and `docs/codebase/repository-map.md` miss or invent a package under `internal/components/`." Amended per L9: `docs/architecture.md` is checked 1:1; `docs/codebase/repository-map.md` has no tree (ownership table), so every `internal/components/<pkg>/` it names must exist, and zero references fail.
- S9 "Fix any drift these tests find."
- Decision: drift tests, not generated blocks (no `go generate`, no Markdown markers). Each PR under 400 lines.

## Tasks

- [x] T1 S1-S4 Retired-term guard (RED test, then clean leftovers) — route: delegated writer (mutr97me-2-8bk7) — commit: 9c8c0368
- [x] T2 S5 PR template checkbox — route: inline — commit: 40de7ebc
- [x] T3 S1-S5 Verify PR 1, RDD review, PR "Refs #5234" — route: verifier + parent — commit: 1857e7e3 (S4 wording); merged as #5236 (ccec7b35)
- [x] T4 S6-S9 Inventory drift tests (branch `docs/5234-inventory-drift-guard`) — route: delegated writer (mutsgd2z-4-yzb3) — commit: 52a56f55 (pre-rebase a1d8318b)
- [ ] T5 S6-S9 Verify PR 2, RDD review, PR "Closes #5234" — route: verifier + parent — commit: a574d8da (pre-rebase 5f32d836)

## Log

- L1 (user, 2026-10-04): "Retomo la wave E de la meta-issue #4556 (docs accuracy re-audit). [...] Scope de la wave E, según la tabla de #4556: "Guards: PR template docs checkbox, CI check for retired terms in living docs, generated agent and package inventories""
- L2 (user): accepted drift tests over generation ("Adelante"); label changed to type:docs; #5234 created and approved ("YA la aprobe, crea la rama y adelante").
- L3 (evidence): explorer mutqjc3c-1-zswz; existing guard covers 4 files only; `allAgents` has 17 entries (`internal/catalog/agents.go:18`); 20 dirs under `internal/components/`.
- L4 (evidence, T1): RED listed 7 lines (engram.md:159, gga-powershell-shim.md:71, skill-registry.md:128, testing-agents-deterministically.md:70,71,251, trigger-rules.md:14); GREEN after fixes; `go test ./internal/app/` ok. Historical rule: `> **Historical` callout in first 12 lines (skips organic-rdd-testing-guide.md, release-v0.1.0-checklist.md). Allowlist: one line-level rule `\b(retired|legacy)\b`. Tradeoff accepted by parent: the 4 entry files lost the file-wide no-exception check; a line saying retired/legacy now passes there too. New wording at testing-agents-deterministically.md matches `assertNoSDDArtifacts` (e2e/organicruntime/organic_runtime_test.go:3478).
- L5 (decision, T2): `skills/branch-pr/SKILL.md:133` keeps its own template copy, which already diverges from the real template; left out of scope.
- L6 (evidence, T3): verifier mutrfwio-3-a1bp: S1-S3, S5 PASS; S4 FAIL (testing-agents-deterministically.md:70,71,251 overclaimed "no workflow ... state"; `assertNoSDDArtifacts` only rejects sdd-*/sdd/trace/evaluation under the common-dir gentle-ai store). `go test ./internal/app/ ./internal/assets/` ok; gofmtcheck and vet clean; 162 changed lines.
- L7 (evidence, T3): user granted RDD consent on the host. Lineage review-470e723a6fe6dccf (medium, one lens review-reliability, base 7af7eef8 committed-only, target sha256:604ea30d...) approved with one informational WARNING R3-broad-retirement-exemption (same tradeoff as L4); acknowledge-approved burned authority (consumed sha256:2f49e370...). The S4 wording was then narrowed to "retired-workflow, trace, or evaluation state" in a passive docs follow-up commit (not reviewed: trivial passive docs edit).
- L8 (evidence, T3): second RDD on the final PR 1 head (tree 6e4bae3e) review-dc3221ab97310f9b approved (same informational warning) and burned. PR #5236 first failed "Check Issue Reference": the Summary prose said "closes #5234" next to `Refs #5234` (ambiguous); prose reworded, 19/19 checks pass. User merged #5236 as ccec7b35.
- L9 (decision, T4; user deferred to the parent, option A): `docs/codebase/repository-map.md` has no components tree, only an ownership table naming a few packages. Requiring all 20 would invent ownership rules, so it is checked only for invented names; `docs/architecture.md` stays 1:1. No drift found (17 agents = 17 rows incl. Conductor, matched by ID; 20 packages = 20 tree entries), so S9 needed no doc edits; parser tests on made-up input prove the tests can fail.
- L10 (evidence, T5): verifier mutssdpx-5-2knu: S6, S7, S9 PASS; S8 FAIL (repository-map check matched only backticked paths and had no zero-match guard). Parent fixed it in 5f32d836; manual mutation (unbackticked `internal/components/ghost/`) failed with `invented: [ghost]`.
- L11 (evidence, T5): RDD review-6e856a5bac87f460 (a1d8318b) approved and burned; review-4018f7c56ac6f025 (5f32d836) approved with informational WARNING R3-001 (tree parser accepts only lowercase alphanumeric names; an underscore package would fail as missing) and burned. PR 2 rebased onto ccec7b35 after #5236 merged.
