---
name: sdd-archive
description: "Archive a completed SDD change by syncing delta specs. Trigger: orchestrator launches archive after implementation and verification."
disable-model-invocation: true
user-invocable: false
license: MIT
metadata:
  author: gentleman-programming
  version: "2.0"
  delegate_only: true
---

> **ORCHESTRATOR GATE**: If you loaded this skill via the `skill()` tool, you are
> the ORCHESTRATOR — STOP. Do NOT execute these instructions inline. Delegate to
> the dedicated `sdd-archive` sub-agent using your platform's delegation primitive
> (e.g., `task(...)`, sub-agent invocation, etc.). This skill is for EXECUTORS
> only.

## Executor Override

If you ARE the `sdd-archive` sub-agent (NOT the orchestrator), the gate above does NOT apply to you. Continue with the phase work below. Do NOT delegate. Do NOT call the Skill tool. You are the executor — execute.


## Language Domain Contract

Generated technical artifacts default to English. Do not inherit the user's conversational language or the active persona's regional voice for SDD artifacts unless the user explicitly requests that artifact language or the project convention requires it.

If technical artifacts are explicitly requested in another language, use a neutral/professional register unless the user explicitly requests a different tone or regional variant.

Public/contextual comments follow the target context language by default. Explicit user language or tone overrides win; otherwise use a neutral/professional register unless the target context clearly calls for another tone or regional variant.

## Purpose

You are a sub-agent responsible for ARCHIVING. You merge delta specs into the main specs (source of truth), then move the change folder to the archive. You complete the SDD cycle.

## What You Receive

From the orchestrator:
- Change name
- Artifact store mode (`engram | openspec | hybrid | none`)
- Structured status from `skills/_shared/sdd-status-contract.md`, including artifact paths, task progress, dependency states, and actionContext
- Explicit final-state facts for work completed after intermediate artifacts were persisted (verify warnings fixed in later commits, blockers resolved, updated test counts), when the orchestrator has them
- Any explicit intentional archive override text from the user/orchestrator

## Final-State Authority

The archive report is the terminal record of the cycle. It describes the state of the change AT CLOSE, not the state at earlier points during the cycle. A future reader consults the archive to learn what actually shipped; a stale claim sends them to redo finished work — or to trust that something is pending when it already closed.

`apply-progress` and `verify-report` are intermediate snapshots. Each describes the state of the work at the time it was written, and work routinely continues after they are persisted: verify warnings get fixed in later commits, blocked tasks get completed, test counts change. A snapshot's "done" stays true — work does not un-complete — but its "pending", "blocked", or "open gap" claims are only valid for the moment the snapshot was written. Never present an intermediate snapshot's statement as the current state of the change.

When sources disagree about a fact, rank them — most authoritative first:

1. **Native review authority** — structured status `reviewGate`, the terminal receipt, and post-apply gate context. Validated delivery facts; they win for everything they cover.
2. **The persisted tasks artifact** — completion visibility, per the Task Completion Gate below.
3. **Explicit final-state facts in the orchestrator's launch prompt** — e.g. "these verify warnings were fixed in later commits", "this blocker was resolved and the gate passed". The launch prompt is the most recent account of the change and outranks intermediate snapshots.
4. **`verify-report` and `apply-progress`** — intermediate snapshots. Lowest rank: valid history of what was true at their time, never evidence of final state.

Reporting rules that follow:

- When a higher-ranked source says done/fixed/resolved and a lower-ranked snapshot says pending/blocked/open, report the final state and cite where the fix landed (commit, later evidence). Do NOT echo the stale claim.
- When a contradiction cannot be ranked — e.g. the launch prompt asserts a fact that no higher-ranked source or repository evidence corroborates — record the contradiction in the archive report explicitly: both statements, their sources, and when each was written. Never resolve it silently in either direction.
- Attribute snapshot-derived claims to their source and time ("per `verify-report` {observation-id}, at verification time ..."). Do not restate them in bare present tense as current facts.
- Carry final numbers (test counts, warnings, open issues) from the highest-ranked source that covers them; do not copy numbers from `verify-report` or `apply-progress` when later work changed them.
- Never merge distinct defects or failures into a single causal story. A cause is recorded as confirmed only with evidence; otherwise record the failure as undiagnosed.

This hierarchy governs how the archive REPORTS facts. It does not weaken gates: CRITICAL issues in `verify-report` still block archive with no prompt override (a claim that a CRITICAL was fixed requires re-running `sdd-verify`, not a prompt assertion), and the Native Review Receipt Gate and Task Completion Gate below keep their own authority rules.

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from `skills/_shared/sdd-phase-common.md`.

- **engram**: Read `sdd/{change-name}/proposal`, `sdd/{change-name}/spec`, `sdd/{change-name}/design`, `sdd/{change-name}/tasks`, and `sdd/{change-name}/verify-report` (all required). Read the exact `sdd/{change-name}/review/{transaction,ledger,receipt,gate-context}` topics only when the Native Review Receipt Gate below finds `reviewGate` present (a review was actually discovered) — when `reviewGate` is structurally absent, no review ever happened for this candidate and none of those topics exist to read. Record all observation IDs actually read in the archive report for traceability. Save as `sdd/{change-name}/archive-report`.
- **openspec**: Read and follow `skills/_shared/openspec-convention.md`. Perform merge and archive folder moves.
- **hybrid**: Follow BOTH conventions — persist archive report to Engram (with observation IDs) AND perform filesystem merge + archive folder moves.
- **none**: Return closure summary only. Do not perform archive file operations.

### Native Review Receipt Gate

Before any task reconciliation, spec sync, or archive move, require structured status. `reviewGate` is a structurally ABSENT key — not a populated value — in every case except a genuine, discovered review artifact for this candidate:

- **`reviewGate` absent, archive proceeds under ordinary repository policy** in both of these cases; there is no `disabled/unmanaged` value to check for, and no explicit-artifact carve-out either:
  - **the kill switch is off**: receipt-driven development does not exist for this candidate, so zero review code ran and there is nothing to read or block on.
  - **the kill switch is on, verify has passed, and no review was ever started for this candidate**: the post-verify offer (`reviewOffer`) is present in the SAME status output — an invitation, never a gate. Declining is proceeding to archive without acting on it, not a verb; nothing about the decline is recorded, and `dependencies.archive: ready` here means proceed, not "investigate why the gate is missing".
- **`reviewGate` present with `result: allow`** (a discovered receipt that governs this candidate and validates): proceed. Read the exact transaction, frozen ledger, approved terminal receipt, and post-apply gate context referenced by status; the receipt must match final candidate tree, paths digest, policy, ledger, fix delta, current independent verification evidence, mode counters, and base relationship.
- **`reviewGate` present with any other result** (pending, malformed, `scope-changed`, `invalidated`, or `escalated` — a review was actually discovered and failed validation): blocks archive with no override and no automatic reviewer launch. The gate never manufactures `allow`, and re-enabling a disabled switch revalidates from the current state.

Do not treat `reviewGate`'s absence itself as a defect to investigate or as grounds to demand a receipt — only a present, non-`allow` value blocks.

### Task Completion Gate

`sdd-apply` is responsible for marking completed tasks in the persisted tasks artifact. `sdd-archive` is responsible for validating that the persisted artifact reflects the final state before closing the cycle.

Before syncing specs or moving any archive folder, inspect the tasks artifact:

- **engram**: read the full `sdd/{change-name}/tasks` observation.
- **openspec/hybrid**: read `openspec/changes/{change-name}/tasks.md`.

If any implementation task remains unchecked (`- [ ]`):

1. STOP and return `blocked`; do not sync specs, move the change folder, or claim the SDD cycle is complete.
2. Report that `sdd-apply` must be rerun or corrected so it marks completed tasks in the persisted tasks artifact.
3. Only proceed if the orchestrator explicitly instructs you to reconcile stale checkboxes and `apply-progress`/`verify-report` prove every unchecked task is complete. If you do this exceptional repair, record the exact reconciliation reason in the archive report.

The archived audit trail MUST NOT contain stale unchecked tasks for completed work. Internal todo state is not enough; the persisted SDD task artifact is the source of truth for completion visibility.

### Strict-vs-OpenSpec Archive Policy

OpenSpec permits archiving with incomplete artifacts or tasks after a user confirmation. gentle-ai is stricter by default:

- Incomplete implementation tasks block archive unless they are stale checkboxes and apply-progress/verify-report prove completion.
- CRITICAL issues in `verify-report` always block archive. Do not accept an override for CRITICAL verification issues.
- `sdd-archive` does not own normal task completion. `sdd-apply` owns checkbox completion; archive may only perform exceptional mechanical reconciliation with proof from apply-progress and verify-report.
- Missing proposal/spec/design artifacts should be reported. Archive may continue only when the user explicitly chooses an intentional partial archive and the archive report records what was missing.

### Action Context Guard

- If structured status reports `actionContext.mode: workspace-planning`, STOP. Do not move workspace changes into repo-local archives or edit linked repos.
- If `allowedEditRoots` is present, archive operations must stay inside those roots.

## Mechanical Copy Contract (MANDATORY)

Archival is a mechanical filesystem operation. File content MUST NEVER pass through the model's Read/Write path to be copied — a model that summarizes, truncates, or alters even one byte while reporting success corrupts the audit trail silently. The only acceptable copy mechanism is a native shell command (`cp -R`, `mv`, or `git mv`), verified by a structural readback.

- Copy artifacts with the shell only: `cp -R`, `mv`, or `git mv`. NEVER use Read → Write to reproduce artifact content into the archive or main specs — that routes bytes through model generation, where truncation is silent and undetectable without an independent diff.
- After every copy or move, run `diff -r` (source vs. destination) as a MANDATORY readback. The archive-report file is additive-only and excluded from the source/destination comparison (it did not exist in the source change folder).
- The verbatim `diff -r` output MUST appear in the phase result. An empty `diff -r` (no differences) is the only passing evidence; any difference is a truncation or alteration and FAILS the phase. A skipped or missing `diff -r` also FAILS the phase — agent self-report is never sufficient.
- If your platform's tool allowlist does not grant shell access, STOP and report `blocked` with the reason `shell access required for mechanical archive copy is unavailable` — do NOT fall back to Read/Write copying.

## What to Do

### Step 1: Load Skills
Follow **Section A** from `skills/_shared/sdd-phase-common.md`.

### Step 2: Sync Delta Specs to Main Specs

Do not start this step until the **Task Completion Gate** above passes.

**IF mode is `engram`:** Skip filesystem sync — artifacts live in Engram only. The archive report (Step 5) records all observation IDs for traceability.

**IF mode is `none`:** Skip — no artifacts to sync.

**IF mode is `openspec` or `hybrid`:** For each delta spec in `openspec/changes/{change-name}/specs/`:

#### If Main Spec Exists (`openspec/specs/{domain}/spec.md`)

Read the existing main spec and apply the delta:

```
FOR EACH SECTION in delta spec:
├── ADDED Requirements → Append to main spec's Requirements section
├── MODIFIED Requirements → Replace the matching requirement in main spec
├── REMOVED Requirements → Delete the matching requirement from main spec after recording Reason/Migration
└── RENAMED Requirements → Rename the matching requirement while preserving scenarios unless the delta also modifies them
```

**Merge carefully:**
- Match requirements by name (e.g., "### Requirement: Session Expiration")
- Preserve all OTHER requirements that aren't in the delta
- Maintain proper Markdown formatting and heading hierarchy
- For REMOVED requirements, require `(Reason: ...)` and `(Migration: ...)` notes in the delta before deleting from main specs
- For RENAMED requirements, require the old and new requirement names to be explicit

#### If Main Spec Does NOT Exist

The delta spec IS a full spec (not a delta). Copy it mechanically with the shell — do NOT Read the file and Write its content back, which routes bytes through the model and can truncate silently:

```bash
# Mechanical copy (MANDATORY): never Read → Write artifact content
target_dir="openspec/specs/{domain}"
target_path="$target_dir/spec.md"
mkdir -p "$target_dir"

temp_path=
cleanup_temp() {
  if [ -n "$temp_path" ]; then
    rm -f "$temp_path" || :
  fi
}
trap cleanup_temp EXIT
temp_path="$(mktemp "$target_dir/.spec.md.XXXXXX")"

if cp "openspec/changes/{change-name}/specs/{domain}/spec.md" "$temp_path"; then
  :
else
  copy_status=$?
  exit "$copy_status"
fi

if diff -r "openspec/changes/{change-name}/specs/{domain}/spec.md" "$temp_path"; then
  diff_status=0
else
  diff_status=$?
fi
if [ "$diff_status" -ne 0 ]; then
  exit "$diff_status"
fi

if mv "$temp_path" "$target_path"; then
  temp_path=
else
  move_status=$?
  exit "$move_status"
fi
# Empty diff above is the only passing evidence; include verbatim output in the result.
```

### Step 3: Move to Archive

**IF mode is `engram`:** Skip — there are no `openspec/` directories to move. The archive report in Engram serves as the audit trail.

**IF mode is `none`:** Skip — no filesystem operations.

**IF mode is `openspec` or `hybrid`:** Move the entire change folder to archive with date prefix, using a mechanical shell move. NEVER Read each artifact and Write it into the archive — that routes file content through the model and can truncate or alter bytes silently:

```bash
# Run this block as one shell transaction so the EXIT trap remains active.
# The snapshot is recursive and must be created before either move attempt.
snapshot_root="$(mktemp -d "${TMPDIR:-/tmp}/sdd-archive.XXXXXX")"
trap 'rm -rf -- "$snapshot_root"' EXIT
cp -R "openspec/changes/{change-name}" "$snapshot_root/source"

# Mechanical move (MANDATORY): git mv when tracked, mv otherwise
mkdir -p openspec/changes/archive
if git mv openspec/changes/{change-name} openspec/changes/archive/YYYY-MM-DD-{change-name}; then
  :
else
  if mv openspec/changes/{change-name} openspec/changes/archive/YYYY-MM-DD-{change-name}; then
    :
  else
    move_status=$?
    exit "$move_status"
  fi
fi

# The source must be gone before comparing the archived tree with its snapshot.
if [ -e "openspec/changes/{change-name}" ] || [ -L "openspec/changes/{change-name}" ]; then
  printf 'archive move left the source directory in place\n' >&2
  exit 1
fi

# MANDATORY readback: only empty diff output passes.
if diff -r "$snapshot_root/source" "openspec/changes/archive/YYYY-MM-DD-{change-name}"; then
  diff_status=0
else
  diff_status=$?
fi
if [ "$diff_status" -ne 0 ]; then
  exit "$diff_status"
fi
```

Use today's date in ISO format (e.g., `2026-02-16`).

The `snapshot_root` is removed safely by the EXIT trap after the readback, including when the move or comparison fails. Compare the archived folder against that pre-move recursive snapshot; do not substitute a model readback, staged tree, or post-move source. The `archive-report` you write in Step 5 is additive and excluded from the comparison because it did not exist in the source snapshot. Any non-empty `diff -r` output or non-zero status is truncation, alteration, or an operational failure and FAILS the phase; a missing `diff -r` also FAILS the phase.

### Step 4: Verify Archive

**IF mode is `openspec` or `hybrid`:** The Mechanical Copy Contract above is the verification: the verbatim `diff -r` output from Steps 2 and 3 MUST appear in the phase result, and an empty diff is the only passing evidence. In addition, confirm:
- [ ] Main specs updated correctly
- [ ] Change folder moved to archive
- [ ] Archive contains all artifacts (proposal, specs, design, tasks)
- [ ] Archived `tasks.md` has no unchecked implementation tasks, unless the orchestrator explicitly approved archive-time stale-checkbox reconciliation backed by apply-progress/verify-report proof
- [ ] Active changes directory no longer has this change
- [ ] Verbatim `diff -r` readback output is included in the result and is empty (no differences)

A failed or skipped `diff -r` FAILS the phase regardless of the checkboxes above — agent self-report is never sufficient evidence of byte-identity.

**IF mode is `engram`:** Confirm all artifact observation IDs are recorded in the archive report and the tasks observation has no unchecked implementation tasks unless the orchestrator explicitly approved archive-time stale-checkbox reconciliation backed by apply-progress/verify-report proof.

**IF mode is `none`:** Skip verification — no persisted artifacts.

### Step 5: Persist Archive Report

**This step is MANDATORY — do NOT skip it.**

Follow **Section C** from `skills/_shared/sdd-phase-common.md`.
- artifact: `archive-report`
- topic_key: `sdd/{change-name}/archive-report`
- type: `architecture`

### Step 6: Return Summary

Return to the orchestrator:

```markdown
## Change Archived

**Change**: {change-name}
**Archived to**: `openspec/changes/archive/{YYYY-MM-DD}-{change-name}/` (openspec/hybrid) | Engram archive report (engram) | inline (none)

### Specs Synced
| Domain | Action | Details |
|--------|--------|---------|
| {domain} | Created/Updated | {N added, M modified, K removed requirements} |

### Archive Contents
- proposal.md ✅
- specs/ ✅
- design.md ✅
- tasks.md ✅ ({N}/{N} tasks complete)

### Source of Truth Updated
The following specs now reflect the new behavior:
- `openspec/specs/{domain}/spec.md`

### SDD Cycle Complete
The change has been fully planned, implemented, verified, and archived.
Ready for the next change.
```

## Rules

- Archival is a MECHANICAL filesystem operation: copy/move artifacts with `cp -R`/`mv`/`git mv` via the shell only, NEVER via model Read/Write — a model can truncate or alter bytes silently while reporting success, and only an independent `diff -r` catches it
- After every archive copy or move, run `diff -r` (source vs. destination, archive-report additive-only) and include its verbatim output in the phase result; an empty diff is the only passing evidence, and a skipped/missing `diff -r` FAILS the phase
- If shell access is unavailable for mechanical copy, STOP and report `blocked` — do NOT fall back to Read/Write copying
- The archive report reflects FINAL state per the Final-State Authority hierarchy: never echo stale `verify-report`/`apply-progress` claims as current facts, and record unrankable contradictions explicitly instead of resolving them silently
- NEVER archive a change that has CRITICAL issues in its verification report
- If the user explicitly approves a non-critical partial archive or stale-checkbox reconciliation, record the exact reason in the archive report and mark the archive as intentional-with-warnings
- NEVER archive completed work while `tasks.md` / the tasks observation still shows stale unchecked implementation tasks
- ALWAYS sync delta specs BEFORE moving to archive
- When merging into existing specs, PRESERVE requirements not mentioned in the delta
- Use ISO date format (YYYY-MM-DD) for archive folder prefix
- If the merge would be destructive (removing large sections), WARN the orchestrator and ask for confirmation
- The archive is an AUDIT TRAIL — never delete or modify archived changes
- If `openspec/changes/archive/` doesn't exist, create it
- Apply any `rules.archive` from `openspec/config.yaml`
- Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.
