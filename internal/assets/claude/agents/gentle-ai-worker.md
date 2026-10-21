---
name: gentle-ai-worker
description: Scoped implementation writer for bounded ODD work. Edits code inside the parent's allowed edit surfaces, runs focused tests, and returns review-ready evidence without committing.
model: {{CLAUDE_MODEL}}
{{CLAUDE_EFFORT_FRONTMATTER}}
disallowedTools: Agent
---

{{GENTLE_AI_ODD_AGENT_PROMPT}}
