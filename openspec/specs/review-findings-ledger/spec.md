# Bounded Review Transaction and Findings Ledger

## Purpose

Define one explicit, content-addressed implementation-review transaction shared by ordinary 4R and explicit Judgment Day. The contract bounds reviewer/refuter/correction work, preserves independent final verification, and emits a reusable lifecycle receipt.

## Requirements

### Requirement: Explicit immutable review operation

The system MUST start review only through `review/start(target)`. A reviewer MUST receive one complete immutable snapshot, run detached and read-only, return one result, and terminate.

#### Scenario: Review operation terminates

- GIVEN a complete target
- WHEN one selected lens finishes its exhaustive sweep
- THEN it returns one structured result and terminates
- AND it cannot edit, delegate, launch a correction, or start another reviewer

### Requirement: Complete current-changes snapshot

The current-changes target MUST combine tracked staged, unstaged, and deleted state with an explicit intended-untracked path list through a temporary Git index. It MUST NOT mutate the user's real index. Every Git subprocess used for repository identity, snapshot, store, gate, archive, release, or bundle derivation MUST use canonical explicit `git -C <repo>` input and MUST remove inherited repository/worktree/common-dir/index/object/alternate/namespace/shallow/graft/replacement/discovery overrides while preserving ordinary credentials and safe Git configuration.

#### Scenario: Intended new file is reviewed

- GIVEN an untracked path listed in the intended manifest
- WHEN native snapshot construction runs
- THEN the candidate tree and paths digest include that path and content
- AND the real index bytes remain unchanged

#### Scenario: Generated golden participates in identity

- GIVEN an affected generated golden
- WHEN authored changed-line risk and target identity are computed
- THEN the golden is excluded from the authored `>400` threshold
- AND it remains included in the candidate tree, paths digest, and receipt validation

#### Scenario: Hostile Git environment cannot redirect authority

- GIVEN valid repository A and hostile inherited selectors that point to repository B
- WHEN snapshot and authoritative-store derivation run for repository A
- THEN every Git subprocess uses repository A's canonical explicit input
- AND top-level, common-dir, index, object, and store authority cannot be redirected
- AND linked worktrees still resolve repository A's shared Git common directory

### Requirement: Initial findings freeze

Each selected lens MUST run exactly one initial sweep and emit neutral claims with `id`, `lens`, `location`, `severity`, `claim`, and `proof_refs`. `BLOCKER | CRITICAL` claims MUST additionally carry `evidence_class` and `causal_disposition`. Per-finding status is derived by the system into evidence outcomes and MUST NOT be emitted by a lens. Full 4R means four initial lens sweeps. Findings MUST freeze after merge.

#### Scenario: Full 4R selects four lenses once

- GIVEN explicit review/start risk classification is high
- WHEN the initial review runs
- THEN risk, readability, reliability, and resilience each run once
- AND no lens reopens the original diff after findings freeze

### Requirement: Evidence-routed severe findings

Only `BLOCKER | CRITICAL` findings enter evidence classification, refutation, correction, or escalation. `WARNING | SUGGESTION` findings remain frozen `info` and cannot consume budgets. Deterministic severe findings MUST become corroborated with proof and skip refutation. Every inferential severe finding from all selected lenses MUST enter exactly one detached transaction-wide refuter operation. Insufficient evidence or missing, malformed, or incomplete refuter output MUST consume the single refuter batch, mark every pending claim inconclusive, clear pending work, and terminally escalate without retry.

#### Scenario: Full 4R uses one refuter batch

- GIVEN four initial lenses emit multiple inferential severe claims
- WHEN evidence routing completes
- THEN exactly one refuter operation receives the complete merged inferential list
- AND it returns one `corroborated | refuted | inconclusive` result per claim

#### Scenario: Deterministic proof bypasses refuter

- GIVEN a failing command proves a severe defect
- WHEN evidence is classified
- THEN the finding becomes corroborated
- AND no refuter receives it

### Requirement: One ordinary correction transaction

Ordinary 4R MUST permit at most one correction transaction. The correction MAY contain multiple atomic work units, but every unit MUST map to frozen corroborated IDs and record focused-test evidence, runtime evidence or justified N/A, and an independent rollback boundary.

#### Scenario: Atomic work units share one budget

- GIVEN three independent fixes address frozen IDs
- WHEN the parent launches their correction transaction
- THEN the native fix counter increases once
- AND work-unit count cannot create another correction budget

### Requirement: One non-iterative ordinary scoped validator

When ordinary correction occurred, exactly one detached read-only scoped validator MUST receive only the frozen ledger and immutable fix delta. It MAY append fix-caused defects with proof. It MUST return only `approve | escalate` and MUST NOT launch another correction or validator.

#### Scenario: Fix-caused defect escalates

- GIVEN the correction introduced a defect on a fix-touched line
- WHEN scoped validation records the defect
- THEN the transaction becomes escalated
- AND no second ordinary correction starts

### Requirement: Explicit Judgment Day replacement

Judgment Day MUST be explicitly selected and MUST replace ordinary 4R for the target. Two distinct blind read-only judge executions and results plus explicit agreement/confirmation provide corroboration without `review-refuter`. Native state and receipts MUST bind their proof hashes and exactly-two execution counter before findings freeze or approval. Judgment Day MUST permit at most two fix rounds and two scoped re-judgments.

#### Scenario: Judgment Day cannot exceed round two

- GIVEN a severe issue remains after the second scoped re-judgment
- WHEN the parent evaluates native counters
- THEN the transaction becomes escalated
- AND no third round starts

### Requirement: Independent final verification

Exactly one independent requirements/runtime final verification MUST check actual requirement/scenario counts, task completion, current test/build evidence, ledger resolution, target identity, and counter coherence. A contradiction MUST escalate and MUST NOT start review/refuter/correction again.

#### Scenario: Final contradiction does not loop

- GIVEN scoped validation approved the fix delta
- WHEN runtime verification discovers a contradiction
- THEN final verification fails and the transaction escalates
- AND no ordinary review or correction budget is reopened

### Requirement: Semantic chain completeness

Every persisted successor MUST satisfy transaction state invariants in addition to an allowed state pair and monotonic counters. Frozen severe findings MUST remain immutable and retain exactly one legal classification and required outcome. Pending refuter IDs MUST clear only with one complete consumed batch. Corroborated IDs MUST equal correction IDs. Corrected final candidates MUST bind completed correction and required scoped-validation counters. `ready_final_verification`, `final_verifying`, and `approved` MUST contain no pending severe work and MUST have coherent mode-specific counters. `WARNING | SUGGESTION` rows remain non-blocking `info`.

#### Scenario: Hash-valid semantic bypass fails closed

- GIVEN a content-hash-valid chain that jumps from `findings_frozen` without classifying a severe finding
- OR clears a pending inferential finding without an outcome and consumed refuter batch
- WHEN the authoritative chain or portable bundle is loaded
- THEN semantic validation rejects the successor
- AND the chain cannot reach final verification or approval

### Requirement: Terminal receipt

Only `approved | escalated` are terminal transaction states. An approved receipt MUST bind `lineage_id`, mode, generation, base tree, `initial_review_tree`, `final_candidate_tree`, `paths_digest`, `fix_delta_hash`, policy hash, ledger hash, evidence hash, mode-specific counters, judge proof where applicable, and terminal state. A release-bound receipt MUST additionally bind immutable release tree, configuration hash, generated-artifact hash, provenance/signing hash, publication-boundary hash and sealed state, plus evidence-freshness hash and current state.

#### Scenario: Post-fix receipt distinguishes trees

- GIVEN ordinary review corrected the initial candidate
- WHEN the receipt is emitted
- THEN `initial_review_tree` identifies what the lenses reviewed
- AND `final_candidate_tree` identifies the scoped-validated candidate
- AND `fix_delta_hash` identifies only their correction delta

### Requirement: Deterministic lifecycle validation

Pre-commit, pre-push, pre-PR, and release MUST validate the same receipt and return `allow | scope-changed | invalidated | escalated`. Native validation MUST derive the current repository target and hash persisted policy, ledger, fix delta, verify evidence, and release artifacts instead of trusting caller-authored tree/hash assertions. Fabricated or nonexistent objects and stale artifacts MUST fail closed. Only `allow` returns process success; every denial still emits parseable machine JSON and returns non-success. Gates MUST NOT start a reviewer, create another budget, or silently start Judgment Day.

#### Scenario: Unchanged approved target is reused

- GIVEN the receipt and gate context match exactly
- WHEN a lifecycle validator runs
- THEN the result is `allow`
- AND zero reviewers run

#### Scenario: External evidence changes

- GIVEN unchanged reviewed code but new CI, vulnerability, base, policy, provenance, or release evidence
- WHEN the lifecycle validator evaluates it
- THEN the result may be `invalidated` or `escalated`
- AND unchanged code review is not reopened

### Requirement: Scope and incident boundaries

Unrelated content/path scope change MUST require a different lineage ID. No generation or gate may reuse an exhausted lineage budget. A separate operational incident transaction is valid only while code, configuration, generated-artifact, and provenance targets remain immutable.

#### Scenario: Incident mutates generated artifact

- GIVEN an operational release incident
- WHEN recovery changes a generated artifact
- THEN incident separation ends
- AND the content lineage must be validated explicitly

### Requirement: Exact persistence references

All artifact modes MUST use the repository-derived authoritative append-only CAS store at `<git-common-dir>/gentle-ai/review-transactions/v1/{lineage-id}/`. The lineage ID MUST be canonical lowercase kebab-case and MUST NOT permit path traversal or aliases. OpenSpec `transaction.json`, frozen `ledger.json`, `receipt.json`, `chain-bundle.json`, and `gate-context.json` artifacts and Engram `sdd/{change-name}/review/{transaction,ledger,receipt,chain-bundle,gate-context}` topics are non-authoritative mirrors. Each append MUST require the expected revision and one legal semantic successor. Each load MUST prove the complete content-addressed predecessor chain from exact HEAD to one valid `review/start` genesis, rejecting missing predecessors, cycles, hash or schema mismatches, immutable-field changes, illegal or reordered transitions, semantically incomplete finding routing, and incoherent counters. Archive readiness MUST cross-check the trusted terminal revision and complete chain against the transaction mirror, frozen ledger, verify evidence, receipt, portable bundle, gate identity, and current repository target; missing, stale, or mismatched artifacts block archive.

#### Scenario: Archive sees pending receipt

- GIVEN tasks and verification pass but the receipt is missing or non-terminal
- WHEN native SDD status evaluates archive
- THEN archive remains blocked with deterministic review action

#### Scenario: Caller supplies an approved alternate store

- GIVEN a caller-selected temporary store contains a hash-valid approved terminal event
- AND the repository-derived lineage store is missing, truncated, or non-terminal
- WHEN archive or a lifecycle gate validates the receipt
- THEN the result is machine-readable `invalidated`
- AND the alternate store cannot influence the authoritative decision

### Requirement: Crash-safe bounded writer ownership

Authoritative writes MUST use a non-blocking cross-platform lock mechanism that is released by the operating system on process death and records actionable owner token, PID, host, and acquisition time. A live owner MUST NOT be stolen. Corrupt or stale owner bytes MUST recover after exclusive acquisition. Concurrent recoverers MUST NOT both win, and contention MUST NOT wait without a bound.

#### Scenario: Writer crashes while holding the store

- GIVEN a writer exits while its lock owner record remains
- WHEN another writer attempts one append
- THEN operating-system ownership has already been released
- AND exactly one new writer acquires and overwrites the stale owner record
- AND a simultaneously live owner still produces immediate actionable contention

### Requirement: Idempotent append and output recovery

An exact append retry MUST repair an already-linked event whose HEAD update was interrupted. If HEAD already equals the exact new revision after machine-mirror or stdout failure, the identical retry MUST return that committed revision without changing counters. Different content and stale expected predecessors MUST remain rejected. Native `review-resume` MUST re-emit current authoritative state and regenerate a non-authoritative transaction mirror without running review work or resetting lineage.

#### Scenario: Output fails after authoritative commit

- GIVEN the exact event and HEAD committed before stdout or mirror output failed
- WHEN the caller retries exact append or runs `review-resume`
- THEN the same authoritative revision is returned
- AND no review, refuter, correction, or validation budget changes

### Requirement: Validated portable full-chain recovery

The system MUST export a content-addressed full-chain bundle containing exact ordered event payloads, genesis/head revisions, ordered chain identity, lineage/generation, initial/final snapshot identities, policy/ledger/evidence bindings, optional terminal receipt, and a domain-separated bundle digest. Import MUST be explicit and MUST derive the destination from the canonical repository. Before CAS installation it MUST validate bundle digest, every event hash, complete predecessor order, semantic transitions, terminal fix-diff semantics, terminal receipt, and gate-bound expected HEAD/genesis/chain/bundle identities. Recovery MUST use delivered-content equivalence rather than snapshot-structure equivalence: the current derived candidate tree and delivered path digest MUST equal `final_candidate_tree` and the receipt/lineage path scope; the authoritative intended-untracked proof MUST reproduce from that candidate; and policy, ledger, evidence, fix delta, lineage, generation, and mode MUST match their receipt and transaction bindings. The terminal fix-diff kind, base, ledger IDs, and identity MUST remain intact for audit, but the recovery snapshot MUST NOT be required to share its kind, base, or identity. Import MUST be idempotent only for the exact installed chain. Portable mirrors and alternate stores MUST NOT become authoritative without this validation.

#### Scenario: Clean clone imports a trusted expected chain

- GIVEN a clean clone has no local authoritative lineage store
- AND persisted receipt, gate context, artifacts, and bundle agree with the clone's delivered candidate, path scope, and authoritative intended-untracked proof
- AND the corrected terminal fix-diff snapshot remains structurally intact for audit
- WHEN explicit bundle import runs
- THEN the complete validated chain is CAS-installed only under that clone's repository-derived destination
- AND lifecycle validation may subsequently use the imported authoritative store

#### Scenario: Untrusted bundle fails closed

- GIVEN content or path scope is wrong, policy/ledger/evidence/fix delta differs, the chain is tampered or truncated, the lineage differs, fix-diff semantics are illegal, or receipt/gate bindings do not match
- WHEN import runs
- THEN no authoritative HEAD is installed
- AND lifecycle validation cannot silently import or trust it

### Requirement: Native machine-readable entry points

The CLI MUST expose `review-start` for current-changes, base-diff, exact commit/range, and ledger-bound fix-diff target creation; `review-resume` for authoritative output recovery; `review-bundle-export` and `review-bundle-import` for explicit portable recovery; and `review-validate` for lifecycle receipt validation. Base/range/revision arguments are target-specific; fix-diff ledger IDs are repeatable and comma-safe. Every command MUST emit machine-readable JSON. `review-start`, resume, export, import, and validation MUST derive authority from `--cwd` plus validated lineage; none accepts a caller-selected authoritative store. Optional `--machine-transaction-out` is explicitly non-authoritative and cannot reset consumed state.

#### Scenario: Native start preserves index

- GIVEN `gentle-ai review-start` receives an intended-untracked manifest
- WHEN it writes transaction output
- THEN the transaction is strict-parseable
- AND the user's real Git index is byte-identical

### Requirement: User-owned runtime selection

No bounded-review contract, reviewer, refuter, validator, or final verifier may enforce a model, provider, profile, or effort choice.

#### Scenario: Adapter renders review guidance

- GIVEN any supported AgentID
- WHEN its orchestrator guidance is rendered
- THEN runtime selection remains an optional user choice
- AND it is not an input to deterministic risk classification
