# Diseño de Arquitectura: Paridad Bidireccional entre TUI y Web UI (INC-17)

## 1. Visión General y Diagrama de Arquitectura

El objetivo del Incremento 17 es establecer una paridad bidireccional integral entre la interfaz de terminal (TUI) y el panel de control web (Web UI), apoyándose en la capa común de dominio de Axiom:

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                                   Interfaces de Usuario                                │
├───────────────────────────────────────────┬────────────────────────────────────────────┤
│           Axiom Web UI (SPA)              │             Axiom TUI (Bubbletea)          │
│       (Browser local en :port)            │             (Terminal interactivo)         │
│                                           │                                            │
│  [Tablero SDD & Nuevo Incremento]         │  [Iniciar instalación / Modelos / ...]     │
│  [Monitor Multi-Rol & Barrera Fan-In]     │  [📁 Proyectos y Gobernanza SDD ➔]         │
│  [Visor de Handoffs Estructurados]        │     ├─ Gestor de Proyectos del Hub         │
│  [Catálogo de Especificaciones Vivas]     │     ├─ Ciclo de Vida de Incrementos SDD    │
│  [⚡ Catálogo de Skills / Autoskills]      │     ├─ Monitor Multi-Rol & Barrera Fan-In  │
│  [⚙️ Ecosistema & Herramientas (NUEVO)]   │     ├─ Visor de Handoffs Estructurados     │
│     ├─ Diagnóstico del Sistema (Doctor)   │     └─ Especificaciones Vivas e INDEX.md   │
│     ├─ Acciones Sync / Upgrade            │                                            │
│     ├─ Gestor de Respaldos (Backups)      │                                            │
│     └─ Configuración de Modelos de IA     │                                            │
└─────────────────────┬─────────────────────┴──────────────────────┬─────────────────────┘
                      │                                            │
                      ▼                                            ▼
┌───────────────────────────────────────────┐ ┌──────────────────────────────────────────┐
│   HTTP API Server (internal/dashboard)    │ │        TUI Model & Router (Bubbletea)    │
│   Rutas /api/ecosystem/*                  │ │        ScreenGovernance, ScreenHub, ...  │
└─────────────────────┬─────────────────────┘ └────────────────────┬─────────────────────┘
                      │                                            │
                      └──────────────────────┬─────────────────────┘
                                             ▼
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                           Capa de Dominio Unificada (internal/)                        │
│                                                                                        │
│   internal/hub        ➔ Proyectos globales, registro en workspaces.json y axiom init   │
│   internal/multirole  ➔ Asignaciones de roles, compuertas y barrera Fan-In             │
│   internal/handoff    ➔ Parser, validador y formateador de handoff.md                  │
│   internal/livingdoc  ➔ Indexador de specs, sincronización y visualización de catálogo  │
│   internal/backup     ➔ Creación, listado y restauración de respaldos en ~/.axiom/     │
│   internal/system     ➔ Detección de agentes instalados, runtime y entorno             │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Decisiones de Arquitectura y Diseño

### D-1: Submenú Modular de Gobernanza en la TUI
- **Decisión:** Para evitar saturar verticalmente el menú principal de la TUI (`WelcomeOptions`), que ya posee 14 elementos, se agrupan las capacidades de gobernanza bajo la opción `📁 Proyectos y Gobernanza SDD ➔`.
- **Estructura del Submenú:**
  1. `1. Proyectos del Hub (conmutar, registrar, inicializar)`
  2. `2. Ciclo de Vida de Incrementos SDD (ver fases, crear, avanzar)`
  3. `3. Monitor Multi-Rol y Barrera Fan-In (inspeccionar, migrar diferidas)`
  4. `4. Visor de Handoffs Estructurados (relevos formales)`
  5. `5. Catálogo de Especificaciones Vivas (specs consolidadas)`
  6. `Volver al menú principal`
- **Razón:** Mantiene la TUI accesible y usable en terminales de 24 líneas estándar y permite una navegación ordenada con `Enter` y `Esc`.

### D-2: Rutas y DTOs para el Ecosistema en la Web UI
- **Decisión:** Crear un conjunto unificado de endpoints bajo el prefijo `/api/ecosystem/` en `internal/dashboard/server.go`:
  - `GET /api/ecosystem/doctor`: Diagnóstico consolidado de salud.
  - `POST /api/ecosystem/sync`: Disparo de sincronización.
  - `POST /api/ecosystem/upgrade`: Disparo de actualización.
  - `GET /api/ecosystem/backups`: Listado de respaldos.
  - `POST /api/ecosystem/backups/create`: Creación de respaldo con snapshot.
  - `POST /api/ecosystem/backups/restore`: Restauración de snapshot.
  - `GET /api/ecosystem/models`: Configuración de modelos asignados.
- **Razón:** Separa las operaciones de mantenimiento del ecosistema de las rutas de gobernanza de incrementos (`/api/increments`), logrando alta cohesión y claridad de API.

### D-3: Reutilización de Paquetes de Dominio Existentes (Cero Duplicación)
- **Decisión:** Tanto el servidor HTTP como las pantallas de la TUI invocan directamente las funciones puras de Go en `internal/hub`, `internal/backup`, `internal/multirole`, `internal/handoff`, `internal/livingdoc` y `internal/system`.
- **Razón:** Elimina la divergencia de comportamiento. La misma regla de barrera Fan-In que valida el Dashboard Web es la que ejecuta la TUI en el terminal.

---

## 3. Especificación de Endpoints y Modelos DTO

En `internal/dashboard/types.go`:

```go
// DoctorCheck representa un resultado de diagnóstico individual.
type DoctorCheck struct {
	Name           string `json:"name"`
	Category       string `json:"category"` // "agent", "tool", "environment"
	Status         string `json:"status"`   // "ok", "warning", "error"
	Details        string `json:"details"`
	Recommendation string `json:"recommendation,omitempty"`
}

// DoctorReport agrupa los diagnósticos de salud del ecosistema.
type DoctorReport struct {
	Timestamp string        `json:"timestamp"`
	Healthy   bool          `json:"healthy"`
	Checks    []DoctorCheck `json:"checks"`
}

// BackupItem representa un respaldo para la API web.
type BackupItem struct {
	Name        string   `json:"name"`
	Created     string   `json:"created"`
	Description string   `json:"description"`
	Pinned      bool     `json:"pinned"`
	Files       []string `json:"files,omitempty"`
}

// BackupActionRequest define la solicitud para crear o restaurar un respaldo.
type BackupActionRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// EcosystemActionResponse reporta el resultado de una operación de sincronización o actualización.
type EcosystemActionResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message"`
	Output  []string `json:"output,omitempty"`
}
```

---

## 4. Diseño de Pantallas y Máquina de Estados TUI

### Pantalla `ScreenGovernanceMenu`:
- Renderiza el menú de 5 áreas de gobernanza más la opción de salida/retorno.
- Teclas: `j`/`k`/flechas para navegar, `Enter` para acceder, `Esc`/`q` para volver a `ScreenWelcome`.

### Pantalla `ScreenHubProjects`:
- Carga `hub.NewManager(home).ListWorkspaces()`.
- Muestra el proyecto activo marcado con `(activo)` y en verde.
- Teclas: `Enter` para conmutar activo, `a` para añadir ruta, `i` para inicializar con `axiom init`, `Esc` para volver.

### Pantalla `ScreenSDDIncrements`:
- Carga incrementos activos y archivados.
- Muestra chips de fase actual: `[PROPOSAL]`, `[SPEC]`, `[DESIGN]`, `[TASKS]`, `[APPLY]`, `[VERIFY]`, `[ARCHIVE]`.
- Teclas: `n` para nuevo incremento, `c` para avanzar fase (`continue`), `v` para validar verificación, `Esc` para volver.

### Pantalla `ScreenMultiRole`:
- Selector de cambio activo. Muestra tabla de roles:
  - Rol ID, Tipo (`blocking`/`deferred`), Progreso de tareas, Estado de barrera.
  - Teclas: `m` para migrar tareas diferidas acumulativas, `Esc` para volver.

### Pantalla `ScreenHandoffs`:
- Carga y parsea el último `handoff.md`.
- Renderiza las secciones del relevo formal con formato estilizado Lipgloss.
- Teclas: `Esc` para volver.

### Pantalla `ScreenLivingDoc`:
- Carga el índice consolidado `openspec/INDEX.md` o listado de specs.
- Teclas: `s` para forzar sincronización del catálogo, `Esc` para volver.
