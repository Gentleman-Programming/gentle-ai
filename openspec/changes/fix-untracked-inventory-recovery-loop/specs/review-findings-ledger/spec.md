# Delta for review-findings-ledger

## ADDED Requirements

### Requirement: Unconditional Eligible-Untracked-Inventory Publication

The review STATUS projection MUST carry the canonical eligible-untracked-inventory digest as a top-level field named `eligible_untracked_inventory` whenever the projection inspects untracked files for the target. Publication MUST NOT depend on whether the caller has already declared a selection, on whether selection is still needed, on RDD mode being enabled or disabled, or on a compact reviewing authority being current for the target. Every scope-replacement path, including the compact-reviewing override, MUST preserve or recompute a real digest rather than substituting an empty one.

#### Scenario: Field published before declaration
- GIVEN a target with eligible untracked files and no prior declaration
- WHEN STATUS is requested
- THEN `eligible_untracked_inventory` is present and non-empty at the top level

#### Scenario: Field published after declaration
- GIVEN a caller already declared a selection (`Declared = true`) for the target
- WHEN STATUS is requested again, as a prior refusal instructed
- THEN `eligible_untracked_inventory` remains present and usable
- AND the caller is not sent back into the same refusal with no way to obtain the value

#### Scenario: Field published while RDD is disabled
- GIVEN the transition for the target resolves to `{stop, rdd_disabled}`
- WHEN STATUS is requested
- THEN `eligible_untracked_inventory` is still published at the top level
- AND publication does not depend on overriding the `rdd_disabled` stop

#### Scenario: Field published under a current compact reviewing authority
- GIVEN a compact reviewing authority is current for the target
- WHEN STATUS replaces the scope for that authority
- THEN the replacement carries a preserved or recomputed digest
- AND `eligible_untracked_inventory` is not zeroed by the replacement

### Requirement: Staged Projection Omits The Field

For the `staged` projection, `eligible_untracked_inventory` MUST be absent from the serialized response — the key itself omitted, not present with an empty value. Staged does not inspect untracked files by design; absence asserts "not applicable", while an empty digest would falsely assert "checked, found none".

#### Scenario: Key is structurally absent for staged
- GIVEN a `staged` projection request
- WHEN the response is serialized
- THEN no `eligible_untracked_inventory` key appears in the JSON output

### Requirement: Schema v7 Advertisement

The review-integration wire contract MUST advertise a `status-v7.schema.json` schema adding `eligible_untracked_inventory` as an optional field, with corresponding `ReviewIntegrationStatusSchemaV7`/`SchemaIDV7` identifiers and a `Schemas` capability entry. The addition MUST remain additive: consumers following `AdditiveMinorPolicy: optional-fields-only` ignore the unknown field without error, and v6 continues to be advertised unchanged.

#### Scenario: Capabilities list both v6 and v7
- GIVEN a capabilities query
- WHEN schema versions are enumerated
- THEN both `status-v6.schema.json` and `status-v7.schema.json` are listed
- AND v7's `$id` and schema identifiers are distinct from v6's

### Requirement: Recovery Route Produces A Usable Value

The recovery command named by an untracked-selection refusal MUST, when run, publish a digest that the same refusal's required flag accepts, so that retrying with that value succeeds. Naming a command that only parses without producing a usable value does not satisfy this requirement.

#### Scenario: Refusal, recovery, and retry close the loop
- GIVEN a caller receives an untracked-selection refusal naming `gentle-ai review status --next-transition` as the recovery route
- WHEN the caller runs the named command
- THEN the response carries a non-empty `eligible_untracked_inventory` digest
- AND passing that digest as `--expected-untracked-inventory` on retry succeeds instead of refusing again

### Requirement: Publication Does Not Weaken Declaration Safety

Making the eligible-untracked-inventory digest obtainable MUST NOT make the untracked declaration requirement optional, implicit, or automatic. The declaration and freshness checks that prevent an undeclared untracked file from silently entering a frozen candidate and being delivered to a reviewer MUST remain exactly as strict as before this change.

#### Scenario: Undeclared untracked file is still refused
- GIVEN an eligible untracked file that the caller has not declared
- WHEN the caller attempts to freeze or advance the candidate without declaring it
- THEN the operation is still refused
- AND publishing the digest does not substitute for an explicit declaration
