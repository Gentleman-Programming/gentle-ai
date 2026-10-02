# Remove external OpenCode community plugins

## Objective and authorization
Remove external community plugins from installation and TUI, including OpenCode Community Plugins and Uninstall OpenCode Plugin menu entries. Preserve OpenCode platform support, built-in Gentle Logo/component UI/CLI, SDK preflight, managed runtime plugins, CodeGraph and other community tools. Preserve CLI uninstall of legacy external IDs. Do not delete existing user registrations/packages automatically.

## Problem and rationale
Welcome menu removal alone leaves installation picker detours and selected-plugin apply/update paths active. Retire the external inventory while retaining uninstall-only metadata.

## Delivery
Feature branch: refactor/remove-opencode-community-plugins.
Strategy: exception-ok; user explicitly requested one PR with size exception. No push or PR creation authorized.
Forecast: 100–250 additions / 900–1,600 deletions; mostly removal of obsolete UI and tests. One coherent work-unit commit with tests. Running authored count: 2278 source/test diff lines in work-unit commit 73f2652c; single source slice for future PR.

## Tasks
- [x] T1 Remove external community offerings and associated TUI flows; verify regression protection and close the work unit.
  - Status: done.
  - Route: delegated writer; multi-file and preparation triggers (about 20 paths).
  - Acceptance: both named menu entries absent; installation never offers or installs external plugins; active update inventory excludes statusline; legacy external installation refuses without writes; legacy CLI uninstall works; logo and managed runtime plugins remain supported; normal TUI forward/Back/Esc flows remain valid.
  - Applicable test-first: add regression assertions, observe focused RED, implement GREEN, normalize before final checks.
  - Checks: go test ./internal/components/opencodeplugin ./internal/update; go test ./internal/tui ./internal/tui/screens; go test ./internal/cli ./internal/app; go test ./internal/update/upgrade ./internal/components/opencoderuntimeplugins; go test -short ./...; git diff --check.
  - Runtime proof: deterministic TUI Model.Update navigation and temp-directory installer/uninstaller fixtures; interactive manual TUI pending unless runnable noninteractive coverage suffices.
  - Native review: enabled globally; inspect/start after normalized implementation and functional checks, follow provider authority.
  - Commit: 73f2652cb65e36292d766839a4db0f17ce0faaad — refactor(opencode): retire external community plugin installation and TUI.
  - Rollback boundary: remove this work unit to restore external community offering without touching unrelated OpenCode infrastructure.

## Evidence and progress
Read-only explorer mapped source and tests using CodeGraph first. Parent spot check confirmed statusline active definition and legacy SDD uninstall metadata in internal/components/opencodeplugin/plugin.go.
Initial worker stopped before edits because its safety policy prohibits physical deletion. Human selected package-only placeholders; writer resumed with same allowed surfaces. Parent will delete the five obsolete screen files after writer returns and rerun affected checks. Implementation now removes external install/TUI/update paths (+157/-1995 before physical deletions). Observed RED: external install succeeded, statusline was in updater inventory, welcome actions and preset detours remained. Partial GREEN: external refusal and welcome absence passed. Update/TUI stale counts corrected but rerun pending; CLI failed compilation due to obsolete selected-plugin fixture in internal/cli/opencode_logo_v2_test.go. Human authorized that exact additional test path. Parent physically deleted all five package-only obsolete screens. Writer normalized changed Go files and all six verification commands passed, including go test -short ./.... Final source diff: 21 files, +225/-2053 (2278 authored lines). Manual interactive TUI check not run. Native review review-705f5ee132137403: all four reviewers admitted, approved, acknowledgement completed with authority burned for target sha256:eb0f8cf2c8e6c814e31dfaad8308d280f3bc8dae1663db002cbf87cb30d7dc26. First consent expired without lineage; fresh consent created current review. ASSESS failed on untracked task document and returned high-risk fallback requiring independent verification. Independent verifier reran all six commands successfully without functional findings (CLI 194.466s; app 34.607s; TUI 2.009s). Parent spot check go test ./internal/components/opencodeplugin ./internal/update and git diff --check passed. No final failing functional checks; ASSESS unavailable, handled via high-risk fallback. Interactive manual TUI skipped.

## Next step
Implementation, independent verification, parent spot check, and work-unit commit are complete. Future delivery: one PR with user-approved size exception; no push or PR created. Native review approved and acknowledged; no further bound STATUS. Manual interactive TUI check remains optional/skipped.
