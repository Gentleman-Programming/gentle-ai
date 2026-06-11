# Proposal: Level-Neutral Persona Parity

## Intent

Neutral should provide Gentleman-equivalent mentor behavior without Rioplatense/regional voice. Today neutral avoids most direct Gentleman voice leakage, but has a weaker behavior contract and legacy sync can reactivate Gentleman when persona state is missing or invalid.

## Scope

### In Scope
- Add neutral parity across agent persona assets: brevity, one-question-at-a-time, no option-menu default, verification-first, and shell/tool behavior expectations.
- Add neutral output-style behavior for Claude and ensure Kimi's injected `output-style.md` is meaningful when neutral is selected.
- Fix sync defaults so missing/invalid persisted persona does not silently fall back to Gentleman.

### Out of Scope
- Changing Gentleman regional voice or mentor contract.
- Reworking OpenCode/Kilocode residual `agent.gentleman` behavior unless needed to prevent neutral regressions.
- Code/test implementation in this proposal phase.

## Capabilities

### New Capabilities
- `persona-behavior-contract`: Cross-agent parity, neutral voice constraints, output-style behavior, and safe persona fallback semantics.

### Modified Capabilities
- None. Existing specs (`antigravity-support`, `gga`, `sdd-orchestrator-assets`) do not define persona or sync fallback behavior.

## Approach

Treat neutral as a level-neutral variant of the same behavior contract, not an unstyled/default assistant. Update generic and agent-specific assets, populate neutral output-style surfaces where supported or injected, and change sync fallback behavior to preserve neutral/default safety instead of reviving Gentleman.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/components/persona/` | Modified | Persona injection, cleanup, neutral asset behavior. |
| `internal/cli/sync.go` | Modified | `applyResolvedPersona` fallback when persisted persona is missing/invalid. |
| `internal/assets/**/persona*`, `internal/assets/**/output-style*` | Modified/New | Cross-agent neutral contract and output-style parity. |
| OpenCode/Kilocode sync assets | Investigate | Confirm residual `agent.gentleman` behavior is intentional and not a neutral regression. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Neutral gains regional/Gentleman voice | Medium | Add neutral voice constraints and regression coverage. |
| Legacy users lose expected Gentleman default | Medium | Define fallback semantics clearly; preserve explicit Gentleman selections. |
| Agent output-style differences cause uneven behavior | Medium | Cover Claude, Kimi injection, and generic consumers separately. |

## Rollback Plan

Revert persona asset changes and restore previous `applyResolvedPersona` fallback. This is prompt/config behavior, so rollback is file-level and requires no data migration.

## Dependencies

- Issue #789 is open and not `status:approved`; proceed because the repository owner requested SDD planning.

## Success Criteria

- [ ] Neutral receives Gentleman-equivalent behavior rules without regional voice.
- [ ] Claude neutral has a non-default output-style contract.
- [ ] Kimi neutral injected `output-style.md` is not empty.
- [ ] Missing/invalid persisted persona no longer silently reactivates Gentleman.
- [ ] Explicit Gentleman selection continues to work.
