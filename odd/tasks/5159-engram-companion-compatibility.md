# 5159: Pi provisioning misses an incompatible engram core behind the current companion

**Issue**: Gentleman-Programming/gentle-ai#5159 (OPEN, `bug`, `status:approved`, no PR)
**Branch**: `fix/5159-engram-companion-compatibility`
**Base**: `main` at `72e0cccb`

## Objective

When provisioning or refreshing a managed memory integration, validate the **effective** engram runtime against the capability the selected agents actually require, instead of treating executable presence as compatibility. On an incompatible runtime, name the active runtime, its version and the unmet requirement, and offer an explicit supported upgrade before the integration is presented as ready, while preserving user consent and the existing memory store.

## Problem

`internal/components/engram/verify.go:15-21` (`VerifyInstalled`) only runs `lookPath("engram")`. An engram core that is too old to serve the `instance-id` capability therefore passes verification, and the mismatch surfaces only when a memory tool initializes.

Root cause is systemic, not incidental: `internal/agents/pi/adapter.go:276` installs `gentle-engram@latest` (always newest), while gentle-ai does not provision or version-pin the core at all. The companion's requirement therefore ratchets forward on every npm install while an untouched user core silently ages.

## Root cause evidence

The requirement is a **capability**, not a version. `instance_id` is absent from all 22 MCP tool schemas (`internal/mcp/mcp.go:437-1091` upstream); it is an HTTP port-ownership concern. Confirmed at the tag boundary: absent in `v2.0.0-rc.10`, present in `v2.0.0-rc.11`. The reporter's `1.20.0` is one sample, not a threshold; the incompatible set is every core below `v2.0.0-rc.11`.

There is no named Go constant for that boundary upstream; the literal lives only in plugin TypeScript strings.

## Upstream per-agent requirement truth table

Verified against the local clone at `engram@3951380` (`v3.0.0-12-g3951380`).

| Agent | Requires `instance-id` | Behavior on an old core |
|-------|------------------------|--------------------------|
| Pi | yes, hard | `DeterministicStartupError` |
| OpenCode | yes, hard (unless `ENGRAM_URL`) | blocks write tools via `tool.execute.before` |
| Claude Code | no | warns to stderr, `exit 0`, memory still works |
| Codex | no | identical soft-fail |
| Gemini / Cursor / Windsurf | no | never probes |

Consequence: a uniform all-agent gate would falsely block working Claude Code / Codex / Gemini setups. The gate must be per-agent.

## Scope

In scope:

- Capability probe `engram instance-id` as the verdict, with `ENGRAM_NO_UPDATE_CHECK=1` and `ENGRAM_DATA_DIR` pointed at a temp dir.
- Per-agent requirement model following the `IsVerifiedSlimAdapter` precedent (`internal/components/engram/protocol.go:84-89`).
- Effective runtime resolution: `ENGRAM_BIN` then PATH; `ENGRAM_URL` set means the remote is authoritative and the local binary is not judged.
- `NoRollback` in the verify model so an incompatible core fails without reverting work gentle-ai did correctly.
- Gate wired into install CLI and sync, plus a pre-flight in `installcmd.ValidateAgentInstallPreflight`.

Out of scope (tracked separately):

- TUI install bypasses post-apply verification entirely (`internal/cli/run.go:1909-1939`). Preexisting gap affecting every check, not just this one.
- Probing a remote server over `GET /health` when `ENGRAM_URL` is set. New network surface with its own failure modes; the companion already performs that validation.

## Constraints

- A missing engram binary must NOT become a hard incompatibility. Existing contract requires OpenCode SDD install to succeed when auto-added Engram cannot be downloaded (`internal/cli/run.go:1588-1602`).
- Adapters stay declarative and must not spawn subprocesses (`docs/prd-command-code-adapter.md:39`). The probe belongs in `installcmd`, not in the adapter.
- `instance-id` is NOT exempt from the core's CLI update check (`cmd/engram/main.go:831-843`), so `ENGRAM_NO_UPDATE_CHECK=1` is mandatory to keep the probe offline and quiet.
- The probe writes to its data dir (mkdir, lock, token mint) but touches no SQLite and starts no daemon. A temp `ENGRAM_DATA_DIR` keeps the real `~/.engram/` untouched, preserving the store.
- Every refusal must name a runnable continuation, and its runnability is verified by executing it (`skills/systemic-issue-triage/SKILL.md:24,33`).
- Never auto-run the upgrade; never silently update components.
- Filesystem tests use `t.TempDir()` and never touch a real home directory.
- No em dashes in user-facing copy.
- Authored budget ~400 changed lines per PR (`CONTRIBUTING.md:329-332`).

## Tasks

- [x] **T0** Investigate current code and establish the design with the user
- [x] **T1** Verify model: `NoRollback` on `Check`, propagated to `CheckResult`, aggregated as `RollbackRequired`
- [x] **T2** Engram capability probe: runtime resolution, probe hygiene, per-agent requirement model
- [ ] **T3** Wire the gate into install and sync, with retained-state persistence and the upgrade offer
- [ ] **T4** Documentation alongside the user-visible change

## Acceptance criteria

1. An old effective core with Pi or OpenCode selected fails verification, names runtime path, version and unmet capability, and does NOT roll back the install.
2. A compatible core passes.
3. `ENGRAM_BIN` is honored as the effective runtime; `ENGRAM_URL` set skips the local judgment entirely.
4. A mixed failure (incompatible core plus a missing managed file) DOES roll back.
5. No selected agent requires the capability yields `skipped`, never `passed`.
6. A missing engram binary does not fail the run.
7. The recovery command named in the message is proven runnable by executing it in a test.
8. `go test ./...` green.

## Verification

- `go build ./...`
- `go test ./internal/verify/... ./internal/components/engram/... ./internal/installcmd/... ./internal/cli/...`
- `go test ./...`

## Route

Delegated direct. The change spans more than two non-trivial files across four packages, which fires the writer trigger. One bounded writer per work unit, in separate commits.

## Progress

Design settled with the user. Investigation complete: two read-only explorations of the upstream engram clone (cross-confirmed against GitHub search and against local source with CodeGraph) and one independent adversarial verification of the plan.

### Known environmental failure

`TestRunSyncMigratesLegacyManagedPiCodeGraphSelection` (`internal/cli/sync_test.go:2080`) fails on this machine. Proven pre-existing by stashing WU1 and reproducing it at base commit `72e0cccb`, so it is not caused by this feature. It still fails with `codegraph` removed from `PATH` and with an isolated `HOME`, so neither is the cause. Baseline for `internal/cli`: exactly this one failure. WU3 must introduce no new failures in that package.

### Work unit log

| WU | Commit | Authored lines | Risk tier | RDD outcome | Verification |
|----|--------|----------------|-----------|-------------|--------------|
| WU1 | `e96e90f2` | 362 (incl. feature doc) | medium | not due, `under_budget` | `go build ./...` ok; `internal/verify` 30 tests ok; `internal/cli` unchanged apart from the known failure |
| WU2 | pending | 1018 | pending | pending | `go build ./...` ok; `go vet ./internal/components/engram/...` ok; `internal/components/engram` ok; `internal/verify` ok |

Reviewed boundary: `main`. Next assessment base stays `main` until a review is acknowledged.

### Deviations from the original plan

The investigation overturned one approved decision and corrected two faulty premises. Recorded here so the reasoning survives.

- **Rejected: uniform all-agent gate.** Originally recommended gating every agent on `instance-id`. Source evidence refuted it; only agents managing a local `engram serve` require the capability. A uniform gate would have blocked working setups. Corrected to a per-agent model.
- **Corrected: `engramHealthChecks` does not reach sync.** It is called only from `runPostApplyVerification` (`internal/cli/run.go:2780`). `runPostSyncVerification` (`internal/cli/sync.go:2149-2205`) has its own file-only check set, so sync needs explicit wiring.
- **Corrected: `NoRollback` on `Check` alone is inert.** `RunChecks` copies only ID and description (`internal/verify/checks.go:32`), so policy must reach `CheckResult` too. Aggregation is rollback if ANY failed hard check has `NoRollback == false`, so a mixed failure still reverts.

### Deviations found in review

- **WU2 shipped a 27-symbol exported surface** where roughly five concepts existed, using aliases rather than distinct capabilities, plus a dead constant. Returned for cleanup; collapsed to 13 exported symbols with a single decision entry point.
- **`IsMissingBinary` over-matched.** It tested `errors.Is(err, os.ErrNotExist)` via a helper returning `os.ErrNotExist`, so any error wrapping it classified as a missing binary. Because a missing binary deliberately does not block while an incompatible core does, that misclassification would have silently disabled the gate. Now matches only `*MissingBinaryError`, with a negative test asserting `os.ErrNotExist` does not match.
- **The decision was not centralized.** `ENGRAM_URL` awareness existed as a helper nothing called, and nothing enforced that an empty agent selection means not-required rather than passed. Both are acceptance criteria, and a caller had to remember three separate facts. Replaced by one entry point returning five explicit outcomes.
- **Probe timeout was conditional.** The bounded timeout applied only when the caller's context had no deadline, so a long-lived context produced an unbounded probe and a hung core could stall the pipeline. Now always clamped to the earlier of the caller's deadline and the probe bound.

### Work units

- **WU1 (T1)**: verify model. Purely additive, no change to existing check behavior.
- **WU2 (T2)**: engram capability probe and per-agent requirement model. Purely additive.
- **WU3 (T3)**: gate wiring, retained-state persistence, upgrade offer. Activates the gate.

WU1 and WU2 land safely on their own; WU3 is the behavioral commit.

## Next step

T1 with tests first: `NoRollback` zero-value safety, policy propagation into `CheckResult`, and mixed-failure rollback.
