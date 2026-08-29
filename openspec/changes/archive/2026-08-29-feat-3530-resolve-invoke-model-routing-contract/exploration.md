# Exploration: feat(pi): resolve and invoke the installed model-routing contract (#3530)

## Current State

Gentle AI installs the Pi harness as Pi resource packages (`pi install npm:gentle-pi`, `npm:pi-mcp-adapter`, `pi-subagents-j0k3r`, etc.) via `internal/agents/pi/adapter.go` (`InstallCommand`, `ProvisionEngramMCP`). Runtime behavior is delegated to `gentle-pi`; Gentle AI never calls a Pi-owned model-routing binary today.

Existing Pi integration in Gentle AI is limited to:
- `internal/agents/pi/adapter.go` — adapter identity, paths (`ConfigPath` → `~/.pi`, `AgentConfigPath` → `~/.pi/agent`, `ProvisionEngramMCP` merges `settings.json`/`npm/package.json`), and detection via `exec.LookPath("pi")`.
- `internal/agents/pi/model_routing.go` — **already landed** read-only `ResolvePackageBin(packageRoot)` that validates `package.json` bin for `gentle-pi-models` (64 KiB bound, duplicate-key rejection, `absent-bin`/`malformed-bin`/`unsafe-bin`/`missing-bin-target`/`non-regular`/`non-executable` checks, symlink-canonical containment via `EvalSymlinks` + `pathWithin`). Used by no model-routing resolver yet, but is the intended leaf for package-source resolution.
- `internal/agents/pi` CodeGraph seam (`CodeGraphPaths`, `EffectiveCodeGraphMCPPath`, `DiscoverCodeGraphChildren`) already demonstrates the correct `PI_CODING_AGENT_DIR` pattern: `PI_CODING_AGENT_DIR` overrides `~/.pi/agent` as the agent home, while project-local `.mcp.json`/`.pi/subagents/` shadowing is handled by precedence and normalization. `docs/pi.md` and `docs/agents.md` echo this.

On the Pi side, `gentle-pi` `main` exposes `lib/model-routing-authority.ts` (shared saved-routing authority: `normalizeRoutingEntry`, `readSavedModelConfig`, thinking-level clamping) and a not-yet-frozen machine-readable contract tracker at `Gentleman-Programming/gentle-pi#381` (`gentle-pi.model-routing/v1` with `capabilities`/`inspect`/`validate`/`apply` over a standalone `gentle-pi-models` executable). Pi packages `docs/packages.md` is cited in #3530 as the source for npm/git/local roots and `PI_CODING_AGENT_DIR`, but is **404 on `gentle-pi/main` today** — it will land with the `#382` foundation (currently blocked on `#389`→`#396/PR#408`→`#395`→`#391` plus durable-write/materialization slices). The contract executable therefore exists only as a planned `bin.gentle-pi-models` entry, not a stable published binary.

For TUI consumers, `internal/tui/screens` holds `claude_model_picker.go`, `codex_model_picker.go`, and `pi_background.go`. Pi model assignment today is owned by `gentle-pi`’s `/gentle:models` modal persisted to `.pi/gentle-ai/models.json`; Gentle AI has no Pi model-routing TUI. Issue #3522 depends on this PR as its resolver/client prerequisite.

The starting point for #3530 is therefore: bin verification primitives exist, PI_CODING_AGENT_DIR/project-precedence patterns exist, but no `gentle-pi-models` resolver or `gentle-pi.model-routing/v1` process client exists.

## Affected Areas

- `internal/agents/pi/model_routing.go` — keep as leaf bin-verification; add resolver and client alongside it rather than modifying its contract. Reuses `MaxPackageManifestBytes`, `packageBinName`, and `UnsafeBinError` taxonomy.
- `internal/agents/pi/adapter.go` — read-only settings discovery (`readPiJSONObject`, `piPackagesAsSlice`, `piPackageIdentity`) is reusable for "selected `gentle-pi` source" but currently private and `pi-mcp-adapter`-specific; will need a `gentle-pi`-aware variant or a narrow exported helper. `CodeGraphPaths`/`AgentConfigPath` already handles `PI_CODING_AGENT_DIR`; resolver must mirror that seam and also accept an explicit `AgentDir` override.
- `internal/agents/pi/model_routing_resolver.go` (new) — bounded resolver: accept `(cwd, agentDir, target)`; probe `PATH` first, then project→user `settings.json` precedence, then map `npm:`/`git:`/`local:` to Pi package roots, then `ResolvePackageBin`, then capability-probe each candidate before accepting. Must stay read-only, no `pi install`/npm fetch/model calls/fallback JSON edits.
- `internal/agents/pi/model_routing_client.go` (new) — typed `ModelRoutingClient` with `Capabilities(ctx)`, `Inspect(ctx, ...)`, `Validate(ctx, draft)`, `Apply(ctx, draft)`; sends one versioned JSON request (`contract: "gentle-pi.model-routing/v1"`), validates one JSON response + documented exit classes, distinguishes `missing`, `malformed`, `timeout`, `invalid-json`, `unsupported-contract`, `probe-failed` errors, respects `context.Context` cancellation (terminate child, return promptly), bounded timeouts, no network/write.
- `internal/agents/pi/model_routing_test.go` — expand to cover resolver/client contracts; keep existing bin tests green. New focused tests for PATH/package layouts/precedence/overrides/probe failures/timeouts/protocol/no-network/no-write (issue requires them, PR ≤400 A+D forces slice discipline).
- `internal/tui/screens` — **out of scope** for this PR (no TUI screen); PR2 `#3522` consumes the resolver/client. No existing Pi/OpenCode/Claude/Codex behavior may regress — adapter detection and `internal/opencode`, `internal/cli` flows must stay unchanged.
- `docs/pi.md` / `openspec/specs` — downstream proposal/spec will document the resolver/client contract; exploration notes the missing `docs/packages.md` dependency.
- `internal/components/communitytool/pi_codegraph.go` — pattern to copy for injectable runner seam (runtime probe with `PI_CODING_AGENT_DIR` passthrough and effective-path decoupling). Do not reuse its probe directly for model-routing.

## Approaches

### 1. Extend `internal/agents/pi` with injectable resolver + subprocess client (recommended shape)

Add two files in the existing `pi` package: `model_routing_resolver.go` exposing `ResolveModelRoutingExecutable(ctx, cwd, agentDir, target)` and `model_routing_client.go` exposing `type Client struct { binary string; timeout time.Duration; runner func(...) }` with `Capabilities`/`Inspect`/`Validate`/`Apply`. Runner and `lookPath`/`stat`/`readFile` are injected vars (matching `Adapter.lookPath`/`piWalkDir` seams) so tests run without a real `pi` or `gentle-pi-models` binary. Bin verification delegates to `ResolvePackageBin`. Reads `settings.json` from `project` (`<cwd>/.pi/agent/settings.json` or `<cwd>/.pi/settings.json` depending on Pi’s actual precedence) and `user` (`<agentDir>/settings.json`), with project precedence, matching Pi’s documented convention — validated against the future `docs/packages.md` before freeze.

- Pros: Minimal new dependencies; reuses proven `ResolvePackageBin` containment and `CodeGraphPaths` override discipline; injectable seams already proven in `adapter_test.go`/`pi_codegraph_test.go`; keeps change at ≤400 A+D; aligns with issue’s "accept explicit cwd, agent/config dir, target context" and "distinct actionable errors".
- Cons: `adapter.go` helpers are currently `pi-mcp-adapter`-specific — requires careful extraction to avoid over-generalizing; Pi package-root layout for npm/git/local must be confirmed once `docs/packages.md` lands; caller must decide target enum (`project` vs `global`) explicitly.
- Effort: Medium (new files + thorough table-driven tests, no TUI).

### 2. Separate subpackage `internal/agents/pi/modelrouting`

Create `internal/agents/pi/modelrouting/{resolver,client}.go` to isolate the contract from the legacy adapter.

- Pros: Cleaner import boundary if the contract grows (version negotiation, streaming); avoids widening `pi` package visibility.
- Cons: Duplicates `PI_CODING_AGENT_DIR`/path helpers or forces them to be exported; introduces a cross-package dependency for `ResolvePackageBin` (needs export or move); heavier for a ≤400-line first slice — splits tests across packages and risks exceeding the budget with boilerplate.
- Effort: Medium-High for PR1; justified only if the design for PR2 foresees a large client surface.

### 3. Share a generic `internal/pi` process client reused across adapters

Extract a generic JSON-RPC process client (`internal/pi/client.go`) parameterized by contract name, then instantiate it for `gentle-pi.model-routing/v1`.

- Pros: Reusable for future Pi contracts beyond model-routing.
- Cons: Violates YAGNI — only one contract consumer exists; generic client must still handle Pi-specific roots/precedence/probe semantics, so abstraction leaks; risks premature generalization before `#383`/`#384` re-freeze. Issue explicitly forbids fallback JSON editing — a shared client that "knows" about Pi config writes is a footgun.
- Effort: High; not bounded for PR1.

## Recommendation

**Approach 1** for PR1.

Rationale: `ResolvePackageBin` and the `pi-mcp-adapter` CodeGraph integration already proved the two hardest invariants (bin containment + `PI_CODING_AGENT_DIR`/`project` vs `user` precedence) inside `internal/agents/pi`. Reusing that package and adding injectable `lookPath`/`readFile`/`commandRunner` seams minimizes new concepts, keeps the diff inside the 400 A+D budget, and matches the chain’s intent: PR1 is a library — no TUI, no install, no network, no write — that PR2 `#3522` imports. Defer a subpackage extraction to PR2 only if the client surface demonstrably outgrows `pi`.

Concrete slice within the 400-line guard:
1. `model_routing_resolver.go` — `type Target int` (`TargetProject`, `TargetGlobal`), `Options{ Cwd, AgentDir, Target }`, `Resolve` returns candidate path; internal helpers `resolveOnPATH` (LookPath + bounded capability probe), `selectedPackageSource` (project→user `settings.json` precedence), `packageRootForSource` (maps `npm:`→`<agentDir>/npm/<name>` or similar, `git:`→`<agentDir>/git/...`, `local:`→absolute path — **exact mapping gated on the pending `docs/packages.md`**), `ResolvePackageBin` delegation, then probe.
2. `model_routing_client.go` — `Client{ Bin string; Timeout time.Duration; Runner }` with `do(ctx, req) (resp, exitClass, err)` that writes one JSON request to stdin, reads bounded JSON from stdout, validates `contract == "gentle-pi.model-routing/v1"` and schema, maps exit codes (`0` success, `2` invalid input, `3` unsupported contract, `4` capability unavailable, `5` persistence/apply failure — exact values deferred to re-frozen Pi contract) to distinct typed errors.
3. Table-driven tests owning the issue’s matrix: PATH hit/miss, npm/git/local layouts, precedence, `PI_CODING_AGENT_DIR` + explicit `AgentDir` override, intact vs missing/malformed manifest, probe failure, timeout, invalid JSON, unsupported contract, cancellation, and negative assertions for no-network (no `exec` of `npm`/`pi install`) and no-write (pre/post snapshots equal — same pattern as `TestResolvePackageBinForms`).

Do not hard-code one transient Pi cache path, do not edit `settings.json`, do not invoke `pi install` or `npm` resolution. Every candidate must be capability-probed with a bounded non-network `capabilities` request before being returned — PATH first, then package-roots in precedence order.

## Risks

- **Upstream contract not frozen.** `gentle-pi#381`/`#382`/`#383`/`#384` are mid-chain and labeled `needs-review`; executable name `gentle-pi-models`, request/response schema, and exit classes may shift after `#382` foundation completion. Mitigation: scope PR1 to the stable subset (PATH + bin verification + bounded probe + typed errors) and feature-flag unknown fields as preserved/unknown rather than failing.
- **Pi package-root layout is undocumented at HEAD.** `docs/packages.md` is 404 on `gentle-pi/main`; the exact user/project npm, git, and local roots (e.g., `~/.pi/packages/npm/...` vs `~/.pi/agent/npm/...` vs `~/.cache/pi/...`) must be confirmed before freeze. Mitigation: route all root computation through a single `packageRootForSource` helper that is trivially corrected once the doc lands, and keep a test fixture per package kind.
- **Budget pressure from required test matrix.** PATH, package layouts, precedence, overrides, probe failures/timeouts, protocol validation, no-network/no-write is a large matrix for ≤400 A+D. Mitigation: use shared helpers and table-driven cases (as `model_routing_test.go` does) and defer TUI/executable publishing to `#384`.
- **TOCTOU between resolution and exec.** `ResolvePackageBin` already documents that the caller must revalidate the returned path before exec; resolver + client must do so or document the gap. Mitigation: re-stat the binary immediately before spawning, or fail closed.
- **Config-root collision with CodeGraph paths.** `CodeGraphPaths` already consumes `PI_CODING_AGENT_DIR`; resolver adding a second interpretation could diverge. Mitigation: share `pi.AgentConfigPath`/`CodeGraphPaths` logic or explicitly thread `AgentDir` through options so tests prove project-override decoupling (`TestCodeGraphPathsKeepsAgentDirectoryWhenProjectMCPOverrides` pattern).
- **Existing adapter behavior regression.** Any change to `settings.json` handling could affect `ProvisionEngramMCP`. Mitigation: keep `mergePiSettingsFile` untouched; resolver is read-only and never calls it.
- **Symlink and platform variance.** `ResolvePackageBin` handles POSIX executable bits and `EvalSymlinks` escapes; Windows ignores exec bits. Mitigation: mirror `model_routing_test.go` skip guards (`runtime.GOOS == "windows"`).

## Ready for Proposal

Yes — with one gate. The exploration is sufficient to write `proposal.md` and a constrained spec for the resolver+client, but the proposal MUST record an explicit dependency on the re-frozen `gentle-pi` `docs/packages.md` (expected via `#382`) and on the final `gentle-pi.model-routing/v1` schema/exit classes (via `#383`/`#384`). If those Pi artifacts move, the `packageRootForSource` mapping and JSON contract are the only isolated change points.

Next phase: `sdd-propose` for `feat-3530-resolve-invoke-model-routing-contract`, carrying the 400-line guard and the read-only/no-network/no-write invariants into the spec (`sdd-spec`).
