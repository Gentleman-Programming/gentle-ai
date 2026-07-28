# Delta for Organic Agent Routing Projection

## ADDED Requirements

### Requirement: Organic implementation intake
Routing MUST preserve direct-inline, delegated-direct, and optional-SDD behavior without universal pre-intake WorkRun creation, runtime authentication, connector negotiation, or dormant ceremony. File count or LOC alone MUST NOT force SDD or high-risk review.

#### Scenario: Ordinary direct change
- GIVEN a small understood change
- WHEN implementation begins
- THEN direct-inline is available without a WorkRun
- AND no runtime authentication is requested

#### Scenario: Ambiguous change
- GIVEN durable planning would materially reduce uncertainty
- WHEN SDD is proposed
- THEN SDD requires explicit request or acceptance
- AND risk alone does not select it

### Requirement: Routing installation is unconditional
Organic routing guidance MUST be installed for every configured agent, independently of whether the user selected the optional SDD component. Routing installation MUST NOT depend on SDD component selection, SDD mode, or any SDD asset being present. Optional SDD assets MUST remain separately selectable without altering whether routing is installed.

#### Scenario: Agent configured without optional SDD
- GIVEN a configured agent whose selection excludes the optional SDD component
- WHEN its managed configuration is installed or synced
- THEN direct-inline, delegated-direct, and optional-SDD routing guidance is present
- AND no SDD orchestration asset is installed

#### Scenario: SDD selection does not change routing presence
- GIVEN two configured agents that differ only in optional SDD component selection
- WHEN both are installed or synced
- THEN both contain semantically equal routing guidance
- AND only the SDD-selected agent additionally contains SDD orchestration assets

### Requirement: Remote-control-plane removal
Routing guidance MUST NOT activate remote connectors, URL/token/CA setup, global agents, daemons, or runtime sessions.

#### Scenario: Adapter guidance is rendered
- GIVEN a supported adapter
- WHEN routing guidance is generated
- THEN it contains no remote activation instruction

### Requirement: Disabled RDD remains organic
When the user kill switch disables new RDD starts, implementation routing MUST remain direct, delegated, or optional SDD. Adapter guidance MUST NOT retry, reactivate, or replace disabled RDD authority.

#### Scenario: RDD is disabled
- GIVEN organic implementation produced a candidate while RDD is disabled
- WHEN the adapter observes the rejected review start
- THEN it reports RDD as disabled without retry
- AND no WorkRun or remote fallback is activated

### Requirement: The RDD kill switch is discoverable from installed guidance
Installed routing guidance MUST make the user-owned RDD kill switch discoverable for every configured agent by naming its exact command surface `gentle-ai review mode enable|disable|status` and stating that `status` is read-only and reports the deciding source plus the effective mode. The guidance MUST state that when the user asks to stop using RDD the agent runs `disable` instead of arguing, working around it, or proposing alternatives first. The guidance MUST state that the agent MUST NOT enable RDD on the user's behalf unless the user explicitly asks.

#### Scenario: User asks to stop using RDD
- GIVEN a configured agent whose installed routing guidance is present
- WHEN the user asks to stop using review-driven development
- THEN the agent runs `gentle-ai review mode disable` without arguing or proposing an alternative first
- AND it keeps implementing through direct, delegated, or optional SDD

#### Scenario: Routing guidance is installed
- GIVEN any supported adapter configured with or without the optional SDD component
- WHEN its routing guidance is generated
- THEN it names `gentle-ai review mode enable|disable|status` and the read-only nature of `status`
- AND it forbids enabling RDD on the user's behalf without an explicit request

## MODIFIED Requirements

### Requirement: Capability manifests describe facts, not outcomes
Each provider-registered adapter MUST expose its own canonical typed capability manifest. A manifest MUST describe truthful static adapter-native features and organic routing facts only; it MUST NOT claim runtime observations, select a route, create or authorize a WorkRun, issue a review decision, or assert PASS. Unsupported and unknown adapter identities MUST be rejected.
(Previously: manifests did not explicitly prohibit WorkRun creation/authorization or identify organic routing facts.)

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

## REMOVED Requirements

### Requirement: Contract exposure fails closed
(Reason: connector-backed writable WorkRun authority is removed.)
(Migration: Local post-candidate receipt validation.)

### Requirement: Runtime transitions are provider-issued
(Reason: pre-intake managed WorkRun transitions are removed.)
(Migration: Local post-candidate receipt validation.)
