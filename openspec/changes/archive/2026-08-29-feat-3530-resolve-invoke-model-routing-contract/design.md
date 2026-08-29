# Design: feat(pi): resolve and invoke installed model-routing contract (#3530)

## Technical Approach

Extend `internal/agents/pi` with `model_routing_resolver.go` + `model_routing_client.go`. Resolver does PATH→project→user roots→`ResolvePackageBin`→bounded `capabilities` probe. Client sends one JSON `{contract:"gentle-pi.model-routing/v1"}` via stdin, bounded 64 KiB stdout, validates contract/exit → typed errors. Read-only, no install/npm/network/write. Injectable `lookPath`/`readFile`/`Runner`; precedence explicit `AgentDir` > `PI_CODING_AGENT_DIR` > `~/.pi/agent`. Single PR ≤400 A+D. Maps to proposal Approach 1; covers `pi-model-routing` (6 req, 16 scenarios).

## Architecture Decisions

| Decision | Option | Tradeoff | Choice |
|---|---|---|---|
| Package location | A `internal/agents/pi` 2 files / B subpackage / C generic `internal/pi` client | B duplicates helpers, exceeds budget. C leaks Pi roots into generic (YAGNI before #383/#384 freeze). A reuses `ResolvePackageBin`+`CodeGraphPaths`. | **A** |
| Precedence | Read `settings.json` in-memory | Avoids `mergePiSettingsFile`; `project (.pi/agent or .pi) > user (agentDir/settings.json)` via `selectedPackageSource`. | **Read-only helper, project>user** |
| Bin verification | Delegate to `ResolvePackageBin` | Keeps 64 KiB bound, duplicate-key reject, `absent/unsafe/missing/non-regular/non-executable`, symlink `EvalSymlinks`+`pathWithin`. | **Reuse unchanged** |
| Probe transport | Injected `Runner(ctx,bin,req)->(stdout,exit,err)` via `exec.CommandContext` | Mirrors `piCodeGraphEffectiveMCPProbe`; timeout/cancel kills child; `LimitReader` 64 KiB. | **Injectable Runner** |
| Upstream drift isolation | `packageRootForSource(source,agentDir)` | Only point changing when `docs/packages.md` (#382) lands. | **Single indirection** |

## Data Flow

```
Caller(tui #3522) → ResolveModelRoutingExecutable
  1 lookPath("gentle-pi-models") → probe(capabilities) → return if ok
  2 selectedPackageSource(project settings → user settings)
  3 packageRootForSource → ResolvePackageBin → probe(capabilities)
  4 → missing (no fallback edit)

ModelRoutingClient → do({contract:"gentle-pi.model-routing/v1",op,payload})
  write-one JSON stdin → bounded read stdout → validate contract → map exit → Result|typed error
```

Re-stat before spawn mitigates TOCTOU (read-only cannot eliminate).

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/agents/pi/model_routing_resolver.go` | Create | `Target` enum, `Resolve…`, `resolveOnPATH`, `selectedPackageSource`, `packageRootForSource`, probe, injectable vars |
| `internal/agents/pi/model_routing_client.go` | Create | `ModelRoutingClient`, `do()`, 4 ops, contract const, exit map, bounded I/O, preserve unknowns |
| `internal/agents/pi/model_routing.go` | Modify | Keep leaf verifier |
| `internal/agents/pi/adapter.go` | Modify | Read-only `gentlePiPackageSource` helper; reuse `AgentConfigPath` |
| `internal/agents/pi/model_routing_test.go` | Modify | Table-driven: PATH, npm/git/local, precedence, env/AgentDir, manifest, probe/timeout/invalid-json/contract, cancel, no-network/no-write |

No writes to `internal/tui/screens` (PR2).

## Interfaces / Contracts

```go
type Target int
const (TargetProject Target = iota; TargetGlobal)
func ResolveModelRoutingExecutable(ctx context.Context, cwd, agentDir string, target Target) (string, error)
var modelRoutingLookPath = exec.LookPath // injectable
var modelRoutingReadFile = os.ReadFile   // injectable

type ModelRoutingClient struct {
    Bin string; Timeout time.Duration
    Runner func(ctx context.Context, bin string, req []byte) (stdout []byte, exit int, err error)
}
func (c *ModelRoutingClient) Capabilities(ctx context.Context) (*CapabilitiesResponse, error)
func (c *ModelRoutingClient) Inspect(ctx context.Context, req InspectRequest) (*InspectResponse, error)
func (c *ModelRoutingClient) Validate(ctx context.Context, draft json.RawMessage) (*ValidateResponse, error)
func (c *ModelRoutingClient) Apply(ctx context.Context, draft json.RawMessage) (*ApplyResponse, error)

type wireRequest struct {
    Contract string `json:"contract"` // "gentle-pi.model-routing/v1"
    Op       string `json:"op"`       // capabilities|inspect|validate|apply
    Payload  json.RawMessage `json:"payload,omitempty"`
}
type RoutingError struct{ Kind, Path string; Cause error } // missing|malformed|timeout|invalid-json|unsupported-contract|probe-failed|absent-bin|unsafe-bin|missing-bin-target|non-regular|non-executable
```

Bounds 64 KiB via `LimitReader(+1)`; duplicate keys rejected; `context` kills child; exit 0 success, non-zero → typed errors (mapping deferred to #383, unknown→`probe-failed`).

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | PATH hit/miss, precedence, env/AgentDir overrides, npm/git/local, unsafe/missing/non-regular/non-exec, probe timeout/cancel, invalid-json/unsupported/oversized, exit map, bounded read, unknown preservation, re-stat, no-network/no-write | Table-driven `go test ./internal/agents/pi` with faked `lookPath`/`readFile`/`Runner`; FS snapshot compare |
| Integration | No regression `Adapter`/`CodeGraphPaths`/`ProvisionEngramMCP` | `go test ./internal/agents/pi ./internal/components/communitytool` |
| E2E | None for PR1 | PR2 #3522 stubs client |

## Threat Matrix

Per `internal/assets/skills/sdd-design/references/threat-matrix.md` — change touches subprocess + executable classification + process integration:

| Boundary | Cases | Applicability | Design response | Planned RED tests |
|----------|-------|---------------|-----------------|-------------------|
| Documentation-like paths | `requirements.txt`, `CMakeLists.txt`, Markdown/MDX, `README.sh` | **N/A** — only `package.json` bin via `ResolvePackageBin` containment | No doc execution | None |
| Git repository selection | `git -C`, relative/absolute | **N/A** — no VCS; cwd only for settings discovery | Explicit `cwd` param, no repo selection | None |
| Commit state | staged, `commit -a`, empty index | **N/A** — no commits | — | None |
| Push state | tracking, first push, refspec | **N/A** — no pushes | — | None |
| PR commands | `--head`, env prefix, composed | **N/A** — single-arg `gentle-pi-models` via `exec.CommandContext`, no `sh -c` | No shell composition | None |

Beyond matrix (→ tasks/RED tests): timeout kills child, cancel via `context`, 64 KiB bound, no `npm`/`pi install`/network, no writes, re-stat, distinct errors, contract preservation.

## Migration / Rollout

No migration. No persisted state. Rollback: revert commit. If Pi contract shifts (#383/#384), patch `packageRootForSource` + exit map only. No flag; unknowns preserved.

## Open Questions

- [ ] Exact `docs/packages.md` roots (npm/git/local) awaiting `gentle-pi#382` — isolated to `packageRootForSource`
- [ ] Final `gentle-pi.model-routing/v1` schema/exits awaiting `#383`/`#384` — unknown→fail-closed
- [ ] Project `settings.json` path (`.pi/agent` vs `.pi`) to validate against Pi runtime
