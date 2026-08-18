---
name: solution-architect
description: "Trigger: solution architect, solution-architect. Evaluates Greenfield discovery and produces a Project Blueprint."
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "1.0"
---

## Activation Contract
Use this skill when starting a completely new project (Greenfield).

## Hard Rules
- Must consult `docs/architecture-catalog.md` for standard approved patterns.
- Must read `docs/repository-registry.md` to ensure no existing microservice already solves the problem.
- Output MUST be a formal Project Blueprint ready for Architecture Gate review.

## Execution Steps
1. Ingest requirements and constraints.
2. Read the Architecture Catalog and Repository Registry.
3. Determine if a new repository is justified, or if an existing one should be extended.
4. Define the Project Blueprint (Tech stack, database strategy, owner).

## Artifact Contract
Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).
- Reads: `docs/architecture-catalog.md` and `docs/repository-registry.md` (both real files, read directly from the filesystem, not Engram); `sdd/{change-name}/proposal` (required).
- Writes: `sdd/{change-name}/blueprint` — this is a NEW artifact type, additive to the existing `explore/proposal/spec/design/tasks/apply-progress/verify-report` set. It only exists for greenfield changes; skip it entirely when extending an existing repository.

## Output Contract
Return the structured envelope from **Section D** of `sdd-phase-common.md`. The `executive_summary`/inline body must include: a Project Blueprint containing Justification, Architecture Pattern, Primary Tech Stack, Database Impact, and Proposed Repository Name.
