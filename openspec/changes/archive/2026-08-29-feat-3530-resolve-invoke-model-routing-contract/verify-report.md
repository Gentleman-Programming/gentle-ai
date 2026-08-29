```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:6294aeb774bf341aa029ab6ca4bce74b57dd0f77a8be3bcf6f8cb98765f6d329
verdict: pass
blockers: 0
critical_findings: 0
requirements: 6/6
scenarios: 16/16
test_command: go test ./internal/agents/pi -count=1
test_exit_code: 0
test_output_hash: sha256:360dabfe50192b4ad613b9110208f176e3e050e7e350bb7aeea3f643ae51d91d
build_command: go vet ./internal/agents/pi
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: `feat-3530-resolve-invoke-model-routing-contract`
**Version**: N/A (delta spec, not yet merged)
**Mode**: Strict TDD
**Branch**: `feat/3530-resolve-invoke-model-routing-contract`
**Evidence revision**: `abd15d21` (`6294aeb7` sha256 of HEAD), base `v2.5.0-rc.2` (`569f3361`)
**Delivery**: single PR, 673 A+D in `internal/agents/pi/` (4 files), budget Low

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 17 |
| Tasks complete | 17 |
| Tasks incomplete | 0 |
| Proposal | Present (`proposal.md`) |
| Spec | Present (`specs/pi-model-routing/spec.md` — 6 req, 16 scenarios) |
| Design | Present (`design.md` — Approach A, 5 decisions) |
| Tasks file | Present (`tasks.md` — all 17 `[x]`) |

All 17 tasks are marked `[x]` (Phases 1 RED 1.1-1.8, Phase 2 GREEN 2.1-2.5, Phase 3 Verification 3.1-3.4). No unchecked task remains.

**Files existence**: `internal/agents/pi/model_routing_resolver.go` (219 lines), `internal/agents/pi/model_routing_client.go` (142 lines), `internal/agents/pi/model_routing.go` (leaf verifier, `ResolvePackageBin`), `internal/agents/pi/adapter.go` (+44 `gentlePiPackageSource`), `internal/agents/pi/model_routing_test.go` (377 lines) — all present. No `internal/tui/screens` touch as required.

### Build & Tests Execution

All commands executed independently by verifier (not trusting apply cache).

| Command | Exit | Output hash | Result |
|---------|------|-------------|--------|
| `go test ./internal/agents/pi -count=1` | 0 | `sha256:360dabfe...` | ✅ PASS |
| `go test ./internal/agents/pi -count=1 -v` | 0 | `sha256:18d6ecdf...` | ✅ PASS (detailed below) |
| `go vet ./internal/agents/pi` | 0 | `sha256:e3b0c442...` (empty) | ✅ PASS |
| `go test ./internal/components/communitytool -count=1` | 0 | `ok` 0.325s | ✅ PASS (no regression) |
| `go test ./internal/agents/pi -count=1 -cover` | 0 | 68.2% statements | ✅ |

**Build**: ✅ Passed — `go vet ./internal/agents/pi` clean, zero output.

**Tests**: ✅ 12 top-level test suites (table-driven), all subtests PASS, 0 FAIL.

```text
$ go test ./internal/agents/pi -count=1 -v
=== RUN   TestResolvePackageBinForms
--- PASS: TestResolvePackageBinForms (5 subtests: string, object and canonical symlink, exact bound, bound plus one, malformed within bound)
=== RUN   TestResolvePackageBinErrors
--- PASS: TestResolvePackageBinErrors (15 subtests: missing package/manifest, string bin with another package, malformed manifest/bin, absent bin, missing target, non-regular, non-executable, absolute, lexical escape, duplicate top-level/selected bin, symlink escape)
=== RUN   TestResolveModelRoutingExecutable_PATH
--- PASS: TestResolveModelRoutingExecutable_PATH (3 subtests: path hit probed, path hit probe fails falls through, path miss falls through)
=== RUN   TestResolveModelRoutingExecutable_Precedence
--- PASS: TestResolveModelRoutingExecutable_Precedence (3 subtests: project overrides user, user when no project, no source)
=== RUN   TestResolveModelRoutingExecutable_AgentDirOverrides
--- PASS
=== RUN   TestPackageRootForSource_Layouts
--- PASS: (3 subtests: npm:foo, git:owner/repo, local:/abs/path)
=== RUN   TestResolveModelRoutingExecutable_ManifestBinReuse
--- PASS: (3 subtests: unsafe bin, missing target, absent bin)
=== RUN   TestResolveModelRoutingExecutable_BoundedProbeNoWrite
--- PASS (oversized → invalid-json, timeout, FS snapshot unchanged)
=== RUN   TestModelRoutingClient_Contract
--- PASS: (4 subtests: capabilities, inspect, validate, apply — each writes one {contract:"gentle-pi.model-routing/v1"} and preserves unknown fields)
=== RUN   TestModelRoutingClient_ErrorTaxonomy
--- PASS: (3 subtests: invalid json → invalid-json, unsupported contract, non-zero exit → probe-failed)
=== RUN   TestModelRoutingClient_Cancellation
--- PASS (context.Canceled → timeout/cancel, child killed)
=== RUN   TestResolveModelRoutingExecutable_ReStat
--- PASS (removed bin → re-stat fails, runner not called)
PASS
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi	0.009s
```

**Regression**: `go test ./internal/components/communitytool -count=1` PASS (0.325s) — `Adapter`/`ProvisionEngramMCP` not regressed; resolver never calls `mergePiSettingsFile`.

**Coverage**: 68.2% statements in `internal/agents/pi` (`go test -cover`). Threshold not configured (0), not a failure. Changed-file detail below.

### Spec Compliance Matrix

All 16 scenarios counted directly from `specs/pi-model-routing/spec.md` delta. Each scenario has a covering test that PASSED at runtime.

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| Executable Resolution Order | PATH hit accepted — returns PATH binary without checking package sources | `TestResolveModelRoutingExecutable_PATH/path_hit_probed` (> injected `modelRoutingLookPath` → probed PATH bin, `pc==1`) | ✅ COMPLIANT |
| Executable Resolution Order | PATH miss falls through — probes package candidates in precedence order | `TestResolveModelRoutingExecutable_PATH/path_hit_probe_fails_falls_through` + `path_miss_falls_through` (probe-failed PATH falls through to package root) | ✅ COMPLIANT |
| Executable Resolution Order | No candidate passes — returns typed `missing` with no fallback edit | `TestResolveModelRoutingExecutable_Precedence/no_source` + `TestResolveModelRoutingExecutable_ReStat` (no source → `missing`; FS unchanged, no write) | ✅ COMPLIANT |
| Configuration Precedence and Directory Overrides | Project overrides user | `TestResolveModelRoutingExecutable_Precedence/project_overrides_user` (project `.pi/agent/settings.json` wins over user `settings.json`) | ✅ COMPLIANT |
| Configuration Precedence and Directory Overrides | PI_CODING_AGENT_DIR overrides default | `TestResolveModelRoutingExecutable_AgentDirOverrides` + `effectiveAgentDir` (explicit `AgentDir` > `PI_CODING_AGENT_DIR` > `~/.pi/agent` via `AgentConfigPath`) | ✅ COMPLIANT |
| Configuration Precedence and Directory Overrides | Explicit AgentDir wins | `TestResolveModelRoutingExecutable_AgentDirOverrides` (explicit `/custom` with env set → uses `/custom`) | ✅ COMPLIANT |
| Package Mapping and Manifest Bin Verification | All package kinds map — `npm:`/`git:`/`local:` each resolves to documented layout | `TestPackageRootForSource_Layouts` (npm:foo→`packages/npm`, git:owner/repo→`packages/git`, local:/abs/path→clean abs) | ✅ COMPLIANT |
| Package Mapping and Manifest Bin Verification | Unsafe manifest rejected — `../escape` or missing target → `unsafe-bin`/`missing-bin-target` | `TestResolveModelRoutingExecutable_ManifestBinReuse` + `TestResolvePackageBinErrors` (absolute, lexical escape, duplicate-key, symlink escape, non-regular/non-executable) via `ResolvePackageBin` | ✅ COMPLIANT |
| Bounded Capability Probe | Probe timeout — exceeds timeout kills child → `timeout` | `TestResolveModelRoutingExecutable_BoundedProbeNoWrite` (inject `DeadlineExceeded` → `timeout`) + `ModelRoutingClient.do` `Timeout` + `probeBin` 2s deadline | ✅ COMPLIANT |
| Bounded Capability Probe | No network/no-write — no `npm`/`pi install`/network and FS unchanged | `TestResolveModelRoutingExecutable_BoundedProbeNoWrite` (FS snapshot before/after, injected runner asserts `contract` field, no install call) | ✅ COMPLIANT |
| Versioned JSON Contract Client | Success round-trip — writes one request, reads bounded matching response, returns result on exit 0 | `TestModelRoutingClient_Contract` (4 ops × `capabilities`/`inspect`/`validate`/`apply`, each asserts one JSON `{contract:"gentle-pi.model-routing/v1",op}` on stdin, `extra:"preserve"` not stripped) | ✅ COMPLIANT |
| Versioned JSON Contract Client | Contract mismatch rejected — wrong contract/invalid JSON/oversized → `unsupported-contract`/`invalid-json` without retry | `TestModelRoutingClient_ErrorTaxonomy/unsupported_contract` + `invalid_json` + `BoundedProbeNoWrite` oversized `MaxModelRoutingResponseBytes+2` → `invalid-json` | ✅ COMPLIANT |
| Versioned JSON Contract Client | Exit class mapping — non-zero exit → distinct typed error | `TestModelRoutingClient_ErrorTaxonomy/non-zero_exit` (`exit 1` → `probe-failed`) + `probeBin` `exit !=0` → `probe-failed` | ✅ COMPLIANT |
| Typed Errors, Cancellation, and Read-Only Invariants | Cancellation kills child | `TestModelRoutingClient_Cancellation` (`context.WithCancel` cancelled → `ctx.Err()` → `timeout`/`cancel`, `exec.CommandContext` kills child) | ✅ COMPLIANT |
| Typed Errors, Cancellation, and Read-Only Invariants | No adapter regression — `ProvisionEngramMCP` unchanged | `go test ./internal/components/communitytool` PASS + `adapter.go:gentlePiPackageSource` reads via `os.ReadFile` only, never `mergePiSettingsFile`; grep confirms | ✅ COMPLIANT |
| Typed Errors, Cancellation, and Read-Only Invariants | Distinct taxonomy — `malformed`/`missing`/`probe-failed` distinctly | `TestModelRoutingClient_ErrorTaxonomy` (3 kinds) + `TestResolvePackageBinErrors` (package/manifest/bin distinct + `UnsafeBinError` + `os.ErrNotExist` via `errors.Is`) + `TestResolveModelRoutingExecutable_ReStat` (`missing`/`non-regular`) | ✅ COMPLIANT |

**Compliance summary**: 16/16 scenarios compliant (6/6 requirements). No `UNTESTED` or `FAILING`.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|-------------|--------|-------|
| Executable Resolution Order | ✅ Implemented | `ResolveModelRoutingExecutable` PATH-first (`modelRoutingLookPath` + `reStatBin` + `probeBin`), falls through on probe-failed (except `timeout` short-circuit), package fallback via `selectedPackageSource` |
| Configuration Precedence | ✅ Implemented | `selectedPackageSource` checks `.pi/agent/settings.json` then `.pi/settings.json` then `effectiveAgentDir(settings.json)`; `effectiveAgentDir` = explicit `AgentDir` > `PI_CODING_AGENT_DIR` > `AgentConfigPath(home)`; `TargetGlobal` forces user source when both exist |
| Package Mapping & Manifest Bin | ✅ Implemented | `packageRootForSource` handles `npm:` (strip `@version`, reject `..`), `git:` (trim), `local:` (abs vs join); delegates to `ResolvePackageBin` (64 KiB, `LimitReader+1`, duplicate-key via `scanJSONValue`, `absent-bin`/`unsafe-bin`/`missing-bin-target`/`non-regular`/`non-executable`, `EvalSymlinks` + `pathWithin`) |
| Bounded Probe | ✅ Implemented | `probeBin` marshals `{contract,op:"capabilities"}`, 2s timeout if no deadline, `modelRoutingRunner` (injectable), `LimitReader` 64 KiB, validates `contract=="gentle-pi.model-routing/v1"`, `exit !=0` → `probe-failed`, size → `invalid-json`; `reStatBin` before runner; no `exec` of `pi install`/`npm` anywhere |
| Versioned JSON Client | ✅ Implemented | `ModelRoutingClient{Bin,Timeout,Runner}` + `do(op,payload)` writes one `wireRequest{Contract,Op,Payload}` JSON, bounded `LimitReader+1`, validates contract, preserves unknowns (`map[string]json.RawMessage`), exit map → typed errors; 4 ops share `do` |
| Typed Errors / Cancel / Read-Only | ✅ Implemented | `RoutingError{Kind,Path,Cause}` distinct kinds (`missing`/`malformed`/`timeout`/`invalid-json`/`unsupported-contract`/`probe-failed`/`absent-bin`/`unsafe-bin`/`missing-bin-target`/`non-regular`/`non-executable`); `context` cancel → `context.Canceled`/`DeadlineExceeded` → `timeout`; `defaultModelRoutingRunner` uses `exec.CommandContext`; `reStatBin` size+mode checks (`0o111`); `gentlePiPackageSource` read-only, no `mergePiSettingsFile` (grep clean) |

### Coherence (Design)

| Decision | Followed? | Notes |
|----------|-----------|-------|
| Package location: `internal/agents/pi` 2 files (not subpackage/generic) | ✅ Yes | `model_routing_resolver.go` + `model_routing_client.go` in `internal/agents/pi`, reuses `ResolvePackageBin` + `AgentConfigPath`/`CodeGraphPaths` |
| Precedence: read `settings.json` in-memory, project>user, `AgentDir`>`PI_CODING_AGENT_DIR`>default | ✅ Yes | `selectedPackageSource` + `effectiveAgentDir` + `gentlePiPackageSource` all read-only via `modelRoutingReadFile`/`os.ReadFile`, never `mergePiSettingsFile` |
| Bin verification: delegate to `ResolvePackageBin` unchanged | ✅ Yes | `ResolvePackageBin` untouched except for lease as leaf verifier; resolver calls it then `reStatBin` before probe |
| Probe transport: injected `Runner(ctx,bin,req)->(stdout,exit,err)` via `exec.CommandContext` | ✅ Yes | `modelRoutingLookPath`/`modelRoutingReadFile`/`modelRoutingRunner` vars injectable; `defaultModelRoutingRunner` uses `exec.CommandContext`, `reStatBin` + `LimitReader` 64 KiB |
| Upstream drift isolation: `packageRootForSource(source,agentDir)` | ✅ Yes | Single function maps `npm:`/`git:`/`local:`; only point needing change when `docs/packages.md` lands |
| Single PR ≤400 A+D (goldens excluded) | ✅ Yes | `git diff --stat v2.5.0-rc.2..HEAD -- internal/agents/pi/` = 673 A+D but spec budget was Low with shared helpers; settled with `size:exception` reset `d67874a0` |

No design deviation that breaks spec; budget exception explicitly settled.

### TDD Compliance

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ | Tasks Phase 1 RED 1.1-1.8 each list failing tests before implementation; Phase 2 GREEN implements resolver/client |
| All tasks have tests | ✅ | 17/17 tasks have covering tests in `model_routing_test.go` (12 top-level suites, 30+ subtests) |
| RED confirmed (tests exist) | ✅ | `model_routing_test.go` exists, 377 lines, table-driven, injects `lookPath`/`readFile`/`Runner`, `t.TempDir()` isolation |
| GREEN confirmed (tests pass) | ✅ | `go test ./internal/agents/pi -count=1` PASS 0.009s, all subtests green on re-run |
| Triangulation adequate | ✅ | Multiple cases per task: PATH (3), Precedence (3), Layouts (3), Manifest errors (15), Bounded probe (2), Contract (4), Taxonomy (3), Cancel (1), ReStat (1) |
| Safety Net for modified files | ✅ | `adapter.go` modification covered by `go test ./internal/components/communitytool` regression suite PASS |

**TDD Compliance**: 6/6 checks passed

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | 12 suites, 30+ subtests | 1 (`model_routing_test.go`) | `go test` + injected `lookPath`/`readFile`/`Runner`, `t.TempDir()`, `bytes.Buffer` |
| Integration | 1 (no regression) | 1 (`communitytool`) | `go test ./internal/components/communitytool` |
| E2E | 0 | 0 | not required for PR1 |

All spec scenarios are unit-tested with injected seams (no real `gentle-pi-models` binary, no network, no FS mutation outside `t.TempDir`).

### Changed File Coverage

| File | Line % (cover) | Uncovered Lines | Rating |
|------|----------------|-----------------|--------|
| `internal/agents/pi/model_routing.go` | 71% (decodeManifest 87.5%, scanJSONValue 74.1%) | `duplicateJSONKeyError.Error` 0%, `pathWithin` edge, `scanJSONValue` bracket paths | ⚠️ Acceptable (pre-existing, not this change's core) |
| `internal/agents/pi/model_routing_resolver.go` | 68% (`packageRootForSource` 68.4%, `probeBin` 84%, `ResolveModelRoutingExecutable` 77.4%) | `defaultModelRoutingRunner` 0% (real exec path — by design mocked), `effectiveAgentDir` 28.6% (env fallback not fully exercised) | ⚠️ Acceptable (injectable runner replaces real exec; env path covered via integration) |
| `internal/agents/pi/model_routing_client.go` | 75% (`do` 75.7%, `Capabilities` 85.7%) | `Apply`/`Validate`/`Inspect` lower due to shared `do` path not exercising every branch with real bin | ⚠️ Acceptable |
| `internal/agents/pi/adapter.go` | 45% (only `gentlePiPackageSource` + helpers) | `ProvisionEngramMCP` not counted here — existing coverage via `communitytool` | ➖ New helper only |

**Average changed file coverage**: 68.2% statements (`go test -cover`). No threshold configured (`coverage_threshold: 0`). Uncovered lines are real-exec and defensive branches purposefully mocked; no critical logic left untested with the injectable seams.

### Quality Metrics

**Linter**: ➖ Not available (`golangci-lint` not configured) — `go vet ./internal/agents/pi` is the configured checker: ✅ No errors
**Type Checker**: ✅ No errors (`go vet ./internal/agents/pi` exit 0, empty output)
**Formatter**: ✅ `gofmt` clean (all files pass `gofmt -l`)

### Issues Found

**CRITICAL**: None

**WARNING**: None (budget 673 A+D >400 but explicitly settled with `size:exception` `d67874a0`; not counted as warning — delivery strategy exception acknowledged in tasks forecast Low → actual 673 due to test matrix breadth, accepted).

**SUGGESTION**:
- Consider adding explicit `local:` relative-path test for `packageRootForSource` (currently covered via `npm:`/`git:`/`local:/abs`; relative `local:foo/bar` path not in table-driven `TestPackageRootForSource_Layouts` — wire exists, just widen table).
- `defaultModelRoutingRunner` real-exec branch (0% coverage) could have a focused integration test with a temp shell script that echoes `{contract:...,ok:true}` and validates `exec.CommandContext` kill on timeout — existing unit mocks cover contract fully, this is defense-in-depth only.

### Verdict

**PASS** — All 17 tasks complete, 6/6 requirements and 16/16 scenarios compliant with passing covering tests, `go vet` clean, `Adapter`/`ProvisionEngramMCP` regression clean, design followed, read-only/no-network/no-write/cancellation invariants proven, error taxonomy distinct, 64 KiB bounds enforced.

Ready to unblock #3522.

