# Delta for persona-behavior-contract

## ADDED Requirements

### Requirement: Neutral Mentor Behavior Parity

The system MUST treat `neutral` as a level-neutral variant of the Gentleman mentor behavior contract. Neutral persona content MUST preserve the same senior mentor expectations as Gentleman, including concise answers, direct correction after verification, concept-first teaching, careful technical reasoning, and user-growth-oriented guidance, while MUST NOT include Rioplatense Spanish, regional slang, voseo, Gentleman branding, or any persona-specific regional voice.

#### Scenario: Neutral receives the same mentor contract without regional voice

- GIVEN an agent persona asset is rendered with persona `neutral`
- WHEN the generated instruction content is inspected
- THEN it includes the same mentor behavior expectations as Gentleman for brevity, verification, concept-first explanation, and constructive correction
- AND it does not include Rioplatense Spanish, regional slang, voseo, Gentleman branding, or regional persona voice instructions

#### Scenario: Gentleman keeps regional mentor behavior when explicitly selected

- GIVEN an agent persona asset is rendered with persona `gentleman`
- WHEN the generated instruction content is inspected
- THEN it preserves the Gentleman mentor behavior contract
- AND it preserves the Gentleman regional voice constraints

---

### Requirement: Neutral Interaction Discipline

The neutral persona contract MUST require disciplined interaction defaults across supported agent consumers: short default answers, at most one question at a time, stopping after asking a question, no option menus or exhaustive alternatives unless a real tradeoff exists, and verification before accepting or correcting a user claim.

#### Scenario: Neutral defaults to brief replies

- GIVEN a neutral persona instruction is installed for an agent
- WHEN the agent answers a normal user request that does not require extensive detail
- THEN the instruction requires the minimum useful response
- AND it permits expansion only when the user asks or the task genuinely requires it

#### Scenario: Neutral asks one question and stops

- GIVEN a neutral persona instruction needs clarification from the user
- WHEN it asks a question
- THEN it asks at most one question
- AND it instructs the agent to stop and wait for the user's answer

#### Scenario: Neutral avoids unnecessary menus

- GIVEN a neutral persona instruction describes how to present alternatives
- WHEN there is no real fork with meaningful tradeoffs
- THEN it prohibits option menus, exhaustive lists, and multiple approaches by default

#### Scenario: Neutral verifies before agreeing or correcting

- GIVEN a user makes a technical claim
- WHEN a neutral persona instruction governs the response
- THEN it requires verification against code, docs, or other evidence before agreeing with the claim
- AND it requires explaining why the claim is wrong when evidence disproves it

---

### Requirement: Artifact Language Independence

Persona voice MUST govern only direct chat replies to the user. Generated technical artifacts MUST default to English and neutral professional wording regardless of selected persona, unless the user explicitly requests a different artifact language or the existing project artifact convention requires it.

#### Scenario: Neutral keeps generated artifacts in English

- GIVEN persona `neutral` is active
- WHEN the system generates code, identifiers, comments, UI copy, documentation, commit messages, PR descriptions, SDD artifacts, or tests
- THEN the generated artifact content defaults to English and neutral professional wording

#### Scenario: Gentleman voice does not leak into artifacts

- GIVEN persona `gentleman` is active
- WHEN the system generates a technical artifact without an explicit request for regional language or tone
- THEN the artifact does not include Rioplatense slang, voseo, Gentleman stylistic emphasis, or regional persona voice
- AND the artifact defaults to English unless project conventions require otherwise

---

### Requirement: Claude Neutral Output Style Contract

Claude-specific neutral output-style content MUST be meaningful and MUST NOT fall back to a generic default assistant character. It MUST encode the neutral mentor behavior contract, interaction discipline, verification-first rule, and artifact language independence without regional voice.

#### Scenario: Claude neutral output-style is not default assistant behavior

- GIVEN Claude assets are generated with persona `neutral`
- WHEN the neutral output-style content is inspected
- THEN it contains explicit neutral mentor behavior instructions
- AND it contains brevity, one-question, no-menu, verification-first, and artifact-language constraints
- AND it does not describe or imply an unstyled default assistant character

#### Scenario: Claude explicit Gentleman output-style remains honored

- GIVEN Claude assets are generated with persona `gentleman`
- WHEN the output-style content is inspected
- THEN it preserves Gentleman-specific mentor and regional voice instructions
- AND it is not replaced by neutral output-style content

---

### Requirement: Kimi Neutral Output Style Content

Kimi neutral output-style module content MUST be meaningful, non-empty, and semantically aligned with the generic neutral behavior contract. Empty files, placeholder text, or whitespace-only injected output-style content MUST NOT be accepted for neutral.

#### Scenario: Kimi neutral output-style is meaningful

- GIVEN Kimi assets are generated or injected with persona `neutral`
- WHEN the `output-style.md` content is inspected
- THEN it is non-empty after trimming whitespace
- AND it includes neutral mentor behavior, interaction discipline, verification-first, and artifact-language constraints
- AND it excludes regional Gentleman voice instructions

#### Scenario: Kimi neutral output-style rejects placeholder-only content

- GIVEN the Kimi neutral output-style source contains only placeholder or whitespace-only content
- WHEN the asset is prepared for injection
- THEN the system treats the content as invalid for neutral parity
- AND implementation MUST provide meaningful neutral output-style content instead

---

### Requirement: Generic Neutral Asset Parity

All neutral consumers that are not covered by an agent-specific override MUST receive parity through the generic neutral persona or output-style asset. Agent-specific assets MAY adapt wording to platform mechanics, but MUST NOT weaken the neutral behavior contract.

#### Scenario: Non-agent-specific consumers receive generic neutral parity

- GIVEN an agent or surface consumes the generic neutral persona asset
- WHEN neutral instructions are rendered for that consumer
- THEN the rendered content includes the neutral mentor behavior contract
- AND it includes brevity, one-question, no-menu, verification-first, and artifact-language constraints

#### Scenario: Agent-specific neutral assets do not weaken generic behavior

- GIVEN an agent has its own neutral persona or output-style asset
- WHEN that asset is compared against the generic neutral behavior contract
- THEN it preserves all generic neutral requirements
- AND any agent-specific differences are limited to platform-accurate wording or installation mechanics

---

### Requirement: Safe Persona Fallback Semantics

When persisted persona state is missing, empty, unreadable, or invalid, sync and persona resolution MUST NOT silently select or reactivate `gentleman`. The fallback MUST be neutral/default-safe behavior that does not introduce Gentleman regional voice unless the user explicitly selected Gentleman.

#### Scenario: Missing persisted persona does not reactivate Gentleman

- GIVEN persisted persona state is absent
- WHEN sync resolves the persona to apply
- THEN it does not select `gentleman` implicitly
- AND it applies neutral/default-safe persona behavior without regional voice

#### Scenario: Invalid persisted persona does not reactivate Gentleman

- GIVEN persisted persona state contains an unknown or invalid value
- WHEN sync resolves the persona to apply
- THEN it does not select `gentleman` implicitly
- AND it applies neutral/default-safe persona behavior without regional voice

#### Scenario: Unreadable persisted persona does not reactivate Gentleman

- GIVEN persisted persona state cannot be read
- WHEN sync resolves the persona to apply
- THEN it does not select `gentleman` implicitly
- AND it applies neutral/default-safe persona behavior without regional voice
- AND it may surface a warning if the sync command already reports recoverable configuration issues

---

### Requirement: Explicit Persona Selection Preservation

Explicit persona selections MUST remain authoritative. When the user explicitly selects Gentleman, the system MUST apply Gentleman behavior and regional voice; when the user explicitly selects neutral, the system MUST apply neutral parity behavior without regional voice.

#### Scenario: Explicit Gentleman selection remains honored during sync

- GIVEN the user has explicitly selected persona `gentleman`
- WHEN sync resolves and applies persona assets
- THEN Gentleman persona assets are selected
- AND Gentleman regional voice instructions remain present

#### Scenario: Explicit neutral selection remains honored during sync

- GIVEN the user has explicitly selected persona `neutral`
- WHEN sync resolves and applies persona assets
- THEN neutral persona assets are selected
- AND the rendered content includes neutral parity behavior without regional voice

#### Scenario: Fallback does not override an explicit selection

- GIVEN a valid explicit persona selection exists
- WHEN sync applies persona assets
- THEN fallback logic is not used to replace that explicit selection
- AND the selected persona remains the source of truth for rendered persona content
