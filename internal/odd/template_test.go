package odd

import (
	"strings"
	"testing"
)

// TestRenderNew cubre la tarea 1.5: las doce secciones canónicas deben
// aparecer, en orden, en el texto generado; dos invocaciones con la misma
// entrada deben producir bytes idénticos; el resultado debe terminar en
// salto de línea. Los títulos esperados se fijan aquí de forma literal
// (REQ-19.1), independientes del mapa interno de producción, para que el
// test siga siendo un oráculo real y no un espejo de la implementación.
func TestRenderNew(t *testing.T) {
	wantTitles := map[SectionID]string{
		SectionObjective:       "Objetivo",
		SectionProblem:         "Problema",
		SectionWhy:             "Porqué",
		SectionScope:           "Alcance",
		SectionConstraints:     "Restricciones",
		SectionAuthorizedScope: "Alcance autorizado",
		SectionChecklist:       "Checklist accionable",
		SectionAcceptance:      "Criterios de aceptación",
		SectionChecks:          "Comprobaciones aplicables",
		SectionProgress:        "Progreso",
		SectionEvidence:        "Evidencia de verificación",
		SectionNextStep:        "Siguiente paso",
	}

	if len(CanonicalSections) != 12 {
		t.Fatalf("CanonicalSections tiene %d elementos, se esperaban 12", len(CanonicalSections))
	}

	tests := []struct {
		name    string
		feature string
		today   string
	}{
		{name: "feature de varios bloques", feature: "gestion-inventario", today: "2026-09-17"},
		{name: "feature de un único bloque", feature: "demo", today: "2026-01-01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderNew(tt.feature, tt.today)

			if got == "" {
				t.Fatalf("RenderNew(%q, %q) devolvió una cadena vacía", tt.feature, tt.today)
			}
			if !strings.HasSuffix(got, "\n") {
				t.Errorf("RenderNew(%q, %q) no termina en salto de línea", tt.feature, tt.today)
			}

			lastIndex := -1
			for _, id := range CanonicalSections {
				title, ok := wantTitles[id]
				if !ok {
					t.Fatalf("no hay título esperado en la tabla del test para la sección %q", id)
				}
				heading := "## " + title
				idx := strings.Index(got, heading)
				if idx == -1 {
					t.Fatalf("RenderNew(%q, %q) no contiene la sección %q", tt.feature, tt.today, heading)
				}
				if idx <= lastIndex {
					t.Errorf("la sección %q aparece fuera de orden respecto a la sección canónica anterior", heading)
				}
				lastIndex = idx
			}
		})
	}

	t.Run("determinismo: misma entrada produce bytes idénticos", func(t *testing.T) {
		first := RenderNew("gestion-inventario", "2026-09-17")
		second := RenderNew("gestion-inventario", "2026-09-17")

		if first != second {
			t.Errorf("RenderNew no es determinista: primera llamada %q, segunda llamada %q", first, second)
		}
	})
}
