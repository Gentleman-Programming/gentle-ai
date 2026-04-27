## Engram Persistent Memory — Protocol

You have access to Engram, a persistent memory system that survives across sessions and compactions.
This protocol is MANDATORY and ALWAYS ACTIVE — not something you activate on demand.

### PROACTIVE SAVE TRIGGERS (mandatory — do NOT wait for user to ask)

Call `mem_save` IMMEDIATELY and WITHOUT BEING ASKED after any of these:
- Architecture or design decision made
- Team convention documented or established
- Workflow change agreed upon
- Tool or library choice made with tradeoffs
- Bug fix completed (include root cause)
- Feature implemented with non-obvious approach
- Configuration change or environment setup done
- Non-obvious discovery, gotcha, or edge case
- Pattern established (naming, structure, convention)
- User preference or constraint learned

Self-check after EVERY task: "Did I make a decision, fix a bug, learn something non-obvious, or establish a convention? If yes, call mem_save NOW."

### MANDATORY SAVE FORMAT

Saved observations are only useful if `mem_search` finds them later. Every step below is mandatory — do NOT improvise.

#### 1. Dedupe before save

Call `mem_search` with the candidate `topic_key` first. If a hit exists → `mem_update`. Otherwise continue.

If unsure which key to use → call `mem_suggest_topic_key`.

#### 2. `type` — closed table

| `type`            | Title prefix     | When                                              |
|-------------------|------------------|---------------------------------------------------|
| `bugfix`          | `Bugfix:`        | Bug fixed, with root cause                        |
| `decision`        | `Decision:`      | Choice between alternatives with tradeoff         |
| `pattern`         | `Pattern:`       | Reusable convention or pattern                    |
| `config`          | `Config:`        | Setup, install, tooling configuration             |
| `discovery`       | `Discovery:`     | Non-obvious finding or gotcha                     |
| `preference`      | `Preference:`    | User preference or constraint                     |
| `proposal`        | `Proposal:`      | SDD proposal phase                                |
| `spec`            | `Spec:`          | SDD spec phase                                    |
| `design`          | `Design:`        | SDD design phase                                  |
| `tasks`           | `Tasks:`         | SDD tasks phase                                   |
| `apply-progress`  | `Apply:`         | SDD apply phase (implementation log)              |
| `verify-report`   | `Verify:`        | SDD verify phase                                  |
| `archive-report`  | `Archive:`       | SDD archive phase                                 |
| `session_summary` | `Session:`       | End-of-session summary                            |

Do NOT invent new types. The legacy `architecture` type is REMOVED — SDD artifacts use the dedicated phase types; non-SDD architectural decisions go under `decision`. If content fits no row, it should not be saved.

#### 3. `title` — fixed format

```
<Prefix>: <ticket-id?> <subject> — <qualifier?>
```

- Prefix MUST come from the table.
- `ticket-id` immediately after the prefix when applicable.
- `subject` MUST be in **English**, even if the conversation is in another language. Search runs cross-project; English is the common denominator. Body language is free.
- No emojis, no decorative quotes. Under 80 chars.

Examples: `Bugfix: TICKET-42 null check in user profile loader`, `Pattern: cache invalidation on write-through`, `Decision: chose Postgres over MySQL for analytics`, `Session: 2025-01-15 auth refactor planning`.

#### 4. `topic_key` — MANDATORY

Path-style, lowercase, kebab-case: `<domain>/<scope>/<slug>`.

| Case             | Pattern                                  |
|------------------|------------------------------------------|
| SDD artifact     | `sdd/<change-name>/<phase>`              |
| Bug fix          | `bugfix/<module>/<short-slug>`           |
| Pattern          | `pattern/<area>/<short-slug>`            |
| Config           | `config/<tool>/<short-slug>`             |
| Decision         | `decision/<area>/<short-slug>`           |
| Discovery        | `discovery/<area>/<short-slug>`          |
| Preference       | `preference/<area>/<short-slug>`         |
| Session summary  | `session/<yyyy-mm-dd>/<topic-slug>`      |

Same topic evolving → reuse the key (upsert). Different topics MUST NOT overwrite each other.

#### 5. `content` — fixed structure

```
**What**: <one sentence — what was done>
**Why**: <motivation — request, bug, perf, deadline, etc.>
**Where**: <files / paths / URLs>
**Learned**: <gotchas, edge cases — omit if none>
```

No `Tags` field — `topic_key` plus a normalized title cover findability.

#### 6. `scope`

`project` (default) | `personal` (truly cross-project preferences/patterns only).

#### Pre-save self-check

- [ ] Did I `mem_search` the topic_key first?
- [ ] Is `type` in the closed table?
- [ ] Title prefix matches the type, subject is English, under 80 chars?
- [ ] `topic_key` set and path-style?
- [ ] Body has What / Why / Where?

Any "no" → fix before saving.

#### Migration

Existing observations are NOT bulk-migrated; they stay searchable via full-text. Convention applies to new saves and to `mem_update` of existing ones (also fix their `type` and `topic_key` on update).

### WHEN TO SEARCH MEMORY

On any variation of "remember", "recall", "what did we do", "how did we solve", or references to past work:
1. `mem_context` — recent session history (fast, cheap)
2. If not found, `mem_search` with relevant keywords
3. `mem_get_observation` for full untruncated content

Also search PROACTIVELY when starting work that might have been done before, when the user mentions a topic you have no context on, or when their FIRST message references a project/feature/problem.

### SESSION CLOSE PROTOCOL (mandatory)

Before ending a session or saying "done" / "listo", call `mem_session_summary` with:

```
## Goal
[What we were working on this session]

## Discoveries
- [Technical findings, gotchas, non-obvious learnings]

## Accomplished
- [Completed items with key details]

## Next Steps
- [What remains for the next session]

## Relevant Files
- path/to/file — [what it does or what changed]
```

This is NOT optional. If you skip this, the next session starts blind.

### AFTER COMPACTION

If you see a compaction message or "FIRST ACTION REQUIRED":
1. IMMEDIATELY call `mem_session_summary` with the compacted summary content — persists what was done before compaction
2. Call `mem_context` to recover additional context
3. Only THEN continue

Skipping step 1 loses everything done before compaction.
