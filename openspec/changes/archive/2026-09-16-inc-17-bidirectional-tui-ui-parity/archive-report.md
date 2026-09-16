# Reporte de Archivado: Paridad Bidireccional entre TUI y Web UI (INC-17)

> **Fecha:** 2026-09-16  
> **Incremento:** `inc-17-bidirectional-tui-ui-parity`  
> **Estado:** ARCHIVED  
> **Veredicto:** ✅ PASS (10/10 Requerimientos, 11/11 Escenarios BDD)  
> **Idioma:** Español (Castellano peninsular)

---

## 1. Resumen del Incremento

El **Incremento 17 (INC-17: `inc-17-bidirectional-tui-ui-parity`)** resuelve integralmente la asimetría de capacidades entre la consola de terminal interactiva (TUI Bubbletea) y el Dashboard Web (`axiom ui`), completando una experiencia de desarrollo cohesionada y bidireccional:

1. **De TUI a Web UI (Ecosistema & Herramientas):**
   - **Diagnóstico Integral (`axiom doctor`):** Endpoint `GET /api/ecosystem/doctor` que inspecciona la disponibilidad de agentes de IA, herramientas CLI base (git, go, node), permisos de `~/.axiom/` y configuración del sistema.
   - **Mantenimiento Reactivo (`sync` y `upgrade`):** Endpoints `POST /api/ecosystem/sync` y `POST /api/ecosystem/upgrade` con ejecución segura, salida estructurada y registro en consola visual.
   - **Gestión Visual de Respaldos:** Endpoints `GET /api/ecosystem/backups`, `POST /api/ecosystem/backups/create` y `POST /api/ecosystem/backups/restore`, conectados con `internal/backup` sobre `~/.axiom/backups/`.
   - **Configuración de Modelos:** Endpoint `GET /api/ecosystem/models` para consultar modelos de IA asignados por rol.
   - **Frontend Reactivo:** Pestaña `⚙️ Ecosistema & Herramientas` en `index.html` con tarjetas de estado, consola oscura colapsable y botones de acción rápida en `app.js`.

2. **De Web UI a TUI (Gobernanza SDD y Proyectos Hub):**
   - **Submenú Unificado:** Opción `📁 Proyectos y Gobernanza SDD ➔` en el menú principal (`welcome.go`) que conduce a un menú secundario ergonómico (`governance.go`).
   - **Gestión Multi-Proyecto (`ScreenHubProjects`):** Lectura y conmutación de proyectos activos desde `~/.axiom/workspaces.json`, vinculación e inicialización con `axiom init`.
   - **Ciclo de Vida SDD (`ScreenSDDIncrements`):** Tablero interactivo con listado de cambios activos y archivados, progreso porcentual, conteo de tareas y avance de fase.
   - **Monitor Multi-Rol y Fan-In (`ScreenMultiRole`):** Visualización de roles declarados, clasificación de compuertas (`blocking`/`deferred`) y veredicto en vivo de la barrera de sincronización.
   - **Visor de Handoffs (`ScreenHandoffs`):** Deserialización y presentación estructurada de `handoff.md` con flujo de fases, metadatos y secciones en castellano.
   - **Catálogo de Living Docs (`ScreenLivingDoc`):** Explorador de especificaciones vivas con sincronización interactiva mediante la tecla `s`.

---

## 2. Artefactos Modificados y Creados

### Backend & Capa de Servicios
- `internal/dashboard/types.go` — DTOs de ecosistema, diagnóstico y respaldos.
- `internal/dashboard/service.go` — Métodos de diagnóstico `doctor`, sincronización, actualización y respaldos.
- `internal/dashboard/server.go` — Endpoints REST `/api/ecosystem/*`.
- `internal/dashboard/dashboard_test.go` — Suite `TestEcosystemEndpoints` con cobertura integral.

### Frontend Web SPA
- `internal/dashboard/assets/index.html` — Pestaña `tab-ecosystem` y controles visuales.
- `internal/dashboard/assets/app.js` — Lógica reactiva de carga y ejecución de acciones.
- `internal/dashboard/assets/style.css` — Estilos de consola, tarjetas de salud y alertas.

### Terminal User Interface (TUI Bubbletea)
- `internal/tui/screens/welcome.go` — Opción de menú de gobernanza.
- `internal/tui/screens/governance.go` — Submenú de gobernanza SDD.
- `internal/tui/screens/hub_projects.go` — Gestión de proyectos del Hub.
- `internal/tui/screens/sdd_increments.go` — Tablero de ciclo de vida SDD.
- `internal/tui/screens/multi_role.go` — Monitor multi-rol y barrera Fan-In.
- `internal/tui/screens/handoffs.go` — Visor de relevos estructurados.
- `internal/tui/screens/living_doc.go` — Catálogo de living documentation.
- `internal/tui/router.go` — Rutas lineales y retroceso con `Esc`.
- `internal/tui/model.go` — Enrutamiento, eventos, teclas y helpers de carga de gobernanza.
- `internal/tui/model_test.go` — Pruebas de integración de pantallas y navegación.
- `internal/tui/screens/welcome_test.go` — Verificación de opciones del menú principal.

### Especificación y Documentación
- `openspec/specs/tui-ui-parity/spec.md` — Especificación viva consolidada.
- `openspec/changes/inc-17-bidirectional-tui-ui-parity/verify-report.md` — Reporte de verificación formal.
- `openspec/changes/inc-17-bidirectional-tui-ui-parity/archive-report.md` — Reporte de archivado.

---

## 3. Conformidad Formal SDD

- **Veredicto `sdd verify-validate`:** PASS (`valid: true, verdict: pass`).
- **Pruebas Unitarias de Regresión:** 100% PASS en toda la suite (`go test ./...`).
- **Compilación de Binario:** `go build ./cmd/axiom` exitosa.
