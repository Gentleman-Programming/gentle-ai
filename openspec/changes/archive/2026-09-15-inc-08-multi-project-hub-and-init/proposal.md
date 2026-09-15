# Propuesta: Hub Multi-Proyecto, Selector Dinámico en Dashboard Web y CLI axiom init (INC-08)

## Propósito (Intent)

En entornos de desarrollo profesionales, un programador o equipo gestiona simultáneamente múltiples repositorios y proyectos en su estación de trabajo (algunos con arquitecturas monorepo, otros multirepo, algunos ya gobernados bajo la metodología SDD de Axiom y otros proyectos legados o en fases previas de adopción).

Actualmente, el binario global de Axiom (`axiom.exe`) opera bajo la asunción de un único workspace fijado al directorio de ejecución actual (`.`). Cuando un usuario ejecuta el comando `axiom ui` desde una carpeta de otro proyecto (por ejemplo `C:\repos\ludeka`):
1. La herramienta intenta cargar `axiom.yaml` local y genera un error bloqueante si el proyecto aún no está configurado.
2. Si el proyecto tiene una carpeta `openspec/` previa, mezcla o expone artefactos sin validación de topología.
3. El usuario carece de una forma sencilla de inicializar formalmente un proyecto (`axiom init`).
4. El Dashboard Web (`axiom ui`) carece de un selector para alternar de forma transparente entre proyectos gestionados en la misma máquina sin reiniciar el servidor ni cambiar de terminal.

El **Incremento 8 (INC-08: `multi-project-hub-and-init`)** soluciona esta fricción convirtiendo a Axiom en una **plataforma Multi-Proyecto integral**:
- El binario `axiom` reside globalmente en el sistema operativo (en `$PATH`).
- La gobernanza y configuración permanecen estrictamente locales y encapsuladas por proyecto (`axiom.yaml`, `openspec/`, `.axiom/`).
- Se introduce un **Registro Global Central de Proyectos** (`~/.axiom/workspaces.json`) que almacena el catálogo de proyectos gestionados por el desarrollador.
- Se implementa el comando **`axiom init`** para inicializar e incorporar cualquier repositorio existente o nuevo con auto-detección tecnológica (Go, C#, Node/TypeScript, Python, Rust) en un solo paso.
- Se transforma el **Dashboard Web (`axiom ui`)** en un **Hub de Control Multi-Proyecto**, dotando al Navbar de un selector interactivo en caliente, registro dinámico de nuevos proyectos y pantalla de bienvenida para carpetas aún no inicializadas (*Zero-Doc Welcome Screen*).

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

1. **Gestor del Registro Central de Workspaces (`internal/hub/`):**
   - Modelos de datos para el registro global (`~/.axiom/workspaces.json`):
     - `WorkspaceRecord`: ID, nombre, ruta absoluta, topología, última fecha de acceso, estado de conformidad.
     - `HubConfig`: Lista de proyectos registrados, proyecto activo actual (`active_workspace_id`), versión del esquema.
   - Funcionalidades del `HubManager`:
     - `Register(path, name, topology)`: Valida y registra un nuevo proyecto.
     - `Unregister(idOrPath)`: Desvincula un proyecto del registro global sin alterar sus archivos físicos.
     - `List()`: Retorna todos los proyectos registrados con su estado de salud.
     - `SetActive(idOrPath)`: Establece el proyecto activo por defecto.
     - `GetActive()`: Obtiene el proyecto activo actual o resuelve por fallback inteligente.
     - `AutoDiscover(baseDir)`: Detecta proyectos con `axiom.yaml` en directorios adyacentes.

2. **Comando CLI `axiom init`:**
   - Sintaxis: `axiom init [--name <nombre>] [--path <dir>] [--topology <tipo>] [--yes]`
   - Auto-detección tecnológica del proyecto:
     - `go.mod` ➔ Go
     - `package.json` ➔ Node / TypeScript / React
     - `*.csproj` / `*.sln` ➔ .NET / C#
     - `Cargo.toml` ➔ Rust
     - `requirements.txt` / `pyproject.toml` ➔ Python
   - Generación del archivo canónico `axiom.yaml` con roles iniciales coherentes (`core`, `qa`).
   - Inicialización del andamiaje base: `.axiom/inbox/skills/`, `openspec/specs/`, `openspec/changes/`.
   - Registro automático en `~/.axiom/workspaces.json` y marcado como proyecto activo.
   - Si el proyecto ya tiene `axiom.yaml`, lo valida y simplemente lo registra en el Hub si no lo estaba.

3. **Comandos CLI de Gestión de Proyectos (`axiom project` / `axiom workspace`):**
   - `axiom project list`: Muestra la tabla de proyectos registrados, cuál está activo y su estado de cumplimiento.
   - `axiom project switch <id|nombre|ruta>`: Cambia el proyecto activo.
   - `axiom project add <ruta> [--name <nombre>]`: Registra un proyecto existente.
   - `axiom project remove <id|nombre>`: Desregistra un proyecto.

4. **Evolución del Dashboard Web Local a Hub Multi-Proyecto (`internal/dashboard/`):**
   - **Backend HTTP (`server.go` & `service.go`):**
     - Integración de `HubManager` en el ciclo de vida del servidor web.
     - Soporte para workspaces dinámicos: el dashboard ya no está anclado a un `rootPath` fijo.
     - Nuevos endpoints REST:
       - `GET /api/projects`: Lista de proyectos registrados, indicando el activo.
       - `POST /api/projects/switch`: Conmutar el proyecto activo en caliente sin reiniciar el servidor.
       - `POST /api/projects/add`: Registrar un nuevo proyecto desde la interfaz web.
       - `POST /api/projects/init`: Inicializar el proyecto actual o una ruta seleccionada directamente desde la UI.
   - **Frontend SPA (`internal/dashboard/assets/`):**
     - **Navbar Project Selector:** Desplegable interactivo en la cabecera mostrando el proyecto actual, permitiendo cambiar de proyecto con un solo clic.
     - **Modal "+ Añadir Proyecto":** Diálogo intuitivo para ingresar una ruta de carpeta local y sumarla al Hub.
     - **Pantalla "Zero-Config Welcome":** Si el servidor se abre en una carpeta sin `axiom.yaml`, muestra una tarjeta de bienvenida visual con la tecnología detectada y un botón de acción primaria: *"Inicializar Proyecto con Axiom"*.

5. **Pruebas Unitarias Exhaustivas:**
   - Pruebas del gestor `internal/hub/hub_test.go` (creación, lectura, concurrencia, idempotencia de registro, persistencia en JSON).
   - Pruebas de inicialización y auto-detección tecnológica.
   - Pruebas de integración de endpoints REST en `internal/dashboard/dashboard_test.go`.

### Fuera de Alcance (Out of Scope)

- Sincronización en la nube o almacenamiento multi-usuario en servidores remotos (Axiom es 100% soberano, local en filesystem).
- Borrado destructivo de archivos de proyectos al desregistrar (la desvinculación solo afecta al índice de workspaces).

---

## Capacidades (Capabilities)

### Nuevas Capacidades

- `multi-project-hub`: Capacidad de registrar, auditar y conmutar múltiples proyectos de software en una sola máquina mediante un registro de usuario centralizado (`~/.axiom/workspaces.json`).
- `project-initializer`: Capacidad de inicializar proyectos desde cero o sobre código legado con auto-detección de stack (`axiom init`).
- `dashboard-project-switcher`: Capacidad de la interfaz web local (`axiom ui`) de operar como un Hub centralizado con selector dinámico de workspaces y bienvenida a proyectos no gestionados.

---

## Enfoque de Implementación (Approach)

1. **Diseñar el paquete `internal/hub`:**
   - Crear `types.go`, `manager.go` y `detector.go`.
   - Implementar lectura/escritura atómica de `~/.axiom/workspaces.json`.
2. **Implementar el Inicializador (`internal/hub/init.go`):**
   - Lógica de auto-detección y generación de `axiom.yaml` y carpetas base.
3. **Desarrollar Pruebas Unitarias de Hub:**
   - Cobertura 100% en `internal/hub/hub_test.go`.
4. **Actualizar el Servicio y Servidor del Dashboard (`internal/dashboard`):**
   - Adaptar `Service` para soportar cambio de workspace en tiempo de ejecución.
   - Implementar endpoints `/api/projects`, `/api/projects/switch`, `/api/projects/add`, `/api/projects/init`.
5. **Actualizar la UI Web (`internal/dashboard/assets/`):**
   - Modificar `index.html`, `app.js` y `style.css` para renderizar el selector desplegable de proyectos en el Navbar, el modal de registro y el estado de bienvenida sin configuración previa.
6. **Extender la CLI en `cmd/axiom/main.go`:**
   - Añadir comando `axiom init`.
   - Añadir comando `axiom project [list|switch|add|remove]`.
   - Enlazar `axiom ui` con el Hub de proyectos.
7. **Verificación y Pruebas Globales:**
   - Ejecución de tests unitarios y verificación manual de la CLI y la UI.
8. **Generación de Artefactos SDD y Cierre:**
   - `spec.md`, `design.md`, `tasks.md`, `verify-report.md`, `archive-report.md`.
