# Organic RDD Recovery Plan

> **Decision:** Gentle AI remains an ecosystem configurator for existing coding
> agents. Normal implementation stays inside the configured agent. Receipt-Driven
> Development begins only after the agent has produced a candidate it considers
> complete.

This plan supersedes
[`2026-07-23-organic-recovery-implementation-plan.md`](./2026-07-23-organic-recovery-implementation-plan.md).
It removes the speculative productive-runtime/control-plane design and restores
the product promise: users ask for outcomes, their chosen agent works
organically, and Gentle AI supplies invisible best practices plus a small
deterministic local receipt boundary.

It does **not** supersede the systemic backlog analysis in
[`2026-07-23-systemic-remediation-architecture.md`](./2026-07-23-systemic-remediation-architecture.md).
That audit remains the canonical reconciliation of all 241 snapshot issues and
92 snapshot pull requests. This recovery changes one drifted implementation
route; it does not discard the generic fixes, invariants, contributor work or
cross-platform edge cases found by the audit.

## 0. Preservation rule

No branch, package, commit, issue, pull request, test or edge-case fix may be
removed merely because it was implemented while the productive-runtime design
was active.

Every changed artifact receives one explicit disposition:

| Disposition | Meaning |
|---|---|
| `KEEP` | Correct product boundary and implementation; retain and revalidate |
| `TRANSPLANT` | Generic invariant is correct, but move it out of WorkRun/runtime coupling |
| `REWRITE` | User-facing behavior or ownership is wrong; preserve the requirement and replace the implementation |
| `DELETE` | Exists only for the rejected remote/control-plane product |
| `REGENERATE` | Derived fixture, schema, golden or asset; rebuild it from its corrected canonical source |
| `DEFER` | Valid independent work that is not required for this recovery and remains publicly traceable |

Deletion is permitted only when all three statements are true:

1. the code or artifact exists solely for the rejected runtime/control-plane
   concept;
2. it contains no independent safety invariant or historical issue coverage;
3. its source issue/PR remains mapped to a retained, transplanted, rewritten or
   explicitly deferred owner.

The recovery therefore uses **selective extraction**, not a branch reset.

### 0.1 The nine systemic contexts remain

| Context | Snapshot coverage | Recovery treatment |
|---|---:|---|
| `HCR` — host, install and command runtime | 43 issues / 15 PRs | Keep host facts, bounded process execution, install provenance and cross-platform behavior. “Runtime” means local command execution, never a model/runtime server. |
| `MMI` — managed mutation integrity | 25 / 15 | Keep transactional writes, CAS, rollback, crash recovery, path/ownership/mode safety and concurrent-edit protection. |
| `ACI` — agent capability and instruction projection | 40 / 26 | Keep canonical agent IDs, aliases, variants, capabilities and deterministic asset projection. Remove pre-request WorkRun negotiation and runtime authentication. |
| `MCA` — model catalog and assignment | 7 / 2 | Keep source provenance, freshness, aliases, tri-state capabilities and profile isolation. Never turn model selection into a remote-runtime prerequisite. |
| `RAR` — review authority and receipts | 39 / 4 | Keep exact candidate identity, immutable evidence admission, bounded correction, terminal receipts and gate replay. Make it post-candidate and proportional. |
| `EPD` — evidence, policy and diagnostics | 21 / 6 | Keep typed evidence, ordered policy and safe diagnostics. Remove transport/authentication concepts that exist only for a remote runtime. |
| `DSR` — desired-state resource reconciliation | 28 / 13 | Keep deterministic install/sync/update/uninstall ownership, idempotence, adoption rules and safe retirement. |
| `SDD` — lifecycle and artifacts | 25 / 7 | Keep typed optional proposal/spec/design/tasks/apply/verify/archive state. It remains one implementation route, not the universal route. |
| `PAD` — product admission and delivery | 13 / 4 | Keep exact-SHA CI, release provenance, contribution credit and repository-policy enforcement. Rewrite admission so it does not force an issue or PR when policy permits another route. |

The 241/92 ledger in Appendix B of the systemic audit remains immutable input
to the recovery. A second ledger will join each item to its final disposition,
owning invariant, implementation location, regression proof and release status.

## 1. Product boundary

Gentle AI configures the coding agent the user already chose with:

- persistent memory;
- skills and MCP integrations;
- delegation rules;
- optional SDD;
- model and subagent routing;
- persona and teaching behavior;
- proportional RDD guidance;
- local deterministic receipt and delivery gates.

Gentle AI is **not**:

- another coding agent;
- a model host;
- a daemon or remote runtime;
- an enterprise control plane;
- a centralized policy server;
- a prerequisite service that must be authenticated before implementation.

The Go binary owns only operations that prompt text cannot safely guarantee:

- install, sync, upgrade and rollback;
- exact Git candidate identity;
- immutable local review authority;
- content-bound receipts;
- lifecycle gate validation;
- bounded local mutations protected by revision/CAS checks.

## 2. New happy path

```text
User asks for an outcome
        |
        v
Configured agent selects direct, delegated or optional SDD
        |
        v
Agent investigates, implements, tests and normalizes the candidate
        |
        v
RDD classifies the completed candidate proportionally
        |
        +-- trivial/docs-only --> local identity + cheap checks, no AI review
        |
        +-- standard ---------> one consolidated review
        |
        +-- genuinely high-risk -> focused 4R review
        |
        v
At most one scoped correction for confirmed candidate-caused blockers
        |
        v
Gentle AI issues one content-bound local receipt
        |
        v
Commit/push/PR/main/release validates that same receipt
```

There is no pre-implementation `work-capabilities` handshake, productive
connector, activation ceremony, or universal WorkRun.

## 3. Implementation routing remains organic

The configured agent owns normal routing:

| Situation | Route |
|---|---|
| Small and already understood | Direct inline |
| Research, mapping, or multiple non-trivial files | Delegated direct |
| Substantial ambiguity or explicit user request | Propose SDD |
| User declines SDD | Use the safest direct/delegated fallback |

SDD remains optional. Complexity may justify proposing it, but never selects it
silently.

RDD does not decide how the feature is implemented. It receives a completed
candidate.

## 4. Candidate normalization and freeze

Before identity freeze, the agent may run source-mutating operations:

- formatters;
- generators;
- migrations;
- snapshot updates;
- safe automatic corrections.

After normalization, Gentle AI freezes:

- repository and Git common-dir identity;
- target projection;
- exact bytes, paths and modes;
- base and candidate relation;
- policy inputs;
- target/subject hash.

Any later byte, path, mode, base or policy change invalidates the receipt. A
receipt never floats with a branch name or user intention.

## 5. Proportional review policy

### Tier 0 — trivial or documentation-only

- No AI reviewer.
- No 4R.
- No runtime verification ceremony when the observable change is already the
  document or metadata itself.
- Gentle AI may perform cheap deterministic parsing, formatting or schema
  checks.
- A local receipt may still bind the exact candidate for delivery without
  exposing ceremony to the user.

### Tier 1 — standard change

- One consolidated review focused on the dominant risk.
- One bounded sweep.
- Warning and suggestion findings are informational.
- Only concrete candidate-caused blocker/critical findings can authorize a
  correction.
- No refuter fan-out when there are no severe candidates.

### Tier 2 — genuinely high-risk change

Use focused 4R only when the candidate can plausibly cause:

- authorization or permission weakening;
- secret or sensitive-data exposure;
- data loss or irreversible corruption;
- unsafe mutation, delivery or rollback behavior;
- release/provenance compromise;
- security-critical execution.

File count or changed-line count alone does not force 4R. Large mechanical or
documentation changes remain proportional to their actual risk.

## 6. Proportional verification policy

| Verification class | Behavior |
|---|---|
| None useful | Record not-applicable; do not invent evidence |
| Cheap and deterministic | Run automatically |
| Expensive or long | Tell the user what will run and ask once before launch |
| Impossible in the current environment | Explain the missing evidence and offer defer/reduce-scope; never loop |
| Destructive or permission-sensitive | Require explicit authorization immediately before the effect |

The verification plan is frozen before any consent question. A resume uses the
same plan; it never asks an AI to regenerate commands, cost, environment or
redactions.

Documentation-only changes do not receive fake runtime verification.

## 7. Review, correction and receipt

1. `review start` freezes the candidate and selects the proportional tier.
2. The configured agent executes only the selected review.
3. Reviewer output is re-admitted as immutable candidate/lens-bound authority.
4. `review finalize` classifies candidate-caused severe findings.
5. If needed, one scoped correction transaction is authorized.
6. Only admitted finding paths and the bounded correction budget may change.
7. Independent evidence verifies the correction and required behavior.
8. Gentle AI issues an approved, escalated or invalidated terminal receipt.

There is no loop-until-clean behavior and no silently renewed correction
budget.

## 8. Delivery and escape routes

Delivery choice remains independent from implementation and review:

- commit directly;
- push directly to main when repository policy permits it;
- create a PR without an issue;
- create a PR with an issue;
- use an explicit emergency path.

Gentle AI validates the same candidate receipt and repository policy before the
effect. It does not force every contribution through one GitHub ceremony.

External Git/GitHub authentication continues to come from the user's existing
`git` or `gh` setup. Gentle AI does not invent another credential system.

## 9. Remove from the current branch

Delete the unreleased control-plane/runtime path:

- productive HTTPS connector and its bootstrap/call protocol;
- URL, token, CA and global-agent environment variables;
- connector session references;
- remote policy snapshot and remote-operation assumptions;
- `work-capabilities/v1` and `/v2`;
- universal `work-start`, route, advance, decision, reconcile, status and
  transition intake used to wrap normal development;
- productive-runtime activation modes whose only purpose is that connector;
- TLS `httptest` runtime presented as product E2E evidence;
- tests and fixtures that prove only remote transport or dormant behavior;
- orchestrator rules that negotiate a WorkRun before every normal request;
- documentation claiming that the remote fixture is a usable shipped runtime.

Do not retain speculative interfaces merely because they might support an
enterprise product later.

## 10. Preserve from the current branch

Preserve and revalidate:

- native compact review authority;
- candidate snapshots and content-bound identity;
- admitted reviewer result capture;
- one-correction budget;
- exact receipt replay;
- post-apply, pre-commit, pre-push, pre-PR and release gates;
- recovery that protects existing receipts without exposing internal taxonomy;
- Windows path/DACL protections;
- exact-head CI;
- useful adversarial tests for data loss, permission weakening and unsafe
  mutation;
- organic direct/delegated/optional-SDD rules;
- Engram, skills, MCPs, model routing and adapter-native configuration.

Generic fixes already developed on this branch are not lost when their current
caller is deleted. They are retained in place or transplanted to their correct
owner. This includes, at minimum:

- Windows private/shared directory DACL distinction, owner rebind order and
  rejection diagnostics;
- canonical repository aliases and Git common-dir/worktree identity;
- repository-scoped leases, CAS revisions and cross-repository isolation;
- exact candidate/head validation, including untracked generated Go files;
- bounded command output, timeout and descendant-process handling;
- successful E2E command-output preservation;
- immutable admitted reviewer results and exact result/receipt pairing;
- atomic rejection of duplicate or terminal transitions;
- one-correction budget and exact replay that does not consume a new budget;
- optional SDD authority kept separate from normal direct/delegated work;
- exact delivery decisions and receipt validation at later gates.

If a generic invariant currently lives inside `workprovider`, `workrun` or a
productive-runtime contract, the recovery first adds equivalent focused tests
against the retained owner and only then removes the obsolete caller.

### 10.1 Historical edge-case catalogue

The following failure classes remain mandatory regression targets:

| Owner | Edge cases that must remain covered |
|---|---|
| `HCR` | probe failure as `unknown`, ambiguous installs, PATH/canonical executable identity, TTY/non-interactive behavior, stdout/stderr truncation, timeout/cancellation, process-tree cleanup, post-install owner/destination/version observation, package-manager and signing provenance |
| `MMI` | symlink/junction escape, case-fold ambiguity, ACL/DACL/SID and Unix mode differences, unexpected ownership, concurrent edits, partial multi-target failure, rollback after user modification, crash/restart, backup/restore, marker migration and operation-ID reuse with different bytes |
| `ACI` | aliases/rebrands/variants, all supported adapters, unsupported versus unknown capabilities, target resolution, semantic parity across provider renderers, duplicate prompt blocks, ordering, language policy and generated-asset drift |
| `MCA` | ambiguous aliases, stale/incomplete catalogs, custom-provider provenance, per-agent/profile isolation, missing capabilities and prohibited silent model substitution |
| `RAR` | worktrees/common-dir, exact base/head/path/bytes/modes/object types, compatible-base relations, tamper after approval, immutable publication, replay versus correction, cumulative delivery, receipt corruption and bounded repair |
| `EPD` | exactly-once tickets, restart-safe budgets, deadlines/terminal causes, raw/semantic evidence binding, redaction/truncation, ordered wildcard policy, locked restrictions, model-authored identity rejection and read-only diagnostics |
| `DSR` | desired/observed/owned state separation, deterministic plans, second-apply no-op, unknown-content preservation, adoption/removal policy, source precedence, stale cache identity, install/sync/uninstall symmetry and resource-family migration |
| `SDD` | explicit store identity, optional selection, durable attempts, replay without budget reset, generation/scope changes, allowed roots, artifact hashes, archive/continuation safety, legacy import-only behavior and review-receipt handoff |
| `PAD` | direct main, direct push, PR without issue, PR with issue, emergency path, exact-head CI, existing Git/`gh` credentials, contributor credit, release provenance and external-product boundaries |

## 11. Migration work units

### Work unit A — freeze and classify the current diff

- Preserve the current branch and uncommitted state.
- Map every changed file and commit to `KEEP`, `TRANSPLANT`, `REWRITE`,
  `DELETE`, `REGENERATE` or `DEFER`.
- Reconcile the map against all 241 issues and 92 PRs from the systemic ledger.
- Keep independent bug fixes that remain valid without the runtime design.
- Extract generic invariants from rejected packages before deleting callers.
- Preserve source PR numbers, authors and contributor credit on every extracted
  implementation.
- Record rollback boundaries before editing.

### Work unit B — remove the pre-implementation runtime

- Delete connector/config/bootstrap/session code.
- Delete the capabilities and universal WorkRun intake from CLI, app, contracts
  and assets.
- Restore direct/delegated/optional-SDD routing as prompt-owned orchestration.
- Remove remote-only and dormant-only tests.

### Work unit C — simplify post-candidate RDD

- Make the post-implementation entry point explicit and local.
- Encode the tier 0/1/2 proportional policy.
- Make docs-only and no-useful-verification flows ceremony-free.
- Keep long/destructive consent as one human decision.
- Reuse the same receipt at every later gate.

### Work unit D — simplify recovery

- Public output becomes one exact `continue`, `validate`, `repair` or `stop`
  action.
- Internal repair taxonomy remains audit data, not user vocabulary.
- Exact replay reads existing authority before repeating work.
- No new state machine or parallel ledger.

### Work unit E — replace E2E evidence

Real OpenCode journeys must prove:

1. small direct implementation;
2. delegated implementation;
3. optional SDD proposal and decline/accept paths;
4. documentation-only candidate with zero reviewer ceremony;
5. standard candidate with one review;
6. high-risk candidate with focused 4R;
7. one bounded correction;
8. direct-main and PR escape routes;
9. exact receipt reuse at later gates;
10. no URL, token, CA, agent env, daemon or fixture server.

### Work unit F — documentation and release

- Update README, intended usage, trigger rules and threat model.
- Mark the prior Organic Recovery plan superseded.
- Correct issue #1794 and PR #1801.
- Publish the final 241-issue/92-PR disposition and regression ledger.
- Run the exact-head gate matrix.
- Run one final focused dual Judgment Day review.
- Merge and publish Gentle AI before adapting Gentle Pi.

## 12. Required traceability artifacts

The implementation is not ready for final validation until it produces these
machine-readable or mechanically checkable views:

1. **Snapshot ledger:** the existing 241 issues and 92 PRs, exactly once.
2. **Change ledger:** every commit and changed file on the recovery branch with
   one disposition.
3. **Invariant ledger:** every generic fix and historical failure class mapped
   to one owning package and at least one proof.
4. **Contribution ledger:** every reused or decomposed community PR linked to
   the retained implementation and credited author.
5. **Test ledger:** every retained historical failure class linked to its unit,
   integration, cross-OS or real-agent test.
6. **Deletion ledger:** every deleted public command, contract, environment
   variable, fixture and test with the reason no retained invariant depends on
   it.
7. **Release ledger:** every issue/PR classified after the released exact SHA
   as `close`, `superseded`, `partially covered`, `still valid` or `needs
   reproduction`.

Required reconciliation checks:

- 241/241 issues appear exactly once;
- 92/92 snapshot PRs appear exactly once;
- all 16 cross-context PRs have an explicit decomposition/credit map;
- every branch commit and dirty file appears exactly once;
- every `DELETE` item has zero uncovered invariants;
- every `KEEP`, `TRANSPLANT` or `REWRITE` safety invariant has a regression
  proof;
- generated assets and fixtures name one canonical source and reproduce
  without unexplained diff.

## 13. Tests to delete versus retain

### Delete

- HTTPS transport/bootstrap/session tests;
- operator environment configuration matrices;
- dormant-by-default product tests;
- TLS server fixtures;
- remote policy/operation simulations;
- universal WorkRun tests that exist only because normal implementation was
  wrapped before it began;
- outdated E2E that proves schemas rather than user journeys.

### Retain

- candidate identity and tamper rejection;
- Git common-dir/worktree correctness;
- immutable publication and exact replay;
- correction budget and scope enforcement;
- receipt/gate content binding;
- crash recovery around actual local receipt mutations;
- Windows filesystem and permission behavior;
- exact-SHA CI and release provenance;
- real-agent journeys.

Tests are selected by invariant, not by package ancestry. A WorkRun test may be
deleted while its generic concurrent-edit or receipt-replay case is transplanted
to `MMI` or `RAR`. Conversely, a test that only proves a remote session,
credential bootstrap or dormant negotiation is removed even if it is large.

## 14. Validation matrix

### 14.1 Structural and traceability validation

- Reconcile the snapshot, change, invariant, contribution, test, deletion and
  release ledgers.
- Prove no historical item or current branch artifact disappears between
  ledgers.
- Prove the dependency graph remains one-way and no replacement god object is
  introduced.
- Prove no deleted runtime contract remains referenced by CLI help, assets,
  schemas, tests or documentation.
- Regenerate and diff all derived contracts, goldens and adapter assets.

### 14.2 Focused owner validation

- `HCR`: unit/race tests plus Linux, macOS and Windows command/install fixtures.
- `MMI`: adversarial path, link, mode/ACL, concurrent-edit, crash and rollback
  tests.
- `ACI`/`MCA`: registry bootstrap, alias collision, capability truthfulness,
  deterministic rendering and all-adapter semantic parity.
- `RAR`/`EPD`: candidate tamper, immutable admission, policy ordering, one
  correction, replay, corruption and receipt gate tests.
- `DSR`: dry-run convergence, idempotent second apply, unknown ownership,
  migration and uninstall symmetry.
- `SDD`: direct/delegated isolation, optional proposal, durable resume, artifact
  and receipt handoff tests.
- `PAD`: every delivery escape route, exact-SHA CI and release provenance.

### 14.3 Real product journeys

On a clean install, without productive-runtime environment variables or a
server fixture, prove:

1. OpenCode receives a small request and completes it directly.
2. OpenCode delegates a research/broad request.
3. OpenCode proposes SDD only for suitable complexity; accept and decline both
   work.
4. A docs-only candidate receives zero AI review.
5. A standard candidate receives exactly one consolidated review.
6. A high-risk candidate receives focused 4R.
7. A confirmed blocker authorizes one correction and cannot loop.
8. Expensive verification asks once; resume reuses the exact frozen plan.
9. Impossible verification stops with an explicit evidence gap.
10. Direct main, PR without issue and PR-with-issue delivery all validate the
    same receipt.
11. Candidate tampering after review is rejected at every delivery gate.
12. The same installed configuration works for every supported adapter through
    its native transport, without a global runtime-agent selector.

### 14.4 Exact-candidate repository gates

Run against one frozen SHA and require the tree to remain unchanged:

```bash
git diff --check
go run ./internal/gofmtcheck
go vet ./...
go test ./... -count=1 -timeout=30m
go test -race ./... -count=1 -timeout=30m
actionlint
```

Also parse every JSON schema/fixture, run Windows cross-compilation and the
repository's supported cross-OS E2E matrix, verify no untracked Go files are
omitted, push once, and require CI to report against the exact same SHA.

### 14.5 GitHub closure validation

No issue or PR is closed merely because the architecture or replacement code
exists. Closure happens only after the released exact SHA:

1. satisfies the mapped invariant and regression proof;
2. has been exercised on the relevant operating system/agent;
3. preserves contributor attribution;
4. contains no still-valid independent behavior from the original item.

Items only partially covered remain open or receive a smaller replacement
issue. Backlog cleanup happens once, after Gentle AI and its Gentle Pi
integration are released and audited.

## 15. Acceptance criteria

- A user opens OpenCode and asks for a feature without configuring Gentle AI
  runtime variables.
- The configured agent chooses direct, delegated or optional SDD organically.
- No Gentle AI command runs merely to authenticate another runtime before
  implementation.
- RDD begins only when a candidate exists.
- Docs-only work creates no AI review or fake runtime verification.
- Standard work uses at most one review.
- 4R is restricted to demonstrated high-risk boundaries.
- No review or verification loop can repeat indefinitely.
- One candidate receives at most one correction transaction.
- The same content-bound receipt validates every delivery gate.
- Direct main, PR without issue and other explicit escape routes remain usable.
- No remote-runtime code, credential setup or speculative enterprise contract
  remains.
- All 241 issues, 92 snapshot PRs, 16 cross-context PRs, branch commits and
  dirty files have an auditable final disposition.
- Every generic safety fix and historical edge case is either retained or
  transplanted with proof before its old implementation is removed.
- No GitHub item is closed before released evidence justifies its exact
  disposition.

## 16. Estimated change size

No day estimates are used.

| Area | Estimated touched lines |
|---|---:|
| Delete connector/runtime/capabilities path | 4,000–6,500 |
| Delete or rewrite universal WorkRun tests/assets | 2,500–4,000 |
| Simplify proportional RDD policy and recovery | 500–900 |
| Replace E2E journeys | 700–1,200 |
| Traceability ledgers and documentation | 700–1,200 |
| **Total estimated touched lines** | **8,400–13,800** |

The expected net result is substantially fewer production lines and fewer
public concepts than the current branch.

## 17. Final execution order

1. Approve this replacement plan.
2. Freeze and reconcile all 241 issues, 92 PRs, 16 cross-context PRs, branch
   commits and dirty files.
3. Extract and test every generic invariant currently coupled to the rejected
   runtime path.
4. Remove the speculative runtime path.
5. Implement proportional post-candidate RDD.
6. Delete outdated tests and replace them with invariant and real-journey
   proofs.
7. Run focused normal/race/cross-OS gates.
8. Publish traceability ledgers and update docs, issue and PR against the
   frozen code SHA.
9. Run full repository gates.
10. Push the exact candidate once.
11. Run one final focused dual Judgment Day review.
12. Merge and release Gentle AI.
13. Adapt Gentle Pi to the released local receipt contract.
14. Audit the released behavior against the ledger.
15. Perform one final backlog cleanup after both releases.
