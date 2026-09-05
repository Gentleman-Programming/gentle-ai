# Kimi Adapter v0.11+ Integration Specification

## Purpose

Defines requirements for comprehensive Kimi Code v0.11+ adapter integration: env-based config override, functional config.toml generation, complete skill directory discovery, and permission defaults.

---

## Requirements

### Requirement: KIMI_CODE_HOME Environment Override

`resolveConfigDir` MUST check the `KIMI_CODE_HOME` environment variable before falling back to `~/.kimi-code` (v0.11+) or `~/.kimi` (legacy). The standalone `ConfigPath` function MUST apply the same priority.

#### Scenario: KIMI_CODE_HOME set to valid directory

- GIVEN `KIMI_CODE_HOME=/custom/kimi` and `/custom/kimi` exists as a directory
- WHEN `resolveConfigDir(homeDir)` is called
- THEN the returned path is `/custom/kimi`

#### Scenario: KIMI_CODE_HOME set to non-existent path

- GIVEN `KIMI_CODE_HOME=/invalid/path` and `/invalid/path` does not exist
- WHEN `resolveConfigDir(homeDir)` is called
- THEN the method falls back to `~/.kimi-code` or `~/.kimi` per standard resolution

#### Scenario: KIMI_CODE_HOME unset

- GIVEN `KIMI_CODE_HOME` is not set
- WHEN `resolveConfigDir(homeDir)` is called
- THEN standard v0.11+ / legacy fallback applies

---

### Requirement: Functional config.toml Generation

`BootstrapTemplate` MUST write a TOML config file with `merge_all_available_skills = true` and a `[permissions]` section containing safe auto-approve defaults when the config file does not already exist.

#### Scenario: Fresh install writes functional config.toml

- GIVEN no `config.toml` exists in the resolved config directory
- WHEN `BootstrapTemplate(homeDir)` is called
- THEN `config.toml` contains `merge_all_available_skills = true`
- AND it contains a `[permissions]` block with `auto_approve_file_read = true`
- AND it contains `auto_approve_file_write = true`
- AND it contains `require_approval_shell = true`

#### Scenario: Existing config.toml is not overwritten

- GIVEN `config.toml` already exists with user content
- WHEN `BootstrapTemplate(homeDir)` is called
- THEN `config.toml` content is unchanged

---

### Requirement: Complete Skills Directory Discovery

The adapter MUST expose an `AllSkillsDirs(homeDir) []string` method returning all directories Kimi Code v0.11+ scans for skills: `~/.kimi-code/skills`, `~/.agents/skills`, and `~/.config/agents/skills`.

#### Scenario: v0.11+ returns three skill directories

- GIVEN Kimi Code v0.11+ is detected (`~/.kimi-code` exists)
- WHEN `AllSkillsDirs(homeDir)` is called
- THEN it returns exactly three paths
- AND the first path is `{homeDir}/.kimi-code/skills`
- AND the second path is `{homeDir}/.agents/skills`
- AND the third path is `{homeDir}/.config/agents/skills`

#### Scenario: Legacy returns only shared skill directory

- GIVEN legacy Kimi is detected (no `~/.kimi-code`)
- WHEN `AllSkillsDirs(homeDir)` is called
- THEN it returns `{homeDir}/.config/agents/skills` as the single entry

---

### Requirement: Permission Rule Defaults

The generated `config.toml` MUST include a `[permissions]` section that auto-approves safe operations (file reads, file writes) and requires manual approval for shell and network operations.

#### Scenario: Permission block present in generated config

- GIVEN a fresh install with no existing `config.toml`
- WHEN `BootstrapTemplate` writes `config.toml`
- THEN the file contains `[permissions]` section
- AND `auto_approve_file_read = true`
- AND `auto_approve_file_write = true`
- AND `require_approval_shell = true`
- AND `require_approval_network = true`

---

### Requirement: KIMI_CODE_HOME Propagation to Standalone ConfigPath

The standalone `ConfigPath(homeDir string)` function MUST apply the same `KIMI_CODE_HOME` env override logic as the adapter method.

#### Scenario: ConfigPath respects env override

- GIVEN `KIMI_CODE_HOME=/custom/kimi` and `/custom/kimi` exists
- WHEN `ConfigPath(homeDir)` is called
- THEN it returns `/custom/kimi`

#### Scenario: ConfigPath falls back when env invalid

- GIVEN `KIMI_CODE_HOME` is empty
- WHEN `ConfigPath(homeDir)` is called
- THEN it returns the standard `~/.kimi-code` or `~/.kimi` path
