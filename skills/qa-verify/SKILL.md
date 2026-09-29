---
name: qa-verify
description: "Trigger: validar la implementación QA contra el spec. Pruebas funcionales y evidencia (G6); la revisión de código/diff se delega a RDD."
license: Apache-2.0
metadata:
  author: JhuniorBrayan123
  version: "3.0"
disable-model-invocation: true
user-invocable: false
---

## Activation Contract

Carga esta skill cuando `qa-supervisor` te delegue validar una implementación QA contra su spec. Eres el sub-agente de **validación funcional y evidencia (G6)** del orquestador QA. NO corriges el código: reportas hallazgos verificables.

## Fuentes de verdad (MANDATORY)

- **Spec** (`qa/{change}/spec`) = contrato de validación.
- **BookStack = fuente de la verdad**: el resultado debe compararse contra la documentación consultada; cita las páginas usadas (G1).
- **Engram = memoria persistente**: registra el progreso en `qa/{change}/verify-report`.
- **Ledger nativo = el gate real**: `verify` no requiere aprobación manual (el humano ya aprobó en spec), pero el ledger DEBE cerrarse para que la cadena avance a `docs`.

## Entrada (obligatorio, antes de validar)

1. Ejecuta `gentle-ai qa-begin --change {change} --stage verify --cwd <repo> --request-id <id-idempotente>`.
2. Si el comando se rechaza, **DETENTE** y devuelve el rechazo tal cual al orquestador/supervisor.
3. Solo si `qa-begin` responde con éxito continúa con la validación de abajo.

## Revisión de código real (RDD) para los ítems 3-4 del checklist G6

A diferencia de v2, esta skill **no reimplementa** verificaciones de código/diff que Gentle-AI 3.7 ya resuelve mejor (secretos, riesgo, diff, mantenibilidad). Para los ítems 3 (lint) y 4 (secretos) de abajo, invoca el ciclo real de RDD directamente sobre el diff que produjo `qa-apply` — el mismo protocolo que ya usa el orquestador para sus propios commits, no una integración Go separada. El stub `QACodeReviewer`/`RDDAdapter` de 3A.11 se retiró en 3C.3 (código muerto: el ciclo real de RDD es interactivo y con estado — consentimiento humano, revisores LLM por lente — y no cabe en una llamada síncrona Go sin depender igual de un agente externo por debajo).

0. **Antes de todo**: `gentle-ai review mode status` (solo lectura). Si el modo efectivo es `disabled`, anota en el reporte "RDD deshabilitado por el usuario — ítems 3-4 no aplicables" y sigue con el resto del checklist sin insistir. Si está habilitado (default), continúa.
1. **Preflight (siempre primero)**: `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent <runtime> --next-transition`. Rutea **solo** desde el `next_transition` que devuelve — nunca inventes un comando desde memoria o de una corrida anterior.
2. Si `next_transition` es `execute` con `operation: "review.start"`, ejecútalo tal cual, con sus argumentos exactos.
3. Si START devuelve un envelope `gentle-ai.review-integration.consent/v3`, preséntaselo íntegro al humano (headline, razón, cada opción con su efecto) y espera su respuesta — nunca decidas `granted`/`declined` en su nombre. Si declina, anota en el reporte que la revisión de código fue omitida por decisión humana y sigue con el resto del checklist; eso no bloquea el cierre de `verify`.
4. Si START confirma la revisión (`state: "reviewing"`), sigue el `next_transition` de STATUS — normalmente un `collect` con uno o más `review.capture-result` por lente. Ejecuta cada uno tal cual (nunca agregues `--input`; capturan en proceso).
5. Al aprobarse, el último capture devuelve `review.acknowledge-approved` — ejecútalo exactamente una vez. Registra los hallazgos (si los hay) en el reporte; ningún hallazgo de RDD bloquea `verify` por sí solo, salvo que tú (checklist funcional G6) también hayas fallado la prueba.

## Checklist funcional G6 (obligatoria, TODA)

Los ítems 1, 2, 5, 6, 7 y 8 se delegan a la herramienta standalone
`qa-evidence` (ver `skills/qa-evidence/SKILL.md`) — invócala con el
`{change}` actual para que el bundle de evidencia quede adjunto a esta
etapa. No los reimplementes inline.

1. `npx tsc --noEmit` — tipos compilan (vía `qa-evidence`).
2. Ejecutar la prueba modificada/creada — pasa (vía `qa-evidence`).
3. Lint del proyecto — vía el ciclo real de RDD de arriba (o "no aplicable" si RDD está deshabilitado).
4. Credenciales/secretos en el diff — vía el ciclo real de RDD de arriba (o "no aplicable" si RDD está deshabilitado).
5. Verificar que NO haya esperas fijas innecesarias (vía `qa-evidence`).
6. Screenplay+POM — criterio completo en `skills/qa-evidence/SKILL.md` (Ítem 6) (vía `qa-evidence`).
7. Comparar el resultado contra la documentación BookStack consultada (vía `qa-evidence`).
8. Entregar el comando de ejecución y el resultado obtenido (vía `qa-evidence`).

## Evidencia reproducible (G6)

Producida por `qa-evidence` (ítems 1,2,5,6,7,8 de arriba) — ver
`skills/qa-evidence/SKILL.md`.

## Salida (obligatorio, cierre de la etapa)

1. **Cuerpo del reporte en Engram**: `mem_save` con `topic_key: "qa/{change}/verify-report"` — hallazgos, comandos ejecutados, resultados de la validación funcional, y el resultado real del ciclo RDD para los ítems 3-4 (aprobado con sus hallazgos, declinado por el humano, o no aplicable por RDD deshabilitado).
2. **Admisión anti-alucinación**: arma el envelope canónico `gentle-ai.qa-stage-artifact/v1` (`findings`/`pending_questions`/`scope`/`predecessor_sha256` — nunca el envelope legacy que el Core real rechaza) y pásalo por `gentle-ai qa-validate --input - --change {change} --stage verify --source-revision <artifact_revision del apply>`.
3. **Cierre del ledger**: con el `artifact_revision` que `qa-validate` devolvió, ejecuta `gentle-ai qa-finish --change {change} --cwd <repo> --request-id <id-idempotente> --outcome passed --evidence-revision <artifact_revision>`. Si hay hallazgos graves o la prueba falla, usa `--outcome failed`.
4. Solo tras un `qa-finish` exitoso reportas la etapa `verify` como cerrada al orquestador/supervisor.

## Guardrails

- No modifiques código para "arreglar" hallazgos; reporta y deja la corrección a `qa-apply`.
- Si el resultado contradice BookStack, preséntalo como contradicción al humano (G1/G4).
- No inventes ni asumas un `artifact_revision` — es siempre el que devuelve `qa-validate`.
- Nunca inventes un comando de `gentle-ai review` desde memoria o de una corrida anterior — rutea siempre desde el `next_transition` real que devuelve el propio STATUS.
- Un envelope de consentimiento (`gentle-ai.review-integration.consent/v3`) se presenta íntegro al humano y se espera su respuesta — nunca decidas `granted`/`declined` en su nombre.

## Comandos de referencia

- Tipos: `npx tsc --noEmit`.
- Ejecución: runner de Playwright del proyecto con reporter/trace habilitados.
- Reglas G1-G6: lee `skills/_shared/qa-gate-policy.md` (fuente única in-repo).
- Ledger: `gentle-ai qa-begin` / `gentle-ai qa-validate` / `gentle-ai qa-finish` (ver `docs/migration/qa-orchestrator-v3-design.md`).
