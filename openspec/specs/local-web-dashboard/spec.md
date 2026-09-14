# Especificación Viva: Servidor HTTP Local Embebido y Dashboard Web

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y escenarios BDD para el servidor HTTP local en Go, la API REST JSON, los assets frontend embebidos (`//go:embed`) y el subcomando de la CLI `axiom ui` en el runtime de Axiom.

---

## 1. Capacidad: `axiom-local-dashboard-server`

El paquete `internal/dashboard` implementa el servidor HTTP ligero basado en `net/http` que agrega y expone el estado del espacio de trabajo y del ciclo SDD mediante una API REST en formato JSON.

### Requirement: Endpoint REST de Información del Workspace (REQ-1.1)

El servidor DEBE responder a la ruta `GET /api/workspace` retornando una estructura JSON con los metadatos del espacio de trabajo declarados en `axiom.yaml`: nombre, topología (`monorepo-embedded`, `monorepo-decoupled`, `multirepo`), repositorio canónico de especificaciones, lista de roles con sus rutas y tecnologías, y el estado general de validación de la topología.

#### Scenario: Retorno exitoso de topología y roles
- **DADO** un espacio de trabajo con un archivo `axiom.yaml` válido
- **CUANDO** un cliente realiza una petición HTTP `GET /api/workspace`
- **ENTONCES** la respuesta tiene código de estado `200 OK`
- **Y** el cuerpo contiene un JSON con `name`, `topology`, `specs_repository` y la lista de roles declarados

#### Scenario: Reporte de error cuando el archivo de configuración es inválido
- **DADO** un espacio de trabajo donde `axiom.yaml` no existe o contiene sintaxis corrupta
- **CUANDO** se invoca `GET /api/workspace`
- **ENTONCES** la respuesta tiene código `500 Internal Server Error`
- **Y** el cuerpo JSON detalla el motivo del fallo de lectura o validación

---

### Requirement: Endpoints REST de Consulta de Incrementos (REQ-1.2)

El servidor DEBE responder a `GET /api/increments` listando todos los incrementos encontrados en el sistema de ficheros tanto en `openspec/changes/` (activos) como en `openspec/changes/archive/` (archivados), indicando su identificador, estado (`active` o `archived`), fase SDD deducida, y conteo de tareas completadas/totales.
Asimismo, DEBE responder a `GET /api/increments/{name}` proveyendo el detalle de los artefactos del incremento especificado (`proposal`, `spec`, `design`, `tasks`, `verifyReport`, `archiveReport`).

#### Scenario: Listado consolidado de incrementos activos y archivados
- **DADO** un repositorio con cambios archivados (ej. `inc-01`, `inc-02`, `inc-03`) y un cambio en curso
- **CUANDO** se realiza una petición HTTP `GET /api/increments`
- **ENTONCES** la respuesta tiene código `200 OK`
- **Y** el JSON contiene una lista con todos los incrementos categorizados por su estado y fase

#### Scenario: Consulta de detalle de un incremento existente
- **DADO** un incremento archivado o activo existente en disco
- **CUANDO** se solicita `GET /api/increments/{name}`
- **ENTONCES** se devuelve código `200 OK` con el resumen de sus artefactos y contenido estructurado

#### Scenario: Petición de incremento no existente
- **DADO** un identificador de cambio que no existe en `openspec/changes/` ni en su histórico
- **CUANDO** se solicita `GET /api/increments/cambio-inexistente`
- **ENTONCES** el servidor responde con código `404 Not Found`

---

### Requirement: Endpoint REST de Estado Multi-Rol y Barrera (REQ-1.3)

El servidor DEBE responder a `GET /api/roles?change={name}` analizando el desglose de roles del cambio solicitado mediante `internal/multirole`, retornando la lista de roles, sus políticas de compuerta (`blocking`, `deferred`, `optional`), el avance de sus tareas (`tasks.<rol>.md`), el estado de verificación (`verify-report.<rol>.md`) y el dictamen global de la barrera de sincronización (*Satisfied* o *Blocked*).

#### Scenario: Consulta de roles y diagnóstico de barrera para un cambio
- **DADO** un cambio con roles concurrentes declarados en `design.md`
- **CUANDO** se realiza una petición `GET /api/roles?change={name}`
- **ENTONCES** el servidor retorna código `200 OK`
- **Y** el JSON incluye la evaluación de cada rol y el estado de la barrera de sincronización

---

### Requirement: Endpoint REST de Consulta de Handoffs (REQ-1.4)

El servidor DEBE responder a `GET /api/handoffs?change={name}` deserializando el archivo `handoff.md` del cambio solicitado mediante `internal/handoff`, proveyendo los metadatos de la transición (fases origen y destino, roles involucrados, timestamp, status) y el contenido de las cinco secciones canónicas.

#### Scenario: Consulta de handoff existente
- **DADO** un cambio con un documento `handoff.md` generado
- **CUANDO** se ejecuta `GET /api/handoffs?change={name}`
- **ENTONCES** el servidor responde `200 OK` con los metadatos y las cinco secciones parseadas

#### Scenario: Consulta de handoff en cambio sin relevo activo
- **DADO** un cambio donde no existe el archivo `handoff.md`
- **CUANDO** se ejecuta `GET /api/handoffs?change={name}`
- **ENTONCES** el servidor responde con código `404 Not Found` y un mensaje JSON informativo

---

### Requirement: Endpoint REST de Catálogo de Skills (REQ-1.5)

El servidor DEBE responder a `GET /api/skills` escaneando los directorios locales de skills (`skills/` e `internal/assets/skills/`) y retornando la lista de skills disponibles, sus nombres, rutas relativas y una descripción resumida extraída de sus respectivos encabezados `SKILL.md`.

#### Scenario: Listado exitoso de skills del proyecto
- **DADO** un proyecto con skills definidas en `skills/` o `internal/assets/skills/`
- **CUANDO** se solicita `GET /api/skills`
- **ENTONCES** el servidor devuelve `200 OK` con el array de skills detectadas y sus metadatos

---

## 2. Capacidad: `axiom-web-ui-assets`

El paquete `internal/dashboard` incorpora los ficheros estáticos de la interfaz web embebidos directamente en el binario compilado mediante `//go:embed assets/*`, garantizando portabilidad absoluta y cero dependencias de NodeJS o NPM.

### Requirement: Servido de Assets Estáticos Embebidos (REQ-2.1)

El servidor HTTP DEBE servir los archivos estáticos HTML, CSS y JavaScript desde el sistema de archivos embebido de Go (`embed.FS`). La ruta raíz `/` DEBE entregar el archivo `index.html` con tipo de contenido `text/html; charset=utf-8`, y las rutas asociadas (`/style.css`, `/app.js`) DEBEN servirse con sus respectivos MIME types correctos.

#### Scenario: Carga de la página principal del dashboard
- **DADO** el servidor HTTP de Axiom en ejecución
- **CUANDO** se realiza una petición HTTP `GET /`
- **ENTONCES** el servidor responde con código `200 OK`
- **Y** entrega el documento HTML de la aplicación

#### Scenario: Carga de recursos estáticos CSS y JS
- **DADO** el servidor HTTP en ejecución
- **CUANDO** el cliente solicita `/style.css` o `/app.js`
- **ENTONCES** el servidor responde `200 OK` con las cabeceras `Content-Type: text/css` y `Content-Type: application/javascript` respectivamente

---

### Requirement: Estructura de Navegación y Vistas en la Web UI (REQ-2.2)

La interfaz web en `index.html` y `app.js` DEBE estructurarse en una experiencia de usuario fluida y reactiva en español que incluya:
1. Encabezado institucional con identidad de Axiom, nombre del workspace y badge de topología.
2. Navegación por pestañas (*tabs*):
   - **Tablero de Incrementos:** vista Kanban o lista con tarjetas interactivas agrupadas por fase SDD y porcentaje de tareas.
   - **Monitor Multi-Rol:** vista de detalle del cambio seleccionado con el estado de cada rol (`blocking` vs `deferred`), tareas y estado de la barrera.
   - **Visor de Handoffs:** renderizado claro de relevos con sus metadatos y acordeón de secciones.
   - **Topología y Salud:** inspección de repositorios asociados y repositorio de especificaciones.
   - **Catálogo de Skills:** visor de skills disponibles en el proyecto.

#### Scenario: Navegación interactiva entre vistas del dashboard
- **DADO** la interfaz web cargada en el navegador
- **CUANDO** el usuario selecciona una pestaña en la barra de navegación
- **ENTONCES** la vista correspondiente se activa dinámicamente sin recargar la página completa
- **Y** los datos se obtienen de los endpoints REST correspondientes

---

## 3. Capacidad: `axiom-cli-ui`

El comando `axiom ui` en la CLI de Axiom permite a los desarrolladores iniciar el servidor local y abrir el panel de control de forma desatendida o personalizada.

### Requirement: Subcomando axiom ui y Control del Servidor (REQ-3.1)

El comando `axiom ui` DEBE iniciar el servidor HTTP en el puerto indicado por `--port` (por defecto `8080`). Si el puerto especificado o por defecto ya está ocupado, el comando DEBE buscar de forma incremental el siguiente puerto libre disponible e informar en consola. Salvo que se indique `--no-browser`, el comando DEBE lanzar automáticamente el navegador predeterminado del sistema operativo apuntando a la URL local. Además, DEBE escuchar señales de interrupción (`SIGINT` / `SIGTERM`) para realizar un cierre ordenado (*graceful shutdown*).

#### Scenario: Inicio del servidor en puerto por defecto con apertura de navegador
- **DADO** el comando `axiom ui` ejecutado sin parámetros
- **CUANDO** el puerto 8080 está libre
- **ENTONCES** el servidor se inicia en `http://127.0.0.1:8080`
- **Y** se envía la orden de abrir el navegador en esa URL
- **Y** se muestra en consola el mensaje de inicio y la instrucción para detener con `Ctrl+C`

#### Scenario: Detección y fallback ante puerto ocupado
- **DADO** que el puerto 8080 está en uso por otro proceso
- **CUANDO** el usuario ejecuta `axiom ui`
- **ENTONCES** el sistema detecta la ocupación y se vincula al siguiente puerto libre (ej. `8081`)
- **Y** notifica en consola el puerto final utilizado

#### Scenario: Ejecución en modo headless con --no-browser
- **DADO** el comando ejecutado con la bandera `axiom ui --no-browser`
- **CUANDO** el servidor arranca
- **ENTONCES** el servidor HTTP queda a la escucha normalmente pero NO se invoca el navegador del sistema
