# Gentle Shell — qualification debt handoff (Alan / Panda)

Context: the Linux/Windows install experience shipped via the chain ending in
#5644, plus follow-ups #5681 (Windows reconfigure replacement), #5692 (worker
env forwarding) and #5695 (channel overlay, open at time of writing). Three CI
lanes carry known debt. None of it blocks the install flow itself — the Organic
Runtime E2E (ubuntu, windows) is green on every merged head — but each lane
should be triaged on a real host.

## 1. Linux qualification (isolated guest)

Workflow: "Gentle Shell Linux qualification". The guest runs
`user-install.test` inside a bounded Docker (non-root uid, private tmpfs).
Four tests fail deterministically in the guest and pass on the runner, so the
root cause is guest-local: uid 1002 vs the runner's uid, umask semantics, and
RLIMIT_FSIZE on the guest tmpfs.

| Test | Failure |
|---|---|
| `TestUserExperienceEntryPreservesSelectionAndRefusal` | "transport admitted unsupported public effects" |
| `TestUserNativeReadbackAdmitsCurrentStockBundle/loose-binary-mode` | "foreign or mutated native state accepted" |
| `TestUserNativeReadbackAdmitsCurrentStockBundle/loose-manifest-mode` | same |
| `TestUserPNPMCodeSealPreservation/oversize` | `truncate …: file too large` (RLIMIT_FSIZE) |
| `TestUserStageOwnedAgentContext7` | "agent directory mode changed" (stat shows gid 1002, mode drift) |

Diagnostic angle: run the same binary in the guest with `-test.v` and dump
`os.Getuid/os.Getumask` at the failure points; verify whether the guest umask
and tmpfs size are contract inputs or incidental. The loose-mode acceptance in
`TestUserNativeReadbackAdmitsCurrentStockBundle` deserves priority: if
`privateNativeFile` genuinely accepts a 0644 file in some guest condition, that
is a real guard gap, not a test bug.

## 2. Windows Full Suite

Workflow: "Windows Full Suite" (scheduled/manual, full suite). Failing on
`main` at a77ca3a206 and 8d157d1aebc4.

**Product regressions from the GGA removal (priority):**

- `TestUserStageExperienceFilesRefusals/explicit_RDD_on|off` — refusal leaks
  files: `config/background-subagents.json` (and sometimes `persona.json`) are
  written before the refusal fires. On unix the writer order differs and the
  test passes; on Windows the guard must refuse before any write.
- `TestUserStageExperienceWindowsRefusesBeforeWrites/explicit_CodeGraph` —
  "unsupported stage admitted".
- `TestUserCodeGraphExperienceHistoricalAndRefusal`,
  `TestUserContext7ExperienceRefusesUnsupportedStage`,
  `TestUserExperienceSelectionRefusesUnsupportedProfileBeforeEffects` — the
  unix versions of these skip on darwin for the same underlying admission
  change; the Windows guards need the equivalent audit.

**Runner limitations (not product bugs):**

- `TestCopilotCLIAdapter_Detect` — `stat path = "\home\u\.copilot", want
  "/home/u/.copilot"`: path-separator handling in the adapter fixture.
- TUI flow tests (`shell_detected_test.go`, `shell_experience_*`,
  `shell_install_test.go`, `shell_main_channel_test.go`,
  `shell_gga_removed_test.go`) — refused by the kernel guard because
  `windows-latest` runners are Windows Server (`UserKernelCheck` requires a
  Windows 11 workstation). Either gate these tests on the kernel check passing
  or run the lane on a Windows 11 runner image.

## 3. macOS qualification

Workflow: "Gentle Shell macOS qualification". Owner: Panda (handoff document:
`docs/gentle-shell-macos-handoff.md` on the feature branch).

Status: installs, launches, versions (Pi 1.3.0, gentle-shell 1.3.0,
node v24.18.0), artifact pins and recovery all PASS.

Resolved: the residue checkpoint failed with "redirected home is not empty:
['.local']" because Separate installs publish `~/.local/bin/gentle-shell`
(7abb1eab5). The checkpoint now requires exactly that symlink, pointing at the
Separate target's `bin/gentle-shell`, and nothing else in HOME. The
`TestUserUnixChannelOverlay*` failures were a test-tag defect (the overlay is
Linux amd64 only); those tests now build only there, and a darwin test pins
the full-reconfigure fallback. The lane also runs `go test ./internal/tui/...`,
whose shell tests now use canonical temporary HOMEs.

## 4. Open PR

- #5695 — Windows in-place channel overlay (Stable ↔ Main swaps only
  `prefix/node_modules/gentle-pi`, retains the stock package as the restore
  source, atomically rebinds selection.json/manifest.json with compensated
  rollback). Compile-verified; native Windows execution pending. Unix parity
  (in-place channel overlay on Linux/Darwin) is the explicit follow-up.
- #5692 — merged: `GENTLE_AI_RECONFIGURE_TARGET` /
  `GENTLE_AI_SHELL_REGISTRY_HOME` are forwarded into the Windows worker env.
- #5681 — merged: Windows reconfigure replacement, darwin test cleanup.
