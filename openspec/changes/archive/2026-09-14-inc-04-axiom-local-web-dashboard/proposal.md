# Propuesta: Servidor HTTP Local Embebido y Dashboard Web de Axiom (INC-04)

## Propósito (Intent)

Hasta ahora, la interacción con las capacidades de **Axiom** (topología de workspaces en INC-01, ciclo de vida de handoffs estructurados en INC-02 y concurrencia multi-rol con barrera de sincronización en INC-03) se ha realizado exclusivamente mediante la interfaz de línea de comandos (`axiom.exe`).

Aunque la CLI es indispensable para la automatización, scripts y agentes, los desarrolladores humanos, líderes técnicos y equipos de producto y QA requieren una forma visual, intuitiva e interactiva de:
1. Inspeccionar de un vistazo la salud y topología del workspace (`axiom.yaml`, repositorios mapeados, repositorio canónico de especificaciones).
2. Ver el tablero de incrementos SDD (activos y archivados), identificando en qué fase se encuentra cada cambio (`explore` ➔ `archive`) y el avance de sus tareas.
3. Monitorear la ejecución concurrente de roles (`core`, `qa`, `e2e`), observando qué roles son obligatorios (`blocking`) vs. asíncronos (`deferred`), y el estado de la barrera de sincronización (*Fan-In*).
4. Leer y consultar los documentos estructurados de relevo (`handoff.md`), sus metadatos y las instrucciones directas para los siguientes roles.
5. Explorar el catálogo de skills disponibles en el proyecto.

El objetivo de este incremento es dotar a Axiom de su **servidor HTTP local embebido** y su **Dashboard Web** (`axiom ui`), empaquetado de forma 100% autónoma en el binario Go mediante `//go:embed`, sin dependencias externas de NodeJS o servidores adicionales.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

- **Paquete de dominio y servidor web en el runtime de Axiom (`internal/dashboard/`):**
  - `types.go`: DTOs para serialización JSON de workspace, incrementos, fases, roles, handoffs y skills.
  - `service.go`: Capa de servicio que integra `internal/workspace`, `internal/multirole`, `internal/handoff` y el sistema de archivos (`openspec/`) para consolidar la información.
  - `server.go`: Servidor HTTP basado en la librería estándar de Go (`net/http`) con endpoints REST JSON y servidor de ficheros estáticos.
  - `assets/`: Frontend web ligero (HTML5, CSS3 responsivo, JavaScript Vanilla) con tema oscuro/claro, diseño visual moderno de Axiom y vistas para:
    - Tablero de incrementos y ciclo SDD.
    - Monitor multi-rol y barrera de sincronización.
    - Visor interactivo de handoffs.
    - Diagnóstico de topología y repositorios.
    - Catálogo de skills.
  - `assets.go`: Embebido de los archivos estáticos mediante la directiva `//go:embed`.
- **Integración CLI en `cmd/axiom/main.go`:**
  - Subcomando `axiom ui`:
    - `--port <n>` (puerto configurable, por defecto `8080`, con fallback automático si está ocupado).
    - `--no-browser` (opción para arrancar sin abrir la ventana del navegador).
    - `--path <directorio>` (ruta del workspace).
  - Apertura automática del navegador del sistema operativo (`cmd /c start` en Windows / soporte multiplataforma).
  - Cierre elegante (*graceful shutdown*) al presionar `Ctrl+C`.
- **Suite completa de pruebas unitarias (`internal/dashboard/dashboard_test.go`):**
  - Pruebas de los endpoints REST con `net/http/httptest`.
  - Pruebas del servicio de lectura y agregación del estado.
  - Verificación del servido de assets embebidos.

### Fuera de Alcance (Out of Scope)

- Minería heurística automática de código para generación de skills (objeto del **INC-05**).
- Navegación semántica de grafos de código con Serena / CodeGraph (objeto del **INC-06**).
- Edición bidireccional pesada o modificación del código fuente desde la interfaz web (el dashboard inicial es de monitoreo, lectura y validación).

---

## Capacidades (Capabilities)

### Nuevas Capacidades

- `axiom-local-dashboard-server`: Servidor HTTP en Go que expone endpoints REST JSON con el estado vivo de Axiom (`/api/workspace`, `/api/increments`, `/api/roles`, `/api/handoffs`, `/api/skills`).
- `axiom-web-ui-assets`: Interfaz web interactiva embebida en el binario sin dependencias externas en tiempo de ejecución.
- `axiom-cli-ui`: Subcomando `axiom ui` para levantar el servidor y abrir el navegador de forma transparente.

---

## Enfoque de Implementación (Approach)

1. Crear el paquete `internal/dashboard/` con `types.go`, `service.go` y `server.go`.
2. Crear los archivos de la interfaz web en `internal/dashboard/assets/` (`index.html`, `style.css`, `app.js`) y el fichero de enlace `assets.go` con `//go:embed assets/*`.
3. Desarrollar la suite de pruebas unitarias en `internal/dashboard/dashboard_test.go`.
4. Integrar el subcomando `axiom ui` en `cmd/axiom/main.go`.
5. Validar con pruebas automatizadas (`go test -v ./internal/dashboard/...`), compilar `axiom.exe` y realizar pruebas en vivo en el navegador.
