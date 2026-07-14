# Proposal: Kimi Code v0.11+ Comprehensive Integration

## Intent

The current v0.11+ adapter only solves path detection — it redirects `~/.kimi` to `~/.kimi-code` but leaves four critical gaps that degrade the user experience for v0.11+ installations:

1. **KIMI_CODE_HOME ignored** — Users who set `KIMI_CODE_HOME` env var get wrong paths
2. **config.toml is empty placeholder** — `merge_all_available_skills` and permission rules never written
3. **AGENTS.md never resolved** — `${KIMI_AGENTS_MD}` in KIMI.md is a dead variable; project instructions are never injected
4. **Skills scan dirs incomplete** — v0.11+ also scans `~/.agents/skills/` (cross-tool shared dir), adapter doesn't expose it
5. **Permission rules absent** — Users must manually approve every tool call; no auto-approve for safe operations

## Scope

### In Scope
- **Env override**: `resolveConfigDir` reads `KIMI_CODE_HOME` before falling back to `~/.kimi-code` / `~/.kimi`
- **config.toml content**: `BootstrapTemplate` writes real config with `merge_all_available_skills = true` and permission rules
- **AGENTS.md injection**: Add `AGENTSMDPath(homeDir)` method; populate `${KIMI_AGENTS_MD}` during bootstrap via a Jinja variable resolver or template post-processing
- **Skills scan dirs**: `AllSkillsDirs(homeDir)` returns all three scan locations for v0.11+
- **Permission rules**: config.toml includes `[permissions]` block with safe defaults (auto-approve file reads, manual shell)

### Out of Scope
- Migration of user data from `~/.kimi` to `~/.kimi-code`
- Plugin distribution mechanism (future PR)
- Changes to other agents

## Capabilities

### New Capabilities
- `kimi-agents-md-injection`: Resolves `${KIMI_AGENTS_MD}` in KIMI.md by reading project-level AGENTS.md content and injecting it during bootstrap

### Modified Capabilities
- `kimi-adapter`: `resolveConfigDir` reads `KIMI_CODE_HOME` env var; `BootstrapTemplate` writes meaningful config.toml; new `AllSkillsDirs` method; new `AGENTSMDPath` method

## Approach

1. **Env override in resolveConfigDir** — Check `os.Getenv("KIMI_CODE_HOME")` first; if non-empty and directory exists, use it. Then check `~/.kimi-code`. Fallback `~/.kimi`. Update `ConfigPath` standalone function identically.

2. **Real config.toml** — `BootstrapTemplate` writes a TOML with `merge_all_available_skills = true`, `[permissions]` section (auto-approve file reads, file writes; manual shell/exec), and a `[skills]` section pointing to the skills directory.

3. **AGENTS.md resolution** — Add `AGENTSMDPath(homeDir) string` to the adapter interface implementation. During `BootstrapTemplate`, after writing KIMI.md, resolve `${KIMI_AGENTS_MD}` by reading the project-root AGENTS.md if it exists, or write a placeholder comment. Use a simple string replacement on the rendered KIMI.md content.

4. **Skills directories** — `AllSkillsDirs(homeDir)` returns `[]string{~/.kimi-code/skills, ~/.agents/skills, ~/.config/agents/skills}` for v0.11+, enabling complete skill discovery.

5. **Permission defaults** — config.toml includes:
   ```toml
   [permissions]
   auto_approve_file_read = true
   auto_approve_file_write = true
   require_approval_shell = true
   require_approval_network = true
   ```

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/agents/kimi/adapter.go` | Modified | `resolveConfigDir` reads `KIMI_CODE_HOME`; `BootstrapTemplate` writes real config; add `AllSkillsDirs`, `AGENTSMDPath`; add `agentsMDContent` field for project AGENTS.md |
| `internal/agents/kimi/adapter_test.go` | Modified | Tests for env override, config.toml content, AGENTS.md resolution, AllSkillsDirs |
| `internal/assets/kimi/config.toml.tmpl` | New | TOML template for v0.11+ config with permissions and skills config |
| `internal/components/sdd/inject.go` | Modified | Ensure Kimi's SDD injection populates AGENTS.md path |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| KIMI_CODE_HOME set to invalid path | Low | Validate directory existence before using; fall back to default |
| config.toml format changes across kimi-code versions | Med | Pin to documented v0.11+ format; version-specific templates in future |
| AGENTS.md content includes Jinja syntax that conflicts with KIMI.md rendering | Low | Escape or sanitize project AGENTS.md before injection |
| Permission rules too permissive for some users | Low | Document how to override in user's own config.toml |

## Rollback Plan

1. Revert adapter changes — `resolveConfigDir` goes back to hardcoded paths
2. `BootstrapTemplate` reverts to writing empty `# Kimi Code Config`
3. Remove `AllSkillsDirs`, `AGENTSMDPath` methods
4. `git revert` the feature branch
5. User runs `gentle-ai sync` to restore clean state

## Dependencies

- kimi-code v0.11+ directory layout and config.toml schema
- Existing `agents.Adapter` interface — no modifications required
- Existing `filemerge.WriteFileAtomic` pattern — reused for config writing

## Success Criteria

- [ ] `KIMI_CODE_HOME=/custom/path` causes adapter to use `/custom/path` instead of `~/.kimi-code`
- [ ] `config.toml` contains `merge_all_available_skills = true` after install
- [ ] `config.toml` contains `[permissions]` section with safe defaults
- [ ] `KIMI_AGENTS_MD` in KIMI.md is resolved to project AGENTS.md content (or placeholder)
- [ ] `AllSkillsDirs()` returns three directories for v0.11+
- [ ] All existing tests pass; new tests cover env override, config content, AGENTS.md injection
