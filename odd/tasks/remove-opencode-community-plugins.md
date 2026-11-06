# Remove external OpenCode community plugins

## Objective and authorization
Remove external community plugins from installation and TUI, including OpenCode Community Plugins and Uninstall OpenCode Plugin menu entries. Preserve OpenCode platform support, built-in Gentle Logo/component UI/CLI, SDK preflight, managed runtime plugins, CodeGraph and other community tools. Preserve CLI uninstall of legacy external IDs. Do not delete existing user registrations/packages automatically.

## Problem and rationale
Welcome menu removal alone leaves installation picker detours and selected-plugin apply/update paths active. Retire the external inventory while retaining uninstall-only metadata.

## Delivery
Feature branch: refactor/remove-opencode-community-plugins.
Strategy: exception-ok; user explicitly requested one PR with size exception. Human authorized current gh session for target repository reads, HTTPS push, PR creation and merge after required checks. PR #5200 closes approved issue #5199; type:breaking-change and size:exception explicitly authorized and confirmed.
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

- [x] T2 Remove newly unreachable InstallPaths left by T1 and pass the deadcode ratchet.
  - Status: done; functional checks and work-unit commit complete.
  - Route: delegated writer; preparation trigger and CI correction.
  - Scope: internal/components/opencodeplugin/plugin.go and its exact tests only; never weaken deadcode baseline.
  - RED: CI Unit Tests failed ./scripts/deadcode-ratchet.sh on InstallPaths (run 37001728573).
  - Checks: ./scripts/deadcode-ratchet.sh; go test ./internal/components/opencodeplugin ./internal/update; go test ./internal/cli ./internal/tui; git diff --check.
  - Acceptance: remove obsolete selection-specific path helper without affecting InstallComponentPaths/logo/uninstall; no newly unreachable functions; applicable focused checks pass.
  - Implementation: removed InstallPaths and obsolete test assertion, 2 files +0/-32. Worker normalized and passed plugin/update and CLI/TUI tests plus diff check. Local ratchet unavailable with GOPROXY=off (tool lookup failure, not code finding); human authorized public proxy.golang.org/sum.golang.org reads without credentials. Independent verifier now running with per-command online environment; no baseline/config changes.
  - Verification: independent ratchet GREEN with no newly unreachable functions; 3 previously baselined entries gone/reachable, baseline unchanged. Offline plugin/update and CLI/TUI tests passed (CLI 190.846s); git diff --check passed. Named InstallComponentPaths was an inaccurate planning reference (no such symbol); actual logo component/uninstall code unchanged.
  - Commit: 23028fb91eee16783633df2a87a72afe8ad0d9a2 — fix(opencode): remove unreachable community plugin path helper.
  - Review/delivery: final committed candidate native review and required CI pending; parent will not edit source after review.
  - Isolation: human authorized /home/gentleman/work/gentle-ai-worktrees/pr-5200-opencode-community-plugins; main checkout belongs to unrelated dirty fix/4994 branch and must remain untouched.

## Evidence and progress
Read-only explorer mapped source and tests using CodeGraph first. Parent spot check confirmed statusline active definition and legacy SDD uninstall metadata in internal/components/opencodeplugin/plugin.go.
Initial worker stopped before edits because its safety policy prohibits physical deletion. Human selected package-only placeholders; writer resumed with same allowed surfaces. Parent will delete the five obsolete screen files after writer returns and rerun affected checks. Implementation now removes external install/TUI/update paths (+157/-1995 before physical deletions). Observed RED: external install succeeded, statusline was in updater inventory, welcome actions and preset detours remained. Partial GREEN: external refusal and welcome absence passed. Update/TUI stale counts corrected but rerun pending; CLI failed compilation due to obsolete selected-plugin fixture in internal/cli/opencode_logo_v2_test.go. Human authorized that exact additional test path. Parent physically deleted all five package-only obsolete screens. Writer normalized changed Go files and all six verification commands passed, including go test -short ./.... Final source diff: 21 files, +225/-2053 (2278 authored lines). Manual interactive TUI check not run. Native review review-705f5ee132137403: all four reviewers admitted, approved, acknowledgement completed with authority burned for target sha256:eb0f8cf2c8e6c814e31dfaad8308d280f3bc8dae1663db002cbf87cb30d7dc26. First consent expired without lineage; fresh consent created current review. ASSESS failed on untracked task document and returned high-risk fallback requiring independent verification. Independent verifier reran all six commands successfully without functional findings (CLI 194.466s; app 34.607s; TUI 2.009s). Parent spot check go test ./internal/components/opencodeplugin ./internal/update and git diff --check passed. No final failing functional checks; ASSESS unavailable, handled via high-risk fallback. Interactive manual TUI skipped.

## Next step
T1 source and final committed slice were approved and acknowledged (final lineage review-db25d90ea1faa380). PR #5200 published; required metadata and all three E2E passed, but Unit Tests failed on newly unreachable InstallPaths. Correct that remnant in the authorized isolated worktree, verify and review the new candidate, publish the correction, then merge only after required CI passes. Manual interactive TUI check remains skipped.
