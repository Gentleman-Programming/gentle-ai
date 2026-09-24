# Proposal: Managed Block Footprint Diagnostic for `gentle-ai doctor`

## Intent

`gentle-ai doctor` checks binary/env health but is blind to the size/token cost of the `gentle-ai:*` managed blocks (persona, engram-protocol, sdd-orchestrator, strict-tdd-mode, codegraph-guidance, etc.) injected into each agent's instruction file. Footprint-reduction work (#801, #259) currently needs manual `wc -c` + cross-referencing. This adds an objective, read-only footprint measurement inside doctor. Closes #1018 (status:approved).

## Scope

### In Scope
- Generic multi-section scanner in `internal/components/filemerge` that finds every `<!-- gentle-ai:X -->...<!-- /gentle-ai:X -->` pair with its ID, raw content, and char/line span.
- Always-on "Managed Block Footprint" check in `RunDoctor`: total blocks, agents covered, rough total token estimate; WARN/FAIL only on structural breakage (orphan/unclosed marker).
- Opt-in `--footprint` flag (doctor's first flag) rendering the full per-agent/per-block breakdown (chars, lines, rough token estimate).
- Small pure token-estimate helper (chars/4 heuristic).
- Table-driven tests for scanner, check, and flag; docs updated with the deliverable.

### Out of Scope
- JSON settings-overlay footprint (not marker-based) — only markdown `SystemPromptFile` targets.
- Precise tokenizer / model-specific counts (estimate stays "rough").
- Auto-remediation or block shrinking (measurement only).
- Standalone `gentle-ai footprint` subcommand (rejected by the issue).

## Capabilities

### New Capabilities
- `doctor-footprint`: managed-block footprint measurement in doctor (compact always-on summary + `--footprint` detailed breakdown, structural-breakage warnings).

### Modified Capabilities
- None (no existing `openspec/specs/`).

## Approach

Exploration Approach 2. Add a generic section scanner (reusable for future marker validation), then a `checkManagedBlockFootprint` wired into `RunDoctor`. Resolve per-agent instruction paths via `agents.NewDefaultRegistry()` + `Adapter.SystemPromptFile(homeDir)` and configured agents via `state.Read(homeDir).InstalledAgents` — reusing existing seams instead of repeating `checkStateJSON`'s hardcoded `agentConfigDir` switch. Add `--footprint` via `flag.NewFlagSet(...)` following `sync`/`install` precedent; `RunDoctor(ctx, w)` becomes `RunDoctor(ctx, args, w)`, updating the `app.go` call site in the same work unit.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/components/filemerge/section.go` (or new `section_scan.go`) | New | Generic multi-section marker scanner |
| `internal/cli/doctor.go` | Modified | New check, flag parsing, extended rendering, signature change |
| `internal/cli/doctor_test.go` | Modified | Table-driven tests |
| `internal/app/app.go` (~230-231) | Modified | Pass `args` to `RunDoctor` |
| `internal/agents`, `internal/state` | Reused | Path/agent resolution |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Scanner rots if hardcoded to known IDs | Med | Scan generically for any `gentle-ai:X` marker pair |
| Signature change breaks callers/tests | Med | Land `app.go` + tests in same work unit |
| Token estimate misread as exact | Low | Label output "rough estimate" |

## Rollback Plan

Single additive PR (`feat/doctor-footprint`). Revert the commit to fully restore prior doctor behavior; no state, schema, or file-format changes, so revert is clean and side-effect-free.

## Dependencies

- None external. Note: `openspec/config.yaml` sets `strict_tdd: true` (contradicts the task brief) — confirm TDD expectation at tasks/apply.

## Success Criteria

- [ ] `gentle-ai doctor` shows a compact footprint line by default.
- [ ] `gentle-ai doctor --footprint` shows per-agent/per-block breakdown.
- [ ] Orphan/unclosed marker surfaces as WARN/FAIL.
- [ ] `go test ./...` and `go vet ./...` pass; PR under 400-line budget; body includes `Closes #1018`.
