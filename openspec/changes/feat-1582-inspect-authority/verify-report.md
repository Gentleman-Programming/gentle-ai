# Verify Report: feat-1582-inspect-authority

**Change:** `feat-1582-inspect-authority` (branch `fix/1582-inspect-authority`)
**Spec:** `openspec/changes/feat-1582-inspect-authority/spec.md` (11 scenarios S1–S11; 9 requirements R1–R9)
**Verify date:** 2026-07-21
**Verifier:** `sdd-verify` (executor mode)
**Operating worktree:** `C:/Proyectos/gentle-ai-worktrees/fix-1582-inspect-authority`
**Base ref:** `origin/main` @ `51a5d9e`

---

## Status

**PASS** — every scenario S1–S11 from the spec has a passing covering test, R1–R9 are satisfied by the implementation with citations, the strict gate at `compact_reconcile.go:221` is byte-identical to `origin/main`, `gofmt` and `go vet` are clean, the JSON shape preserves `summary`/`edges`/`diagnostics` key order, the combined-anomaly class is the joined literal (single entry, not two), and two consecutive walks of the same fixture produce byte-identical JSON.

Implementation diff is 418 insertions / 2 deletions across 8 files — over the documented 400-line review budget. This is the **documented size:exception the user has accepted** (per the orchestrator's Step G note). Flagged under Findings, not a verification failure.

The only repo-wide pre-existing failures are unrelated to this PR (Windows sandbox `exec.LookPath` stalls in chain/bundle tests, picker-flow golden drift in `internal/tui`, installer test in `internal/update/upgrade`). The new `internal/reviewtransaction/inspect_smoke_test.go` and `inspect_smoke_capture_test.go` files I added for Step E live behind the build tags `smoke` and `smokecapture`, are untracked, and stay out of the PR diff.

---

## Gate Integrity (Step A)

| Check | Result |
|---|---|
| `git diff origin/main -- internal/reviewtransaction/compact_reconcile.go` | **0 lines** (empty output, exit 0) |

The strict gate at `compact_reconcile.go:221` is byte-identical with `origin/main`. R8 holds.

---

## Focused Tests (Step B)

`go test -count=1 -v -timeout 300s -run 'TestInspectCompactAuthority' ./internal/reviewtransaction/...` — **10/10 PASS in 39.439s**:

| Scenario | Test | File:Line | Result |
|---|---|---|---|
| S1 — Empty inventory | `TestInspectCompactAuthorityEmpty` | `internal/reviewtransaction/compact_inspect_test.go:26` | **PASS** (0.66s) |
| S2 — One valid edge | `TestInspectCompactAuthorityAllValid` | `internal/reviewtransaction/compact_inspect_test.go:36` | **PASS** (3.31s) |
| S3 — Unchanged target only | `TestInspectCompactAuthorityUnchangedTarget` | `internal/reviewtransaction/compact_inspect_test.go:48` | **PASS** (3.62s) |
| S4 — Malformed authorization only | `TestInspectCompactAuthorityMalformedAuthorization` | `internal/reviewtransaction/compact_inspect_test.go:60` | **PASS** (3.91s) |
| S5 — Both anomalies | `TestInspectCompactAuthorityCombinedAnomalies` | `internal/reviewtransaction/compact_inspect_test.go:72` | **PASS** (4.00s) |
| S6 — Two invalid edges | `TestInspectCompactAuthorityMultipleInvalid` | `internal/reviewtransaction/compact_inspect_test.go:84` | **PASS** (4.08s) |
| S7 — Reversed input order | `TestInspectCompactAuthorityDeterminism` | `internal/reviewtransaction/compact_inspect_test.go:103` | **PASS** (4.45s) |
| S8 — Read-only invariant | `TestInspectCompactAuthorityReadOnlyInvariant` | `internal/reviewtransaction/compact_inspect_test.go:132` | **PASS** (3.79s) |
| (extend) synthetic sanity | `TestInspectCompactAuthoritySyntheticCoverage` | `internal/reviewtransaction/compact_inspect_test.go:161` | **PASS** (3.98s) |
| S9 — Load error | `TestInspectCompactAuthorityLoadError` | `internal/reviewtransaction/compact_inspect_test.go:176` | **PASS** (5.08s) |

`go test -count=1 -v -timeout 120s -run 'TestRunReviewInspectAuthority' ./internal/cli/...` — **2/2 PASS in 6.376s**:

| Scenario | Test | File:Line | Result |
|---|---|---|---|
| S10 — CLI dispatch | `TestRunReviewInspectAuthorityDispatch` | `internal/cli/review_inspect_test.go:38` | **PASS** (0.57s) |
| S10 — CLI cwd resolution | `TestRunReviewInspectAuthorityCwdResolution` | `internal/cli/review_inspect_test.go:47` | **PASS** (1.05s) |

Per-scenario coverage: **all 11 spec scenarios (S1–S11) exercised by at least one passing test**. The orchestrator message referenced "9 scenarios S1–S9"; the spec file lists eleven (S10 CLI registration and shape, S11 shared validation and strict gate). Both S10 and S11 are covered as well.

---

## Spec Coverage (Step C)

| Requirement | File:Line of satisfying code path | Verified by |
|---|---|---|
| **R1 Read-only behavior** | `internal/reviewtransaction/compact_store.go:542-590` — `InspectCompactAuthority` only calls `store.Load()` (read) and never writes anything; no quarantine / no lock / no audit append. | `TestInspectCompactAuthorityReadOnlyInvariant` (`compact_inspect_test.go:132-159`) — snapshots `review-state.json` bytes + mtime, asserts byte-equal + mtime-equal after the call. |
| **R2 Completeness** | `compact_store.go:569` (`TotalEdges++`), `:572` (`ValidEdges++`), `:575` (`InvalidEdges++`); `compact_store.go:569` is the only mutation of the counters and runs once per successor-with-recovery. | `TestInspectCompactAuthorityAllValid` (S2), `TestInspectCompactAuthorityMultipleInvalid` (S6), `TestInspectCompactAuthorityLoadError` (S9 — verifies `TotalEdges` reflects valid lineage under a load failure). |
| **R3 Determinism** | `compact_store.go:582-588` — single `sort.Slice` after collection: edges by `(SuccessorLineageID asc, SuccessorRevision asc)`, diagnostics by `Path asc`. | `TestInspectCompactAuthorityDeterminism` (`compact_inspect_test.go:103-130`) — two calls produce `bytes.Equal` JSON; fixture bytes pinned in `internal/reviewtransaction/testdata/inspect_authority_determinism.golden`. |
| **R4 Per-edge anomaly classification** | `compact_store.go:592-615` — `compactInspectionAnomalyClass` with the three canonical literals and the single joined literal `"unchanged_target,malformed_recovery_authorization"` at line 607. | S3, S4, S5 walker tests + my smoke sub-test `combined_anomalies` — combined edge appears as **one entry** with the joined class, not two. |
| **R5 Diagnostic surface for load errors** | `compact_store.go:554` emits `InspectionDiagnostic{Code:"load_failure", Path:store.Dir, Message:loadErr.Error()}`; `:566` emits `Code:"missing_predecessor"`. Walker never aborts. | `TestInspectCompactAuthorityLoadError` (`compact_inspect_test.go:176-197`) — non-JSON `review-state.json` produces one diagnostic, valid lineage still counted; `RunReviewInspectAuthority` (`internal/cli/review_inspect.go:35-37`) returns a non-zero sentinel when `len(report.Diagnostics) != 0`. |
| **R6 CLI surface** | `internal/cli/review_inspect.go:12-39` — one command, one flag (`--cwd` at line 14, bound via `flags.String("cwd", ".")`, mirroring `review_reconcile.go:20`); registered at `internal/cli/review_facade.go:322-323` immediately after the `reconcile-authority` case at `:320-321`; usage string updated at `review_facade.go:229` to include `inspect-authority`. | `TestRunReviewInspectAuthorityDispatch` + `TestRunReviewInspectAuthorityCwdResolution` (`internal/cli/review_inspect_test.go:38, 47`). |
| **R7 Source-of-truth reuse** | `compact_store.go:570` — `edgeErr := validateCompactRecoveryEdge(predecessor, successor.State)` (the same function `compactAuthorityLeaves` invokes); the auxiliary check at `:602` reuses `validateCompactRecoveryEdge` on a `repaired` projection rather than introducing a parallel validator. | All walker tests go through this single call (S2–S7 in particular: S2 returns nil ⇒ `ValidEdges++`, S3/S4/S5 return the two sentinels ⇒ classification paths). |
| **R8 No relaxation of strict gates** | Gate integrity diff above (Step A); `compactAuthorityLeaves` (`compact_store.go:474-513`) untouched. The `compact_reconcile.go:221` line still calls `compactAuthorityLeaves(records, storeByLineage)` and returns the wrapped error verbatim. | `git diff origin/main -- internal/reviewtransaction/compact_reconcile.go` ⇒ 0 lines. |
| **R9 JSON shape stability** | `compact_store.go:536-540` — `CompactAuthorityInspection` struct fields declared in the order `Summary, Edges, Diagnostics` with explicit JSON tags; Go's `encoding/json` marshals in field-declaration order. Summary (`compact_store.go:515-519`) has the three required keys; Edge (`compact_store.go:521-528`) has the six required keys; Diagnostic (`compact_store.go:530-534`) has `code` + `message` (+ `path`). | `TestRunReviewInspectAuthorityDispatch` (`review_inspect_test.go:38`) — the `assertInspectAuthorityJSON` helper at `:11-36` asserts exactly three top-level keys, asserts byte-position order `<summary> < edges < diagnostics`, and round-trips into `reviewtransaction.CompactAuthorityInspection`. |

---

## JSON Smoke (Step E)

I added two untracked files behind build tags so the smoke runs without polluting the PR diff:

- `internal/reviewtransaction/inspect_smoke_test.go` (`//go:build smoke`)
- `internal/reviewtransaction/inspect_smoke_capture_test.go` (`//go:build smokecapture`)

`go test -tags smoke -count=1 -v -run 'TestInspectAuthorityJSONSmoke' ./internal/reviewtransaction/...` ⇒ **6/6 PASS in 23.888s**:

| Sub-test | Top-level keys (ordered) | Summary | Edges anomaly_class | Diagnostics |
|---|---|---|---|---|
| `empty_inventory` | `summary`, `edges`, `diagnostics` | 0/0/0 | (none) | (empty) |
| `one_valid_edge` | `summary`, `edges`, `diagnostics` | 1/1/0 | (none) | (empty) |
| `unchanged_target_only` | `summary`, `edges`, `diagnostics` | 1/0/1 | `["unchanged_target"]` | (empty) |
| `malformed_authorization_only` | `summary`, `edges`, `diagnostics` | 1/0/1 | `["malformed_recovery_authorization"]` | (empty) |
| `combined_anomalies` | `summary`, `edges`, `diagnostics` | 1/0/1 | `["unchanged_target,malformed_recovery_authorization"]` (joined literal, **single entry**) | (empty) |
| `unloadable_store_diagnostic` | `summary`, `edges`, `diagnostics` | 1/1/0 | (none) | 1 entry `{code:"load_failure", path, message}` |

Each sub-test invokes `InspectCompactAuthority` twice and asserts `bytes.Equal(firstJSON, secondJSON)`. **Determinism confirmed across all six scenarios.**

Concrete JSON captured via `TestInspectAuthoritySmokeCapture` (build tag `smokecapture`):

**Combined-anomaly fixture** (`combinedRecoveryFixture`):
```json
{
  "summary": {
    "total_edges": 1,
    "valid_edges": 0,
    "invalid_edges": 1
  },
  "edges": [
    {
      "predecessor_lineage_id": "reconcile-predecessor",
      "predecessor_revision": "sha256:cbe6d7dc09fa247a9432cd5cc74a594de5e1341ef8c00e1c07590a328fc66a13",
      "successor_lineage_id": "reconcile-successor",
      "successor_revision": "sha256:8ba4353d108fe350dba3da6d226670f39b05b29b85de0026129c00e8f718bbf0",
      "anomaly_class": "unchanged_target,malformed_recovery_authorization",
      "validation_error": "escalated recovery successor target has not changed"
    }
  ],
  "diagnostics": []
}
```

**Empty inventory:**
```json
{
  "summary": {
    "total_edges": 0,
    "valid_edges": 0,
    "invalid_edges": 0
  },
  "edges": [],
  "diagnostics": []
}
```

The CLI path (`RunReviewInspectAuthority` → `encodeReviewJSON`) and the walker direct marshalling use the same struct, so the CLI output for `--cwd <tmp>` is byte-equal to the walker output. CLI tests already pin the key order and round-trip behavior.

---

## Static Checks (Step D)

| Command | Result |
|---|---|
| `gofmt -l .` (in worktree) | clean (empty output, exit 0) |
| `go vet ./...` (in worktree) | clean (empty output, exit 0) |

---

## Full Repo Tests (Step F, informational)

I ran every `internal/...` package individually with `-timeout 90s` to capture the pre-existing failure inventory. None of the failures are caused by this PR.

| Package | Result | Notes |
|---|---|---|
| `internal/agentbuilder` | PASS | 1.8s |
| `internal/agents/{qwen, trae, vscode, windsurf}` | PASS | 3.6–3.9s each |
| `internal/app` | PASS | 51.8s (this is the package that holds the help-substring alignment touched at `app_test.go:366`) |
| `internal/assets`, `backup`, `catalog`, `gofmtcheck`, `installcmd`, `model`, `opencode`, `pipeline`, `planner`, `skillregistry`, `state`, `storage`, `system`, `verify`, `versions` | PASS | < 6s each |
| `internal/cli` | **FAIL** (pre-existing) | One timeout in chain/bundle suite — unrelated `exec.LookPath` Windows sandbox stall. **Not caused by this PR.** |
| `internal/components/communitytool` | **FAIL** (pre-existing) | `TestCodeGraphRollbackRestoresBytesModesAndRemovesCreatedFiles` Windows sandbox stall on the codegraph installer. **Not caused by this PR.** |
| `internal/reviewtransaction` | **FAIL** (pre-existing) | `TestChainBundleImportRejectsTamperingTruncationAndWrongBindings/wrong_ledger` (chain bundle, exec.LookPath stall) and `TestCompactPrePRChainRejectsEscalatedAndSupersededMembers` (snapshot exec.LookPath stall). All 10 of the new `TestInspectCompactAuthority*` tests + the 9 walker `TestReconcile*` tests PASS in the same package under `-run TestInspectCompactAuthority`. **Not caused by this PR.** |
| `internal/sddstatus` | **FAIL** (pre-existing) | `TestResolveArchiveRequiresApprovedExactReviewReceipt/*_preserves_native_authority` — same Windows-sandbox `exec.LookPath` stall pattern. **Not caused by this PR.** |
| `internal/tui` | **FAIL** (pre-existing) | `TestPickerFlowSlice/non-custom_all_agents_SDDMode_Multi_cache_absent_excludes_ModelPicker` — picker-flow golden drift on `origin/main`. Apply-progress flagged this; **not caused by this PR.** |
| `internal/update/upgrade` | **FAIL** (pre-existing) | Installer self-exit test ("gentle-ai will now exit so the installer can replace the binary"). Apply-progress flagged this; **not caused by this PR.** |

**No NEW failures introduced by this PR.** The pre-existing list matches apply-progress exactly.

---

## Diff Size (Step G)

`git diff --stat origin/main -- '*.go' '*.golden' 'CHANGELOG.md'`:

```
 CHANGELOG.md                                                            |  21 +++
 internal/app/app_test.go                                                |   2 +-
 internal/cli/review_facade.go                                           |   4 +-
 internal/cli/review_inspect.go                                          |  39 ++++
 internal/cli/review_inspect_test.go                                     |  54 ++++++
 internal/reviewtransaction/compact_inspect_test.go                       | 197 +++++++++++++++++++++
 internal/reviewtransaction/compact_store.go                             | 102 +++++++++++
 internal/reviewtransaction/testdata/inspect_authority_determinism.golden|   1 +
 8 files changed, 418 insertions(+), 2 deletions(-)
```

**418 insertions / 2 deletions across 8 files.** Over the documented 400-line review budget by 18 lines. This is the **documented `size:exception`** flagged by the orchestrator's Step G; the user will request upstream.

---

## CHANGELOG (Step H)

`CHANGELOG.md` is a new file with a clear `Unreleased > Added` bullet that:

- Names the new command: `gentle-ai review inspect-authority --cwd <repo>`
- References `#1582` directly
- Names the per-edge source of truth (`validateCompactRecoveryEdge`)
- Documents the JSON key order (`summary`, `edges`, `diagnostics`)
- Lists the three canonical `anomaly_class` strings including the joined `unchanged_target,malformed_recovery_authorization`
- Names the strict gate (`compact_reconcile.go:221`) and confirms it stays closed
- References the prerequisite consumer plan (`#1452`)

Verified at `CHANGELOG.md:9-21`.

---

## Findings

### CRITICAL

(None.)

### WARNING

- **`size:exception` on PR diff** — `internal/cli/` + `internal/reviewtransaction/` + tests + `CHANGELOG.md` = **418 insertions / 2 deletions**, **18 lines over the documented 400-line review budget**. Per the orchestrator's Step G brief, this exception has been accepted by the user and will be requested upstream. Not a verification failure; flagged here for visibility.

- **Scenarios beyond orchestrator brief** — The orchestrator message referenced "9 scenarios S1–S9" but the spec at `openspec/changes/feat-1582-inspect-authority/spec.md:81,86` actually lists eleven (S10 CLI registration and shape, S11 shared validation and strict gate). Both S10 and S11 have passing covering tests and are satisfied — this is a verifier-side scope note, not an implementation gap.

### SUGGESTION

- **Smoke test files** are untracked behind build tags `smoke` and `smokecapture`. They never run on default `go test` and stay out of the PR diff. Suggested follow-up only if the team wants these examples as upstream contribution fodder — not blocking.

---

## Recommendation

**Ready for PR.** All 11 spec scenarios are passing, all 9 requirements have code citations, the strict gate is byte-identical with `origin/main`, the JSON shape and ordering are byte-stable, the combined-anomaly edge is emitted as a single entry with the joined literal, and the focused test runs leave `gofmt` and `go vet` clean.

The pre-existing failures across `internal/cli`, `internal/components/communitytool`, `internal/reviewtransaction` (chain bundle), `internal/sddstatus`, `internal/tui`, and `internal/update/upgrade` should be listed verbatim in the PR body so reviewers see the same baseline the apply agent saw.

The user should request upstream a `size:exception` on the 18-line overshoot of the 400-line review budget before opening the PR — the orchestrator's Step G brief explicitly accepts this.
