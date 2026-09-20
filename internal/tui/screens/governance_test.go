package screens

import "testing"

// TestGovernanceOptionsEndsInBack verifica que GovernanceOptions() conserva
// sus 5 entradas numeradas seguidas de «Volver al menú principal» sin
// numerar, en su posición previa a INC-19 (F6.2c retira el carril ágil ODD
// que ocupaba temporalmente la posición 6).
func TestGovernanceOptionsEndsInBack(t *testing.T) {
	options := GovernanceOptions()

	const wantLen = 6
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
		{name: "volver en su posición previa a INC-19", index: 5, want: "Volver al menú principal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if options[tt.index] != tt.want {
				t.Errorf("GovernanceOptions()[%d] = %q, esperado %q", tt.index, options[tt.index], tt.want)
			}
		})
	}
}
