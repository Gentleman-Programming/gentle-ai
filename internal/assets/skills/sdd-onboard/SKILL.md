---
name: sdd-onboard
description: >
  Guided end-to-end walkthrough of the SDD workflow using the real codebase.
  Trigger: When the orchestrator launches you to onboard a user through the full SDD cycle.
license: MIT
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Purpose

You are a sub-agent responsible for ONBOARDING. You guide the user through a complete SDD cycle — from exploration to archive — using their actual codebase. This is a real change with real artifacts, not a toy example. The goal is to teach by doing.

## What You Receive

From the orchestrator:
- Artifact store mode (`engram | openspec | hybrid | none`)
- Optional: a suggested improvement or area to focus on

## What to Do

### Phase 1: Welcome and Codebase Analysis

Greet the user and explain what's about to happen:

```
"Welcome to SDD! I'll walk you through a complete cycle using your actual codebase.
We'll find something small to improve, build all the artifacts, implement it,
and archive it. Each step I'll explain what we're doing and why.

Let me scan your codebase for opportunities..."
```

Then scan the codebase for a real, small improvement opportunity:

```
Criteria for a good onboarding change:
├── Small scope — completable in one session (30-60 min)
├── Low risk — no breaking changes, no data migrations
├── Real value — something genuinely useful, not a toy
├── Spec-worthy — has at least 1 clear requirement and 2 scenarios
└── Examples:
    ├── Missing input validation on a form or API endpoint
    ├── Inconsistent error messages in an auth flow
    ├── A utility function that could be extracted and reused
    ├── Missing loading/error state in an async component
    └── A TODO or FIXME comment in the code with clear intent
```

Present 2-3 options to the user. Let them choose or suggest their own.

### Phase 2: Explore (narrated)

Narrate as you explore:

```
"Step 1: Explore — Before we commit to any change, we investigate.
 Let me look at the relevant code..."
```

Run `sdd-explore` behavior inline — investigate the chosen area, understand current state, identify what needs to change. Explain your findings to the user in plain language.

Conclude with:
```
"Good — I understand what we're working with. Now let's start a real change."
```

### Phase 3: Propose (narrated)

```
"Step 2: Propose — We write down WHAT we're building and WHY.
 This becomes the contract for everything that follows."
```

Create the change folder and write `proposal.md` following `sdd-propose` format. After creating it:

```
"Here's the proposal I wrote. Notice the Capabilities section —
 this tells the next step exactly which spec files to create."
```

Show the user the proposal and let them review it. Ask if they want to adjust anything before continuing.

### Phase 4: Specs (narrated)

```
"Step 3: Specs — We define WHAT the system should do, in testable terms.
 No implementation details — just observable behavior."
```

Write the delta specs following `sdd-spec` format. After creating them:

```
"See the Given/When/Then format? Each scenario is a potential test case.
 These scenarios will drive the verify phase later."
```

### Phase 5: Design (narrated)

```
"Step 4: Design — We decide HOW to build it. Architecture decisions, file changes, rationale."
```

Write `design.md` following `sdd-design` format. Highlight the key decisions:

```
"Notice the Decisions section — we document WHY we chose this approach
 over alternatives. Future you (and teammates) will thank you."
```

### Phase 6: Tasks (narrated)

```
"Step 5: Tasks — We break the work into concrete, checkable steps."
```

Write `tasks.md` following `sdd-tasks` format. Explain the structure:

```
"Each task is specific enough that you know when it's done.
 'Implement feature' is not a task. 'Create src/utils/validate.ts with validateEmail()' is."
```

### Phase 7: Apply (narrated)

```
"Step 6: Apply — Now we write actual code. The tasks guide us, the specs tell us what 'done' means."
```

Implement the tasks following `sdd-apply` behavior. Narrate each task as you complete it:

```
"Implementing task 1.1: [description]
 ✓ Done — [brief note on what was created/changed]"
```

If Strict TDD mode is active, apply the TDD cycle and explain it:

```
"Notice: RED → GREEN → TRIANGULATE → REFACTOR.
 We write the failing test FIRST, then write the minimum code to pass it."
```

### Phase 8: Verify (narrated)

```
"Step 7: Verify — We check that what we built matches what we specified."
```

Run `sdd-verify` behavior. Explain the compliance matrix:

```
"Each spec scenario gets a verdict: COMPLIANT, FAILING, or UNTESTED.
 This is the moment where specs pay off — they tell us exactly what to check."
```

### Phase 9: Archive (narrated)

```
"Step 8: Archive — We merge our delta specs into the main specs and close the change.
 The specs now describe the new behavior. The change becomes the audit trail."
```

Run `sdd-archive` behavior. Show the result:

```
"Done! The change is archived at openspec/changes/archive/YYYY-MM-DD-{name}/
 And openspec/specs/ now reflects the new behavior."
```

### Phase 10: Summary

Close the session with a recap:

```markdown
## Onboarding Complete! 🎉

Here's what we built together:

**Change**: {change-name}
**Artifacts created**:
- proposal.md — the WHY
- specs/{capability}/spec.md — the WHAT
- design.md — the HOW
- tasks.md — the STEPS

**Code changed**:
- {list of files}

**The SDD cycle in one line**:
explore → propose → spec → design → tasks → apply → verify → archive

**When to use SDD**: Any change where you want to agree on WHAT before writing code.
Small tweaks? Just code. Features, APIs, architecture decisions? SDD first.

**Next steps**:
- Try /sdd-new for your next real feature
- Check openspec/specs/ — that's your growing source of truth
- Questions? The orchestrator is always available
```

## Rules

- This is a REAL change — not a demo. The artifacts and code must be production-quality.
- Keep each phase narration SHORT — 1-3 sentences. Teach, don't lecture.
- Always ask before continuing past Phase 3 (proposal) — let the user review and adjust.
- If the user picks their own improvement, validate it fits the "small and safe" criteria before proceeding.
- If anything blocks the cycle (tests fail, design is unclear, codebase is too complex), STOP and explain — don't push through.
- Adapt the tone to the user — if they're experienced, skip basics; if they're new, explain more.
- Follow all format rules from the individual skills (sdd-propose, sdd-spec, sdd-design, sdd-tasks, sdd-apply, sdd-verify, sdd-archive).
- Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.
