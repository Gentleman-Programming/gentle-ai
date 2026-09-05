# Apply Progress: doctor-footprint

## Status

- **WU1 (Phase 1 — Generic Section Scanner)**: DONE. This is PR 1 of the stacked chain.
- **WU2+WU3 (Phase 2 — Token helper + collector/classifier)**: NOT STARTED. Next batch, on a new branch off `feat/doctor-footprint-scanner`.
- **WU4 (Phase 3 — flag + `RunDoctor` signature migration)**: NOT STARTED.
- **WU5 (Phase 4 — `renderFootprintDetail`)**: NOT STARTED.
- **WU6 (Phase 5 — integration test + full verification)**: NOT STARTED.

## What was implemented (WU1, this batch)

- `internal/components/filemerge/section_scan.go` — `Section`, `AnomalyKind`, `Anomaly`, `ScanResult` types and `ScanSections(content string) ScanResult`, per design.md §2's exact 4-step line-based single-pass algorithm. Reuses existing `markerPrefix`/`markerSuffix`/`closePrefix` constants from `section.go` (no redefinition).
- `internal/components/filemerge/section_scan_test.go` — `TestScanSections`, table-driven, 9 cases matching design.md §8.1 and tasks.md 1.1 exactly: single well-formed block, multi-block, empty-content block, unclosed opener at EOF (`AnomalyOrphanOpen`), stray closer with no opener (`AnomalyOrphanClose`), id mismatch (`AnomalyMismatch`), novel/unknown id (genericity proof, no whitelist), leading indentation + trailing `\r`, zero markers.

## Scope discipline

Touched ONLY `internal/components/filemerge/`. Did NOT touch `internal/cli/doctor.go`, `internal/app/app.go`, or anything else — those are WU2/WU3 (PR2) and WU4/WU5/WU6 (PR3), reserved for future branches per the stacked-PRs delivery strategy.

## Verification run

- `go test ./internal/components/filemerge/... -v` — all tests pass, including pre-existing suite (safety net, no regressions).
- `go vet ./internal/components/filemerge/...` — clean.
- `gofmt -l internal/components/filemerge/section_scan.go internal/components/filemerge/section_scan_test.go` — clean (not in the repo-wide `gofmt -l .` unformatted list).
- `go test ./...` (full repo) — `internal/components/filemerge` passes (cached ok). Failures present in `internal/components/engram`, `internal/components/gga`, `internal/components/permissions`, `internal/components/sdd`, `internal/components/uninstall`, `internal/tui`, `internal/update/upgrade` are PRE-EXISTING and environment-specific (Windows symlink privilege errors, `TempDir` cleanup lock contention, network/install-script mocking) — none touch any file this PR modified, and this PR adds only new, unreferenced exported symbols (nothing outside `filemerge` imports them yet). Not introduced by WU1.

## Next steps (WU2 — new branch off this one)

Per design.md §3-4 and tasks.md Phase 2:
1. `estimateTokens(chars int) int` in `internal/cli/doctor.go`.
2. New seam `var newDoctorRegistry = agents.NewDefaultRegistry`.
3. `AgentFootprint` / `FootprintSummary` structs.
4. `collectFootprint(homeDir string) FootprintSummary` — consumes `filemerge.ScanSections` from this PR.
5. `footprintCheckResult(sum FootprintSummary) CheckResult`.
6. RED tests first: `TestCollectFootprint` / `TestFootprintCheckResult`, 7 cases per design.md §8.2.
