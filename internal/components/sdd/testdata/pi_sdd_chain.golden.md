---
name: sdd
description: Executable PI SDD chain for pi-subagents.
---

This file follows the native `pi-subagents` chain format.
Each `## <agent-name>` heading is an executable step.

## sdd-init
model: anthropic/claude-sonnet-4-5

Initialize SDD for this project when needed, or confirm existing SDD context and continue with the next phase for task: {task}

## sdd-explore
model: anthropic/claude-sonnet-4-5

Explore and validate the problem/constraints using prior context from {previous}. Return the standard SDD result contract.

## sdd-propose
model: anthropic/claude-sonnet-4-5

Create or refine the change proposal from {previous}. Return the standard SDD result contract.

## sdd-spec
model: anthropic/claude-sonnet-4-5

Write or update the delta spec based on {previous}. Return the standard SDD result contract.

## sdd-design
model: anthropic/claude-sonnet-4-5

Produce/update technical design decisions from {previous}. Return the standard SDD result contract.

## sdd-tasks
model: anthropic/claude-sonnet-4-5

Break implementation into explicit checklist tasks from {previous}. Return the standard SDD result contract.

## sdd-apply
model: openai/gpt-5

Implement the approved tasks from {previous} and report progress with the standard SDD result contract.

## sdd-verify
model: anthropic/claude-sonnet-4-5

Verify implementation against proposal/spec/design/tasks from {previous}. Return the standard SDD result contract.

## sdd-archive
model: anthropic/claude-sonnet-4-5

Archive completed change artifacts after successful verification from {previous}. Return the standard SDD result contract.

### Phase model summary (informational)

- sdd-init: anthropic/claude-sonnet-4-5
- sdd-explore: anthropic/claude-sonnet-4-5
- sdd-propose: anthropic/claude-sonnet-4-5
- sdd-spec: anthropic/claude-sonnet-4-5
- sdd-design: anthropic/claude-sonnet-4-5
- sdd-tasks: anthropic/claude-sonnet-4-5
- sdd-apply: openai/gpt-5
- sdd-verify: anthropic/claude-sonnet-4-5
- sdd-archive: anthropic/claude-sonnet-4-5
- sdd-onboard: anthropic/claude-sonnet-4-5
