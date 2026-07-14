# Kimi AGENTS.md Injection Specification

## Purpose

Defines requirements for handling `${KIMI_AGENTS_MD}` in KIMI.md while keeping
project instructions project-scoped. The adapter MUST NOT copy cwd-derived
AGENTS.md content into the global Kimi config.

---

## Requirements

### Requirement: AGENTSMDPath Method

The adapter MUST implement an `AGENTSMDPath(homeDir string) string` method
returning the path to the Kimi-level AGENTS.md file in the resolved config
directory.

#### Scenario: v0.11+ returns correct path

- GIVEN Kimi Code v0.11+ is detected (`~/.kimi-code` exists or `KIMI_CODE_HOME`
  is set)
- WHEN `AGENTSMDPath(homeDir)` is called
- THEN it returns `{resolvedConfigDir}/AGENTS.md`

#### Scenario: Legacy returns correct path

- GIVEN legacy Kimi is detected (no `~/.kimi-code` and no `KIMI_CODE_HOME`)
- WHEN `AGENTSMDPath(homeDir)` is called
- THEN it returns `{homeDir}/.kimi/AGENTS.md`

---

### Requirement: Project AGENTS.md Remains Project-Scoped

`BootstrapTemplate` MUST NOT read the project-root `AGENTS.md` from the current
working directory, nor write cwd-derived content to the global Kimi config.
Instead, it MUST replace `${KIMI_AGENTS_MD}` in KIMI.md with a project-scoped
placeholder that instructs Kimi to respect the current worktree's AGENTS.md at
runtime.

#### Scenario: Project AGENTS.md exists

- GIVEN a project directory containing `AGENTS.md`
- WHEN `BootstrapTemplate(homeDir)` is called
- THEN the written KIMI.md does NOT contain the project AGENTS.md content
- AND the written KIMI.md contains a placeholder explaining that project
  AGENTS.md is read from the worktree at runtime

#### Scenario: No project AGENTS.md

- GIVEN no `AGENTS.md` in the project root
- WHEN `BootstrapTemplate(homeDir)` is called
- THEN the written KIMI.md still contains the project-scoped placeholder

---

### Requirement: No Global AGENTS.md Written from CWD

`BootstrapTemplate` MUST NOT write project AGENTS.md content (or a placeholder
derived from cwd state) to `AGENTSMDPath(homeDir)`.

#### Scenario: Bootstrap from a project with AGENTS.md

- GIVEN a project with `AGENTS.md` content
- WHEN `BootstrapTemplate(homeDir)` is called
- THEN no file is created at `AGENTSMDPath(homeDir)`

#### Scenario: Bootstrap from a project without AGENTS.md

- GIVEN no project AGENTS.md exists
- WHEN `BootstrapTemplate(homeDir)` is called
- THEN no file is created at `AGENTSMDPath(homeDir)`
