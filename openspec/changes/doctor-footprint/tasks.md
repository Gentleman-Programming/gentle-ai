# Tasks: Managed Block Footprint Diagnostic for `gentle-ai doctor`

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~990 (prod ~400: WU1 ~110, WU2 ~5, WU3 ~170, WU4 ~55, WU5 ~60; tests ~590) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR1 (WU1) → PR2 (WU2+WU3) → PR3 (WU4+WU5+WU6) |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending (user decision required) |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | WU1 scanner (`filemerge/section_scan.go` + test) | PR 1 | Standalone, ~290 lines, no dependency on doctor.go |
| 2 | WU2+WU3 collector/classifier (`cli/doctor.go` additions + test) | PR 2 | ~395 lines, depends on PR 1 (`filemerge.ScanSections`) |
| 3 | WU4+WU5+WU6 flag, signature migration, render, integration test | PR 3 | ~305 lines, depends on PR 2; signature-migration task (3.4/3.5) is a compile gate that MUST land in this PR's single commit |

## Phase 1: WU1 — Generic Section Scanner (filemerge)

- [x] 1.1 RED: `filemerge/section_scan_test.go` — `TestScanSections` table, 9 cases (well-formed, multi-block, empty-content, unclosed-opener, stray-closer, id-mismatch, novel-id, indent/CR, no-markers).
- [x] 1.2 GREEN: `filemerge/section_scan.go` — `Section`, `AnomalyKind`, `Anomaly`, `ScanResult`, `ScanSections(content string) ScanResult`.
- [x] 1.3 REFACTOR: `gofmt`, `go vet ./internal/components/filemerge/...`, confirm tests green.

## Phase 2: WU2+WU3 — Token Helper + Collector/Classifier (cli/doctor.go)

- [x] 2.1 RED: `cli/doctor_test.go` — `TestCollectFootprint`/`TestFootprintCheckResult`, 7 cases (healthy, orphan-open→Fail, stray-closer→Warn, state-missing, no-agents, unresolved-agent, absent-file→Pass).
- [x] 2.2 GREEN: add `estimateTokens(chars int) int` to `cli/doctor.go`.
- [x] 2.3 GREEN: add seam var `newDoctorRegistry = agents.NewDefaultRegistry`.
- [x] 2.4 GREEN: add `AgentFootprint`, `FootprintSummary` structs.
- [x] 2.5 GREEN: implement `collectFootprint(homeDir string) FootprintSummary`.
- [x] 2.6 GREEN: implement `footprintCheckResult(sum FootprintSummary) CheckResult`.
- [x] 2.7 REFACTOR: `gofmt`, `go vet ./internal/cli/...`, confirm tests green.

## Phase 3: WU4 — `--footprint` Flag + `RunDoctor` Signature Migration (compile gate)

- [x] 3.1 RED: `cli/doctor_test.go` — `TestParseDoctorFlags`, 5 cases (nil/empty, `--footprint`, `--footprint=false`, bad flag, extra arg).
- [x] 3.2 GREEN: add `DoctorFlags` struct + `ParseDoctorFlags(args []string) (*DoctorFlags, error)`.
- [x] 3.3 GREEN: change `RunDoctor` to `RunDoctor(ctx context.Context, args []string, w io.Writer) error`; wire flag parsing, `collectFootprint`, `footprintCheckResult`.
- [x] 3.4 GREEN (same commit, compile gate): update call site `internal/app/app.go` `case "doctor"` to `cli.RunDoctor(context.Background(), args[1:], stdout)`.
- [x] 3.5 GREEN (same commit, compile gate): migrate existing `TestRunDoctor` (`doctor_test.go:476`) and `TestRunDoctor_HomeDirError` (`:498`) calls to `RunDoctor(ctx, nil, &buf)`.
- [x] 3.6 REFACTOR: `go build ./...` to confirm compile gate, `gofmt`, `go vet ./...`.

## Phase 4: WU5 — `renderFootprintDetail`

- [x] 4.1 RED: `cli/doctor_test.go` — `TestRenderFootprintDetail`, 3 cases (healthy, absent-file, unresolved-agent).
- [x] 4.2 GREEN: implement `renderFootprintDetail(w io.Writer, sum FootprintSummary)`; leave `renderDoctorReport` untouched.
- [x] 4.3 GREEN: gate the call behind `flags.Footprint` inside `RunDoctor`.
- [x] 4.4 REFACTOR: `gofmt`, `go vet ./internal/cli/...`, confirm tests green.

## Phase 5: WU6 — Integration Test + Full Verification

- [x] 5.1 RED: `TestRunDoctor_FootprintFlag` — temp home + subset registry + fixtures; detail table present with `--footprint`, absent with `nil`.
- [x] 5.2 GREEN: confirm the test passes against the Phase 2-4 implementation (no new production code expected).
- [x] 5.3 Full gate: `go test ./...`, `go vet ./...`, `gofmt -l .` — must be clean.
- [x] 5.4 Cross-check all 13 spec scenarios (`specs/doctor/spec.md`, 6 requirements) against test coverage; flag any gap before `sdd-verify`.

## Post-review additions (PR3, not in the original plan)

Two independent code reviews (readability + reliability) on the WU4-6 diff surfaced 2 must-fix issues + 2 test gaps before merge. Fixed via strict TDD in the same PR3 commit:

- [x] 6.1 Split `FootprintSummary.NoAgents` into three distinct fields (`NoAgents`, `StateUnreadable`, `RegistryUnavailable`) so a corrupt `state.json` no longer gets the same "run install" remedy as a genuinely empty agent list — was contradicting `checkStateJSON`'s own diagnosis of the same file.
- [x] 6.2 Added `safeCollectFootprint` with `defer recover()` around the live footprint scan, so a panic reading an arbitrary on-disk instruction file can't crash `RunDoctor` and lose the other 4 checks.
- [x] 6.3 Added `TestRunDoctor_FootprintFailureDegradesOverallStatus` — end-to-end assertion that a broken marker flips `RunDoctor`'s printed `Status:` line, not just `footprintCheckResult` in isolation.
- [x] 6.4 Added `TestRunDoctor_FootprintFlagFreshInstallNoStateFile` — `--footprint` against a genuinely fresh install (no `state.json` at all) produces no crash/garbage output.

**Change status: DONE.** All 6 work units + 4 post-review fixes implemented across the 3-PR stacked chain (`feat/doctor-footprint-scanner`, `feat/doctor-footprint-check`, `feat/doctor-footprint-flag`). PR3 exceeds the 400-line budget (~463 lines) due to the review fixes above; `size:exception` requested in that PR per CONTRIBUTING.md.
