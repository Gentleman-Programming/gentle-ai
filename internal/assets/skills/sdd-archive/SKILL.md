---
name: sdd-archive
description: "Archive an SDD change honestly by syncing delta specs and preserving its artifacts. Trigger: orchestrator launches archive."
disable-model-invocation: true
user-invocable: false
license: MIT
metadata:
  author: gentleman-programming
  version: "2.0"
  delegate_only: true
---

## Execution Role

Confirm your role before acting. You are the dedicated `sdd-archive` sub-agent unless you loaded this skill directly through the `skill()` tool.

- If you are the `sdd-archive` sub-agent, continue with the phase work below. Do not delegate. Do not call the Skill tool.
- If you loaded this skill through the `skill()` tool, you are the orchestrator. Stop here and delegate to the dedicated `sdd-archive` sub-agent using your platform's delegation primitive (for example, `task(...)` or a sub-agent invocation).


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

## Final-State Authority

The archive report is the terminal record of the cycle. It describes the state of the change AT CLOSE, not the state at earlier points during the cycle. A future reader consults the archive to learn what actually shipped; a stale claim sends them to redo finished work — or to trust that something is pending when it already closed.

`apply-progress` and `verify-report` are intermediate snapshots. Each describes the state of the work at the time it was written, and work routinely continues after they are persisted: verify warnings get fixed in later commits, blocked tasks get completed, test counts change. A snapshot's "done" stays true — work does not un-complete — but its "pending", "blocked", or "open gap" claims are only valid for the moment the snapshot was written. Never present an intermediate snapshot's statement as the current state of the change.

When sources disagree about a fact, rank them — most authoritative first:

1. **The persisted tasks artifact** — observed completion visibility; unchecked tasks remain unchecked.
2. **Explicit final-state facts in the orchestrator's launch prompt** — e.g. "these verify warnings were fixed in later commits", "this blocker was resolved and verification passed". The launch prompt is the most recent account of the change and outranks intermediate snapshots.
3. **`verify-report` and `apply-progress`** — intermediate snapshots. Lowest rank: valid history of what was true at their time, never evidence of final state.

Reporting rules that follow:

- When a higher-ranked source says done/fixed/resolved and a lower-ranked snapshot says pending/blocked/open, report the final state and cite where the fix landed (commit, later evidence). Do NOT echo the stale claim.
- When a contradiction cannot be ranked — e.g. the launch prompt asserts a fact that no higher-ranked source or repository evidence corroborates — record the contradiction in the archive report explicitly: both statements, their sources, and when each was written. Never resolve it silently in either direction.
- Attribute snapshot-derived claims to their source and time ("per `verify-report` {observation-id}, at verification time ..."). Do not restate them in bare present tense as current facts.
- Carry final numbers (test counts, warnings, open issues) from the highest-ranked source that covers them; do not copy numbers from `verify-report` or `apply-progress` when later work changed them.
- Never merge distinct defects or failures into a single causal story. A cause is recorded as confirmed only with evidence; otherwise record the failure as undiagnosed.

This hierarchy governs reporting, not admission. Missing, stale, malformed, or failed optional reports and unfinished tasks do not block archive. Record unresolved findings and incomplete work without claiming they passed or were completed.

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from `skills/_shared/sdd-phase-common.md`.

- **engram**: Read `sdd/{change-name}/proposal`, `sdd/{change-name}/spec`, `sdd/{change-name}/design`, `sdd/{change-name}/tasks`, and `sdd/{change-name}/verify-report` when available; verification is optional. Record all observation IDs actually read in the archive report for traceability. Save as `sdd/{change-name}/archive-report`.
- **openspec**: Read and follow `skills/_shared/openspec-convention.md`. Perform merge and archive folder moves.
- **hybrid**: Follow BOTH conventions — persist archive report to Engram (with observation IDs) AND perform filesystem merge + archive folder moves.
- **none**: Return closure summary only. Do not perform archive file operations.

### Archive Readiness

Before any spec sync or archive move, require structured status. Use refreshed native SDD status and preserve actual edit permissions. Completed implementation normally recommends archive; an explicit archive request may close unfinished work without a verification certificate. SDD never offers or launches RDD; review mode is not archive state.

For a change with a sealed `kickoff.yaml`, archive additionally requires the "integration" block-review gate approved: evidence that the change actually merged or deployed toward a preproduction or production environment, never a substitute for or a shortcut past a favorable local `verify-report.md` (REQ-21.16). Native status reports this as `dependencies.archive`; when blocked, it names the missing gate and the exact command that satisfies it. Record that evidence with `axiom sdd gate record --cwd <repo> --change <name> --gate integration --decision approved --evidence-kind pr_merged|deployment|attestation [--commit <sha>] [--base-ref <ref>] [--evidence "<text>"]` before retrying archive: `pr_merged` is verified against the local commit graph, while `deployment` and `attestation` are honestly recorded as declared, not checked. Archiving with this precondition unmet is refused explicitly, and the change stays out of `openspec/changes/archive/` (REQ-21.17 introduces no new archive mechanism — only this precondition ahead of the existing one).

Archive records actual progress and findings; ordinary repository policy decides delivery.

### Honest Partial Archive

Read the available tasks and reports before closure. Record completed and unfinished tasks, missing artifacts, unresolved findings, and unrun checks. Do not repair checkboxes or rewrite historical reports during archive. Archive may close incomplete work; it must never describe that work as fully implemented or verified. No additional waiver or verification verdict is required.

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

Inspect available delta specs; preserve edit permissions and the mechanical safety checks below.

**IF mode is `engram`:** Skip filesystem sync — artifacts live in Engram only. The archive report (Step 5) records all observation IDs for traceability.

**IF mode is `none`:** Skip — no artifacts to sync.

**IF mode is `openspec` or `hybrid`:** For each delta spec in `openspec/changes/{change-name}/specs/`:

#### If Main Spec Exists (`openspec/specs/{domain}/spec.md`)

**Mandatory Native Composition (#4119):** never Read the main spec and apply
ADDED/MODIFIED/REMOVED/RENAMED sections yourself. A model-driven Read/Edit
merge is exactly how archive previously dropped unrelated requirements or
left a delta unapplied while still reporting success. Composition MUST run
through the native `sdd-archive-compose` command, which matches requirements
by name (e.g., "### Requirement: Session Expiration"), preserves every
unrelated requirement byte-for-byte, and applies RENAMED before MODIFIED
before REMOVED before ADDED so a rename is visible to a same-change MODIFIED:

```bash
axiom sdd archive-compose \
  --canonical "openspec/specs/{domain}/spec.md" \
  --delta "openspec/changes/{change-name}/specs/{domain}/spec.md" \
  --output "openspec/specs/{domain}/spec.md.compose-tmp" \
&& mv "openspec/specs/{domain}/spec.md.compose-tmp" "openspec/specs/{domain}/spec.md"
```

- A nonzero exit means the command refused: it wrote nothing, and its stderr
  names the exact section (ADDED/MODIFIED/REMOVED/RENAMED) and requirement it
  could not apply (unknown requirement name, missing `(Reason: ...)` note,
  duplicate ADDED name, or a malformed RENAMED heading). Treat this as a
  blocking failure — STOP the phase and report `blocked` with that exact
  message. Do NOT retry with a manual Read/Edit merge, and do NOT move the
  change into the archive.
- The `.compose-tmp` intermediate file plus `mv` keeps the write atomic: the
  main spec is only ever replaced by a composition the command already
  proved is complete, never by a partial write from a failed run.
- Only a zero exit is composition evidence. Include the command invocation
  in the phase result.

##### Poda Progresiva y Deprecación (ODD-4.1)

Cuando un incremento reemplaza o vuelve obsoletos requisitos de la especificación viva canonical, se debe usar la bandera `--supersede` en `axiom sdd archive-compose` en lugar de purgarlos en silencio. Esto preserva el requisito marcándolo con `[SUPERSEDED / DEPRECADO]` y su `(Reason: ...)`, evitando la doble fuente de la verdad sin romper la auditoría histórica:

```bash
axiom sdd archive-compose \
  --canonical "openspec/specs/{domain}/spec.md" \
  --delta "openspec/changes/{change-name}/specs/{domain}/spec.md" \
  --supersede \
  --output "openspec/specs/{domain}/spec.md.compose-tmp" \
&& mv "openspec/specs/{domain}/spec.md.compose-tmp" "openspec/specs/{domain}/spec.md"
```

Si además existen requisitos de especificaciones monolíticas previas que quedan superados fuera de la sección `REMOVED`, especifícalos con `--superseded-requirements "Nombre Req 1, Nombre Req 2"`.

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
source="openspec/changes/{change-name}"
destination="openspec/changes/archive/YYYY-MM-DD-{change-name}"
snapshot_root="$(mktemp -d "${TMPDIR:-/tmp}/sdd-archive.XXXXXX")"
trap 'rm -rf -- "$snapshot_root"' EXIT
cp -R "$source" "$snapshot_root/source"

# Mechanical move (MANDATORY): git mv when tracked, mv otherwise
mkdir -p openspec/changes/archive
if [ -e "$destination" ] || [ -L "$destination" ]; then
  printf 'archive destination collision: source %s and destination %s remain unchanged. Resolve the destination collision, then rerun this archive step.\n' "$source" "$destination" >&2
  exit 1
fi

if git mv "$source" "$destination"; then
  :
else
  git_mv_status=$?
  if [ -e "$source" ] || [ -L "$source" ]; then
    :
  else
    printf 'git mv failed with status %s and source %s is absent; refusing plain mv fallback.\n' "$git_mv_status" "$source" >&2
    exit "$git_mv_status"
  fi
  if diff -r "$snapshot_root/source" "$source"; then
    fallback_source_diff_status=0
  else
    fallback_source_diff_status=$?
  fi
  if [ "$fallback_source_diff_status" -ne 0 ]; then
    printf 'git mv failed with status %s and source %s changed; refusing plain mv fallback.\n' "$git_mv_status" "$source" >&2
    exit "$git_mv_status"
  fi
  if [ -e "$destination" ] || [ -L "$destination" ]; then
    printf 'archive destination collision: source %s and destination %s remain unchanged. Resolve the destination collision, then rerun this archive step.\n' "$source" "$destination" >&2
    exit 1
  fi
  if mv "$source" "$destination"; then
    :
  else
    move_status=$?
    exit "$move_status"
  fi
fi

# The source must be gone before comparing the archived tree with its snapshot.
if [ -e "$source" ] || [ -L "$source" ]; then
  printf 'archive move left the source directory in place\n' >&2
  exit 1
fi

# MANDATORY readback: only empty diff output passes.
if diff -r "$snapshot_root/source" "$destination"; then
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

The portable destination guard rejects a destination that already exists before either move attempt; it does not provide an atomic cross-process no-clobber guarantee. Do not add a suffix, overwrite, merge, delete, or otherwise choose a destination automatically.

### Historical Malformed Nesting Recovery (Manual Only)

This guidance is only for the historical malformed shape `archive/YYYY-MM-DD-{change-name}/{change-name}/`. Run this block manually only after inspecting the paths:

```bash
active_source="openspec/changes/{change-name}"
outer_destination="openspec/changes/archive/YYYY-MM-DD-{change-name}"
nested_source="$outer_destination/{change-name}"

if [ -e "$active_source" ] || [ -L "$active_source" ] ||
   [ ! -d "$outer_destination" ] || [ -L "$outer_destination" ] ||
   [ ! -d "$nested_source" ] || [ -L "$nested_source" ]; then
  printf 'historical archive recovery refused: active source must be absent, and outer destination and nested source must be real directories; all paths remain unchanged. Resolve the ambiguous shape manually.\n' >&2
  exit 1
fi

mv "$nested_source" "$active_source"
```

Never automatically delete, overwrite, or merge the outer archive directory. If the active source exists or is a symlink, the outer destination or nested source is not a real directory, or the shape is otherwise ambiguous, stop and resolve the paths manually. After the active source is restored and the collision is resolved, rerun this archive step.

### Step 4: Verify Archive

**IF mode is `openspec` or `hybrid`:** The Mechanical Copy Contract above is the verification: the verbatim `diff -r` output from Steps 2 and 3 MUST appear in the phase result, and an empty diff is the only passing evidence. In addition, confirm:
- [ ] Main specs updated correctly
- [ ] Change folder moved to archive
- [ ] Archive preserves all artifacts that existed; missing artifacts are reported
- [ ] Archived tasks retain their original bytes; completed and unfinished counts are reported honestly
- [ ] Active changes directory no longer has this change
- [ ] Verbatim `diff -r` readback output is included in the result and is empty (no differences)

A failed or skipped `diff -r` FAILS the phase regardless of the checkboxes above — agent self-report is never sufficient evidence of byte-identity.

**IF mode is `engram`:** Confirm available artifact observation IDs are recorded in the archive report; preserve the tasks observation and report unfinished work and diagnostic findings honestly.

**IF mode is `none`:** Skip verification — no persisted artifacts.

### Step 5: Persist Archive Report

**This step is MANDATORY — do NOT skip it.**

Follow **Section C** from `skills/_shared/sdd-phase-common.md`.
- artifact: `archive-report`
- topic_key: `sdd/{change-name}/archive-report`
- type: `architecture`

En proyectos gobernados por Axiom, el informe `archive-report.md` (o en Engram) DEBE redactarse en español y contener obligatoriamente la sección de trazabilidad de poda histórica (ODD-4.2):

```markdown
# Reporte de Archivado Formal: {change-name}

## Resumen de Cierre
- Fecha de Archivado: {YYYY-MM-DD}
- Estado de Verificación: {Aprobado (PASS) con evidencias formales}
- Destino en Archivo: `openspec/changes/archive/{YYYY-MM-DD}-{change-name}/`

## Especificaciones Históricas Superadas (ODD-4.2)
| Dominio | Requisito Previo | Estado | Razón de la Poda / Superación | Nueva Referencia |
|---------|------------------|--------|-------------------------------|------------------|
| {dominio} | {Requisito previo} | [SUPERSEDED] | {Explicación del reemplazo o deprecación} | `{dominio}/spec.md#{requisito}` |
*(Si no se superó ningún requisito previo, indicar explícitamente: "Ninguno. Incremento puramente aditivo.")*

## Artefactos Consolidados
- `proposal.md`
- `spec.md`
- `design.md`
- `tasks.md`
- `verify-report.md`
```

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
- proposal.md: {observed present/missing; do not infer completion}
- specs/: {observed present/missing; do not infer completion}
- design.md: {observed present/missing; do not infer completion}
- tasks.md: {present/missing}, {completed}/{total} tasks complete; {pending} unfinished

### Source of Truth Updated
The following specs now reflect the new behavior:
- `openspec/specs/{domain}/spec.md`

### SDD Cycle Complete
The change is archived. Implementation: {actual state}. Verification: {not run / partial / actual results}.
Unfinished tasks and unresolved findings: {list or none observed}.
```

## Rules

- Archival is a MECHANICAL filesystem operation: copy/move artifacts with `cp -R`/`mv`/`git mv` via the shell only, NEVER via model Read/Write — a model can truncate or alter bytes silently while reporting success, and only an independent `diff -r` catches it
- After every archive copy or move, run `diff -r` (source vs. destination, archive-report additive-only) and include its verbatim output in the phase result; an empty diff is the only passing evidence, and a skipped/missing `diff -r` FAILS the phase
- If shell access is unavailable for mechanical copy, STOP and report `blocked` — do NOT fall back to Read/Write copying
- The archive report reflects FINAL state per the Final-State Authority hierarchy: never echo stale `verify-report`/`apply-progress` claims as current facts, and record unrankable contradictions explicitly instead of resolving them silently
- ALWAYS sync delta specs BEFORE moving to archive
- When merging into existing specs, PRESERVE requirements not mentioned in the delta
- Use ISO date format (YYYY-MM-DD) for archive folder prefix
- If the merge would be destructive (removing large sections), WARN the orchestrator and ask for confirmation
- The archive is an AUDIT TRAIL — never delete or modify archived changes
- If `openspec/changes/archive/` doesn't exist, create it
- Apply any `rules.archive` from `openspec/config.yaml`
- Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.
