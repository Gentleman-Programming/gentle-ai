# Delta for review-findings-ledger

## ADDED Requirements

### Requirement: Compact recovery edge admission

A recovery successor MUST reach `validateCompactRecoveryEdge` for semantic admission instead of being rejected earlier by a CLI pre-gate that ties the requested flag shape to the predecessor's target kind. Release-scope recovery MUST accept an ordinary one-parent delivery commit as HEAD, not only a two-parent merge commit.

Re-targeting across target kinds is a deliberate degree of freedom in recovery, not a gap: recovery exists precisely to move an authority to a different target. The boundary is therefore held by identity-bound authority rather than by kind matching, and every one of these protections MUST remain intact. Escalated and invalidated recovery MUST keep requiring their exact predecessor state and a maintainer authorization bound to the predecessor lineage, the predecessor revision, and the successor's snapshot identity. Approved scope-changed recovery MUST keep requiring a substantive scope change. A base-diff predecessor MUST keep requiring the successor snapshot to retain its frozen base tree. The argv-coherence rule pairing `--base-ref` with `--committed-only` MUST remain, so a base-diff target can never be built without a base reference. Above all, a recovery successor MUST NOT inherit its predecessor's approval: it begins a fresh review that must earn its own receipt.

#### Scenario: Compatible predecessor reaches semantic admission (1744)

- GIVEN a recovery successor whose predecessor kind is compatible with base-diff recovery
- WHEN the CLI evaluates `--base-ref`/`--committed-only` flag shape
- THEN the CLI pre-gate does not reject it on flag shape alone
- AND `validateCompactRecoveryEdge` makes the semantic admission decision

#### Scenario: Release-scope recovery accepts a one-parent delivery commit (1816)

- GIVEN HEAD is an ordinary one-parent squash or rebase delivery commit
- WHEN release-scope recovery builds its snapshot
- THEN the one-parent commit is accepted as a valid release-scope HEAD
- AND recovery does not require a two-parent merge commit

#### Scenario: Escalated recovery without bound maintainer authorization is still rejected (negative)

- GIVEN an escalated recovery successor whose maintainer authorization does not bind the predecessor lineage, predecessor revision, and successor snapshot identity exactly
- WHEN `validateCompactRecoveryEdge` evaluates it
- THEN the successor is rejected
- AND no recovery edge is admitted, regardless of the requested target kind

#### Scenario: Invalidated recovery still requires an invalidated predecessor (negative)

- GIVEN a recovery successor requesting the invalidated disposition over a healthy predecessor
- WHEN `validateCompactRecoveryEdge` evaluates it
- THEN the successor is rejected

#### Scenario: A base-diff predecessor still binds its frozen base tree (negative)

- GIVEN a base-diff predecessor and a successor snapshot whose base tree differs from the predecessor's frozen base tree
- WHEN recovery builds the successor
- THEN the recovery is rejected

#### Scenario: A recovery successor never inherits approval

- GIVEN any admitted recovery successor
- WHEN its lifecycle state is inspected
- THEN it begins a fresh review that must earn its own receipt
- AND no gate treats the predecessor's approval as governing the successor's candidate

### Requirement: Start-scoped operation deadline

The `review/start` operation MUST use its own deadline, sized for real candidates and distinct from the shared facade operation deadline, so that a valid large workspace completes the operation instead of being cut off mid-sweep.

#### Scenario: Large valid workspace completes START (1778)

- GIVEN a valid workspace whose candidate construction exceeds the shared 25-second facade deadline
- WHEN `review/start` runs
- THEN it completes under its own start-scoped deadline
- AND it does not require a raised shared facade deadline or a configurable timeout framework

### Requirement: Per-issue E2E traceability and typed escape naming

Every one of the eighteen review-lifecycle-hardening defects MUST have at least one named end-to-end scenario in `e2e/organicruntime` whose test name carries that issue's number. Every typed refusal introduced by this hardening MUST name the concrete escape command verbatim in its denial message, never only a prose description of the escape.

#### Scenario: Each issue has a named, numbered E2E scenario

- GIVEN the eighteen defects in scope for this hardening
- WHEN the `e2e/organicruntime` suite is inspected
- THEN each issue has exactly one subtest whose name carries that issue's number
- AND a shared per-group journey may set up the fixture reused by its subtests

#### Scenario: Typed refusal names the exact escape command

- GIVEN a typed refusal introduced by this hardening fires
- WHEN its denial message is inspected
- THEN the message names the concrete escape command verbatim
- AND it does not rely on a prose description the caller must translate into a command

## MODIFIED Requirements

### Requirement: Deterministic lifecycle validation

Pre-commit, pre-push, pre-PR, and release MUST validate the same receipt and return `allow | scope-changed | invalidated | escalated`. Native validation MUST derive the current repository target and hash persisted policy, ledger, fix delta, verify evidence, and release artifacts instead of trusting caller-authored tree/hash assertions. Fabricated or nonexistent objects and stale artifacts MUST fail closed. Only `allow` returns process success; every denial still emits parseable machine JSON and returns non-success. Gates MUST NOT start a reviewer, create another budget, or silently start Judgment Day.

Pre-push discovery of a candidate whose reviewed delivery base was already published MUST classify with the existing `scope-changed` family, never a new terminal code and never the corruption catch-all. With reviews disabled this MUST still report `disabled/unmanaged` and exit success. With reviews enabled it MUST be a typed, evidence-bearing scope-changed denial. Untyped push-target failures — multiple merge bases, an unconfigured push remote — MUST classify as a typed target-resolution failure, distinct from authority corruption.

Pre-PR receipt-chain selection MUST NOT report a chain convergence when a historical approved receipt merely ends at the current publication base, because that shape is a linear chain rather than two chains meeting. A genuine convergence inside the publication range MUST still be reported.

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

#### Scenario: Published delivery reclassifies as scope-changed, disabled mode (pre-push published delivery)

- GIVEN a candidate whose reviewed delivery was already published
- AND reviews are disabled for the repository
- WHEN pre-push discovery evaluates the candidate
- THEN the result reports `disabled/unmanaged`
- AND the process exits success

#### Scenario: Published delivery reclassifies as scope-changed, enabled mode (pre-push published delivery)

- GIVEN a candidate whose reviewed delivery was already published
- AND reviews are enabled for the repository
- WHEN pre-push discovery evaluates the candidate
- THEN the result is a typed, evidence-bearing `scope-changed` denial
- AND it is not classified as authority corruption

#### Scenario: Untyped push-target ambiguity types as target-resolution failure (1699-adjacent target typing)

- GIVEN pre-push assessment encounters multiple merge bases or an unconfigured push remote
- WHEN the gate classifies the assessment failure
- THEN it is typed as a target-resolution failure
- AND it is not folded into the authority-corrupted catch-all

#### Scenario: Receipt chain ending at the publication base is not a convergence (1782)

- GIVEN a historical approved receipt whose chain ends exactly at the current publication base
- AND later approved receipts form a linear chain above that base
- WHEN the pre-PR gate selects the receipt chain for the publication range
- THEN the historical receipt does not make the selection report a chain convergence
- AND the gate evaluates the linear chain instead of demanding a full-range re-review

#### Scenario: A real chain convergence is still rejected (regression, negative)

- GIVEN two receipt chains genuinely converge on the same tree inside the publication range
- WHEN the pre-PR gate selects the receipt chain
- THEN the convergence is still reported
- AND the gate does not silently pick one branch

#### Scenario: Pre-push before push still allows (regression)

- GIVEN an approved receipt for a candidate not yet pushed
- WHEN pre-push runs before the push
- THEN the result is `allow`
- AND no reclassification changes this outcome

#### Scenario: Committed-only review stays unaffected (regression)

- GIVEN a `--committed-only` review target
- WHEN pre-push discovery and validation run
- THEN behavior is unchanged by this hardening
- AND the receipt validates exactly as before

#### Scenario: Governing receipt still allows (regression)

- GIVEN a governing receipt that already covers the current target
- WHEN a lifecycle gate validates it
- THEN the result remains `allow`
- AND no reviewer or budget reopens

#### Scenario: Disabled/unmanaged discovery codes stay byte-identical (regression)

- GIVEN the existing disabled/unmanaged discovery code paths unrelated to published-delivery classification
- WHEN pre-push, pre-commit, pre-PR, or release discovery runs under disabled mode
- THEN the emitted codes and messages remain byte-identical to their pre-hardening form

#### Scenario: Genuinely corrupted or ambiguous authority still fails closed (regression, negative)

- GIVEN authority is genuinely malformed, hash-mismatched, or ambiguous for reasons other than published delivery or typed target-resolution failures
- WHEN discovery or validation evaluates it
- THEN the result still fails closed as authority-corrupted or ambiguous
- AND neither reclassification introduced by this hardening weakens that outcome

### Requirement: One ordinary correction transaction

Ordinary 4R MUST permit at most one correction transaction. The correction MAY contain multiple atomic work units, but every unit MUST map to frozen corroborated IDs and record focused-test evidence, runtime evidence or justified N/A, and an independent rollback boundary. Candidate-causal admission MUST canonicalize both the frozen/verified finding IDs and the submitted candidate-causal finding IDs before comparing them, so an omitted canonicalization step on the submitted side MUST NOT produce a misleading out-of-scope admission decision for IDs that are semantically identical but differently ordered or formatted.

#### Scenario: Atomic work units share one budget

- GIVEN three independent fixes address frozen IDs
- WHEN the parent launches their correction transaction
- THEN the native fix counter increases once
- AND work-unit count cannot create another correction budget

#### Scenario: Differently formatted candidate-causal IDs still admit (1699)

- GIVEN frozen verified finding IDs and submitted candidate-causal finding IDs that are semantically equal but not identically canonicalized or ordered
- WHEN artifact admission compares them
- THEN both sides are canonicalized before matching
- AND admission is not misclassified `out_of_scope`

### Requirement: Native machine-readable entry points

The CLI MUST expose `review-start` for current-changes, base-diff, exact commit/range, and ledger-bound fix-diff target creation; `review-resume` for authoritative output recovery; `review-bundle-export` and `review-bundle-import` for explicit portable recovery; `review-validate` for lifecycle receipt validation; `review next-transition` to drive the lifecycle forward; and `review schema` to publish every accepted input payload contract. Base/range/revision arguments are target-specific; fix-diff ledger IDs are repeatable and comma-safe. Every command MUST emit machine-readable JSON. `review-start`, resume, export, import, and validation MUST derive authority from `--cwd` plus validated lineage; none accepts a caller-selected authoritative store. Optional `--machine-transaction-out` is explicitly non-authoritative and cannot reset consumed state.

Every emitted transition argument MUST be literally executable as printed, including boolean flags rendered in their exact executable flag form. `review schema` MUST publish the verification-evidence payload contract alongside every other accepted input schema. When a finalize call is lineage-only and canonically captured verification evidence already exists for that lineage, finalize MUST consume that evidence instead of returning a no-op continuation. Repeating a finalize whose results were already consumed MUST return a typed already-consumed rejection and MUST NOT report a silent success. Every operation-integration failure envelope MUST carry the wrapped real native cause instead of a fixed placeholder message. `review next-transition` routing MUST surface the escalate-to-recover continuation when it applies, including for an already-edited over-budget correction.

#### Scenario: Native start preserves index

- GIVEN `gentle-ai review-start` receives an intended-untracked manifest
- WHEN it writes transaction output
- THEN the transaction is strict-parseable
- AND the user's real Git index is byte-identical

#### Scenario: Boolean transition arguments are literally executable (1745)

- GIVEN `review next-transition` emits an `Execute` transition with a boolean argument
- WHEN the argument is rendered
- THEN it is printed in its exact executable flag form
- AND running the printed argument literally succeeds without reformatting

#### Scenario: review schema publishes the verification-evidence contract (1775)

- GIVEN `gentle-ai review schema` is invoked
- WHEN the input schema catalog is emitted
- THEN it includes the `gentle-ai.review-verification-evidence/v1` payload contract
- AND the usage/error text also names it

#### Scenario: Lineage-only finalize consumes captured evidence (1663)

- GIVEN a lineage-only finalize call
- AND canonically captured verification evidence already exists for that lineage
- WHEN finalize runs
- THEN it consumes the captured evidence
- AND it does not return a no-op continuation

#### Scenario: Repeated finalize on already-consumed results is rejected (1788)

- GIVEN a finalize call whose results were already consumed
- WHEN the same finalize is repeated
- THEN it returns a typed already-consumed rejection
- AND it does not report a silent success

#### Scenario: Operation failure envelope carries the wrapped cause (1666, 1807)

- GIVEN an operation-integration call fails for a specific native reason
- WHEN the `operation_outcome_unknown` envelope is emitted
- THEN it carries the wrapped real native cause
- AND it does not discard the cause behind a fixed placeholder message

#### Scenario: Escalate-to-recover continuation is surfaced for an over-budget correction (1800)

- GIVEN an already-edited over-budget correction has escalated
- WHEN `review next-transition` computes the next step
- THEN it surfaces the escalate-to-recover continuation
- AND the caller is not left without an in-product next action

### Requirement: Exact persistence references

All artifact modes MUST use the repository-derived authoritative append-only CAS store at `<git-common-dir>/gentle-ai/review-transactions/v1/{lineage-id}/`. The lineage ID MUST be canonical lowercase kebab-case and MUST NOT permit path traversal or aliases. OpenSpec `transaction.json`, frozen `ledger.json`, `receipt.json`, `chain-bundle.json`, and `gate-context.json` artifacts and Engram `sdd/{change-name}/review/{transaction,ledger,receipt,chain-bundle,gate-context}` topics are non-authoritative mirrors. Each append MUST require the expected revision and one legal semantic successor. Each load MUST prove the complete content-addressed predecessor chain from exact HEAD to one valid `review/start` genesis, rejecting missing predecessors, cycles, hash or schema mismatches, immutable-field changes, illegal or reordered transitions, semantically incomplete finding routing, and incoherent counters. Archive readiness MUST cross-check the trusted terminal revision and complete chain against the transaction mirror, frozen ledger, verify evidence, receipt, portable bundle, gate identity, and current repository target; missing, stale, or mismatched artifacts block archive.

Store discovery MUST enumerate only entries that carry a review-state document. A TERMINAL lineage that fails semantic validation MUST be excluded from that enumeration alone, MUST surface a structured diagnostic in the store status diagnostics surface whenever status is queried, and MUST NOT block discovery, status, start, abandon, or reclaim for any other lineage. Excluded lineages MUST remain quarantined and auditable; discovery MUST NOT silently drop them without a diagnostic, and MUST NOT repeat the diagnostic as a stderr warning on every unrelated operation.

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

#### Scenario: One invalid terminal lineage is quarantined, not store-wide (1813)

- GIVEN one TERMINAL lineage fails semantic validation among several healthy lineages
- WHEN store discovery enumerates the compact store
- THEN only the invalid lineage is excluded
- AND status, start, abandon, and reclaim remain operable for every other healthy lineage

#### Scenario: Excluded lineage diagnostic is always surfaced (negative)

- GIVEN a TERMINAL lineage was excluded for failing semantic validation
- WHEN store status is queried
- THEN a structured diagnostic entry names the excluded lineage
- AND the exclusion is never a silent skip and never a repeated stderr warning on every unrelated operation

### Requirement: Crash-safe bounded writer ownership

Authoritative writes MUST use a non-blocking cross-platform lock mechanism that is released by the operating system on process death and records actionable owner token, PID, host, and acquisition time. A live owner MUST NOT be stolen. Corrupt or stale owner bytes MUST recover after exclusive acquisition. Concurrent recoverers MUST NOT both win, and contention MUST NOT wait without a bound.

The lock acquisition walk MUST anchor at the repository's Git common directory rather than the filesystem root. A failure occurring before the lock is acquired MUST report as not-started rather than as an unknown-mutation outcome. Immutable publication MUST fall back to an exclusive-create-then-rename sequence when the platform rename syscall reports it is unsupported (`ENOTSUP`), instead of failing the publication outright.

#### Scenario: Writer crashes while holding the store

- GIVEN a writer exits while its lock owner record remains
- WHEN another writer attempts one append
- THEN operating-system ownership has already been released
- AND exactly one new writer acquires and overwrites the stale owner record
- AND a simultaneously live owner still produces immediate actionable contention

#### Scenario: Lock walk anchors at the repository common directory (1781)

- GIVEN a secure lock acquisition walk begins
- WHEN it resolves its anchor
- THEN it anchors at the repository's Git common directory
- AND it does not walk from the filesystem root

#### Scenario: Pre-lock failure reports not-started (1781)

- GIVEN a failure occurs before the lock is acquired
- WHEN the failure is reported
- THEN it reports as not-started
- AND it is not reported as an unknown-mutation outcome

#### Scenario: ENOTSUP rename falls back to exclusive-create and rename (1804)

- GIVEN the platform rename syscall reports `ENOTSUP` during immutable publication
- WHEN publication proceeds
- THEN it falls back to an exclusive-create-then-rename sequence
- AND publication still succeeds instead of failing outright

### Requirement: Complete current-changes snapshot

The current-changes target MUST combine tracked staged, unstaged, and deleted state with an explicit intended-untracked path list through a temporary Git index. It MUST NOT mutate the user's real index. Every Git subprocess used for repository identity, snapshot, store, gate, archive, release, or bundle derivation MUST use canonical explicit `git -C <repo>` input and MUST remove inherited repository/worktree/common-dir/index/object/alternate/namespace/shallow/graft/replacement/discovery overrides while preserving ordinary credentials and safe Git configuration.

A staged projection combined with an explicit base ref MUST be refused with a typed error that names the plain staged-projection escape verbatim, rather than silently proceeding without the index-freezing semantics the combination implies. Selector-free status evaluated on an unborn HEAD MUST resolve to the empty-tree projection instead of failing with a raw Git command error.

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

#### Scenario: Staged projection with base ref is refused by name (1812)

- GIVEN a caller combines `--projection staged` with an explicit `--base-ref` and no workspace overlay
- WHEN `review-start` evaluates the target
- THEN it is refused with a typed error
- AND the error names the plain staged-projection escape verbatim

#### Scenario: Selector-free status on unborn HEAD resolves to empty-tree projection (1771)

- GIVEN an unborn HEAD and a selector-free status call
- WHEN the target is resolved
- THEN it resolves to the empty-tree projection
- AND it does not fail with a raw Git command error

### Requirement: Terminal receipt

Only `approved | escalated` are terminal transaction states. An approved receipt MUST bind `lineage_id`, mode, generation, base tree, `initial_review_tree`, `final_candidate_tree`, `paths_digest`, `fix_delta_hash`, policy hash, ledger hash, evidence hash, counters, and terminal state. A release-bound receipt MUST additionally bind immutable release tree, configuration hash, generated-artifact hash, provenance/signing hash, publication-boundary hash and sealed state, plus evidence-freshness hash and current state.

First publication attempted from an empty-base receipt MUST be refused with a typed error that names the proven in-product escape verbatim — commit an authorized empty root, then run committed base-diff review — rather than failing silently or with a generic error.

#### Scenario: Post-fix receipt distinguishes trees

- GIVEN ordinary review corrected the initial candidate
- WHEN the receipt is emitted
- THEN `initial_review_tree` identifies what the lenses reviewed
- AND `final_candidate_tree` identifies the scoped-validated candidate
- AND `fix_delta_hash` identifies only their correction delta

#### Scenario: Empty-base first publication is refused by name (1641)

- GIVEN a first publication attempt derives from an empty-base receipt
- WHEN publication is evaluated
- THEN it is refused with a typed error
- AND the error names the proven in-product escape verbatim
