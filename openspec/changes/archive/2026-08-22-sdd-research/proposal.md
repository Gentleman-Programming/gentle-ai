# Proposal: Source-Backed SDD Research

## Intent

Add an optional `sdd-research` lane for auditable external evidence, separate from local `sdd-explore`. Once selected, it fails closed and feeds orchestrator-owned product discovery for deterministic, restart-safe proposals.

## Scope

### In Scope
- Add the research phase, source-backed artifact, commands, executors, model/profile registration, and hybrid persistence.
- Define a versioned capability/admission contract with closed source classes.
- Add an orchestrator pre-proposal gate and move all product questioning out of `sdd-propose`.
- Maintain parity across 12 templates and 16 AgentIDs.

### Out of Scope
- Native status v2 or changes to status-v1 shape, tokens, or routing.
- Mandatory research, implicit product consent, or support for unverified source classes.
- Bench journeys unless native binary routing changes.

## Capabilities

### New Capabilities
- `sdd-research`: Covers capability admission, external-source evidence, research artifact schema, hybrid recovery, and selected-lane completion semantics.

### Modified Capabilities
- `sdd-orchestrator-assets`: Adds optional research selection, pre-proposal readiness enforcement, mode-independent product discovery, proposal handoff, and cross-runtime parity.

## Approach

Implement approach 3 with `gentle-ai.sdd-research-capability/v1`. Adapters declare maximum supported classes: `documentation` and `open-web`. Admission verifies the exact granted tool for each requested class. Unknown versions/classes, absent declarations, Bash, generic MCP, and unnamed inherited tools deny admission.

Persist versioned, revisioned pre-proposal state: exploration outcome; research request, classes, admission, outcome, and artifact references; product-decision status; and `proposal_ready`. Selected research must reach `done`; hybrid write failure or revision mismatch blocks. The orchestrator confirms product decisions for a non-interviewing proposer. Native status v1 remains unchanged; every orchestrator gates its `propose` recommendation.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/agents/`, `internal/model/`, `internal/catalog/`, `internal/components/sdd/`, `internal/opencode/`, `internal/tui/` | Modified | Contract, inventories, profiles, assignments, and permissions |
| `internal/assets/skills/` and `internal/assets/*/` | New/Modified | Research assets, persistence rules, proposal ownership, and 12-template lifecycle parity |
| `internal/sddstatus/` and tests/goldens | Modified | Compatibility and fail-closed parity coverage only |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Static declarations overstate runtime grants | Medium | Verify exact phase grants and default-deny tests |
| Hybrid stores diverge | Medium | Revision comparison blocks proposal |
| Broad asset churn exceeds 800 lines | High | Plan reviewable slices and ask before apply |

## Rollback Plan

Remove research registrations/assets and pre-proposal state handling, then restore prior proposer/orchestrator assets. Unchanged status v1 keeps existing changes operable.

## Dependencies

- Explicit runtime source-tool grants and available OpenSpec/Engram persistence.

## Success Criteria

- [ ] Unsupported or unverifiable research requests fail closed without source claims.
- [ ] Selected research, confirmed product decisions, and matching hybrid state are required before proposal.
- [ ] All runtime assets remain parity-tested while status v1 stays compatible.
