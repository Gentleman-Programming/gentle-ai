# Delta for adapter

## MODIFIED Requirements

### Requirement: Functional config.toml Generation

`BootstrapTemplate` MUST write a TOML config file with `merge_all_available_skills = true` and a `[permissions]` section containing safe auto-approve defaults when the config file does not already exist. For v0.11+, `BootstrapTemplate` MUST also generate `kimi.plugin.json` via `InstallPlugin` after writing config.toml/KIMI.md/AGENTS.md. `resolveConfigTOMLContent` MUST include a `[[hooks]]` block with `sessionStart` referencing `sdd-init`, and MUST include `extra_skill_dirs` entries for cross-tool skill discovery.
(Previously: only wrote config.toml and KIMI.md skeleton; no hooks or extra_skill_dirs)

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
- THEN `InstallPlugin` is called with the adapter's resolved homeDir and the Gentle AI version constant
- AND `kimi.plugin.json` is created at the plugin manifest path

#### Scenario: config.toml includes sessionStart hook

- GIVEN a fresh install with no existing `config.toml`
- WHEN `BootstrapTemplate` writes `config.toml`
- THEN the file contains `[[hooks]]` section
- AND the hooks block contains `sessionStart` referencing `sdd-init`

#### Scenario: config.toml includes extra_skill_dirs

- GIVEN a fresh install with no existing `config.toml`
- WHEN `BootstrapTemplate` writes `config.toml`
- THEN the file contains `extra_skill_dirs` array
- AND the array includes `~/.config/agents/skills`
- AND the array includes `~/.agents/skills`

---

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

### Requirement: Version Constant Accessibility

The adapter layer MUST be able to resolve the installed Gentle AI version from a single source of truth. A `GentleAI` version constant MUST be defined in `internal/versions/versions.go` and imported by the adapter when calling `InstallPlugin`.
(Previously: no version constant exposed to adapter)

#### Scenario: Version constant exists and is importable

- GIVEN the `versions` package is compiled
- WHEN the adapter imports `versions.GentleAI`
- THEN it resolves to a non-empty string (e.g. `"0.1.0"`)

#### Scenario: InstallPlugin receives version from constant

- GIVEN `BootstrapTemplate` calls `InstallPlugin`
- WHEN `InstallPlugin(homeDir, version)` is invoked
- THEN `version` matches `versions.GentleAI`

---

## ADDED Requirements

### Requirement: Kimi Skill-Registry Automation

`installSkillRegistryAutomation` MUST handle `model.AgentKimi` by writing a `sessionStart` hook to `config.toml` that runs `gentle-ai skill-registry refresh`. This requirement covers Gap 5 from the proposal.
(Previously: `installSkillRegistryAutomation` only handled Codex and Claude)

#### Scenario: Kimi agent triggers skill-registry hook

- GIVEN the SDD inject component runs for a Kimi adapter
- WHEN `installSkillRegistryAutomation` is called with `model.AgentKimi`
- THEN a `sessionStart` hook is written to `config.toml`
- AND the hook command is `gentle-ai skill-registry refresh`

#### Scenario: Non-Kimi agents are unaffected

- GIVEN the SDD inject component runs for a Codex or Claude adapter
- WHEN `installSkillRegistryAutomation` is called
- THEN existing behavior for those agents is unchanged

#### Scenario: Hook is not duplicated on re-run

- GIVEN `config.toml` already contains the skill-registry refresh hook
- WHEN `installSkillRegistryAutomation` is called again for Kimi
- THEN the hook section is not duplicated
