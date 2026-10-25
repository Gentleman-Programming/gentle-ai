# Windows Full Suite green before the gentle-ai release (2026-10-12)
Objective: make `.github/workflows/windows-full-suite.yml` green on main so gentle-ai and gentle-pi can be released. Branch: `fix/windows-full-suite-oct12`. Delivery: single PR (test-only, small). Test runner: `go test` (Windows proof via `gh workflow run windows-full-suite.yml --ref <branch>`).

## Specs
- S1: "Saca release de gentle ai y de gentle Shell" — releases require Windows Full Suite green on the exact origin/main head (user choice: "Arreglar Windows primero").
- S2: Fix every failing test of run 38201358953 (head 8af3aa9d8) without weakening product security checks: G1 shellinstaller `TestUserWindows*` ("foreign Windows owner"), G2 mode asserts (theme, uninstall), G3 gen_digests unreadable asset, G4 legacyassets `gen` without `.exe`, G5 opencode activation rollback on Windows host, G6 opencode locked settings subtest, G7 stale managed plugin fixture, G8 doctor quoting of `RUNNER~1`, G9 consent invocation split with `strings.Fields`.
- S3 (decision): product refuses elevated terminals and Windows Server on purpose (`internal/shellinstaller/user_install_windows.go:39-43`); G1 is fixed in tests by making the elevated test process create objects owned by its user SID (TokenOwner), skipping explicitly when that is impossible. Product code unchanged.

## Tasks
- [x] T1 (S1-S3) issue #5470 — inline
- [x] T2 (S2 G2-G9) test-only fixes in components, opencode, cli — inline — 8f46493e4
- [x] T3 (S2-S3 G1) shellinstaller Windows TestMain owner — inline — 8f46493e4, 4b4edcf5a
- [ ] T4 (S1) Windows Full Suite dispatch green on branch, PR, merge — inline
- [ ] T5 (S1) gentle-ai release, then gentle-pi pin bump + release

## Log
- L1 (2026-10-12, user): "Saca release de gentle ai y de gentle Shell"
- L2 (2026-10-12, user choice): "Arreglar Windows primero" — fix the Windows Full Suite before releasing gentle-ai.
- L4 (2026-10-12, user): "Estoy mejorando gentle shell. Así que este release no todavía" — releases paused; the Windows Full Suite fix continues.
- L3: Windows Full Suite red on 100 consecutive runs since 2026-10-05; last fix was PR #5174 (issue #5170). Root-cause map by read-only explorer; G1 classification corrected by parent: elevation refusal is intended product behavior.
- L5: Windows Full Suite run 38227003006 on 26871264d: 11/11 shards success (cli-n-o split into cli-n and cli-o after it hit the 1800s go-test timeout; shellinstaller also needed the long temp path). Slowest shard cli-a-m 26m18s of the 30m go-test timeout. T4 waits for merge decision; T5 paused (L4).
