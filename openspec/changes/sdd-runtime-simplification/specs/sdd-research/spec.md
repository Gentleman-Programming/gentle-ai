# Delta for sdd-research

## MODIFIED Requirements

### Requirement: Closed Capability Admission

The lane MUST accept only `gentle-ai.sdd-research-capability/v1` and classes `documentation` and `open-web`; admission MUST verify each selected declaration and its exact granted tool capability. Unknown or missing values, Bash, generic MCP, unnamed inherited tools, and substituted tool names MUST deny. The admitted record MUST authoritatively constrain research routes in child provisioning. Separately authorized read, write, and persistence tools MUST remain available within their existing scope. Admission MUST NOT invent a capability, silently broaden a grant, or convert a denied capability into a write authorization.
(Previously: admission verified known classes and exact grants, denying unknown or unverifiable tools.)

#### Scenario: RS-01 supported request

- GIVEN a selected known class has its declared exact grant
- WHEN admission runs
- THEN it records the grant and permits research

#### Scenario: RS-02 denied or unknown capability

- GIVEN a request has an unknown class/version or unverifiable tool
- WHEN admission runs
- THEN it records denial, emits no claim, and blocks

#### Scenario: RS-03 child receives exact admitted research capabilities

- GIVEN research admission recorded selected classes and exact grants
- WHEN a child is provisioned for that research phase
- THEN it receives only the recorded capabilities and grants for research routes
- AND separately authorized read, write, and persistence tools remain available within their existing scope

#### Scenario: RS-04 denied research capability preserves truthful evidence

- GIVEN a required research capability is missing or denied
- WHEN the phase records its denial or partial evidence
- THEN it uses already-authorized artifact scope to preserve that record
- AND selected research and proposal remain blocked without fallback, substitution, or scope expansion

### Requirement: Hybrid Completion and Recovery

Selected research MUST persist revisioned intent, admission, outcome, and references. `openspec` validates OpenSpec only, `engram` validates Engram only, `hybrid` requires matching OpenSpec/Engram writes, and `none` cannot set `proposal_ready`. Where the selected store and admitted phase already authorize an artifact write, recovery and downstream handoff MUST retain that bounded store/path scope; they MUST NOT create multi-store authority or bypass consent. Failure, missing artifact, divergence, partial, or blocked MUST retain intent and block.
(Previously: store-specific completion and hybrid byte/revision recovery retained intent and blocked failure or divergence.)

#### Scenario: RS-05 matching restart

- GIVEN both stores recover equivalent revisions and research is done
- WHEN pre-proposal state is recovered
- THEN request and evidence references are restored

#### Scenario: RS-06 divergent restart

- GIVEN either store failed to write or recovered revisions differ
- WHEN recovery runs
- THEN proposal remains blocked and neither copy is silently preferred

#### Scenario: RS-07 one-sided hybrid write recovery

- GIVEN one hybrid store write failed and retained pre-write intent and canonical desired content exist
- WHEN recovery runs
- THEN it writes a new positive revision to both stores and reads both back for equal revision and bytes before readiness

#### Scenario: RS-08 missing recovery intent

- GIVEN retained pre-write intent is unavailable
- WHEN hybrid recovery runs
- THEN it remains blocked and requires explicit re-entry without inventing state

#### Scenario: RS-09 authorized OpenSpec scope survives continuation

- GIVEN OpenSpec is the selected store and research already has an authorized change-local write scope
- WHEN corrected facts permit native continuation
- THEN the continuation retains that same path and scope
- AND it does not require unrelated launch or delivery authorization

#### Scenario: RS-10 bounded recovery rejects stale authority

- GIVEN a recovery record is stale, divergent, outside the authorized scope, or bound to another worktree
- WHEN continuation is requested
- THEN recovery refuses without selecting a write
- AND it retains the recorded intent and uncertainty
