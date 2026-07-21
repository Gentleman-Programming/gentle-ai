# Tasks: Engram MCP Doctor Lifecycle Hardening

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~190–230 (additions only; no deletions in core files) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | single-pr-default |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Lifecycle test (RED-first, tagged) | PR 1 | `go test -count=1 -tags engram_lifecycle ./internal/components/engram/...` | Real `engram mcp --tools=agent` subprocess via `exec.Command` | Delete `lifecycle_test.go`; no other file affected |
| 2 | Doctor check + wiring + tests | PR 1 | `go test -count=1 ./internal/components/engram/... && go test -count=1 ./internal/cli/...` | `gentle-ai doctor` (no real binary needed) | Revert 3-line edit in `doctor.go`; remove `doctor.go` and `doctor_test.go` |
| 3 | Docs + apply-progress | PR 1 | N/A (documentation) | N/A | Delete `docs/engram-mcp-lifecycle.md` |

## Phase 1: Lifecycle Test — RED First (S1, S2, S3, S4)

- [ ] 1.1 **RED**: Confirm `internal/components/engram/lifecycle_test.go` does not exist. Read `communitytool/pi_codegraph.go:550-574` for reference JSON-RPC sequence. Create `lifecycle_test.go` with `//go:build engram_lifecycle`; stub the test body so it compiles and runs `t.Skip("RED — implementation pending")` on the binary-absent path. Verify: `go test -count=1 -tags engram_lifecycle ./internal/components/engram/...` shows SKIP or compile error confirming the file is gate-controlled. (S1, S4)

- [ ] 1.2 **GREEN**: Implement full lifecycle driver in `lifecycle_test.go`. Use `exec.LookPath("engram")` → `t.Skip` if missing. Spawn `exec.Command("engram", "mcp", "--tools=agent")` with `Stdin`/`Stdout` pipes. Write JSON-RPC frames via `json.Encoder`; read responses via `bufio.Scanner`. Per-step timeout: `time.AfterFunc(5*time.Second, cancel)`. Sequence: (1) `initialize` → assert `protocolVersion` non-empty, (2) `notifications/initialized` (no response), (3) `tools/list` → assert `tools` array non-empty, (4) `ping` → assert no `error` field. After ping: `time.Sleep(10*time.Second)` then `process.Signal(syscall.Signal(0))` for liveness. On any failure: `t.Fatalf` with step name, partial exchange, and `#1019` reference. All four spec scenarios as `t.Run` subtests: S1 (absent), S2 (healthy), S3 (dies after init). (S2, S3)

## Phase 2: Doctor Check — Version + Wiring (R5, R6, R7, R8)

- [ ] 2.1 **RED**: Create `internal/components/engram/doctor.go` with a stub `CheckEngramMCPVersion` that returns `cli.CheckResult{Name: "engram-mcp-lifecycle-version", Status: CheckStatusPass}`. Compile-gate the file. (R5 pre-condition)

- [ ] 2.2 **GREEN**: Implement `CheckEngramMCPVersion` in `doctor.go`. Import `internal/components/engram` from `internal/cli/doctor.go` context — confirm import path as `github.com/gentleman-programming/gentle-ai/internal/components/engram`. Reuse `engram.VerifyVersionCommand("engram")` (verify.go:31-37). Add `const MinEngramVersionForHealthyLifecycle = "0.0.0"` with inline `// TODO: Set this to…` per spec R8. Add `parseStrictSemver(raw string) (major, minor, patch int, ok bool)`. In `CheckEngramMCPVersion`: call `VerifyVersionCommand`; on parse failure return `CheckStatusWarn` with parse-detail (never FAIL); on version below threshold return `CheckStatusWarn` with `#1019` reference and upgrade command; on version at/above threshold return `CheckStatusPass`. (R5, R6, R7, R8, S5, S7, S8)

- [ ] 2.3 Wire `CheckEngramMCPVersion` into `internal/cli/doctor.go:111-114`. After the existing 4 `append` calls, add: `report.Checks = append(report.Checks, engram.CheckEngramMCPVersion(ctx, homeDir))`. Confirm import compiles. (R5 registration)

- [ ] 2.4 **Tests for S5–S8**: Create `internal/components/engram/doctor_test.go`. Table-driven for `parseStrictSemver`: valid `"1.5.0"` → ok, invalid `"v1.5"` → !ok, `""` → !ok. For doctor check: mock `VerifyVersionCommand` via a package-level `var versionFunc = engram.VerifyVersionCommand` that tests override. Test cases: S5 — `versionFunc` returns `"0.5.0"` + threshold `"1.5.0"` → WARN + `#1019`; S7 — `"1.5.0"` + threshold `"1.5.0"` → PASS; S6 — WARN-only → exit code 0 (no FAIL flip); S8 — threshold `"0.0.0"` (dormant) → no WARN for any version; parse failure → WARN only (not FAIL). Mock restores via `t.Cleanup`. (S5, S6, S7, S8)

## Phase 3: Documentation

- [ ] 3.1 Create `docs/engram-mcp-lifecycle.md`. Include: registration JSON shape `{"command":"engram","args":["mcp","--tools=agent"]}` (cite `inject.go:85-91`); file path `~/.claude/mcp/engram.json` (cite `inject.go:318-332,798-828`); lifecycle sequence `initialize → notifications/initialized → tools/list → ping` (cite `pi_codegraph.go:550-574`); link to `#1019`; "Known-good version: TBD" with inline `TODO` for maintainer. ≤ 80 lines. (R9)

## Phase 4: Verification & Artifact Close

- [ ] 4.1 Run verification commands and capture evidence:
  - `gofmt -l internal/components/engram/ internal/cli/ docs/` → empty
  - `go vet ./internal/components/engram/... ./internal/cli/...` → clean
  - `go test -count=1 ./internal/components/engram/... ./internal/cli/...` → pass
  - `go test -count=1 -tags engram_lifecycle ./internal/components/engram/...` → runs (SKIP or PASS per binary)
  - `git diff origin/main -- internal/components/communitytool/pi_codegraph.go internal/components/engram/inject.go` → empty
  Record output of each. (R10 verification)

- [ ] 4.2 Create `openspec/changes/feat-engram-mcp-doctor-lifecycle/apply-progress.md` with task completion checkboxes and the verification evidence captured in 4.1. (sdd-verify preload)

## Phase 5: Commits (Conventional Commits, no Co-Authored-By)

Suggested commits in order:
1. `test(engram): add tagged stdio lifecycle test for MCP handshake (#1019)` — `internal/components/engram/lifecycle_test.go`
2. `feat(engram): add doctor check for MCP lifecycle version` — `internal/components/engram/doctor.go` + `internal/cli/doctor.go` edit
3. `test(engram): add S5–S8 unit tests for doctor check` — `internal/components/engram/doctor_test.go`
4. `docs(engram): add MCP lifecycle runbook` — `docs/engram-mcp-lifecycle.md`
5. `docs(sdd): record apply progress` — `openspec/changes/feat-engram-mcp-doctor-lifecycle/apply-progress.md`
