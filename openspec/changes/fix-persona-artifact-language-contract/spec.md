# Persona and artifact language contract

## ADDED Requirements

### Requirement: Direct conversation follows the active persona

Direct user and orchestrator conversation MUST be governed by the active persona. Persona rules MUST apply to conversational replies, clarification prompts, and user-facing orchestration status, but MUST NOT be treated as the default language or regional style for generated technical artifacts.

#### Scenario: Gentleman governs direct conversation

- GIVEN the active persona is `gentleman`
- WHEN the agent replies directly to the user or orchestrator
- THEN the reply MUST preserve the Gentleman teaching voice
- AND the reply MUST use the expected Rioplatense senior-architect style when Spanish is used
- AND the reply MAY use voseo, warm/direct phrasing, and concept-before-code explanations

#### Scenario: Neutral governs direct conversation

- GIVEN the active persona is `neutral`
- WHEN the agent replies directly to the user or orchestrator
- THEN the reply MUST keep the same teaching core and clear technical guidance
- AND the reply MUST NOT use Rioplatense regional expressions by default

#### Scenario: Persona does not cross the artifact boundary

- GIVEN the active persona has regional Spanish conversation rules
- WHEN the agent generates a technical artifact
- THEN the artifact language MUST be selected from the artifact language contract
- AND the artifact MUST NOT inherit regional persona phrasing unless the user explicitly requests that regional style for the artifact

---

### Requirement: Technical artifacts default to English

Generated technical artifacts MUST default to English regardless of active persona or conversation language. Technical artifacts SHALL include OpenSpec proposal, spec, design, task, verification, and archive artifacts; SDD phase artifacts; prompt-generated technical files; generated code comments; UI copy; tests; fixtures; and other repository-facing files unless an explicit user request or project convention requires another language.

#### Scenario: Spanish conversation produces English OpenSpec artifacts

- GIVEN the user and agent are conversing in Spanish
- AND no explicit Spanish artifact request exists
- WHEN the agent writes an OpenSpec artifact
- THEN the artifact MUST be written in English
- AND the artifact MUST NOT include Rioplatense conversational expressions

#### Scenario: Persona-specific voice is excluded from generated technical files

- GIVEN the active persona is `gentleman`
- WHEN the agent writes specs, designs, tasks, generated code comments, UI copy, tests, fixtures, or prompt-generated technical files
- THEN those files MUST default to English
- AND they MUST NOT use voseo or regional Spanish terms from the Gentleman persona unless explicitly requested for that artifact

#### Scenario: Project convention can require a non-English artifact

- GIVEN a repository convention clearly requires a technical artifact in Spanish
- WHEN the agent writes that artifact
- THEN the artifact MAY be written in Spanish
- AND the artifact MUST follow the Spanish technical artifact contract

---

### Requirement: Spanish technical artifacts use neutral professional Spanish

When technical artifacts are explicitly requested in Spanish, or when a project convention requires Spanish artifacts, the artifact MUST use neutral/professional Spanish by default. Regional Spanish variants MAY be used only when the user explicitly requests the regional style for that artifact.

#### Scenario: Explicit Spanish artifact request defaults neutral

- GIVEN the user asks for a technical artifact in Spanish
- AND the user does not request a regional variant
- WHEN the agent writes the artifact
- THEN the artifact MUST use neutral/professional Spanish
- AND it MUST avoid Rioplatense expressions such as voseo by default

#### Scenario: Explicit regional artifact request is honored

- GIVEN the user asks for a Spanish technical artifact
- AND the user explicitly requests Rioplatense tone for that artifact
- WHEN the agent writes the artifact
- THEN the artifact MAY use Rioplatense phrasing
- AND the artifact MUST still remain professional and technically precise

---

### Requirement: Comment writer follows target context language

The `comment-writer` skill MUST be context-reactive. It MUST write public or contextual comments in the target context language by default, and an explicit user language or tone override MUST take precedence over inferred context.

#### Scenario: Spanish context yields Spanish comment

- GIVEN the target issue, pull request, review thread, or message is in Spanish
- AND the user does not override the language
- WHEN `comment-writer` drafts the comment
- THEN the comment MUST be written in Spanish

#### Scenario: English context yields English comment

- GIVEN the target issue, pull request, review thread, or message is in English
- AND the user does not override the language
- WHEN `comment-writer` drafts the comment
- THEN the comment MUST be written in English

#### Scenario: Mixed context follows target message language

- GIVEN the surrounding thread is mixed language
- AND a specific target message or audience language is identifiable
- WHEN `comment-writer` drafts the comment
- THEN the comment MUST use the target message or audience language

#### Scenario: User override wins

- GIVEN the target context language is identifiable
- AND the user explicitly requests a different language or tone
- WHEN `comment-writer` drafts the comment
- THEN the comment MUST follow the explicit user request

---

### Requirement: Spanish comments default neutral professional

Spanish comments produced by `comment-writer` MUST default to neutral/professional Spanish. Regional Spanish tone MAY be used only when the user explicitly asks for it or when the surrounding target context clearly calls for that regional tone.

#### Scenario: Spanish comment without regional signal is neutral

- GIVEN `comment-writer` is drafting a Spanish comment
- AND neither the user request nor the target context calls for a regional tone
- WHEN the comment is produced
- THEN the comment MUST use neutral/professional Spanish
- AND it MUST NOT force Rioplatense wording or voseo

#### Scenario: Regional Spanish comment requires a clear signal

- GIVEN `comment-writer` is drafting a Spanish comment
- AND the user explicitly requests Rioplatense tone or the target context clearly uses and expects that tone
- WHEN the comment is produced
- THEN the comment MAY use Rioplatense phrasing
- AND the comment MUST remain appropriate for the public or contextual target

#### Scenario: Root and embedded skill contracts stay aligned

- GIVEN the root `skills/comment-writer/SKILL.md` and embedded `internal/assets/skills/comment-writer/SKILL.md` exist
- WHEN their language behavior rules are inspected
- THEN both skill sources MUST require context-reactive comment language
- AND both skill sources MUST require neutral/professional Spanish by default
- AND neither source MUST force Rioplatense Spanish for all Spanish comments

---

### Requirement: All supported SDD agent assets implement the contract

Every supported SDD orchestrator asset MUST codify the persona/artifact/comment language boundary. Covered assets SHALL include OpenCode, Kilocode through the OpenCode asset path, Claude, Kimi, Codex, Gemini, Qwen, Cursor, Windsurf, Antigravity, Kiro, the generic fallback, and any additional supported agent-specific SDD orchestrator assets discovered in `internal/assets` or the supported agent registry.

#### Scenario: Known supported asset set is covered

- GIVEN the SDD orchestrator assets are reviewed
- WHEN language-contract coverage is evaluated
- THEN coverage MUST include `internal/assets/opencode/sdd-orchestrator.md`
- AND coverage MUST include Kilocode behavior that uses the OpenCode SDD asset path
- AND coverage MUST include `internal/assets/claude/sdd-orchestrator.md`
- AND coverage MUST include `internal/assets/kimi/sdd-orchestrator.md`
- AND coverage MUST include `internal/assets/codex/sdd-orchestrator.md`
- AND coverage MUST include `internal/assets/gemini/sdd-orchestrator.md`
- AND coverage MUST include `internal/assets/qwen/sdd-orchestrator.md`
- AND coverage MUST include `internal/assets/cursor/sdd-orchestrator.md`
- AND coverage MUST include `internal/assets/windsurf/sdd-orchestrator.md`
- AND coverage MUST include `internal/assets/antigravity/sdd-orchestrator.md`
- AND coverage MUST include `internal/assets/kiro/sdd-orchestrator.md`
- AND coverage MUST include `internal/assets/generic/sdd-orchestrator.md`

#### Scenario: Newly discovered supported assets are not skipped

- GIVEN an additional supported agent-specific SDD orchestrator asset exists
- WHEN implementation and tests enumerate language-contract coverage
- THEN the asset MUST be included in the same contract checks
- AND the generic fallback MUST remain covered for unsupported or unrecognized agents

#### Scenario: Persona-specific direct conversation assets remain allowed

- GIVEN asset language guards inspect supported assets
- WHEN Gentleman direct-conversation persona assets are inspected
- THEN tests MAY allow intentional Rioplatense direct-conversation wording in those persona assets
- AND tests MUST still prevent those persona rules from becoming the default for technical artifacts or comments

---

### Requirement: Install and sync preserve the updated language contract

Install and sync flows MUST install, refresh, and regenerate only assets that comply with the updated language contract. They MUST NOT regenerate old persona leaks from stale embedded sources, overlays, shared prompt files, generated agent files, or root skill copies.

#### Scenario: Fresh install does not regenerate old leaks

- GIVEN a fresh install writes SDD and skill assets
- WHEN generated assets are inspected after install
- THEN they MUST contain the updated artifact/comment language contract
- AND they MUST NOT contain the known old leak terms in persona-agnostic SDD assets

#### Scenario: Sync does not restore stale wording

- GIVEN an installation already contains outdated SDD or `comment-writer` wording
- WHEN sync refreshes SDD and skill assets
- THEN the refreshed assets MUST contain the updated language contract
- AND sync MUST NOT restore stale Rioplatense defaults for persona-agnostic artifacts or comments

#### Scenario: OpenCode and Kilocode leak path is guarded

- GIVEN OpenCode or Kilocode SDD assets are installed or synced
- WHEN generated orchestrator prompts, overlays, or shared prompt files are inspected
- THEN the generated content MUST NOT include the old OpenCode leak wording
- AND the generated content MUST preserve the updated artifact/comment contract

---

### Requirement: Delegated prompts forward the artifact and comment contract

Delegated SDD phase prompts and subagent instructions MUST forward the artifact and comment language contract. Delegated agents MUST know that direct conversation may be persona-governed, generated technical artifacts default to English, Spanish technical artifacts default to neutral/professional Spanish, and comments follow the target context language by default.

#### Scenario: Delegated phase prompt receives artifact defaults

- GIVEN the orchestrator delegates an SDD phase to a phase executor or subagent
- WHEN the delegated prompt is constructed
- THEN the prompt MUST state that generated technical artifacts default to English
- AND the prompt MUST state that Spanish technical artifacts use neutral/professional Spanish unless explicitly regional

#### Scenario: Delegated comment work receives comment defaults

- GIVEN the orchestrator delegates comment-writing or review-comment drafting work
- WHEN the delegated prompt is constructed
- THEN the prompt MUST forward that comments use the target context language by default
- AND the prompt MUST forward that Spanish comments default to neutral/professional Spanish unless user or context clearly calls for regional tone

#### Scenario: Delegation does not convert persona into artifact language

- GIVEN the active persona uses Rioplatense direct conversation
- WHEN the orchestrator delegates artifact-writing work
- THEN the delegated prompt MUST NOT instruct the phase executor to write artifacts in Rioplatense Spanish by default
- AND the delegated prompt MUST preserve the artifact language contract

---

### Requirement: Known language leaks are prevented

Persona-agnostic SDD assets, generated technical artifacts, delegated prompts, install/sync outputs, and root or embedded comment-writer skill contracts MUST prevent recurrence of the known leaks: `elegí`, `Respondé`, the hardcoded `¿Querés ajustar algo o continuamos?` continuation in persona-agnostic SDD flows, and root `comment-writer` forcing Rioplatense Spanish.

#### Scenario: Persona-agnostic SDD assets reject known leak terms

- GIVEN persona-agnostic SDD orchestrator assets are inspected
- WHEN language guard tests run
- THEN the assets MUST NOT contain `elegí`
- AND the assets MUST NOT contain `Respondé`
- AND the assets MUST NOT contain `¿Querés ajustar algo o continuamos?`

#### Scenario: Generated artifacts reject known leak terms

- GIVEN install, sync, or SDD artifact generation produces prompt or artifact files
- WHEN generated files are inspected
- THEN persona-agnostic generated outputs MUST NOT contain the known leak terms
- AND any allowed mentions MUST be limited to explicit regression-test assertions or boundary documentation that names the leak as prohibited

#### Scenario: Comment writer no longer forces Rioplatense Spanish

- GIVEN `comment-writer` drafts a Spanish comment without a regional override or clear regional context
- WHEN the comment is produced
- THEN the comment MUST NOT use Rioplatense Spanish solely because the root skill requested it
- AND the root skill MUST NOT contain a rule that forces Rioplatense Spanish for all Spanish comments
