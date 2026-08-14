# Archive Report: fix-persona-output-style-rollback

**Change**: fix-persona-output-style-rollback (issue #3163, status:approved)
**Archived**: 2026-08-13
**Archive path**: `openspec/changes/archive/2026-08-13-fix-persona-output-style-rollback/`
**Artifact store**: openspec (filesystem)
**Status**: success — SDD cycle complete

## What Shipped

Closed the removal-failure gap in persona output-style transitions: `removeFileAtomic` (inject.go) called `os.Remove` after the new style and settings were written, so a removal error (e.g. a Windows file lock) left a mixed state and exited 1. The change routed removal through an injectable exported seam (`persona.RemoveFileFn`, mirroring the `backup.UserHomeDirFn` precedent), made the failure a typed `*persona.OutputStyleRemovalError` that propagates to the existing #3161 pipeline rollback boundary, and classified the rolled-back outcome at both CLI exit points (`run.go`, `sync.go`) into `WARNING` + exit 0 — with no retry loop, install/sync parity, and user files / unrelated settings preserved.

Implementation landed on `main` in 3 conventional commits (no Co-Authored-By trailers):

| Commit | Message |
|--------|---------|
| `919b7db1` | `fix(persona): add removal seam and typed output-style removal error` |
| `e5a5d414` | `fix(cli): warn and exit 0 after rolled-back persona output-style transition` |
| `5b2c7a58` | `test(cli): prove persona output-style transition rollback parity` |

Files: `internal/components/persona/inject.go` (modified), `internal/cli/persona_rollback.go` (new), `internal/cli/run.go` + `internal/cli/sync.go` (modified), `inject_test.go` (modified), `persona_rollback_test.go` + `persona_transition_test.go` (new).

## Verification Outcome

**Verify phase**: PASS — 0 CRITICAL, 0 WARNING, 7/7 requirements and 10/10 scenarios compliant (fresh `-count=1` runs, `go vet` clean, native `gentle-ai sdd-verify-validate` → `valid: true, verdict: pass`). 2 cosmetic SUGGESTIONs recorded (test-name prefix match in `-run` pattern; strict-TDD task 2.3 RED documented as `N/A` with the wiring proven by the 3.1/3.2 e2e tests) — neither blocks archive.

- 11/11 tasks complete in `tasks.md` (Task Completion Gate passed).
- Full-suite `go test ./... -count=1`: 64 packages `ok`, 2 package FAIL with exactly the 3 known pre-existing `codex` failures (`TestEveryManifestDigestStaysByteStable/codex`, `TestNegotiatedConsentEnvelopeBindsTheDeclaredRuntimeIdentity/codex`, `TestDirectRouteStillRefusesADeclaredRuntime/codex`). These are environmental and were reproduced identically at baseline `2ef223bf`; none of the changed files participate in those paths. Recorded per the launch-prompt final-state facts — not findings of this change, triaged separately.

## Spec Promotion (new capability)

This delta introduced a **NEW capability** `persona-output-style-transitions` (#3163) — a new domain, not a modification of an existing spec. Per the design's explicit boundary, `openspec/specs/persona-behavior-contract/spec.md` (a content contract) was NOT rewritten and remains untouched. Following the repo convention for new-capability promotions (precedent: `level-neutral-persona-parity` → `persona-behavior-contract`), the delta was promoted into a new canonical main spec:

| Domain | Action | Source | Destination |
|--------|--------|--------|-------------|
| `persona-output-style-transitions` | Created main spec for new capability | `spec.md` (this archive folder) | `openspec/specs/persona-output-style-transitions/spec.md` |

Promotion method: title normalized to `# Persona output-style transitions Specification`; delta framing (`Delta for the new ... capability (#3163): ...` line and the `## ADDED Requirements` wrapper) removed per main-spec convention (`## Requirements`); all 7 requirement blocks and 10 scenarios carried over verbatim, no summarization, no scenario dropped or paraphrased.

### Spec Promotion Self-Check

Delta spec: `openspec/changes/archive/2026-08-13-fix-persona-output-style-rollback/spec.md`
Main spec: `openspec/specs/persona-output-style-transitions/spec.md`

- Requirement headings in delta: 7 — all 7 present verbatim in main spec.
- Scenario blocks in delta: 10 — all 10 present verbatim in main spec.
- Mechanical copy readback: `diff -r` of the byte-identical copy — IDENTICAL (empty).
- Contract-region readback: `diff -r` of delta vs. main spec from first `### Requirement:` to EOF — IDENTICAL (empty).
- `persona-behavior-contract` untouched: no diff, no edits.

## Archive Integrity (Mechanical Copy Contract)

- Pre-move recursive snapshot taken before the move.
- Move: `git mv` rejected (change folder untracked) → `mv` fallback used, as the skill prescribes.
- Post-move `diff -r <snapshot> <archived folder>`: **empty output — byte-identical**, the only passing evidence.
- Archived artifacts: `proposal.md`, `spec.md`, `design.md`, `tasks.md` (11/11 checked), `apply-progress.md`, `verify-report.md`, plus this `archive-report.md` (additive-only, excluded from the readback).
- Active `openspec/changes/fix-persona-output-style-rollback/` no longer exists.
- Archive name collision check: none for `2026-08-13-fix-persona-output-style-rollback`.

## Follow-Ups

1. **Pre-existing codex failures** (`TestEveryManifestDigestStaysByteStable/codex`, `TestNegotiatedConsentEnvelopeBindsTheDeclaredRuntimeIdentity/codex`, `TestDirectRouteStillRefusesADeclaredRuntime/codex`): reproduced at baseline `2ef223bf`, environmental (codex manifest digest drift + codex review-transport capability absence), untouched by this change. To be triaged separately — not a finding of this change.
2. **No retry loop** on removal failure remains a deliberate boundary (Windows locks → direct rollback); documented in proposal/design, pinned by call-count assertions.

## SDD Cycle Complete

The change has been fully planned, implemented, verified, and archived. `openspec/specs/persona-output-style-transitions/spec.md` now reflects the new behavior as the source of truth for the new capability. Ready for the next change.
