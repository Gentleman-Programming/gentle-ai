## Engram Persistent Memory — Protocol

You have access to Engram, a persistent memory system that survives across sessions and compactions.
This protocol is MANDATORY and ALWAYS ACTIVE — not something you activate on demand.

### DECISION: Engram vs ambient context files (push vs pull) — READ THIS FIRST

Engram is **pull-based** (reactive) — it is only consulted when a search is triggered by keywords like "remember", "recall", or "what did we do". Ambient context files referenced from `AGENTS.md` (or the agent's equivalent system prompt file) via `{file:./context/name.md}` are **push-based** — always loaded into the system prompt at session start, no trigger required. **Do NOT confuse them — this is the #1 reason Engram feels broken in practice.**

**BEFORE calling `mem_save`, ask yourself:**

> "Does this information need to be ALWAYS present in every future session's context, without the user having to trigger a search?"

- **YES** → it belongs in a static file under `context/` referenced from `AGENTS.md` via `{file:./context/name.md}`. **Do NOT save it to Engram.**
- **NO** (it is episodic — you will only need it when the topic resurfaces) → save it to Engram.

**What goes in `context/` files (push / ambient):**
- **User infrastructure**: machines, hosts, IPs, credentials, SSH shortcuts, tailnet topology
- **Stack defaults**: "always use TypeScript + Drizzle + Hono for new APIs"
- **Active project list**: "current projects are X, Y, Z with these roles"
- **Shortcut resolutions**: "when the user says 'my PC', they mean hostname X"
- **User preferences applying to EVERY task**: naming conventions, commit style, preferred explanations

**What goes in Engram (pull / episodic):**
- **Architecture decisions with rationale**: "We chose Zustand over Redux because..."
- **Bug fixes with root cause**: "Fixed N+1 in UserList by adding eager loading at line X"
- **Non-trivial patterns discovered**: "Auth uses JWT with refresh token rotation in middleware X"
- **Config changes with context**: "Migrated from X to Y because of performance issue Z"

**Rule of thumb:**
- **"When the user says X, I should automatically know Y"** → ambient, use `context/`
- **"When I'm working on X, I might want to recall how we handled a similar case"** → episodic, use Engram

**If `context/` doesn't exist yet:** create it next to the `AGENTS.md` you own (e.g. `~/.config/opencode/context/` for OpenCode, `~/.claude/context/` for Claude Code, `~/.codeium/windsurf/context/` for Windsurf), add the file, and reference it from the nearest `AGENTS.md`-equivalent in a `## Personal Context (always loaded)` section using `{file:./context/name.md}`. The file is automatically expanded into the system prompt at boot — no further setup needed.

### PROACTIVE SAVE TRIGGERS (mandatory — do NOT wait for user to ask)

Call `mem_save` IMMEDIATELY and WITHOUT BEING ASKED after any of these:
- Architecture or design decision made
- Team convention documented or established
- Workflow change agreed upon
- Tool or library choice made with tradeoffs
- Bug fix completed (include root cause)
- Feature implemented with non-obvious approach
- Notion/Jira/GitHub artifact created or updated with significant content
- Configuration change or environment setup done
- Non-obvious discovery about the codebase
- Gotcha, edge case, or unexpected behavior found
- Pattern established (naming, structure, convention)
- User preference or constraint learned

Self-check after EVERY task: "Did I make a decision, fix a bug, learn something non-obvious, or establish a convention? If yes, call mem_save NOW."

Format for `mem_save`:
- **title**: Verb + what — short, searchable (e.g. "Fixed N+1 query in UserList")
- **type**: bugfix | decision | architecture | discovery | pattern | config | preference
- **scope**: `project` (default) | `personal`
- **topic_key** (recommended for evolving topics): stable key like `architecture/auth-model`
- **content**:
  - **What**: One sentence — what was done
  - **Why**: What motivated it (user request, bug, performance, etc.)
  - **Where**: Files or paths affected
  - **Learned**: Gotchas, edge cases, things that surprised you (omit if none)

Topic update rules:
- Different topics MUST NOT overwrite each other
- Same topic evolving → use same `topic_key` (upsert)
- Unsure about key → call `mem_suggest_topic_key` first
- Know exact ID to fix → use `mem_update`

### WHEN TO SEARCH MEMORY

On any variation of "remember", "recall", "what did we do", "how did we solve", "recordar", "qué hicimos", or references to past work:
1. Call `mem_context` — checks recent session history (fast, cheap)
2. If not found, call `mem_search` with relevant keywords
3. If found, use `mem_get_observation` for full untruncated content

Also search PROACTIVELY when:
- Starting work on something that might have been done before
- User mentions a topic you have no context on
- User's FIRST message references the project, a feature, or a problem — call `mem_search` with keywords from their message to check for prior work before responding

### SESSION CLOSE PROTOCOL (mandatory)

Before ending a session or saying "done" / "listo" / "that's it", call `mem_session_summary`:

## Goal
[What we were working on this session]

## Instructions
[User preferences or constraints discovered — skip if none]

## Discoveries
- [Technical findings, gotchas, non-obvious learnings]

## Accomplished
- [Completed items with key details]

## Next Steps
- [What remains to be done — for the next session]

## Relevant Files
- path/to/file — [what it does or what changed]

This is NOT optional. If you skip this, the next session starts blind.

### AFTER COMPACTION

If you see a compaction message or "FIRST ACTION REQUIRED":
1. IMMEDIATELY call `mem_session_summary` with the compacted summary content — this persists what was done before compaction
2. Call `mem_context` to recover additional context from previous sessions
3. Only THEN continue working

Do not skip step 1. Without it, everything done before compaction is lost from memory.
