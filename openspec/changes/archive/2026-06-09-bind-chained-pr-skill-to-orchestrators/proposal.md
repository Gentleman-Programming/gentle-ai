# Proposal: Bind the chained-pr skill to all SDD orchestrator assets

## Intent

Make the `chained-pr` skill (registry name `chained-pr`, frontmatter `gentle-ai-chained-pr`) the operational source of truth for PR chaining during SDD, by binding it into every SDD orchestrator asset so that `sdd-tasks` and `sdd-apply` sub-agents are required to load and follow it before planning or creating any chained/stacked PR.

Success looks like: across all 11 orchestrator templates, the orchestrator (a) keeps a short inline summary of the two chain strategies so it knows what to ask and static validation keeps passing, and (b) explicitly resolves the `chained-pr` skill by registry name and injects it into `sdd-tasks`/`sdd-apply` prompts under `## Skills to load before work`. The spec requires this binding, and static-validation tests plus golden fixtures enforce it.

## Motivation (the gap)

The SDD orchestrator templates at `internal/assets/<agent>/sdd-orchestrator.md` describe PR chaining via a `### Chain Strategy` section that documents `stacked-to-main` and `feature-branch-chain` INLINE and forwards `chain_strategy` to `sdd-tasks`/`sdd-apply`. None of them bind the existing `chained-pr` skill.

Consequence: during SDD, the apply/tasks agents improvise chaining from a thin inline summary instead of loading and following the skill that is the actual source of truth. The skill encodes operational rules the inline summary omits:

- the 400-line changed-budget limit and `size:exception` gate,
- branch targeting per strategy (tracker PR; child PR #1 targets tracker, later children target the immediate parent),
- the draft/no-merge tracker PR lifecycle,
- the per-PR Chain Context body (start, end, dependencies, follow-up, out-of-scope),
- the dependency diagram marking the current PR with the `📍` marker,
- decision gates and the per-PR verification/clean-diff contract.

Without the binding, those rules depend on each sub-agent re-deriving them, which is exactly the kind of improvisation chained PRs are meant to prevent. Additionally, `cursor/sdd-orchestrator.md` is missing the `### Chain Strategy` section entirely (it has Delivery Strategy and the Review Workload Guard but no chain strategy block), so Cursor is also below parity today.

## Scope

### In scope

- Add a skill-binding instruction to all 11 SDD orchestrator templates (`antigravity`, `claude`, `codex`, `cursor`, `gemini`, `generic`, `kimi`, `kiro`, `opencode`, `qwen`, `windsurf`): when delivery planning produces chained PRs, the orchestrator MUST resolve the `chained-pr` skill by registry name through its existing Sub-Agent Launch Pattern and inject it into `sdd-tasks` and `sdd-apply` prompts under `## Skills to load before work`, instructing those sub-agents to read and follow it BEFORE planning or creating any PR.
- Add the missing `### Chain Strategy` section to `cursor/sdd-orchestrator.md`, aligned with the other 10 templates, plus the binding.
- Keep a SHORT inline summary of `stacked-to-main` and `feature-branch-chain` in each template (the skill is the operational source of truth, but the inline summary stays so the orchestrator knows what to ask and existing static assertions keep passing).
- Update the spec `openspec/specs/sdd-orchestrator-assets/spec.md` to REQUIRE the chained-pr skill binding across orchestrator assets (new requirement plus scenarios).
- Update static-validation tests (`internal/assets/assets_test.go`, `internal/components/sdd/inject_test.go`) with a new assertion that the binding is present; add a cursor entry to the chain-strategy parity assertions.
- Regenerate the 12 golden fixtures that reference Chain Strategy (`go test ./internal/components/ -run ... -update`).

### Out of scope

- Editing the installed `~/.claude/CLAUDE.md` directly — it regenerates from these templates; changing source templates is the correct vector.
- Reworking the `chained-pr` skill content itself (`skills/chained-pr/SKILL.md` and its references) — the skill is the source of truth and is bound, not rewritten.
- Changing the chain strategy concepts, names, or the `delivery_strategy`/`Review Workload Guard` behavior.
- Touching non-orchestrator assets or other skills.

## Approach (high level)

1. Bind by registry NAME, not a hardcoded path. The orchestrator already has a Sub-Agent Launch Pattern that resolves skills from the registry and injects matching `SKILL.md` paths into sub-agent prompts. The binding instruction tells the orchestrator to treat `chained-pr` as a required skill match whenever delivery planning yields chained PRs, so the resolved path flows into `sdd-tasks`/`sdd-apply` prompts under `## Skills to load before work`. Sub-agents read the full skill file, preserving author intent and staying compaction-safe.
2. Place the binding inside (or immediately adjacent to) each template's `### Chain Strategy` section so it sits next to the inline summary and the existing `chain_strategy` forwarding wording.
3. Bring Cursor to parity by adding the full `### Chain Strategy` section (matching the other templates' canonical strategy names and forwarding wording) plus the binding.
4. Encode the requirement in the spec as a new requirement with scenarios that assert the binding is present and references the skill by registry name (no hardcoded path).
5. Lock it down with static assertions and regenerated goldens so the binding cannot silently regress.

## Affected areas

- 11 orchestrator templates: `internal/assets/{antigravity,claude,codex,cursor,gemini,generic,kimi,kiro,opencode,qwen,windsurf}/sdd-orchestrator.md` (cursor also gains the missing section).
- Spec: `openspec/specs/sdd-orchestrator-assets/spec.md` (new requirement + scenarios).
- Tests: `internal/assets/assets_test.go` (new binding assertion; add cursor to chain-strategy parity), `internal/components/sdd/inject_test.go` (binding assertion).
- Golden fixtures: the 12 golden files referencing Chain Strategy, regenerated with `-update`.

## Risks / open questions

- Golden churn: the 12 chain-strategy goldens must be regenerated; reviewers must confirm the diff is limited to the intended binding wording and the new cursor section, with no unrelated churn (the spec already forbids unrelated golden churn).
- Test breakage: existing static assertions for chain strategy must continue to pass; the new binding assertion must use wording that is identical across all 11 templates to avoid per-agent drift. Cursor currently has no `### Chain Strategy` assertion, so the parity test must add it.
- Cursor section parity: the added Cursor section must use the canonical strategy names and the same forwarding/binding wording as the other 10 to avoid alias drift flagged by the "Strategy naming remains canonical" scenarios.
- Wording uniformity vs platform accuracy: the binding sentence should be uniform, but each template's skill-resolution mechanism wording differs (Claude Agent/Task, Cursor named subagents, Kimi/Kiro/Windsurf/Antigravity platform-native). The binding must reference the skill by registry name and defer path resolution to each template's existing Sub-Agent Launch Pattern, so the uniform sentence stays accurate per platform without reintroducing inaccurate persistence/delegation claims.
- Solo-inline hosts (windsurf, antigravity, kimi, kiro): these run phases inline or via platform-native subagents rather than persisted delegation. The binding must phrase "inject under `## Skills to load before work`" in a way that maps to each host's actual phase-context mechanism, preserving the platform-native wording the spec already requires.
