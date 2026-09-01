# Delta for RDD Authority Graph Classification

Wave 2 leaves classification unchanged: the classifier still returns `healthy | repairable | blocked` and derives nothing. What changes is what happens after a `repairable` result — a separate, deterministic plan derivation (`rdd-authority-disposition-plan`) MAY now consume that classification, and Wave 2's leaf-only executor (`rdd-leaf-disposition-execution`) MAY consume that plan. The classifier's own boundary — inspection-only, no plan derivation — is now stated explicitly because it must survive contact with a real downstream consumer.

## MODIFIED Requirements

### Requirement: No Mutation or Execution

The classifier MUST NOT mutate the authority graph, derive or execute a disposition plan, quarantine bytes, or acquire the maintenance lock. Classification is inspection-only. A separate, deterministic plan derivation (owned by `rdd-authority-disposition-plan`) MAY consume a `repairable` classification's evidence to produce a `DispositionPlan`; that derivation is not part of the classifier and MUST NOT be implemented inside it.
(Previously: stated the classifier has no side effect, without addressing a downstream consumer; Wave 2 introduces the first such consumer, `rdd-authority-disposition-plan`, so the boundary is now explicit.)

#### Scenario: Classification has no side effect

- GIVEN any authority graph
- WHEN the classifier runs
- THEN no authority record, receipt, or graph edge is modified
- AND no disposition plan is created or persisted

#### Scenario: Repairable result feeds a separate plan derivation, not the classifier itself

- GIVEN a `repairable` classification for #1892's leaf anomaly shape
- WHEN a `DispositionPlan` is subsequently derived from that classification's evidence
- THEN the derivation happens in `rdd-authority-disposition-plan`, and re-running the classifier alone still produces no plan and no mutation
