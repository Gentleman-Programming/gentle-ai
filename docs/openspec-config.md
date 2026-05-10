# OpenSpec Config for SDD

`openspec/config.yaml` is a documented project-level convention for SDD in `gentle-ai` when working in `openspec` or `hybrid` persistence modes.

It is useful, and parts of the SDD prompt/skill stack look for it today, but this should not be read as a formally versioned runtime schema enforced by Go code.

## What "Support" Means Today

In the current repo, `openspec/config.yaml` support is mostly prompt-driven:

- SDD skills and orchestrator prompts tell agents to read or write this file
- `sdd-init` and shared convention examples show file shapes agents are expected to create
- later phases may reuse values such as `context`, `strict_tdd`, `rules`, and `testing`

What is NOT true today:

- there is no Go-side parser or validator that enforces a canonical `openspec/config.yaml` schema
- there is no strong compatibility contract guaranteeing every documented field is consumed uniformly across all phases
- the exact shape is still best understood as the repo's current convention, not a locked public spec

## What This File Can Customize

This file is used by the SDD skills as shared project context and as a place to declare phase-specific rules.

`openspec/config.yaml` can be used to customize SDD behavior by project conventions, specifically:

- project context reused across phases
- strict TDD enablement
- phase-specific rules for proposal, specs, design, tasks, apply, verify, and archive
- command and coverage overrides used by apply/verify prompts
- cached testing capabilities for apply/verify flows

## Which Phases Reference It

The following SDD phases explicitly reference `openspec/config.yaml` in the current prompt/skill assets:

| Phase | How it uses the config |
|-------|-------------------------|
| `sdd-init` | In OpenSpec mode, prompt instructions tell the agent to create the file and write detected `context`, `rules`, and `testing` sections. |
| `sdd-explore` | Reads it as part of project context discovery. |
| `sdd-propose` | Applies `rules.proposal` if present. |
| `sdd-design` | Applies `rules.design` if present. |
| `sdd-spec` | Applies `rules.specs` if present. |
| `sdd-tasks` | Applies `rules.tasks` if present. |
| `sdd-apply` | Reads `strict_tdd`, `testing`, and `rules.apply` if present. |
| `sdd-verify` | Reads `strict_tdd`, `testing`, and `rules.verify` if present. |
| `sdd-archive` | Applies `rules.archive` if present. |

## Synthesized Convention Example

Combining the current shared convention doc, `sdd-init` guidance, and apply/verify references, the practical top-level structure looks like this:

```yaml
schema: spec-driven

context: |
  Tech stack: ...
  Architecture: ...
  Testing: ...
  Style: ...

strict_tdd: true

rules:
  proposal:
    - Include rollback plan for risky changes
  specs:
    - Use Given/When/Then for scenarios
  design:
    - Document architecture decisions with rationale
  tasks:
    - Keep tasks completable in one session
  apply:
    - Follow existing code patterns
  verify:
    test_command: ""
    build_command: ""
    coverage_threshold: 0
  archive:
    - Warn before merging destructive deltas

testing:
  strict_tdd: true
  detected: "YYYY-MM-DD"
  runner:
    command: "go test ./..."
    framework: "Go standard testing"
```

Treat this as a practical synthesis of fields the prompt layer currently may read or emit, not as a strict schema definition.

## Field Reference

### `schema`

- Expected value in examples: `spec-driven`
- Purpose: identifies the file as an SDD/OpenSpec config in current examples and shared conventions.

### `context`

- Type: multiline string
- Purpose: cached project context for later SDD phases.
- Typical contents: stack, architecture, testing, style, and other project conventions.

### `strict_tdd`

- Type: boolean
- Referenced by: `sdd-init`, orchestrator prompts, `sdd-apply`, `sdd-verify`
- Purpose: enables or disables strict TDD behavior when testing support exists.

### `rules`

- Type: phase-keyed map
- Purpose: attach project conventions to each SDD phase.
- Known phase keys:
  - `proposal`
  - `specs`
  - `design`
  - `tasks`
  - `apply`
  - `verify`
  - `archive`

### `testing`

- Type: structured object
- Usually written by: `sdd-init`
- Referenced by: `sdd-apply`, `sdd-verify`
- Purpose: cache detected testing capabilities so phases do not have to rediscover them every time.

## Caveats

### Shape Is Not Fully Uniform

There are important inconsistencies in the current examples and skill references:

- `sdd-init` shows `rules.apply` and `rules.verify` as plain instruction lists.
- The shared OpenSpec convention doc shows `rules.apply` with `tdd` and `test_command`, and `rules.verify` with structured override keys.
- `sdd-apply/strict-tdd.md` refers to `rules.apply.test_command`.
- `sdd-verify` refers to `rules.verify.test_command`, `rules.verify.build_command`, and `rules.verify.coverage_threshold`.

That means `rules.apply` and `rules.verify` are currently treated as if they may contain structured keys, while other examples also show those same phase rules as plain lists.

### Prefer "Current Convention" Over "Stable Contract"

If you are documenting or relying on this file, the safest framing today is:

- `openspec/config.yaml` is part of the documented OpenSpec workflow
- several SDD skills and prompt assets look for it and may write to it
- the documented fields reflect current repo conventions
- consumers should not assume a formally enforced, versioned runtime schema unless the implementation adds explicit parsing/validation later
