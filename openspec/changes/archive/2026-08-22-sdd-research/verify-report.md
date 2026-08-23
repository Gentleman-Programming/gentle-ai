```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:23e9786b02d602271c5aa92f350953edc90a1f7ffb5e3add0ba5b2e344b1a6f3
verdict: pass
blockers: 0
critical_findings: 0
requirements: 6/6
scenarios: 13/13
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:01694edc94682f2095e39acfc28ab1bbb001ac68536b22b168d401310e53e6cf
build_command: go vet ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: `sdd-research`
**Version**: N/A
**Mode**: Strict TDD
**Verdict**: PASS WITH WARNINGS

This is the single corrective re-verification after deterministic remediation. It inspected the current proposal, both OpenSpec delta specs, design, tasks, cumulative apply progress, failed report, and their Engram mirrors. All 6 requirements, 13 scenarios, and 15 tasks are complete; focused corrections, all affected non-mutating goldens, the Kilocode baseline, the scenario matrix, the full uncached module, vet, formatter check, diff hygiene, and coverage now pass.

Native review lineage `review-72bc533ab7d806fe` remains completed, approved, and burned. The verification did not start, reopen, or mutate review. The approved review corrections remain present and green; deterministic remediation revision `sha256:c64b9680e4426dcf053171e2e60d9167e2e5c1580fb6f00c4323fecac384b379` resolves the blockers reported by failed evidence revision `sha256:0dbb6b018dc2c452c6ff5fb5edc623c134f3d63c8326ad4ac08764195d96f62c`.

### Completeness

| Metric | Value |
|---|---:|
| Requirements | 6/6 |
| Scenarios | 13/13 |
| Task checkboxes complete | 15/15 |
| Task objectives currently green | 15/15 |
| OpenSpec/Engram spec scenarios | 13/13 in both stores |

Native status reported `taskProgress=15/15`, `apply=all_done`, and `nextRecommended=verify` because the persisted report was still the failed revision. The orchestrator-held attempt token for `verify-post-baselines` was observed but no attempt-ledger operation was performed.

### Build & Tests Execution

| Check | Command (Markdown-escaped where needed) | Exit | Output SHA-256 | Result |
|---|---|---:|---|---|
| Corrected findings | `go test ./internal/assets ./internal/components/sdd -run '^(TestSDDResearchRuntimeAssetsDeclareExactEvidenceGrants\|TestSelectedResearchReadinessMatrixFailsClosed\|TestHybridRestartRequiresByteEqualStateWithoutStorePreference)$' -count=1` | 0 | `sha256:15675d4f781cc890234ff74cabc7e8043eb56e98341aeffcd02ca8c08759becc` | Exact grants, store-mode readiness, and deterministic recovery passed |
| Affected non-mutating goldens | `go test ./internal/components -run '^(TestGoldenSDD_.*\|TestGoldenCombined_(Claude\|Windsurf))$' -count=1` | 0 | `sha256:50907fd875e78145b7989dd2b3cd13b6a3c6b2d96f103423e0934ac4efb41e74` | All 14 affected top-level golden tests passed without `-update` |
| Golden byte stability | `sha256sum` over the 14 remediation goldens before and after the non-mutating run, then `cmp` the manifests | 0 | Before and after: `sha256:b9936c9ffea09187e5f74c1b59aafc8a319e21a020ccd2410b5626865c3aeb0e` | No golden changed during verification |
| Kilocode baseline | `go test ./internal/components/sdd -run '^TestKilocodeReviewSettingsMatchCurrentMainBaseline$' -count=1` | 0 | `sha256:85846a0a216e9c4226a02419c80c7d8b473e7ef8ee6a441b34b4c7a5dc83e060` | Rendered hash matches `c834b94f527238b68a801a04bcee871d9fe2094a86ea81640bcf72aa68147c7d` |
| All 13 scenario mappings | `go test ./internal/agents/researchcapability ./internal/assets ./internal/components/sdd ./internal/sddstatus -run '^(TestAdmitFailsClosedAndReturnsOnlyVerifiedGrants\|TestDocumentationLikeInputsNeverBecomeGrants\|TestForAgentDeclaresOnlyClaudeAndKiro\|TestSDDResearchRuntimeAssetsDeclareExactEvidenceGrants\|TestSDDOrchestratorAssetsUseCanonicalResearchGate\|TestSDDProposeAssetsRequireConfirmedHandoffWithoutInterview\|TestSharedSDDProposeSkillRequiresConfirmedHandoffWithoutInterview\|TestWindsurfSDDNewWorkflowOrdersPreProposalLifecycle\|TestWindsurfSDDNewWorkflowRejectsLegacySDDArtifacts\|TestResearchEvidenceContractIsCompleteAndFailClosed\|TestSelectedResearchReadinessMatrixFailsClosed\|TestHybridRestartRequiresByteEqualStateWithoutStorePreference\|TestBoundedReviewContractRendersForEverySupportedAgent\|TestOpenCodeResearchCommandHasExplicitTaskPermissionAndDefaultDenial\|TestResolveMissingProposalStillRecommendsPropose\|TestProjectStatusV1FreezesExactLegacyShape\|TestProjectStatusV1RejectsNonLegacyValues)$' -count=1` | 0 | `sha256:f443273493b1a0e3d922de55bdafe756012b160047d6c827ca6aae5d775a9a65` | Four packages and all mapped test functions passed |
| Full uncached root module | `go test ./... -count=1` | 0 | `sha256:01694edc94682f2095e39acfc28ab1bbb001ac68536b22b168d401310e53e6cf` | Every root-module package passed |
| Vet/type check | `go vet ./...` | 0 | `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | No output |
| Formatter check-only | `gofmt -l $(git diff --name-only -- '*.go') internal/agents/researchcapability/*.go internal/components/sdd/research_state_contract_test.go internal/assets/windsurf_workflow_test.go` | 0 | `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | No files listed; no file was modified |
| Diff hygiene | `git diff --check` | 0 | `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | No output |
| Coverage | `go test -coverprofile="/tmp/opencode/sdd-research-corrective.cover" ./... -count=1` | 0 | `sha256:87fc403542c6de7a91ec0fd66e96e5218a6a7bc6c78c354a1a6b2b2aa2feded3` | Coverage profile `sha256:cde7436d2d7642c08d131666cda5fc4051e58ef963ec374af7c1d6d82531d5fa` |

Exact regex-bearing commands as executed:

```text
go test ./internal/assets ./internal/components/sdd -run '^(TestSDDResearchRuntimeAssetsDeclareExactEvidenceGrants|TestSelectedResearchReadinessMatrixFailsClosed|TestHybridRestartRequiresByteEqualStateWithoutStorePreference)$' -count=1
go test ./internal/components -run '^(TestGoldenSDD_.*|TestGoldenCombined_(Claude|Windsurf))$' -count=1
go test ./internal/components/sdd -run '^TestKilocodeReviewSettingsMatchCurrentMainBaseline$' -count=1
go test ./internal/agents/researchcapability ./internal/assets ./internal/components/sdd ./internal/sddstatus -run '^(TestAdmitFailsClosedAndReturnsOnlyVerifiedGrants|TestDocumentationLikeInputsNeverBecomeGrants|TestForAgentDeclaresOnlyClaudeAndKiro|TestSDDResearchRuntimeAssetsDeclareExactEvidenceGrants|TestSDDOrchestratorAssetsUseCanonicalResearchGate|TestSDDProposeAssetsRequireConfirmedHandoffWithoutInterview|TestSharedSDDProposeSkillRequiresConfirmedHandoffWithoutInterview|TestWindsurfSDDNewWorkflowOrdersPreProposalLifecycle|TestWindsurfSDDNewWorkflowRejectsLegacySDDArtifacts|TestResearchEvidenceContractIsCompleteAndFailClosed|TestSelectedResearchReadinessMatrixFailsClosed|TestHybridRestartRequiresByteEqualStateWithoutStorePreference|TestBoundedReviewContractRendersForEverySupportedAgent|TestOpenCodeResearchCommandHasExplicitTaskPermissionAndDefaultDenial|TestResolveMissingProposalStillRecommendsPropose|TestProjectStatusV1FreezesExactLegacyShape|TestProjectStatusV1RejectsNonLegacyValues)$' -count=1
```

The exact evidence preimage is `/tmp/opencode/sdd-research-corrective-evidence.json`; its SHA-256 is the envelope `evidence_revision`.

### Bench and CI Disposition

No driven bench is applicable. The deterministic remediation changed snapshots, one test-only hash, Go formatting, and hybrid SDD artifacts; it changed no `bench/` journey, native route, runtime entrypoint, provider boundary, or journey-pinned product semantic. No `go test ./bench` result is claimed as driven execution proof.

### Spec Compliance Matrix

| Requirement | Scenario | Covering passing test | Result |
|---|---|---|---|
| Closed Capability Admission | Supported request | `TestAdmitFailsClosedAndReturnsOnlyVerifiedGrants` | ✅ COMPLIANT |
| Closed Capability Admission | Denied or unknown capability | `TestAdmitFailsClosedAndReturnsOnlyVerifiedGrants`; `TestDocumentationLikeInputsNeverBecomeGrants`; `TestForAgentDeclaresOnlyClaudeAndKiro` | ✅ COMPLIANT |
| Auditable Evidence Integrity | Complete source-backed result | `TestResearchEvidenceContractIsCompleteAndFailClosed` | ✅ COMPLIANT |
| Auditable Evidence Integrity | Partial or blocked research | `TestResearchEvidenceContractIsCompleteAndFailClosed`; `TestSelectedResearchReadinessMatrixFailsClosed` | ✅ COMPLIANT |
| Hybrid Completion and Recovery | Matching restart | `TestHybridRestartRequiresByteEqualStateWithoutStorePreference` | ✅ COMPLIANT |
| Hybrid Completion and Recovery | Divergent restart | `TestSelectedResearchReadinessMatrixFailsClosed`; `TestHybridRestartRequiresByteEqualStateWithoutStorePreference` | ✅ COMPLIANT |
| Hybrid Completion and Recovery | One-sided hybrid write recovery | `TestHybridRestartRequiresByteEqualStateWithoutStorePreference` | ✅ COMPLIANT |
| Hybrid Completion and Recovery | Missing recovery intent | `TestHybridRestartRequiresByteEqualStateWithoutStorePreference` | ✅ COMPLIANT |
| Orchestrator-Owned Discovery and Confirmed Handoff | Confirmed proposal handoff | `TestSDDOrchestratorAssetsUseCanonicalResearchGate`; both proposer non-interview tests | ✅ COMPLIANT |
| Orchestrator-Owned Discovery and Confirmed Handoff | Automatic unresolved choices | `TestBoundedReviewContractRendersForEverySupportedAgent` | ✅ COMPLIANT |
| Optional-Lane and Status-v1 Compatibility | Legacy change without research | `TestResolveMissingProposalStillRecommendsPropose`; canonical gate test | ✅ COMPLIANT |
| Optional-Lane and Status-v1 Compatibility | Status-v1 remains frozen | `TestProjectStatusV1FreezesExactLegacyShape`; `TestProjectStatusV1RejectsNonLegacyValues` | ✅ COMPLIANT |
| Bounded Review Contract Parity Across Catalog Agents | Catalog-derived parity test renders all agents | `TestBoundedReviewContractRendersForEverySupportedAgent`; canonical source-family test | ✅ COMPLIANT |

**Compliance summary**: 13/13 scenarios have covering tests that passed in the current worktree.

### Correctness (Static Evidence)

| Requirement | Status | Current implementation evidence |
|---|---|---|
| Closed capability admission | ✅ Implemented | Separate closed v1 contract, exact multiset grants, Claude/Kiro declarations, defensive copies, default denial, and no admission-created claims |
| Auditable evidence integrity | ✅ Implemented | Versioned assets require complete source identity, claim mappings, contradictions, uncertainty/freshness, and separate product choices |
| Hybrid completion and recovery | ✅ Implemented | Store-mode-aware readiness, byte-equal hybrid readback, retained-intent fix-forward recovery, and explicit blocked re-entry without invented state |
| Orchestrator-owned discovery | ✅ Implemented | Research collectors are evidence-only; orchestrators own state and product decisions; proposers consume confirmed handoff without interviewing |
| Optional lane and frozen status v1 | ✅ Implemented | Unselected research remains optional; status v1 omits research state and rejects research routing tokens |
| 12-template/16-AgentID parity | ✅ Implemented | One canonical lifecycle renders across 12 source families and every `catalog.AllAgents()` ID without weakening bounded-review clauses |

Review corrections R1-001, R3-001, and R4-002 remain corrected and pass their targeted tests. R4-001 remains refuted; prior advisory follow-ups are not reopened by verification.

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| Sibling `researchcapability` contract | ✅ Yes | Generic capability manifest v1 remains unchanged |
| Exact Claude/Kiro grants and default denial | ✅ Yes | Production declarations and runtime asset tests agree |
| Store-mode-aware consistency and fix-forward recovery | ✅ Yes | Shared lifecycle/persistence assets and readiness tests agree |
| Orchestrator/research/proposer ownership split | ✅ Yes | Collector and persistence authority remain separate |
| Shared 12-family rendering and catalog-derived 16-ID parity | ✅ Yes | Direct parity, all affected goldens, combined goldens, and Kilocode baseline pass |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD evidence reported | ✅ | Cumulative apply progress contains 15 task rows, four work units, and deterministic-remediation evidence |
| All tasks have tests/evidence | ✅ | 15/15 objectives map to existing test files or final verification commands |
| RED confirmed | ✅ | Named RED files exist; historical RED and coverage-gap evidence are preserved, and the failed verify report supplied genuine deterministic RED for remediation |
| GREEN confirmed fresh | ✅ | Focused corrections, 14 non-mutating goldens, Kilocode baseline, 13-scenario mapping, and full uncached module all pass now |
| Triangulation adequate | ✅ | Exact grants/denials, four store modes, both recovery branches, proposal gates, 12 families, 16 IDs, workflow ordering, and frozen status v1 are covered |
| Safety nets valid | ✅ | Apply evidence records pre-edit safety nets; remediation used the approved update path and a non-mutating rerun with byte-stability proof |

**TDD compliance**: 6/6 checks pass.

### Test Layer Distribution

| Layer | Current verification checks | Files | Tool |
|---|---:|---:|---|
| Unit/state projection | 6 | 3 | Go `testing`, `t.TempDir()` |
| Embedded asset/renderer integration | 11 | 6 | Go `testing`, embedded FS, JSON/rendering paths |
| Golden/rendered baseline | 15 | 2 | Go `testing`, exact bytes and SHA-256 |
| E2E/driven bench | 0 | 0 | Not applicable for this scope |
| **Total** | **32** | **11** | |

### Changed File Coverage

The fresh coverage profile passes. `internal/agents/researchcapability/contract.go` is 100% covered. Average full-file statement coverage across the 17 modified production Go files is 84.4%; Go branch coverage is unavailable.

| Changed production file | Statements | Rating |
|---|---:|---|
| `internal/agents/researchcapability/contract.go` | 100.0% | ✅ Excellent |
| `internal/catalog/skills.go` | 100.0% | ✅ Excellent |
| `internal/components/sdd/boundedreview.go` | 93.5% | ⚠️ Acceptable |
| `internal/components/sdd/commands.go` | 100.0% | ✅ Excellent |
| `internal/components/sdd/inject.go` | 81.6% | ⚠️ Acceptable |
| `internal/components/sdd/profiles.go` | 88.9% | ⚠️ Acceptable |
| `internal/model/codex_model.go` | 94.7% | ⚠️ Acceptable |
| `internal/model/kiro_model.go` | 85.0% | ⚠️ Acceptable |
| `internal/model/types.go` | 100.0% | ✅ Excellent |
| `internal/opencode/models.go` | 82.5% | ⚠️ Acceptable |
| `internal/tui/screens/skill_picker.go` | 95.6% | ✅ Excellent |
| `internal/agents/kimi/adapter.go` | 62.5% | ⚠️ Low |
| `internal/components/skills/presets.go` | 78.9% | ⚠️ Low |
| `internal/components/uninstall/service.go` | 74.8% | ⚠️ Low |
| `internal/model/claude_model.go` | 66.7% | ⚠️ Low |
| `internal/tui/screens/claude_model_picker.go` | 53.1% | ⚠️ Low |
| `internal/tui/screens/codex_model_picker.go` | 77.1% | ⚠️ Low |

The low values are whole-file coverage on broad pre-existing files; the additive research inventory paths are directly exercised by focused inventory, picker, profile, uninstall, and full-module tests. They are informational and do not contradict scenario compliance.

### Assertion Quality

Current scenario, baseline, and golden tests invoke production admission, status projection, injection, asset rendering, and persistence-contract reads, then assert concrete outputs. Table and loop inputs are fixed non-empty collections. No tautology, orphan empty assertion, type-only assertion, assertion-free path, ghost loop, smoke-only assertion, CSS/internal-state coupling, or mock-heavy test was found.

**Assertion quality**: ✅ All audited assertions verify real behavior.

### Quality Metrics

**Formatter**: ✅ Check-only command produced no paths.
**Linter**: ➖ No dedicated linter configured.
**Type checker / vet**: ✅ No errors.
**Diff hygiene**: ✅ No whitespace errors.
**Coverage**: ✅ Full coverage command passed; six broad modified files remain below 80% whole-file statement coverage.
**Backend consistency**: ✅ Engram spec now contains the same 6 requirements and 13 scenarios as the two OpenSpec deltas.

### Attempt Settlement Handoff

| Field | Value |
|---|---|
| Attempt token | `sha256:4eed3c845605d40df27cf0a2776c07542242ffeac7fd6c37e6a596b9ea28e294` |
| Work unit | `verify-post-baselines` |
| Outcome | `passed` |
| Evidence revision | `sha256:23e9786b02d602271c5aa92f350953edc90a1f7ffb5e3add0ba5b2e344b1a6f3` |
| Diagnosis | Deterministic remediation is confirmed: focused corrections, 14 non-mutating goldens, Kilocode baseline, 13 scenario mappings, full uncached tests, vet, formatter, diff hygiene, and coverage all pass. |
| Harness disposition | `reused` |
| Cleanup evidence | Golden checksum manifests are byte-identical; formatter and diff checks are empty; verification changed no production code or tests. |
| Process evidence | Review remained closed; Engram spec is synchronized; driven bench is not applicable. |

The verifier did not settle or otherwise operate the attempt ledger; the orchestrator retains settlement responsibility.

### Issues Found

**CRITICAL**: None.
**WARNING**: Six broad modified production files are below 80% whole-file statement coverage; their changed research inventory behavior is directly covered and all required runtime checks pass.
**SUGGESTION**: None.

### Verdict

**PASS WITH WARNINGS**

The prior deterministic blockers are resolved, all current mandatory checks pass, and the strict envelope is archive-ready. Coverage warnings are informational and non-blocking.
