# Design: Level-Neutral Persona Parity

## Technical Approach

Make `neutral` a first-class twin of `gentleman`: the same mentor/verification/response-length contract, but with neutral professional language and no Rioplatense/regional speech rules. Persona injection remains centralized in `internal/components/persona/inject.go`; assets carry the behavioral contract; sync resolves unsafe or missing persisted state to neutral rather than reviving Gentleman.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Neutral output-style twin | Add neutral output-style assets and activate them where Gentle AI manages output styles: Claude gets `output-styles/neutral.md` plus `settings.json` `outputStyle: "Neutral"`; Kimi gets non-empty generated `.kimi/output-style.md` from a new neutral asset. | Leave neutral output style empty; only update persona files. | Empty/Claude-only behavior is the current parity bug. A named twin keeps the same behavior contract on surfaces where output style is the strongest instruction layer. |
| Asset strategy | Update `generic/persona-neutral.md` and `hermes/persona-neutral.md`; add `claude/output-style-neutral.md` and `kimi/output-style-neutral.md`; do not add agent-specific neutral persona files unless a platform needs divergent mechanics. | Duplicate neutral persona for Claude/Kimi/OpenCode/Kiro. | Existing `personaContent` already uses generic neutral for all non-Hermes agents. Duplication would increase drift without adding platform-specific behavior. |
| Sync fallback | `applyResolvedPersona` keeps explicit `selection.Persona`; valid persisted values are honored; missing or invalid persisted persona resolves to `PersonaNeutral`. | Continue missing/invalid fallback to Gentleman; fail sync on invalid state. | Missing/invalid state is not an explicit Gentleman selection. Neutral is safer because it avoids regional voice and surprise persona reactivation while preserving explicit Gentleman installs. |
| OpenCode/Kilocode residuals | Keep Gentleman agent overlay install-only for merge safety, but allow sync-managed non-Gentleman cleanup to remove only `agent.gentleman` while preserving other `agent` children. | Never touch residuals during sync; fully manage the overlay during sync. | Narrow cleanup prevents neutral regressions without reintroducing the SDD `agent` clobbering risk documented in `InjectForSync`. |

## Data Flow

```text
Selection/persona state ──→ applyResolvedPersona ──→ persona.Inject/InjectForSync
                                      │
                                      ├─→ personaContent(agent, neutral) ─→ generic/hermes persona asset
                                      ├─→ Claude output style ────────────→ neutral.md + settings outputStyle
                                      ├─→ Kimi Jinja module ──────────────→ .kimi/output-style.md
                                      └─→ OpenCode/Kilocode cleanup ──────→ remove agent.gentleman only
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/assets/generic/persona-neutral.md` | Modify | Bring neutral rules/behavior to Gentleman parity: short answers, one question, no option-menu default, verification-first, artifact language boundaries. |
| `internal/assets/hermes/persona-neutral.md` | Modify | Same contract as generic, preserving Hermes skill/memory/identity sections. |
| `internal/assets/claude/output-style-neutral.md` | Create | Claude output-style twin named `Neutral`, with behavior parity and neutral language rules. |
| `internal/assets/kimi/output-style-neutral.md` | Create | Kimi module content for neutral instead of the current empty output-style include. |
| `internal/components/persona/inject.go` | Modify later | Select neutral output-style assets, write Claude `neutral.md`, set/clean managed `outputStyle` values, populate Kimi output-style module, and permit sync cleanup of OpenCode/Kilocode `agent.gentleman`. |
| `internal/cli/sync.go` | Modify later | Change missing/invalid persisted persona fallback to `PersonaNeutral`, while preserving explicit selections. |
| `internal/components/persona/*test.go`, `internal/cli/sync_test.go` | Modify later | Update/add regression coverage. |

## Interfaces / Contracts

No public API changes. Internal contract changes:

```go
// Empty explicit selection is resolved from persisted state.
// Valid persisted persona wins; missing/invalid persisted persona defaults neutral.
func applyResolvedPersona(selection *model.Selection, persisted string)
```

Managed Claude output-style names are `Gentleman` and `Neutral`; cleanup must remove only managed values, not arbitrary user output styles.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | Neutral assets contain parity rules and exclude Rioplatense/voseo/regional markers. | Extend `internal/components/persona/persona_language_contract_test.go` and CLI/TUI language contract tests. |
| Unit | Claude neutral writes `neutral.md`, sets `outputStyle: "Neutral"`, removes stale Gentleman style/settings, and is idempotent. | Update `internal/components/persona/inject_test.go`; replace the old “neutral does not write output style” expectation. |
| Unit | Kimi neutral writes non-empty `.kimi/output-style.md` and still bootstraps `KIMI.md`. | Add Kimi neutral injection test beside existing Gentleman test. |
| Unit | Missing and invalid persisted persona resolve to neutral; explicit `PersonaGentleman`, `PersonaNeutral`, and `PersonaCustom` still win. | Update `internal/cli/sync_test.go` fallback cases. |
| E2E | Sync/install does not regress cross-agent asset generation. | Existing `go test ./...`; full Docker E2E only if sync/install behavior appears platform-sensitive. |

## Migration / Rollout

No data migration required. On next sync, old state without persona or with invalid persona will resolve to neutral; explicit persisted `gentleman` remains Gentleman. Rollback is file-level: revert assets and fallback logic.

## Open Questions

None.
