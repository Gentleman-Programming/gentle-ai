# Conditional recovery from an unavailable correction request
Follow-up to #6086 / #6087. Owner approved manual eligible abandonment and executable journey coverage.
Delivery: stacked-to-main PR slices, each <=400 authored changed lines. Real RDD remains off.

## Specs
S1. Owner approved: "Recomiendo que, **cuando abandonar sea elegible**, se informe esa salida como recuperación manual; cuando no lo sea, se mantenga la parada actual. Sin abandonar automáticamente ni desactivar RDD." Reply: "si".
S2. Owner approved: "¿Avanzo con esa distinción y la prueba del ejecutable, separando los cambios en PRs si exceden el límite de 400 líneas?" Reply: "si".

S3. Owner approved: "¿Aprobás ese criterio: arreglo acotado de v9 y mantener v1 sin la continuación nueva?" Reply: "Si". Scope: exact two correction-release reasons, closed v9 continuation schema, no v1 schema or historical v7/v8 changes, real envelope and negative validation tests.

S4. Owner approved: "¿Avanzo también con ese arreglo separado de compatibilidad v1?" Reply: "si". Accepted proposal: "arreglar también la respuesta v1 en un cambio separado, respetando su esquema congelado y sin inventar recibos"; existing PR stays scoped to v9 repair and continuation suppression, with explicit evidence of pre-existing v1 defects.

## Tasks
T1 | S1 | inline | done | conditional eligibility probe, release continuation, eligible/ineligible and unchanged-state tests, documentation | commit: 945b80290
T2 | S2 | inline | done | driven zero-budget journey against built executable, corpus/CI registration | commit: 945b80290
T3 | S1-S2 | independent verifier | done | isolated driven proof, focused tests, exact public differential | commit: 945b80290
T4 | S1-S2 | parent | pending | Linux references, stacked PR CI/review and delivery | commit: pending

T5 | S3 | inline / independent verifier | done | published v9 schema repair and frozen-v1 compatibility, RED/GREEN and focused re-verification | commit: 19866a939

T6 | S4 | inline / independent verifier | done | truthful frozen-v1 serialization implemented; six boundary regression cases independently pass; executable closure recovery authorized | commit: pending

## Log
L1. Original accepted proposal: "La revisión confirmó **dos pendientes reales** en #6087; no lo mergeé aunque el CI esté verde:\n\n1. **Falta documentar una salida válida:** algunas revisiones con presupuesto cero permiten `review abandon`, pero solo si pasan sus controles y hay autorización explícita. Eso descarta la revisión; **no la corrige ni aprueba los cambios**.\n2. **Falta un caso de prueba contra el ejecutable:** el test actual cubre ese escenario dentro del paquete CLI, pero las corridas de bench no prueban ese caso concreto. Haber pasado los otros journeys no cubre esa ausencia.\n\n**Hay una decisión de comportamiento:** hoy ese stop se anuncia como terminal y falla de la herramienta. Recomiendo que, **cuando abandonar sea elegible**, se informe esa salida como recuperación manual; cuando no lo sea, se mantenga la parada actual. Sin abandonar automáticamente ni desactivar RDD.\n\n¿Avanzo con esa distinción y la prueba del ejecutable, separando los cambios en PRs si exceden el límite de 400 líneas?"
L2. User: "si".
L3. Starting commit 71bb14d49d149ba371a0971c74e7a693384d5da7 has green CI, including Linux baseline tests. Existing PR approximately 350 changed lines. Forecast: behavior/tests/docs 170-275; journey 120-210. Keep separate delivery slices.
L5. T1 RED: job-28 exit 1, eligible stop lacked continuation. Implementation now probes only the typed request refusal, attaches the existing release continuation, admits only this additional stop reason, and updates narration/contracts. Added explicit ineligible and absent-probe cases. First GREEN job-29 failed compilation on AgentID/string; corrected conversion. Job-30 checks are pending. No new delivery or completion claim.
L4. Read-only exploration: newReviewForecast and strict status validation define every stop as terminal within its lineage. Existing correction-context stop already offers conditional release while retaining terminal forecast. Preserve that contract: conditional manual recovery does not resume the frozen lineage. Only the per-reason documentation classification needs to reflect caller recovery. Reuse reviewCorrectionReleaseContinuation unchanged, probe only ErrCorrectionPlanRequestUnbuildable; inspection failure means no continuation.

L6. Job-30 failed the shipped generic continuation documentation check; named the flagless abandon command explicitly in the generic contract, preserving the Pi facade-only wording. Job-31 passed focused CLI routing, stop-contract and continuation tests plus git diff --check. T1 is not closed: differential/independent proof remains. Explorer mwd21afo-18-o9sv maps T2 public journey helpers; no source changes delegated.

L7. T2 implemented as j129 with separate fixture/capture helper, manifest, constructor collision registry, central review precondition and CI assertion. Job-32 failed corpus registration before driven execution; corrected missing central precondition. Job-33 build/vet/bench tests passed, then built-base driven RED exited 1 specifically: eligible stop omitted manual release. Evidence /private/tmp/correction-release-bench.Jv8ryE/red.json. Job-34 builds changed binary and runs GREEN; pending.

L8. Job-34 built changed executable and j129 completed (exit 0), git diff --check passed. No model calls; synthetic reviewer results exercise public CLI in isolated HOME. Evidence green.json beside red.json. Full driven corpus job-35 and independent verifier mwd27zf4-19-9fm4 (including differential) running. T1/T2 retain open verification dependencies; not delivered.

L9. Independent verifier mwd2eb3i-1a-50ao: S1/S2 met, no actionable code blocker. Corrected preparation succeeded; focused tests, bench build/vet/tests, independently built j129 and public differential passed; one incorrect sibling-probe assumption retained as failure evidence before correct provider-context comparisons. Authority byte hashes unchanged, exact flagless command refuses with binding template. Parent full corpus: 53 completed, zero unsupported/failed. No full root suite; Linux baseline refresh pending. Actual abandonment intentionally not performed.
L10. Delivery boundary: original #6087 ends at 71bb14d49; follow-up branch fix/correction-unavailable-manual-release starts there. Commit 945b80290 contains only recovery and its executable coverage, 281 changed lines including this task document. Follow-up PR targets original branch until #6087 merges, then retarget main with a clean incremental diff. No oversized exception.

L11. User approved scoped in-place v9 correction and frozen-v1 continuation suppression: "Si". Source review found closed inherited schemas reject both correction release continuations; reproduce against published schema before fix. Prior CI green is not evidence of schema compatibility. Reopen verification for S3; original S1/S2 implementation evidence retained.

L12. Jobs 42/43 demonstrated v9 continuation rejection and undesired v1 continuation. Initial GREEN attempts hit test-helper regex/URL preparation failures, corrected by reusing the existing regexp engine. Job-46 then exposed missing receipt / extra eligible_untracked_inventory in actual v1 envelopes. Job-47 reproduced both plus continuation on baseline 0e2a7c704 in /private/tmp/v1-schema-baseline.bFNXqc. No schema thaw or receipt invention. User approved separate v1 repair with "si"; explorer mwdbe4li-1c-dc81 maps truthful serializer adaptation.

L13. Jobs 48 RED and 49/50 GREEN established truthful v1 receipt not_applicable without identity, v2-only inventory/frozen digest, and preserved legacy/corrupt authority. Independent sweep mwdbp45z-1e-w5dz found three blockers: escaped CR / Unicode blank schema parity, generated manifest metadata, and root disposition. Job-52 reproduced all six boundary cases. Jobs 53/54 failed due regex Unicode encoding and premature manifest projection; both failures preserved. Final projection occurs only after full validation, immediately before encoding. Job-55 broad schema checks and diff check passed.

L14. Independent bounded recheck mwdcaghb-1f-yl72: six regression cases passed (1.529s), both builds passed; separate executable probes never ran because prior temporary schema_check.go was absent. No residual product defect reproduced; no unconditional completion or publication. Worker stopped without repair loop.

L15. User: "si recupera y deja todo listo please". Accepted bounded recovery: same verifier reconstructs isolated helper, at most one preparation correction, only missing public v1/v9 and unaffected v2 exact differential/state checks, no product changes. Task mwdd5m9k-1g-0ou5 running. Preserve evidence until explicit release. Delivery remains blocked on proof and CI; separate S3/S4 slices required.

L16. User authorized fresh preparation: "si, pero porque tantas recuperaciones? quiero que esto termine". Independent verifier mwddn2v9-1h-rpdv completed all four missing checks: S3/S4 met, first-attempt mandatory preparation, 4 negative v9 boundaries rejected / 2 valid accepted, both built v1 reproductions validate, 3 exact v2 controls and 12 authority hash checks pass. Durable evidence: /private/tmp/status-schema-verify.o8WAAv/final-evidence/. No additional product correction requested. T5/T6 implementation and verification complete; delivery remains T4.
L17. Split S3 from S4 as approved. First slice isolated CLI tests passed (job-57, 33.925s); format command failed because checkout-index snapshot lacked Git metadata, not a source format finding. Added a local Git index to the snapshot; job-58 runs the unchanged formatting check. No tests weakened.

L18. First delivery slice committed 19866a939, 162 additions+deletions, published as #6103 targeting #6101. Isolated tests pass; job-58 identified one extra blank line from splitting the test; formatted index content and job-59 gofmtcheck/diff checks passed. Remaining S4 source is unchanged from the independently verified candidate; receipt/serialization explanation added to docs. New slice branch fix/frozen-v1-status-serialization.
