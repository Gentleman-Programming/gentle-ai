---
name: figma-analyzer
description: "Trigger: figma-analyzer, design_ref, Figma design reference. Rules for analyzing a referenced Figma design and honestly reporting when it cannot be reached."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

<!-- contract: no-access-stop -->

## Technology: Figma Design Analysis

This skill is injected when an artifact's `design_ref:` frontmatter points to
a Figma design and the requesting agent implements or plans UI (see the
carrier-placement convention docs for the exact gated agents and carrier
artifacts). It governs how to use that reference safely.

## Mandatory: No Access, No Invention

If you cannot actually retrieve the referenced design, STOP and report explicitly that you do not have design access.

You MUST NOT infer or invent any design detail — structure, spacing, tokens, states, or components — as a substitute for design you cannot see.

A confidently invented design spec is strictly worse than an honest report
that you lack access: it looks authoritative, gets implemented, and produces
a UI that only accidentally matches the real design, if at all. When you
cannot see the design, say so plainly, name the `design_ref` value you were
given, and ask the user to provide the design detail directly or connect a
working retrieval path (for example the user's own Figma Dev Mode MCP
server, if one is connected to this session) before proceeding.

## When You Do Have Real Access

If a real retrieval path is connected and available to you (for example a
Figma Dev Mode MCP server, or design detail the user pastes directly into
the conversation), you may use it to read the design referenced by
`design_ref` and ground your implementation in what it actually shows:

- Read layout, spacing, and structure directly from the design; do not
  approximate from memory of "typical" designs.
- Read design tokens (color, typography, spacing scale) from the design
  system if the design exposes one, rather than guessing values.
- Note component states (default, hover, disabled, error, etc.) only when
  the design actually shows them; do not assume a state exists.
- If the design reference resolves but a specific detail you need is not
  shown (e.g. a state, a breakpoint, empty-state copy), treat that specific
  gap the same as full non-access for that detail: stop and ask, rather than
  inventing it.

## Reporting

When you stop due to no access, your report must state:

1. That you attempted to use the `figma-analyzer` skill for this task.
2. The exact `design_ref` value from the artifact, if one was present.
3. That no working retrieval path was available in this session.
4. What you need from the user to proceed (direct design detail, or a
   connected retrieval path).
