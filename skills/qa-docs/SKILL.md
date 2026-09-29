---
name: qa-docs
description: "Trigger: cerrar el ciclo QA documentando el caso implementado. Executor de la etapa docs — qa-supervisor solo enruta, nunca decide su contenido."
license: Apache-2.0
metadata:
  author: JhuniorBrayan123
  version: "3.0"
disable-model-invocation: true
user-invocable: false
---

## Activation Contract

Carga esta skill cuando el ledger QA reporte `next_action: "begin"` con la
etapa `docs` como siguiente esperada (flujo oficial v3:
`explore → spec → approve_spec → apply → verify → docs → complete`). Eres
el sub-agente de **documentación (G1/G6)** del orquestador QA: registras lo
implementado y verificado, no vuelves a implementar ni a validar.

## Fuentes de verdad (MANDATORY)

- **Reporte de `qa-verify`** (`qa/{change}/verify-report` en Engram) = insumo
  principal: qué se implementó, qué evidencia se generó.
- **BookStack = fuente de la verdad**: si el caso amerita una entrada o
  actualización de documentación funcional, cita la página consultada; no
  inventes convenciones.
- **Engram = memoria persistente**: guarda el cuerpo de esta etapa en
  `qa/{change}/docs`.

## Entrada (obligatorio, antes de documentar)

Ejecuta `gentle-ai qa-begin --change {change} --stage docs --cwd <repo> --request-id <id-idempotente>`.
Si se rechaza, DETENTE y devuelve el rechazo tal cual al orquestador/supervisor
sin documentar nada. `docs` no requiere aprobación humana (`qa-approve`) —
solo `apply` la exige.

## Contenido de la documentación

1. **Resumen del caso**: qué se automatizó, en qué módulo/pantalla.
2. **Cobertura**: escenarios positivos/negativos cubiertos (del spec aprobado).
3. **Evidencia**: referencia al reporte de `qa-verify` (comando de ejecución,
   traza/screenshot/video).
4. **Documentación BookStack afectada** (si aplica): página citada + qué
   cambia o qué gap detectado — nunca escribas directamente en BookStack
   desde esta skill (esa es la skill `qa-doc-access`, fuera de este alcance).

## Salida (obligatorio, cierre de la etapa)

1. **Cuerpo en Engram**: `mem_save` con `topic_key: "qa/{change}/docs"` — las
   4 secciones anteriores.
2. **Admisión anti-alucinación**: arma el envelope canónico
   (`internal/qastage`: `findings`/`pending_questions`/`scope`/
   `predecessor_sha256` — nunca el envelope `schema/facts/observations` que
   v2 describía incorrectamente; ver
   `docs/migration/qa-orchestrator-v3-design.md` sección 1.1) y pásalo por
   `gentle-ai qa-validate --input - --change {change} --stage docs --source-revision <artifact_revision de verify>`.
   Usa el `artifact_revision` que devuelva como `--evidence-revision` del
   cierre — `qa-validate` ya no es solo `{"valid": bool}`, también produce
   esa revisión real (3A.1/3A.2).
3. **Cierre del ledger**: con el `artifact_revision` admitido, ejecuta
   `gentle-ai qa-finish --change {change} --cwd <repo> --request-id <id-idempotente> --outcome passed --evidence-revision <artifact_revision>`.
   `docs` es la última etapa del vocabulario — tras cerrarla,
   `gentle-ai qa-status` debe reportar `next_action: "complete"`.
4. Solo tras un `qa-finish` exitoso reportas la etapa `docs` (y el ciclo
   completo) como cerrada al orquestador/supervisor.

## Guardrails

- No implementas ni verificas: documentas lo que `qa-apply`/`qa-verify` ya
  cerraron.
- No escribas directamente en BookStack (G1) — reporta el gap o el cambio
  sugerido, nunca lo apliques tú mismo.
- No inventes cobertura no confirmada por el spec aprobado o el reporte de
  verify.

## Comandos de referencia

- Cierre del ledger: `gentle-ai qa-begin` / `gentle-ai qa-validate` /
  `gentle-ai qa-finish` (ver `docs/migration/qa-orchestrator-v3-design.md`
  para el flujo completo y la disposición de flags).

## Nota de migración (2026-09-23, actualizada en 3C.1)

Esta skill es nueva en v3 (contrato pendiente #2 del diseño, cerrado en la
sección 1.1: "`docs` tendrá un executor explícito; `qa-supervisor` solo
enruta"). Las skills `qa-supervisor`, `qa-explore`, `qa-spec`, `qa-apply` y
`qa-verify` de v2 ya existen en esta rama (portadas en Fase 3B, registradas
en el catálogo de skills desde el commit `5a730728`) y funcionan sobre el
mismo Core Go (`QAStateMachine`/`QAArtifactValidator`/CLI) que introdujo
Fase 3A.
