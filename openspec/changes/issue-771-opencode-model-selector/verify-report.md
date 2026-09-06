```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:9f21114784072d0d936565f565b39b6652abdcf6a77ec7ca809ad8b3d88064ba
verdict: fail
blockers: 1
critical_findings: 1
requirements: 5/5
scenarios: 11/11
test_command: "TMPDIR=/private/var/folders/k8/75vmfj855sx43xb1x0wpftww0000gn/T go test ./internal/opencode ./internal/tui/... ./internal/components/sdd ./internal/cli -count=1"
test_exit_code: 1
test_output_hash: sha256:06189f6399d683687da22b5a38115985c158db98a7a1f7a84564665935d097f0
build_command: "TMPDIR=/private/var/folders/k8/75vmfj855sx43xb1x0wpftww0000gn/T go vet ./internal/opencode ./internal/tui/... ./internal/components/sdd ./internal/cli"
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: issue-771-opencode-model-selector
**Version**: N/A
**Mode**: Standard

### Completeness
| Metric | Value |
|--------|-------|
| Requirements total | 5 |
| Requirements compliant | 5 |
| Scenarios total | 11 |
| Scenarios compliant | 11 |
| Tasks total | 13 |
| Tasks complete | 13 |
| Tasks incomplete | 0 |

### Build & Tests Execution
**Build**: ✅ Passed
```text
TMPDIR=/private/var/folders/k8/75vmfj855sx43xb1x0wpftww0000gn/T go vet ./internal/opencode ./internal/tui/... ./internal/components/sdd ./internal/cli
exit 0
output_hash sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

**Tests**: ❌ Broad affected package command failed after the corrected TMPDIR removed the prior local lock error; focused #771 coverage passed.
```text
TMPDIR=/private/var/folders/k8/75vmfj855sx43xb1x0wpftww0000gn/T go test ./internal/opencode ./internal/tui/... ./internal/components/sdd ./internal/cli -count=1
exit 1
output_hash sha256:06189f6399d683687da22b5a38115985c158db98a7a1f7a84564665935d097f0
ok internal/opencode 1.848s; ok internal/tui 2.825s; ok internal/tui/screens 0.259s; internal/tui/styles [no test files]; ok internal/components/sdd 137.811s; FAIL internal/cli 600.915s; panic: test timed out after 10m0s

TMPDIR=/private/var/folders/k8/75vmfj855sx43xb1x0wpftww0000gn/T go test ./internal/opencode -run 'Config|Catalog|URL' -count=1
exit 0
output_hash sha256:5ee46e72f92c52fd75d94aac49244f932460b6662b082299baa7101ef47fa4f8
ok internal/opencode 0.679s

TMPDIR=/private/var/folders/k8/75vmfj855sx43xb1x0wpftww0000gn/T go test ./internal/tui/... -run 'ModelPicker|Configure' -count=1
exit 0
output_hash sha256:346c466b2239d63755c5e095dbed78ff806020baef08b5b3aa1cb3ebaba3693b
ok internal/tui 0.258s; ok internal/tui/screens 0.409s; internal/tui/styles [no test files]

TMPDIR=/private/var/folders/k8/75vmfj855sx43xb1x0wpftww0000gn/T go test ./internal/components/sdd ./internal/cli -run 'TestInjectOpenCodeWritesExistingJSONCConfig|TestRunSyncPreserves(Current|Cleared)OpenCodeAssignmentOverStaleState' -count=1
exit 0
output_hash sha256:6a73b00c380e015860eb51b37330e485814f06c5de678463a702d7b5ea17a7e7
ok internal/components/sdd 1.449s; ok internal/cli 2.726s
```

**Coverage**: ➖ Not available; no coverage command was required or run.

### Spec Compliance Matrix
| Requirement | Scenario | Test / Runtime Evidence | Result |
|-------------|----------|--------------------------|--------|
| Effective OpenCode Config Discovery | JSONC config is effective | `internal/opencode/config_test.go > TestResolveEffectiveConfigReadsJSONCAndAssignments`; focused opencode test passed | ✅ COMPLIANT |
| Effective OpenCode Config Discovery | No config exists | `internal/opencode/config_test.go > TestResolveEffectiveConfigPrecedenceAndDefaultWriteTarget`; focused opencode test passed | ✅ COMPLIANT |
| Configure Models Shows Effective Providers | Custom configured provider appears | `internal/tui/model_test.go > TestConfigureOpenCodeModelsShowsJSONCCustomProviderWithRuntimeProviders`; focused TUI test passed | ✅ COMPLIANT |
| Configure Models Shows Effective Providers | Runtime discovery remains authoritative | `internal/tui/model_test.go > TestConfigureOpenCodeModelsShowsJSONCCustomProviderWithRuntimeProviders`; `internal/opencode/catalog_test.go > TestMergeConfiguredCatalogKeepsRuntimeAuthoritative`; focused tests passed | ✅ COMPLIANT |
| Install and Sync Preserve Assignment Presence | Install writes existing JSONC config | `internal/components/sdd/inject_test.go > TestInjectOpenCodeWritesExistingJSONCConfig`; focused install/sync test passed | ✅ COMPLIANT |
| Install and Sync Preserve Assignment Presence | Current assignment is preserved | `internal/cli/sync_test.go > TestRunSyncPreservesCurrentOpenCodeAssignmentOverStaleState`; focused install/sync test passed | ✅ COMPLIANT |
| Install and Sync Preserve Assignment Presence | Cleared assignment stays cleared | `internal/cli/sync_test.go > TestRunSyncPreservesClearedOpenCodeAssignmentOverStaleState`; focused install/sync test passed | ✅ COMPLIANT |
| LM Studio URL Resolution | Direct URL wins | `internal/opencode/config_test.go > TestResolveEffectiveConfigReadsJSONCAndAssignments`; focused opencode test passed | ✅ COMPLIANT |
| LM Studio URL Resolution | baseURL fallback is used | `internal/opencode/config_test.go > TestResolveEffectiveConfigUsesBaseURLFallback`; focused opencode test passed | ✅ COMPLIANT |
| Scope Exclusions | Excluded files untouched | `git diff --name-only -- internal/assets/opencode/plugins/review-result-artifacts.ts internal/assets/review_plugin_recovery_test.go`; exit 0 with empty output | ✅ COMPLIANT |
| Scope Exclusions | Unrelated issues stay out of scope | Diff keyword scan for `#934`, `#2288`, `#1015`, `tool-call`, `tool call`, `sqlite`, `runtime discovery`, `PR #1280`, `1280`, and `fix/custom-opencode-model-selector`; zero matches in implementation diff | ✅ COMPLIANT |

**Compliance summary**: 11/11 scenarios compliant with focused runtime evidence; settlement remains blocked by the broad affected package command exit code.

### Correctness (Static Evidence)
| Requirement | Status | Notes |
|------------|--------|-------|
| Effective OpenCode Config Discovery | ✅ Implemented | `internal/opencode/config.go` defines `ResolveEffectiveConfig`, `ConfigSnapshot`, JSONC-tolerant parsing through `filemerge.UnmarshalJSONObject`, supported names `opencode.jsonc` then `opencode.json`, and a default write path when no config exists. |
| Configure Models Shows Effective Providers | ✅ Implemented | `internal/tui/model.go` resolves effective config for the picker and `internal/tui/screens/model_picker.go` merges runtime discovery with configured providers via `opencode.MergeConfiguredCatalog`. |
| Install and Sync Preserve Assignment Presence | ✅ Implemented | `internal/components/sdd/inject.go`, `internal/cli/sync.go`, and `internal/cli/run.go` resolve existing `opencode.jsonc`/`opencode.json`; sync overlays persisted assignments only when current config presence is absent. |
| LM Studio URL Resolution | ✅ Implemented | `providerURL` returns direct `url` before `options.baseURL`. |
| Scope Exclusions | ✅ Implemented | Protected review plugin/recovery files have an empty diff; unrelated #934/#2288/#1015 keyword scan found no implementation diff hits. |

### Coherence (Design)
| Decision | Followed? | Notes |
|----------|-----------|-------|
| Resolver/parser boundary in `internal/opencode/config.go` | ✅ Yes | Boundary owns config resolution, provider/model extraction, URL metadata, and assignment presence. |
| Precedence/write target honors existing config | ✅ Yes | Resolver and OpenCode install/sync helpers prefer existing `opencode.jsonc` before `opencode.json` in the target directory and fall back to default path. |
| Runtime merge keeps OpenCode discovery primary | ✅ Yes | `MergeConfiguredCatalog` clones runtime catalog first, keeps runtime duplicate providers/models, and only adds configured-only data or missing URLs. |
| Assignment presence is transient | ✅ Yes | `AssignmentPresence` is in-memory and sync does not add a new persisted state schema. |
| Exclusions remain outside rollback boundary | ✅ Yes | Protected files are unchanged and unrelated issue-scope markers are absent from implementation diff. |

### RDD Evidence
| Flow / Control | Evidence | Result |
|----------------|----------|--------|
| Configure Models JSONC custom provider | Focused TUI test passed with corrected TMPDIR | ✅ |
| Install/inject JSONC target | Focused SDD inject test passed with corrected TMPDIR | ✅ |
| Sync present assignment | Focused CLI sync test passed with corrected TMPDIR | ✅ |
| Sync cleared assignment | Focused CLI sync test passed with corrected TMPDIR | ✅ |
| LM Studio direct URL | Focused opencode config test passed with corrected TMPDIR | ✅ |
| LM Studio fallback URL | Focused opencode config test passed with corrected TMPDIR | ✅ |
| Excluded-file negative control | Protected diff guard produced no output | ✅ |
| Stale PR #1280 branch bytes | Diff keyword scan found no `PR #1280`, `1280`, or stale branch marker hits | ✅ |
| #934/#2288/#1015 scope exclusion | Diff keyword scan found zero unrelated issue markers | ✅ |

### Cleanup Evidence
| Check | Evidence | Result |
|-------|----------|--------|
| Protected excluded files | `git diff --name-only -- internal/assets/opencode/plugins/review-result-artifacts.ts internal/assets/review_plugin_recovery_test.go`; exit 0, empty output, hash `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | ✅ |
| Whitespace | `git diff --check`; exit 0, empty output, hash `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | ✅ |
| Scope keyword guard | Implementation diff scan found zero unrelated markers; diff hash `sha256:25ea0be82d47068e23093c77ee35da5b847607ed2fa150675d856cba7614b738` | ✅ |

### Process Evidence
| Field | Value |
|-------|-------|
| Runtime attempt state | proceed |
| Runtime attempt token | retained by orchestrator: `sha256:9fea5c952cbc7581e26264bded790295a8b35f74f11750b95eb843ca3bc2d417` |
| Narrowed work unit | verify-corrected-env-only |
| Corrected environment | `TMPDIR=/private/var/folders/k8/75vmfj855sx43xb1x0wpftww0000gn/T` |
| Branch | `feat/opencode-model-selector-771` |
| Implementation edits by verify rerun | None |

### Issues Found
**CRITICAL**: Broad affected package command `TMPDIR=/private/var/folders/k8/75vmfj855sx43xb1x0wpftww0000gn/T go test ./internal/opencode ./internal/tui/... ./internal/components/sdd ./internal/cli -count=1` exited 1 because `internal/cli` timed out after 10 minutes. The previously diagnosed `/var` versus `/private/var` lock error did not appear in this rerun's captured summary, but the command still failed and is therefore not settlement-ready.

**WARNING**: `openspec` CLI validation was not rerun separately; the SDD verify-report validator admitted this report before persistence.

**SUGGESTION**: Investigate the remaining `internal/cli` 10-minute timeout outside this narrowed #771 corrected-TMPDIR verification rerun before archive readiness.

### Verdict
FAIL
Focused #771 behavior remains compliant, vet passes, and excluded files are unchanged, but `verification_outcome_for_settle` cannot be `passed` because the required broad affected package command still exits non-zero.
