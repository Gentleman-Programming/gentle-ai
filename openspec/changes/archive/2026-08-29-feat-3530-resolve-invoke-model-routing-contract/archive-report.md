# Archive Report — feat-3530-resolve-invoke-model-routing-contract

**Change**: `feat-3530-resolve-invoke-model-routing-contract`
**Title**: feat(pi): resolve and invoke installed model-routing contract
**Archived**: 2026-08-29
**Branch**: `feat/3530-resolve-invoke-model-routing-contract`
**Commit**: `abd15d21` (HEAD, final — no extra commits after verify)
**Base**: `v2.5.0-rc.2` (569f3361)
**Mode**: `openspec`
**Status**: COMPLETED — all 17 tasks complete, verify PASS (6/6 req, 16/16 scenarios), delivery exception acknowledged, archived with byte-identity proof

---

## Executive Summary

The `pi-model-routing` change extends `internal/agents/pi` with an injectable resolver (`ResolveModelRoutingExecutable` + `ModelRoutingClient`) that resolves `gentle-pi-models` PATH-first then via package sources (`npm:`/`git:`/`local:`), verifies `package.json` bin via `ResolvePackageBin` (64 KiB bound, duplicate-key reject, unsafe/missing/non-regular/non-executable + symlink containment), and invokes `gentle-pi.model-routing/v1` through a bounded JSON contract client (`capabilities`/`inspect`/`validate`/`apply`) with distinct error taxonomy (`missing`/`malformed`/`timeout`/`invalid-json`/`unsupported-contract`/`probe-failed`) and cancellation/no-network/no-write invariants. Verification passed on evidence `6294aeb7` (abd15d21) with zero CRITICAL findings; size 673 A+D in `internal/agents/pi/` was accepted under `size:exception` reset `d67874a0` (forecast Low, actual test-matrix breadth). Unblocks #3522.

---

## Specs Synced

| Domain | Action | Details |
|--------|--------|---------|
| `pi-model-routing` | Created | 6 ADDED requirements, 16 scenarios — `Executable Resolution Order` (3), `Configuration Precedence and Directory Overrides` (3), `Package Mapping and Manifest Bin Verification` (2), `Bounded Capability Probe` (2), `Versioned JSON Contract Client` (3), `Typed Errors, Cancellation, and Read-Only Invariants` (3). Main spec `openspec/specs/pi-model-routing/spec.md` did not exist; created by mechanical `cp` + `diff -r` + `mv` byte-identity copy. |

**Source of Truth Updated**:
- `openspec/specs/pi-model-routing/spec.md` (new domain, byte-identical to `openspec/changes/archive/2026-08-29-feat-3530-resolve-invoke-model-routing-contract/specs/pi-model-routing/spec.md`)

**Mechanical copy proof (delta → main spec)**:

```
diff -r openspec/changes/feat-3530-resolve-invoke-model-routing-contract/specs/pi-model-routing/spec.md openspec/specs/pi-model-routing/.spec.md.XXXXXX
# exit 0 — empty, no differences

diff -r openspec/changes/feat-3530-resolve-invoke-model-routing-contract/specs/pi-model-routing/spec.md openspec/specs/pi-model-routing/spec.md
# exit 0 — empty, byte-identical
```

---

## Archive Contents

**Archived to**: `openspec/changes/archive/2026-08-29-feat-3530-resolve-invoke-model-routing-contract/`

- `proposal.md` ✅ (intent: resolve+invoke `gentle-pi.model-routing/v1` without TUI/install scope)
- `exploration.md` ✅
- `design.md` ✅ (Approach A: 2 files in `internal/agents/pi`, 5 decisions, injectable Runner)
- `specs/pi-model-routing/spec.md` ✅ (delta, 6 req / 16 scenarios)
- `tasks.md` ✅ (17/17 tasks complete — Phases 1.1-1.8 RED, 2.1-2.5 GREEN, 3.1-3.4 Verification)
- `verify-report.md` ✅ (schema `gentle-ai.verify-result/v1`, verdict `pass`, evidence `6294aeb7`, `requirements 6/6`, `scenarios 16/16`, `blockers 0`, `critical_findings 0`)
- `archive-report.md` ✅ (this file, additive-only — excluded from byte-identity diff)

**Task Completion Gate**: PASSED — persisted `tasks.md` has zero unchecked items (`- [ ]` absent). No stale-checkbox reconciliation required.

**Mechanical archive proof (snapshot vs archived tree)**:

```
diff -r /tmp/sdd-archive.XXXXXX/source openspec/changes/archive/2026-08-29-feat-3530-resolve-invoke-model-routing-contract
# exit 0 — empty, no differences (archive-report.md excluded as additive-only)
```

Source directory `openspec/changes/feat-3530-resolve-invoke-model-routing-contract` no longer exists (moved via `git mv`, verified `[ -e source ]` false).

---

## Verification — Final State Authority

**Structured status handoff**: commit `abd15d21` is final; verify `pass` (6/6 req, 16/16 scenarios, evidence `6294aeb7` sha256 of HEAD); `size:exception` reset `d67874a0`; no extra commits after verify. Ranked per Final-State Authority hierarchy: orchestrator explicit final-state facts (rank 3) corroborate `verify-report` intermediate snapshot (rank 4) — no contradiction to record.

**Native Review Receipt Gate**: `reviewGate` structurally ABSENT — receipt-driven development does not govern this candidate (kill switch off or no review started post-verify). Per skill §Native Review Receipt Gate, absence is not a defect; `dependencies.archive: ready` means proceed. No `reviewGate` with non-`allow` result blocks archive.

**Verify report (intermediate snapshot, at verification time)**:
- Verdict: `pass` — `evidence_revision: sha256:6294aeb7`, base `v2.5.0-rc.2`
- `test_command: go test ./internal/agents/pi -count=1` → exit 0, hash `360dabfe...`
- `build_command: go vet ./internal/agents/pi` → exit 0, empty output `e3b0c442...`
- Additional: `go test ./internal/agents/pi -count=1 -v` PASS (12 suites, 30+ subtests), `go test ./internal/components/communitytool` PASS (no regression), `68.2%` coverage (threshold 0, not a failure)
- CRITICAL: 0, blockers 0 — archive not blocked
- WARNING: none (673 A+D >400 explicitly settled with `size:exception` — not a warning per report §Coherence)
- SUGGESTION: 2 deferred (relative `local:` path table widening; real-exec integration for `defaultModelRoutingRunner` — defense-in-depth only)

**Final numbers carried from highest-ranked source**: 6/6 req, 16/16 scenarios, 0 CRITICAL, 17/17 tasks — from `verify-report` + orchestrator final-state facts (no later commits changed them).

---

## SDD Cycle Complete

The change has been fully planned (proposal → spec → design → tasks), implemented (2 new + 2 modified files in `internal/agents/pi/` + 377-line test matrix), verified (independent `go test`/`go vet`/spec matrix), and archived with byte-identity proof. Ready for the next change (#3522 TUI PR2).

## Risks

None — no open CRITICAL/WARNING, no partial archive, no stale tasks, no destructive spec merge (new domain, no main spec to overwrite). Upstream `gentle-pi`/`docs/packages.md` drift remains isolated to `packageRootForSource` + exit map per design.

## Next Recommended

Merge `feat/3530-resolve-invoke-model-routing-contract` to main (cherry-picks cleanly on `v2.5.0-rc.2`); then start `feat/3522` PR2 (`internal/tui/screens` model-routing UI) via `sdd-init`/`sdd-propose`.
