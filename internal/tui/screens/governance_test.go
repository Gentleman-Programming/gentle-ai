package screens

import "testing"

// TestGovernanceOptionsIncludesODDLaneBeforeBack verifica que
// GovernanceOptions() incluye la nueva entrada del carril ágil ODD
// («6. Carril Ágil ODD») inmediatamente antes de «Volver al menú
// principal», que queda desplazada a la posición 6 (REQ-19.14, REQ-19.15;
// design.md §4.5).
func TestGovernanceOptionsIncludesODDLaneBeforeBack(t *testing.T) {
	options := GovernanceOptions()

	const wantLen = 7
	if len(options) != wantLen {
		t.Fatalf("GovernanceOptions() longitud = %d, esperado %d: %v", len(options), wantLen, options)
	}

	tests := []struct {
		name  string
		index int
		want  string
	}{
		{name: "opción 1 sin cambios", index: 0, want: "1. Proyectos del Hub (conmutar, registrar, inicializar)"},
		{name: "opción 2 sin cambios", index: 1, want: "2. Ciclo de Vida de Incrementos SDD (ver fases, crear, avanzar)"},
		{name: "opción 3 sin cambios", index: 2, want: "3. Monitor Multi-Rol y Barrera Fan-In (inspeccionar, migrar diferidas)"},
		{name: "opción 4 sin cambios", index: 3, want: "4. Visor de Handoffs Estructurados (relevos formales)"},
		{name: "opción 5 sin cambios", index: 4, want: "5. Catálogo de Especificaciones Vivas (specs e INDEX.md)"},
		{name: "nueva opción 6: carril ágil ODD", index: 5, want: "6. Carril Ágil ODD (documentos vivos, promoción)"},
		{name: "volver desplazada a la posición 6", index: 6, want: "Volver al menú principal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if options[tt.index] != tt.want {
				t.Errorf("GovernanceOptions()[%d] = %q, esperado %q", tt.index, options[tt.index], tt.want)
			}
		})
	}
}
