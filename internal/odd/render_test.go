package odd

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/multirole"
)

// TestRenderStatusText cubre la salida en texto legible de "axiom odd
// status" (REQ-19.6) para 0, 1 y N documentos vivos: la línea de referencia
// de promoción cuando el documento ya fue promovido (REQ-19.12), y la línea
// de estado del espejo de recuperación en Engram cuando
// FeatureSummary.Mirror está poblado (REQ-19.7).
func TestRenderStatusText(t *testing.T) {
	tests := []struct {
		name   string
		report StatusReport
		want   string
	}{
		{
			name:   "sin documentos vivos",
			report: StatusReport{},
			want:   "No se encontraron documentos vivos ODD en odd/tasks/.\n",
		},
		{
			name: "un documento activo sin promoción ni espejo",
			report: StatusReport{
				Features: []FeatureSummary{
					{
						Feature:  "gestion-inventario",
						Status:   StatusActive,
						Progress: multirole.RoleTaskProgress{Total: 5, Completed: 2, Pending: 3, Percent: 40},
					},
				},
			},
			want: "gestion-inventario [activo] — 2/5 tareas completadas (40%)\n",
		},
		{
			name: "documento promovido incluye la referencia de cambio",
			report: StatusReport{
				Features: []FeatureSummary{
					{
						Feature:    "gestion-inventario",
						Status:     StatusPromoted,
						PromotedTo: "openspec/changes/gestion-inventario/",
						Progress:   multirole.RoleTaskProgress{Total: 3, Completed: 3, Percent: 100},
					},
				},
			},
			want: "gestion-inventario [promovido] — 3/3 tareas completadas (100%)\n" +
				"  Promovido a: openspec/changes/gestion-inventario/\n",
		},
		{
			name: "documento con espejo sincronizado añade la línea de estado del espejo",
			report: StatusReport{
				Features: []FeatureSummary{
					{
						Feature:  "gestion-inventario",
						Status:   StatusActive,
						Progress: multirole.RoleTaskProgress{Total: 4, Completed: 2, Pending: 2, Percent: 50},
						Mirror:   &MirrorReport{State: MirrorSynced, Topic: "odd/gestion-inventario/tasks"},
					},
				},
			},
			want: "gestion-inventario [activo] — 2/4 tareas completadas (50%)\n" +
				"  Espejo: sincronizado\n",
		},
		{
			name: "documento con espejo no disponible incluye el motivo",
			report: StatusReport{
				Features: []FeatureSummary{
					{
						Feature:  "gestion-inventario",
						Status:   StatusActive,
						Progress: multirole.RoleTaskProgress{Total: 1, Completed: 0, Pending: 1},
						Mirror: &MirrorReport{
							State:  MirrorUnavailable,
							Topic:  "odd/gestion-inventario/tasks",
							Reason: "no se pudo exportar el espejo Engram: fallo simulado",
						},
					},
				},
			},
			want: "gestion-inventario [activo] — 0/1 tareas completadas (0%)\n" +
				"  Espejo: no disponible (no se pudo exportar el espejo Engram: fallo simulado)\n",
		},
		{
			name: "N documentos combinan promoción y espejo con separador en blanco entre bloques",
			report: StatusReport{
				Features: []FeatureSummary{
					{
						Feature:  "alfa",
						Status:   StatusActive,
						Progress: multirole.RoleTaskProgress{Total: 2, Completed: 1, Pending: 1, Percent: 50},
					},
					{
						Feature:  "beta",
						Status:   StatusActive,
						Progress: multirole.RoleTaskProgress{Total: 4, Completed: 4, Percent: 100},
						Mirror:   &MirrorReport{State: MirrorDiverged, Topic: "odd/beta/tasks"},
					},
					{
						Feature:    "gamma",
						Status:     StatusPromoted,
						PromotedTo: "openspec/changes/gamma/",
						Progress:   multirole.RoleTaskProgress{},
					},
				},
			},
			want: "alfa [activo] — 1/2 tareas completadas (50%)\n" +
				"\n" +
				"beta [activo] — 4/4 tareas completadas (100%)\n" +
				"  Espejo: divergente\n" +
				"\n" +
				"gamma [promovido] — 0/0 tareas completadas (0%)\n" +
				"  Promovido a: openspec/changes/gamma/\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderStatusText(tt.report)
			if got != tt.want {
				t.Errorf("RenderStatusText() = %q, se esperaba %q", got, tt.want)
			}
		})
	}
}
