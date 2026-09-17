package dashboard

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/odd"
)

// TestNameParity cubre la tarea 5.5 (D-10): todo nombre aceptado por
// odd.ValidateFeatureName también debe casar con validIncrementNameRegex, el
// mismo patrón kebab-case que ya aplica axiom change create y
// POST /api/increments. Este es el único paquete con visibilidad de
// validIncrementNameRegex (service.go:760), que no está exportada: por eso
// la propiedad se comprueba aquí y no en internal/odd.
//
// odd.ValidateFeatureName es estrictamente MÁS restrictiva que
// validIncrementNameRegex [D-10]: además del mismo patrón, exige una
// longitud máxima de 64 caracteres y rechaza la denylist de nombres
// reservados de Windows. Por construcción, ambos patrones son textualmente
// idénticos (^[a-z0-9]+(-[a-z0-9]+)*$), así que la propiedad se cumple para
// cualquier nombre que odd.ValidateFeatureName acepte: las restricciones
// adicionales de ODD sólo RECHAZAN más nombres, nunca aceptan uno que el
// patrón compartido no acepte primero. El corpus incluye deliberadamente
// nombres que odd.ValidateFeatureName rechaza por motivos ajenos al patrón
// compartido (longitud, denylist de Windows) para dejar constancia de esa
// asimetría [hallazgo O-2 del diseño, deliberadamente diferido]; la
// propiedad en sí sólo se comprueba cuando odd acepta el nombre.
func TestNameParity(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "un solo carácter", input: "a"},
		{name: "kebab-case simple", input: "gestion-inventario"},
		{name: "múltiples segmentos con dígitos", input: "modulo-2-v3-beta9"},
		{name: "segmentos de un solo carácter", input: "a-b-c-d-e"},
		{name: "exactamente 64 caracteres (límite de ODD)", input: strings.Repeat("a", 64)},
		{name: "65 caracteres: rechazado por longitud, no por el patrón compartido", input: strings.Repeat("a", 65)},
		{name: "mayúsculas: rechazado por el patrón compartido", input: "Gestion-Inventario"},
		{name: "guion bajo: rechazado por el patrón compartido", input: "gestion_inventario"},
		{name: "espacio: rechazado por el patrón compartido", input: "gestion inventario"},
		{name: "guion inicial: rechazado por el patrón compartido", input: "-gestion"},
		{name: "guion final: rechazado por el patrón compartido", input: "gestion-"},
		{name: "guiones consecutivos: rechazado por el patrón compartido", input: "gestion--inventario"},
		{name: "separador de ruta: rechazado por el patrón compartido", input: "a/b"},
		{name: "cadena vacía: rechazada por el patrón compartido", input: ""},
		{name: "nombre reservado de Windows: rechazado por la denylist de ODD [O-2, D-10]", input: "con"},
		{name: "otro nombre reservado de Windows: rechazado por la denylist de ODD [O-2, D-10]", input: "nul"},
		{name: "dispositivo serie reservado: rechazado por la denylist de ODD [O-2, D-10]", input: "com1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := odd.ValidateFeatureName(tt.input); err != nil {
				t.Logf("odd.ValidateFeatureName(%q) rechazado (%v); la propiedad de esta prueba no aplica a este caso", tt.input, err)
				return
			}
			if !validIncrementNameRegex.MatchString(tt.input) {
				t.Errorf("odd.ValidateFeatureName(%q) aceptó el nombre, pero validIncrementNameRegex lo rechaza: viola la invariante de D-10 (todo nombre válido para ODD debe ser válido para CreateIncrement)", tt.input)
			}
		})
	}
}
