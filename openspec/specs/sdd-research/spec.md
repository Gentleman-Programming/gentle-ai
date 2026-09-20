# sdd-research Specification

## Purpose

Selected research is optional. When the orchestrator decides to invoke it, it runs under verified capability admission; its absence does not by itself block the proposal.

(Previously: "Selected research is required.")

## Requirements

### Requirement: Open Capability Admission for Optional Research (formerly: Closed Capability Admission)

The selected-research lane is optional, not mandatory, following the absorption of upstream's replacement of closed admission (commit `ba3ed690`). The lane MUST continue to accept only `gentle-ai.sdd-research-capability/v1` and classes `documentation` and `open-web`; admission MUST continue to verify declaration and exact grant. Bash, generic MCP, and unnamed legacy tools MUST continue to be denied. The absence of a selected-research invocation for a given change MUST NOT by itself block the proposal or any other SDD artifact.

(Previously: the lane MUST accept only `gentle-ai.sdd-research-capability/v1` and classes `documentation` and `open-web` under a closed, mandatory admission model, with no clause about the absence of research.)

#### Scenario: Request admitted when research is invoked

- GIVEN the orchestrator decides to invoke selected research for a change
- AND a known, selected class has its exact grant declared
- WHEN admission runs
- THEN it records the grant and permits the research

#### Scenario: Unknown or denied capability still blocks that invocation

- GIVEN a request has an unknown class/version or unverifiable tool
- WHEN admission runs
- THEN it records denial, emits no claim, and blocks that invocation
- AND that block does not prevent the change from continuing through a planning lane that does not depend on that research

#### Scenario: Absence of selected research does not block the proposal

- GIVEN a change for which the orchestrator did not invoke selected research
- WHEN that change's proposal is drafted
- THEN the absence of that research does not by itself block the proposal
- AND any block declared in `sdd-propose` comes from a reason other than the absence of selected research

### Requirement: Auditable Evidence Integrity

Completed artifacts MUST record questions, admission/grants, source IDs (class/title/publisher/URL/access date/excerpt), claim mappings, contradictions, uncertainty/freshness, and separate product choices.

#### Scenario: Complete source-backed result

- GIVEN admission succeeds and sources answer the questions
- WHEN the artifact is completed
- THEN each claim maps to source IDs and remains separate from product choices

#### Scenario: Partial or blocked research

- GIVEN research is partial or blocked
- WHEN its artifact is persisted
- THEN outcome is explicit, unvalidated claims are excluded, and readiness is false

### Requirement: Hybrid Completion and Recovery

Selected research MUST persist revisioned intent, admission, outcome, and references. `openspec` validates OpenSpec only, `engram` validates Engram only, `hybrid` requires matching OpenSpec/Engram writes, and `none` cannot set `proposal_ready`. Failure, missing artifact, divergence, partial, or blocked MUST retain intent and block.

#### Scenario: Matching restart

- GIVEN both stores recover equivalent revisions and research is done
- WHEN pre-proposal state is recovered
- THEN request and evidence references are restored

#### Scenario: Divergent restart

- GIVEN either store failed to write or recovered revisions differ
- WHEN recovery runs
- THEN proposal remains blocked and neither copy is silently preferred

#### Scenario: One-sided hybrid write recovery

- GIVEN one hybrid store write failed and retained pre-write intent and canonical desired content exist
- WHEN recovery runs
- THEN it writes a new positive revision to both stores and reads both back for equal revision and bytes before readiness

#### Scenario: Missing recovery intent

- GIVEN retained pre-write intent is unavailable
- WHEN hybrid recovery runs
- THEN it remains blocked and requires explicit re-entry without inventing state
