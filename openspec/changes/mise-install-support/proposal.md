# Proposal: mise Install Support

## Intent

mise has no supported path in this repo. Contributors reconstruct the toolchain by hand — Go from `go.mod:3`, Node from `ci.yml:368-371` — and `CONTRIBUTING.md:104-108` never mentions Node at all, even though CI needs Node 24 and `gentle-ai install` needs Node 18+ at runtime. Users who install through mise get worse: the self-updater replaces the binary in place, silently desyncing the version mise believes it manages.

This change makes mise first-class inside this repo: one `mise install` provisions the toolchain, CI proves the pins never drift, and `gentle-ai upgrade` stops fighting mise. Sibling registry PRs (`aquaproj/aqua-registry#59466`, `jdx/mise#12444`) have both merged and shipped in mise `v2026.9.0`, so the registry short name resolves and is what's documented here.

## Scope

### In Scope

| # | Stream | Shape |
|---|---|---|
| 1 | Contributor toolchain | repo-root `mise.toml` pinning `go = "1.25.10"` (mirrors `go.mod`) and `node = "24"` (mirrors `ci.yml`), plus a drift-check script wired into the **`unit-tests`** job — the `scripts/deadcode-ratchet.sh` placement at `ci.yml:141`, chosen there because only Unit Tests is a required check on main |
| 2 | Upgrade safety | new `internal/update/upgrade/mise.go` detects a mise-managed gentle-ai binary; a pre-backup preflight in `executor.go` skips **only** gentle-ai's own upgrade, with a manual hint pointing at `mise upgrade gentle-ai` |
| 3 | Documentation | mise as a documented install method in `README.md` (alternatives `<details>`, 127-190), `docs/quickstart.md`, the `docs/platforms.md` matrix (7-13), and `CONTRIBUTING.md` Prerequisites — closing the Node gap through mise |

Documented command: `mise use -g gentle-ai@latest` (the registry short name; verified empirically against mise `v2026.9.0`).

### Out of Scope

- Stale "v2.3.0 stable / v2.4.0-rc.1" text in `README.md` / `docs/quickstart.md` — tracked separately.
- Any `CONTRIBUTING.md` change beyond the mise-linked Node prerequisite.
- Windows mise support — no Windows release archives exist yet.

## Resolved decisions (do not relitigate downstream)

| Decision | Resolution |
|---|---|
| Abort semantics | **Per-tool skip**, not whole-command abort. Mirror `preflightWindowsGentleAIUpgrades` (`executor.go:583-612`): filter the gentle-ai candidate out of the executable set and replace it with `ToolUpgradeResult{Status: UpgradeSkipped, ManualHint: ...}` via `AsManualFallback` / `*ManualFallbackError` (`types.go:25-41`). Other tools in the same invocation still upgrade. A hard abort would block unrelated upgrades and diverges from every manual-fallback precedent here. |
| Containment test | `internal/pathidentity.Contains(root, path)` (`identity.go:83-102`) against `os.Executable()`, **not** a hand-rolled `EvalSymlinks` + prefix check. `Contains` handles case-insensitive and Unicode-equivalent filesystems that `homebrew.go`'s `pathWithinPrefix` misses, and returns `false` when the root does not exist — the correct "mise not installed" answer with no extra guard. |
| Install-root resolution | `$MISE_INSTALLS_DIR` → `$MISE_DATA_DIR/installs` → `$XDG_DATA_HOME/mise/installs` → `~/.local/share/mise/installs`. `MISE_INSTALLS_DIR` is mise's own higher-priority override. |

## Capabilities

### New Capabilities

- `mise-toolchain-support`: repo-root mise pins that mirror the authoritative Go/Node sources, the CI drift guard, and mise as a documented install method.
- `mise-managed-upgrade-safety`: detecting a mise-managed gentle-ai binary and skipping its self-upgrade with an actionable manual hint.

### Modified Capabilities

None.

## Approach

Streams are independent and can land in any order; 1 and 3 are additive, 2 is the only behavioral change.

**Stream 1** — greenfield `mise.toml` (nothing exists to reconcile) plus a bash guard in the repo idiom (`set -euo pipefail`, `repo_root` resolution, actionable failure message) that reads the pins back from `go.mod` and `ci.yml` and fails on mismatch. CI already uses `go-version-file: go.mod` everywhere, so `mise.toml` is a *second* independent pin — the guard is what keeps it honest.

**Stream 2** — a new preflight sibling to the Windows one, running before the backup snapshot (`executor.go:467-504`) and the `executeOne` loop (`executor.go:537-570`). Detection lives in a new `internal/update/upgrade/mise.go`, not in `system.PlatformProfile`: mise-managed-ness is self-referential to the running process, not a general machine capability. Swappable package-level func vars (`currentExecutableFn`) for tests, matching the package's existing idiom; test file shaped like `homebrew_ownership_test.go` / `effective_method_routing_test.go`.

**Stream 3** — additive doc entries only, alongside Homebrew and `go install` as another alternative.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `mise.toml` | New | Go + Node pins at repo root |
| `scripts/` | New | drift-check script (naming decided in design) |
| `.github/workflows/ci.yml` | Modified | one step in `unit-tests`, near `ci.yml:141` |
| `internal/update/upgrade` | New + Modified | `mise.go`, preflight wiring in `executor.go`, new test file |
| `internal/pathidentity` | Consumer | first import from `internal/update/upgrade`; no boundary violation (`executor.go:5-8` forbids only `pipeline`, `planner`, `cli`) |
| `README.md`, `docs/quickstart.md`, `docs/platforms.md`, `CONTRIBUTING.md` | Modified | documented install method + Node prerequisite |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| mise `github:` asset autodetection picks the decoy `gentle-ai-review-provider-contract-<semver>.tar.gz` over a platform tarball | Low | Inferred from mise docs, **not empirically tested**. `sdd-tasks` MUST carry an explicit manual-verification task running the documented command end to end. If it fails, the doc command gains an inline `asset_pattern` caveat. |
| `os.Executable()` (preflight) and `lookPathFn(r.Tool.Name)` (`download.go:108`, the write path) resolve different files when multiple `gentle-ai` binaries are on PATH | Low | Same file in the normal case; design notes the divergence and decides whether the hint should mention inspecting PATH duplicates. |
| `mise.toml` pins drift from `go.mod` / `ci.yml` | Med | Exactly what stream 1's guard exists to prevent; it must sit in a required status check to matter. |

## Rollback Plan

Each stream is an independently revertible commit. Stream 1: delete `mise.toml`, the script, and the CI step — nothing else reads them. Stream 2: revert the preflight call in `executor.go`; the unreferenced `mise.go` degrades to dead code and can be removed separately. Stream 3: docs revert cleanly with zero runtime impact.

## Dependencies

- None blocking. The sibling `aquaproj/aqua-registry` and `jdx/mise` PRs have both merged and shipped, so the short name they added is now the documented command rather than a parallel stream.

## Success Criteria

- [ ] `mise install` at repo root provisions Go 1.25.10 and Node 24 with no manual steps.
- [ ] Editing `go.mod`'s Go directive or `ci.yml`'s Node pin without updating `mise.toml` fails the `unit-tests` job.
- [ ] `gentle-ai upgrade` on a mise-managed binary reports gentle-ai as skipped with a `mise upgrade gentle-ai` hint, while every other requested tool still upgrades in the same invocation.
- [ ] A non-mise install is unaffected: no skip, no behavior change.
- [ ] The documented `mise use -g gentle-ai@latest` command is manually verified to install a working binary before merge.
- [ ] `CONTRIBUTING.md` Prerequisites names Node and points at mise as the one-step path.
