# Diseño de Arquitectura: Orquestación Interactiva SDD y Creación de Cambios en Dashboard Web y CLI (INC-15)

## 1. Visión General y Diagrama de Arquitectura

El objetivo de este incremento es dotar al entorno de usuario (tanto en la interfaz web local como en la terminal) de la capacidad de operar activamente sobre el ciclo de vida Spec-Driven Development (SDD):

```
┌────────────────────────────────────────────────────────────────────────┐
│                          Axiom Interfaces                              │
├──────────────────────────────────┬─────────────────────────────────────┤
│      Dashboard Web SPA           │           CLI canónica              │
│    (Navegador / HTTP API)        │          (cmd/axiom/main)           │
│                                  │                                     │
│  [+ Nuevo Incremento]            │  axiom change create <nombre>       │
│  [▶ Continuar SDD (continue)]    │  axiom sdd continue <nombre>        │
│  [✓ Validar Verificación]        │  axiom sdd verify-validate          │
│  [+ Crear Handoff]               │  axiom handoff create               │
└─────────────────┬────────────────┴──────────────────┬──────────────────┘
                  │                                   │
                  ▼                                   ▼
┌────────────────────────────────────────────────────────────────────────┐
│                   Servidor HTTP Local (internal/dashboard)             │
│                                                                        │
│  POST /api/increments          POST /api/increments/continue           │
│  POST /api/increments/verify   POST /api/handoffs                      │
└─────────────────────────────────┬──────────────────────────────────────┘
                                  │
                                  ▼
┌────────────────────────────────────────────────────────────────────────┐
│                   Capa de Servicio (Service)                           │
│                                                                        │
│  - CreateIncrement(workspace, req)  ➔ Genera openspec/changes/<name>/  │
│  - ContinueIncrement(workspace, name) ➔ Invoca cli.RunSDDContinue      │
│  - VerifyIncrement(workspace, name)   ➔ Evalúa reporte formal          │
│  - CreateHandoff(workspace, req)    ➔ Invoca handoff.WriteFile         │
└─────────────────────────────────┬──────────────────────────────────────┘
                                  │
                                  ▼
┌────────────────────────────────────────────────────────────────────────┐
│                  Sistema de Ficheros & Especificaciones                │
│                                                                        │
│  openspec/changes/<name>/proposal.md                                   │
│  openspec/changes/<name>/spec.md                                       │
│  openspec/changes/<name>/design.md                                     │
│  openspec/changes/<name>/tasks.md                                      │
│  openspec/changes/<name>/handoff.md                                    │
│  openspec/changes/<name>/verify-report.md                              │
└────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Decisiones de Diseño

### D-1: Generación Canónica de Propuestas en Español
- **Decisión:** Al crear un cambio mediante la API web o `axiom change create`, el sistema generará automáticamente una plantilla de `proposal.md` estrictamente en español (castellano), con el título capitalizado, el propósito provisto por el usuario, secciones canónicas de alcance (`Dentro de Alcance`, `Fuera de Alcance`), capacidades y enfoque.
- **Razón:** Garantizar el cumplimiento del contrato de idioma de Axiom desde el primer segundo de vida del incremento, evitando plantillas en inglés o estructuras ad-hoc.

### D-2: Reutilización Directa de Motores Existentes sin Subprocesos
- **Decisión:** Para ejecutar `continue` y `verify`, `internal/dashboard/service.go` no ejecuta el binario mediante comandos de sistema (`exec.Command`), sino que invoca directamente las funciones de `internal/cli` (`cli.RunSDDContinue` y lógica de verificación) redirigiendo la salida a un `bytes.Buffer`.
- **Razón:** Rendimiento óptimo, portabilidad homogénea entre Windows, macOS y Linux, y captura determinista de salidas JSON o Markdown sin lidiar con bloqueos de procesos en segundo plano.

### D-3: Composición de Handoffs con Validación Estricta de INC-02
- **Decisión:** La creación de handoffs desde la Web UI utiliza el paquete `internal/handoff`:
  1. Parsea y valida las fases (`explore`, `propose`, `spec`, `design`, `tasks`, `apply`, `verify`, `archive`).
  2. Valida la transición semántica permitida.
  3. Ensambla el Frontmatter YAML y las 5 secciones obligatorias mediante `handoff.Format` y `handoff.WriteFile`.
- **Razón:** Asegura que cualquier handoff generado visualmente mantenga paridad absoluta con los contratos canónicos de Axiom y Engram.

### D-4: Rutas REST Explícitas y Compatibles con `http.ServeMux`
- **Decisión:** Utilizar rutas POST explícitas:
  - `POST /api/increments` (creación de incremento)
  - `POST /api/increments/continue` (cuerpo: `{ "name": "<nombre>" }`)
  - `POST /api/increments/verify` (cuerpo: `{ "name": "<nombre>" }`)
  - `POST /api/handoffs` (cuerpo completo del handoff)
- **Razón:** Evita colisiones de prefijo en el enrutador estándar de Go con `s.mux.HandleFunc("/api/increments/", ...)` y simplifica las llamadas AJAX del frontend.

---

## 3. Especificación de Modelos de Datos (DTOs)

En `internal/dashboard/types.go`:

```go
// CreateIncrementRequest define los parámetros para crear un nuevo incremento SDD.
type CreateIncrementRequest struct {
	Name   string `json:"name"`
	Intent string `json:"intent"`
	Type   string `json:"type"` // "feature", "fix", "refactor", "architecture"
}

// CreateIncrementResponse reporta el resultado de la creación.
type CreateIncrementResponse struct {
	Success bool   `json:"success"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// IncrementActionRequest define la acción a ejecutar sobre un incremento.
type IncrementActionRequest struct {
	Name string `json:"name"`
}

// IncrementActionResponse reporta el resultado de una acción SDD.
type IncrementActionResponse struct {
	Success    bool   `json:"success"`
	ChangeName string `json:"change_name"`
	Action     string `json:"action"`
	Output     string `json:"output"`
	Error      string `json:"error,omitempty"`
}

// CreateHandoffRequest define los campos enviados desde la Web UI para un handoff.
type CreateHandoffRequest struct {
	Change           string `json:"change"`
	FromPhase        string `json:"from_phase"`
	ToPhase          string `json:"to_phase"`
	FromRole         string `json:"from_role"`
	ToRole           string `json:"to_role"`
	Status           string `json:"status"`
	ExecutiveSummary string `json:"executive_summary"`
	Artifacts        string `json:"artifacts"`
	Decisions        string `json:"decisions"`
	Risks            string `json:"risks"`
	Instructions     string `json:"instructions"`
}
```

---

## 4. Experiencia de Usuario en la Web UI (`index.html`, `app.js`, `style.css`)

1. **Tablero de Incrementos (`tab-increments`):**
   - Encabezado con botón primario `+ Nuevo Incremento`.
   - Modal flotante `#modal-new-increment`:
     - Input de texto con validación en vivo para `kebab-case`.
     - Área de texto para propósito/intención.
     - Selector de tipo de cambio.
     - Botón `Crear Incremento` que invoca `POST /api/increments`, cierra el modal y recarga el tablero.

2. **Modal de Detalle del Incremento (`#inc-modal`):**
   - Barra de herramientas con botones de acción rápida:
     - `▶ Continuar SDD`: ejecuta `POST /api/increments/continue` y muestra el resultado en un panel colapsable de consola.
     - `✓ Validar Verificación`: evalúa la verificación formal y muestra si el veredicto es PASS.
     - `🤝 Redactar Handoff`: abre el modal de handoff preseleccionando el cambio.

3. **Visor de Handoffs (`tab-handoffs`):**
   - Botón `+ Crear Handoff`.
   - Modal `#modal-create-handoff`:
     - Selectores de fase origen y destino (`propose`, `spec`, `design`, `apply`, etc.).
     - Roles origen y destino.
     - Entradas de texto para las 5 secciones requeridas.
     - Al enviar, guarda atómicamente el archivo `handoff.md` y refresca el visor.
