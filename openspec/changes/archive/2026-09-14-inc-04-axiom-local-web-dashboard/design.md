# Diseño Técnico: Servidor HTTP Local Embebido y Dashboard Web (INC-04)

## Contexto y Arquitectura General

El objetivo de este incremento es dotar al runtime de **Axiom** de una interfaz gráfica web local que permita inspeccionar visualmente la topología, el progreso de los incrementos SDD, el estado multi-rol y sus barreras de sincronización, los relevos estructurados (*handoffs*) y el catálogo de skills.

Para preservar la premisa fundacional de **cero dependencias externas y portabilidad absoluta**, la solución no utilizará servidores externos de Node.js, gestores de paquetes NPM ni frameworks pesados. Todo el servidor web y los assets estáticos residirán de forma autónoma dentro del binario `axiom.exe` mediante la librería estándar `net/http` y la directiva `//go:embed` de Go.

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Binario axiom.exe                                │
│                                                                             │
│  [ cmd/axiom/main.go ] ➔ Subcomando 'axiom ui'                              │
│         │                                                                   │
│         ▼                                                                   │
│  [ internal/dashboard/server.go ] ── (Servidor net/http + Router)           │
│         │                                        │                          │
│         ├─► [ assets.go //go:embed ]             ├─► [ service.go ]         │
│         │     ├─ index.html                      │     │                    │
│         │     ├─ style.css                       │     ├─► internal/workspace
│         │     └─ app.js                          │     ├─► internal/multirole
│         │                                        │     ├─► internal/handoff
│         │                                        │     ├─► openspec/ (disco)
│         │                                        │     └─► skills/ (disco)
│         ▼                                        ▼                          │
│   Ruta "/" (HTML/CSS/JS)                 API "/api/*" (JSON)                │
└───────────────────────────────────────────────────┬─────────────────────────┘
                                                    │
                                                    ▼
                                   [ Navegador Web del Usuario ]
                                   http://127.0.0.1:8080
```

---

## Roles Participantes

Siguiendo el estándar establecido en **INC-03 (`multi-role-sdd-fan-out`)**, este incremento asigna responsabilidades a dos roles concurrentes de la topología declarada en `axiom.yaml`:

```yaml
roles:
  core:
    gate_policy: blocking
    repositories:
      - "."
    deliverables:
      - internal/dashboard/types.go
      - internal/dashboard/service.go
      - internal/dashboard/server.go
      - internal/dashboard/assets.go
      - internal/dashboard/assets/index.html
      - internal/dashboard/assets/style.css
      - internal/dashboard/assets/app.js
      - cmd/axiom/main.go

  qa:
    gate_policy: blocking
    repositories:
      - "."
    deliverables:
      - internal/dashboard/dashboard_test.go
```

---

## 1. Diseño del Paquete `internal/dashboard`

### 1.1. Modelos de Transferencia de Datos (`types.go`)

Estructuras JSON para las respuestas de la API REST:

```go
package dashboard

import "github.com/gentleman-programming/gentle-ai/v2/internal/multirole"

// WorkspaceDTO expone los metadatos y salud del workspace.
type WorkspaceDTO struct {
    Name            string              `json:"name"`
    Topology        string              `json:"topology"`
    SpecsRepository string              `json:"specs_repository"`
    Root            string              `json:"root"`
    Roles           map[string]RoleMeta `json:"roles"`
    Compliant       bool                `json:"compliant"`
    Message         string              `json:"message"`
}

type RoleMeta struct {
    Name         string   `json:"name"`
    GatePolicy   string   `json:"gate_policy"`
    Repositories []string `json:"repositories"`
    Tech         []string `json:"tech"`
}

// IncrementSummaryDTO resume un cambio en el sistema.
type IncrementSummaryDTO struct {
    Name           string `json:"name"`
    Type           string `json:"type"` // "active" o "archived"
    Phase          string `json:"phase"`
    TasksTotal     int    `json:"tasks_total"`
    TasksCompleted int    `json:"tasks_completed"`
    ProgressPct    int    `json:"progress_pct"`
    Date           string `json:"date,omitempty"`
}

// IncrementDetailDTO contiene los artefactos y estado completo del cambio.
type IncrementDetailDTO struct {
    Summary        IncrementSummaryDTO        `json:"summary"`
    HasProposal    bool                       `json:"has_proposal"`
    HasSpec        bool                       `json:"has_spec"`
    HasDesign      bool                       `json:"has_design"`
    HasTasks       bool                       `json:"has_tasks"`
    HasVerify      bool                       `json:"has_verify"`
    HasArchive     bool                       `json:"has_archive"`
    Proposal       string                     `json:"proposal,omitempty"`
    Spec           string                     `json:"spec,omitempty"`
    Design         string                     `json:"design,omitempty"`
    TasksContent   string                     `json:"tasks_content,omitempty"`
    VerifyReport   string                     `json:"verify_report,omitempty"`
    ArchiveReport  string                     `json:"archive_report,omitempty"`
    BarrierReport  *multirole.BarrierReport   `json:"barrier_report,omitempty"`
}

// SkillDTO representa una skill disponible en el catálogo.
type SkillDTO struct {
    Name        string `json:"name"`
    Path        string `json:"path"`
    Description string `json:"description"`
    Trigger     string `json:"trigger,omitempty"`
}
```

### 1.2. Capa de Servicio (`service.go`)

`Service` encapsula el acceso al sistema de archivos local y orquesta las consultas a los paquetes de dominio existentes:
- `GetWorkspace(root string) (*WorkspaceDTO, error)`: Carga y valida la configuración con `workspace.LoadConfig` y `workspace.ValidateTopology`.
- `GetIncrements(root string) ([]IncrementSummaryDTO, error)`: Escanea `openspec/changes/` y `openspec/changes/archive/`. Detecta la fase actual inspeccionando la presencia de artefactos (`proposal.md`, `spec.md`, `design.md`, `tasks.md`, `verify-report.md`, `archive-report.md`) y cuenta tareas.
- `GetIncrementDetail(root, name string) (*IncrementDetailDTO, error)`: Lee el contenido completo de los artefactos del incremento.
- `GetRoleStatus(root, changeName string) (*multirole.BarrierReport, error)`: Invoca `multirole.EvaluateBarrier` con la topología de `axiom.yaml` para obtener el estado de compuertas y tareas por rol.
- `GetHandoff(root, changeName string) (*handoff.Handoff, error)`: Lee y parsea el archivo `handoff.md` usando `handoff.Parse`.
- `GetSkills(root string) ([]SkillDTO, error)`: Escanea `skills/` e `internal/assets/skills/` leyendo los metadatos de los archivos `SKILL.md`.

### 1.3. Servidor HTTP y Router REST (`server.go`)

Implementado mediante el `http.ServeMux` nativo de Go:
- Rutas registradas:
  - `GET /api/workspace`: Handler para estado del workspace.
  - `GET /api/increments`: Handler para listado de incrementos.
  - `GET /api/increments/`: Handler con captura de parámetro de ruta para el detalle.
  - `GET /api/roles`: Handler para estado multi-rol (`?change=<nombre>`).
  - `GET /api/handoffs`: Handler para handoff activo (`?change=<nombre>`).
  - `GET /api/skills`: Handler para catálogo de skills.
  - `GET /`: Servidor de archivos estáticos sobre `assetsFS` (sirve `index.html`, `style.css`, `app.js`).
- Middleware de cabeceras:
  - `Access-Control-Allow-Origin: *` (para facilitar pruebas locales).
  - `Content-Type: application/json; charset=utf-8` para rutas `/api/*`.
- Función `StartServer(addr string, svc *Service) (*http.Server, net.Listener, error)`:
  - Intenta abrir el listener en el puerto solicitado.
  - Si el puerto está ocupado, busca incrementalmente en los siguientes 10 puertos (ej. 8080 -> 8081 -> ... -> 8090).

### 1.4. Assets Embebidos (`assets.go`, `index.html`, `style.css`, `app.js`)

Ubicados en `internal/dashboard/assets/`:
- `assets.go`:
  ```go
  package dashboard

  import "embed"

  //go:embed assets/*
  var AssetsFS embed.FS
  ```
- **`index.html`:** Estructura SPA semántica con tema visual Axiom Enterprise (modo oscuro con paleta slate/azul/púrpura, tipografías del sistema limpias, indicadores con estados y badges).
- **`style.css`:** Sistema de diseño responsivo y moderno basado en CSS Grid y Flexbox, variables CSS, animaciones suaves y tarjetas visuales.
- **`app.js`:** Lógica de cliente en JavaScript Vanilla con `fetch` asíncrono, renderizado declarativo de tablas, barras de progreso de tareas, estado de la barrera de sincronización y visor de handoffs.

---

## 2. Integración en la CLI de Axiom (`cmd/axiom/main.go`)

Se añade el comando `axiom ui`:

```go
uiCmd := flag.NewFlagSet("ui", flag.ExitOnError)
uiPort := uiCmd.Int("port", 8080, "Puerto de escucha para el dashboard web (default: 8080)")
uiNoBrowser := uiCmd.Bool("no-browser", false, "No abrir el navegador automáticamente")
uiPath := uiCmd.String("path", ".", "Ruta raíz del espacio de trabajo de Axiom")
```

### Apertura del Navegador del Sistema Operativo
Se implementa la función utilitaria `openBrowser(url string) error`:
- En Windows: `exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()` o `exec.Command("cmd", "/c", "start", url).Start()`.
- En macOS: `exec.Command("open", url).Start()`.
- En Linux: `exec.Command("xdg-open", url).Start()`.

### Cierre Elegante (*Graceful Shutdown*)
El comando captura las señales del sistema operativo (`os.Interrupt`, `syscall.SIGTERM`) mediante un canal y ejecuta `server.Shutdown(ctx)` garantizando la liberación inmediata del puerto.

---

## 3. Plan de Pruebas Unitarias (`internal/dashboard/dashboard_test.go`)

1. `TestServiceWorkspace`: Verifica la carga correcta de metadatos de workspace y topología.
2. `TestServiceIncrements`: Verifica la detección de cambios activos y archivados con cálculo de tareas y fases.
3. `TestServiceIncrementDetail`: Verifica la recuperación del detalle de un incremento con artefactos.
4. `TestServiceRolesAndBarrier`: Verifica la integración con `internal/multirole` para consultar compuertas y barreras.
5. `TestServiceHandoff`: Verifica la lectura y deserialización de `handoff.md`.
6. `TestServiceSkills`: Verifica el escaneo de skills del proyecto.
7. `TestHTTPRoutes`: Verifica con `httptest.NewServer` que todas las rutas `/api/*` y la raíz `/` respondan con código `200 OK` y los tipos MIME apropiados.
8. `TestPortFallback`: Verifica la lógica de selección de puerto alternativo cuando un listener ya está ocupando el puerto inicial.
