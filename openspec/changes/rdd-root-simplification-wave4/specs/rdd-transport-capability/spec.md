# RDD Transport Capability Specification

## Purpose

Define pre-freeze transport-capability admission: a runtime/adapter declares whether it can carry a review transition BEFORE any authority, tier, budget, or collection state exists for a candidate. Unsupported transport MUST deny cleanly with no recoverable remnant, never an unsafe fallback.

## Requirements

### Requirement: Wave-0 Adapter Behavioral-Depth Trace Is a Hard Prerequisite

The CON-09/10/11 adapter behavioral-depth trace (Wave 0 inventory gap) MUST complete and its findings MUST be recorded and referenced before any consumer-thinning task in this wave begins. This is the FIRST requirement of this spec and gates all others below.

#### Scenario: Trace recorded before adapter thinning starts

- GIVEN Wave 4 implementation is about to touch `internal/agents/{opencode,claude,pi}/adapter.go`
- WHEN a thinning task is picked up
- THEN CON-09/10/11 trace findings already exist and are cited by that task
- AND the task description links to the recorded trace evidence

#### Scenario: Missing trace blocks the task

- GIVEN the CON-09/10/11 trace has not been completed and recorded
- WHEN an implementer attempts an adapter-thinning task
- THEN the task MUST NOT proceed
- AND the blocker is reported as "Wave-0 trace incomplete"

### Requirement: Capability Declared Before Any Review State Exists

A runtime/adapter MUST declare its transport capability BEFORE any authority, tier, lens set, budget, or collection state is created for a candidate. Unsupported transport MUST deny before that state is created, leaving no recoverable remnant. (Issue #1247)

#### Scenario: Supported transport proceeds normally

- GIVEN an adapter declares transport capability as supported
- WHEN a review is started
- THEN capability admission succeeds and authority/tier/budget/collection state is created afterward, in that order

#### Scenario: Unsupported transport denies before state creation

- GIVEN an adapter declares transport capability as unsupported (or absent/unrecognized)
- WHEN a review start is attempted
- THEN the provider denies before creating any authority, tier, budget, or collection state
- AND no partial or orphaned review record remains

### Requirement: Adapter Declares, Provider Fails Closed, No Probing

Each adapter MUST declare its own capability. The provider MUST NOT probe or infer adapter capability. The provider MUST fail closed when a declaration is absent or unrecognized.

#### Scenario: Adapter self-declares capability

- GIVEN a specific adapter (opencode, claude, or Pi)
- WHEN it initiates a review-capable interaction
- THEN it declares its own transport capability explicitly
- AND the provider does not attempt to detect capability independently

#### Scenario: Absent or unrecognized declaration fails closed

- GIVEN a declaration is missing or does not match a known capability value
- WHEN the provider evaluates it
- THEN the provider treats the transport as unsupported
- AND no review state is created

### Requirement: Per-Adapter Unavailable Mode, Never Unsafe Fallback

On unsupported or absent capability, the adapter MUST enter a per-adapter unavailable mode. The adapter MUST NOT construct its own flags, revisions, targets, or bindings, and MUST NOT silently fall back to legacy/unsafe behavior. (Issue #1385)

#### Scenario: Pi adapter without capability enters unavailable mode

- GIVEN the Pi adapter still uses the old, non-opaque shape and has not declared capability
- WHEN it attempts a review interaction
- THEN it enters unavailable mode
- AND it does not self-construct a transition, flag, or binding of any kind

#### Scenario: Capable in-repo adapter executes only opaque transitions

- GIVEN the opencode or claude adapter declares supported capability
- WHEN it participates in a review
- THEN it executes only provider-issued opaque transitions
- AND it constructs no flag, revision, target, or binding itself
