# Especificación de Requerimientos: Paridad Bidireccional entre TUI y Web UI (INC-17)

## Propósito

Definir formalmente los requerimientos funcionales, contratos de API, pantallas y escenarios BDD para lograr la paridad bidireccional entre la Terminal User Interface (TUI Bubbletea) y el Dashboard Web (`axiom ui`), permitiendo tanto la gestión completa del ecosistema local desde la Web UI como la gobernanza SDD y multi-proyecto desde la TUI.

---

## 1. Capacidad: `dashboard-ecosystem-management`

Permite a los usuarios operar las tareas de mantenimiento, diagnóstico y configuración del ecosistema de desarrollo de Axiom directamente desde el Dashboard Web SPA.

### Requirement: Diagnóstico de Salud del Sistema (Axiom Doctor) en Web UI (REQ-17.1)
El servidor HTTP local DEBE exponer el endpoint `GET /api/ecosystem/doctor` que recopila y retorna el estado de salud de los agentes compatibles (Claude Code, OpenCode, Kiro, Codex, Pi), herramientas del sistema (Git, Go, Node.js), permisos del directorio de usuario (`~/.axiom/`) y configuración general. El frontend web DEBE renderizar estos diagnósticos en tarjetas con indicadores visuales claros (verde: saludable, amarillo: advertencia, rojo: error).

#### Scenario: Consulta de diagnóstico de salud del sistema
- **DADO** el servidor del Dashboard Web activo
- **CUANDO** se realiza una petición `GET /api/ecosystem/doctor`
- **ENTONCES** el servidor responde con código `200 OK`
- **Y** el cuerpo JSON contiene la lista de diagnósticos con campos `name`, `status` (`ok`, `warning`, `error`), `details` y `recommendation`

---

### Requirement: Sincronización y Actualización Reactiva de Herramientas en Web UI (REQ-17.2)
El servidor HTTP local DEBE exponer los endpoints `POST /api/ecosystem/sync` y `POST /api/ecosystem/upgrade` para disparar las operaciones equivalentes a `axiom sync` y `axiom upgrade`. El frontend web DEBE ofrecer botones de acción interactiva en la nueva pestaña `⚙️ Ecosistema & Herramientas` y mostrar un registro de salida con los resultados de la operación.

#### Scenario: Ejecución de sincronización de configuraciones
- **DADO** la pestaña de Ecosistema en el Dashboard Web
- **CUANDO** el usuario pulsa el botón "Sincronizar Configuraciones"
- **Y** se envía una petición `POST /api/ecosystem/sync`
- **ENTONCES** el servidor ejecuta la sincronización de archivos gestionados y reglas de agentes
- **Y** responde con código `200 OK` y el resumen de archivos sincronizados en formato JSON

#### Scenario: Ejecución de comprobación y actualización de herramientas
- **DADO** la pestaña de Ecosistema en el Dashboard Web
- **CUANDO** el usuario pulsa el botón "Actualizar Herramientas"
- **Y** se envía una petición `POST /api/ecosystem/upgrade`
- **ENTONCES** el servidor comprueba versiones disponibles y actualiza componentes si procede
- **Y** retorna el informe de actualización en formato JSON

---

### Requirement: Gestión Visual de Respaldos en Web UI (REQ-17.3)
El servidor HTTP local DEBE exponer los endpoints `GET /api/ecosystem/backups`, `POST /api/ecosystem/backups/create` y `POST /api/ecosystem/backups/restore` integrados con el paquete `internal/backup`. El frontend web DEBE presentar la lista cronológica de snapshots en `~/.axiom/backups/`, permitir la creación de un nuevo respaldo bajo demanda con nombre descriptivo opcional y permitir restaurar un respaldo seleccionado con confirmación previa.

#### Scenario: Listado y creación de un respaldo desde el navegador
- **DADO** el servidor activo con respaldos existentes en `~/.axiom/backups/`
- **CUANDO** se realiza una petición `GET /api/ecosystem/backups`
- **ENTONCES** el servidor retorna la lista de respaldos con fecha, identificador y lista de archivos
- **CUANDO** se envía `POST /api/ecosystem/backups/create` con `{ "description": "pre-migracion" }`
- **ENTONCES** el servidor crea el nuevo snapshot y responde con `201 Created`

---

### Requirement: Inspección y Configuración de Modelos de IA en Web UI (REQ-17.4)
El servidor HTTP local DEBE exponer el endpoint `GET /api/ecosystem/models` para consultar la asignación vigente de modelos de IA y configuraciones por rol (arquitecto, ejecutor, revisor) para los agentes soportados.

#### Scenario: Consulta de modelos asignados en el Dashboard
- **DADO** una configuración activa de modelos en el proyecto o usuario
- **CUANDO** se realiza una petición `GET /api/ecosystem/models`
- **ENTONCES** el servidor responde con `200 OK` y el mapa de asignaciones de modelos y niveles de esfuerzo de razonamiento

---

## 2. Capacidad: `tui-hub-and-sdd-governance`

Permite a los desarrolladores operar la gobernanza Spec-Driven Development y la gestión multi-proyecto directamente desde la interfaz interactiva de terminal Bubbletea.

### Requirement: Submenú Unificado de Proyectos y Gobernanza SDD en TUI (REQ-17.5)
La pantalla de bienvenida de la TUI (`WelcomeOptions`) DEBE incorporar una opción de menú `📁 Proyectos y Gobernanza SDD ➔` que conduzca a un submenú o pantalla navegable dedicada a la gestión de proyectos y ciclo de vida de la plataforma, preservando la ergonomía visual en terminales compactos.

#### Scenario: Navegación al submenú de gobernanza desde la bienvenida
- **DADO** el menú principal de la TUI interactiva de Axiom
- **CUANDO** el cursor selecciona la opción `📁 Proyectos y Gobernanza SDD ➔` y presiona Enter
- **ENTONCES** la TUI conmuta a la pantalla o submenú de gobernanza
- **Y** al presionar Esc o seleccionar "Volver al menú principal", regresa a `ScreenWelcome`

---

### Requirement: Pantalla de Gestión Multi-Proyecto en TUI (REQ-17.6)
La TUI DEBE implementar la pantalla `ScreenHubProjects` que lea los proyectos registrados en `~/.axiom/workspaces.json` mediante `internal/hub`. DEBE permitir:
1. Visualizar la lista de proyectos registrados con su nombre, ruta y estado de salud.
2. Conmutar el proyecto activo en caliente.
3. Registrar un proyecto existente introduciendo su ruta (`axiom project add`).
4. Inicializar el directorio actual si carece de `axiom.yaml` (`axiom init`) con detección de stack tecnológico.

#### Scenario: Conmutación de proyecto activo en la TUI
- **DADO** la pantalla `ScreenHubProjects` con 2 o más proyectos registrados
- **CUANDO** el usuario navega a un proyecto y presiona Enter
- **ENTONCES** el proyecto se marca como activo en el Hub
- **Y** se actualiza el contexto del workspace en la sesión interactiva

---

### Requirement: Pantalla de Ciclo de Vida de Incrementos SDD en TUI (REQ-17.7)
La TUI DEBE implementar la pantalla `ScreenSDDIncrements` que liste los incrementos en `openspec/changes/` (activos) y `openspec/changes/archive/` (archivados), mostrando su fase actual (`explore`, `propose`, `spec`, `design`, `tasks`, `apply`, `verify`, `archive`). DEBE permitir:
1. Crear un nuevo incremento de forma guiada (`axiom change create`) solicitando nombre e intención.
2. Disparar el avance de fase (`sdd continue`) con atajo de teclado.
3. Validar el reporte de verificación formal (`sdd verify-validate`).

#### Scenario: Visualización y avance de fase de un incremento
- **DADO** la pantalla `ScreenSDDIncrements` con un cambio activo en fase `proposal`
- **CUANDO** el usuario selecciona el cambio y presiona `c` (continuar)
- **ENTONCES** se ejecuta la transición del despachador SDD
- **Y** la interfaz actualiza en vivo el estado de la siguiente fase recomendada

---

### Requirement: Pantalla de Monitor Multi-Rol y Barrera Fan-In en TUI (REQ-17.8)
La TUI DEBE implementar la pantalla `ScreenMultiRole` que permita seleccionar un cambio activo, inspeccionar los roles definidos en `design.md`, clasificar compuertas obligatorias (`blocking`) vs diferidas (`deferred`), verificar el avance porcentual de tareas por rol y evaluar la barrera de sincronización Fan-In, ofreciendo la opción de migrar tareas diferidas acumulativas a `openspec/changes/e2e-cumulative/tasks.md`.

#### Scenario: Evaluación de barrera multi-rol desde la TUI
- **DADO** un cambio activo con múltiples roles asignados
- **CUANDO** el usuario accede a `ScreenMultiRole`
- **ENTONCES** se evalúa la barrera de sincronización usando `internal/multirole`
- **Y** se renderiza el veredicto de compuerta (abierta o bloqueada) con el desglose de tareas pendientes

---

### Requirement: Pantalla de Visor de Handoffs Estructurados en TUI (REQ-17.9)
La TUI DEBE implementar la pantalla `ScreenHandoffs` que presente el contenido parseado de `handoff.md` del cambio activo con sus metadatos de transición (fase origen ➔ fase destino, roles) y secciones canónicas en castellano.

#### Scenario: Inspección de relevo formal en la TUI
- **DADO** un cambio con artefacto `handoff.md` válido
- **CUANDO** el usuario accede a `ScreenHandoffs`
- **ENTONCES** la pantalla muestra el flujo de fases y el contenido de las secciones estructuradas

---

### Requirement: Pantalla de Catálogo de Especificaciones Vivas en TUI (REQ-17.10)
La TUI DEBE implementar la pantalla `ScreenLivingDoc` que liste las especificaciones vivas disponibles en `openspec/specs/` con sus dominios y versiones, y permita disparar la sincronización del catálogo maestro `INDEX.md` con un atajo de teclado `s`.

#### Scenario: Sincronización del catálogo maestro desde la TUI
- **DADO** la pantalla `ScreenLivingDoc`
- **CUANDO** el usuario presiona la tecla `s` (sincronizar)
- **ENTONCES** se invoca `livingdoc.Service.SyncCatalog()`
- **Y** se muestra un mensaje de confirmación con el total de dominios y requisitos actualizados
