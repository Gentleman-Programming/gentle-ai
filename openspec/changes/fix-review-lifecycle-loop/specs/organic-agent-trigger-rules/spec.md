# Delta for Organic Agent Trigger Rules

## MODIFIED Requirements

### Requirement: Deterministic validation outcomes

Rendered rules MUST define `allow`, `scope-changed`, `invalidated`, and `escalated` actions. Missing receipt after implementation/post-apply starts an explicit new review; `scope-changed` MUST require explicit maintainer action and MUST NOT direct automatic lineage creation, review, or budget allocation; invalidation requires maintainer action; escalation stops delivery.

(Previously: Unrelated scope change required a new lineage.)

#### Scenario: Receipt is invalidated

- GIVEN policy or provenance changed
- WHEN validation runs
- THEN the result is `invalidated`
- AND no review budget is silently reset

#### Scenario: Scope change guidance is maintainer-only

- GIVEN rendered rules describe `scope-changed`
- WHEN a maintainer reads the action
- THEN the rules require explicit maintainer action
- AND they contain no automatic lineage, review, or budget instruction
