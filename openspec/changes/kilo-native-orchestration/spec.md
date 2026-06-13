# Kilo Native Orchestration — Delta Spec

## Purpose

Replace the OpenCode overlay (issue #729) with native `.kilo/agents/*.md` sub-agent files, `kilo.jsonc` config, and Kilo Gateway model routing.

---

## ADDED Requirements

### Requirement: Native Sub-Agent File Generation

The Kilo adapter MUST generate `.kilo/agents/*.md` files for each SDD phase agent with YAML frontmatter containing `name`, `description`, `tools`, and `model` fields.

#### Scenario: Install creates agent files

- GIVEN `gentle-ai install --agent kilocode` runs
- WHEN install completes
- THEN `.kilo/agents/sdd-apply.md` and `.kilo/agents/sdd-verify.md` exist
- AND each file has valid YAML frontmatter with `name`, `description`, `tools`, `model`

#### Scenario: Frontmatter model field is non-empty

- GIVEN a generated `.kilo/agents/sdd-apply.md`
- WHEN frontmatter is parsed
- THEN `model` is a non-empty Kilo Gateway model identifier

---

### Requirement: Sub-Agent Support Enabled

The adapter MUST report `SupportsSubAgents() = true`, `SubAgentsDir()` → `~/.kilo/agents/`, `EmbeddedSubAgentsDir()` → `"kilocode/agents"`.

#### Scenario: Adapter returns true

- GIVEN the Kilo adapter is instantiated
- WHEN `SupportsSubAgents()` called
- THEN returns `true`

#### Scenario: SubAgentsDir path correct

- GIVEN `homeDir` = `/home/user`
- WHEN `SubAgentsDir("/home/user")` called
- THEN returns `/home/user/.kilo/agents`

---

### Requirement: Kilo Model Alias Resolution

A `kiloModelResolver` MUST map SDD phases to Kilo Gateway model identifiers via `KiloModelAlias` and `KiloModelID()`.

#### Scenario: Alias resolves

- GIVEN `kiloModelResolver` initialized
- WHEN `KiloModelID(KiloModelAuto)` is called
- THEN returns a valid Kilo Gateway model identifier

#### Scenario: Unknown alias fallback

- GIVEN `kiloModelResolver` initialized
- WHEN `KiloModelID("unknown")` is called
- THEN returns the default model (not empty)

#### Scenario: All phases have assignments

- GIVEN default balanced preset loaded
- WHEN phase assignments inspected
- THEN `sdd-explore`, `sdd-spec`, `sdd-design`, `sdd-tasks`, `sdd-apply`, `sdd-verify` each have a non-empty alias

---

### Requirement: Provider Config Generation

The system MUST generate `kilo.jsonc` at workspace root with a `providers` block containing Kilo Gateway endpoint, API key placeholder, and model routing table.

#### Scenario: kilo.jsonc generated on install

- GIVEN `gentle-ai install --agent kilocode` runs
- WHEN install completes
- THEN `kilo.jsonc` exists at workspace root
- AND contains a `providers` key with `kilo-gateway` entry

#### Scenario: Provider config has required fields

- GIVEN `kilo.jsonc` is generated
- WHEN parsed
- THEN `kilo-gateway` entry has `baseUrl` (non-empty) and `apiKey` fields

---

### Requirement: Profile Detection

The system MUST check `~/.config/kilo/profiles/` for Kilo profiles. On missing directory, fall back to default with a warning.

#### Scenario: Profile detected

- GIVEN `~/.config/kilo/profiles/cheap/` exists
- WHEN `gentle-ai sync --profile cheap:kilo` runs
- THEN sync uses the profile from that directory

#### Scenario: Missing profile fallback

- GIVEN `~/.config/kilo/profiles/cheap/` does NOT exist
- WHEN `gentle-ai sync --profile cheap:kilo` runs
- THEN sync falls back to default profile with a warning

---

### Requirement: Post-Injection Verification

The system MUST verify all `.kilo/agents/sdd-*.md` files exist, are non-empty, and have valid YAML frontmatter after injection.

#### Scenario: Verification passes

- GIVEN `gentle-ai install --agent kilocode` completes
- WHEN verification runs
- THEN all expected agent files confirmed present and valid

#### Scenario: Verification fails on missing file

- GIVEN only `.kilo/agents/sdd-apply.md` exists
- WHEN verification runs
- THEN reports failure listing missing files

---

### Requirement: Orchestrator Must Not Use Primary Mode

The orchestrator agent file MUST NOT contain `"mode": "primary"` in YAML frontmatter (Kilo v7 rejects this for sub-agents).

#### Scenario: No primary mode

- GIVEN `.kilo/agents/sdd-orchestrator.md` generated
- WHEN frontmatter parsed
- THEN no `mode: primary` field exists

---

## MODIFIED Requirements

### Requirement: Kilo Adapter Sub-Agent Capabilities

The Kilo adapter MUST enable native sub-agent support: `SupportsSubAgents()` → `true`, `SubAgentsDir()` → `~/.kilo/agents/`, `EmbeddedSubAgentsDir()` → `"kilocode/agents"`. Adapter MUST expose `KiloModelID()`.
(Previously: `SupportsSubAgents()` returned `false`, both dir methods returned empty string)

#### Scenario: Support enabled

- GIVEN Kilo adapter instantiated
- WHEN `SupportsSubAgents()` called
- THEN returns `true`

#### Scenario: EmbeddedSubAgentsDir returns asset prefix

- GIVEN Kilo adapter instantiated
- WHEN `EmbeddedSubAgentsDir()` called
- THEN returns `"kilocode/agents"`

---

## REMOVED Requirements

None. OpenCode overlay retained as fallback (NFR-1).

---

## RENAMED Requirements

None.
