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

## Carrier Placement

`design_ref:` frontmatter is only actionable when it reaches an agent that
can use it. The dev-orchestrator's artifact-type gate (`AllowedArtifactTypes`)
refuses an artifact whose derived type is not allowed for the target agent
*before the prompt is ever built* — this skill never gets a chance to run:

| Gated agent | Usable `design_ref` carrier filenames |
|---|---|
| `frontend-implementer` | `tasks.md`, `spec.md` |
| `solution-architect` | `explore.md` |

Placing `design_ref:` on `proposal.md` or `design.md` is silently useless:
neither filename derives an artifact type either gated agent accepts, so the
artifact is refused before this skill is ever injected. This is a
documentation constraint only — no artifact type is added or widened to work
around it.

No agent outside this table ever receives `figma-analyzer`. The gate is
closed and exhaustive, not a heuristic: an unlisted agent gets nothing,
regardless of whether `design_ref:` is present on the artifact it reads.

## Value Shape

Only a value that parses as an HTTPS `figma.com`/`www.figma.com` URL with a
recognized path prefix (`file`, `design`, or `proto`) and a file key matching
`^[A-Za-z0-9]{8,64}$` is recognized; anything else is treated as absent. That
charset is deliberately **stricter** than Figma's real key alphabet: a real
key can be rejected (fails safe to "no reference"), but nothing outside this
charset can ever be accepted. Do not loosen it to accept more real keys —
that trade only runs in the false-negative direction. What renders into
`<context_package>` as `design_ref:` is always the reconstructed canonical
form `https://www.figma.com/design/<file-key>[?node-id=<node-id>]`, never the
raw frontmatter value.

## What This Skill Does Not Do

Real Figma retrieval is **not implemented** by this skill or by any part of
the dev-orchestrator today. `design_ref:` only tells you *which* design was
meant — it does not connect you to it. The "No Access, No Invention" rule
below is not an edge-case fallback; it is the expected path for every
session, unless you already have your own separate retrieval connection (for
example a Figma Dev Mode MCP server, or design detail the user pastes
directly into the conversation).

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
