---
name: sdd
description: Executable PI SDD chain for pi-subagents.
---

This file follows the native `pi-subagents` chain format.
Each `## <agent-name>` heading is an executable step.

## sdd-init
{{MODEL_LINE_sdd-init}}

Initialize SDD for this project when needed, or confirm existing SDD context and continue with the next phase for task: {task}

## sdd-explore
{{MODEL_LINE_sdd-explore}}

Explore and validate the problem/constraints using prior context from {previous}. Return the standard SDD result contract.

## sdd-propose
{{MODEL_LINE_sdd-propose}}

Create or refine the change proposal from {previous}. Return the standard SDD result contract.

## sdd-spec
{{MODEL_LINE_sdd-spec}}

Write or update the delta spec based on {previous}. Return the standard SDD result contract.

## sdd-design
{{MODEL_LINE_sdd-design}}

Produce/update technical design decisions from {previous}. Return the standard SDD result contract.

## sdd-tasks
{{MODEL_LINE_sdd-tasks}}

Break implementation into explicit checklist tasks from {previous}. Return the standard SDD result contract.

## sdd-apply
{{MODEL_LINE_sdd-apply}}

Implement the approved tasks from {previous} and report progress with the standard SDD result contract.

## sdd-verify
{{MODEL_LINE_sdd-verify}}

Verify implementation against proposal/spec/design/tasks from {previous}. Return the standard SDD result contract.

## sdd-archive
{{MODEL_LINE_sdd-archive}}

Archive completed change artifacts after successful verification from {previous}. Return the standard SDD result contract.

### Phase model summary (informational)

{{CHAIN_MODELS}}
