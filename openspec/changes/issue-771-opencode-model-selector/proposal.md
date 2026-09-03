# Proposal: OpenCode Model Selector Effective Config

## Intent

Fix approved issue #771: Configure Models, install/inject, and sync must honor the user's effective OpenCode config, including `opencode.jsonc`, custom providers/models, and current or explicitly cleared assignments.

## Scope

### In Scope
- Discover and parse supported OpenCode config files with JSONC tolerance and precedence.
- Surface custom providers/models while preserving `opencode models --verbose` runtime catalog discovery.
- Preserve assignments during install/sync using presence-aware reads so stale Gentle AI state cannot overwrite current or cleared OpenCode assignments.
- Preserve merged #1917 LM Studio URL behavior: direct `url` takes precedence; `options.baseURL` is fallback when direct `url` is absent.

### Out of Scope
- Using old branch `fix/custom-opencode-model-selector` or PR #1280 branch bytes as implementation source.
- Touching `internal/assets/opencode/plugins/review-result-artifacts.ts` or `internal/assets/review_plugin_recovery_test.go`.
- Absorbing #934/#2288 tool-call warning UX or #1015 SQLite/runtime discovery.

## Capabilities

### New Capabilities
- `opencode-model-selector`: Effective OpenCode config handling for model selection, install/sync preservation, and custom provider/model visibility.

### Modified Capabilities
- None.

## Approach

Add a focused OpenCode config boundary that resolves effective settings, parses JSON/JSONC provider and assignment data, records assignment presence, and merges configured providers with the runtime catalog. Wire it into Configure Models, install/inject, and sync without touching unrelated plugin/recovery code.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/opencode/` | Modified | Config discovery/parsing, provider URL metadata, catalog merge. |
| `internal/tui/` | Modified | Configure Models receives custom providers/models. |
| `internal/components/sdd/` | Modified | Assignment reads and injection honor effective JSONC config. |
| `internal/cli/sync.go` | Modified | Sync preserves current and cleared assignments. |

## RDD Evidence Gates

- Authority: issue #771 only; PR #1280 is closed/stale evidence only.
- Prove Configure Models, install/inject, sync-present, sync-cleared, and LM Studio direct/fallback URL flows.
- Keep authored changed lines within 400 or split through auto-chain before apply.

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Config precedence differs across flows | Medium | Centralize resolution and test each flow. |
| Cleared assignments are restored from stale Gentle AI state | Medium | Model assignment presence explicitly. |
| LM Studio URL regression | Low | Add direct `url` and `options.baseURL` fallback tests. |

## Rollback Plan

Revert this change's config boundary, selector wiring, assignment preservation, and sync/inject updates. No plugin/recovery files are in the rollback boundary.

## Dependencies

- Current `main` behavior and maintainer comments on PR #1280.
- Existing OpenCode runtime catalog command: `opencode models --verbose`.

## Success Criteria

- [ ] Custom providers/models from `opencode.jsonc` appear in Configure Models.
- [ ] Install/inject and sync preserve current or explicitly cleared assignments.
- [ ] LM Studio direct URL and `options.baseURL` fallback behavior remain covered.
