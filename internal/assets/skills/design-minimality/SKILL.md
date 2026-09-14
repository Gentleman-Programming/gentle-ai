---
name: design-minimality
description: "Trigger: design minimality, minimal plan, organic planning, YAGNI. Run a minimality ladder over requirements to produce the smallest viable implementation plan."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## When to Use

Load this skill during organic delegation between exploration and writer implementation.

Use it when:
- Exploration has mapped codebase context and execution paths, but implementation has not started.
- Deciding how much engineering the requested change truly deserves before composing a writer brief.
- Preventing speculative abstractions, unnecessary layers, or unneeded custom utilities.
- Producing the smallest viable plan that satisfies the user's explicit outcome.

## The Minimality Ladder

Evaluate every proposed part of the implementation sequentially through the minimality ladder. Stop at the lowest step that satisfies the requirement:

1. **YAGNI (You Aren't Gonna Need It)**: Does this capability, abstraction, or configuration need to exist at all for the stated goal? If not, drop it immediately.
2. **Codebase Reuse**: Does an existing function, helper, type, or pattern in the codebase already do this or can it be reused directly without modification?
3. **Standard Library**: Does the programming language's standard library provide a native way to achieve this without bespoke code?
4. **Native Platform Feature**: Does the operating system, runtime, shell, or host platform natively offer this capability?
5. **Installed Dependency**: Does an already-declared, already-installed dependency in the project solve this problem without adding new libraries?
6. **Minimum New Code**: Only after steps 1–5 are exhausted, write the minimum targeted new code necessary to deliver the outcome.

## Output Contract

Every design-minimality pass returns a structured artifact containing:

1. **Minimal Viable Implementation Plan**:
   - Step-by-step list of targeted files and exact modifications.
   - Specific functions, types, or configuration keys to add or adjust.

2. **Estimated Authored-Line Footprint**:
   - Projected additions plus deletions.
   - Verification that the change remains comfortably within the 400-line review budget.
   - Slicing recommendations if the minimal solution naturally approaches or exceeds 400 lines.

3. **Deliberate Deferrals Table**:
   Record every intentional cut or deferral explicitly so the human owns the scope:

   | Item | Ceiling | Trigger / When to Revisit |
   |------|---------|---------------------------|
   | [Cut or deferred capability] | [Current boundary or limit] | [Condition or event that justifies adding it] |

4. **Writer Brief Format**:
   - A crisp, self-contained handoff for the writer subagent.
   - Lists exact files to touch, required tests, and strict boundary invariants.
   - Forbids out-of-scope refactoring or speculative enhancements.

## Critical Guardrails

- **Shorten the solution, never the understanding**: Minimality applies to the authored code footprint, not to the depth of exploration or verification.
- **Never remove critical invariants**: Never drop or simplify input validation at trust boundaries, error handling that prevents data loss, security controls, or accessibility requirements.
- **Preserve explicit requirements**: Never cut or compromise an outcome explicitly requested by the user.
- **Budget constrains slicing, never the code (anti-code-golf)**: Do not compress readable, idiomatic code into dense one-liners or omit necessary tests to fit a line budget. If a clean implementation exceeds the budget, slice it into chained work units instead.
