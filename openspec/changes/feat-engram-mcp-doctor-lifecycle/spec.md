# Spec: engram-mcp-lifecycle-health

## Purpose
Add regression evidence, a version-aware doctor warning, and an integration-shape runbook for the incomplete MCP handshake defect tracked in `Gentleman-Programming/gentle-ai#1019`. The binary fix lives in `gentleman-programming/engram`; this PR ships evidence + diagnosis only.
**Source**: `openspec/changes/feat-engram-mcp-doctor-lifecycle/proposal.md` (§"In Scope", §"Approach", §"Test Plan", §"Out of Scope").

## Deliverables

| Deliverable | Location | Visibility |
|---|---|---|
| Tagged lifecycle test | `internal/components/engram/lifecycle_test.go` | Behind `//go:build engram_lifecycle` (default off) |
| Doctor check `engram-mcp-lifecycle-version` | `internal/components/engram/doctor.go` + registered in `internal/cli/doctor.go` | Always on; threshold default `"0.0.0"` is dormant |
| Runbook | `docs/engram-mcp-lifecycle.md` | Always shipped |

## Hard no-touch zones
- `internal/components/communitytool/pi_codegraph.go:550-574` — reference lifecycle (read-only).
- `internal/components/engram/inject.go:85-91` — JSON shape `engramServerJSONWithCmd`.
- `internal/components/engram/inject.go:318-332` and `:798-828` — file path resolution + `buildSeparateMCPContent` rebuild.
- `internal/components/engram/verify.go:31-37` — `runVersionCommand` seam; the doctor MUST reuse `VerifyVersionCommand`, NOT introduce a parallel `engram --version` invocation.

---

## ADDED Requirements

### R1 — Lifecycle test guarded by build tag
The system MUST guard `internal/components/engram/lifecycle_test.go` with `//go:build engram_lifecycle`. Default `go test ./...` MUST NOT compile or execute it. Operators MAY invoke it via `go test -tags engram_lifecycle ./internal/components/engram/...`.

#### Scenario S4: Default `go test ./...` excludes the lifecycle test
- GIVEN the repo at HEAD with no `-tags` flag
- WHEN `go test -count=1 ./...` runs
- THEN the lifecycle test is compiled out
- AND the default run is unaffected

#### Scenario: Tagged invocation runs the lifecycle test
- GIVEN the repo at HEAD
- WHEN `go test -count=1 -tags engram_lifecycle ./internal/components/engram/...` runs
- THEN the lifecycle test executes (PASS, FAIL, or SKIP per binary availability)

### R2 — Lifecycle test graceful skip when binary missing
When `exec.LookPath("engram")` returns an error, the test MUST call `t.Skip(...)` with a message naming the missing binary. The test MUST NOT fail.

#### Scenario S1: Binary absent → SKIP with named message
- GIVEN `exec.LookPath("engram")` returns an error
- WHEN the tagged test runs
- THEN `go test` reports `SKIP` with a message naming the `engram` binary
- AND `go test` exits 0

### R3 — Lifecycle test happy path
Given an `engram` binary on `$PATH`, the test SHALL drive the JSON-RPC lifecycle over stdio with a 5-second per-step timeout:

| # | Step | Assert |
|---|---|---|
| 1 | `initialize` | `result.protocolVersion` is non-empty |
| 2 | `notifications/initialized` | Send succeeds (no response expected) |
| 3 | `tools/list` | `result.tools` is a non-empty array |
| 4 | `ping` | Response is JSON-RPC success (no `error` field) |
| 5 | wait 10s | Binary process is still alive |

Reference (must NOT be modified): `internal/components/communitytool/pi_codegraph.go:550-574`.

#### Scenario S2: Healthy binary completes the full lifecycle
- GIVEN a healthy `engram` binary on `$PATH`
- WHEN the lifecycle test runs
- THEN all four JSON-RPC steps succeed within their per-step timeouts
- AND the binary process is still alive after the 10-second wait
- AND the test reports PASS

### R4 — Lifecycle test diagnostic on failure
If any JSON-RPC step errors, times out, or the binary exits before the 10-second wait completes, the test MUST fail with a diagnostic message that:
- Names the failed step (one of `initialize`, `notifications/initialized`, `tools/list`, `ping`, `alive-after-10s`).
- Includes the partial JSON-RPC exchange (last successful request id + response received, or the partial write that errored).
- References `Gentleman-Programming/gentle-ai#1019`.
- Suggests the likely cause is the engram binary's MCP server lifecycle implementation, not gentle-ai.

#### Scenario S3: Binary dies after initialize → fail with diagnostic
- GIVEN a binary that exits immediately after responding to `initialize`
- WHEN the lifecycle test runs
- THEN the test reports FAIL with a message containing `Gentleman-Programming/gentle-ai#1019`
- AND the message names the failed step
- AND the message includes the partial JSON-RPC exchange

### R5 — Doctor version check with WARN finding
`gentle-ai doctor` SHALL register a check named `engram-mcp-lifecycle-version` that:
- Reuses `VerifyVersionCommand("engram")` from `internal/components/engram/verify.go:31-37` (no parallel `engram --version` invocation; the existing `engram version` seam stays as-is).
- Parses the trimmed version string with strict semver rules; on parse failure emits a parse-warning WARN, never a FAIL.
- Compares the parsed version to `MinEngramVersionForHealthyLifecycle`.
- Emits a `WARN`-level finding when the parsed version is strictly below the threshold.

#### Scenario S5: Doctor warns on version below threshold
- GIVEN `MinEngramVersionForHealthyLifecycle = "1.5.0"`
- AND the installed binary reports `"1.4.0"`
- WHEN `gentle-ai doctor` runs
- THEN the output contains a WARN finding referencing `Gentleman-Programming/gentle-ai#1019`
- AND the finding recommends upgrading the binary

#### Scenario S7: Doctor stays clean on healthy version
- GIVEN `MinEngramVersionForHealthyLifecycle = "1.5.0"`
- AND the installed binary reports `"1.5.0"` or later
- WHEN `gentle-ai doctor` runs
- THEN the WARN finding is NOT emitted

### R6 — Doctor warning content
The WARN finding text MUST:
- Reference `Gentleman-Programming/gentle-ai#1019`.
- Recommend the user upgrade their `engram` binary.
- Provide the exact upgrade command the user can run.

#### Scenario: WARN finding carries upgrade guidance
- GIVEN the WARN finding is emitted
- WHEN a reviewer reads the rendered doctor report
- THEN the finding text contains `#1019`, an upgrade recommendation, and an actionable command

### R7 — WARN findings do not fail the doctor
A `WARN` finding SHALL NOT cause `gentle-ai doctor` to exit non-zero. Only `FAIL` findings cause non-zero exit (current behavior, preserved by `internal/cli/doctor.go:97-117` aggregation).

#### Scenario S6: WARN-only output exits 0
- GIVEN the only non-OK finding is the WARN from R5
- WHEN `gentle-ai doctor` runs
- THEN the exit code is 0
- AND the WARN line is rendered in the report

### R8 — Threshold constant ships at 0.0.0
`MinEngramVersionForHealthyLifecycle` SHALL be declared in `internal/components/engram/doctor.go` (or sibling) with default `"0.0.0"`. A `TODO` comment SHALL state: *"Set this to the engram release that first shipped the `notifications/initialized` notification once that release is published."* With the default, no WARN fires in production until the maintainer flips the switch.

#### Scenario S8: Default threshold is dormant
- GIVEN no override of `MinEngramVersionForHealthyLifecycle`
- AND the installed binary reports any version
- WHEN `gentle-ai doctor` runs
- THEN the version check does NOT emit a WARN

### R9 — Documentation runbook
A markdown runbook MUST be added at `docs/engram-mcp-lifecycle.md` and MUST include:
- JSON shape: `{"command": "engram", "args": ["mcp", "--tools=agent"]}` — cite `internal/components/engram/inject.go:85-91`.
- File path: `~/.claude/mcp/engram.json` on Linux/macOS, equivalent Windows path under `%USERPROFILE%` — cite `internal/components/engram/inject.go:318-332` and `:798-828`.
- Lifecycle sequence: `initialize` → `notifications/initialized` → `tools/list` → `ping`, citing `internal/components/communitytool/pi_codegraph.go:550-574` as the reference.
- Link to `Gentleman-Programming/gentle-ai#1019`.
- A "Known-good version" section with value `TBD` and an inline TODO instructing the maintainer to fill it in once the binary fix ships.

#### Scenario: Runbook links the contract and the open defect
- GIVEN the runbook at `docs/engram-mcp-lifecycle.md`
- WHEN a reviewer reads it
- THEN they can locate the JSON shape, file path, lifecycle sequence, and GitHub link in one page
- AND the "Known-good version" is `TBD` with a TODO marker

### R10 — JSON registration shape is unchanged
This change MUST NOT modify the JSON shape written by `internal/components/engram/inject.go:85-91` nor the file path resolved by `:318-332` and `:798-828`. Verified by an empty `git diff origin/main -- internal/components/engram/inject.go internal/components/communitytool/pi_codegraph.go` in the merged PR.

#### Scenario: inject.go and pi_codegraph.go are byte-identical
- GIVEN this PR is merged
- WHEN `git diff origin/main -- internal/components/engram/inject.go internal/components/communitytool/pi_codegraph.go` runs
- THEN the diff is empty

---

## Acceptance gate (for sdd-verify)
- `gofmt -l .` → empty.
- `go vet ./...` → clean.
- `go test -count=1 ./...` (no tag) → passes; lifecycle compiled out.
- `go test -count=1 -tags engram_lifecycle ./internal/components/engram/...` → lifecycle runs (SKIP or PASS per binary availability).
- S1–S8 each trace back to at least one Go test.
- `git diff origin/main -- internal/components/communitytool/pi_codegraph.go internal/components/engram/inject.go` → empty.
- Total changed lines ≤ 400 (target ≤ 200).

## Out of scope (binding)
- Fixing the `notifications/initialized` gap in the engram binary (lives in `gentleman-programming/engram`).
- Changing the registration JSON keys, file path, or transport (stdio).
- Adding `timeout` / `keepalive` / transport options.
- Modifying `internal/components/communitytool/pi_codegraph.go`.
- Filing an issue in the engram repo (orchestrator action).
- Setting the real `MinEngramVersionForHealthyLifecycle` threshold (depends on the binary fix).

## References
- Proposal: `openspec/changes/feat-engram-mcp-doctor-lifecycle/proposal.md`.
- Reference lifecycle: `internal/components/communitytool/pi_codegraph.go:550-574`.
- JSON shape: `internal/components/engram/inject.go:85-91`.
- File path: `internal/components/engram/inject.go:318-332`, `:798-828`.
- Version seam: `internal/components/engram/verify.go:31-37`.
- Doctor aggregation: `internal/cli/doctor.go:97-117`.
- Existing capability (UNCHANGED — not a delta): `openspec/specs/engram-protocol-injection/spec.md`.