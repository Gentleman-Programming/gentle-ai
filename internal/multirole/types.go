package multirole

import "strings"

// RoleFullstack is the reserved role identity the kickoff assigns when a
// change is not subdivided into specialized roles (REQ-21.6). It is valid
// in every workspace, declared in axiom.yaml or not, because REQ-21.6
// requires every SDD change to carry at least one informed role and a
// workspace cannot be excluded from that guarantee just because its
// axiom.yaml was never edited to list it (design.md D-07).
const RoleFullstack = "fullstack"

// IsReservedRole reports whether role names an identity the engine
// recognises on its own, independent of any workspace's declared roles
// (currently only RoleFullstack). It is the single, shared predicate both
// copies of roleExists (this package's detector.go and
// internal/handoff/validator.go) consult, so the reserved set can never
// silently diverge between the two (design.md D-07).
func IsReservedRole(role string) bool {
	return strings.EqualFold(role, RoleFullstack)
}

// GatePolicy define la política de compuerta y el nivel de exigencia de un rol frente a la barrera de sincronización.
type GatePolicy string

const (
	// PolicyBlocking exige que el rol complete el 100% de tareas y obtenga verificación PASS para autorizar el cierre.
	PolicyBlocking GatePolicy = "blocking"

	// PolicyDeferred permite que el rol continúe de forma asíncrona (ej. E2E); no bloquea la entrega principal y sus tareas pendientes se migran.
	PolicyDeferred GatePolicy = "deferred"

	// PolicyOptional representa tareas accesorias no vinculantes que no condicionan la barrera.
	PolicyOptional GatePolicy = "optional"
)

// RoleAssignment representa la asignación formal de un rol en un cambio SDD.
type RoleAssignment struct {
	Role         string     `yaml:"role"`
	Name         string     `yaml:"name,omitempty"`
	GatePolicy   GatePolicy `yaml:"gate_policy"`
	Repositories []string   `yaml:"repositories,omitempty"`
	Deliverables []string   `yaml:"deliverables,omitempty"`
}

// RoleTaskProgress almacena el desglose numérico y porcentual de tareas de un rol.
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

// DeferredTask representa una tarea no concluida de un rol diferido con su trazabilidad de origen.
type DeferredTask struct {
	Change       string
	Role         string
	TaskText     string
	SpecRef      string
	ChangeDirRef string
}

// BarrierReport consolida la evaluación determinista de la barrera de sincronización en Archive / Staging.
type BarrierReport struct {
	Satisfied     bool
	Change        string
	Roles         []RoleExecutionStatus
	Blockers      []string
	Warnings      []string
	DeferredTasks []DeferredTask
}
