# Delta for RDD Leaf Disposition Execution

## MODIFIED Requirements

### Requirement: Cardinality-One Admission

The executor MUST admit a plan when `closure(S)` has cardinality exactly one, as the N=1 base case of the general closure admission rule defined in `rdd-closure-disposition-execution` (closed anomaly classes, cardinality `>= 1`). This requirement no longer refuses multi-node closures itself; multi-node admission and its ordering, manifest, and resume semantics are owned by `rdd-closure-disposition-execution`.

(Previously: this requirement was the sole admission gate and refused any cardinality other than exactly one, naming #2014/#1656 and "a future wave (Wave 6)" in the refusal. Wave 6 relaxes admission to `>= 1` for closed classes; the multi-node refusal and its #2014-naming scenario retire.)

#### Scenario: Single-node closure is admitted

- GIVEN a plan whose `ordered_closure` has exactly one entry, classified from #1892's historical exact-binding edge shape
- WHEN the executor evaluates admission
- THEN the plan is admitted for execution
