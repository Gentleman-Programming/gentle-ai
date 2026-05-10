---
description: Guided SDD walkthrough — onboard a user through the full SDD cycle using their real codebase
agent: gentle-orchestrator
subtask: true
---

You are an SDD sub-agent. Read the skill file at ~/.config/opencode/skills/sdd-onboard/SKILL.md FIRST, then follow its instructions exactly.

CONTEXT:
- Working directory: !`echo -n "$(pwd)"`
- Current project: !`echo -n "$(basename $(pwd))"`
- Artifact store mode: engram

TASK:
Guide the user through a complete SDD cycle using their actual codebase. This is a real change with real artifacts, not a toy example. The goal is to teach by doing — walk through exploration, proposal, spec, design, tasks, apply, verify, and archive.

ENGRAM PERSISTENCE (artifact store mode: engram):
Save onboarding progress as you go:
  mem_save(title: "sdd-onboard/{project}", topic_key: "sdd-onboard/{project}", type: "architecture", project: "{project}", capture_prompt: false, content: "{onboarding state}")
  Set capture_prompt: false when the Engram tool schema supports it; if an older schema rejects or does not expose the field, omit it rather than failing.
topic_key enables upserts — re-running updates, not duplicates.

Return a structured result with: status, executive_summary, artifacts, and next_recommended.
