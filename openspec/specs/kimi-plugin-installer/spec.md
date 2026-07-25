# Kimi Plugin Installer Specification

## Purpose

Defines requirements for the optional `PluginInstaller` interface and its Kimi Code implementation, enabling plugin-based skill distribution with manifest generation, directory isolation, and MCP server bundling.

---

## Requirements

### Requirement: PluginInstaller Optional Interface

The system MUST define a `PluginInstaller` interface with `PluginDir(homeDir) string`, `PluginManifestPath(homeDir) string`, and `InstallPlugin(homeDir, version) error` methods. Any adapter MAY implement this interface; non-implementing adapters are unaffected.

#### Scenario: Adapter implements PluginInstaller

- GIVEN an adapter satisfies the `PluginInstaller` interface
- WHEN the SDD inject component checks adapter capabilities
- THEN `PluginDir` returns the plugin root directory
- AND `PluginManifestPath` returns the path to `kimi.plugin.json`
- AND `InstallPlugin` creates the manifest on disk

#### Scenario: Adapter does not implement PluginInstaller

- GIVEN an adapter does not satisfy the `PluginInstaller` interface
- WHEN the SDD inject component checks adapter capabilities
- THEN the adapter falls through to the existing `SkillsDir` path unchanged

---

### Requirement: Plugin Directory Structure

The Kimi adapter MUST write plugin artifacts to `~/.kimi-code/plugins/managed/gentle-ai/` for current kimi-code installs. Skill files MUST be placed under the `skills/` subdirectory within this path.

#### Scenario: current kimi-code plugin directory path

- GIVEN current Kimi Code is detected (`~/.kimi-code` exists)
- WHEN `PluginDir(homeDir)` is called
- THEN it returns `{homeDir}/.kimi-code/plugins/managed/gentle-ai`

#### Scenario: Skills subdirectory within plugin

- GIVEN current kimi-code is detected
- WHEN skills are injected via `PluginInstaller`
- THEN skill files are written to `{homeDir}/.kimi-code/plugins/managed/gentle-ai/skills/`

---

### Requirement: kimi.plugin.json Manifest Generation

The adapter MUST generate a `kimi.plugin.json` manifest containing `name`, `version`, `description`, `skills.path`, `hooks.sessionStart.skill`, and `mcpServers.engram` fields.

#### Scenario: Manifest contains required fields

- GIVEN a current kimi-code install with `InstallPlugin(homeDir, version)` called
- WHEN `kimi.plugin.json` is written to disk
- THEN it contains `"name": "gentle-ai"`
- AND it contains `"version"` matching the installed version
- AND it contains `"description"` identifying Gentle AI
- AND it contains `"skills": { "path": "skills" }`
- AND it contains `"hooks": { "sessionStart": { "skill": "sdd-init" } }`

#### Scenario: Manifest declares Engram MCP server

- GIVEN a current kimi-code install with `InstallPlugin(homeDir, version)` called
- WHEN `kimi.plugin.json` is written to disk
- THEN it contains `"mcpServers": { "engram": { ... } }` with the Engram server configuration

#### Scenario: Manifest file does not overwrite user-edited version

- GIVEN `kimi.plugin.json` already exists with user modifications
- WHEN `InstallPlugin` is called
- THEN the manifest is regenerated and written atomically, replacing the previous version

---

### Requirement: kimi-code Layout Detection Guard

The adapter MUST treat an install as current kimi-code when `KIMI_CODE_HOME` points at an existing directory, when `~/.kimi-code` exists, or when neither `~/.kimi-code` nor legacy `~/.kimi` exists (fresh machine — gentle-ai auto-installs the npm kimi-code CLI). Legacy installs (`~/.kimi` exists without `~/.kimi-code`) MUST NOT attempt plugin-based installation.

#### Scenario: current kimi-code detected enables plugin path

- GIVEN `~/.kimi-code` directory exists
- WHEN the adapter resolves the skills directory
- THEN it returns the plugin subdirectory path, not the flat skills path

#### Scenario: Legacy install falls back to flat skills

- GIVEN `~/.kimi` exists and `~/.kimi-code` does not
- WHEN the adapter resolves the skills directory
- THEN it returns `~/.config/agents/skills` (legacy path)

---

### Requirement: Graceful Fallback on Permission Failure

If plugin directory creation fails due to filesystem permissions, the adapter MUST fall back to the flat skills directory and log a warning.

#### Scenario: Plugin dir creation fails gracefully

- GIVEN `InstallPlugin` is called but `~/.kimi-code/plugins/managed/` cannot be created (permissions)
- WHEN `InstallPlugin` returns an error
- THEN the SDD inject component catches the error
- AND falls back to `SkillsDir(homeDir)` for skill placement
- AND a warning is logged indicating the fallback

---

### Requirement: installed.json Registry Preservation

`InstallPlugin` MUST register the gentle-ai plugin in Kimi Code's `plugins/installed.json` without destroying data it does not own. Unknown top-level fields and unknown per-plugin fields MUST be preserved on rewrite. If the existing registry cannot be parsed, `InstallPlugin` MUST fail with an error instead of overwriting the file.

#### Scenario: Unknown fields survive re-registration

- GIVEN `installed.json` contains another plugin's record and fields gentle-ai does not know about
- WHEN `InstallPlugin` re-registers the gentle-ai plugin
- THEN the other plugin's record and all unknown fields remain intact

#### Scenario: Corrupt registry is not overwritten

- GIVEN `installed.json` exists but contains invalid JSON
- WHEN `InstallPlugin` is called
- THEN it returns an error
- AND the corrupt file content is left unmodified

---

### Requirement: Engram MCP Server Declaration

The `kimi.plugin.json` manifest MUST declare the Engram MCP server under `mcpServers` with the correct command and transport configuration.

#### Scenario: Engram server declared in manifest

- GIVEN `InstallPlugin` generates the manifest
- WHEN the manifest is written to disk
- THEN `mcpServers.engram` contains a valid server declaration with command and args

---

## Interface Contract

```go
// PluginInstaller is an optional adapter capability for plugin-based distribution.
// Adapters that implement this interface enable the SDD inject component to place
// skills in an isolated plugin directory and generate a plugin manifest.
type PluginInstaller interface {
    // PluginDir returns the root directory for the plugin (e.g. ~/.kimi-code/plugins/managed/gentle-ai/).
    PluginDir(homeDir string) string
    // PluginManifestPath returns the full path to kimi.plugin.json.
    PluginManifestPath(homeDir string) string
    // InstallPlugin creates the plugin directory and generates the manifest.
    InstallPlugin(homeDir string, version string) error
}
```

---

## Test Requirements

- Unit tests for manifest struct serialization (all required fields present)
- Unit tests for `PluginDir` and `PluginManifestPath` path resolution
- Unit tests for `InstallPlugin` creating directory + writing manifest
- Unit tests for legacy fallback (`~/.kimi` exists without `~/.kimi-code`)
- Unit tests for permission failure graceful fallback
- Unit tests for installed.json preservation (unknown fields kept, corrupt registry rejected)
- Integration test verifying manifest JSON schema matches current Kimi Code expectations
