# Delta for Review Findings Ledger

## ADDED Requirements

### Requirement: Self-Derived Recovery Authorization

Every authorization-gated review command MUST be able to emit its own required `--maintainer-authorization` binding value, generalizing the existing `--prepare`/`--preflight` pattern to all ten gated commands. For deterministic recovery shapes already proven legal, the facade's routing layer MUST auto-derive and apply that binding without operator input, stamping `actor` from the repository Git identity and a machine-generated reason into the same CAS audit entry shape used today.

#### Scenario: Gated command self-emits its binding

- GIVEN any of the ten `--maintainer-authorization`-gated commands
- WHEN the operator requests the binding via its emitter flag
- THEN the command prints the exact deterministic binding value the facade will accept

#### Scenario: Facade applies binding for a deterministic recovery shape

- GIVEN a review reaches one of the deterministic recovery shapes already proven legal
- WHEN the facade routes that transition
- THEN it derives and applies the required binding automatically
- AND it stamps `actor` from Git identity and a machine-generated reason into the CAS audit trail
- AND no operator argv is required

### Requirement: Fail-Closed Recovery Boundaries

Self-derived recovery MUST NOT weaken authorization refusal behavior. An explicitly supplied wrong binding, corrupted or ambiguous authority, an ACTIVE attempt, and a failed-criteria escalation MUST each continue to refuse exactly as before self-derivation existed. A no-drift elective ledger reset MUST still refuse under the existing drift gate.

#### Scenario: Explicit wrong binding refuses

- GIVEN an operator supplies an explicit but incorrect `--maintainer-authorization` value
- WHEN the command validates it
- THEN the command refuses
- AND self-derivation does not substitute a correct value on the operator's behalf

#### Scenario: Corrupted or ambiguous authority refuses

- GIVEN the authoritative store is corrupted or its authority is ambiguous
- WHEN a recovery is attempted
- THEN the command refuses
- AND no auto-derived binding proceeds

#### Scenario: Active attempt is never auto-reset

- GIVEN a transaction is in an ACTIVE attempt state
- WHEN the routing layer evaluates auto-recovery
- THEN the ACTIVE attempt is not reset automatically

#### Scenario: Failed-criteria escalation never auto-recovers

- GIVEN a transaction escalated on failed criteria
- WHEN the routing layer evaluates auto-recovery
- THEN escalation stands and no automatic recovery is applied

#### Scenario: No-drift elective reset still refuses

- GIVEN an elective ledger reset with no drift present
- WHEN the drift gate evaluates the reset
- THEN the reset refuses exactly as before self-derivation

### Requirement: No-Stop-With-Successor Invariant

No reachable status or transition emission MUST report `stop` while a legal native continuation edge exists for that exact state. This invariant MUST be asserted structurally by test over the emission surface. Every newly discovered unrouted stop MUST be fixed by routing, and MUST NOT be resolved by a test exemption.

#### Scenario: Structural test asserts the invariant

- GIVEN the complete set of reachable native states
- WHEN the invariant test enumerates each state's legal continuation edges
- THEN no state with a legal continuation edge emits `stop`

#### Scenario: Newly discovered unrouted stop is fixed, not exempted

- GIVEN the invariant test discovers a new unrouted `stop`
- WHEN the finding is triaged
- THEN it is resolved by adding a routing edge
- AND it is never resolved by excluding the state from the test

### Requirement: Visible Correction and Budget Accounting

Every escalation and every budget-relevant surface MUST print spent, remaining, and total as distinctly labeled values. No state transition MUST be caused by a number that is absent from the surfaces the operator can see.

#### Scenario: Escalation prints full accounting

- GIVEN an escalation driven by `cumulative_correction_lines` or an equivalent budget
- WHEN the escalation is emitted
- THEN spent, remaining, and total each appear as distinct labeled values

#### Scenario: No invisible number drives a transition

- GIVEN any budget-relevant state transition
- WHEN the transition is triggered
- THEN the triggering number is already printed on a surface visible to the operator

### Requirement: Three-Tier Narration Contract

Every user-facing emission MUST classify into exactly one of Tier A (domain), Tier B (machinery), or Tier C (terminal).

Tier A splits by who owns the surface, and this requirement governs only the CLI's half. The CLI does not run lenses — the orchestrating agent does — so narrating the work in progress ("running the lenses", "a lens failed, here is the finding") belongs to the orchestrator prompt assets, and is specified in the `sdd-orchestrator-assets` delta, not here. The CLI's own Tier A surface is what it genuinely emits to a human: the consent prompt and the escalation accounting reason. Both MUST be narrated confidently in domain vocabulary and MUST obey the vocabulary bans below.

Tier B events (receipt/authority anomalies, recovery, retries, drift resets, quarantines) MUST NOT appear on the human surface; they MUST be recorded in the CAS trail and exposed via `--json`/verbose. When Tier B self-recovery succeeds, Tier-A narration MUST continue uninterrupted. Tier C states MUST produce exactly one statement naming the outcome in domain terms plus the single decision or command. Uncertainty or exploration phrasing MUST NOT appear on any surface, including Tier C. Internal identifiers (lineage, ordinal, CAS, facade, receipt) MUST remain confined to negotiated/JSON envelopes.

#### Scenario: Tier B self-recovery is invisible on the human surface

- GIVEN a machinery event self-recovers successfully
- WHEN the human surface renders the run
- THEN zero Tier-B output appears
- AND the CAS trail and `--json`/verbose still record the event

#### Scenario: The CLI's own Tier A surface stays in domain vocabulary

- GIVEN the consent prompt or the escalation accounting reason is emitted to a human
- WHEN its text is inspected
- THEN it speaks in domain terms
- AND it carries no internal identifier and no uncertainty phrasing

#### Scenario: Tier C terminal state emits exactly one statement

- GIVEN a state genuinely needs a human decision
- WHEN the human surface renders it
- THEN exactly one statement names the outcome in domain terms and the single decision or command

#### Scenario: Uncertainty phrasing is banned everywhere

- GIVEN any human-facing surface, including Tier C
- WHEN its text is inspected
- THEN no uncertainty or exploration phrasing appears

#### Scenario: Internal identifiers stay out of the human surface

- GIVEN lineage, ordinal, CAS, facade, or receipt identifiers
- WHEN the human surface renders any tier
- THEN those identifiers appear only in negotiated/JSON envelopes, never on the human surface

#### Scenario: Paired recoverable-vs-terminal machinery failure

- GIVEN one underlying machinery failure occurs
- WHEN the recoverable variant self-recovers
- THEN the human surface shows uninterrupted Tier-A narration with zero Tier-B output
- WHEN the same failure is instead terminal
- THEN the human surface shows exactly one Tier-C decision statement

### Requirement: Single Human Consent Ceremony

The one-time consent prompt MUST remain the only human ceremony on the happy path. A happy-path run from request to receipt MUST show zero authorization tokens and zero prompts beyond that single consent prompt.

#### Scenario: Happy path shows one prompt and zero tokens

- GIVEN a run proceeds from request through receipt without a recovery escalation
- WHEN the operator observes the session
- THEN exactly one human prompt (RDD consent) appears
- AND zero authorization tokens are typed

### Requirement: In-Band Recovery Discoverability

Every blocking state whose recovery exists MUST name its continuation in the block itself: the machine envelope's `next_action` (or denial context) MUST carry the operation that continues the flow, and the human Tier-C statement MUST carry the decision or command. A block whose only route lives in documentation is not discoverable. A state with no recovery MUST be classified terminal with its unblocking precondition stated, and the terminal classification MUST agree with the published continuation documentation — a code the docs prove continuable MUST NOT be pinned terminal.

#### Scenario: Gate before any review names the way in

- GIVEN an agent runs a delivery gate in a repository with no governing review
- WHEN the receipt-missing block is emitted
- THEN the envelope's next_action names the start operation
- AND the agent can continue without consulting documentation

#### Scenario: Ledger refusal points at the envelope that knows

- GIVEN an attempt-ledger operation refuses (budget exhausted, active attempt, no active attempt)
- WHEN the CLI error is emitted
- THEN it names the status operation whose envelope carries the authoritative next_action

#### Scenario: Terminal classification cannot contradict the docs (negative)

- GIVEN a stop reason code whose documented continuation table row names a concrete command
- WHEN the stop-invariant classification is built
- THEN classifying that code as terminal fails the invariant test
