# Conditional recovery from an unavailable correction request
Follow-up to #6086 / #6087. Owner approved manual eligible abandonment and executable journey coverage.
Delivery: stacked-to-main PR slices, each <=400 authored changed lines. Real RDD remains off.

## Specs
S1. Owner approved: "Recomiendo que, **cuando abandonar sea elegible**, se informe esa salida como recuperación manual; cuando no lo sea, se mantenga la parada actual. Sin abandonar automáticamente ni desactivar RDD." Reply: "si".
S2. Owner approved: "¿Avanzo con esa distinción y la prueba del ejecutable, separando los cambios en PRs si exceden el límite de 400 líneas?" Reply: "si".

## Tasks
T1 | S1 | inline | in_progress | conditional eligibility probe, release continuation, eligible/ineligible and unchanged-state tests, documentation | commit: pending
T2 | S2 | inline | pending | driven zero-budget journey against built executable, corpus/CI registration | commit: pending
T3 | S1-S2 | independent verifier | pending | isolated driven proof, focused tests, review and delivery slices | commit: pending

## Log
L1. Original accepted proposal: "La revisión confirmó **dos pendientes reales** en #6087; no lo mergeé aunque el CI esté verde:\n\n1. **Falta documentar una salida válida:** algunas revisiones con presupuesto cero permiten `review abandon`, pero solo si pasan sus controles y hay autorización explícita. Eso descarta la revisión; **no la corrige ni aprueba los cambios**.\n2. **Falta un caso de prueba contra el ejecutable:** el test actual cubre ese escenario dentro del paquete CLI, pero las corridas de bench no prueban ese caso concreto. Haber pasado los otros journeys no cubre esa ausencia.\n\n**Hay una decisión de comportamiento:** hoy ese stop se anuncia como terminal y falla de la herramienta. Recomiendo que, **cuando abandonar sea elegible**, se informe esa salida como recuperación manual; cuando no lo sea, se mantenga la parada actual. Sin abandonar automáticamente ni desactivar RDD.\n\n¿Avanzo con esa distinción y la prueba del ejecutable, separando los cambios en PRs si exceden el límite de 400 líneas?"
L2. User: "si".
L3. Starting commit 71bb14d49d149ba371a0971c74e7a693384d5da7 has green CI, including Linux baseline tests. Existing PR approximately 350 changed lines. Forecast: behavior/tests/docs 170-275; journey 120-210. Keep separate delivery slices.
L5. T1 RED: job-28 exit 1, eligible stop lacked continuation. Implementation now probes only the typed request refusal, attaches the existing release continuation, admits only this additional stop reason, and updates narration/contracts. Added explicit ineligible and absent-probe cases. First GREEN job-29 failed compilation on AgentID/string; corrected conversion. Job-30 checks are pending. No new delivery or completion claim.
L4. Read-only exploration: newReviewForecast and strict status validation define every stop as terminal within its lineage. Existing correction-context stop already offers conditional release while retaining terminal forecast. Preserve that contract: conditional manual recovery does not resume the frozen lineage. Only the per-reason documentation classification needs to reflect caller recovery. Reuse reviewCorrectionReleaseContinuation unchanged, probe only ErrCorrectionPlanRequestUnbuildable; inspection failure means no continuation.

L6. Job-30 failed the shipped generic continuation documentation check; named the flagless abandon command explicitly in the generic contract, preserving the Pi facade-only wording. Job-31 passed focused CLI routing, stop-contract and continuation tests plus git diff --check. T1 is not closed: differential/independent proof remains. Explorer mwd21afo-18-o9sv maps T2 public journey helpers; no source changes delegated.

L7. T2 implemented as j129 with separate fixture/capture helper, manifest, constructor collision registry, central review precondition and CI assertion. Job-32 failed corpus registration before driven execution; corrected missing central precondition. Job-33 build/vet/bench tests passed, then built-base driven RED exited 1 specifically: eligible stop omitted manual release. Evidence /private/tmp/correction-release-bench.Jv8ryE/red.json. Job-34 builds changed binary and runs GREEN; pending.

L8. Job-34 built changed executable and j129 completed (exit 0), git diff --check passed. No model calls; synthetic reviewer results exercise public CLI in isolated HOME. Evidence green.json beside red.json. Full driven corpus job-35 and independent verifier mwd27zf4-19-9fm4 (including differential) running. T1/T2 retain open verification dependencies; not delivered.
