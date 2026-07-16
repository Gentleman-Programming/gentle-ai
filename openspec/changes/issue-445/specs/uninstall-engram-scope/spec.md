# uninstall-engram-scope Specification

## Purpose

Engram cleanup scope for managed uninstall (`none` / `project` / `global`): fail-closed service, opt-in profile-aware TUI defaults, truthful confirm/result. MUST NOT force destructive Engram cleanup.

## Requirements

### Requirement: Engram Uninstall Scope Values

The system MUST represent Engram cleanup scope as exactly `none`, `project`, or `global`. `none` MUST be first-class.

#### Scenario: Recognized scopes accepted

- GIVEN managed uninstall includes Engram
- WHEN scope is `none`, `project`, or `global`
- THEN the system MUST accept it without remapping

### Requirement: None Scope Performs Zero Engram Ops

When scope is `none`, the service MUST plan and execute zero Engram cleanup ops (no project `.engram/` removal, no global Engram integration removal).

#### Scenario: None plans and runs no Engram cleanup

- GIVEN Engram selected and scope `none`
- WHEN the service plans and runs managed uninstall
- THEN Engram targets/ops MUST be zero
- AND project `.engram/` and global Engram integration MUST NOT be removed via Engram cleanup

### Requirement: Project and Global Remain Explicit Opt-In

The system MUST support `project` (workspace) and `global` (integration) Engram cleanup after a non-destructive default.

#### Scenario: User opts into project or global

- GIVEN Engram scope selection is available
- WHEN the user selects `project` or `global`
- THEN uninstall MUST apply that chosen scope

### Requirement: Fail-Closed Unknown Scope

The service MUST accept only `none`, `project`, and `global`. Unknown, empty, or invalid scope MUST NOT coerce to `global`; it MUST error or plan zero Engram ops.

#### Scenario: Unknown or invalid never becomes global

- GIVEN unrecognized, empty, or invalid Engram scope
- WHEN the service applies or plans scope
- THEN it MUST NOT map to `global`
- AND MUST reject or plan zero Engram cleanup ops

### Requirement: Profile-Aware Defaults to None

On profile-aware / Uninstall Scope Selection entry and refresh where Engram scope is relevant, the TUI MUST default scope to `none` (not `global`).

#### Scenario: Entry and refresh default to none

- GIVEN entry or components→profiles refresh into Uninstall Scope Selection with Engram relevant
- WHEN scope state initializes or refreshes
- THEN scope MUST be `none` and MUST NOT be forced to `global`

### Requirement: Full Mode May Retain Global When Engram Selected

Full uninstall modes MAY keep `global` when Engram is selected (design pins defaults). That MUST NOT force `global` on profile-aware paths that require `none`.

#### Scenario: Full-mode boundary

- GIVEN full uninstall with Engram selected
- WHEN default scope is resolved
- THEN the system MAY use `global` for full mode
- AND MUST NOT force that default onto profile-aware none-default entry paths

### Requirement: No Cleanup Option Visible

When Engram scope UI is shown, the system MUST show **No cleanup** for `none`. Option count and cursor MUST match rendered rows.

#### Scenario: No cleanup shown with navigable rows

- GIVEN Engram selected and scope selection shown
- WHEN the UI renders and the user navigates
- THEN **No cleanup** MUST appear for `none`
- AND option count/cursor MUST match rendered scope rows

### Requirement: Lifecycle Does Not Re-Arm Global Unexpectedly

Reset, re-entry, back-navigation, and profile-discovery failure MUST NOT re-arm `global` on profile-aware flows. Prefer reset only on mode entry / explicit refresh boundaries.

#### Scenario: Reset, re-entry, and discovery failure stay non-destructive

- GIVEN profile-aware flow with Engram scope relevant
- WHEN state resets, the user re-enters without choosing destructive scope, or profile discovery fails
- THEN scope MUST NOT be forced or restored to `global` solely due to those events

### Requirement: Confirm and Result Truthful for None

Confirm and result MUST be truthful for `none` and MUST NOT imply Global cleanup.

#### Scenario: Confirm and result for none

- GIVEN Engram selected and scope `none`, or uninstall completed with `none`
- WHEN confirm or result is shown
- THEN copy MUST NOT claim/imply Global Engram cleanup
- AND MUST reflect No cleanup / zero Engram cleanup ops
