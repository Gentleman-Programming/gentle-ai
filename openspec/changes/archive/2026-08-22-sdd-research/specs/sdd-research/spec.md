# sdd-research Specification

## Purpose

Selected research is required.

## Requirements

### Requirement: Closed Capability Admission

The lane MUST accept only `gentle-ai.sdd-research-capability/v1` and classes `documentation` and `open-web`; admission MUST verify declaration and exact grant. Unknown or missing values, Bash, generic MCP, and unnamed inherited tools MUST deny.

#### Scenario: Supported request

- GIVEN a selected known class has its declared exact grant
- WHEN admission runs
- THEN it records the grant and permits research

#### Scenario: Denied or unknown capability

- GIVEN a request has an unknown class/version or unverifiable tool
- WHEN admission runs
- THEN it records denial, emits no claim, and blocks

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
