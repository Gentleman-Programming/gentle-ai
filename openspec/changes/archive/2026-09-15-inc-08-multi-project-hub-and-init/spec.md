# Especificación de Requerimientos: Hub Multi-Proyecto, Selector Dinámico en Dashboard Web y CLI axiom init (INC-08)

## Propósito

Definir formal y deterministamente los requerimientos funcionales, no funcionales y escenarios BDD para dotar a Axiom de una arquitectura multi-proyecto de nivel empresarial: registro global central de workspaces (`~/.axiom/workspaces.json`), comando CLI `axiom init` con auto-detección tecnológica, comandos CLI `axiom project`, y un Dashboard Web (`axiom ui`) transformado en Hub con selector dinámico de workspaces en tiempo real y pantalla de bienvenida para repositorios sin inicializar.

---

## 1. Capacidad: `multi-project-hub`

El paquete `internal/hub` provee persistencia, auditoría y gestión de múltiples workspaces en la máquina del desarrollador.

### Requirement: Registro Global Centralizado de Workspaces (REQ-1.1)

El sistema DEBE persistir la lista de proyectos conocidos en un archivo JSON ubicado en el perfil de usuario: `$HOME/.axiom/workspaces.json` (o `%USERPROFILE%\.axiom\workspaces.json` en Windows). El archivo DEBE almacenar:
- Versión del esquema (`"1.0"`).
- ID o ruta del proyecto activo por defecto (`active_workspace`).
- Lista de proyectos registrados (`workspaces`), donde cada entrada contiene:
  - `id`: Identificador alfanumérico único derivado del nombre o slug del directorio.
  - `name`: Nombre descriptivo del proyecto.
  - `path`: Ruta absoluta canónica en el sistema de archivos local.
  - `topology`: Tipo de topología (`monorepo-embedded`, `multirepo`, o `unconfigured`).
  - `registered_at`: Timestamp RFC3339 de registro.
  - `last_accessed`: Timestamp RFC3339 del último acceso.

#### Scenario: Creación del archivo de registro en el primer uso
- **DADO** que no existe el directorio ni el archivo `~/.axiom/workspaces.json`
- **CUANDO** se invoca cualquier operación del gestor de Hub
- **ENTONCES** crea automáticamente el directorio `.axiom` y el archivo `workspaces.json` inicial con un array vacío de proyectos
- **Y** no produce ningún fallo de ejecución

#### Scenario: Registro idempotente de un proyecto existente
- **DADO** un proyecto ubicado en `C:\repos\axiom`
- **CUANDO** se llama a `Register(path, name, topology)`
- **ENTONCES** si el proyecto ya estaba registrado por su ruta normalizada, actualiza su nombre, topología y `last_accessed` sin duplicar la entrada
- **Y** persiste el cambio de forma atómica

#### Scenario: Desregistro de un proyecto
- **DADO** un proyecto registrado con ID `ludeka`
- **CUANDO** se ejecuta `Unregister("ludeka")`
- **ENTONCES** elimina la entrada de `workspaces.json`
- **Y** no elimina ni modifica ningún archivo físico del disco del proyecto

---

### Requirement: Conmutación y Resolución de Proyecto Activo (REQ-1.2)

El gestor DEBE permitir consultar y establecer cuál es el proyecto activo del desarrollador. Si el proyecto marcado como activo ya no existe físicamente en el disco, el gestor DEBE realizar un fallback inteligente hacia el directorio actual si contiene `axiom.yaml`, o hacia el primer proyecto válido del catálogo.

#### Scenario: Conmutación de proyecto activo
- **DADO** un Hub con dos proyectos registrados: `axiom` y `ludeka`
- **CUANDO** se invoca `SetActive("ludeka")`
- **ENTONCES** el campo `active_workspace` pasa a ser el ID de `ludeka`
- **Y** su timestamp `last_accessed` se actualiza al momento actual

#### Scenario: Fallback cuando la ruta configurada fue eliminada
- **DADO** un proyecto activo cuya ruta fue borrada del disco
- **CUANDO** se invoca `GetActive()`
- **ENTONCES** advierte la inexistencia del directorio y retorna el primer proyecto disponible o el directorio de trabajo actual

---

## 2. Capacidad: `project-initializer`

El gestor provee inicialización asistida y automática de proyectos (`axiom init`), permitiendo adoptar proyectos existentes o inicializar nuevos sin fricción.

### Requirement: Detección Heurística de Tecnologías (REQ-2.1)

El inicializador DEBE inspeccionar los archivos del repositorio para deducir la tecnología principal y configurar adecuadamente los roles y herramientas en `axiom.yaml`:
- Si detecta `go.mod` ➔ Tecnología: `go` / `go-test`.
- Si detecta `package.json` ➔ Tecnología: `nodejs` / `typescript` (o `react`/`vue`/`svelte` si están en dependencias).
- Si detecta `*.csproj` o `*.sln` ➔ Tecnología: `dotnet` / `csharp`.
- Si detecta `Cargo.toml` ➔ Tecnología: `rust`.
- Si detecta `pyproject.toml` o `requirements.txt` ➔ Tecnología: `python`.

#### Scenario: Detección automática en proyecto Go
- **DADO** un directorio con un archivo `go.mod`
- **CUANDO** se ejecuta el detector tecnológico
- **ENTONCES** retorna `go` como tecnología base del rol `core` y `go-test` para el rol `qa`

#### Scenario: Detección automática en proyecto .NET / C#
- **DADO** un directorio con un archivo `Ludeka.sln` o `*.csproj`
- **CUANDO** se ejecuta el detector tecnológico
- **ENTONCES** retorna `csharp` y `dotnet` en la configuración inicial

---

### Requirement: Generación de Andamiaje `axiom.yaml` y Registro Automático (REQ-2.2)

Al ejecutar `axiom init` sobre un directorio:
1. Si NO existe `axiom.yaml`, genera un archivo canónico con el nombre del proyecto, topología seleccionada (`monorepo-embedded`), repositorio de especificaciones (`openspec`), roles detectados (`core` y `qa`) y gobernanza en español.
2. Crea las carpetas base necesarias: `.axiom/inbox/skills/`, `openspec/specs/` y `openspec/changes/`.
3. Registra automáticamente el proyecto en `~/.axiom/workspaces.json` y lo marca como proyecto activo.
4. Si YA existe `axiom.yaml`, no lo sobreescribe; valida su estructura y lo registra en el Hub global si no estaba registrado.

#### Scenario: Inicialización exitosa de proyecto virgen
- **DADO** un directorio limpio sin archivos de Axiom
- **CUANDO** se ejecuta `axiom init --name "MiProyecto"`
- **ENTONCES** se crea `axiom.yaml` con la estructura canónica
- **Y** se crean las carpetas `openspec/specs/`, `openspec/changes/` y `.axiom/inbox/skills/`
- **Y** el proyecto queda registrado en `~/.axiom/workspaces.json`

---

## 3. Capacidad: `dashboard-project-switcher`

El Dashboard Web (`axiom ui`) opera como un Centro de Control Multi-Proyecto con conmutación en tiempo real.

### Requirement: Endpoints REST de Gestión Multi-Proyecto (REQ-3.1)

El servidor HTTP del dashboard DEBE exponer endpoints bajo `/api/projects`:
- `GET /api/projects`: Retorna el listado de todos los proyectos registrados, indicando cuál es el activo, sus rutas y si tienen `axiom.yaml` válido.
- `POST /api/projects/switch`: Recibe `{"id": "..."}` o `{"path": "..."}` y conmuta el workspace activo del servicio en caliente sin requerir reinicio del servidor.
- `POST /api/projects/add`: Recibe `{"path": "...", "name": "..."}` para incorporar una carpeta existente al registro.
- `POST /api/projects/init`: Recibe parámetros de inicialización y ejecuta `axiom init` sobre la ruta actual o indicada.

#### Scenario: Conmutación en caliente de workspace en la UI
- **DADO** el dashboard ejecutándose en el puerto 8080 mostrando el proyecto `Axiom`
- **CUANDO** el cliente envía `POST /api/projects/switch` con el ID de `Ludeka`
- **ENTONCES** las subsiguientes llamadas a `/api/workspace`, `/api/increments` y `/api/archive/specs` devuelven inmediatamente los datos del proyecto `Ludeka`

---

### Requirement: Componente UI Selector de Proyectos y Pantalla de Bienvenida (REQ-3.2)

La interfaz web (SPA) DEBE incorporar:
1. **Selector Desplegable en el Navbar:** Muestra el nombre del proyecto activo actual con un icono de switch. Al desplegarlo, lista los proyectos disponibles con acceso rápido para alternar entre ellos y un botón "+ Añadir Proyecto".
2. **Modal de Registro de Proyecto:** Formulario simple para ingresar la ruta de un repositorio y agregarlo al catálogo.
3. **Estado "Zero-Config / Welcome":** Si el proyecto seleccionado o la carpeta donde se lanzó la UI no cuenta con `axiom.yaml`, la UI NO muestra un error 500 ni pantalla en blanco; muestra una tarjeta visual informativa:
   - *"Este repositorio no está gobernado con Axiom aún."*
   - *"Tecnología detectada: [X]."*
   - Botón de acción: *"[ Inicializar Proyecto con Axiom ]"*.

#### Scenario: Navegación a proyecto no inicializado
- **DADO** que la UI se abre en una carpeta sin `axiom.yaml`
- **CUANDO** carga la página en el navegador
- **ENTONCES** se presenta la pantalla de bienvenida con el botón para inicializar
- **Y** al pulsar el botón, invoca `/api/projects/init` y recarga la UI con el proyecto ya operativo
