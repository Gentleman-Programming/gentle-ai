# Proposal: Integrate RTK (Rust Token Killer) into Gentleman AI Ecosystem

## Intent

AI coding agents consume significant tokens on repetitive shell output (git status, test results, ls, cat). RTK is a CLI proxy that filters/compresses command output before it reaches the LLM, reducing token consumption by 60-90%. Integrating RTK fills the **optimization gap** in the Gentleman ecosystem — GGA handles code *quality*, Engram handles *memory*, SDD handles *workflow*, but nothing currently handles *cost efficiency*.

## Scope

### In Scope
- Add RTK as a new ecosystem component (`rtk`) in the component catalog
- Install RTK binary via Homebrew or curl (same pattern as Engram/GGA)
- Configure RTK hooks for all supported agents: Claude Code, OpenCode, Cursor, Gemini CLI, Codex, Windsurf
- Add RTK to presets (Full Gentleman, Ecosystem Only) as opt-in component
- Update `docs/components.md` with RTK entry and behavior description
- Add agent-specific configuration in `internal/assets/` for each agent's hook setup

### Out of Scope
- Custom RTK command profiles per project (future work)
- RTK telemetry dashboard integration
- Benchmarking token savings post-install

## Approach

Leverage RTK's existing `rtk init` commands which already support per-agent hook installation. The Gentleman installer:
1. Installs the RTK binary (Homebrew preferred, curl fallback)
2. Runs `rtk init -g` (Claude Code), `rtk init -g --opencode`, `rtk init -g --agent cursor`, etc. — one per selected agent
3. No manual hook writing needed — RTK handles its own hook configuration

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `docs/components.md` | Modified | Add RTK row to component table |
| `docs/usage.md` | Modified | Add RTK flags/usage section |
| `internal/assets/` | New dirs | `rtk/` asset dir with install scripts per platform |
| `cmd/gentle-ai/` | Modified | Wire RTK into component selection and install flow |
| `openspec/specs/rtk/spec.md` | New | RTK component specification |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| RTK project becomes unmaintained (15k stars, active — unlikely) | Low | Graceful fallback: skip hook if `rtk init` fails, agent continues without optimization |
| RTK token estimates don't match real-world savings | Medium | Document as "estimated savings" — never promise exact percentages |
| Name collision with Rust Type Kit on crates.io | Medium | Use `cargo install --git` in docs, mention collision warning |
| RTK updates break hook compatibility | Low | Version pin minimum RTK version, add `rtk --version` health check |

## Rollback Plan

1. Run `rtk init -g --uninstall` for each configured agent (removes hooks, RTK.md)
2. Remove RTK from Gentleman preset selections
3. Remove RTK component from `docs/components.md`
4. Agent continues to work without RTK — no config corruption

## Dependencies

- RTK binary available on GitHub Releases (already proven: v0.34.1, 70 releases)
- Homebrew tap available (`brew install rtk`)
- Agent hook APIs stable (Claude Code PreToolUse, OpenCode tool.execute.before, etc.)

## Success Criteria

- [ ] `gentle-ai --component rtk` installs RTK and configures hooks for selected agents
- [ ] RTK hooks active on all 8 supported agents (Claude Code, OpenCode, Cursor, Gemini CLI, Codex, Windsurf, VSCode Copilot, Cline)
- [ ] `rtk gain` shows token savings after one development session
- [ ] Uninstall removes hooks cleanly without affecting agent configs
- [ ] Documentation updated with RTK section in components and usage docs
