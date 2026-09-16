# Reporte de Archivado: Orquestación Interactiva SDD y Creación de Cambios en Dashboard Web y CLI (INC-15)

**Fecha:** 2026-09-16  
**Incremento:** `inc-15-dashboard-interactive-sdd-orchestration`  
**Estado:** ARCHIVED  

---

## Resumen del Incremento

El **Incremento 15 (INC-15: `inc-15-dashboard-interactive-sdd-orchestration`)** transforma el Dashboard Web local (`axiom ui`) en un centro de mando operativo e interactivo para desarrolladores y agentes de IA, al tiempo que dota a la CLI canónica `axiom` del subcomando `axiom change create` (con alias `axiom change new`), permitiendo inicializar cambios con plantilla canónica en español, avanzar fases del ciclo SDD, validar reportes de verificación formal y componer relevos estructurados (handoffs) en un solo clic:

1. **Capa de Dominio y Servicios Backend (`internal/dashboard/`):**
   - Nuevos DTOs estructurados en `types.go`: `CreateIncrementRequest`, `CreateIncrementResponse`, `IncrementActionRequest`, `IncrementActionResponse`, `CreateHandoffRequest`, `CreateHandoffResponse`.
   - Método `CreateIncrement` en `service.go`: valida nombres en formato kebab-case, previene colisiones con cambios existentes activos o archivados y genera automáticamente el archivo `proposal.md` con plantilla canónica estructurada en español castellano.
   - Métodos `ContinueIncrement` y `VerifyIncrement`: ejecutan en proceso las transiciones de despacho de `internal/cli` (`RunSDDContinue` y `RunSDDVerifyValidate`), capturando stdout/stderr en memoria de forma portable y determinista.
   - Método `CreateHandoff`: valida fases, estados y transiciones con el motor de `internal/handoff`, poblando defaults coherentes y escribiendo atómicamente `handoff.md` con Frontmatter YAML y las 5 secciones obligatorias en castellano.

2. **Endpoints REST del Servidor Local (`internal/dashboard/server.go`):**
   - `POST /api/increments`: Creación de incrementos retornando `201 Created`.
   - `POST /api/increments/continue`: Ejecución de transición SDD retornando `200 OK`.
   - `POST /api/increments/verify`: Verificación formal de conformidad retornando `200 OK`.
   - `POST /api/handoffs`: Registro de relevos formales retornando `201 Created`.
   - Manejo exhaustivo de errores HTTP 400 (bad request) y 404 (not found).

3. **Interfaz Web SPA Interactiva (`internal/dashboard/assets/`):**
   - **Tablero de Incrementos (`index.html`, `app.js`, `style.css`):**
     - Botón `+ Nuevo Incremento` en la barra de herramientas superior.
     - Modal flotante `#modal-new-increment` con validación interactiva de formato kebab-case, selector de tipo de cambio y área de texto para la intención técnica.
   - **Modal de Detalle del Incremento (`#inc-modal`):**
     - Barra de herramientas con botones de acción rápida: `▶ Continuar SDD`, `✓ Validar Verificación` y `🤝 Redactar Handoff`.
     - Panel de consola colapsable `#modal-console-container` con terminal oscura y scroll para visualizar salidas de ejecución en tiempo real.
     - Deshabilitación de acciones de avance para incrementos archivados.
   - **Visor de Handoffs (`tab-handoffs`):**
     - Botón `+ Crear Handoff` y modal `#modal-create-handoff` con preselección de cambio, roles origen/destino y campos para las 5 secciones canónicas.

4. **Subcomando en la CLI canónica `axiom` (`cmd/axiom/`):**
   - Implementación de `axiom change create <nombre>` y alias `axiom change new <nombre>`.
   - Banderas `--intent` (`-i`), `--type` (`-t`) y `--cwd`.
   - Función despachadora `runChange` con ayuda contextual `--help`.
   - Soporte para banderas antes o después del nombre del cambio.

5. **Pruebas y Verificación Formal:**
   - Pruebas unitarias de endpoints en `internal/dashboard/dashboard_test.go` (`TestInteractiveSDDOrchestrationEndpoints`).
   - Pruebas de integración CLI en `cmd/axiom/main_test.go` (`TestRunChangeHelp`, `TestRunChangeCreate_Success`, `TestRunChangeCreate_ValidationAndCollision`).
   - Verificación formal con `axiom sdd-verify-validate` (VERDICT: PASS, 5/5 requerimientos, 8/8 escenarios BDD).

---

## Artefactos Consolidados y Modificados

- **Capa de Servicio y Dashboard Backend:**
  - `internal/dashboard/types.go`
  - `internal/dashboard/service.go`
  - `internal/dashboard/server.go`
  - `internal/dashboard/dashboard_test.go`
- **Interfaz Web SPA (Frontend):**
  - `internal/dashboard/assets/index.html`
  - `internal/dashboard/assets/app.js`
  - `internal/dashboard/assets/style.css`
- **CLI Canónica de Axiom:**
  - `cmd/axiom/main.go`
  - `cmd/axiom/main_test.go`
- **Ciclo SDD de INC-15:**
  - `openspec/changes/inc-15-dashboard-interactive-sdd-orchestration/proposal.md`
  - `openspec/changes/inc-15-dashboard-interactive-sdd-orchestration/spec.md`
  - `openspec/changes/inc-15-dashboard-interactive-sdd-orchestration/design.md`
  - `openspec/changes/inc-15-dashboard-interactive-sdd-orchestration/tasks.md`
  - `openspec/changes/inc-15-dashboard-interactive-sdd-orchestration/verify-report.md`
  - `openspec/changes/inc-15-dashboard-interactive-sdd-orchestration/archive-report.md`
- **Especificaciones Vivas:**
  - `openspec/specs/dashboard-sdd-orchestration/spec.md` (nueva)
  - `openspec/INDEX.md` (sincronizado)
