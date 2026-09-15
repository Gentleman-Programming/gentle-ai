# Progreso de Implementación: Motor de Documentación Viva y Adopción Orgánica en Archive (INC-07)

## Estado de Fases y Tareas

### Fase 1: Motor de Dominio Core (`internal/livingdoc/`)
- [x] **T-01** Crear `internal/livingdoc/types.go`: Definir estructuras de datos para requerimientos (`RequirementMeta`), especificaciones vivas (`LivingSpecEntry`), catálogo maestro (`LivingCatalog`) e informe de sincronización (`SyncReport`).
- [x] **T-02** Crear `internal/livingdoc/indexer.go`: Implementar analizador de especificaciones vivas en `openspec/specs/`, extractor de requerimientos/escenarios BDD y generador determinista del manifiesto Markdown `openspec/INDEX.md`.
- [x] **T-03** Crear `internal/livingdoc/synthesizer.go`: Implementar motor de adopción orgánica (*Cold Start*) que sintetiza la especificación viva inicial a partir de un cambio cerrado o en curso.
- [x] **T-04** Crear `internal/livingdoc/service.go`: Implementar la capa de servicio que resuelve rutas del workspace, coordina el indexador y el sintetizador, y expone `GetCatalog`, `GetSpecDetail`, `Sync` y `ColdStart`.
- [x] **T-05** Crear `internal/livingdoc/livingdoc_test.go`: Suite completa de pruebas unitarias para indexación, generación de `INDEX.md`, síntesis *Cold Start* y tolerancia ante especificaciones malformadas (100% PASS).

### Fase 2: Integración con el Dashboard Web Local (`internal/dashboard/`)
- [x] **T-06** Actualizar `internal/dashboard/service.go`: Integrar `livingdoc.Service` en la capa de agregación del dashboard.
- [x] **T-07** Actualizar `internal/dashboard/server.go`: Exponer endpoints REST `/api/archive/specs` (GET), `/api/archive/specs/{domain}` (GET) y `/api/archive/sync` (POST).
- [x] **T-08** Actualizar `internal/dashboard/assets/` (`index.html`, `style.css`, `app.js`): Crear la pestaña "Especificaciones Vivas" en el menú de navegación, con tarjetas métricas, selector de dominios, visor de contenido Markdown y botón de sincronización reactiva.
- [x] **T-09** Actualizar `internal/dashboard/dashboard_test.go`: Incorporar pruebas unitarias HTTP `TestArchiveEndpoints` para los endpoints de documentación viva (11/11 tests PASS).

### Fase 3: Integración CLI (`cmd/axiom/main.go`)
- [x] **T-10** Implementar el grupo de subcomandos `axiom archive`: `axiom archive sync`, `axiom archive list`, `axiom archive show` y `axiom archive coldstart` en `cmd/axiom/main.go`.

### Fase 4: Verificación, Cierre y Archivado SDD
- [x] **T-11** Ejecutar suite completa de pruebas unitarias en Go y compilar el binario `axiom.exe`.
- [x] **T-12** Ejecutar en vivo `axiom archive sync` para generar el primer `openspec/INDEX.md` oficial consolidando todos los incrementos del proyecto (34 specs, 243 reqs, 400 escenarios BDD).
- [x] **T-13** Generar artefactos de trazabilidad: `apply-progress.md`, `verify-report.md` y `archive-report.md`.
- [x] **T-14** Promover especificación viva a `openspec/specs/living-documentation/spec.md`, archivar formalmente INC-07 en `openspec/changes/archive/2026-09-15-inc-07-archive-living-documentation-engine/` y actualizar `docs/ROADMAP.md` completando la totalidad del Roadmap fundacional de Axiom.
