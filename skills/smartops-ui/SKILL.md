---
name: smartops-ui
description: "Trigger: implement a Vue screen/component from a Figma design using SmartOps UI. Single entrypoint /smartops-ui — no stages, no ledger, no sub-agents."
license: Apache-2.0
metadata:
  author: JhuniorBrayan123
  version: "1.0"
---

# smartops-ui

Implement a Vue screen or component from a Figma reference using the `smartops-ui`
design system, in the fewest possible MCP calls and context reads.

## Required tools
- Figma MCP (design context for the given frame)
- SmartOps UI MCP (`get_figma_mapping`, `get_component`, `search_components`/`list_components`,
  `get_architecture_rules`, `get_usage_guides`)

If either MCP is not configured in the consumer project, stop and report it — do not
substitute with assumptions.

## Inputs
1. Figma reference (frame URL or node-id) — required.
2. Target path in the consumer repo — required.

## Flow (max 6 steps)
1. Resolve inputs. Check once whether the consumer repo has its own local
   architecture/convention doc — confirm it exists, don't read it yet.
2. Fetch Figma design context for the given frame, once.
3. Call `get_figma_mapping`, once. Primary source for Figma→component translation.
4. Resolve anything the mapping leaves ambiguous or in `unmappedInFigma`
   (`search_components`/`list_components` as fallback only). For each component
   actually selected, call `get_component` once if its real props/events/slots are
   needed to implement it correctly — never for components not being used. In this
   step, also read — narrowly — the local business-rule/architecture fragment for
   the target module only.
5. Implement the target file(s) following the local convention read in step 4.
6. Run the project's real verification scripts (typecheck/lint/build from
   package.json) and report the result.

## Tool usage rules
- `get_figma_mapping`: once per session, cached in session memory.
- `get_component`: only for components actually used, only if their real shape is
  needed.
- `get_usage_guides`: ALWAYS call with an explicit `name` argument. Never call it
  with no argument — the server concatenates every guide file when `name` is
  omitted.
- `get_architecture_rules`: only if the consumer repo has no local architecture doc
  of its own.
- Local repo reads: only the business-rule/convention fragment for the target
  module, and the real script names in `package.json`.
- No manual verification via curl or browser navigation — only the project's own
  scripts.

## Anti-patterns
- Do not re-read a project's full orchestrator doc or unrelated business rules.
- Do not call `get_usage_guides()` without `name`.
- Do not call `get_component` for a component not used in this implementation.
- Do not write a new Markdown file that copies `figma-map.json` or any SmartOps MCP
  output.
- Do not spawn sub-agents per step — one thread executes steps 1-6.
- Do not call the same MCP tool with the same argument twice in one session.
- Do not save to Engram after each step.

## Result contract
Report: files created/modified; SmartOps components used and how each mapping was
resolved; any `unmappedInFigma` elements and how they were resolved; verification
command(s) and result; blockers or next steps. Never report "done" if verification
failed or is incomplete.

## Engram policy
- Optional one `mem_search` at the start (check for a prior reusable decision about
  this exact screen/component).
- One `mem_save` at the end, only if something genuinely reusable across sessions
  was learned — never per step.

## Out of scope
- `documentacion-pantallas-figma` stays a separate, optional standalone tool.
- Project-specific orchestrators, business rules, module docs, and API contracts
  stay local to the consumer repo — read narrowly, never ported into Gentle-AI.
