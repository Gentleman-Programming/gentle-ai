# Persona output-style transitions

Delta for the new `persona-output-style-transitions` capability (#3163): failed removal of a retired style file restores pre-transition state via the #3161 rollback boundary. `persona-behavior-contract` content is unchanged.

## ADDED Requirements

### Requirement: REQ-REMOVE-HOOK — Injectable removal hook

Removal of a retired persona output-style file MUST go through an injectable removal seam so tests can force removal failure. A successful transition MUST leave the selected persona's style file present, the retired file absent, and settings selecting the selected persona.

#### Scenario: SEN-SUCCESS-TRANSITION — Gentleman to Neutral completes

- GIVEN Gentleman is installed (`gentleman.md` present, settings select Gentleman)
- WHEN install or sync applies Neutral
- THEN `neutral.md` exists, `gentleman.md` is absent, settings select Neutral, and the command exits 0

#### Scenario: SEN-INJECTED-REMOVAL-FAILURE — Removal failure is injectable

- GIVEN a test forces the removal seam to fail
- WHEN a persona transition removes the retired style file
- THEN the removal failure surfaces from the transition

### Requirement: REQ-ROLLBACK-PROPAGATION — Removal failure reaches pipeline rollback

If removal of the retired style file fails after the new style and settings are written, the failure MUST propagate to the pipeline rollback boundary and restore the pre-transition style file and settings. A partial transition MUST NOT remain after rollback.

#### Scenario: SEN-ROLLBACK-RESTORES — Previous style and settings restored

- GIVEN a Gentleman-to-Neutral transition writes new style and settings, then the removal seam fails
- WHEN the pipeline rollback completes
- THEN the pre-transition style file and settings are restored byte-for-byte

#### Scenario: SEN-NO-PARTIAL-STATE — No partial Neutral remains

- GIVEN the new Neutral style file and settings were written before the removal failure
- WHEN rollback finishes
- THEN on-disk state contains no partial Neutral style file or settings entry

### Requirement: REQ-EXIT-WARNING — Exit 0 with explanatory warning

After a removal failure that was fully rolled back, the command MUST exit 0 and MUST print a warning explaining that the previous style file and settings were restored and nothing was half-applied. It MUST neither fail hard nor complete silently.

#### Scenario: SEN-EXIT-ZERO-ON-ROLLBACK — Rolled-back failure exits 0

- GIVEN a removal failure was rolled back successfully
- WHEN the command finishes
- THEN the exit code is 0

#### Scenario: SEN-WARNING-EXPLAINS-ROLLBACK — Warning names what was restored

- GIVEN a removal failure was rolled back successfully
- WHEN the command output is inspected
- THEN it contains a warning stating the previous style file and settings were restored
- AND the warning states that nothing was half-applied

### Requirement: REQ-NO-RETRY — No removal retry loop

On removal failure the system MUST NOT retry the removal and MUST roll back directly. The retired file removal MUST be attempted at most once per transition.

#### Scenario: SEN-NO-RETRY-LOOP — Persistent failure rolls back immediately

- GIVEN the removal seam fails persistently
- WHEN a transition runs
- THEN the retired style file removal is attempted exactly once
- AND rollback begins immediately without a retry

### Requirement: REQ-NOOP — Second successful run is a semantic no-op

After a successful transition, re-running install or sync with the same selection MUST be a semantic no-op: managed output-style files and persisted settings MUST remain unchanged and the command MUST exit 0.

#### Scenario: SEN-IDEMPOTENT-SECOND-RUN — Re-run leaves state unchanged

- GIVEN a transition to Neutral succeeded and is on disk
- WHEN install or sync runs again with the same selection
- THEN no managed style file or settings entry changes
- AND the command exits 0

### Requirement: REQ-USER-FILES — User-owned files and unrelated settings preserved

The transition MUST remove only the retired Gentle-AI-owned style file and MUST NOT alter user-owned files or unrelated settings keys, on success or during rollback.

#### Scenario: SEN-USER-FILE-PRESERVED — User files survive success and rollback

- GIVEN a user-owned style file or an unrelated settings key exists alongside managed persona state
- WHEN a transition succeeds or rolls back
- THEN the user-owned file and the unrelated settings key remain byte-identical

### Requirement: REQ-PARITY — Install and sync share the transition contract

Install and sync MUST apply the same transition contract: the same removal seam, the same rollback propagation and restoration, and the same warning-plus-exit-0 behavior for rolled-back removal failures.

#### Scenario: SEN-INSTALL-SYNC-PARITY — Both pipelines roll back identically

- GIVEN the same removal failure is injected for each pipeline
- WHEN install and sync each run
- THEN both restore the previous style file and settings, print the warning, and exit 0
