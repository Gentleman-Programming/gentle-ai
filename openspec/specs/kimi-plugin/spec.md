# Kimi Plugin Specification

## Purpose

Defines requirements for the current Kimi Code plugin scaffolding: directory structure creation, manifest generation with sessionStart hook, and the `skills/` subdirectory that enables skill injection.

---

## Requirements

### Requirement: InstallPlugin Creates Full Directory Structure

`InstallPlugin` MUST create the complete plugin directory tree under `~/.kimi-code/plugins/managed/gentle-ai/`, including a `skills/` subdirectory, when called for current kimi-code installs. The directory creation MUST be idempotent.

#### Scenario: Fresh install creates full tree

- GIVEN current kimi-code is detected and `~/.kimi-code/plugins/managed/gentle-ai/` does not exist
- WHEN `InstallPlugin(homeDir, version)` is called
- THEN `{homeDir}/.kimi-code/plugins/managed/gentle-ai/` exists
- AND `{homeDir}/.kimi-code/plugins/managed/gentle-ai/skills/` exists

#### Scenario: Re-run does not fail

- GIVEN the plugin directory and `skills/` subdirectory already exist
- WHEN `InstallPlugin(homeDir, version)` is called again
- THEN no error is returned
- AND existing directory contents are preserved

#### Scenario: Parent directories created as needed

- GIVEN `~/.kimi-code/` exists but `plugins/managed/` does not
- WHEN `InstallPlugin(homeDir, version)` is called
- THEN `plugins/` and `managed/` directories are created intermediate

---

### Requirement: Plugin Manifest Includes sessionStart Hook

The `kimi.plugin.json` manifest MUST include a `hooks.sessionStart` field referencing the `sdd-init` skill. This enables Kimi Code to auto-trigger SDD initialization on session start.

#### Scenario: Manifest contains sessionStart hook

- GIVEN `InstallPlugin(homeDir, version)` is called
- WHEN `kimi.plugin.json` is written to disk
- THEN the manifest contains `"hooks": { "sessionStart": { "skill": "sdd-init" } }`

#### Scenario: Manifest version matches installed version

- GIVEN `InstallPlugin(homeDir, "0.1.0")` is called
- WHEN `kimi.plugin.json` is read from disk
- THEN `"version"` field equals `"0.1.0"`

#### Scenario: Manifest contains all required fields

- GIVEN `InstallPlugin(homeDir, version)` is called
- WHEN `kimi.plugin.json` is written to disk
- THEN it contains `"name": "gentle-ai"`
- AND it contains `"description"` identifying Gentle AI
- AND it contains `"skills": { "path": "skills" }`
- AND it contains `"mcpServers"` with Engram configuration

---

### Requirement: Plugin Directory Scaffolding

The `skills/` subdirectory MUST be created during `InstallPlugin` so that skill injection has a target directory ready before any skill files are written.

#### Scenario: Skills directory exists before injection

- GIVEN `InstallPlugin` completes successfully
- WHEN the SDD inject component runs skill placement
- THEN `{homeDir}/.kimi-code/plugins/managed/gentle-ai/skills/` exists as a writable directory

#### Scenario: Skills directory is empty on fresh install

- GIVEN a fresh install with no prior skill files
- WHEN `InstallPlugin` completes
- THEN `{homeDir}/.kimi-code/plugins/managed/gentle-ai/skills/` exists and contains no files

#### Scenario: Graceful fallback on permission failure

- GIVEN `InstallPlugin` is called but `~/.kimi-code/plugins/managed/` cannot be created (permissions)
- WHEN `InstallPlugin` returns an error
- THEN the caller falls back to `SkillsDir(homeDir)` for skill placement
- AND a warning is logged indicating the fallback
