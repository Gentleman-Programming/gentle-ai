# Documento de Diseño Técnico: Despliegue Multi-Rol y Barrera de Sincronización (INC-03)

## Resumen del Diseño

Este documento especifica la arquitectura de software en Go, modelos de dominio, algoritmos de detección de responsabilidades con políticas de compuerta (`gate_policy`), cómputo de tareas por rol (`tasks.<rol>.md`), el evaluador determinista de la barrera de sincronización en `Archive` y el mecanismo de migración de tareas diferidas (ej. E2E) con trazabilidad al incremento de origen.

---

## 1. Arquitectura de Componentes

```text
axiom/
├── cmd/
│   └── axiom/
│       └── main.go                               # Ampliado con comandos 'axiom role [list|status|barrier]'
├── internal/
│   └── multirole/
│       ├── types.go                              # Tipos de roles, políticas, tareas diferidas y barrera
│       ├── detector.go                           # Extracción de roles desde design.md y validación
│       ├── barrier.go                            # Evaluador de barrera y extractor de tareas diferidas
│       └── multirole_test.go                     # Batería de pruebas unitarias exhaustiva
└── openspec/
    └── changes/
        └── inc-03-multi-role-sdd-fan-out/        # Artefactos SDD del incremento
```

---

## Roles Participantes del Cambio

```yaml
roles:
  - role: core
    name: "Axiom Core Engine"
    gate_policy: blocking
    repositories: ["."]
    deliverables: ["internal/multirole", "cmd/axiom"]
  - role: qa
    name: "Quality Assurance & Verification"
    gate_policy: blocking
    repositories: ["."]
    deliverables: ["internal/multirole/multirole_test.go", "verify-report.md"]
  - role: e2e
    name: "E2E & Automation Testing"
    gate_policy: deferred
    repositories: ["."]
    deliverables: ["e2e/multirole_workflow_test.go"]
```

---

## 2. Modelos de Dominio (`internal/multirole/types.go`)

```go
package multirole

// GatePolicy define el comportamiento de un rol frente a la barrera de sincronización.
type GatePolicy string

const (
    PolicyBlocking GatePolicy = "blocking" // Obligatorio: debe completar tareas y verificar pass
    PolicyDeferred GatePolicy = "deferred" // Asíncrono: no bloquea Archive, se acumulan sus tareas
    PolicyOptional GatePolicy = "optional" // Opcional: no vinculante
)

// RoleAssignment representa la asignación formal de un rol a un cambio SDD.
type RoleAssignment struct {
    Role         string     `yaml:"role"`
    Name         string     `yaml:"name,omitempty"`
    GatePolicy   GatePolicy `yaml:"gate_policy"`
    Repositories []string   `yaml:"repositories,omitempty"`
    Deliverables []string   `yaml:"deliverables,omitempty"`
}

// RoleTaskProgress contiene el cómputo de tareas de un rol.
type RoleTaskProgress struct {
    Total     int
    Completed int
    Pending   int
    Percent   float64
}

// RoleExecutionStatus consolida el estado del ciclo de vida de un rol en el cambio.
type RoleExecutionStatus struct {
    Assignment RoleAssignment
    TasksFound bool
    Tasks      RoleTaskProgress
    ApplyDone  bool
    VerifyDone bool
    Verdict    string // "pass", "fail", "missing"
    Compliant  bool
}

// DeferredTask modela una tarea no concluida de un rol diferido para su migración trazable.
type DeferredTask struct {
    Change       string
    Role         string
    TaskText     string
    SpecRef      string
    ChangeDirRef string
}

// BarrierReport consolida la evaluación de la barrera de sincronización en Archive.
type BarrierReport struct {
    Satisfied     bool
    Change        string
    Roles         []RoleExecutionStatus
    Blockers      []string
    Warnings      []string
    DeferredTasks []DeferredTask
}
```

---

## 3. Extracción y Detección de Roles (`internal/multirole/detector.go`)

### Contrato
```go
func DetectRoles(designPath string, wsConfig *workspace.WorkspaceConfig) ([]RoleAssignment, error)
func ParseRolesMarkdown(content string) ([]RoleAssignment, error)
```

### Comportamiento:
1. **Detección Explícita:**
   - Busca en `design.md` el bloque YAML de roles dentro de `## Roles Participantes` o bloque `roles:`.
   - Si no se especifica `gate_policy` en un rol, se asume por defecto `blocking`.
   - Ejemplo en `design.md`:
     ```yaml
     roles:
       - role: backend
         gate_policy: blocking
         repositories: ["api"]
       - role: frontend
         gate_policy: blocking
         repositories: ["web"]
       - role: e2e
         gate_policy: deferred
         repositories: ["tests/e2e"]
     ```
2. **Validación con Topología:**
   - Valida contra `axiom.yaml` asegurando que los roles declarados existan en el espacio de trabajo.
3. **Modo Retrocompatible (Fallback):**
   - Si no se detectan roles explícitos, genera automáticamente una asignación única con el rol principal del workspace con `gate_policy: blocking`.

---

## 4. Evaluador de Barrera y Migración Diferida (`internal/multirole/barrier.go`)

### Contrato
```go
func EvaluateBarrier(changeDir, changeName string, roles []RoleAssignment) (*BarrierReport, error)
func CountTasks(tasksContent string) RoleTaskProgress
func ExtractPendingTasks(changeName, role, specRef, changeDirRef, tasksContent string) []DeferredTask
func FormatDeferredTaskMarkdown(dt DeferredTask) string
```

### Reglas de Evaluación:
1. **Evaluación de Roles `blocking`:**
   - Deben tener `tasks.<rol>.md` con 100% tareas completas (`Pending == 0 && Total > 0`).
   - Deben tener `verify-report.<rol>.md` con `verdict: pass`.
   - Si no se cumple, se agrega a `Blockers` y `Satisfied = false`.
2. **Evaluación de Roles `deferred`:**
   - Si tienen tareas pendientes, NO se agregan a `Blockers` (no impiden `Satisfied = true`).
   - Se genera un `Warning` descriptivo.
   - Las tareas incompletas se extraen como `DeferredTask`, vinculando el cambio de origen `[Ref: <cambio>]`.
3. **Evaluación de Roles `optional`:**
   - No bloquean la barrera bajo ninguna circunstancia.

---

## 5. Extensión de la CLI (`cmd/axiom/main.go`)

Se amplía el grupo `axiom role`:
```bash
# Listar roles participantes asignados en design.md con su gate_policy
axiom role list [--change <nombre>] [--path <directorio>]

# Ver estado de avance por rol (tareas, apply, verify)
axiom role status [--change <nombre>] [--path <directorio>]

# Evaluar barrera de sincronización antes de archivar / abrir PR
axiom role barrier [--change <nombre>] [--path <directorio>] [--migrate-deferred]
```

La bandera `--migrate-deferred` opcionalmente vuelca las tareas pendientes diferidas en el archivo acumulativo `openspec/changes/e2e-cumulative/tasks.md`.
