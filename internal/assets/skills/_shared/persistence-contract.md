# Persistence Contract (shared across all SDD skills)

## Mode Resolution

The orchestrator passes `artifact_store.mode` with one of: `engram | openspec | hybrid | none`.

The orchestrator ASKs the user which mode they want when `/sdd-new`, `/sdd-ff`, or `/sdd-continue` is invoked for the first time in a session. The choice is cached for the session.

Default (if user doesn't specify): if Engram is available → `engram`. Otherwise → `none`.

## Mode Roles

- **`engram`**: Working memory between sessions. Upserts overwrite — no iteration history. Local only, not shareable.
- **`openspec`**: Source of truth. Files in repo, git history, team-shareable, full audit trail.
- **`hybrid`**: Both — files for team + engram for recovery. Higher token cost.
- **`none`**: Ephemeral. Lost when conversation ends.

### Mode Comparison

| Capability | `engram` | `openspec` | `hybrid` | `none` |
|------------|----------|------------|----------|--------|
| Cross-session recovery | ✅ | ❌ (needs git) | ✅ | ❌ |
| Compaction survival | ✅ | ❌ | ✅ | ❌ |
| Shareable with team | ❌ (local DB) | ✅ (committed files) | ✅ (files) | ❌ |
| Full iteration history | ❌ (upsert overwrites) | ✅ (git history) | ✅ (files + git) | ❌ |
| Audit trail (archive) | Partial (report only) | ✅ (full folder) | ✅ (both) | ❌ |
| Project files created | Never | Yes | Yes | Never |

### `engram` mode limitation

Engram uses `topic_key`-based upserts. Re-running a phase for the same change **overwrites** the previous version — no revision history is kept. The archive phase saves a summary report, not the full artifact folder. For iteration history or team collaboration, use `openspec` or `hybrid`.

## Behavior Per Mode

| Mode | Read from | Write to | Project files |
|------|-----------|----------|---------------|
| `engram` | Engram | Engram | Never |
| `openspec` | Filesystem | Filesystem | Yes |
| `hybrid` | Resolved artifact locators | Both | Yes |
| `none` | Orchestrator prompt context | Nowhere | Never |

### Hybrid Mode

Use the declared `artifactStore` and resolved `artifactPaths` from the orchestrator, as defined in `sdd-phase-common.md`. Read each resolved locator, not an Engram-first or filesystem fallback. Never silently substitute another store's copy.

Attempt both declared writes, following the Engram and OpenSpec conventions, and read back each successful write. Hybrid writes are not atomic: preserve successful writes, report any failed or unavailable mirror as partial, and name the outstanding locator. Do not claim synchronization or roll back valid progress. Preserve both versions of a genuine content conflict; ask only when it affects the next decision.

### Optional research notes

The output-only collector returns findings to the orchestrator. Persist useful research notes only through the selected store or an explicit request; `none` can return them inline. No research revision, readiness matrix or cross-store equality check admits proposal work. Report failed writes honestly and retain available progress; do not invent a successful mirror or overwrite conflicting historical research/preproposal data. Resolve a genuine content conflict only when it affects the next decision.

## Recovery (Orchestrator)

Recover from native status and the actual artifacts at its resolved locators, including full `mem_get_observation` content for topic keys. Existing `state.yaml` and `sdd/{change-name}/state` snapshots are optional recovery hints, not required per-phase writes or a second state authority. Missing or stale snapshots do not replace actual task progress or archive closure. Preserve existing snapshots, including `dependsOn` metadata, and historical artifacts.

READ-MERGE-WRITE cumulative tasks and apply-progress: retrieve the full current artifact, preserve prior completed and unrelated work, merge this batch, then persist and read back the full result.

## Common Rules

- `none` → do NOT create or modify any project files; return results inline only
- `engram` → do NOT write any project files; persist to Engram and return observation IDs
- `openspec` → write files ONLY to paths defined in `openspec-convention.md`
- `hybrid` → persist to BOTH Engram AND filesystem; follow both conventions
- NEVER force `openspec/` creation unless orchestrator explicitly passed `openspec` or `hybrid`
- If the declared store or a required locator is unresolved, report it; do not select another store.

### Optional verification reports

Persist requested diagnostics in the selected store. No report validator or attestation is required. Missing or failed verification never gates archive. Preserve historical report content and findings; archive summarizes their provenance rather than rewriting them as current passing evidence.

## Sub-Agent Context Rules

Sub-agents launch with a fresh context and NO access to the orchestrator's instructions or memory protocol.

Who reads, who writes:
- Non-SDD (general task): orchestrator searches engram, passes summary in prompt; sub-agent saves discoveries via `mem_save`
- SDD (artifact-producing phase with dependencies): sub-agent reads artifacts directly from backend; sub-agent saves its artifact
- SDD (artifact-producing phase without dependencies, e.g. explore): nobody reads; sub-agent saves its artifact
- SDD research collector: child reads no local artifacts or Engram state and saves nothing; it returns evidence for the orchestrator to validate and persist through the selected store route

Why this split:
- Orchestrator reads for non-SDD: it knows what context is relevant; sub-agents doing their own searches waste tokens on irrelevant results
- Sub-agents read for SDD: SDD artifacts are large; inlining them in the orchestrator prompt would consume the entire context window
- Artifact-producing SDD sub-agents write: they have the complete detail on what happened; nuance is lost by the time results flow back to the orchestrator
- The output-only SDD research collector returns evidence without persistence so the orchestrator can handle any authorized persistence without a research-readiness gate

## Orchestrator Prompt Instructions for Sub-Agents

Non-SDD:
```
PERSISTENCE (MANDATORY):
If you make important discoveries, decisions, or fix bugs, you MUST save them to engram before returning:
  mem_save(title: "{short description}", type: "{decision|bugfix|discovery|pattern}",
           project: "{project}", content: "{What, Why, Where, Learned}")
Do NOT return without saving what you learned. This is how the team builds persistent knowledge across sessions.
```

SDD (artifact-producing phases):
```
Artifact store: {declared artifactStore}
Artifact locators: {resolved artifactPaths and output locators}
Read required dependencies at those locators; topic keys require project-scoped
mem_search followed by full mem_get_observation, never a search preview.
Persist the full artifact through the declared store as in sdd-phase-common.md:
files for openspec, canonical topic keys for engram, both for hybrid, inline for none.
Read back writes and return their actual locators and outcomes. Report partial
persistence honestly; never force an Engram write for openspec or none.
Preserve prior progress with READ-MERGE-WRITE rather than replacing a batch history.
```

For SDD artifacts, `capture_prompt: false` is explicit and mandatory when the Engram tool schema supports it. Engram v1.15.3 defaults `capture_prompt` to true for normal human/proactive saves, but automated pipeline artifacts must not capture the user's prompt. Do not infer this from `type` because SDD artifacts and real human architecture decisions both use `architecture`. If an older schema rejects or does not expose `capture_prompt`, omit it rather than failing.

## Sub-Agent Response Ordering

When a sub-agent persists artifacts (via `mem_save` or file writes), the persistence call MUST happen BEFORE the final text response. The sub-agent's absolute last output must be text, never a tool call.

**Why**: The Task tool returns the sub-agent's final output to the parent. If the sub-agent ends with a tool call, the parent receives only the tool result (e.g., `"Observation saved"`) — the sub-agent's text analysis is lost. Always: do your work → save → respond with text envelope.

Sub-agents must NOT call `mem_session_summary` — that's reserved for top-level agents only.

## Skill Registry

The orchestrator pre-resolves skill paths from the skill registry and injects them as `## Skills to load before work` in your launch prompt. Sub-agents read those exact `SKILL.md` files before task-specific work.

To generate/update: run the `skill-registry` skill, or run `sdd-init`.

Sub-agent skill loading: check for a `## Skills to load before work` block in your prompt — if present, read those exact files. If not present, check for `SKILL: Load` instructions as a fallback. If neither exists, proceed without — this is not an error.

## Detail Level

The orchestrator may pass `detail_level`: `concise | standard | deep`. This controls output verbosity but does NOT affect what gets persisted — always persist the full artifact.
