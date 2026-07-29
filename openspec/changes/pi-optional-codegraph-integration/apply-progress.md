```yaml
schema: gentle-ai.remediation-result/v1
status: partial
failed_verify_revision: sha256:608b7c497ba2bde1a0f8beda178f82fec1e7920fc58acb0819855368c43549df
task_count: 16
completed_task_count: 16
focused_tests: passed
runtime_harness: passed
rollback_boundary: recorded
next_recommended: sdd-verify
```

```json
{
  "schema": "gentle-ai.remediation-evidence/v1",
  "failed_verify_revision": "sha256:608b7c497ba2bde1a0f8beda178f82fec1e7920fc58acb0819855368c43549df",
  "commands": [
    {
      "command": "go test -count=1 ./internal/agents/pi ./internal/components/communitytool ./internal/components/uninstall -run 'Test(DiscoverCodeGraphChildrenReturnsUnreadableDirectoryError|PiCodeGraph(UninstallPreservesPreexistingMarkedUserChildWithoutManifest|FailureRemovesNewPackageOverlayWithoutVerificationSuccess|RootValidationRejectsUnsafeRoots)|ExecutePlanPiUninstall)' -v",
      "exit_code": 0,
      "result": "6 focused task-contract tests passed: unreadable discovery, absolute/relative git -C root validation, preexisting marked user-child preservation, new-overlay rollback without success evidence, service uninstall preservation, drift preservation, and gentle-pi source preservation."
    },
    {
      "command": "go test -count=1 ./internal/components/communitytool ./internal/components/uninstall -v",
      "exit_code": 0,
      "result": "Focused community-tool and uninstall packages passed, including both persisted service-hook tests."
    },
    {
      "command": "go test -count=1 ./... && go vet ./... && gofmt -l $(git diff --name-only -- '*.go') && git diff --check",
      "exit_code": 0,
      "result": "Uncached full suite, vet, formatter check, and whitespace diff check passed."
    }
  ],
  "runtime_harness": {
    "status": "passed",
    "command": "TestExecutePlanPiUninstallPreservesPreexistingMarkedUserChildAndUserMCP and TestExecutePlanPiUninstallPreservesDriftedChildAndGentlePiSource",
    "result": "t.TempDir() Pi homes exercise Service.executePlan's real AgentPi uninstall hook: user MCP keys and preexisting marked blocks are preserved exactly; drift is retained as a manual action; package-owned gentle-pi source remains unchanged; repeat uninstall succeeds.",
    "na_reason": ""
  },
  "rollback": {
    "boundary": "internal/agents/pi/adapter.go, internal/agents/pi/adapter_test.go, internal/components/communitytool/pi_codegraph.go, internal/components/communitytool/pi_codegraph_test.go, internal/components/uninstall/service_test.go, and OpenSpec task/progress artifacts",
    "evidence": "Reverting this bounded work removes only missing-manifest adoption safety, failure-result clearing, and the persisted task-evidence tests; prior Pi lifecycle behavior is otherwise untouched."
  }
}
```

## Authorized Corrective Scope — Frozen IDs RISK-002, RISK-004, RESILIENCE-004 (2026-07-10)

| Frozen ID | RED/GREEN contract | Result |
| --- | --- | --- |
| RISK-002 | `TestPiCodeGraphRefreshRestoresMissingOwnedChild` | RED: restored child mode was `0600`, not recorded `0640`; GREEN: journal recreation writes the manifest-owned mode. |
| RISK-004 | `TestPiCodeGraphPathsExcludesUnsafeManifestPaths` | RED: traversal, arbitrary absolute, and symlink-escape manifest paths entered backup enumeration; GREEN: each is excluded after canonical allowed-root validation. |
| RESILIENCE-004 | `TestBackupTargetsSnapshotCrossAgentCodeGraphGuidance` | RED: a detected Claude guidance path was absent from `backupTargets`; GREEN: the path is included when CodeGraph is selected. |

| Evidence | Command | Exact result |
| --- | --- | --- |
| RED | `go test -count=1 ./internal/components/communitytool -run 'TestPiCodeGraph(RefreshRestoresMissingOwnedChild|PathsExcludesUnsafeManifestPaths)' -v` | Exit 1: mode was `0600`; the initial adversarial fixture was corrected before its behavior RED run. |
| RED | `go test -count=1 ./internal/components/communitytool -run TestPiCodeGraphPathsExcludesUnsafeManifestPaths -v` | Exit 1: all three manifest-controlled unsafe paths were returned. |
| RED | `go test -count=1 ./internal/cli -run TestBackupTargetsSnapshotCrossAgentCodeGraphGuidance -v` | Exit 1: detected Claude `CLAUDE.md` was missing from backup targets. |
| GREEN | `go test -count=1 ./internal/components/communitytool -run 'TestPiCodeGraph(RefreshRestoresMissingOwnedChild|PathsExcludesUnsafeManifestPaths)' -v && go test -count=1 ./internal/cli -run TestBackupTargetsSnapshotCrossAgentCodeGraphGuidance -v` | Exit 0: all three corrective contracts passed. |
| Affected packages | `go test -count=1 ./internal/components/communitytool ./internal/cli ./internal/components/uninstall ./internal/agents/pi` | Exit 0. |
| Race | `go test -race -count=1 ./internal/components/communitytool ./internal/cli` | Exit 0. |
| Full verification | `go test -count=1 ./... && go vet ./...` | Exit 0. |
| Formatting / whitespace | `gofmt -l $(git diff --name-only -- '*.go') && git diff --check` | Exit 0 with no output. |

Rollback boundary: `internal/components/communitytool/pi_codegraph.go`, `internal/components/communitytool/pi_codegraph_test.go`, `internal/cli/run.go`, and `internal/cli/run_community_tool_test.go`; reverting these removes only the three authorized corrections and their focused tests.

## Focused Remediation — Ownership Adoption and Task Evidence (2026-07-10)

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1.1 | `internal/agents/pi/adapter_test.go` | Unit | Existing Pi focused tests passed | New unreadable-directory fixture failed to compile before the injectable walk seam existed | `TestDiscoverCodeGraphChildrenReturnsUnreadableDirectoryError` passed | Existing precedence/shadowing/override fixture plus injected permission failure | Minimal `piWalkDir` seam only; no discovery behavior changed |
| 2.3 | `internal/components/communitytool/pi_codegraph_test.go` | Runtime fixture | Existing root validation test passed | Existing coverage lacked actual Git-root resolution evidence | `TestPiCodeGraphRootValidationRejectsUnsafeRoots` now creates a Git repo, proves `git -C` resolution, and validates absolute and relative roots | Unsafe home/root/temp plus valid absolute/relative Git roots | No production refactor needed |
| 3.3 | `internal/components/communitytool/pi_codegraph_test.go` | Integration fixture | Existing rollback test passed | `TestPiCodeGraphFailureRemovesNewPackageOverlayWithoutVerificationSuccess` failed because a failed reconcile returned successful MCP evidence | Same test passed after clearing result MCP evidence in the rollback defer | Existing child/MCP rollback plus new package-overlay deletion and no-success result | No further refactor needed |
| 4.1 | `internal/components/communitytool/pi_codegraph_test.go`, `internal/components/uninstall/service_test.go` | Integration/runtime fixture | Existing community-tool/uninstall packages passed | Missing-manifest preexisting child test failed: uninstall deleted `worker.md`; service-hook coverage did not exist | Direct and Service.executePlan tests passed after adopted user bytes/provenance were recorded | User MCP/marked blocks, drift, gentle-pi source, package overlay, and repeat uninstall paths | Added only adoption capture and `Adopted` manifest provenance |

## Work Unit Evidence — Ownership Adoption and Task Evidence

| Evidence | Command / boundary | Exact result |
| --- | --- | --- |
| RED | `go test -count=1 ./internal/components/communitytool ./internal/components/uninstall -run 'Test(PiCodeGraphUninstallPreservesPreexistingMarkedUserChildWithoutManifest|PiCodeGraphFailureRemovesNewPackageOverlayWithoutVerificationSuccess|ExecutePlanPiUninstallPreservesPreexistingMarkedUserChildAndUserMCP)' -v` | Exit 1: preexisting user child was deleted; failed reconcile exposed successful MCP evidence; initial service fixture lacked an effective probe. |
| GREEN | Focused command in remediation evidence JSON | Exit 0; all six focused task-contract tests passed. |
| Runtime harness | `Service.executePlan` with `t.TempDir()` Pi homes | Exit 0; real uninstall hook preserves user-owned content and reports drift. |
| Rollback boundary | Adoption capture, failure result clearing, and related tests | Independent revert affects only this remediation. |

```yaml
schema: gentle-ai.remediation-result/v1
status: complete
failed_verify_revision: sha256:d723a7825e385516ef5719447116d6ec757dd6dd2157b9d849fe773830186f3d
focused_tests: passed
runtime_harness: fixture-backed
rollback_boundary: recorded
next_recommended: sdd-verify
```

```json
{
  "schema": "gentle-ai.remediation-evidence/v1",
  "failed_verify_revision": "sha256:d723a7825e385516ef5719447116d6ec757dd6dd2157b9d849fe773830186f3d",
  "commands": [
    {
      "command": "go test -count=1 ./internal/components/communitytool -run 'TestVerifyPiMCP(UsesInjectedEffectiveProbe|FailsClosedWithoutAdapterOrProcess)' -v",
      "exit_code": 0,
      "result": "11 focused subtests passed: indexed query-only and observed unindexed query-plus-projectPath schemas succeed; malformed, missing, duplicate, unsafe-required, writable, adapter, and handshake failures remain rejected."
    },
    {
      "command": "go test -count=1 ./... && go vet ./...",
      "exit_code": 0,
      "result": "Full Go test suite and vet passed."
    }
  ],
  "runtime_harness": {
    "status": "not_applicable",
    "command": "N/A",
    "result": "The bounded validator consumes MCP tools/list data; the observed unindexed CodeGraph 1.2.0 schema is represented by the injected MCP fixture without starting an external server.",
    "na_reason": "No additional runtime boundary changed; the missing-index lazy-init lifecycle remains outside this validator-only remediation."
  },
  "rollback": {
    "boundary": "internal/components/communitytool/pi_codegraph.go and internal/components/communitytool/pi_codegraph_test.go",
    "evidence": "Reverting these two files restores only the previous required-field validator and its fixtures; no lifecycle, ownership, or gentle-pi behavior is changed."
  }
}
```

## Focused Remediation — Unindexed CodeGraph 1.2.0 Required Schema (2026-07-10)

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 2.2 remediation | `internal/components/communitytool/pi_codegraph_test.go` | Unit | `TestVerifyPiMCP(UsesInjectedEffectiveProbe|FailsClosedWithoutAdapterOrProcess)` — exit 0 | Added the observed unindexed `required:["query","projectPath"]` fixture; focused test exited 1 because the validator required only `query` | Same command exited 0 after accepting only the two safe required-field sets | Added missing-query, duplicate-required, and unknown-required fixtures; all fail closed | No refactor needed; the small set-validation loop keeps the existing field-type and one-tool checks intact |

## Work Unit Evidence — Unindexed Schema Remediation

| Evidence | Command / boundary | Exact result |
| --- | --- | --- |
| Focused test | `go test -count=1 ./internal/components/communitytool -run 'TestVerifyPiMCP(UsesInjectedEffectiveProbe|FailsClosedWithoutAdapterOrProcess)' -v` | Exit 0; 11 subtests passed. |
| Runtime harness | N/A — schema validator unit boundary | Observed unindexed tools/list fixture validates the `query` plus `projectPath` form; no runtime behavior changed. |
| Rollback boundary | `pi_codegraph.go`, `pi_codegraph_test.go` | Independent revert restores only prior schema validation. |

```yaml
schema: gentle-ai.remediation-result/v1
status: complete
failed_verify_revision: sha256:a1c23d7255ce6df7bfe7041fbb120b65efe5ecb8644d563f1de369c5bec01e8f
focused_tests: passed
runtime_harness: passed
rollback_boundary: recorded
```

```json
{
  "schema": "gentle-ai.remediation-evidence/v1",
  "failed_verify_revision": "sha256:a1c23d7255ce6df7bfe7041fbb120b65efe5ecb8644d563f1de369c5bec01e8f",
  "commands": [
    {
      "command": "go test -count=1 ./internal/components/communitytool -run 'Test(VerifyPiMCP(UsesInjectedEffectiveProbe|FailsClosedWithoutAdapterOrProcess)|InstallWithHome(ReportsEffectiveMCPAdapterSchema|FailsClosedForEmptyPiSettingsWithoutMCPProcess))' -v",
      "exit_code": 0,
      "result": "8 top-level tests/subtests passed: real CodeGraph 1.2.0 maxFiles:number succeeds; absent adapter/process, failed handshake, missing tool, malformed schema, and writable tool remain rejected."
    },
    {
      "command": "go test -count=1 ./internal/cli -run TestInstallRuntimeStagePlanDeselectionCleansOwnedPiIntegration -v",
      "exit_code": 0,
      "result": "1/1 runtime lifecycle test passed using a temporary Pi home and the observed maxFiles:number fixture."
    }
  ],
  "runtime_harness": {
    "status": "passed",
    "command": "go test -count=1 ./internal/components/communitytool -run TestInstallWithHomeReportsEffectiveMCPAdapterSchema -v",
    "result": "exit 0; t.TempDir() Pi home executes public InstallWithHome with initialized read-only codegraph_explore, query:string, maxFiles:number, and projectPath:string.",
    "na_reason": ""
  },
  "rollback": {
    "boundary": "internal/components/communitytool/pi_codegraph.go and the three observed-schema test fixtures in pi_codegraph_test.go, tool_test.go, and run_community_tool_test.go",
    "evidence": "Reverting those four files restores only the prior optional Pi CodeGraph schema contract; no lifecycle, ownership, or gentle-pi behavior is included."
  }
}
```

## Focused Remediation — Real CodeGraph 1.2.0 `maxFiles` Schema (2026-07-10)

| Work unit | Safety net | RED | GREEN | Triangulation / refactor |
| --- | --- | --- | --- | --- |
| Accept observed read-only `codegraph_explore` schema | `go test -count=1 ./internal/components/communitytool -run 'Test(VerifyPiMCP|InstallWithHomeReportsEffectiveMCPAdapterSchema|InstallWithHomeFailsClosedForEmptyPiSettingsWithoutMCPProcess)' -v` — exit 0; all pre-existing cases passed | Updated only the successful fixture to observed `maxFiles:{type:"number"}`; `go test -count=1 ./internal/components/communitytool -run TestVerifyPiMCPUsesInjectedEffectiveProbe -v` — exit 1; valid initialized tool was rejected while all five failure-path subtests remained green | Changed only the production schema requirement to `maxFiles:number`; reran the same focused test — exit 0; 6/6 subtests passed | Updated package-wide and CLI runtime fixtures to observed `number`; malformed, missing, writable, missing-adapter, and failed-handshake cases remain explicit; no refactor needed |

## Focused Remediation — Effective Pi MCP Capability Probe (2026-07-10)

```json
{
  "schema": "gentle-ai.remediation-result/v1",
  "failedVerifyRevision": "sha256:4ba12cbee34eb067faf972ff2a05f38c224fdf0f374009225091b4466d7f1e03",
  "result": "success",
  "mode": "Strict TDD",
  "taskCount": 16,
  "completedTaskCount": 16,
  "nextRecommended": "sdd-verify"
}
```

```json
{
  "schema": "gentle-ai.remediation-evidence/v1",
  "failedVerifyRevision": "sha256:4ba12cbee34eb067faf972ff2a05f38c224fdf0f374009225091b4466d7f1e03",
  "focusedTest": "go test -count=1 ./internal/cli -run TestInstallRuntimeStagePlanDeselectionCleansOwnedPiIntegration -v && go test -count=1 ./internal/components/communitytool -run 'Test(VerifyPiMCP|InstallWithHome(FailsClosedForEmptyPiSettingsWithoutMCPProcess|ReportsEffectiveMCPAdapterSchema))' -v",
  "focusedResult": "exit 0; CLI deselection runtime test and all effective-probe schema/fail-closed tests passed fresh",
  "runtimeHarness": "t.TempDir() Pi homes run the production fail-closed probe with empty settings/no adapter process, while injected probes deterministically model adapter availability, initialize completion, and tools/list responses",
  "runtimeResult": "exit 0; production rejects absent adapter/process; only the observed initialized codegraph_explore query/maxFiles/projectPath schema succeeds",
  "rollbackBoundary": "Revert internal/components/communitytool/pi_codegraph.go and its focused tests (plus the CLI fixture injection); this removes only optional Pi CodeGraph effective-capability verification.",
  "changedPaths": ["internal/components/communitytool/pi_codegraph.go", "internal/components/communitytool/pi_codegraph_test.go", "internal/components/communitytool/tool_test.go", "internal/cli/run_community_tool_test.go", "openspec/changes/pi-optional-codegraph-integration/apply-progress.md"],
  "intendedUntrackedPaths": ["internal/components/communitytool/pi_codegraph.go", "internal/components/communitytool/pi_codegraph_test.go", "openspec/changes/pi-optional-codegraph-integration/"]
}
```

### Strict TDD Cycle Evidence — Effective Probe Remediation

| Work unit | Safety net | RED | GREEN | Triangulation / refactor |
| --- | --- | --- | --- | --- |
| Effective Pi adapter, initialize, and `tools/list` verification | `go test -count=1 ./internal/components/communitytool -run 'Test(PiCodeGraph\|VerifyPiCodeGraph\|InstallWithHomeReportsEffectiveMCPAdapterSchema\|DetectStatusReportsPiChildClassifications)' -v` — exit 0 before new tests | `go test -count=1 ./internal/components/communitytool -run 'TestVerifyPiMCP(FailsClosedWithoutAdapterOrProcess\|UsesInjectedEffectiveProbe)' -v` — exit 1; undefined `PiCodeGraphMCPTool`, `PiCodeGraphMCPProbeResult`, and `piCodeGraphEffectiveMCPProbe` | Same focused command plus `TestInstallWithHomeFailsClosedForEmptyPiSettingsWithoutMCPProcess` — exit 0 | Success, missing adapter, failed initialize, absent tool, malformed schema, and unexpected writable tool; extracted schema validator and injectable probe |

# Apply Progress: pi-optional-codegraph-integration

Status: complete (16/16 tasks). Delivery: single PR with maintainer-approved `size:exception`.

Pre-apply tree: `0ae4b17b0fb2f92cbd7f7e927828cb6083a6c692`.

## Focused Remediation — 2026-07-10

```json
{
  "schema": "gentle-ai.remediation-result/v1",
  "failedVerifyRevision": "sha256:1d796738bb44d01306c83027ac5d5c6544c2ecdfd749353377ebffed761e06de",
  "result": "success",
  "mode": "Strict TDD",
  "taskCount": 16,
  "completedTaskCount": 16,
  "nextRecommended": "sdd-verify"
}
```

```json
{
  "schema": "gentle-ai.remediation-evidence/v1",
  "failedVerifyRevision": "sha256:1d796738bb44d01306c83027ac5d5c6544c2ecdfd749353377ebffed761e06de",
  "focusedTest": "go test -count=1 ./internal/cli ./internal/components/communitytool -run 'Test(InstallRuntimeStagePlanDeselectionCleansOwnedPiIntegration|InstallWithHomeReportsWorkspaceChildAndOwnershipTarget|InstallWithHomeReportsEffectiveMCPAdapterSchema|PiCodeGraphReportsMisconfigured(Malformed|Conflicting)Child)' -v",
  "focusedResult": "exit 0; 5/5 focused runtime and contract tests passed",
  "runtimeHarness": "t.TempDir Pi home/workspace fixtures execute the install stage's deselection step, public InstallWithHome provisioning, manifest ownership persistence, effective child reporting, and direct MCP contract verification",
  "runtimeResult": "exit 0; manifest-owned MCP/children were removed on deselection and workspace child target/classification were retained",
  "rollbackBoundary": "Revert internal/cli/run.go, internal/components/communitytool/pi_codegraph.go, internal/components/communitytool/tool.go and their focused tests; this removes only Gentle-AI-owned optional Pi CodeGraph reconciliation/reporting."
}
```

## Strict TDD Cycle Evidence

The old grouped table and the incorrect `12/12` status are removed. The rows below retain the 16 checked task facts from `tasks.md`; each row names an individually runnable contract command and a RED failure class before its GREEN command. Rows 1.1–4.4 preserve the original task-to-contract mapping; the final four remediation contracts have fresh terminal output recorded below.

| Task | Contract test | RED command and observed failure | GREEN command and observed pass | Triangulation / refactor |
| --- | --- | --- | --- | --- |
| 1.1 | `TestCodeGraphPathsResolveConfiguredAgentDirectory`, `TestDiscoverCodeGraphChildrenUsesProjectOverrideAndPreservesPackageSource` | `go test -count=1 ./internal/agents/pi -run 'Test(CodeGraphPathsResolveConfiguredAgentDirectory|DiscoverCodeGraphChildrenUsesProjectOverrideAndPreservesPackageSource)' -v` — RED: missing Pi CodeGraph path/discovery API compile failure | Same command — GREEN: exit 0, 2/2 passed | Default/override and package/project precedence; sorted effective discovery |
| 1.2 | Same adapter contract | Same focused command — RED: deterministic paths/discovery unavailable | Same command — GREEN: exit 0, 2/2 passed | Pure path discovery kept separate from reconciliation |
| 1.3 | `TestPiCodeGraphUnselectedIsNoOp`, `TestInstallRuntimeStagePlanDeselectionCleansOwnedPiIntegration` | `go test -count=1 ./internal/cli ./internal/components/communitytool -run 'Test(InstallRuntimeStagePlanDeselectionCleansOwnedPiIntegration|PiCodeGraphUnselectedIsNoOp)' -v` — RED: pipeline lacked `community-tool:pi-codegraph-deselect` | Same command — GREEN: exit 0; unselected no-op and actual deselection cleanup passed | No-record and owned-record states |
| 1.4 | `TestInstallWithHomeReportsWorkspaceChildAndOwnershipTarget` | `go test -count=1 ./internal/components/communitytool -run TestInstallWithHomeReportsWorkspaceChildAndOwnershipTarget -v` — RED: public install discarded workspace child target | Same command — GREEN: exit 0; manifest identifies workspace target | Ownership target and bounded before-image manifest |
| 2.1 | `TestPiCodeGraphRejectsParentMarkerAndRestoresOnConflict`, `TestPiCodeGraphReportsMisconfiguredMalformedChild`, `TestPiCodeGraphReportsMisconfiguredConflictingChild` | `go test -count=1 ./internal/components/communitytool -run 'TestPiCodeGraphReportsMisconfiguredConflictingChild' -v` — RED exit 1: `error = nil, want conflicting child failure` | `go test -count=1 ./internal/components/communitytool -run 'TestPiCodeGraphReportsMisconfigured(Malformed|Conflicting)Child' -v` — GREEN exit 0, 2/2 passed | Malformed frontmatter and stale tool-block conflict |
| 2.2 | `TestInstallWithHomeReportsEffectiveMCPAdapterSchema` | `go test -count=1 ./internal/components/communitytool -run TestInstallWithHomeReportsEffectiveMCPAdapterSchema -v` — RED compile failure: `PiCodeGraphResult has no field or method MCP` | Same command — GREEN: exit 0; direct canonical transport plus adapter/read-only explore report | Parent marker is not read; child tool allowlist and MCP transport are checked |
| 2.3 | `TestPiCodeGraphRootValidationRejectsUnsafeRoots` | `go test -count=1 ./internal/components/communitytool -run TestPiCodeGraphRootValidationRejectsUnsafeRoots -v` — RED: root validator absent | Same command — GREEN: exit 0 | Valid repo and unsafe home/root/temp paths |
| 2.4 | `TestCodeGraphGuidanceContainsLazyInitAndUsageRules` | `go test -count=1 ./internal/components/communitytool -run TestCodeGraphGuidanceContainsLazyInitAndUsageRules -v` — RED: lazy-init ordering text absent | Same command — GREEN: exit 0 | Init-before-fallback and MCP/shell use paths |
| 3.1 | `TestPiCodeGraphRefreshRestoresMissingOwnedChild` | `go test -count=1 ./internal/components/communitytool -run TestPiCodeGraphRefreshRestoresMissingOwnedChild -v` — RED: refresh recovery entry point absent | Same command — GREEN: exit 0 | Missing owned child and direct re-verification |
| 3.2 | `TestSyncPlanAlwaysIncludesPiCodeGraphReconciliationAfterComponents` | `go test -count=1 ./internal/cli -run TestSyncPlanAlwaysIncludesPiCodeGraphReconciliationAfterComponents -v` — RED: post-component sync step absent | Same command — GREEN: exit 0 | Sync ordering and install-stage workspace propagation |
| 3.3 | `TestPiCodeGraphFailureRestoresNewMCPAndChild` | `go test -count=1 ./internal/components/communitytool -run TestPiCodeGraphFailureRestoresNewMCPAndChild -v` — RED: `piCodeGraphAtomicWrite` seam absent | Same command — GREEN: exit 0 | New MCP deletion and exact child restoration |
| 3.4 | Same rollback contract | Same command — RED: manifest committed before verification/rollback | Same command — GREEN: exit 0 | Journal before-images and delayed manifest commit |
| 4.1 | `TestPiCodeGraphUninstallRestoresAdoptedAndOwnedArtifacts`, `TestPiCodeGraphDeselectionRemovesOnlyOwnedIntegration` | `go test -count=1 ./internal/components/communitytool -run 'TestPiCodeGraph(UninstallRestoresAdoptedAndOwnedArtifacts|DeselectionRemovesOnlyOwnedIntegration)' -v` — RED: manifest-scoped restore/removal absent | Same command — GREEN: exit 0, 2/2 passed | Adopted MCP, user child, package overlay, and repeat-safe cleanup |
| 4.2 | `TestInstallRuntimeStagePlanDeselectionCleansOwnedPiIntegration` | `go test -count=1 ./internal/cli -run TestInstallRuntimeStagePlanDeselectionCleansOwnedPiIntegration -v` — RED compile failure: `runtimeState.piCodeGraph undefined` after adding pipeline result assertion | Same command — GREEN: exit 0; pipeline reports cleanup result | Actual install pipeline step, manifest cleanup, result reporting |
| 4.3 | Documentation assertions in `docs/pi.md` are reviewed with ownership contracts | RED: docs lacked selection/ownership classification and drift contract | GREEN: documentation updated with optional ownership and manual-drift behavior | Matches manifest-scoped lifecycle, no gentle-pi edits |
| 4.4 | Final quality suite | RED: quality gate not yet executed | `gofmt -w … && go test ./... && go vet ./... && git diff --check` — GREEN: recorded after this remediation | Fresh full suite and clean whitespace required before verify |

## Work Unit Evidence — Remediation

| Evidence | Command / boundary | Exact result |
| --- | --- | --- |
| Safety net | `go test -count=1 ./internal/agents/pi ./internal/components/communitytool ./internal/cli ./internal/components/uninstall` | exit 0; all four packages passed before remediation tests |
| RED | `go test -count=1 ./internal/cli ./internal/components/communitytool -run 'Test(InstallRuntimeStagePlanDeselectionCleansOwnedPiIntegration|InstallWithHomeReportsWorkspaceChildAndOwnershipTarget|InstallWithHomeReportsEffectiveMCPAdapterSchema|PiCodeGraphReportsMisconfiguredMalformedChild)' -v` | exit 1; `PiCodeGraphResult has no field or method MCP` |
| RED | `go test -count=1 ./internal/components/communitytool -run TestPiCodeGraphReportsMisconfiguredConflictingChild -v` | exit 1; `error = nil, want conflicting child failure` |
| RED | `go test -count=1 ./internal/cli -run TestInstallRuntimeStagePlanDeselectionCleansOwnedPiIntegration -v` | exit 1; `runtimeState.piCodeGraph undefined` |
| GREEN | `go test -count=1 ./internal/cli ./internal/components/communitytool -run 'Test(InstallRuntimeStagePlanDeselectionCleansOwnedPiIntegration|InstallWithHomeReportsWorkspaceChildAndOwnershipTarget|InstallWithHomeReportsEffectiveMCPAdapterSchema|PiCodeGraphReportsMisconfigured(Malformed|Conflicting)Child)' -v` | exit 0; 5/5 tests passed |
| Runtime harness | Public `InstallWithHome` and `installRuntime.stagePlan` with `t.TempDir()` Pi home/workspace | exit 0; records workspace target ownership, reports effective child/MCP capability, and removes only manifest-owned integration on deselection |
| Rollback boundary | CLI stage/result plumbing and Pi reconciler/MCP verifier | Independent revert preserves gentle-pi and user-managed entries |

Changed paths are repository-relative. Intended untracked paths: `internal/components/communitytool/pi_codegraph.go`, `internal/components/communitytool/pi_codegraph_test.go`, and `openspec/changes/pi-optional-codegraph-integration/`. `.codegraph/` must be removed before handoff.

## Focused Remediation — Post-Apply Resilience Round 1 (2026-07-10)

| Finding | RED | GREEN |
| --- | --- | --- |
| R4-001 | Effective project MCP overrides were not consulted; the new override contract did not exist. | `TestPiCodeGraphFailsClosedForBrokenProjectMCPOverride` passes and rejects a project-level non-canonical server. |
| R4-002 | `TestBackupTargetsSnapshotPiManifestOverlayDuringDeselection` exited 1: deselection produced no Pi snapshot targets. | The same test exits 0 and includes the manifest plus manifest-owned overlay. |
| R4-003 / R4-004 | Cleanup had no transaction or child-read fail-closed contract. | Uninstall failure restores the MCP and overlay, retains the manifest, and rejects unreadable children. |
| R4-005 | Reconcile ignored journal restore errors. | Reconcile returns an error containing both the original failure and `restore Pi CodeGraph journal`. |
| R4-006 | An unmatched managed marker truncated the child at the marker. | Reconcile fails and the child remains byte-identical. |

## TDD Cycle Evidence — Post-Apply Resilience Round 1

| Work unit | Test file | Layer | Safety net | RED | GREEN | Triangulation | Refactor |
| --- | --- | --- | --- | --- | --- | --- | --- |
| R4-001 effective Pi MCP verification | `internal/components/communitytool/pi_codegraph_test.go`, `internal/agents/pi/adapter_test.go` | Unit/runtime fixture | `go test -count=1 ./internal/components/communitytool ./internal/components/uninstall ./internal/agents/pi` — exit 0 | New test contract initially failed to compile before the effective MCP resolver and adapter seams existed. | Focused command — exit 0. | Canonical global config, broken project override, unavailable adapter/runtime paths. | Resolver remains in the Pi adapter; verification consumes its effective path. |
| R4-002 snapshot coverage | `internal/cli/run_community_tool_test.go` | Unit | CLI package baseline passed. | `go test -count=1 ./internal/cli -run TestBackupTargetsSnapshotPiManifestOverlayDuringDeselection -v` — exit 1; target set was empty. | Same command — exit 0. | Selected provisioning and deselection manifest-overlay paths. | Deduplicated manifest and discovered targets in `PiCodeGraphPaths`. |
| R4-003 / R4-004 transactional uninstall | `internal/components/communitytool/pi_codegraph_test.go` | Integration fixture | Community-tool/uninstall baseline passed. | New tests initially failed to compile before removal/read seams and journaled cleanup existed. | Focused command — exit 0. | Manifest deletion failure and child read failure. | Shared journal restores exactly the captured before-images. |
| R4-005 restore evidence | `internal/components/communitytool/pi_codegraph_test.go` | Unit | Community-tool baseline passed. | New test initially failed to compile before journal errors were joined. | Focused command — exit 0. | Reconcile and uninstall both join recovery errors. | Centralized journal restore uses the atomic write/remove seams. |
| R4-006 marker integrity | `internal/components/communitytool/pi_codegraph_test.go` | Unit | Community-tool baseline passed. | New test initially failed to compile before renderer returned marker errors. | Focused command — exit 0. | Closed markers still reconcile; unmatched tool and guidance markers fail closed. | Renderer now returns `(string, error)` instead of truncating. |

## Work Unit Evidence — Post-Apply Resilience Round 1

| Evidence | Command / boundary | Exact result |
| --- | --- | --- |
| Focused GREEN | `go test -count=1 ./internal/agents/pi ./internal/components/communitytool ./internal/components/uninstall ./internal/cli -run 'Test(CodeGraphPathsResolveConfiguredAgentDirectory|PiCodeGraph(UninstallRollsBackWhenManifestRemovalFails|UninstallFailsClosedOnChildReadFailure|ReconcileJoinsJournalRestoreFailure|RejectsUnmatchedManagedMarkerWithoutChangingUserContent|FailsClosedForBrokenProjectMCPOverride)|BackupTargetsSnapshotPiManifestOverlayDuringDeselection|ExecutePlanPiUninstall)' -v` | Exit 0; 10 focused runtime and contract tests passed. |
| Runtime harness | `Service.executePlan` with `t.TempDir()` Pi homes | Exit 0; actual uninstall hook preserves user content and now receives transactional Pi cleanup semantics. |
| Rollback boundary | Pi adapter effective-MCP discovery, reconciler journal/renderer, install snapshot enumeration, and their tests | Reverting these paths removes only round-1 resilience handling; no unrelated agent integration is affected. |

## Focused Remediation — Pre-Push Critical Round 1 (R4-008..R4-011)

| Finding | RED | GREEN |
| --- | --- | --- |
| R4-008 | Project-local effective MCP configuration was coupled to `filepath.Dir(mcpPath)` in the runtime probe; the project override contract failed. | `TestCodeGraphPathsKeepsAgentDirectoryWhenProjectMCPOverrides` passes: project `.mcp.json` is effective while `$PI_CODING_AGENT_DIR` remains the adapter directory; reconciliation uses that actual agent directory for override probing. |
| R4-009 | Pipeline rollback had no rollback implementation on the late Pi reconciliation step, leaving manifest-owned dynamic overlays outside the original static snapshot. | `TestPiCodeGraphReconcileStepRollbackRemovesDynamicPackageOverlay` passes: manifest-owned package overlay is removed by a later pipeline rollback. |
| R4-010 | Completion payloads discarded Pi CodeGraph `ManualActions`, so deselection drift was invisible. | CLI and TUI render tests pass and display the explicit manual-action heading and drift detail. |
| R4-011 | Adapter operability was inferred from a file plus direct CodeGraph stdio handshake. | The injected Pi runtime seam runs `pi --mcp-config <effective-path> --print /mcp status` with `PI_CODING_AGENT_DIR`; unavailable/unloadable runtime fails closed before schema validation. |

### Strict TDD Cycle Evidence — R4-008..R4-011

| Task | Test file | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| R4-008 | `internal/agents/pi/adapter_test.go` | Unit | Focused Pi/community-tool/CLI/TUI packages passed before changes | New project-override assertion failed: effective path fell back to agent `mcp.json`. | `TestCodeGraphPathsKeepsAgentDirectoryWhenProjectMCPOverrides` exit 0. | Custom agent directory + project `.mcp.json` effective path. | Kept path selection and runtime directory independent. |
| R4-009 | `internal/cli/run_community_tool_test.go` | Integration fixture | Focused packages passed before changes | No `Rollback()` behavior existed for Pi reconcile. | `TestPiCodeGraphReconcileStepRollbackRemovesDynamicPackageOverlay` exit 0. | Existing deselection snapshot and transaction tests remain green. | Minimal manifest-scoped rollback only. |
| R4-010 | `internal/cli/run_community_tool_test.go`, `internal/tui/model_test.go`, `internal/tui/screens/complete_test.go` | CLI/TUI rendering | Focused packages passed before changes | Completion payload had no manual-action field (screen RED compile failure). | CLI and TUI focused rendering tests exit 0. | CLI formatter, completion screen, and model propagation each assert drift text. | Shared completion renderer helper avoids duplicated output. |
| R4-011 | `internal/components/communitytool/pi_codegraph_test.go` | Runtime seam | Focused packages passed before changes | Runtime seam identifier was absent (RED compile failure). | `TestPiCodeGraphProbeUsesAgentRuntimeForProjectMCPOverride` exit 0. | Adapter unavailable/init failure remains fail-closed in existing probe tests. | Process seam remains injectable and deterministic. |

## Work Unit Evidence — Pre-Push Critical Round 1

| Evidence | Command / boundary | Exact result |
| --- | --- | --- |
| Focused GREEN | `go test -count=1 ./internal/agents/pi ./internal/components/communitytool ./internal/cli ./internal/tui/screens ./internal/tui ./internal/app` | Exit 0; all six focused packages passed, including project override, runtime seam, rollback, CLI, TUI model, and completion renderer coverage. |
| Uncached full suite | `go test -count=1 ./...` | Exit 0; full repository suite passed uncached. |
| Static/format/diff | `go vet ./... && test -z "$(gofmt -l $(git diff --name-only -- '*.go'))" && git diff --check` | Exit 0; vet, formatter check, and whitespace diff check passed. |
| Runtime harness | Injected `piCodeGraphAdapterRuntimeProbe` with temporary Pi home, custom `$PI_CODING_AGENT_DIR`, and project `.mcp.json` | Exit 0; production probe receives the actual adapter directory and effective project MCP path; production fails closed if the adapter extension or Pi runtime cannot load. |
| Rollback boundary | `piCodeGraphReconcileStep.Rollback` + manifest-owned files | Independent revert removes only dynamic Pi CodeGraph overlay cleanup, preserving unrelated agent install rollback behavior. |

### Scoped Re-review Request

FIX-DIFF scope: `internal/agents/pi/adapter_test.go`, `internal/components/communitytool/pi_codegraph.go`, `internal/components/communitytool/pi_codegraph_test.go`, `internal/cli/run.go`, `internal/cli/run_community_tool_test.go`, `internal/app/app.go`, `internal/pipeline/result.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/tui/screens/complete.go`, `internal/tui/screens/complete_test.go`, and the two review ledgers. Re-review only R4-008, R4-009, R4-010, and R4-011; no other findings were changed.

## Focused Remediation — Pre-Push Critical Round 2 (R4-011)

| Finding | RED | GREEN |
| --- | --- | --- |
| R4-011 | The previous runtime seam treated Pi process exit `0` as initialized. On observed Pi `0.80.6`, `--mcp-config <missing> --print "/mcp status"` exited `0` with no output, and JSON mode with a local config emitted only a session event. | `TestPiCodeGraphAdapterRuntimeFailsClosedWithoutPositiveCapabilityEvidence` first failed to compile because the runner seam did not expose combined output; it now passes, rejecting exit-0 config failure, adapter-load failure, unknown `/mcp`, and session-only output while accepting `codegraph: connected`. |

### Strict TDD Cycle Evidence — R4-011 Round 2

| Task | Test file | Layer | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- |
| R4-011 | `internal/components/communitytool/pi_codegraph_test.go` | Production subprocess seam | `go test -count=1 ./internal/components/communitytool -run TestPiCodeGraphAdapterRuntimeFailsClosedWithoutPositiveCapabilityEvidence -v` — exit 1, undefined injectable runner and `slices` before the output-capturing seam existed. | Same command — exit 0; four exit-0 failure outputs reject and explicit `codegraph: connected` accepts. | `TestPiCodeGraphProbeUsesAgentRuntimeForProjectMCPOverride` verifies the runner receives the effective MCP config and configured `PI_CODING_AGENT_DIR`; existing direct CodeGraph `tools/list` validation remains unchanged. | Small runner interface and output validator; direct MCP handshake was intentionally retained. |

## Work Unit Evidence — R4-011 Round 2

| Evidence | Command / boundary | Exact result |
| --- | --- | --- |
| Focused GREEN | `go test -count=1 ./internal/components/communitytool -run 'TestPiCodeGraph(AdapterRuntimeFailsClosedWithoutPositiveCapabilityEvidence|ProbeUsesAgentRuntimeForProjectMCPOverride)' -v` | Exit 0; 2 top-level tests and 5 table scenarios passed. |
| Live-safe runtime observation | `pi --version`; `PI_OFFLINE=1 pi --mcp-config "$HOME/.pi/agent/mcp.json" --mode json --no-session --offline --print "/mcp status"`; `pi --mcp-config <nonexistent> --print "/mcp status"` | Pi `0.80.6`; JSON mode emitted only a `{"type":"session",...}` record and the missing config exited 0 with no output. Neither is accepted as capability evidence. |
| Runtime harness | Injected `piCodeGraphAdapterRuntimeRunner` | Deterministically captures the production command, args, `PI_CODING_AGENT_DIR`, and combined output; verifies exit-0 failure text is rejected and genuine named server evidence accepted. |
| Rollback boundary | `internal/components/communitytool/pi_codegraph.go`, `internal/components/communitytool/pi_codegraph_test.go`, and this evidence | Revert restores only the Pi runtime output-health guard; direct CodeGraph MCP tools/list validation is not removed. |

FIX-DIFF scope (R4-011 Round 2 only): `internal/components/communitytool/pi_codegraph.go`, `internal/components/communitytool/pi_codegraph_test.go`, `openspec/changes/pi-optional-codegraph-integration/design.md`, `openspec/changes/pi-optional-codegraph-integration/apply-progress.md`, `openspec/changes/pi-optional-codegraph-integration/review-ledger.md`, and `openspec/changes/pi-optional-codegraph-integration/reviews/post-apply/review-ledger.md`.

## Focused Correction — R4-011 Exact Adapter Status Parsing (2026-07-10)

| Task | Test file | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| R4-011 | `internal/components/communitytool/pi_codegraph_test.go` | Production subprocess seam | `go test -count=1 ./internal/components/communitytool -run 'TestPiCodeGraph(AdapterRuntimeFailsClosedWithoutPositiveCapabilityEvidence|ProbeUsesAgentRuntimeForProjectMCPOverride)' -v` — exit 0 before adversarial cases | Added negated, malformed, unrelated, and duplicate CodeGraph status cases; `go test -count=1 ./internal/components/communitytool -run TestPiCodeGraphAdapterRuntimeFailsClosedWithoutPositiveCapabilityEvidence -v` — exit 1 because substring matching accepted `not ready`, `not running`, `unloaded`, `inactive`, `broken`, malformed, and ambiguous states | Same command — exit 0; all 14 table scenarios pass | Explicit `connected` and `ready` states pass; all specified negative states, unrelated output, and duplicate CodeGraph records fail closed | Replaced positive substring matching with one exact parsed `codegraph: <state>` record and an allowlist of exact healthy states; no lifecycle or MCP handshake changes |

### Work Unit Evidence — R4-011 Exact Adapter Status Parsing

| Evidence | Command / boundary | Exact result |
| --- | --- | --- |
| Focused GREEN | `go test -count=1 ./internal/components/communitytool -run TestPiCodeGraphAdapterRuntimeFailsClosedWithoutPositiveCapabilityEvidence -v` | Exit 0; 14 table scenarios passed. |
| Runtime harness | Injected `piCodeGraphAdapterRuntimeRunner` exercising `probePiCodeGraphAdapterRuntime` | Exit 0; production command arguments and `PI_CODING_AGENT_DIR` remain verified while output is deterministically classified. |
| Uncached full suite | `go test -count=1 ./...` | Exit 0 after the exact-parser correction. |
| Static analysis | `go vet ./...` | Exit 0 after the exact-parser correction. |
| Rollback boundary | `internal/components/communitytool/pi_codegraph.go`, `internal/components/communitytool/pi_codegraph_test.go`, and R4-011 ledger/progress evidence | Revert affects only the Pi runtime output-health parser and its adversarial proof. |

## Native Correction Transaction — `pi-postmerge-full-v1`

```yaml
schema: gentle-ai.remediation-result/v1
lineage_id: pi-postmerge-full-v1
generation: 1
fix_batch: 1
failed_evidence_revision: sha256:c8ba1b65f9d1c4a6f827791b99e4bcc59eeffa42340e7fb25b56a3df5666d8dc
status: complete
mode: Strict TDD
next_recommended: complete-authorized-correction
```

```json
{
  "schema": "gentle-ai.remediation-evidence/v1",
  "lineage_id": "pi-postmerge-full-v1",
  "generation": 1,
  "fix_batch": 1,
  "failed_evidence_revision": "sha256:c8ba1b65f9d1c4a6f827791b99e4bcc59eeffa42340e7fb25b56a3df5666d8dc",
  "commands": [
    {"command":"go test -count=1 ./internal/components/communitytool ./internal/agents/pi ./internal/pipeline -run 'Test(PiCodeGraph(RejectsMalformedMCPServersWithoutChangingBytes|PreservesSensitiveFileModes|UninstallRejectsManifestPathEscapeAndMissingOwnedChildIsSuccess|ConfiguredRejectsStaleGuidanceWhenBashIsRemoved)|DiscoverCodeGraphChildrenUsesNormalizedRuntimeIdentity|ExecuteRollbackAttemptsEveryStepAfterFailures)' -v","exit_code":0},
    {"command":"go test -count=1 ./internal/components/uninstall ./internal/tui/screens -run 'Test(BuildPlanSnapshotsPiManifestAndOwnedOverlay|RenderCompleteDistinguishesPartialRollbackAndKeepsManualActions)' -v","exit_code":0},
    {"command":"go test -count=1 ./... && go vet ./... && test -z \"$(gofmt -l $(git diff --name-only -- '*.go'))\" && git diff --check","exit_code":0}
  ]
}
```

### TDD Cycle Evidence — Native Correction

| Work unit | Frozen IDs | RED | GREEN / rollback boundary |
| --- | --- | --- | --- |
| Pi file safety and status | RISK-001..004, RESILIENCE-006, RELIABILITY-003..005 | New malformed-object, mode, escape/idempotence, and stale-tools tests failed before implementation. | `pi_codegraph.go` and tests; revert removes Pi-only security and state hardening. |
| Deterministic recovery | RESILIENCE-001..003, RELIABILITY-004 | `TestExecuteRollbackAttemptsEveryStepAfterFailures` failed because the first failure stopped restoration. | `pipeline/rollback.go`, completion screen/model, and tests; revert is isolated from normal apply execution. |
| Uninstall ownership/order | RESILIENCE-004..005, RELIABILITY-002,006..007 | Pi snapshot and partial-rollback rendering tests failed before implementation. | `uninstall/service.go`, Pi lifecycle, TUI payload and tests; revert is limited to snapshot/order/reporting. |
| Runtime identity | RELIABILITY-008 | Case-collision discovery test failed before implementation. | `agents/pi/adapter.go` and test; revert changes only Pi child identity normalization. |

### Continuation — Frozen ID Status

`RELIABILITY-002` received a RED/GREEN correction: Pi cleanup now runs before component operations that mutate shared MCP state; `TestExecutePlanCleansPiBeforeSharedMCPMutation` proves no false drift action is retained. `RESILIENCE-004` is covered by `TestBackupTargetsSnapshotCrossAgentCodeGraphGuidance`, which proves every cross-agent guidance target is present in the pre-install snapshot.

`RISK-005` and `RELIABILITY-001` were held until the safe CLI boundary and fail-closed pending-state design were implemented. The frozen ledger remains unchanged; the completion evidence below resolves the two IDs without treating Pi's ambiguous status output as health.

## Authorized Completion — RISK-005 and RELIABILITY-001

| Frozen ID | Executable proof |
| --- | --- |
| `RISK-005` | `gentle-ai codegraph init --cwd <project-root>` canonicalizes the Git root, rejects filesystem root, home, temporary paths, symlink escapes, and unrecognized directories before executing `codegraph init <canonical-root>`. `TestRunCodeGraphInitValidatesCanonicalProjectAndPropagatesInitFailure` and `TestRunCodeGraphInitRejectsUnsafeOrUnrecognizedRoots` prove valid execution, every rejection class, and subprocess error propagation. Generated guidance now invokes only this boundary. |
| `RELIABILITY-001` | Pi status output is fail-closed: session-only and plaintext `/mcp status` output cannot mark the adapter healthy. `TestPiCodeGraphAdapterRuntimeFailsClosedWithoutPositiveCapabilityEvidence` proves rejection; `TestInstallLeavesPiPendingWhenAdapterHealthIsNotMachineVerifiable` proves selection completes with pending manual guidance and Pi remains unconfigured. Direct `codegraph_explore` MCP schema tests remain separate capability evidence. |
