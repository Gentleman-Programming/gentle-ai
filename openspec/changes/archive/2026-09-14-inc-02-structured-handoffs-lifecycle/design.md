# Documento de Diseño Técnico: Handoffs Estructurados y Ciclo de Vida de Transición (INC-02)

## Resumen del Diseño

Este documento define la arquitectura técnica, las interfaces en Go, el esquema de serialización de `handoff.md` y los subcomandos de la CLI `axiom` en el runtime del producto Axiom (`internal/handoff` y `cmd/axiom`).

---

## 1. Arquitectura de Componentes

```text
axiom/
├── cmd/
│   └── axiom/
│       └── main.go                               # Despachador de CLI (ampliado con 'axiom handoff')
├── internal/
│   └── handoff/
│       ├── types.go                              # Modelos de datos, estados de handoff y fases
│       ├── parser.go                             # Deserializador de handoff.md (YAML + Markdown)
│       ├── writer.go                             # Generador y formateador canónico de handoff.md
│       ├── validator.go                          # Validación semántica y máquina de estados
│       ├── mirror.go                             # Adaptador de sincronización para Engram MCP
│       └── handoff_test.go                       # Batería exhaustiva de tests unitarios
└── openspec/
    └── changes/
        └── inc-02-structured-handoffs-lifecycle/ # Artefactos SDD del cambio
```

---

## 2. Modelo de Dominio (`internal/handoff/types.go`)

### Fases de SDD y Estados de Relevo

```go
package handoff

import "time"

type Phase string

const (
    PhaseExplore Phase = "explore"
    PhasePropose Phase = "propose"
    PhaseSpec    Phase = "spec"
    PhaseDesign  Phase = "design"
    PhaseTasks   Phase = "tasks"
    PhaseApply   Phase = "apply"
    PhaseVerify  Phase = "verify"
    PhaseArchive Phase = "archive"
)

type Status string

const (
    StatusReady              Status = "ready"
    StatusBlocked            Status = "blocked"
    StatusNeedsClarification Status = "needs_clarification"
)

// Metadata encapsula los campos obligatorios del Frontmatter YAML
type Metadata struct {
    Change    string    `yaml:"change"`
    FromPhase Phase     `yaml:"from_phase"`
    ToPhase   Phase     `yaml:"to_phase"`
    FromRole  string    `yaml:"from_role"`
    ToRole    string    `yaml:"to_role"`
    Timestamp time.Time `yaml:"timestamp"`
    Status    Status    `yaml:"status"`
}

// Sections contiene las cinco secciones canónicas de contenido markdown
type Sections struct {
    ExecutiveSummary    string
    Artifacts           string
    Decisions           string
    RisksAndBlockers    string
    DirectInstructions  string
}

// Handoff representa el documento de relevo completo
type Handoff struct {
    Metadata Metadata
    Sections Sections
}
```

---

## 3. Deserialización y Serialización (`parser.go` y `writer.go`)

### Contrato de Parser
```go
func Parse(r io.Reader) (*Handoff, error)
func ParseFile(filePath string) (*Handoff, error)
```
- Lee el bloque delimitado por `---` al inicio para deserializar `Metadata` con `gopkg.in/yaml.v3`.
- Escanea el cuerpo restante buscando los encabezados exactos de las secciones canónicas:
  1. `## 1. Resumen Ejecutivo`
  2. `## 2. Artefactos Modificados y Creados`
  3. `## 3. Decisiones Técnicas y Acuerdos`
  4. `## 4. Riesgos, Bloqueos y Preguntas Abiertas`
  5. `## 5. Instrucciones Directas para el Siguiente Rol`
- Retorna error descriptivo en español si el frontmatter está corrupto o falta alguna de las 5 secciones.

### Contrato de Writer
```go
func Format(h *Handoff) (string, error)
func WriteFile(filePath string, h *Handoff) error
```
- Escribe el frontmatter YAML formateado con sangría consistente.
- Escribe las secciones en el orden canónico estandarizado.

---

## 4. Validador Semántico (`validator.go`)

```go
func Validate(h *Handoff, wsConfig *workspace.WorkspaceConfig) error
```

Reglas implementadas:
1. **Completitud:** Ningún campo de `Metadata` puede estar vacío. Ninguna de las cinco secciones puede ser una cadena en blanco.
2. **Máquina de Estados de Fases:**
   - Transiciones directas hacia adelante:
     - `explore` ➔ `propose`
     - `propose` ➔ `spec`
     - `spec` ➔ `design`
     - `design` ➔ `tasks`
     - `tasks` ➔ `apply`
     - `apply` ➔ `verify`
     - `verify` ➔ `archive`
   - Transiciones de remediación (solo permitidas si `Status != StatusReady`):
     - `verify` ➔ `apply` | `tasks`
     - `apply` ➔ `design`
   - Cualquier otra transición o salto arbitrario (ej. `propose` ➔ `apply`) es rechazada con un error de transición ilegal.
3. **Consistencia con Workspace (`axiom.yaml`):**
   - Si se provee `wsConfig`, se valida que `FromRole` y `ToRole` existan en `wsConfig.Roles`.

---

## 5. Espejo Persistente para Engram MCP (`mirror.go`)

```go
type EngramPayload struct {
    Title        string `json:"title"`
    TopicKey     string `json:"topic_key"`
    Type         string `json:"type"`
    Content      string `json:"content"`
    Project      string `json:"project,omitempty"`
    CapturePrompt bool  `json:"capture_prompt"`
}

func ToEngramPayload(h *Handoff) EngramPayload
```
Genera un payload optimizado para la herramienta `mem_save`, permitiendo que un agente en un nuevo contexto o sesión recupere de inmediato el último handoff mediante:
`mem_search(query: "sdd/{change}/handoff")` ➔ `mem_get_observation(id)`.

---

## 6. Interfaz CLI en `cmd/axiom/main.go`

Comandos soportados:
```bash
axiom handoff show [--change <nombre>] [--path <directorio>]
axiom handoff create --change <nombre> --from <fase> --to <fase> --from-role <rol> --to-role <rol> [--status <estado>] [--path <directorio>]
axiom handoff validate [--change <nombre>] [--path <directorio>]
```

Salidas legibles con códigos de salida estándar (`0` para éxito, `1` para error/no-conforme).

