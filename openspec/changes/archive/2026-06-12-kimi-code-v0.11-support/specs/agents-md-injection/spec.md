# Kimi AGENTS.md Injection Specification

## Purpose

Defines requirements for resolving `${KIMI_AGENTS_MD}` in KIMI.md by reading project-level AGENTS.md content and injecting it during bootstrap.

---

## Requirements

### Requirement: AGENTSMDPath Method

The adapter MUST implement an `AGENTSMDPath(homeDir string) string` method returning the path to the Kimi-level AGENTS.md file in the resolved config directory.

#### Scenario: v0.11+ returns correct path

- GIVEN Kimi Code v0.11+ is detected (`~/.kimi-code` exists)
- WHEN `AGENTSMDPath(homeDir)` is called
- THEN it returns `{resolvedConfigDir}/AGENTS.md`

#### Scenario: Legacy returns correct path

- GIVEN legacy Kimi is detected (no `~/.kimi-code`)
- WHEN `AGENTSMDPath(homeDir)` is called
- THEN it returns `{homeDir}/.kimi/AGENTS.md`

---

### Requirement: Project AGENTS.md Content Resolution

During `BootstrapTemplate`, after writing KIMI.md, the adapter MUST read the project-root `AGENTS.md` file (if present) and resolve `${KIMI_AGENTS_MD}` in the rendered KIMI.md by substituting its content. If no project AGENTS.md exists, a placeholder comment MUST be written.

#### Scenario: Project AGENTS.md exists and gets injected

- GIVEN a project directory containing `AGENTS.md` with content `# Project Rules\nBe nice`
- WHEN `BootstrapTemplate(homeDir)` is called
- THEN the written KIMI.md contains `# Project Rules\nBe nice` where `${KIMI_AGENTS_MD}` was

#### Scenario: No project AGENTS.md — placeholder written

- GIVEN no `AGENTS.md` in the project root
- WHEN `BootstrapTemplate(homeDir)` is called
- THEN the written KIMI.md contains a placeholder comment in place of `${KIMI_AGENTS_MD}`

---

### Requirement: AGENTS.md Content Sanitization

The adapter MUST sanitize project AGENTS.md content before injection to prevent Jinja template syntax conflicts with KIMI.md rendering.

#### Scenario: Jinja syntax in AGENTS.md is escaped

- GIVEN project `AGENTS.md` contains `{{ variable }}` Jinja-like syntax
- WHEN content is injected into KIMI.md
- THEN the Jinja syntax is escaped or neutralized and does not break template rendering

---

### Requirement: AGENTS.md File Creation

`BootstrapTemplate` MUST write the resolved AGENTS.md content to the adapter's `AGENTSMDPath` location.

#### Scenario: AGENTS.md written to config directory

- GIVEN a project with `AGENTS.md` content
- WHEN `BootstrapTemplate(homeDir)` is called
- THEN a file exists at `AGENTSMDPath(homeDir)` with the project AGENTS.md content

#### Scenario: No project AGENTS.md — minimal file written

- GIVEN no project AGENTS.md exists
- WHEN `BootstrapTemplate(homeDir)` is called
- THEN a file exists at `AGENTSMDPath(homeDir)` with a placeholder comment
