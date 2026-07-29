# Delta for Review Findings Ledger

## ADDED Requirements

### Requirement: Compact-v2 persisted receipt compatibility

Compact-v2 persisted state and receipt fields MUST remain schema-v2 compatible. Regeneration MUST preserve the final candidate tree from the current snapshot and the frozen initial/genesis path digest without migration.

#### Scenario: Persisted v2 receipt regenerates

- GIVEN a persisted legacy-v2 compact state and receipt fixture
- WHEN the receipt is reloaded and regenerated
- THEN its v2-compatible receipt identity is preserved

#### Scenario: Corrected intended-untracked scope validates

- GIVEN a correction whose final snapshot includes intended-untracked content
- WHEN lifecycle validation evaluates that final target
- THEN it returns `allow` using the exact final tree and paths within the frozen genesis envelope

## MODIFIED Requirements

### Requirement: Terminal receipt

Only `approved | escalated` are terminal transaction states. An approved receipt MUST bind `lineage_id`, mode, generation, base tree, `initial_review_tree`, `final_candidate_tree`, `paths_digest`, `fix_delta_hash`, policy hash, ledger hash, evidence hash, mode-specific counters, judge proof where applicable, and terminal state. For compact v2, `final_candidate_tree` MUST identify the current final snapshot while `paths_digest` MUST preserve the frozen initial/genesis authorized path envelope. A release-bound receipt MUST additionally bind immutable release tree, configuration hash, generated-artifact hash, provenance/signing hash, publication-boundary hash and sealed state, plus evidence-freshness hash and current state.

(Previously: Receipt identity did not explicitly require final tree and paths digest to come from one final snapshot.)

#### Scenario: Post-fix receipt distinguishes trees

- GIVEN ordinary review corrected the initial candidate
- WHEN the receipt is emitted
- THEN `initial_review_tree` identifies what the lenses reviewed
- AND `final_candidate_tree` identifies the scoped-validated candidate
- AND `fix_delta_hash` identifies only their correction delta

### Requirement: Deterministic lifecycle validation

Pre-commit, pre-push, pre-PR, and release MUST validate the same receipt and return `allow | scope-changed | invalidated | escalated`. Native validation MUST derive the current repository target and hash persisted policy, ledger, fix delta, verify evidence, and release artifacts instead of trusting caller-authored tree/hash assertions. Fabricated or nonexistent objects and stale artifacts MUST fail closed. Only `allow` returns process success; every denial still emits parseable machine JSON and returns non-success. Gates MUST NOT start a reviewer, create another budget, silently start Judgment Day, or allocate a lineage. `scope-changed` MUST require explicit maintainer action.

(Previously: Scope-changed denial action and zero-allocation constraint were not explicit.)

#### Scenario: Unchanged approved target is reused

- GIVEN the receipt and gate context match exactly
- WHEN a lifecycle validator runs
- THEN the result is `allow`
- AND zero reviewers run

#### Scenario: External evidence changes

- GIVEN unchanged reviewed code but new CI, vulnerability, base, policy, provenance, or release evidence
- WHEN the lifecycle validator evaluates it
- THEN the result may be `invalidated` or `escalated`
- AND unchanged code review is not reopened

#### Scenario: Scope change fails closed

- GIVEN the current target differs from the receipt scope
- WHEN lifecycle validation runs
- THEN it returns non-success `scope-changed` with maintainer action
- AND no lineage, review, or budget is allocated
