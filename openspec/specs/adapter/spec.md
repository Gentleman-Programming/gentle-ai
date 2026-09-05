# Kimi Code Adapter Integration Specification

## Purpose

Defines requirements for comprehensive current Kimi Code adapter integration: env-based config override, functional config.toml generation, complete skill directory discovery, and permission defaults.

---

## Requirements

### Requirement: KIMI_CODE_HOME Environment Override

`resolveConfigDir` MUST check the `KIMI_CODE_HOME` environment variable first, then `~/.kimi-code` (current kimi-code), then `~/.kimi` (legacy) — the legacy path applies only when that directory exists. When neither directory exists (fresh machine), resolution MUST default to `~/.kimi-code`, because that is the config root of the npm-installed kimi-code CLI that gentle-ai auto-installs. The standalone `ConfigPath` function MUST apply the same priority.

#### Scenario: Fresh machine defaults to kimi-code

- GIVEN neither `~/.kimi-code` nor `~/.kimi` exists and `KIMI_CODE_HOME` is unset
- WHEN `resolveConfigDir(homeDir)` is called
- THEN the returned path is `{homeDir}/.kimi-code`
- AND `usesKimiCodeLayout(homeDir)` returns true

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
- THEN standard current kimi-code / legacy fallback applies

---

### Requirement: Functional config.toml Generation

`BootstrapTemplate` MUST write a TOML config file with `merge_all_available_skills = true` and a `[permissions]` section containing safe auto-approve defaults when the config file does not already exist. For current kimi-code, `BootstrapTemplate` MUST also generate `kimi.plugin.json` via `InstallPlugin` after writing config.toml/AGENTS.md. `resolveConfigTOMLContent` MUST include a `[[hooks]]` block with `sessionStart` referencing `sdd-init`, and MUST include `extra_skill_dirs` entries for cross-tool skill discovery.
(Previously: only wrote config.toml and AGENTS.md skeleton; no hooks or extra_skill_dirs)

#### Scenario: Fresh install writes functional config.toml

- GIVEN no `config.toml` exists in the resolved config directory
- WHEN `BootstrapTemplate(homeDir)` is called
- THEN `config.toml` contains `merge_all_available_skills = true`
- AND it contains `[[permission.rules]]` entries that allow `Read`, `Grep`, `Glob`, `Write`, `Edit`, and `Agent`
- AND it contains an `ask` rule for `Bash`

#### Scenario: Existing config.toml is not overwritten

- GIVEN `config.toml` already exists with user content
- WHEN `BootstrapTemplate(homeDir)` is called
- THEN `config.toml` content is unchanged

#### Scenario: current kimi-code generates plugin manifest during bootstrap

- GIVEN current kimi-code is detected and no `kimi.plugin.json` exists
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

### Requirement: Complete Skills Directory Discovery

The adapter MUST expose an `AllSkillsDirs(homeDir) []string` method returning all directories current Kimi Code scans for skills. For current kimi-code with plugin support, the first entry MUST be the plugin skills subdirectory (`{resolvedConfigDir}/plugins/managed/gentle-ai/skills`), followed by the shared directories.
(Previously: returned `~/.kimi-code/skills`, `~/.agents/skills`, `~/.config/agents/skills`)

#### Scenario: current kimi-code returns plugin dir plus shared dirs

- GIVEN current Kimi Code is detected (`~/.kimi-code` exists or `KIMI_CODE_HOME` is set)
- WHEN `AllSkillsDirs(homeDir)` is called
- THEN it returns four paths
- AND the first path is `{resolvedConfigDir}/plugins/managed/gentle-ai/skills`
- AND the second path is `{resolvedConfigDir}/skills`
- AND the third path is `{homeDir}/.agents/skills`
- AND the fourth path is `{homeDir}/.config/agents/skills`

#### Scenario: Legacy returns only shared skill directory

- GIVEN legacy Kimi is detected (`~/.kimi` exists and `~/.kimi-code` does not)
- WHEN `AllSkillsDirs(homeDir)` is called
- THEN it returns `{homeDir}/.config/agents/skills` as the single entry

---

### Requirement: Permission Rule Defaults

The generated `config.toml` MUST include `[[permission.rules]]` entries that auto-approve safe tools (`Read`, `Grep`, `Glob`, `Write`, `Edit`, `Agent`) and require manual approval for `Bash`. Because Kimi bypasses the permissions component overlay (its permissions live in `config.toml`), the generated config MUST also include `deny` rules for `Read`, `Write`, and `Edit` on credential and secret file patterns equivalent to the deny lists injected for other agents (`.env`, `.env.*`, `.ssh`, `.credentials`, `Library/Keychains`, `.aws/credentials`, `.config/gh/hosts.yml`, `**/*.pem`, `**/*.key`, `**/secrets/*`). Kimi evaluates `deny > ask > allow` regardless of rule order, so the broad allows never override these denies.

#### Scenario: Permission rules present in generated config

- GIVEN a fresh install with no existing `config.toml`
- WHEN `BootstrapTemplate` writes `config.toml`
- THEN the file contains `allow` rules for `Read`, `Grep`, `Glob`, `Write`, `Edit`, and `Agent`
- AND an `ask` rule for `Bash`

#### Scenario: Credential files are denied

- GIVEN a fresh install with no existing `config.toml`
- WHEN `BootstrapTemplate` writes `config.toml`
- THEN the file contains `deny` rules for `Read`, `Write`, and `Edit` on each sensitive file pattern (e.g. `Read(.env)`, `Write(**/*.pem)`, `Edit(.ssh/*)`)

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

---

### Requirement: PluginInstaller Interface Implementation

The Kimi adapter MUST implement the `PluginInstaller` interface, providing `PluginDir`, `PluginManifestPath`, and `InstallPlugin` methods that enable plugin-based skill distribution for current kimi-code.

#### Scenario: PluginInstaller methods return correct paths

- GIVEN current kimi-code is detected (`~/.kimi-code` exists or `KIMI_CODE_HOME` is set)
- WHEN `PluginDir(homeDir)` is called
- THEN it returns `{resolvedConfigDir}/plugins/managed/gentle-ai`
- AND `PluginManifestPath(homeDir)` returns `{resolvedConfigDir}/plugins/managed/gentle-ai/kimi.plugin.json`

#### Scenario: InstallPlugin creates directory and writes manifest

- GIVEN current kimi-code is detected and plugin directory does not exist
- WHEN `InstallPlugin(homeDir, "1.0.0")` is called
- THEN the directory `{resolvedConfigDir}/plugins/managed/gentle-ai/` is created
- AND `kimi.plugin.json` is written with version `"1.0.0"`

---

### Requirement: SkillsDir Returns Plugin Path for current kimi-code

The `SkillsDir` method MUST return the plugin skills subdirectory for current kimi-code installs instead of the flat `~/.kimi-code/skills` path.
(Previously: returned `~/.kimi-code/skills`)

#### Scenario: current kimi-code SkillsDir returns plugin path

- GIVEN current kimi-code is detected (`~/.kimi-code` exists or `KIMI_CODE_HOME` is set)
- WHEN `SkillsDir(homeDir)` is called
- THEN it returns `{resolvedConfigDir}/plugins/managed/gentle-ai/skills`

#### Scenario: Legacy SkillsDir unchanged

- GIVEN legacy Kimi is detected (`~/.kimi` exists and `~/.kimi-code` does not)
- WHEN `SkillsDir(homeDir)` is called
- THEN it returns `{homeDir}/.config/agents/skills`

---

### Requirement: KIMI_CODE_HOME Propagation to Plugin and Skills Paths

All current kimi-code plugin and skill paths — including `PluginDir`, `PluginManifestPath`,
`SkillsDir`, the user skills directory used by `AllSkillsDirs`, and the
`installed.json` registry path written by `InstallPlugin` — MUST be rooted under
the directory returned by `resolveConfigDir(homeDir)` so that `KIMI_CODE_HOME`
does not split config, plugins, and skills across different trees.

#### Scenario: PluginDir respects env override

- GIVEN `KIMI_CODE_HOME=/custom/kimi` and `/custom/kimi` exists
- WHEN `PluginDir(homeDir)` is called
- THEN it returns `/custom/kimi/plugins/managed/gentle-ai`

#### Scenario: InstallPlugin respects env override

- GIVEN `KIMI_CODE_HOME=/custom/kimi` and `/custom/kimi` exists
- WHEN `InstallPlugin(homeDir, "1.0.0")` is called
- THEN `kimi.plugin.json` is written under `/custom/kimi/plugins/managed/gentle-ai/`
- AND `installed.json` is written under `/custom/kimi/plugins/`

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

### Requirement: Kimi Skill-Registry Automation

`installSkillRegistryAutomation` MUST handle `model.AgentKimi` by writing a `sessionStart` hook to `config.toml` that runs `gentle-ai skill-registry refresh`.
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
