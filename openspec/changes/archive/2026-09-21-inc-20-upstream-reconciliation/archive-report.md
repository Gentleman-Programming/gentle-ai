# Informe de Archivo: Reconciliación con Upstream v3.4.0 e Identidad de Distribución (INC-20)

> **Incremento:** `inc-20-upstream-reconciliation`  
> **Fase:** `sdd-archive` · **Fecha:** 2026-09-21 (Formalizado: 2026-09-23)  
> **Estado:** Completo y Archivado  
> **Artefactos:** proposal.md, spec.md, design.md, tasks.md, verify-report.md

---

## 1. Resumen Ejecutivo

**Cambio archivado:** `inc-20-upstream-reconciliation` — Reconciliación auditada con upstream (`Gentleman-Programming/gentle-ai`) e identidad de distribución de Axiom.

**Integración y cierre:**
- Nueve PRs fusionadas en `main` (#26 a #34).
- 91 commits de upstream analizados y absorbidos en `docs/upstream-absorption-ledger.md`.
- Primera release oficial de Axiom publicada bajo su propia identidad: **`v3.5.0`** (2026-09-21), con binarios canónicos `axiom_*`, firmas minisign y procedencia atestiguada.
- Retirada de la implementación propia de ODD (`internal/odd`) en favor de la directriz de agente de upstream, adoptada de forma unificada.
- Migración de módulo a `/v3` completada.

---

## 2. Especificaciones Vivas Promovidas

Este incremento promueve las siguientes capacidades al catálogo de especificaciones vivas en `openspec/specs/`:
1. `upstream-absorption-protocol`: Protocolo por tandas, reglas de derivación y registro durable.
2. `axiom-distribution-identity`: Taxonomía de interoperabilidad vs. identidad, releases propios y nombres de servicio.
3. `axiom-binary-ci-coverage`: Cobertura bloqueante de CI sobre el binario canónico `cmd/axiom`.

Y consolida las modificaciones sobre:
- `axiom-sdd-cli-integration`
- `rdd-sdd-receipt-consumption`
- `sdd-research`
- `organic-agent-trigger-rules`
- `odd-*` (retiradas)

---

## 3. Resolución de Deuda y Estado Final

El incremento queda formalmente concluido al 100%. Las tareas condicionales de los scripts standalone curl de instalación quedan documentadas y resueltas, al estar la distribución canónica respaldada por la release `v3.5.0` y el actualizador integrado de Axiom.
