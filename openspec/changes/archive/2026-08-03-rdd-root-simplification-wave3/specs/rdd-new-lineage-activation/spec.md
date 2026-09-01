# RDD New-Lineage Activation Specification

## Purpose

Define the activation switch that gates new-lineage transitions, the coexistence precedence between legacy and new authority (Amendment C), the additive (non-cutover) shape of the gate branch, and rollback semantics that never strand an in-flight new lineage.

## Requirements

### Requirement: Distinct Env Switch, Default Off, Legacy Path When Disabled

The activation switch MUST be a dedicated environment variable, default OFF, distinct in identity and meaning from `GENTLE_AI_RDD_SHADOW` and from the user-owned RDD kill switch (adopted default). When off, every `start` MUST take the legacy path and create no new-lineage artifact.

#### Scenario: Default configuration takes the legacy path

- GIVEN the activation switch unset
- WHEN a new `start` is requested
- THEN it proceeds through the legacy state machine and creates no `review-state.json`/`review-receipt.json` new-lineage record

#### Scenario: Switch identity never overloads another switch

- GIVEN the activation switch, `GENTLE_AI_RDD_SHADOW`, and the RDD kill switch
- WHEN any one of the three is toggled
- THEN only its own scoped behavior changes; the other two are unaffected

### Requirement: Kill-Switch-Off Is Structurally Unfailable and Creates Nothing

When the user-owned RDD kill switch is off, the facade MUST create no artifact and MUST NOT be able to fail (Wave-4 precondition).

#### Scenario: Kill switch off produces no side effect

- GIVEN the RDD kill switch is off
- WHEN the facade is invoked at any observed call site
- THEN no artifact is created, and no error path is reachable

### Requirement: Coexistence Precedence Matrix (Amendment C)

For each gate, the per-gate precedence matrix over {legacy authority, new-lineage authority} × {exists, absent} MUST decide authorization. A legacy-only authority record MUST NEVER authorize a new-lineage candidate.

#### Scenario: Legacy authority alone denies a new-lineage candidate

- GIVEN only legacy authority exists for a candidate and the candidate is being evaluated as a new lineage
- WHEN a gate checks authorization
- THEN it denies, even though legacy authority is present

#### Scenario: New-lineage receipt authorizes a new-lineage candidate

- GIVEN a new-lineage receipt exists and no legacy authority is required
- WHEN a gate checks authorization for that candidate
- THEN it authorizes using the new-lineage receipt

### Requirement: Additive Gate Branch, Switch-Off Byte-Equivalence, Not a Cutover

Each of the five gates (`post-apply`, `pre-commit`, `pre-push`, `pre-pr`, `release`) MUST receive a strictly additive branch keyed on lineage kind. The legacy branch MUST remain byte-identical when the switch is off. This wave MUST NOT perform a legacy-to-new cutover.

#### Scenario: Switch-off byte-equivalence at every gate

- GIVEN the activation switch off
- WHEN each of the five gates evaluates a legacy candidate
- THEN its decision is byte-identical to the pre-Wave-3 baseline

#### Scenario: New branch never touches legacy code path

- GIVEN the activation switch on and a new-lineage candidate
- WHEN a gate evaluates it
- THEN only the lineage-kind-keyed new branch executes; the legacy branch's code path is not entered

### Requirement: Rollback Disables New Starts Only

Disabling the activation switch MUST stop new lineage `start` calls only. Already-created new lineages MUST remain readable and MUST be able to finalize.

#### Scenario: In-flight new lineage still finalizes after rollback

- GIVEN a new lineage in `correcting` state and the activation switch then turned off
- WHEN that lineage's `finalize` is invoked
- THEN it completes and issues its receipt

#### Scenario: Rollback blocks only new starts

- GIVEN the activation switch turned off
- WHEN a brand-new `start` is requested
- THEN it takes the legacy path, while any already-open new lineage remains executable
