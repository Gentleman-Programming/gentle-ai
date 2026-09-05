# Delta for Kimi Adapter (Modified Capabilities)

## MODIFIED Requirements

### Requirement: Complete Skills Directory Discovery

The adapter MUST expose an `AllSkillsDirs(homeDir) []string` method returning all directories Kimi Code v0.11+ scans for skills. For v0.11+ with plugin support, the first entry MUST be the plugin skills subdirectory (`~/.kimi-code/plugins/managed/gentle-ai/skills`), followed by the shared directories.
(Previously: returned `~/.kimi-code/skills`, `~/.agents/skills`, `~/.config/agents/skills`)

#### Scenario: v0.11+ returns plugin dir plus shared dirs

- GIVEN Kimi Code v0.11+ is detected (`~/.kimi-code` exists)
- WHEN `AllSkillsDirs(homeDir)` is called
- THEN it returns four paths
- AND the first path is `{homeDir}/.kimi-code/plugins/managed/gentle-ai/skills`
- AND the second path is `{homeDir}/.kimi-code/skills`
- AND the third path is `{homeDir}/.agents/skills`
- AND the fourth path is `{homeDir}/.config/agents/skills`

#### Scenario: Legacy returns only shared skill directory

- GIVEN legacy Kimi is detected (no `~/.kimi-code`)
- WHEN `AllSkillsDirs(homeDir)` is called
- THEN it returns `{homeDir}/.config/agents/skills` as the single entry

---

### Requirement: Functional config.toml Generation

`BootstrapTemplate` MUST write a TOML config file with `merge_all_available_skills = true` and a `[permissions]` section containing safe auto-approve defaults when the config file does not already exist. For v0.11+, `BootstrapTemplate` MUST also generate `kimi.plugin.json` via `InstallPlugin`.
(Previously: only wrote config.toml and KIMI.md skeleton)

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

#### Scenario: v0.11+ generates plugin manifest during bootstrap

- GIVEN v0.11+ is detected and no `kimi.plugin.json` exists
- WHEN `BootstrapTemplate(homeDir)` is called
- THEN `kimi.plugin.json` is created at the plugin manifest path
- AND the manifest contains the required name, version, skills, hooks, and mcpServers fields

---

## ADDED Requirements

### Requirement: PluginInstaller Interface Implementation

The Kimi adapter MUST implement the `PluginInstaller` interface, providing `PluginDir`, `PluginManifestPath`, and `InstallPlugin` methods that enable plugin-based skill distribution for v0.11+.

#### Scenario: PluginInstaller methods return correct paths

- GIVEN v0.11+ is detected
- WHEN `PluginDir(homeDir)` is called
- THEN it returns `{homeDir}/.kimi-code/plugins/managed/gentle-ai`
- AND `PluginManifestPath(homeDir)` returns `{homeDir}/.kimi-code/plugins/managed/gentle-ai/kimi.plugin.json`

#### Scenario: InstallPlugin creates directory and writes manifest

- GIVEN v0.11+ is detected and plugin directory does not exist
- WHEN `InstallPlugin(homeDir, "1.0.0")` is called
- THEN the directory `{homeDir}/.kimi-code/plugins/managed/gentle-ai/` is created
- AND `kimi.plugin.json` is written with version `"1.0.0"`

---

### Requirement: SkillsDir Returns Plugin Path for v0.11+

The `SkillsDir` method MUST return the plugin skills subdirectory for v0.11+ installs instead of the flat `~/.kimi-code/skills` path.
(Previously: returned `~/.kimi-code/skills`)

#### Scenario: v0.11+ SkillsDir returns plugin path

- GIVEN v0.11+ is detected
- WHEN `SkillsDir(homeDir)` is called
- THEN it returns `{homeDir}/.kimi-code/plugins/managed/gentle-ai/skills`

#### Scenario: Legacy SkillsDir unchanged

- GIVEN legacy Kimi is detected (no `~/.kimi-code`)
- WHEN `SkillsDir(homeDir)` is called
- THEN it returns `{homeDir}/.config/agents/skills`
