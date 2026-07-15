# Profile-Only Uninstall Validation

**Date**: 2026-07-15
**Spec**: `.specs/features/profile-only-uninstall/spec.md`
**Diff range**: `3b5da78^..9e9b2ec` (both implementation and follow-up test commit)
**Verifier**: independent sub-agent (author != verifier)

---

## Task Completion

No `tasks.md` exists: this is a small feature. The verified implementation is
`3b5da78` (`fix(tui): isolate profile-only uninstall`); `9e9b2ec`
adds the outcome tests required by the prior validation pass.

## Spec-Anchored Acceptance Criteria

| Requirement | Criterion and spec-defined outcome | `file:line` + assertion expression | Result |
| --- | --- | --- | --- |
| PROF-01 | Confirming selected profiles removes only their SDD-agent keys: the selected names are passed to profile removal, and the removal test leaves no selected SDD/JD keys. | `internal/tui/model_test.go:2216-2247` — managed callbacks call `t.Fatal`; `reflect.DeepEqual(removedProfiles, []string{"cheap", "fast"})`. `internal/components/sdd/profiles_test.go:900-914` — exactly 11 default agents remain and no `-cheap` SDD/JD key remains. | ✅ PASS |
| PROF-02 | Profile-only confirmation neither invokes managed-component uninstall nor deletes/rewrites Engram artifacts. | `internal/tui/model_test.go:2232-2238` — both managed callbacks fail the test if called. `internal/tui/model_test.go:2309-2315` and `internal/tui/screens/uninstall_result_test.go:88-92` — profile-only UI omits the Engram cleanup scope while identifying profile-only removal. | ✅ PASS |
| PROF-03 | Default SDD agents, an unselected profile, and unrelated `opencode.json` content remain unchanged. | `internal/components/sdd/profiles_test.go:917-932` — every default key remains; `theme == "custom-theme"`; `reflect.DeepEqual(root["mcp"], unrelatedMCP)`. `internal/components/sdd/profiles_lifecycle_test.go:266-270` — after removing `cheap`, only `premium` remains. | ✅ PASS |
| PROF-04 | A profile-removal error is reported as failure, never success. | `internal/tui/model_test.go:2265-2270` — `errors.Is(msg.Err, removalErr)` and stops after `cheap`. `internal/tui/model_test.go:2282-2297` — result screen, failure marker and exact error are present; success marker is absent. | ✅ PASS |

**Status**: ✅ 4/4 requirements have test assertions matching their
spec-defined outcomes; no spec-precision gaps.

## Edge Cases

| Edge case | Evidence | Result |
| --- | --- | --- |
| No named profile selected keeps the ordinary managed uninstall available. | `internal/tui/model_test.go:1937-1949` reaches confirmation without profiles; `internal/tui/model_test.go:1992-2025` invokes `UninstallFn` for that normal partial flow. Full-mode profile selection remains managed as asserted at `internal/tui/model_test.go:2183-2212`. | ✅ PASS |
| A selected profile cannot be removed: surface the error and do not claim success. | `internal/tui/model_test.go:2250-2297`. | ✅ PASS |

## Gate Check

- **Gate command**: `go test ./...` (no feature `tasks.md`; this is the full
  unit-suite command in `CONTRIBUTING.md:103-120`).
- **Result**: ✅ PASS — exit code 0; all 57 listed Go packages completed, with
  no test failures.
- **Test integrity**: 2,938 declared `Test*` functions at `3b5da78^`;
  2,943 at `9e9b2ec` (**+5**). The range adds six test functions and renames
  one existing test; it removes none. No skips or weakened assertions were
  reported/introduced in the range.

## Discrimination Sensor

All mutations ran in a detached temporary git worktree at `9e9b2ec`. Each
targeted test was run with `-count=1`; the mutation was then reversed, the
scratch worktree verified clean, and removed. The real worktree was not
mutated.

| # | File:line | Behavior-level mutation | Targeted test | Result |
| --- | --- | --- | --- | --- |
| 1 | `internal/tui/model.go:2948` | Changed the profile-only branch from partial mode to full mode, re-enabling managed uninstall for a selected partial profile. | `go test ./internal/tui -run '^TestStartUninstall_PartialProfileRemovalDoesNotRunManagedUninstall$' -count=1` | ✅ Killed — `UninstallWithProfilesFn must not run for profile-only removal`. |
| 2 | `internal/components/sdd/profiles.go:738` | Deleted the unrelated `mcp` root entry before serializing `opencode.json`. | `go test ./internal/components/sdd -run '^TestRemoveProfileAgents_RemovesProfileSDDAndJDAgents$' -count=1` | ✅ Killed — unrelated MCP configuration was missing. |
| 3 | `internal/tui/model.go:2951` | Suppressed the returned profile-removal error and returned a success message. | `go test ./internal/tui -run '^TestStartUninstall_PartialProfileRemovalReturnsError$' -count=1` | ✅ Killed — expected removal error was nil. |

**Sensor depth**: lightweight (3 behavior-level mutations)
**Result**: ✅ 3/3 killed; 0 survived.

## Code Quality

| Check | Result |
| --- | --- |
| No scope creep; only TUI profile routing, presentation, and supporting tests changed | ✅ |
| No single-use abstraction or unnecessary flexibility | ✅ |
| Surgical change matching existing callback/rendering patterns | ✅ |
| Tests are non-shallow and map to requirements/edge cases | ✅ |
| Spec-anchored asserted outcomes match the spec | ✅ |
| Coverage expectations met: callback/domain preservation and TUI success/error paths are covered | ✅ |
| Every changed test is claimed | ✅ — profile-only routing, Engram suppression, error presentation, data preservation, or retained managed flow |
| Documented quality/testing guidance | ✅ — `CONTRIBUTING.md:103-120` |

## Requirement Traceability

| Requirement | Previous status | Validation status |
| --- | --- | --- |
| PROF-01 | Implementing | ✅ Verified |
| PROF-02 | Implementing | ✅ Verified |
| PROF-03 | Implementing | ✅ Verified |
| PROF-04 | Implementing | ✅ Verified |

## Lessons

Clean PASS: no lesson recorded. `scripts/lessons.py` is absent in this
repository, and no manual lesson artifact was created.

## Summary

**Overall**: ✅ Ready
**Spec-anchored check**: 4/4 requirements matched exact asserted outcomes
**Gate**: `go test ./...` passed
**Sensor**: 3/3 mutations killed
