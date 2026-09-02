# Delta for rdd-sdd-receipt-consumption

## ADDED Requirements

### Requirement: Ledger-Owned Untracked-Freshness Refusal For Finish And Settle

For `finish` and `settle`, the untracked-freshness refusal MUST originate from SDD's runtime ledger, which recomputes the current live inventory and reports a usable digest, rather than from a duplicate CLI-side precondition that performs the same check ahead of the ledger and reports no digest. The CLI MUST NOT run a separate finish/settle untracked-freshness precondition that can refuse before the ledger is reached.

#### Scenario: Finish refusal carries the ledger's message
- GIVEN an untracked-freshness mismatch is detected during `finish`
- WHEN the refusal is reported to the caller
- THEN the message and digest come from the runtime ledger's refusal path
- AND no separate generic CLI precondition refusal precedes it

#### Scenario: Settle refusal carries the ledger's message
- GIVEN an untracked-freshness mismatch is detected during `settle`
- WHEN the refusal is reported to the caller
- THEN the message and digest come from the runtime ledger's refusal path
- AND no separate generic CLI precondition refusal precedes it

### Requirement: Begin And Acquire Keep Their Own Preflight

`begin` and `acquire` MUST retain their existing CLI-side untracked-freshness preflight unchanged, because no ledger-side equivalent exists for those operations. Removing or weakening it is out of scope for the finish/settle ledger-ownership change.

#### Scenario: Begin preflight is unchanged
- GIVEN an untracked-freshness mismatch at `begin`
- WHEN the caller invokes `begin`
- THEN the existing CLI preflight still refuses with its own check
- AND no ledger-side substitute is introduced for `begin`

#### Scenario: Acquire preflight is unchanged
- GIVEN an untracked-freshness mismatch at `acquire`
- WHEN the caller invokes `acquire`
- THEN the existing CLI preflight still refuses with its own check
- AND no ledger-side substitute is introduced for `acquire`

### Requirement: Refusal Ownership Change Does Not Weaken The Check

Removing the duplicate finish/settle CLI precondition MUST NOT relax, bypass, or skip the untracked-freshness check itself. The same freshness comparison MUST still run and still refuse on a genuine mismatch; only the duplicate CLI-side evaluation and its inferior message are removed.

#### Scenario: Genuine mismatch still refuses after the CLI precondition is removed
- GIVEN an untracked-freshness mismatch that would have refused under the old CLI precondition
- WHEN `finish` or `settle` runs after the duplicate precondition is removed
- THEN the operation still refuses
- AND the refusal is reported through the ledger's check instead of a duplicate one
