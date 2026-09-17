# sdd-native-runtime Specification

## Purpose

Define the existing native status, handoff, recovery, settlement, and managed-consumer behavior as future acceptance targets. These scenarios do not claim that any reported outcome has been reproduced or fixed.

## Requirements

### Requirement: Pure Native Status and Authoritative Action

The native runtime MUST return a read-only status projection without requiring launch, delivery, review, or archive authorization. The projection MUST identify the declared artifact store, planning location, intrinsic blocked reasons, action context, all seven declared dependencies (`proposal`, `specs`, `design`, `tasks`, `apply`, `verify`, `archive`), and the four instruction groups (`apply`, `verify`, `remediate`, `archive`). It MUST select at most one typed next action; status MUST NOT grant mutation authority.

#### Scenario: NR-01 status remains inspectable

- GIVEN a declared OpenSpec or Engram change has a condition that would refuse execution or delivery
- WHEN native status is requested
- THEN it returns the projection and intrinsic refusal facts without requiring that authorization
- AND it does not authorize a write

#### Scenario: NR-02 status exposes the complete declared contract

- GIVEN a native status projection is produced
- WHEN a supported consumer decodes it
- THEN every declared dependency and instruction group is available under the published contract
- AND no hard-coded subset is required

### Requirement: Governed Producer and Consumer Contract

A consumer MUST use the native-selected typed action and action context to decide its next step. It MUST NOT derive lifecycle transitions, readiness, mutation authority, or commands from prose. A consumer MUST reject a malformed, unknown, or unsupported action before phase execution and MUST NOT invent a command or substitute a different action.

#### Scenario: NR-03 consumer follows the selected action

- GIVEN status selects a supported typed action with action context
- WHEN the consumer prepares the next phase
- THEN it follows that action and context without recomputing native readiness

#### Scenario: NR-04 invalid action is refused

- GIVEN status contains an unknown, malformed, or unsupported action
- WHEN the consumer decodes the projection
- THEN it returns an explicit refusal before phase execution
- AND it does not infer a replacement action

### Requirement: Scoped Capability and Artifact Handoff

The native handoff MUST carry only the selected phase's declared capabilities, artifact-store identity, planning path, and already-authorized write scope. A managed child MUST receive the exact declared capability names and grants required by that phase. Missing capabilities, out-of-scope writes, or a mismatched worktree MUST refuse; the handoff MUST NOT create a second authority, silently broaden scope, or bypass consent.

#### Scenario: NR-05 authorized OpenSpec scope continues

- GIVEN a selected phase already has an authorized OpenSpec write scope
- WHEN a managed child receives the native handoff
- THEN it receives that same bounded scope, store identity, and planning path
- AND it can continue only within that scope

#### Scenario: NR-06 unavailable or unauthorized capability refuses

- GIVEN a required declared capability is absent, a write is outside the authorized scope, or the worktree differs
- WHEN the child prepares to execute
- THEN it refuses before the phase runs
- AND it does not substitute an undeclared capability or broaden authority

### Requirement: Corrected-Fact Recovery and Bounded Freshness

The native runtime MUST re-evaluate corrected facts through its existing recovery predicates rather than retain a stale blocker. Historical intended-untracked selections MAY reconcile only when replayed into a present candidate capture. Fresh caller-supplied selections and later candidate drift MUST remain strictly validated and MUST NOT gain a permanent exemption.

#### Scenario: NR-07 corrected fact continues through native recovery

- GIVEN a previously blocking fact has been corrected within the preserved subject and authority bounds
- WHEN status or recovery is requested
- THEN the native runtime re-evaluates the fact through its recovery predicates
- AND it selects or refuses the resulting authorized outcome without retaining the stale blocker

#### Scenario: NR-08 replay is narrow and fresh selection remains strict

- GIVEN a historical intended-untracked selection is replayed into a present candidate capture
- WHEN reconciliation is evaluated
- THEN it is reconciled only for that replay
- AND a fresh selection or later drift is validated without inheriting that reconciliation

### Requirement: Typed Failed-Evidence Remediation

Native remediation MUST be a bounded typed action distinct from apply and MUST retain the failed-evidence revision binding. A consumer without the exact remediation capability MUST return an explicit unsupported-capability result and MUST NOT execute apply. Successful change or remediation acceptance, including archive eligibility, MUST require fresh independent verification. Failed or interrupted remediation MUST remain eligible for truthful settlement under existing required facts; this MUST NOT add a verification gate.

#### Scenario: NR-09 unsupported remediation does not alias apply

- GIVEN status selects remediation bound to failed evidence
- WHEN the consumer lacks the exact remediation capability
- THEN it reports unsupported capability
- AND it does not run apply or invent a continuation command

#### Scenario: NR-10 successful remediation requires fresh independent verification

- GIVEN a capable consumer successfully completes selected remediation
- WHEN successful acceptance or archive eligibility is considered
- THEN the failed-evidence revision remains bound to the remediation
- AND fresh verification by an independent verifier is required

#### Scenario: NR-10a failed remediation settles truthfully

- GIVEN selected remediation fails or is interrupted
- WHEN the native runtime settles the attempt
- THEN it records the existing required settlement facts without independent-success verification
- AND it does not grant acceptance or archive eligibility

### Requirement: Real-Subject Finite Settlement and Replay

Each admitted attempt MUST retain its candidate, worktree, consent, evidence, and accounting bindings in the existing immutable common-directory authority. Passed, failed, interrupted, and no-diff outcomes MUST settle finitely and truthfully; no-diff execution MUST NOT be free by default. A lost reply after native settlement is committed MAY recover that existing result without re-execution or double accounting. An external effect before durable settlement MUST reconcile only supported facts; unknown effects MUST remain unknown, unsafe replay MUST refuse, and no committed record or success MAY be fabricated. Existing capped legitimate refunds MUST remain preserved.

#### Scenario: NR-11 terminal outcomes settle against the real subject

- GIVEN an admitted attempt reaches pass, failure, interruption, or no-diff completion
- WHEN it is settled
- THEN it records applicable real evidence, diagnosis, process, cleanup, and accounting facts once
- AND no-diff is not treated as automatically free

#### Scenario: NR-12 lost reply recovers committed settlement

- GIVEN native settlement was committed and its reply was lost
- WHEN the retained result is replayed or recovered
- THEN the existing committed result is returned without re-execution or double accounting

#### Scenario: NR-12a external effect before settlement remains uncertain

- GIVEN an external effect may have occurred before durable settlement
- WHEN supported facts are reconciled
- THEN unknown effects remain unknown and unsafe replay is refused
- AND no committed settlement record or success is fabricated

### Requirement: Strict TDD and Verifier Independence

Implementation acceptance MUST preserve RED, GREEN, TRIANGULATE, and REFACTOR progression. An implementer's claim or process success MUST NOT substitute for evidence-based verification by an independent verifier. These requirements MUST NOT add a redundant verification gate.

#### Scenario: NR-13 quality acceptance is independent

- GIVEN a change is implemented for a native-runtime scenario
- WHEN acceptance evidence is evaluated
- THEN strict TDD progression and independent verifier evidence are required
- AND implementer self-report alone is insufficient

### Requirement: Managed Consumer Uptake

Managed consumer assets and provisioning MUST expose the governed native contract and the exact declared child capabilities before phase execution. If uptake is incomplete, the managed child MUST fail with an actionable capability mismatch; decoder or self-report evidence alone MUST NOT establish uptake.

#### Scenario: NR-14 managed child proves contract uptake

- GIVEN a managed child is launched with its extension selection fixed
- WHEN a declared native capability is required
- THEN assets and provisioning make the governed contract available before phase execution
- AND acceptance includes an end-to-end managed-child boundary check

#### Scenario: NR-15 incomplete managed uptake fails safely

- GIVEN managed assets or provisioning do not supply a required declared capability
- WHEN the child begins the phase
- THEN it fails before phase execution with an actionable mismatch
- AND it does not bypass the missing capability
