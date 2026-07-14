# Tasks: Kimi Code v0.11+ Comprehensive Integration

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 230–280 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Full Kimi v0.11+ integration | PR 1 | adapter + permissions + tests; under 400 lines |

---

## Phase 1: Foundation — Permissions Bypass + Env Override

- [x] 1.1 **[RED]** Write failing test `TestPermissionsInject_KimiReturnsNil` in `internal/components/permissions/inject_test.go`: call `agentOverlay(model.AgentKimi)` — assert returns `nil`
- [x] 1.2 **[GREEN]** Add `case model.AgentKimi: return nil` to `agentOverlay()` in `internal/components/permissions/inject.go`. **Test**: `go test ./internal/components/permissions/...`
- [x] 1.3 **[RED]** Write failing test `TestAdapter_KIMI_CODE_HOME_Override` in `internal/agents/kimi/adapter_test.go`: `t.Setenv("KIMI_CODE_HOME", tmpCustomDir)` + create tmp dir; assert `resolveConfigDir(homeDir)` returns custom dir
- [x] 1.4 **[GREEN]** Modify `resolveConfigDir()`: check `os.Getenv("KIMI_CODE_HOME")` first — if non-empty AND dir exists, return it. **Test**: `go test ./internal/agents/kimi/...`
- [x] 1.5 **[RED]** Write failing test `TestConfigPath_KIMI_CODE_HOME`: `t.Setenv` + assert `ConfigPath(homeDir)` returns custom dir
- [x] 1.6 **[GREEN]** Modify standalone `ConfigPath()`: add same `KIMI_CODE_HOME` env check before `~/.kimi-code` fallback. **Test**: `go test ./internal/agents/kimi/...`
- [x] 1.7 **[RED]** Write failing test `TestAdapter_KIMI_CODE_HOME_FallbackOnInvalid`: set env to non-existent path; assert falls back to `~/.kimi-code`
- [x] 1.8 **[GREEN]** Ensure env check validates directory existence (os.Stat); fall back to standard resolution if missing. **Test**: `go test ./internal/agents/kimi/...`

## Phase 2: Core Methods — AllSkillsDirs + AGENTSMDPath

- [x] 2.1 **[RED]** Write failing test `TestAdapter_AllSkillsDirs_V11Plus`: mock `pathExists` true for `~/.kimi-code`; assert returns exactly 3 paths: `{home}/.kimi-code/skills`, `{home}/.agents/skills`, `{home}/.config/agents/skills`
- [x] 2.2 **[GREEN]** Add `AllSkillsDirs(homeDir string) []string` to Adapter: check `isV11Plus` → return 3-element slice or legacy single-element slice. **Test**: `go test ./internal/agents/kimi/...`
- [x] 2.3 **[RED]** Write failing test `TestAdapter_AllSkillsDirs_Legacy`: no `~/.kimi-code`; assert returns `[{home}/.config/agents/skills]`
- [x] 2.4 **[GREEN]** Already covered by 2.2 implementation. **Test**: `go test ./internal/agents/kimi/...`
- [x] 2.5 **[RED]** Write failing test `TestAdapter_AGENTSMDPath_V11Plus`: assert returns `{resolvedConfigDir}/AGENTS.md`
- [x] 2.6 **[GREEN]** Add `AGENTSMDPath(homeDir string) string`: return `filepath.Join(a.resolveConfigDir(homeDir), "AGENTS.md")`. **Test**: `go test ./internal/agents/kimi/...`

## Phase 3: Config.toml Generation

- [x] 3.1 **[RED]** Write failing test `TestAdapter_BootstrapTemplate_WritesConfigTOML`: create temp home with no config.toml; call `BootstrapTemplate(homeDir)`; read config.toml; assert contains `merge_all_available_skills = true`
- [x] 3.2 **[RED]** Extend test to assert: contains `[[permission.rules]]` with `decision = "allow"` / `pattern = "Read"`, and `decision = "ask"` / `pattern = "Bash"`
- [x] 3.3 **[GREEN]** Add `resolveConfigTOMLContent() string` helper returning the full TOML string with `merge_all_available_skills`, `[[permission.rules]]` for Read/Grep/Glob/Write/Edit/Agent (allow) and Bash (ask). Update `BootstrapTemplate` to write this instead of `# Kimi Code Config`. **Test**: `go test ./internal/agents/kimi/...`
- [x] 3.4 **[RED]** Write failing test `TestAdapter_BootstrapTemplate_ExistingConfigNotOverwritten`: write custom config.toml first; run bootstrap; assert content unchanged
- [x] 3.5 **[GREEN]** Verify existing `os.Stat` guard in `BootstrapTemplate` already prevents overwrite. **Test**: `go test ./internal/agents/kimi/...`

## Phase 4: AGENTS.md Injection

- [x] 4.1 **[RED]** Write failing test `TestAdapter_ResolveProjectAGENTSMD_Found`: create temp dir with `AGENTS.md` containing `# Rules\nBe nice`; assert `resolveProjectAGENTSMD(homeDir)` returns content
- [x] 4.2 **[GREEN]** Add `resolveProjectAGENTSMD(homeDir string) string`: search upward from `homeDir` for `AGENTS.md`; read and return content if found, else return `<!-- No project AGENTS.md found -->`. **Test**: `go test ./internal/agents/kimi/...`
- [x] 4.3 **[RED]** Write failing test `TestAdapter_ResolveProjectAGENTSMD_NotFound`: no AGENTS.md; assert returns placeholder comment
- [x] 4.4 **[GREEN]** Covered by 4.2 implementation. **Test**: `go test ./internal/agents/kimi/...`
- [x] 4.5 **[RED]** Write failing test `TestAdapter_SanitizeJinjaSyntax`: content with `{{ variable }}` and `{% block %}` → assert escaped to `{{{{ variable }}}}` and `{{% block %}}`
- [x] 4.6 **[GREEN]** Add `sanitizeJinjaSyntax(content string) string`: escape `{{` → `{{{{`, `}}` → `}}}}`, `{%` → `{{%`, `%}` → `%}}`. **Test**: `go test ./internal/agents/kimi/...`
- [x] 4.7 **[RED]** Write failing test `TestAdapter_BootstrapTemplate_AGENTSMD_Injection`: create temp home + project `AGENTS.md`; run bootstrap; read KIMI.md; assert `${KIMI_AGENTS_MD}` replaced with project content
- [x] 4.8 **[GREEN]** Update `BootstrapTemplate`: after writing KIMI.md, call `resolveProjectAGENTSMD` → sanitize → `strings.ReplaceAll(kimiMD, "${KIMI_AGENTS_MD}", sanitized)`. Also write resolved content to `AGENTSMDPath`. **Test**: `go test ./internal/agents/kimi/...`
- [x] 4.9 **[RED]** Write failing test `TestAdapter_BootstrapTemplate_AGENTSMD_NoProject`: no project AGENTS.md; assert KIMI.md contains placeholder comment
- [x] 4.10 **[GREEN]** Covered by 4.8 implementation (placeholder path). **Test**: `go test ./internal/agents/kimi/...`

## Phase 5: Verification & Cleanup

- [x] 5.1 Run full test suite: `go test ./...` — all tests pass
- [x] 5.2 Run `go vet ./...` — no issues
- [x] 5.3 Verify `TestAdapter_ConfigPaths` (legacy) still passes — no regression
- [x] 5.4 Verify `TestAdapter_ConfigPaths_V11Plus` still passes — no regression
- [x] 5.5 Review all new tests cover spec scenarios: adapter spec (10 scenarios) + agents-md-injection spec (8 scenarios)
