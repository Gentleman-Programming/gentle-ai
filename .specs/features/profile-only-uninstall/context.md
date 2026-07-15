# Profile-Only Uninstall Context

**Gathered:** 2026-07-15
**Spec:** `.specs/features/profile-only-uninstall/spec.md`
**Status:** Ready for implementation

## Feature Boundary

The TUI's selected-profile path removes only the selected named OpenCode SDD profile agents from `opencode.json`. It does not perform an Engram or managed-component uninstall.

## Implementation Decisions

### Operation boundary

- Reuse the existing `sdd.RemoveProfileAgents` capability already used by the dedicated profile-management screen.
- Add a TUI callback for profile-only removal, keeping the managed uninstall callback for component uninstall only.

### Interaction and result

- Keep the current profile-selection and confirm screens; do not show Engram cleanup as part of profile-only removal.
- Reuse the uninstall result screen for success/error feedback, with selected profile names as the observable result.

### Failure and data safety

- Stop at the first removal error and surface it through the existing TUI error/result route.
- Do not introduce a backup mechanism in this bounded fix; direct profile deletion already uses this behavior.

## Deferred Ideas

- Review default component selections in a separate issue; this change guarantees the selected-profile action itself cannot invoke component or Engram cleanup.
