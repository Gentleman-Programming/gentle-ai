# Profile-Only Uninstall Specification

## Problem Statement

The uninstall flow currently combines OpenCode SDD profile removal with Engram cleanup. A user who selects a profile can therefore delete unrelated Engram data, even though profile removal should affect only that profile's entries in `opencode.json`.

## Goals

- [ ] Remove selected named OpenCode SDD profiles without invoking managed-component uninstall.
- [ ] Preserve Engram files, configuration, and all non-selected profile/default-agent entries.
- [ ] Keep the existing Uninstall flow's profile confirmation/result experience.

## Out of Scope

| Feature | Reason |
| --- | --- |
| Redesigning component uninstall selections | The bug is the profile-removal path, not the general component picker. |
| Changing CLI uninstall semantics | The issue is limited to the TUI. |
| Adding backup snapshots to profile management | Existing direct profile deletion is the established behavior and is not changed here. |

---

## Assumptions & Open Questions

| Assumption / decision | Chosen default | Rationale | Confirmed? |
| --- | --- | --- |
| Selecting one or more named profiles in the uninstall flow means profile-only removal. | Call the existing profile-removal capability directly; do not call the managed uninstall callback. | This is the only interpretation that guarantees no Engram cleanup and matches the issue's expected behavior. | yes — issue #445 and approved implementation request |
| Profile-only removal remains TUI-local. | No CLI flags or component-picker behavior change. | The issue identifies only the TUI flow. | yes — scoped issue |
| A failed multi-profile deletion stops and reports the error. | Preserve existing deletion error behavior; do not silently continue. | Prevents claiming a complete removal after a partial failure. | assumption |

**Open questions:** none — all resolved or logged above.

## User Stories

### P1: Remove a profile without deleting Engram ⭐ MVP

**User Story**: As an OpenCode user, I want to remove selected SDD profiles from the uninstall flow so that stale profile agents disappear without affecting Engram or unrelated managed configuration.

**Why P1**: This prevents unintended deletion of persistent project or global memory configuration.

**Acceptance Criteria**:

1. WHEN a user confirms one or more selected profiles on the uninstall profile-selection screen, THEN the system SHALL remove only the selected profiles' SDD agent keys from `opencode.json`.
2. WHEN profile-only removal is confirmed, THEN the system SHALL NOT invoke the managed-component uninstall path and SHALL NOT delete or rewrite Engram artifacts.
3. WHEN a selected profile is removed, THEN default SDD agents, unselected profiles, and unrelated `opencode.json` content SHALL remain unchanged.
4. WHEN profile removal returns an error, THEN the TUI SHALL show the failure rather than reporting an uninstall success.

**Independent Test**: Configure selected and unselected profile names, invoke the profile-only operation, and assert the selected names are sent only to the profile-removal callback while the component-uninstall callback is not called.

## Edge Cases

- WHEN no named profile is selected, THEN the normal managed-component uninstall flow SHALL remain available.
- WHEN a selected profile cannot be removed, THEN the operation SHALL surface its error and SHALL NOT claim success.

## Requirement Traceability

| Requirement ID | Story | Phase | Status |
| --- | --- | --- | --- |
| PROF-01 | P1: profile-only removal | Execute | Implementing |
| PROF-02 | P1: protect Engram | Execute | Implementing |
| PROF-03 | P1: preserve unrelated configuration | Execute | Implementing |
| PROF-04 | P1: surface errors | Execute | Implementing |

## Success Criteria

- [ ] A selected profile is removed without calling managed uninstall or touching Engram.
- [ ] Focused TUI tests and the complete Go test suite pass.
