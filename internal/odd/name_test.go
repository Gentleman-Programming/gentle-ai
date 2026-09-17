package odd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateFeatureName cubre la frontera T-7 de la matriz de amenazas del
// diseño: formato kebab-case, longitud máxima y denylist de nombres
// reservados del sistema de ficheros de Windows.
func TestValidateFeatureName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error // nil si se espera éxito
	}{
		{name: "kebab-case válido de varios bloques", input: "gestion-inventario", wantErr: nil},
		{name: "kebab-case válido de un único bloque", input: "demo", wantErr: nil},
		{name: "mayúsculas rechazadas", input: "Gestion-Inventario", wantErr: ErrInvalidFeatureName},
		{name: "cadena vacía rechazada", input: "", wantErr: ErrInvalidFeatureName},
		{name: "nombre de 300 caracteres supera el límite de 64", input: strings.Repeat("a", 300), wantErr: ErrInvalidFeatureName},
		{name: "doble punto rechazado", input: "..", wantErr: ErrInvalidFeatureName},
		{name: "doble punto anidado rechazado", input: "../..", wantErr: ErrInvalidFeatureName},
		{name: "ruta absoluta unix rechazada", input: "/etc/passwd", wantErr: ErrInvalidFeatureName},
		{name: "ruta absoluta windows rechazada", input: `C:\Windows`, wantErr: ErrInvalidFeatureName},
		{name: "separador unix embebido rechazado", input: "a/b", wantErr: ErrInvalidFeatureName},
		{name: "separador windows embebido rechazado", input: `a\b`, wantErr: ErrInvalidFeatureName},
		{name: "nombre reservado con", input: "con", wantErr: ErrReservedName},
		{name: "nombre reservado prn", input: "prn", wantErr: ErrReservedName},
		{name: "nombre reservado aux", input: "aux", wantErr: ErrReservedName},
		{name: "nombre reservado nul", input: "nul", wantErr: ErrReservedName},
		{name: "nombre reservado com1", input: "com1", wantErr: ErrReservedName},
		{name: "nombre reservado com2", input: "com2", wantErr: ErrReservedName},
		{name: "nombre reservado com3", input: "com3", wantErr: ErrReservedName},
		{name: "nombre reservado com4", input: "com4", wantErr: ErrReservedName},
		{name: "nombre reservado com5", input: "com5", wantErr: ErrReservedName},
		{name: "nombre reservado com6", input: "com6", wantErr: ErrReservedName},
		{name: "nombre reservado com7", input: "com7", wantErr: ErrReservedName},
		{name: "nombre reservado com8", input: "com8", wantErr: ErrReservedName},
		{name: "nombre reservado com9", input: "com9", wantErr: ErrReservedName},
		{name: "nombre reservado lpt1", input: "lpt1", wantErr: ErrReservedName},
		{name: "nombre reservado lpt2", input: "lpt2", wantErr: ErrReservedName},
		{name: "nombre reservado lpt3", input: "lpt3", wantErr: ErrReservedName},
		{name: "nombre reservado lpt4", input: "lpt4", wantErr: ErrReservedName},
		{name: "nombre reservado lpt5", input: "lpt5", wantErr: ErrReservedName},
		{name: "nombre reservado lpt6", input: "lpt6", wantErr: ErrReservedName},
		{name: "nombre reservado lpt7", input: "lpt7", wantErr: ErrReservedName},
		{name: "nombre reservado lpt8", input: "lpt8", wantErr: ErrReservedName},
		{name: "nombre reservado lpt9", input: "lpt9", wantErr: ErrReservedName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFeatureName(tt.input)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateFeatureName(%q) = %v, se esperaba nil", tt.input, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateFeatureName(%q) = nil, se esperaba error %v", tt.input, tt.wantErr)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateFeatureName(%q) = %v, no envuelve %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// TestDocumentPath confirma que DocumentPath reutiliza ValidateFeatureName,
// calcula la ruta esperada para un nombre válido, y que ningún caso
// rechazado deja ficheros fuera de t.TempDir(): DocumentPath es una función
// pura de cálculo de rutas, sin E/S.
func TestDocumentPath(t *testing.T) {
	rejected := []struct {
		name  string
		input string
	}{
		{name: "mayúsculas", input: "Gestion-Inventario"},
		{name: "cadena vacía", input: ""},
		{name: "300 caracteres", input: strings.Repeat("a", 300)},
		{name: "doble punto", input: ".."},
		{name: "doble punto anidado", input: "../.."},
		{name: "ruta absoluta unix", input: "/etc/passwd"},
		{name: "ruta absoluta windows", input: `C:\Windows`},
		{name: "separador unix", input: "a/b"},
		{name: "separador windows", input: `a\b`},
		{name: "reservado con", input: "con"},
		{name: "reservado nul", input: "nul"},
		{name: "reservado com1", input: "com1"},
		{name: "reservado lpt9", input: "lpt9"},
	}

	for _, tt := range rejected {
		t.Run("rechaza "+tt.name, func(t *testing.T) {
			root := t.TempDir()

			path, err := DocumentPath(root, tt.input)
			if err == nil {
				t.Fatalf("DocumentPath(%q, %q) = %q, se esperaba error", root, tt.input, path)
			}

			entries, readErr := os.ReadDir(root)
			if readErr != nil {
				t.Fatalf("no se pudo leer el directorio temporal %q: %v", root, readErr)
			}
			if len(entries) != 0 {
				t.Errorf("DocumentPath(%q, %q) dejó %d fichero(s) inesperado(s) en %q", root, tt.input, len(entries), root)
			}
		})
	}

	t.Run("nombre válido produce la ruta esperada bajo la raíz", func(t *testing.T) {
		root := t.TempDir()

		got, err := DocumentPath(root, "gestion-inventario")
		if err != nil {
			t.Fatalf("DocumentPath(%q, %q) devolvió error inesperado: %v", root, "gestion-inventario", err)
		}

		want := filepath.Join(root, "odd", "tasks", "gestion-inventario.md")
		if got != want {
			t.Errorf("DocumentPath(%q, %q) = %q, se esperaba %q", root, "gestion-inventario", got, want)
		}

		entries, readErr := os.ReadDir(root)
		if readErr != nil {
			t.Fatalf("no se pudo leer el directorio temporal %q: %v", root, readErr)
		}
		if len(entries) != 0 {
			t.Errorf("DocumentPath no debe escribir nada; se encontraron %d entradas en %q", len(entries), root)
		}
	})
}
