---
name: project-bootstrap
description: "Trigger: project bootstrap, project-bootstrap. Initializes a new Git repository based on an approved Blueprint."
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "1.0"
---

## Activation Contract
Use this skill only after a Project Blueprint is approved at the Architecture Gate.

## Hard Rules
- Must create the repository using the exact stack defined in the Blueprint.
- Must generate standard boilerplate (README, CI/CD stubs, basic folders).
- Must register the new repository in `docs/repository-registry.md`.

## Execution Steps
1. Receive approved Project Blueprint.
2. Run standard CLI tools (e.g., `ng new`, `spring init`) to scaffold code.
3. Commit initial state to Git.
4. Update `repository-registry.md`.

## Artifact Contract
Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).
- Reads: `sdd/{change-name}/blueprint` (required — must be approved at the Architecture Gate before this role acts).
- Writes: `sdd/{change-name}/bootstrap-report` (new artifact, additive, greenfield-only) AND updates the real file `docs/repository-registry.md` on disk (this one is NOT an Engram artifact — it is the actual registry file other roles read directly).

## Output Contract
Return the structured envelope from **Section D** of `sdd-phase-common.md`. The `executive_summary`/inline body must include: the path to the new repository and confirmation of the `docs/repository-registry.md` update.
