# Proposal: Engram MCP Doctor Lifecycle Hardening

## Intent

Issue #1019 suggests Claude Code terminates Engram when the MCP handshake is incomplete. This hardens gentle-ai with regression evidence, an actionable doctor warning, and documentation; it is **not** the end-to-end fix. The actual `notifications/initialized`/ping fix belongs in the external Engram binary (`gentleman-programming/engram`).

## Proposal question round

- Confirm whether the requested `engram --version` contract supersedes the existing `engram version` seam (`internal/components/engram/verify.go:31-37`).

## Scope

### In Scope
- **Lifecycle test:** add tagged `internal/components/engram/lifecycle_test.go`; use `exec.LookPath("engram")` and skip with `engram binary not available` when absent. Otherwise send `initialize`, `notifications/initialized`, `tools/list`, and `ping`, read responses, validate `serverInfo`, and fail with a #1019 diagnostic if the process exits before 10 seconds. Document `go test -tags engram_lifecycle ./internal/components/engram`; default `go test ./...` excludes it.
- **Doctor warning:** add configurable `MinEngramVersionForHealthyLifecycle = "0.0.0"`; include TODO text “set this to the version that fixed `notifications/initialized` once known”; versions below it warn, reference #1019, and recommend manual upgrade.
- **Documentation:** add `docs/engram-mcp-lifecycle.md` with Claude Code’s registration shape, lifecycle, unknown-until-confirmed known-good version, `~/.claude/mcp/engram.json`, and `{"command":"engram","args":["mcp","--tools=agent"]}`.

### Out of Scope
- Fixing the Engram binary; changing registration JSON/path/transport; or adding `timeout`/`keepalive` options.
- Touching `internal/components/communitytool/pi_codegraph.go:550-574`, `internal/components/engram/inject.go:85-91`, or `internal/components/engram/inject.go:798-828`.

## Capabilities

### New Capabilities
- `engram-mcp-lifecycle-health`: tagged lifecycle evidence, version warning, and guidance.

### Modified Capabilities
- None; `engram-protocol-injection` remains unchanged (`openspec/specs/engram-protocol-injection/spec.md:33-54`).

## Approach

Strict TDD: lifecycle RED first, then version policy/check, then docs. Reuse `VerifyVersion` (`internal/components/engram/verify.go:25-64`) and doctor aggregation (`internal/cli/doctor.go:97-117`). Preserve registration (`internal/components/engram/inject.go:85-91`, `internal/components/engram/inject.go:316-332`).

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `internal/components/engram/lifecycle_test.go` | New | Tagged stdio lifecycle test. |
| `internal/components/engram/lifecycle.go`, `version_test.go` | New | Threshold, parsing, comparison tests. |
| `internal/cli/doctor.go`, `doctor_test.go` | Modified | Version finding; existing HTTP check: `internal/cli/doctor.go:381-412`. |
| `docs/engram-mcp-lifecycle.md` | New | Runbook linked to #1019. |

## Test Plan

- Lifecycle: missing binary → skip; healthy binary → `serverInfo`/ping pass; post-init exit → fail with #1019.
- Doctor: `<` threshold → `WARN`; `≥` threshold → `OK`; parse failure → explicit warning.
- Run default suite and tagged command with `engram` installed.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Non-deterministic binary exit flakes tagged test. | Med | Opt-in tag, bounded 10 seconds, step diagnostics. |
| Doctor false positives when parsing fails. | Med | Strict parsing and warning fallback. |

## Rollback Plan

Revert each deliverable as a single-file change. The tagged test is inactive by default, so its absence is harmless to default CI; doctor wiring/policy and docs revert independently without changing registration.

## Dependencies

- `engram` on PATH for the tagged test; upstream fix/version remains external.

## Out-of-Scope Follow-ups

- Orchestrator may file the upstream `gentleman-programming/engram` issue.
- Set the threshold to the version fixing `notifications/initialized` once known.

## Success Criteria

- [ ] Default suite is unaffected; tagged probe gives #1019 evidence.
- [ ] Doctor warns below threshold and recommends manual upgrade.
- [ ] Docs preserve the JSON contract and lifecycle.
- [ ] Implementation stays within the 400-line review budget (target ≤200 changed lines).
