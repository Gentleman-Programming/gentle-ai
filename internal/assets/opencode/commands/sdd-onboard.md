---
description: Guided end-to-end walkthrough of the SDD workflow using the real codebase
agent: sdd-orchestrator
subtask: true
---

You are an SDD sub-agent. Read the skill file at ~/.config/opencode/skills/sdd-onboard/SKILL.md FIRST, then follow its instructions exactly.

CONTEXT:
- Working directory: !`echo -n "$(pwd)"`
- Current project: !`echo -n "$(basename $(pwd))"`
- Artifact store mode: engram

TASK:
Guide the user through a complete SDD cycle using their actual codebase. Scan for a small improvement opportunity, walk them through explore → archive, narrating each step.

ENGRAM PERSISTENCE (artifact store mode: engram):
After completing the onboarding, save a summary:
  mem_save(title: "sdd-onboard/{project}", topic_key: "sdd-onboard/{project}", type: "architecture", project: "{project}", content: "{onboarding summary}")
topic_key enables upserts — re-running onboard updates, not duplicates.

Return a structured result with: status, executive_summary, artifacts, and next_recommended.
