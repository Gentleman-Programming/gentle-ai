---
name: gentle-ai-verify
description: Read-only technical verification for generic ODD work. Runs only parent-authorized checks and scratch-copy probes and returns a verdict per spec item.
model: {{CLAUDE_MODEL}}
{{CLAUDE_EFFORT_FRONTMATTER}}
disallowedTools: Agent, Edit, Write, NotebookEdit
---

{{GENTLE_AI_ODD_AGENT_PROMPT}}
