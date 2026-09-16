# Reporte de Archivado: Desacoplamiento de RDD y Parches de Estabilidad Upstream v3 (INC-18)

> **Fecha:** 2026-09-16  
> **Incremento:** `inc-18-rdd-decoupling-and-v3-stability-fixes`  
> **Estado:** ARCHIVED  
> **Veredicto:** ✅ PASS (6/6 Requerimientos, 6/6 Escenarios BDD)  
> **Idioma:** Español (Castellano peninsular)  

---

## 1. Resumen del Incremento

El **Incremento 18 (INC-18: `inc-18-rdd-decoupling-and-v3-stability-fixes`)** culmina con éxito dos metas críticas en la arquitectura de Axiom:

1. **Desacoplamiento Total de RDD:**
   - Se eliminó la inyección forzada de ofertas de revisión (`applyReviewOfferRouting`) del motor de estado SDD (`internal/sddstatus/status.go`).
   - Se neutralizó `internal/sddstatus/review_door.go`, garantizando que `status.ReviewOffer` sea siempre `nil` y se omita limpiamente en JSON mediante `omitempty`.
   - Se desacopló `internal/cli/sdd_status.go` del callback de kill-switch de RDD (`ReviewDisabledForWorkspace`).
   - Las herramientas de revisión de código (`axiom review ...`) permanecen 100% operativas y accesibles como utilidades autónomas e independientes.

2. **Absorción Selectiva de Parches Críticos de Gentle-AI v3:**
   - **Rutas Windows (`1a2f6775`):** Deserialización nativa de JSON (`StatusV2Projection`) en pruebas bajo Windows (`internal/cli/sdd_attempt_test.go`), tolerando barras invertidas escapadas.
   - **Aislamiento CWD (`8c078527`):** Eliminación de dependencias de `resolveOpenClawWorkspaceDir` en `internal/cli/run.go` y `sync.go`, resolviendo los perfiles y artefactos de agentes contra sus raíces canónicas (`homeDir` y `componentInjectionDir`).
   - **Saneamiento de Presets de Skills (`11f6c000`):** Segregación estricta de las 6 skills internas de colaboración (`contributorSkills`) fuera de los presets de usuario general (`selectableFoundationSkills`) en `internal/components/skills/presets.go`.
   - **Resiliencia de Engram (`59e6705f`, `90992285`):** Protocolo documentado con pautas explícitas de resolución ante `ambiguous_project` en `internal/assets/engram/protocol.md`.

3. **Descarte Ratificado de RTK:**
   - Confirmada la no adopción de RTK (`rtk-ai/rtk`) debido a sus hooks de pre-ejecución intrusivos y falta de soporte en Windows.

---

## 2. Artefactos Modificados y Creados

### Motor de Estado SDD y CLI
- `internal/sddstatus/status.go` — Neutralización de `applyReviewOfferRouting`.
- `internal/sddstatus/review_door.go` — Stub desacoplado sin dependencia de `reviewtransaction`.
- `internal/sddstatus/status_v2_clean_break_test.go` — Adaptación de guardas de ausencia de review.
- `internal/cli/sdd_status.go` — Eliminación de acoplamiento con kill-switch RDD.
- `internal/cli/sdd_attempt_test.go` — Deserialización estructurada de JSON tolerante a Windows.
- `internal/cli/run.go` & `internal/cli/sync.go` — Aislamiento de CWD en resolución de artefactos de agentes.

### Catálogo de Skills y Protocolos
- `internal/components/skills/presets.go` — Segregación de `contributorSkills`.
- `internal/components/skills/presets_test.go` — Tests de presets actualizados.
- `internal/components/skills/testdata/golden/skills-presets.json` — Golden file saneado.
- `internal/assets/engram/protocol.md` — Protocolo robusto ante `ambiguous_project`.

### Gobernanza SDD y Especificación
- `openspec/specs/rdd-decoupling-v3-stability/spec.md` — Especificación viva consolidada.
- `openspec/changes/inc-18-rdd-decoupling-and-v3-stability-fixes/proposal.md` — Propuesta formal.
- `openspec/changes/inc-18-rdd-decoupling-and-v3-stability-fixes/spec.md` — Especificación del cambio.
- `openspec/changes/inc-18-rdd-decoupling-and-v3-stability-fixes/design.md` — Diseño técnico y de arquitectura.
- `openspec/changes/inc-18-rdd-decoupling-and-v3-stability-fixes/tasks.md` — Tareas de implementación (7/7 completadas).
- `openspec/changes/inc-18-rdd-decoupling-and-v3-stability-fixes/verify-report.md` — Informe de verificación con veredicto PASS.
- `openspec/changes/inc-18-rdd-decoupling-and-v3-stability-fixes/archive-report.md` — Este reporte de archivado.

---

## 3. Certificación de Calidad

- **Compilación:** `go build -o axiom.exe ./cmd/axiom` exitosa con código 0.
- **Pruebas Unitarias e Integración:** 100% PASS en paquetes afectados (`internal/sddstatus`, `internal/cli`, `internal/components/skills`).
- **Aislamiento Funcional:** Verificado que `sdd status` ya no contiene ofertas ni bloqueos de RDD.
