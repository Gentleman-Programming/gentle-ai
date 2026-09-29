# Reporte de validación — Migración QA-Orchestrator v2 → v3.7.0

**Fecha:** 2026-09-23
**Rama de trabajo:** `qa-orchestrator-v3` (base `v3.7.0`, commit `6dee8f83`)
**Estado real verificado:** ninguna funcionalidad ha sido portada, adaptada, integrada o retirada en código todavía. `git log` de `qa-orchestrator-v3` es idéntico a `v3.7.0`; el único artefacto nuevo en esa rama es `docs/migration/qa-orchestrator-v2-to-v3.md` (inventario), aún sin commitear. `skills/qa-*` no existe en esta rama.

> **Nota de alcance.** Este documento NO valida una migración ejecutada — valida que la **Fase 1 (inventario)** está completa y en qué condiciones se puede aprobar avanzar a la **Fase 2 (diseño)**. No confundir con un reporte QA de código migrado y probado, porque ese código todavía no existe.

## 1. Resumen ejecutivo

- Fase 1 (inventario funcional componente por componente) — **completa**, documentada en [`qa-orchestrator-v2-to-v3.md`](qa-orchestrator-v2-to-v3.md).
- Fase 2 (matriz de compatibilidad / diseño) — **no iniciada**.
- Fase 3-6 (implementación incremental, review/RDD por bloque, pruebas) — **no iniciadas**.
- Hallazgo bloqueante principal: el `StageVocabulary`/`Stage` del que dependen los 4 verbos CLI (`qa-begin/finish/approve/status`) no existe en `v3.7.0`. Ningún verbo QA compilaría "tal cual" contra el `sddstatus` actual — cualquier PORTAR real requiere antes una decisión de diseño (pregunta abierta #1 del inventario).
- No hay tests nuevos que validar porque no hay código nuevo. Los únicos tests relevantes hoy son los de `legacy-v2-custom`, que validan el comportamiento v2 que se busca preservar o reemplazar conscientemente.

## 2. Estado por componente (condensado del inventario)

| Componente | Clasificación propuesta | ¿Qué lo valida HOY? | Riesgo principal |
|---|---|---|---|
| `qa-begin` | ADAPTAR 60% / PORTAR 40% | `internal/cli/qa_begin_test.go`, `qa_cli_test.go` (en `legacy-v2-custom`; no ejecutables contra `qa-orchestrator-v3` sin el vocabulario) | Depende de que se resuelva el vocabulario de stages (ver riesgo #1) |
| `qa-finish` | ADAPTAR / OBSOLETO (parcial) | `internal/cli/qa_finish_test.go` | Verificación de hash de evidencia no tiene equivalente en v3.7.0 |
| `qa-approve` | INTEGRAR con `consent_contract.go`/`edit_authority.go` | `internal/cli/qa_approve_test.go` | Ese mecanismo aprueba *acceso de escritura*, no *contenido de etapa* — no es un reemplazo directo, requiere diseño |
| `qa-status` | ADAPTAR (patrón `next_action` ya idiomático en v3.7.0) | `internal/cli/qa_status_cwd_test.go` | v3.7.0 reimplementa `next_action` 3 veces de forma independiente; no hay librería compartida que adoptar |
| QA ledger (`StageVocabulary`) | ADAPTAR (motor) / PORTAR (vocabulario) | `internal/qastage/e2e_test.go`, `vocabulary_test.go` | **Máximo riesgo del inventario** — pieza central sin equivalente en v3.7.0 |
| `qa-supervisor` | ADAPTAR (ruteo) / RETIRAR (BookStack/GitLab/G1-G6 como "core") | ninguno (skill markdown) | Mezcla ~50% mecánica portable con ~50% reglas de negocio específicas del cliente |
| `qa-explore` | ADAPTAR 75% / RETIRAR 25% | ninguno (skill markdown) | Depende de si la maquinaria generadora de `sdd-explore.md` admite extensiones de dominio |
| `qa-spec` | ADAPTAR 70% / INTEGRAR 30% | ninguno (skill markdown) | No confirmado si v3.7.0 tiene algún gate de aprobación humana equivalente entre spec y apply |
| `qa-apply` | ADAPTAR 65% / RETIRAR 35% | ninguno (skill markdown) | El error tipado del gate (`ErrRuntimeStageApprovalRequired`) no existe en v3.7.0 |
| `qa-verify` | ADAPTAR 55% / INTEGRAR 30% / RETIRAR 15% | ninguno (skill markdown) | Candidato a delegar en el sistema de lens de RDD (`internal/reviewtransaction`), pendiente de decisión de diseño |

Ninguna fila tiene tests que validen *la versión migrada*, porque no existe todavía.

## 3. Riesgos abiertos consolidados (de mayor a menor impacto)

1. **Vocabulario de stages ausente en v3.7.0** — bloquea PORTAR directo de los 4 verbos CLI y del ledger. Bloqueante para: `qa-begin`, `qa-finish`, `qa-approve`, `qa-status`, ledger.
2. **Gate de aprobación humana pre-apply sin equivalente tipado** (`ErrRuntimeStageApprovalRequired`/`ErrRuntimeStageOutOfOrder` no existen en v3.7.0). Bloqueante para: `qa-approve`, `qa-spec`, `qa-apply`.
3. **Cambio de paradigma de autoría**: las skills SDD de v3.7.0 se generan desde Go (`internal/components/sdd/*.go`), no son archivos `.md` estáticos como las `skills/qa-*` actuales. Migrar implica decidir si se fork-ea el generador o se mantienen skills QA como capa separada.
4. **Dependencias de negocio sin tracción en upstream**: BookStack MCP (0 referencias en `v3.7.0`), GitLab MCP, reglas G1-G6, Screenplay+POM/Playwright/locator-hunting. Ninguna tiene equivalente — hay que decidir si se conservan como extensión de dominio o se reevalúan.
5. **`qa-verify` vs. lens de RDD**: v3.7.0 tiene un sistema de revisión automatizada (`gentle-ai review inspect-candidate --lens`) más riguroso que el checklist narrativo G6 actual, pero absorción no está diseñada.
6. **Tests existentes (~650 líneas en `internal/cli/qa_*_test.go` + 516 líneas en `internal/qastage/*_test.go`)** quedarán huérfanos o deberán reescribirse contra cualquier diseño nuevo del ledger.

## 4. Preguntas de decisión pendientes (bloquean el paso a Fase 2 — diseño)

Éstas son las mismas 7 preguntas del inventario, priorizadas por cuánto diseño destraban:

1. Vocabulario de stages: ¿reintroducir `StageVocabulary` genérico en `sddstatus`, o construir el ledger QA aparte?
2. Gate de aprobación: ¿extender `consent_contract.go`, o crear un contrato de consentimiento nuevo?
3. BookStack/GitLab MCP: ¿siguen siendo fuente de verdad, o se evalúa Engram/ODD como reemplazo?
4. `ErrRuntimeStageApprovalRequired`/`ErrRuntimeStageOutOfOrder`: ¿se recrean, o se delega el gate a otro mecanismo (RDD)?
5. `qa-verify`: ¿delega en el sistema de lens de RDD, o mantiene su checklist propio?
6. Screenplay+POM/Playwright/locator-hunting: ¿extensión de dominio sobre SDD genérico, o familia de skills separada (como hoy)?
7. Tests de `internal/qastage` y `internal/cli/qa_*_test.go`: ¿se migran adaptados, o se reescriben desde cero según el diseño elegido?

**No se recomienda iniciar Fase 2 (diseño) sin respuesta al menos a las preguntas 1, 2 y 4** — son las que determinan si el ledger QA puede compilar contra el `sddstatus` actual.

## 5. Escenarios de prueba manual en OpenCode — baseline pre-migración

Como todavía no existe una v3 del QA-Orchestrator, "probar antes de aprobar la migración" significa **documentar el comportamiento real de v2 hoy** (checkout de `legacy-v2-custom` en OpenCode), para que sirva de contrato de referencia: cualquier decisión de PORTAR/ADAPTAR/RETIRAR se contrasta contra este comportamiento observado, no contra lo que el markdown de la skill *dice* que hace.

Antes de aprobar el inventario y pasar a diseño, correr en OpenCode sobre `legacy-v2-custom`:

1. **Ciclo feliz completo**: `qa-supervisor` → `qa-explore` → `qa-spec` (con aprobación humana) → `qa-apply` → `qa-verify` → cierre. Confirmar que `qa-status --change <x>` refleja el `next_action` correcto después de cada paso (`begin`, `finish`, `complete`).
2. **Gate de aprobación bloqueante**: intentar `qa-apply` sin haber corrido `qa-approve` sobre el spec. Confirmar que se recibe `ErrRuntimeStageApprovalRequired` (o el mensaje equivalente que expone la skill) y que `qa-apply` NO avanza.
3. **Orden de etapas**: intentar `qa-begin --stage apply` sin haber cerrado `explore`/`spec`. Confirmar `ErrRuntimeStageOutOfOrder`.
4. **Ficha documental BookStack**: correr `qa-explore` o una consulta vía `qa-doc-reference` sobre un módulo real y confirmar que la ficha de 13 campos + URL exacta se genera, y que ante una divergencia real el flujo hace STOP en vez de decidir por su cuenta.
5. **Locator hunting de 2 niveles**: un caso donde el locator ya existe en el POM (nivel 0, debe reutilizarse sin tocar GitLab) y un caso donde no existe (nivel 1, debe consultar GitLab vía MCP).
6. **Evidencia G6**: correr `qa-verify` sobre un caso ya implementado y confirmar que produce evidencia reproducible (screenshots/traces/videos) y que `qa-finish` valida el hash de esa evidencia antes de cerrar.
7. **Cierre con MR en GitLab**: confirmar que `qa-supervisor`, al cerrar el ciclo, efectivamente crea el MR (o falla de forma explícita y visible si el MCP de GitLab no está disponible).
8. **Recalculo de estado, nunca caché**: modificar el ledger externamente (o simular una interrupción) y confirmar que `qa-status` refleja el estado real inmediatamente, sin caché obsoleto.

Cada escenario que falle o se comporte distinto a lo documentado en las skills debe anotarse — es evidencia adicional para la fila correspondiente del inventario, no un bloqueo del reporte en sí.

## 6. Criterio de aprobación propuesto para pasar a Fase 2

Se puede considerar aprobado el inventario y habilitado el diseño cuando:

- [ ] El usuario respondió al menos las preguntas 1, 2 y 4 de la sección 4.
- [ ] Los 8 escenarios de la sección 5 se corrieron sobre `legacy-v2-custom` y sus resultados quedaron documentados (aunque sea informalmente).
- [ ] No hay discrepancias sin explicar entre lo que las skills `.md` dicen y el comportamiento observado en OpenCode.

Hasta que estas tres condiciones se cumplan, no se recomienda empezar a escribir código en `qa-orchestrator-v3`.
