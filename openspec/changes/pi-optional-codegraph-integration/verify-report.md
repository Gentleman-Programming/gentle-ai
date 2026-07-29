```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:e174e8b76f2fe8ab4b7d8189bb71dbcafb3027675d813d7858d18f29c6f5c08f
verdict: pass
blockers: 0
critical_findings: 0
requirements: 5/5
scenarios: 12/12
test_command: go test -count=1 ./...
test_exit_code: 0
test_output_hash: sha256:0d541aa04f973b29d2c0107ca18c03a94ddd3343a141d4fb20d59f0c8d703d2d
build_command: go vet ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: `pi-optional-codegraph-integration`
**Version**: N/A
**Mode**: Strict TDD
**Verdict**: PASS WITH WARNINGS

The current committed implementation plus the uncommitted R4-008 through R4-011 remediation satisfies all 5 requirements, 12 scenarios, and 16 tasks. Fresh focused and full uncached tests, vet, formatting, coverage, and final diff checks passed. Archive remains outside this verification result because the native status reports a missing approved review receipt.

### Completeness

| Metric | Value |
| --- | ---: |
| Requirements | 5/5 |
| Scenarios | 12/12 |
| Tasks total | 16 |
| Tasks complete | 16 |
| Tasks incomplete | 0 |

`gentle-ai sdd-status pi-optional-codegraph-integration --cwd /tmp/opencode/gentle-ai-pi-codegraph --json --instructions` reported `taskProgress.total=16`, `completed=16`, and `allComplete=true`. It also reported `reviewGate.result=invalidated`, `nextRecommended=resolve-review`, and `approved review receipt is missing`.

### Build, Tests, and Quality Execution

| Command | Exit | Exact result |
| --- | ---: | --- |
| `go test -count=1 ./internal/components/communitytool -run TestPiCodeGraphAdapterRuntimeFailsClosedWithoutPositiveCapabilityEvidence -v` | 0 | One top-level test and all 14 table scenarios passed; output `sha256:d51f5f36bb78975f9e2be6126f4c6483e19ad33e28f7f3e148d68d1fd5427163` |
| Frozen-ledger focused command across Pi adapter, community tool, uninstall, CLI, and TUI packages | 0 | R4-001 through R4-006 and R4-008 through R4-011 focused contracts passed; output `sha256:17a4c4aaea34c0b70869ab7225e2f25263e36bdef45fd3a75d719fe5095687d3` |
| `go test -count=1 ./...` | 0 | All repository packages passed uncached; output `sha256:0d541aa04f973b29d2c0107ca18c03a94ddd3343a141d4fb20d59f0c8d703d2d` |
| `go vet ./...` | 0 | No output; `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| Changed-package coverage command with `-count=1` | 0 | All 8 selected packages passed with coverage; output `sha256:551c88d341555136c0b351842f574e7b8f2294fcfbd3fc4cfb405ee0a594b469` |
| `gofmt -l` over Go paths changed from merge-base `b22a7eb8730e0e255c7a6d142aedfc606cbb020e` | 0 | No files listed; output `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| `git diff --check` | 0 | Uncommitted remediation has no whitespace errors; output `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| `git diff --check $(git merge-base origin/main HEAD)` after report refresh | 0 | Full candidate diff has no whitespace errors; output `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |

The initial full-candidate diff check exited 1 only because the previous verify report contained eight Markdown hard-break trailing-space lines. Replacing this phase artifact removed that stale report-only defect; no production file was edited.

### R4-011 Exact Adapter Status Parsing

The current parser lowercases and trims combined Pi output, rejects explicit adapter/config failures and unknown `/mcp`, collects only lines whose trimmed capability name is exactly `codegraph`, requires exactly one such record, and accepts only exact states from `connected`, `ready`, `running`, `loaded`, `active`, or `ok`.

Fresh runtime-seam evidence passed all 14 cases:

- Rejected: failed MCP config, adapter-load failure, unknown `/mcp`, session-only output, `not ready`, `not running`, `unloaded`, `inactive`, `broken`, malformed status, unrelated healthy status, and duplicate/ambiguous CodeGraph records.
- Accepted: exactly one `codegraph: connected` or `codegraph: ready` record.
- The companion project-override test confirmed the production runner receives the effective MCP path and `PI_CODING_AGENT_DIR`.

### Frozen Severe Ledger Resolution

| Ledger rows | Persisted status | Fresh evidence | Result |
| --- | --- | --- | --- |
| R4-001 through R4-006 | `verified` | Effective project MCP failure, snapshot coverage, transactional uninstall, unreadable-child rejection, joined restore errors, and unmatched-marker integrity tests passed | Resolved |
| R4-007 | WARNING / `info` | No severe disposition required | Informational |
| R4-008 | `fixed` | Adapter-directory/project-MCP decoupling test passed | Behaviorally resolved |
| R4-009 | `fixed` | Dynamic-overlay pipeline rollback test passed | Behaviorally resolved |
| R4-010 | `fixed` | CLI and TUI manual-action propagation tests passed | Behaviorally resolved |
| R4-011 | `fixed` | Exact parser and project-runtime tests passed, including 14 parser cases | Behaviorally resolved |

No BLOCKER or CRITICAL ledger row remains open. Final review approval is not claimed: the OpenSpec tree has Markdown ledgers but no native `reviews/transaction.json` or approved `reviews/receipt.json`, so the native review gate remains invalidated.

### Spec Compliance Matrix

| Requirement | Scenario | Covering runtime test/evidence | Result |
| --- | --- | --- | --- |
| Optional selection boundary | Pi unselected | `TestPiCodeGraphUnselectedIsNoOp`; install-pipeline deselection test | COMPLIANT |
| Optional selection boundary | Selection records ownership | `TestInstallWithHomeReportsWorkspaceChildAndOwnershipTarget` | COMPLIANT |
| Selected provisioning and direct verification | Compatible child | Child classification and effective MCP schema tests | COMPLIANT |
| Selected provisioning and direct verification | Marker-only evidence | Parent-marker and noncanonical MCP rejection tests | COMPLIANT |
| Selected provisioning and direct verification | Unsupported child | Compatible/guidance-only child test | COMPLIANT |
| Root-scoped lazy initialization | Index absent | Guidance ordering and valid Git-root fixture | COMPLIANT |
| Root-scoped lazy initialization | Init fails | Guidance contract requires attempted init before reported fallback | COMPLIANT |
| Reconciliation, idempotence, and recovery | Sync drift | Missing-owned-child restoration and sync ordering tests | COMPLIANT |
| Reconciliation, idempotence, and recovery | Repeat provisioning | Byte-idempotence test | COMPLIANT |
| Reconciliation, idempotence, and recovery | Update fails | Existing-child/MCP and new-overlay rollback tests | COMPLIANT |
| Ownership-safe removal | User-config uninstall | Adoption, drift, overlay, and service-hook tests | COMPLIANT |
| Ownership-safe removal | Repeat uninstall | Direct and service repeat-uninstall assertions | COMPLIANT |

**Compliance summary**: 12/12 scenarios compliant.

### Requirement Correctness

| Requirement | Status | Static and runtime evidence |
| --- | --- | --- |
| Optional selection boundary | Implemented | Selection gates reconciliation; deselection removes only owned artifacts; normal Pi install remains unchanged |
| Selected provisioning and direct verification | Implemented | Canonical MCP merge, child classifications, direct child/tool/guidance verification, and fail-closed adapter status |
| Root-scoped lazy initialization | Implemented | Guidance resolves and validates root, checks the index before exploration, initializes once, and explains fallback |
| Reconciliation, idempotence, and recovery | Implemented | Reconcile-last lifecycle, byte no-op, drift repair, journal rollback, and delayed manifest commit |
| Ownership-safe removal | Implemented | Before-image adoption, overlay deletion, drift preservation, service hook, and repeat-safe cleanup |

### Task Verification

| Phase | Tasks | Result |
| --- | --- | --- |
| Pi contracts and selection | 1.1-1.4 | 4/4 complete |
| Provisioning and direct verification | 2.1-2.4 | 4/4 complete |
| Reconciliation and recovery | 3.1-3.4 | 4/4 complete |
| Ownership-safe removal and evidence | 4.1-4.4 | 4/4 complete |

### Design Coherence

| Decision | Followed? | Evidence |
| --- | --- | --- |
| Keep gentle-pi CodeGraph-agnostic | Yes | No gentle-pi path changed; package-source preservation test passed |
| Use the observed Pi MCP contract | Yes | Canonical server merge plus strict adapter and tools/list validation |
| Discover effective user/project/package children | Yes | Precedence, workspace target, package overlay, and unreadable discovery tests |
| Inject least-privilege tools | Yes | Only compatible children receive `mcp`; all readable children receive guidance |
| Never accept a parent marker as capability | Yes | Marker-only verification fails |
| Reconcile after refresh and remain idempotent | Yes | Sync final-step and repeated-provision tests pass |
| Restore before-images and remove only owned artifacts | Yes | Rollback, adoption, drift, deselection, and service-hook tests pass |

### TDD Compliance

| Check | Result | Details |
| --- | --- | --- |
| TDD evidence reported | Pass | Apply progress contains the 16-task matrix and remediation RED/GREEN records |
| Task objectives mapped | Pass | 16/16 objectives map to executable tests, documentation review, or the final quality gate |
| RED evidence present | Pass | Original and remediation failure classes are recorded with focused commands |
| GREEN confirmed fresh | Pass | All mapped current tests pass; full suite passes uncached |
| Triangulation adequate | Pass | Success/failure schemas, ownership, drift/repeat, root safety, rollback, and 14 exact-parser variants execute |
| Safety net and rollback boundary | Pass | Apply progress records safety nets and bounded rollback evidence |

**TDD compliance**: 6/6 checks passed.

### Test Layer Distribution

The candidate adds 45 change-related top-level tests across 7 files.

| Layer | Tests | Files | Tool |
| --- | ---: | ---: | --- |
| Unit/structural | 13 | 5 | Go `testing` |
| Integration/runtime fixture | 32 | 5 | Go `testing`, `t.TempDir`, filesystem/process seams |
| E2E | 0 | 0 | Not used |
| **Total** | **45** | **7** | |

### Changed File Coverage

Go reports statement rather than branch coverage. Weighted statement coverage across changed production Go files with executable statements is 67.2%. Coverage is informational under the Strict TDD verification contract.

| Changed production file | Statement coverage | Rating |
| --- | ---: | --- |
| `internal/agents/pi/adapter.go` | 68.8% | Low |
| `internal/app/app.go` | 65.1% | Low |
| `internal/cli/run.go` | 79.0% | Low |
| `internal/cli/sync.go` | 82.6% | Acceptable |
| `internal/components/communitytool/codegraph_guidance.go` | 72.4% | Low |
| `internal/components/communitytool/pi_codegraph.go` | 79.5% | Low |
| `internal/components/communitytool/tool.go` | 77.1% | Low |
| `internal/components/uninstall/service.go` | 61.6% | Low |
| `internal/pipeline/result.go` | N/A | Structural fields only |
| `internal/tui/model.go` | 58.0% | Low |
| `internal/tui/screens/complete.go` | 46.6% | Low |

### Assertion Quality

All added/modified change-related test deltas were inspected. They call production boundaries and assert concrete errors, bytes, classifications, command arguments/environment, state, output, ownership, and side effects. No tautologies, assertion-free paths, ghost loops, smoke-only checks, CSS/internal-style assertions, or mock-heavy files were found.

**Assertion quality**: All assertions verify real behavior.

### Quality Metrics

**Formatter**: Passed

**Linter**: Not available; no configured linter

**Static checker**: `go vet ./...` passed

**Whitespace integrity**: Full merge-base candidate diff passed after this report refresh

**Generated index residue**: `.codegraph/` absent after verification

### Issues Found

**CRITICAL**: None.

**WARNING**:

1. Nine changed production Go files with executable statements are below the informational 80% statement-coverage threshold; only `internal/cli/sync.go` is above it.
2. Native review approval is absent. `gentle-ai sdd-status` reports an invalidated review gate and routes to `resolve-review`; verification does not manufacture or substitute a receipt.

**SUGGESTION**: Persist a native review transaction and approved receipt for the exact final candidate before archive.

### Verdict

**PASS WITH WARNINGS**

Implementation verification passes. The next action is `resolve-review`, not archive, because the approved receipt is missing.
