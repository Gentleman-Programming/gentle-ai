# Design: RDD Root Simplification — Wave 0 (Freeze Expansion)

## Technical Approach

Wave 0 ships four documents and zero code. The architecture question is therefore not "what runs" but "what is derivable, and from which authority". The strategy is: **one normative document (the design), and three instruments derived from it by a stated, repeatable method**.

```text
docs/audits/2026-07-21-rdd-system-audit.md ─┐
snapshot ece470da source (CodeGraph)       ─┤
design (amended, normative) ───────────────┴─→ ownership inventory (surface → one target owner)
                                            ├─→ freeze policy      (what may still be touched)
                                            └─→ backlog disposition (what the backlog now means)
```

Each instrument cites the design section it derives from, so a later wave amends the design once and the instruments are regenerated rather than re-argued. Every derived row carries a file-anchored evidence reference pinned to `ece470da`; nothing in Wave 0 is live authority.

## Architecture Decisions

### Decision: Amend the design in place, section-targeted

**Choice**: Copy the 515-line source document into `docs/architecture/rdd-root-simplification-design.md` unchanged in structure and register, then apply A–E as surgical edits to the exact sections below, plus a closing `Amendment log` table mapping each ID to the sections it touched.
**Alternatives considered**: (a) rewrite the document around the amendments; (b) land the document as-is and add an `amendments.md` overlay.
**Rationale**: (a) destroys reviewability — the maintainer already approved this text; the diff must show only the five accepted deltas. (b) creates two documents that can disagree, which is the exact root cause the design names. The `Amendment log` makes success criterion "A–E individually verifiable" a table lookup instead of a full re-read.

| ID | Sections touched | Edit shape |
|---|---|---|
| A | `Canonical candidate identity and relation algebra` (relation table, `compatible_base_advance` row) + new subsection immediately after the table + `Adversarial safety analysis` (`Incompatible base advance` row) + `Evidence and scope` (new proof-source row) | The `Proof requirement` cell points to the normative source; the new subsection names `internal/reviewtransaction/prepr.go` `deriveBaseAdvanceCompatibility` (line 73) and lists its conditions — merge-base tree preservation, path digest identity, patch identity, path disjointness, conflict-free `merge-tree --write-tree`, issuer-bound CI attestation and trust root, plus base/HEAD non-advance revalidation — as normative rather than re-derived |
| B | Same relation table (`provable_contraction` row) + the same new subsection + `Adversarial safety analysis` (new `Unsound contraction` row) + `Acceptance criteria` (one bullet) | Soundness condition: contraction validates only when admitted findings reference no excluded path; otherwise the relation degrades to `changed` |
| C | `Migration waves` (Wave 3 row, `Rollback boundary` cell) + a precedence note under the wave table + `Adversarial safety analysis` (new `Coexistence precedence` row) | Legacy readable authority never authorizes delivery of a candidate that has a new lineage |
| D | `Unresolved maintainer and product decisions` (two new rows, same `Decision / Recommended default / Tradeoff` columns) | External evidence retention horizon; SDD attempt-ledger ownership. Defaults per proposal decisions 8–9 |
| E | `Adversarial safety analysis` (new `Cross-lineage receipt contamination` row) + `Issue and PR coverage map` (`Receipt-only delivery validation` row gains `#1379`) | One coverage row only — the map is a coverage index, not a duplicate index |
| — | `Evidence and scope` (`Prior system audit` row) | `/home/gentleman/work/rdd-system-audit.md` → `docs/audits/2026-07-21-rdd-system-audit.md`; the SHA-256 row is re-verified against the in-repo file before landing |

Amendment E adds `#1379` to exactly one coverage row. If the recomputed SHA-256 of the in-repo audit differs from `4b41d15a…`, record the measured digest and a discrepancy note — never assert the expected value.

### Decision: Inventory records observed ownership and target ownership separately

**Choice**: Each inventory row carries `current owner(s)` (observed at `ece470da`) and exactly one `target owner` drawn from the design's closed ownership-boundary set (`ReviewCore`, `AuthorityStore`, `CandidateResolver`, `ReviewAdapter`, `DeliveryGate`, `SDD`). Findings are then mechanical, not editorial.
**Alternatives considered**: a single `owner` column resolved by judgment per row.
**Rationale**: a single column forces Wave 0 to *decide* ownership for surfaces whose split ownership is the defect under study — that is out of scope and would launder a finding into a fact. Two columns make the proposal's rule computable:

| Observed current owners | Row status |
|---|---|
| exactly 1 | clean |
| 0 | finding `unowned` |
| >1 | finding `split-ownership` (the design's named root cause) |
| target not derivable from the design | finding `undesignated-target` |

Findings are listed in a `Findings` section by row ID and are **not** resolved in Wave 0.

### Decision: Enumerate from authoritative enumeration points, never from prose

**Choice**: Every family is enumerated from a named source-of-record so the inventory can be re-derived at a later SHA.

| Family | Row ID | Authoritative enumeration point at `ece470da` |
|---|---|---|
| Lifecycle transitions | `TRN-nn` | `internal/reviewtransaction/transaction.go` state vocabulary (`StateUnreviewed`…`StateInvalidated`) + compact state set + `internal/cli/review_next_transition.go` (`next_transition` producer) |
| Public operation forms | `TRN-nn` | `internal/app/app.go` dispatch (`review`, `review-resume`, `review-step`, bundle/validate legacy forms), cross-checked against `internal/app/help_test.go` |
| Persisted artifacts | `ART-nn` | `internal/reviewtransaction` store writers (authority record, receipt, journal, bundle, sidecar, quarantine residue) |
| Contract surfaces | `CTR-nn` | `contracts/review-integration/v1/**` and `v2/**` schema + fixture files, cross-checked against the schema-ID constants in `internal/cli/review_*.go` |
| Delivery gates | `CON-nn` | `GateKind` constants (`GatePostApply`, `GatePreCommit`, `GatePrePush`, `GatePrePR`, release) + `reviewIntegrationGatesInOrder` |
| Consumers | `CON-nn` | `internal/sddstatus/review_gate.go`, `internal/sddstatus/review_binding.go`, adapter/plugin assets, CI hook definitions |

**Alternatives considered**: derive the inventory from the design's own tables only.
**Rationale**: the design's tables are the *target*; an inventory built from them would prove nothing about the current system and would silently omit surfaces the design has not yet noticed. CodeGraph-first (`codegraph_explore`) read-only tracing is the method; no `codegraph index`, no source mutation.

**Evidence rule**: every row cites at least one `path:line @ece470da` or contract file/schema ID. A row that cannot be anchored is dropped and logged in `Inventory gaps` — a stated gap is honest, an unanchored row is not.

**Row schema**: `ID | Surface | Kind | Current owner(s) | Target owner | Consumers | Evidence | Target disposition` — the last column reuses the design's control-reduction verbs (`KEEP` / `MERGE` / `DERIVE` / `DOWNGRADE` / `REMOVE` / `FAIL-CLOSED ONLY`), which is what makes the inventory usable as wave input rather than a census.

### Decision: Freeze policy is a four-part conjunctive test, guidance-only

**Choice**: Document structure — `Decision` → `Quick path` (contributor with an old-facade change, 4 steps) → `Scope` (frozen surfaces by inventory row ID and path glob; explicitly not frozen: docs, tests that pin existing behavior, wave work on the tracker chain) → `Proven security defect` → `Required evidence` → `Escalation path` → `Status and expiry`.

`Proven security defect` is a conjunction of four criteria — all four, or it is not exempt:

1. Concrete confidentiality, integrity, or authorization impact on a named row of the design's non-negotiable-invariants table.
2. Reproduction on current `main` at a named SHA, per `skills/rdd-defect-workflow` (proposal Q3 assumption: reproduction required, a credible report alone is not).
3. Minimal fix inside the already-frozen surface — no new state value, verb, reason code, contract version, or persisted artifact. A fix that must add one of those is a wave, not an exemption.
4. Declared rollback boundary.

**Alternatives considered**: a severity-label test (`security` label ⇒ exempt), or a maintainer waiver per case.
**Rationale**: a label test is trivially satisfiable and would reopen the expansion path the wave exists to close; a waiver-per-case makes the maintainer the bottleneck for genuine security work. The conjunction is self-serving for a real defect (criterion 3 is the only one that bites) and needs no waiver, matching the proposal's risk mitigation. Required evidence includes the mechanism-not-site sweep and the class-closing guard from `backlog-triage` Phase 2b, plus the ≤400 authored-line forecast from the RDD defect workflow. Three-of-four escalates instead of exempting. Enforcement is guidance-only in Wave 0 (proposal Q1 assumption); the policy expires when Wave 7 completes or the tracker branch is abandoned.

### Decision: Backlog seeding is closed; GitHub is read-only enrichment

**Choice**: Seed the baseline from the union of the design's `Issue and PR coverage map` (including `#1379` after amendment E) and the audit's issue map, deduplicated by number. One read-only `gh` pass records title and state **for seeded numbers only**; it never discovers new items.
**Alternatives considered**: a full live `gh issue list` / `gh pr list` sweep of the open backlog.
**Rationale**: a live sweep drifts the moment it is written, and classifying items neither document supports would require triage authority Wave 0 does not have. A closed seed set is reproducible and auditable; enrichment keeps the rows readable without expanding the claim.

Classification is a closed vocabulary, exactly one per item:

| Class | Rule |
|---|---|
| `superseded-by-design` | The target architecture removes the mechanism; the reported site does not exist in the target |
| `absorbed-into-wave-N` | The fix is scheduled inside wave `N`'s scope (`N` from the migration-waves table) |
| `still-valid-fix-now` | The mechanism survives the target architecture, or the item is a security defect under the freeze policy |
| `orthogonal` | No relationship to the RDD lifecycle root cause |
| `unclassified — needs triage` | The two source documents do not support a single class, or support two equally |

The ambiguity rule mirrors the design's fail-closed stance: equal support for two classes yields `unclassified`, never a coin flip.

**Snapshot dating**: header records `snapshot: ece470da` and `github state read at: <ISO-8601 UTC>`; each row records the state observed then (`open` / `closed` / `merged`). The document states in its own first paragraph that it is advisory and closes nothing (proposal Q2 assumption).

**Row schema**: `# | Kind | Title | State @ read | Class | Wave | Source (design coverage row / audit map) | Rationale (one line)`.

**Closure audit protocol (post-Wave 7)** — the baseline must make the end-of-migration pass a *diff*, not a re-triage. The document carries the protocol as its own section:

1. Re-read each row against the shipped architecture.
2. Anything still claimed open requires reproduction on then-current `main`.
3. For `absorbed-into-wave-N`, confirm wave `N` shipped its exit evidence.
4. For `superseded-by-design`, require the design deletion-plan proof for the retired surface.
5. **Final step**: verify obsoleted issues and PRs are closed as outdated, each with a comment linking the wave PR and the design section, and record the closure date in the row.

Step 5 is why the row schema carries stable identity, source, wave, and rationale.

### Decision: Chain plan lives in the design, not a fifth document

**Choice**: New design section `Migration chain and delivery plan`, placed immediately after `Migration waves` and before `Issue and PR coverage map`.
**Alternatives considered**: `docs/architecture/rdd-migration-chain.md`.
**Rationale**: the wave table and its delivery mechanics are one decision; separating them adds a cross-reference and a drift surface for no reader benefit. The proposal already scopes this deliverable as "design doc section". Contents: tracker `feature/rdd-root-simplification` off `main@ece470dacd0041f394e7f6f3877a6a9fcb3482af`; PR #1 targets the tracker, PR #n targets PR #(n−1)'s branch; only the tracker merges to `main`; ~1000 changed lines per PR; if GitHub shows a previous slice inside a child diff, retarget or rebase until the diff is clean; a wave's exit evidence gates the next slice.

## File Changes

| File | Action | Description |
|---|---|---|
| `docs/architecture/rdd-root-simplification-design.md` | Create | Source document + amendments A–E + evidence-path correction + chain-plan section + amendment log (~570 lines) |
| `docs/architecture/rdd-ownership-inventory.md` | Create | Row-ID'd inventory, method section, findings, inventory gaps (~250 lines) |
| `docs/architecture/rdd-freeze-expansion-policy.md` | Create | Seven-section policy, four-criterion exemption (~120 lines) |
| `docs/architecture/rdd-backlog-disposition.md` | Create | Seeded classification baseline + closure audit protocol (~180 lines) |
| `openspec/changes/rdd-root-simplification-wave0/**` | Create | SDD artifacts (spec, tasks, verify report) |
| `internal/**`, `contracts/**` | None | Explicit non-goal — read-only evidence sources |

## Testing Strategy

`strict_tdd: true` is set for this repo, but the deliverables are prose with no executable unit. The honest equivalent is **mechanical verification authored before the content**: write the assertion recipe from the proposal's success criteria, confirm it fails against the empty `docs/architecture/` state, then write until it passes.

| Layer | What to test | Approach |
|---|---|---|
| Structural (RED first) | Each of A–E present; amendment log rows resolve to real sections; audit path resolves; recomputed audit SHA-256 recorded | `rg` assertions + `sha256sum docs/audits/2026-07-21-rdd-system-audit.md` |
| Completeness | Every seeded issue/PR number appears exactly once in the disposition baseline; every gate, contract dir, and state constant appears exactly once in the inventory | Set-difference between the enumeration point and the document's rows |
| Non-regression | No source change | `git diff --name-only` restricted to `docs/` + `openspec/`; `go build ./...`, `go vet ./...`, `go test ./...` unchanged |
| Link integrity | Every `path:line` evidence reference exists at `ece470da` | Spot-check by path existence; unresolvable references become `Inventory gaps` rows |

The recipe is executed ad hoc and recorded in `verify-report.md`; it is not committed, because committing a script would violate the "no file outside `docs/` and `openspec/`" success criterion.

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary is created. The chain plan *documents* Git topology; it automates nothing. Read-only `gh` and `git` usage during authoring produces no committed artifact.

## Migration / Rollout

No migration. Rollback is deletion of the four documents on the tracker branch; `main` is untouched.

### PR slicing preview (input to `sdd-tasks`)

Session budget is 1000 authored lines per PR (`review_budget_lines: 1000`). Individually every deliverable fits; combined they do not. Recommended boundaries on the feature-branch chain:

| Slice | Target branch | Contents | Forecast (add+del) |
|---|---|---|---|
| PR 0 | `feature/rdd-root-simplification` | `openspec/changes/rdd-root-simplification-wave0/**` SDD artifacts | ~450 |
| PR 1 | PR 0 branch | Amended design + chain-plan section + amendment log | ~570 |
| PR 2 | PR 1 branch | Ownership inventory | ~250 |
| PR 3 | PR 2 branch | Freeze policy + backlog disposition | ~300 |

Rationale for the boundaries: PR 1 is the only normative artifact and must be reviewable alone; PR 2 depends on PR 1's ownership-boundary and control-reduction tables; PR 3's two documents share one reviewer context (policy + classification instrument) and are ~120/~180 lines each, so splitting them adds a round-trip without reducing load. Isolating the SDD artifacts into PR 0 keeps PR 1 from drifting toward the budget ceiling. `sdd-tasks` owns the binding forecast and guard lines.

## Open Questions

- [ ] The five proposal-round product questions (Q1–Q5) remain answered only by assumption. This design encodes Q1 (guidance-only), Q2 (classify, never close), and Q3 (reproduction required) into document structure; a later answer changes the freeze policy and disposition sections, not the architecture.
- [ ] Whether the in-repo audit's SHA-256 matches `4b41d15a…` is unverified until the landing task recomputes it.
- [ ] Consumer enumeration for adapters (OpenCode, Pi, Claude plugin assets) may extend past this repository; out-of-repo consumers are recorded as `CON-nn` rows with `evidence: out-of-repo` and flagged as an inventory gap rather than guessed.
