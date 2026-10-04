# Delta for Doctor

## ADDED Requirements

### Requirement: Generic Managed-Block Section Scanner

The system MUST scan any markdown file for well-formed `<!-- gentle-ai:X --> ... <!-- /gentle-ai:X -->` pairs, returning each section's ID, raw content, and char/line span. The scanner MUST NOT use a hardcoded ID whitelist — new IDs added later MUST be recognized without code changes. The scanner MUST also flag structurally broken pairs (opening marker with no matching close, or vice versa) as orphan/unclosed, distinct from well-formed sections.

#### Scenario: File with multiple distinct sections

- GIVEN a file with three distinct marker pairs (`persona`, `engram-protocol`, `strict-tdd-mode`)
- WHEN the scanner parses it
- THEN it MUST return three sections with correct ID, content, and span

#### Scenario: Unknown section ID

- GIVEN a file with marker pair `gentle-ai:brand-new-block`, unknown when the scanner was written
- WHEN the scanner parses it
- THEN it MUST return that section like any other, with no code change required

#### Scenario: Unclosed marker

- GIVEN a file with opening marker `gentle-ai:sdd-orchestrator` and no matching close
- WHEN the scanner parses it
- THEN it MUST flag that section as orphan/unclosed, not merge it with adjacent content

### Requirement: Always-On Managed Block Footprint Check

`gentle-ai doctor` MUST run a "Managed Block Footprint" check every invocation, no flag required, reporting total block count, agents covered (`InstalledAgents`), and a rough total token estimate. Status MUST be `OK` when all configured agents' instruction files parse with no structural breakage. Status MUST be `WARN` or `FAIL` when any orphan/unclosed marker is found. Block count and token estimate alone MUST NOT trigger `WARN`/`FAIL`.

#### Scenario: Zero configured agents

- GIVEN `InstalledAgents` is empty
- WHEN `gentle-ai doctor` runs
- THEN the check MUST report `OK`, 0 agents, 0 blocks, and MUST NOT error

#### Scenario: One agent with several well-formed blocks

- GIVEN one configured agent whose `SystemPromptFile` has four well-formed marker pairs
- WHEN `gentle-ai doctor` runs without `--footprint`
- THEN the compact line MUST report 4 blocks, 1 agent, a rough token estimate, and status `OK`

#### Scenario: Orphan/unclosed marker present

- GIVEN one configured agent's `SystemPromptFile` has an unclosed `gentle-ai:X` marker
- WHEN `gentle-ai doctor` runs
- THEN it MUST report `WARN` or `FAIL`, naming the agent and section ID

### Requirement: Opt-In Detailed Footprint Breakdown (--footprint)

`gentle-ai doctor` MUST accept an opt-in `--footprint` flag. WHEN present, it MUST render a per-agent, per-block table (section ID, char count, line count, rough token estimate) in addition to the compact summary. WHEN absent, doctor MUST print only the compact summary, never the table.

#### Scenario: --footprint with multiple agents

- GIVEN two configured agents, each with at least two well-formed marker pairs
- WHEN `gentle-ai doctor --footprint` runs
- THEN output MUST include one row per (agent, section ID), each with char/line count and token estimate
- AND the compact summary line MUST still appear

#### Scenario: --footprint omitted

- GIVEN one configured agent with several managed blocks
- WHEN `gentle-ai doctor` runs without `--footprint`
- THEN output MUST NOT contain the per-block breakdown table

### Requirement: Approximate Token Estimation

Token counts, in both the compact check and `--footprint` breakdown, MUST come from a pure chars/4-style heuristic and MUST be labeled approximate (e.g. "rough", "approx", "~") in rendered output. The system MUST NOT imply exact, tokenizer-precise counts.

#### Scenario: Estimate labeled as rough

- GIVEN any footprint output (compact or `--footprint`)
- WHEN token estimates render
- THEN the text MUST include a qualifier marking the value as an estimate

### Requirement: Markdown Marker Scope Boundary

The footprint check MUST only scan markdown files from `agents.Adapter.SystemPromptFile(homeDir)`. It MUST NOT scan or report JSON settings-overlay files, even for agents that also use JSON configuration.

#### Scenario: Agent with JSON settings overlay

- GIVEN a configured agent with a JSON overlay file alongside its markdown `SystemPromptFile`
- WHEN `gentle-ai doctor` runs, with or without `--footprint`
- THEN it MUST report only on marker sections in the markdown `SystemPromptFile`
- AND MUST NOT parse or report on the JSON overlay file
