# Design: OpenCode Model Selector Effective Config

## Technical Approach

Add one `internal/opencode` config boundary that resolves the effective OpenCode config path, parses JSON/JSONC, extracts configured providers/models, URL metadata, and assignment presence, then feeds the existing TUI, install/inject, and sync paths. `opencode models --verbose` remains the runtime authority; configured data only augments what the runtime catalog does not expose. Context7 for `/anomalyco/opencode` confirms JSONC config support, project discovery for `opencode.jsonc`/`opencode.json`, and provider `options.baseURL`; local `opencode-expert` reference confirms global config under `~/.config/opencode/`.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Resolver/parser boundary | Create `internal/opencode/config.go` with resolver, JSONC parser, configured catalog reader, and assignment-presence reader. | Duplicate JSONC parsing in TUI, inject, and sync. | One boundary keeps precedence and presence semantics identical across flows. |
| Precedence/write target | Resolve explicit path first when a caller supplies one; otherwise prefer the highest-priority existing supported config, with `opencode.jsonc` before `opencode.json` in the same directory and nearest project config before parent/global fallback. If none exists, write the current supported default path. | Always write `opencode.json`; always create `opencode.jsonc`. | Honors existing user intent without inventing a new file when no config exists. |
| Runtime merge | Keep `DiscoverCatalog` as primary and merge configured providers/models afterward: runtime values win on duplicate IDs, configured-only providers/models are added, configured URL fills missing provider URL metadata. | Replace runtime discovery with file parsing. | Preserves dynamic LM Studio/runtime data while making file-backed custom providers selectable. |
| Assignment presence | Add transient `AssignmentPresence{Present, Cleared, Assignment}` semantics; do not persist a new state schema unless implementation proves it necessary. | Treat missing map entries as cleared. | Absent means no current OpenCode opinion; cleared means user intent that stale Gentle AI state must not restore. |

## Data Flow

```text
OpenCode config files ──→ opencode.ResolveEffectiveConfig ──→ ConfigSnapshot
                         ├─ configured catalog ─┐
opencode models --verbose ──→ DiscoverCatalog ──┴─→ ModelPickerState
                         └─ assignment presence ──→ install/inject + sync merge
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/opencode/config.go` | Create | Effective config resolver, JSONC-tolerant parser, provider/model URL extraction, assignment presence. |
| `internal/opencode/models.go` | Modify | Add provider URL metadata if needed; keep default path helper. |
| `internal/opencode/catalog.go` | Modify | Add merge helper; do not change command execution contract. |
| `internal/tui/model.go` | Modify | Replace `DefaultSettingsPath`/assignment reader wiring with effective snapshot resolver. |
| `internal/tui/screens/model_picker.go` | Modify | Accept merged runtime+configured catalog without changing picker navigation semantics. |
| `internal/components/sdd/inject.go` | Modify | Write the resolved effective target, including existing `opencode.jsonc`, using assignment presence from the shared OpenCode config boundary. |
| `internal/cli/sync.go` | Modify | Overlay persisted state only for absent assignments; preserve present and cleared current config entries. |
| `internal/cli/run.go` | Modify | Include the effective OpenCode settings path in sync backup/verification scope so an existing `opencode.jsonc` is protected instead of only the adapter default `opencode.json`. |

## Interfaces / Contracts

- `ConfigSnapshot`: resolved read paths, write target, providers, assignment presence, parse diagnostics.
- LM Studio URL rule: direct `provider.<id>.url` wins; `provider.<id>.options.baseURL` is used only when direct `url` is absent.
- Explicitly excluded from all file actions: `internal/assets/opencode/plugins/review-result-artifacts.ts`, `internal/assets/review_plugin_recovery_test.go`, #934/#2288 warning UX, and #1015 SQLite/runtime discovery.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | JSONC parse, precedence, write target, URL direct/fallback, catalog merge, presence states. | New `internal/opencode/config_test.go` plus existing catalog tests. |
| Integration | Install/inject writes existing `opencode.jsonc`; sync preserves current and cleared assignments. | Extend `internal/components/sdd/*_test.go` and `internal/cli/sync_test.go` with temp homes. |
| TUI/RDD | Configure Models shows custom config provider plus runtime providers. | Bubbletea model-picker tests; add a black-box journey only if the bench can truthfully drive this flow. |

RDD operator flows to prove: Configure Models JSONC custom provider, install/inject JSONC target, sync-present, sync-cleared, LM Studio direct URL, LM Studio fallback URL, and excluded-file diff guard.

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A: no executable-file classification. | No classifier change. | None. |
| Git repository selection | N/A: no VCS automation. | Resolver uses config paths, not git commands. | None. |
| Commit state | N/A: no commit operation. | No index semantics. | None. |
| Push state | N/A: no push operation. | No ref resolution. | None. |
| PR commands | N/A: no PR automation. | No command composition. | None. |

## Migration / Rollout

No migration required. Existing Gentle AI state remains readable; current OpenCode config presence wins at install/sync time. Line-budget risk is medium under the 400-line review budget; with `auto-chain`, split tasks if the parser/merge plus sync/inject tests forecast above budget.

## Open Questions

None.
