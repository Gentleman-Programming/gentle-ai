# Propuesta: Paridad Bidireccional de Funcionalidades entre TUI y Web UI (INC-17)

## Propósito (Intent)

Con la culminación de las Fases 1 y 2 de Axiom, el proyecto dispone de dos interfaces interactivas complementarias:
1. **TUI (Terminal User Interface en Bubbletea / Lipgloss)**: Enfocada originariamente en la instalación de agentes, actualización de herramientas (`upgrade`), sincronización de configuraciones (`sync`), configuración de modelos de IA, gestión de respaldos (`backups`), perfiles de OpenCode y revisión formal RDD.
2. **Web UI (Dashboard Local SPA en `net/http` + Vanilla JS embebido)**: Creada y evolucionada en las Fases 1 y 2 para la gobernanza SDD, multi-proyecto (Hub), monitor multi-rol, barrera Fan-In, visor de handoffs estructurados, catálogo de autoskills, análisis semántico Serena/CodeGraph y catálogo de especificaciones vivas.

A pesar de compartir el mismo binario `axiom`, existía una asimetría funcional entre ambas interfaces:
- En la Web UI no era posible inspeccionar la salud del ecosistema (`axiom doctor`), ejecutar sincronizaciones o actualizaciones, gestionar respaldos ni configurar modelos de IA sin recurrir a la terminal.
- En la TUI no era posible explorar los proyectos del Hub, cambiar de proyecto activo, inspeccionar el ciclo de vida de los incrementos SDD, evaluar la barrera multi-rol ni consultar los handoffs estructurados.

El **Incremento 17 (INC-17: `inc-17-bidirectional-tui-ui-parity`)** resuelve integralmente esta brecha estableciendo **paridad bidireccional de funcionalidades**, permitiendo a los desarrolladores operar indistintamente desde el terminal interactivo o desde el dashboard web con coherencia funcional y reutilización directa de la capa de servicios de dominio en `internal/`.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

1. **Eje A: Traslado de Capacidades de la TUI a la Web UI (`internal/dashboard/`)**:
   - **Nueva pestaña en la navegación principal:** `⚙️ Ecosistema & Herramientas`.
   - **Diagnóstico del Sistema (Axiom Doctor):** Endpoint y panel visual con el estado de salud de agentes (Claude Code, OpenCode, Kiro, Codex, Pi), herramientas (Git, Go, Node) y entorno local.
   - **Acciones de Mantenimiento Reactivo:** Disparadores interactivos en un clic para "Sincronizar Configuraciones" (`axiom sync`) y "Actualizar Herramientas" (`axiom upgrade`), con registro visual de logs.
   - **Gestor Visual de Respaldos:** Listado de snapshots en `~/.axiom/backups/` con fecha, tamaño y tipo, botón para crear un nuevo respaldo bajo demanda y botón para restaurar un respaldo previo con confirmación.
   - **Configuración de Modelos de IA:** Visor de modelos asignados por agente y roles de desarrollo (arquitecto, ejecutor, revisor), permitiendo inspección e interacción.

2. **Eje B: Traslado de Capacidades de la Web UI a la TUI (`internal/tui/`)**:
   - **Submenú de Gobernanza en la TUI:** Nueva opción de primer nivel en `WelcomeOptions`: `📁 Proyectos y Gobernanza SDD ➔`.
   - **Pantalla de Gestión Multi-Proyecto (`ScreenHubProjects`):**
     * Listado interactivo de proyectos registrados en `~/.axiom/workspaces.json`.
     * Conmutación en caliente del proyecto activo.
     * Vinculación de un nuevo proyecto por ruta (`axiom project add`).
     * Asistente guiado de inicialización (`axiom init`) con detección de stack si el directorio actual no contiene `axiom.yaml`.
   - **Pantalla del Ciclo de Vida de Incrementos SDD (`ScreenSDDIncrements`):**
     * Listado navegable de incrementos activos y archivados.
     * Inspección de fases del ciclo (`explore`, `propose`, `spec`, `design`, `tasks`, `apply`, `verify`, `archive`).
     * Creación guiada de nuevo incremento (`axiom change create`).
     * Avance de fase interactivo (`sdd continue`) y validación de verificación (`sdd verify-validate`).
   - **Pantalla de Monitor Multi-Rol y Barrera Fan-In (`ScreenMultiRole`):**
     * Selector de cambio activo.
     * Inspección de roles asignados, políticas de compuerta (`blocking` vs `deferred`) y porcentaje de avance.
     * Diagnóstico de barrera de sincronización y atajo para migrar tareas diferidas a `openspec/changes/e2e-cumulative/tasks.md`.
   - **Pantalla de Visor de Handoffs Estructurados (`ScreenHandoffs`):**
     * Consulta del último `handoff.md` del cambio activo con desglose de secciones y estados.
   - **Pantalla de Catálogo de Especificaciones Vivas (`ScreenLivingDoc`):**
     * Navegación por las especificaciones consolidadas en `openspec/specs/` y botón de sincronización de `INDEX.md`.

3. **Eje C: Capa de Dominio Unificada y Reutilización DRY**:
   - Centralizar en `internal/dashboard/service.go` y los paquetes de dominio correspondientes (`internal/hub`, `internal/multirole`, `internal/handoff`, `internal/livingdoc`, `internal/backup`, `internal/doctor`) la lógica de negocio para evitar duplicidades de código entre TUI y Web UI.

4. **Suite de Pruebas Unitarias y Regresión**:
   - Pruebas completas para los nuevos endpoints de `internal/dashboard/`.
   - Pruebas de renderizado y navegación para las nuevas pantallas de `internal/tui/screens/` y `internal/tui/router.go`.

### Fuera de Alcance (Out of Scope)
- Reescribir frameworks subyacentes (se mantienen Vanilla JS en el Dashboard Web y Bubbletea/Lipgloss en la TUI).
- Soporte para edición concurrente multi-usuario de archivos de especificación en tiempo real.

---

## Capacidades (Capabilities)

### Nuevas Capacidades Introducidas
- `dashboard-ecosystem-management`: Panel de control de salud, doctor, actualización, sincronización, respaldos y modelos de IA en el Dashboard Web.
- `tui-hub-and-sdd-governance`: Navegación, conmutación e inicialización de proyectos, monitor multi-rol, barrera Fan-In, ciclo de vida SDD, handoffs y living docs en la TUI Bubbletea.
- `bidirectional-feature-parity`: Consistencia funcional simétrica entre el terminal y el navegador en toda la plataforma Axiom.

---

## Enfoque de Implementación (Approach)
1. **Fase spec:** Definir formalmente los requerimientos de cada pantalla TUI y panel Web UI con escenarios BDD en `spec.md`.
2. **Fase design:** Diseñar los modelos de datos, endpoints REST, estados de Bubbletea y diagramas de flujo en `design.md`.
3. **Fase tasks:** Desglosar las tareas atómicas de backend, TUI, frontend web y pruebas en `tasks.md`.
4. **Fase apply:** Implementar incrementalmente los endpoints del servidor web, los componentes frontend del dashboard y las pantallas y rutas de la TUI.
5. **Fase verify & archive:** Validar la suite completa con 100% de éxito, verificar cobertura y archivar el incremento en la especificación viva.
