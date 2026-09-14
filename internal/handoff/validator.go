package handoff

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/workspace"
)

var validPhases = map[Phase]bool{
	PhaseExplore: true,
	PhasePropose: true,
	PhaseSpec:    true,
	PhaseDesign:  true,
	PhaseTasks:   true,
	PhaseApply:   true,
	PhaseVerify:  true,
	PhaseArchive: true,
}

var validStatuses = map[Status]bool{
	StatusReady:              true,
	StatusBlocked:            true,
	StatusNeedsClarification: true,
}

// forwardTransitions define la secuencia natural de avance en el ciclo SDD.
var forwardTransitions = map[Phase]Phase{
	PhaseExplore: PhasePropose,
	PhasePropose: PhaseSpec,
	PhaseSpec:    PhaseDesign,
	PhaseDesign:  PhaseTasks,
	PhaseTasks:   PhaseApply,
	PhaseApply:   PhaseVerify,
	PhaseVerify:  PhaseArchive,
}

// remediationTransitions define retrocesos controlados permitidos exclusivamente por bloqueo o aclaración.
var remediationTransitions = map[Phase][]Phase{
	PhaseVerify: {PhaseApply, PhaseTasks},
	PhaseApply:  {PhaseDesign},
}

// Validate comprueba la completitud, consistencia de transición y conformidad con el espacio de trabajo.
func Validate(h *Handoff, wsConfig *workspace.WorkspaceConfig) error {
	if h == nil {
		return fmt.Errorf("el handoff a validar es nulo")
	}

	// 1. Validación de campos obligatorios en metadatos
	if strings.TrimSpace(h.Metadata.Change) == "" {
		return fmt.Errorf("el campo 'change' en los metadatos es obligatorio")
	}
	if strings.TrimSpace(string(h.Metadata.FromPhase)) == "" {
		return fmt.Errorf("el campo 'from_phase' en los metadatos es obligatorio")
	}
	if strings.TrimSpace(string(h.Metadata.ToPhase)) == "" {
		return fmt.Errorf("el campo 'to_phase' en los metadatos es obligatorio")
	}
	if strings.TrimSpace(h.Metadata.FromRole) == "" {
		return fmt.Errorf("el campo 'from_role' en los metadatos es obligatorio")
	}
	if strings.TrimSpace(h.Metadata.ToRole) == "" {
		return fmt.Errorf("el campo 'to_role' en los metadatos es obligatorio")
	}
	if !validStatuses[h.Metadata.Status] {
		return fmt.Errorf("estado de relevo 'status' inválido: %q (admitidos: ready, blocked, needs_clarification)", h.Metadata.Status)
	}
	if h.Metadata.Timestamp.IsZero() {
		return fmt.Errorf("el campo 'timestamp' en los metadatos no puede ser nulo o vacío")
	}

	// 2. Validación de validez de fases
	if !validPhases[h.Metadata.FromPhase] {
		return fmt.Errorf("fase emisora 'from_phase' desconocida: %q", h.Metadata.FromPhase)
	}
	if !validPhases[h.Metadata.ToPhase] {
		return fmt.Errorf("fase receptora 'to_phase' desconocida: %q", h.Metadata.ToPhase)
	}

	// 3. Validación de las cinco secciones obligatorias
	if strings.TrimSpace(h.Sections.ExecutiveSummary) == "" {
		return fmt.Errorf("la sección '1. Resumen Ejecutivo' no puede estar vacía")
	}
	if strings.TrimSpace(h.Sections.Artifacts) == "" {
		return fmt.Errorf("la sección '2. Artefactos Modificados y Creados' no puede estar vacía")
	}
	if strings.TrimSpace(h.Sections.Decisions) == "" {
		return fmt.Errorf("la sección '3. Decisiones Técnicas y Acuerdos' no puede estar vacía")
	}
	if strings.TrimSpace(h.Sections.RisksAndBlockers) == "" {
		return fmt.Errorf("la sección '4. Riesgos, Bloqueos y Preguntas Abiertas' no puede estar vacía")
	}
	if strings.TrimSpace(h.Sections.DirectInstructions) == "" {
		return fmt.Errorf("la sección '5. Instrucciones Directas para el Siguiente Rol' no puede estar vacía")
	}

	// 4. Validación de transición entre fases
	from := h.Metadata.FromPhase
	to := h.Metadata.ToPhase

	// Avance directo normal
	expectedNext, hasForward := forwardTransitions[from]
	if hasForward && expectedNext == to {
		// Transición hacia adelante válida
		return validateRoles(h, wsConfig)
	}

	// Verificación de retroceso por remediación
	allowedRemediations, isRemediationFrom := remediationTransitions[from]
	if isRemediationFrom {
		for _, target := range allowedRemediations {
			if target == to {
				if h.Metadata.Status == StatusReady {
					return fmt.Errorf("transición de remediación hacia atrás (%s -> %s) no permitida con estado 'ready'; requiere estado 'blocked' o 'needs_clarification'", from, to)
				}
				return validateRoles(h, wsConfig)
			}
		}
	}

	return fmt.Errorf("transición de fase ilegal (%s -> %s): no se permite saltar fases en el ciclo SDD ni retrocesos no autorizados", from, to)
}

func validateRoles(h *Handoff, wsConfig *workspace.WorkspaceConfig) error {
	if wsConfig == nil || len(wsConfig.Roles) == 0 {
		return nil
	}

	if !roleExists(wsConfig, h.Metadata.FromRole) {
		return fmt.Errorf("el rol emisor %q no está declarado en la configuración de roles de axiom.yaml", h.Metadata.FromRole)
	}
	if !roleExists(wsConfig, h.Metadata.ToRole) {
		return fmt.Errorf("el rol receptor %q no está declarado en la configuración de roles de axiom.yaml", h.Metadata.ToRole)
	}

	return nil
}

func roleExists(cfg *workspace.WorkspaceConfig, role string) bool {
	for k, v := range cfg.Roles {
		if strings.EqualFold(k, role) || strings.EqualFold(v.Name, role) {
			return true
		}
	}
	return false
}
