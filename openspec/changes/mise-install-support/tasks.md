# Tasks: mise Install Support

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~640 total (~90 PR1, ~265 PR2, ~235 PR3, ~50 PR4) |
| 400-line budget risk | High as a single PR; Low-Medium per chained unit below |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 -> PR 2 -> PR 3 -> (empirical gate) -> PR 4 |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main (decided — proposal.md: each stream is independently revertible and streams can land in any order) |

Decision needed before apply: No (resolved)
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Toolchain pins + CI drift guard | PR 1 | `./scripts/verify-mise-pins.sh` | N/A — bash/CI text only, no runtime path | revert `mise.toml`, script, CI step |
| 2 | Mise-managed detector + unit tests | PR 2 | `go test ./internal/update/upgrade/... -run Mise` | N/A — pure unit tests, no live mise binary | revert `mise.go` + `mise_ownership_test.go` |
| 3 | Executor preflight wiring + integration tests | PR 3 | `go test ./internal/update/upgrade/...` | existing `Execute()` test harness (`mise_preflight_test.go`) | revert wiring + `preflightMiseGentleAIUpgrades`; `mise.go` goes unreachable — revert together with Unit 2, or re-baseline `deadcode-ratchet.sh --update` |
| 4 | Empirical install verification (gate, not a PR) | none | `mise use -g github:Gentleman-Programming/gentle-ai@latest` | real mise + Docker (available in this environment) | N/A — no repo change |
| 5 | Documentation | PR 4 | N/A — docs only | N/A — prose only | revert doc diffs |

## Phase 1: Toolchain Pins & CI Drift Guard (Work Unit 1)

- [x] 1.1 Create `mise.toml` at repo root: `[tools]`, `go = "1.25.10"` (mirrors `go.mod:3`), `node = "24"` (mirrors `ci.yml:371`).
- [x] 1.2 Create `scripts/verify-mise-pins.sh`: `set -euo pipefail`, `die()` prefixed `mise pins:`, extracts the go pin from `go.mod` and node pin from `ci.yml`, requires exactly one match per extraction, compares both against `mise.toml`, pin-equality only — no Node version-floor check.
- [x] 1.3 Manually run the script against 4 cases: matching pins (pass); go drift (fail, actionable message); node drift (fail); a duplicated `node-version:` line in `ci.yml` (fail closed, not a silent first-match).
- [x] 1.4 Wire `./scripts/verify-mise-pins.sh` into `.github/workflows/ci.yml`'s `unit-tests` job, immediately after the "Check for new unreachable functions" step (~line 141), same "only Unit Tests is required on main" rationale comment.
- [x] 1.5 Verify: script passes against current repo state; diff touches only `mise.toml`, `scripts/verify-mise-pins.sh`, `ci.yml`.

## Phase 2: Mise-Managed Detector (Work Unit 2)

- [x] 2.1 RED: `internal/update/upgrade/mise_ownership_test.go` — table-driven (`t.Run(tt.name, ...)`, `t.TempDir()`, `t.Setenv`, no `t.Parallel()`), covering all 4 env-precedence rungs (`MISE_INSTALLS_DIR` > `MISE_DATA_DIR/installs` > `XDG_DATA_HOME/mise/installs` > `~/.local/share/mise/installs`), missing `HOME`, and a contained-vs-sibling-prefix path (`…/installsX/`). Must include these 3 threat-matrix cases verbatim: (a) empty/whitespace `MISE_INSTALLS_DIR` must NOT resolve a cwd-relative root; (b) a relative override is answered by `Contains`'s own `filepath.Abs`, never a string prefix; (c) a set-but-nonexistent root reports not-mise-managed, not an error.
- [x] 2.2 GREEN: create `internal/update/upgrade/mise.go` — unexported swappable `currentExecutableFn`/`userHomeDirFn` vars (default `os.Executable`/`os.UserHomeDir`), `miseInstallsRoot() string` (first **set**, non-blank env var wins), `runningBinaryIsMiseManaged() bool` via `pathidentity.Contains(miseInstallsRoot(), currentExecutableFn())`.
- [x] 2.3 Verify: `go test ./internal/update/upgrade/... -run Mise` green.

## Phase 3: Executor Preflight Wiring (Work Unit 3)

- [ ] 3.1 RED: `internal/update/upgrade/mise_preflight_test.go` (reuses `Execute()` harness: `makeResult`, `linuxProfile`, `execCommand`/`snapshotCreator` injection) — mise-managed gentle-ai is `UpgradeSkipped` with hint `` mise-managed install — run `mise upgrade gentle-ai` instead; replacing the binary in place would desync the version mise tracks `` (asserted verbatim); a second requested tool in the same invocation still upgrades; `BackupID == ""` when the mise skip empties the executable set; non-mise install is byte-identical to today; mise + Windows preflights compose without double-skipping.
- [ ] 3.2 GREEN: add `preflightMiseGentleAIUpgrades(executable []executableUpdate, profile system.PlatformProfile) ([]executableUpdate, []ToolUpgradeResult)` to `executor.go`, mirroring `preflightWindowsGentleAIUpgrades`'s signature, gated on `r.Tool.Name == "gentle-ai"` plus `runningBinaryIsMiseManaged()` only (no `effectiveMethod` filter — containment already excludes non-mise roots).
- [ ] 3.3 GREEN: compose sequentially at `executor.go:462-465` inside the existing `!dryRun` guard, mise first: `executable, miseSkips = preflightMiseGentleAIUpgrades(...)`; `executable, windowsSkips = preflightWindowsGentleAIUpgrades(...)`; `preflightSkips = append(miseSkips, windowsSkips...)`.
- [ ] 3.4 Verify: `go test ./internal/update/upgrade/...` green; `--dry-run` executable-set preview stays unchanged.

## Phase 4: Empirical Install Verification (gate before Phase 5)

- [ ] 4.1 Run `mise use -g github:Gentleman-Programming/gentle-ai@latest` in a clean environment (mise + Docker available here); confirm mise selects a real platform tarball, not the decoy `gentle-ai-review-provider-contract-<semver>.tar.gz`; confirm `os.Executable()` on the installed binary resolves under an `installs/` path. If the wrong asset is selected, Phase 5's doc tasks gain an inline `asset_pattern`-equivalent caveat instead of the plain command — record the outcome before starting Phase 5.

## Phase 5: Documentation (Work Unit 5, gated by Phase 4's outcome)

- [ ] 5.1 `README.md`: add mise to the "Alternative install and scope options" `<details>` block (127-190), one option equal to Homebrew/`go install`/Scoop, no preferred-path language, using the Phase-4-verified command form.
- [ ] 5.2 `docs/quickstart.md`: add a mise entry using the Phase-4-verified command form.
- [ ] 5.3 `docs/platforms.md`: add mise to the support matrix (7-13).
- [ ] 5.4 `CONTRIBUTING.md` Prerequisites (~104-108): add Node as a named prerequisite, naming mise as one available, non-mandatory option — closes the existing Node-mention gap; no other CONTRIBUTING.md change.

## Phase 6: Final Verification

- [ ] 6.1 `go test ./...` green.
- [ ] 6.2 `go run ./internal/gofmtcheck` clean.
- [ ] 6.3 Confirm every proposal.md Success Criteria checkbox is satisfied.
