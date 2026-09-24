# Design: mise Install Support

## Technical Approach

Three seams, no new dependency, no wire-contract change. Stream 2 is the only
behavioral one: a new detector file plus one composition site in `Execute`.
Streams 1 and 3 add files nothing else reads. Nothing here is a TUI flow, so the
`rules.design` sequence-diagram clause does not apply.

    os.Executable() ──→ pathidentity.Contains(miseInstallsRoot(), exe)
                                   │
              false ───────────────┴─────────────── true
                │                                     │
        candidate stays in `executable`      ToolUpgradeResult{UpgradeSkipped,
        (backup + executeOne as today)        ManualHint: mise upgrade …}

## Stream 2a — detection (`internal/update/upgrade/mise.go`)

Two unexported functions and two swappable vars. Unexported is a constraint, not
taste: `scripts/deadcode-ratchet.sh` fails on **new** unreachable functions, so
everything this change adds must be reachable from `./cmd/gentle-ai`.

```go
var currentExecutableFn = os.Executable
var userHomeDirFn = os.UserHomeDir

// miseInstallsRoot resolves mise's install root using mise's own precedence.
// "" means unresolvable — then no binary can be mise-managed.
func miseInstallsRoot() string

// runningBinaryIsMiseManaged reports whether the executable backing THIS
// process lives under that root.
func runningBinaryIsMiseManaged() bool
```

Precedence: `MISE_INSTALLS_DIR` → `MISE_DATA_DIR/installs` → `XDG_DATA_HOME/mise/installs`
→ `~/.local/share/mise/installs`. The first **set** variable wins, and an empty or
whitespace-only value counts as unset — otherwise an exported-but-empty override
resolves the root to a cwd-relative `installs`, and the containment test answers a
different question than the one asked. One root, one `pathidentity.Contains(root, exe)`
call, which already returns false for a root that does not exist: the correct
"mise not installed" answer with no extra guard.

Return `bool`, not a result struct: the hint is a constant and nothing downstream
consumes the root.

## Stream 2b — wiring (`executor.go:462-465`)

```go
var preflightSkips []ToolUpgradeResult
if !dryRun {
    var miseSkips, windowsSkips []ToolUpgradeResult
    executable, miseSkips = preflightMiseGentleAIUpgrades(executable, profile)
    executable, windowsSkips = preflightWindowsGentleAIUpgrades(executable, profile)
    preflightSkips = append(miseSkips, windowsSkips...)
}
```

Sequential composition, **no shared combinator**: the two preflights have different
shapes (the Windows one also writes `candidate.goInstallDestination`), and a rule
engine for two rules is abstraction ahead of need.

mise runs first so a mise-managed binary never reaches the Windows go-install
destination probe and can only be skipped once. `preflightMiseGentleAIUpgrades`
mirrors the Windows signature but gates on `r.Tool.Name == "gentle-ai"` plus
detection only — no `effectiveMethod` filter, because containment already answers
it (a Homebrew or `go install` binary is not under mise's root). Every other tool
in `internal/versions` passes through untouched. Staying inside the existing
`!dryRun` guard keeps `--dry-run` a preview of the executable set exactly as today.

Hint (asserted verbatim by test):

> ``mise-managed install — run `mise upgrade gentle-ai` instead; replacing the binary in place would desync the version mise tracks``

## The `os.Executable()` / `lookPathFn` divergence — decided, not hedged

Under mise **shims** this divergence is the normal case, not a corner: PATH holds
`…/mise/shims/gentle-ai`, so `lookPathFn` (`download.go:108`) returns the shim
while `os.Executable()` returns the real binary under `installs/`. That is exactly
why the detector must ask `os.Executable()`, and exactly what this change prevents —
without it, `Download` overwrites mise's *shim* with a raw binary. Under
`mise activate` both resolve the same file.

The residual case (two distinct `gentle-ai` binaries where the running one and the
PATH one disagree about mise) gets a code comment and no code. A false-positive skip
costs one instructional hint; the false negative requires invoking a non-mise
`gentle-ai` by absolute path while a mise one shadows PATH. Defending it means a
second detection pass over `lookPathFn` for a self-inflicted setup, and this repo
does not speculatively handle scenarios it cannot realistically reach.

## Stream 1 — `mise.toml` + `scripts/verify-mise-pins.sh`

`mise.toml` is three lines: `[tools]`, `go = "1.25.10"`, `node = "24"`.

The guard follows the repo bash idiom (`set -euo pipefail`, `die()` prefixed
`mise pins:`, `pwd -P` repo root, arguments forbidden) and asserts **pin equality
only** — no runtime-floor assertion:

| Pin | Authoritative source | Extraction |
|---|---|---|
| go | `go.mod:3` go directive | `awk '$1 == "go" {print $2; exit}'` |
| node | `ci.yml:371` `node-version: "24"` | anchored grep, quotes stripped |
| both | `mise.toml` | anchored `^go = "…"` / `^node = "…"` |

Every extraction asserts **exactly one** match. A second `node-version:` appearing
in `ci.yml` fails the guard telling the author to update it, rather than silently
validating the wrong job — failing closed is the whole point of a drift guard.
Wired into `unit-tests` immediately after `./scripts/deadcode-ratchet.sh`
(`ci.yml:141`), carrying the same "only Unit Tests is a required status check on
main" rationale comment.

Rejected: a Go checker in the `internal/releasepolicycmd` style. Two of the three
sources are not TOML, so a Go version still hand-parses them, and it adds a package
that must itself stay reachable for the ratchet.

## Stream 3 — documentation

Additive only. `mise.toml` is presented as **one option among** Homebrew, `go install`
and Scoop inside README's existing alternatives `<details>` (127-190) — not the
recommended or mandatory path. `docs/quickstart.md`, the `docs/platforms.md` matrix
(7-13), and `CONTRIBUTING.md` Prerequisites (Node gap) follow the same framing. The
command ships plain: `mise use -g gentle-ai@latest` (the registry short name, live
as of mise `v2026.9.0`), with no permanent `asset_pattern` hedge.

## Decisions

| Decision | Chosen | Rejected | Rationale |
|---|---|---|---|
| Detector visibility | Unexported | Exported API | An exported, uncalled function is a new deadcode-ratchet failure |
| Detector return | `bool` | Result struct | Hint is constant; nothing consumes the root |
| Empty env override | Treated as unset | `os.Getenv` as-is | An empty value resolves a cwd-relative root and silently changes the question |
| Root selection | First **set** var, one root | Probe all four roots | Matches mise's own precedence; probing all four reports a stale leftover dir as managed |
| Preflight composition | Sequential, mise first | Shared rule combinator | Different shapes; mise-first prevents a doubly-skipped candidate and a wasted destination probe |
| mise preflight filter | Name + containment | Also `effectiveMethod` | Containment already excludes Homebrew/go-install roots |
| Dry-run | Unchanged (`!dryRun` guard) | Skip in dry-run too | Symmetry with the Windows preflight; dry-run touches nothing |
| PATH divergence | Comment only | Defensive `lookPathFn` cross-check | Shim divergence is the motivating case, not the risk; the residual needs a self-inflicted setup |
| Drift guard language | Bash | Go program | Two of three sources are not TOML; the guarded file is three authored lines |
| Guard strictness | Exactly-one-match, fail closed | Best-effort first match | A guard that validates the wrong line is worse than no guard |

## File Changes

| File | Action | Description |
|---|---|---|
| `mise.toml` | Create | `[tools]` go + node pins at repo root |
| `scripts/verify-mise-pins.sh` | Create | Pin-equality drift guard |
| `.github/workflows/ci.yml` | Modify | One step in `unit-tests` after `ci.yml:141` |
| `internal/update/upgrade/mise.go` | Create | Root resolution + containment detection |
| `internal/update/upgrade/executor.go` | Modify | mise preflight composed before the Windows one (462-465) |
| `internal/update/upgrade/mise_ownership_test.go` | Create | Detector table tests |
| `internal/update/upgrade/mise_preflight_test.go` | Create | `Execute` wiring tests |
| `README.md`, `docs/quickstart.md`, `docs/platforms.md`, `CONTRIBUTING.md` | Modify | Additive install method + Node prerequisite |

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | **N/A** — no file-classification change; docs edits are inert prose | — | — |
| Git repository selection | **N/A** — no git subprocess, no `-C`, no repo resolution added | — | — |
| Commit state | **N/A** — nothing reads or writes index/worktree | — | — |
| Push state | **N/A** — no ref or remote interaction | — | — |
| PR commands | **N/A** — no PR argument composition | — | — |

Additional applicable case — **executable-path classification**, the real boundary
here. The detector decides whether a binary gets overwritten, so three cases are
design requirements and must reach `tasks.md` unchanged: (1) an empty or
whitespace-only `MISE_INSTALLS_DIR` must not resolve a cwd-relative root; (2) a
relative override must be answered by `Contains`' own `filepath.Abs`, never a
string prefix; (3) a set-but-nonexistent root must report *not* mise-managed rather
than erroring the upgrade. All three get RED tests before `mise.go` exists.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | Env precedence (all four rungs), empty/whitespace override, relative root, missing root, missing `HOME`, contained vs sibling-prefix path (`…/installsX/`) | `mise_ownership_test.go`, table-driven `t.Run(tt.name, …)` over `t.TempDir()` + `t.Setenv` (no `t.Parallel()`), `currentExecutableFn`/`userHomeDirFn` swapped with `t.Cleanup` restore |
| Integration | `Execute` wiring: mise-managed gentle-ai is `UpgradeSkipped` with the exact hint; a second tool in the same invocation still upgrades; `BackupID == ""` when the mise skip empties the candidate set; non-mise install is byte-identical to today; mise + Windows preflights compose without double-skipping | `mise_preflight_test.go`, reusing the existing `Execute` harness (`makeResult`, `linuxProfile`, `execCommand`/`snapshotCreator` injection) |
| Script | Guard passes on matching pins; fails on a go drift, a node drift, and a duplicated `node-version:` | Run `scripts/verify-mise-pins.sh` in CI; drift cases exercised manually during apply |
| Manual | `mise use -g github:…@latest` installs a working binary and `os.Executable()` reports an `installs/` path | Empirical, before merge (carries the proposal's asset-autodetection risk) |

Strict TDD is active (`openspec/config.yaml: strict_tdd: true`): every case above is
written RED first. Verification command is `go test ./...` plus
`go run ./internal/gofmtcheck`.

## Migration / Rollout

No migration; no persisted schema. Each stream reverts independently. One caveat the
proposal's rollback plan understates: reverting **only** the `executor.go` call leaves
`mise.go` unreachable and trips the deadcode ratchet — revert both files together, or
re-baseline with `scripts/deadcode-ratchet.sh --update`.

## Open Questions

- [x] mise `github:` asset autodetection may prefer the decoy
      `gentle-ai-review-provider-contract-<semver>.tar.gz`. Resolved empirically by the
      manual-verification task: the real platform tarball was selected both via the
      `github:` backend and via the registry short name (`aqua:` backend, verified
      against mise `v2026.9.0`). No `asset_pattern` caveat needed.
