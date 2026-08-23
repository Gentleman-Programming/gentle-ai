---
description: Initialize SDD context — detects project stack and bootstraps persistence backend
---

If the native `sdd-init` sub-agent is available, delegate this command to it.
Otherwise, read the skill file at `~/.claude/skills/sdd-init/SKILL.md` FIRST, then follow its instructions exactly inline.

CONTEXT:
- Working directory: Detect agent-side before proceeding by running `git rev-parse --show-toplevel` with the Bash tool; if that fails, run `pwd` with the Bash tool.
- Logical Engram identity: use the canonical project returned by `mem_current_project`, not the active workspace basename. The injected Engram Project Identity Contract defines caching and fallback behavior.
- Artifact store mode: engram

TASK:
Initialize Spec-Driven Development in this project. Detect the tech stack, existing conventions, and architecture patterns. Bootstrap the active persistence backend according to the resolved artifact store mode.

ENGRAM PERSISTENCE (artifact store mode: engram):
After detecting the project context, save it:
  mem_save(title: "sdd-init/{project}", topic_key: "sdd-init/{project}", type: "architecture", project: "{project}", capture_prompt: false, content: "{detected context}")
  Set capture_prompt: false when the Engram tool schema supports it; if an older schema rejects or does not expose the field, omit it rather than failing.
topic_key enables upserts — re-running init updates, not duplicates.

Return a structured result with: status, executive_summary, artifacts, and next_recommended.
