# Organic Agent Routing Projection

## Purpose

Define the managed, provider-neutral instructions that project Gentle AI's
canonical implementation-routing facts into each supported adapter. The
projection helps an orchestrator choose the smallest useful implementation
topology; it does not own runtime route admission, verification, review,
delivery, or lifecycle transitions.

## Requirements

### Requirement: Canonical provider-owned routing facts

Managed routing instructions MUST render from the canonical typed capability
manifest. They MUST NOT maintain an independent prompt event catalog, risk
router, or implementation-route selector.

#### Scenario: Adapter routing is rendered

- GIVEN a provider-registered adapter with a valid capability manifest
- WHEN its managed routing instructions are rendered
- THEN direct, delegated-direct, and optional-SDD facts equal the canonical manifest
- AND the rendered prose does not become runtime authority

### Requirement: Direct-inline boundary

The routing projection MUST allow inline decision or verification when
understanding requires one to three files. It MAY keep one mechanical file
change inline only when the change is already understood, requires no research,
and has no unresolved design decision.

#### Scenario: Small understood change remains inline

- GIVEN one already-understood mechanical file change
- AND no research or design decision remains
- WHEN implementation topology is considered
- THEN `direct_inline` is an available route
- AND no SDD artifact is created

#### Scenario: Understanding expands to four files

- GIVEN the work requires understanding four or more files
- WHEN implementation topology is considered
- THEN the understanding step is not kept inline

### Requirement: Delegated-direct boundary

The routing projection MUST delegate narrow exploration when understanding
requires four or more files. It MUST delegate one writer for two or more
non-trivial implementation files, and MUST delegate reading that prepares a
write or broad research when the adapter supports delegation.

#### Scenario: Multi-file understanding delegates

- GIVEN implementation requires understanding four files
- WHEN the route is projected
- THEN narrow exploration uses delegated-direct topology
- AND delegation does not create an SDD lifecycle

#### Scenario: Two non-trivial files require one writer

- GIVEN implementation changes two non-trivial files
- WHEN the write topology is projected
- THEN one delegated writer owns the implementation
- AND the file count alone does not select SDD

### Requirement: Delegation applies per action

Implementation route and action-level delegation MUST remain independent.
Tests, builds, installs, and owner-selected checking actors MAY use fresh
workers without changing a direct implementation route or creating an SDD run.

#### Scenario: Direct implementation delegates a build

- GIVEN implementation is already bound to `direct_inline`
- WHEN an execution-heavy build needs a fresh worker
- THEN the build may be delegated
- AND the implementation route remains `direct_inline`
- AND no synthetic SDD state is created

### Requirement: SDD remains optional

The routing projection MUST propose SDD only when work is genuinely substantial
or ambiguous and durable proposal, specification, design, or task artifacts
would materially reduce uncertainty. SDD MUST be selected only after an
explicit request or an accepted proposal. Risk alone MUST NOT force SDD.

#### Scenario: User accepts an SDD proposal

- GIVEN substantial ambiguity would benefit from durable planning artifacts
- AND the user did not explicitly request SDD
- WHEN the orchestrator proposes SDD
- THEN the proposal remains a pending decision until accepted
- AND only acceptance may bind the `sdd` implementation route

#### Scenario: User declines SDD

- GIVEN SDD was proposed and declined
- WHEN a safe smaller topology exists
- THEN work continues through a justified direct or delegated-direct route
- AND no SDD artifacts are fabricated

### Requirement: Capability manifests describe facts, not outcomes

Each provider-registered adapter MUST expose its own canonical typed capability
manifest. A manifest MUST describe static adapter features and routing facts
only; it MUST NOT claim runtime observations, select a route, issue a review
decision, or assert PASS. Unsupported adapter identities MUST be rejected.

#### Scenario: Adapter-specific features differ

- GIVEN two registered adapters have different native integration mechanisms
- WHEN their capability manifests are read
- THEN their feature claims may differ truthfully
- AND their canonical implementation-routing facts remain semantically equal

#### Scenario: Unknown adapter is requested

- GIVEN an adapter identity not registered by the provider
- WHEN its capability manifest is requested
- THEN the request is rejected
- AND no generic capability claim is invented

### Requirement: Contract exposure fails closed

Typed work-routing capability MUST distinguish dormant from advertised
exposure. A dormant, disabled, unknown, or read-only capability MUST NOT be
presented as writable runtime authority.

#### Scenario: Work routing is dormant

- GIVEN an adapter manifest whose work-routing contract is dormant
- WHEN managed guidance is used
- THEN no productive transition is advertised
- AND the orchestrator cannot infer a transition from the static routing facts

### Requirement: Runtime transitions are provider-issued

When a managed WorkRun and advertised capability exist, instructions MUST
request the exact typed work-status contract and apply only its zero-or-one
provider-issued `authorizedTransition` through the typed work-transition
contract. Instructions MUST NOT reconstruct flags, select review lenses, infer
PASS from prose or process exit, or retry stale or mismatched authorization.

#### Scenario: Status returns one authorized transition

- GIVEN a managed WorkRun
- AND typed status returns one current `authorizedTransition`
- WHEN the orchestrator advances the work
- THEN it applies that exact authorization and expected revision
- AND it does not synthesize an alternate operation

#### Scenario: Runtime authority is unavailable

- GIVEN the capability is unavailable, disabled, unknown, or read-only
- WHEN the orchestrator requests progress
- THEN it reports the typed stop
- AND it does not fall back to prompt-owned lifecycle authority

### Requirement: Public progress remains outcome-first

Managed routing instructions MUST present only **Working**, **Checking**,
**Ready**, and **Needs your decision** as normal progress states. Hashes,
lineages, lenses, receipts, and recovery operations MUST remain internal unless
maintainer diagnostics are explicitly requested.

#### Scenario: Provider performs bounded review

- GIVEN implementation has entered provider-owned verification and review
- WHEN progress is presented to the user
- THEN the visible state is **Checking**
- AND prompt-level review ceremonies are not exposed as extra workflow states

### Requirement: No prompt-owned lifecycle router

Managed routing instructions MUST NOT bind pre-commit, pre-push, pre-PR,
release, SDD-phase, CI, or schedule events to review actions. They MUST NOT
select a reviewer, refuter, correction, receipt outcome, or delivery decision.
Those decisions remain with their typed owner contexts.

#### Scenario: Managed rules are installed

- GIVEN adapter synchronization renders the managed routing block
- WHEN the block is inspected
- THEN it contains no event-to-review binding or lens-selection table
- AND later gates consume owner-issued evidence and authorization

### Requirement: Deterministic text-only installation

Routing rendering MUST remain deterministic, marker-free text generation.
Installation MAY update managed configuration files, but MUST NOT create Git
hooks, evaluate events in a daemon, invoke an agent, or start runtime work.

#### Scenario: Routing rules are installed

- GIVEN an adapter synchronization
- WHEN managed routing instructions are rendered and injected
- THEN repeated rendering produces identical text
- AND no review, verification, or delivery effect launches

### Requirement: User-owned model configuration

Routing policy MUST keep model, provider, profile, and effort as user-owned
choices. Those choices MUST NOT change the canonical routing thresholds or
become inputs to typed safety decisions.

#### Scenario: Runtime preferences change

- GIVEN the user selects a different model or effort
- WHEN routing instructions are evaluated
- THEN canonical direct, delegated-direct, and optional-SDD facts are unchanged
- AND no safety outcome is inferred from the runtime preference
