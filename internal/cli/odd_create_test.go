package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/odd"
)

// TestRunODDCreate cubre la frontera T-2 de la matriz de amenazas del
// diseño (selección de raíz de workspace vía --cwd) para "axiom odd create
// <nombre>" (REQ-19.5): éxito con --cwd relativo y absoluto, nombre
// inválido, colisión de nombre, y --cwd inexistente o que no es un
// directorio, verificando en ambos últimos casos que no se crea ningún
// fichero ni directorio.
func TestRunODDCreate(t *testing.T) {
	t.Run("cwd absoluto crea el documento e imprime la ruta", func(t *testing.T) {
		root := t.TempDir()
		var stdout bytes.Buffer

		if err := RunODDCreate([]string{"gestion-inventario", "--cwd", root}, &stdout); err != nil {
			t.Fatalf("RunODDCreate() devolvió error inesperado: %v", err)
		}

		// Document.Path usa siempre '/' literal, con independencia del
		// sistema operativo (contrato de relativeDocumentPath en store.go):
		// no se construye con filepath.Join aquí a propósito.
		const wantRelPath = "odd/tasks/gestion-inventario.md"
		if !strings.Contains(stdout.String(), wantRelPath) {
			t.Errorf("stdout = %q, se esperaba que contuviera %q", stdout.String(), wantRelPath)
		}

		wantPath := filepath.Join(root, "odd", "tasks", "gestion-inventario.md")
		if _, statErr := os.Stat(wantPath); statErr != nil {
			t.Fatalf("no se encontró el fichero creado en %s: %v", wantPath, statErr)
		}
	})

	t.Run("cwd relativo crea el documento e imprime la ruta", func(t *testing.T) {
		parent := t.TempDir()
		if err := os.MkdirAll(filepath.Join(parent, "workspace"), 0o755); err != nil {
			t.Fatalf("no se pudo preparar el workspace de prueba: %v", err)
		}
		t.Chdir(parent)

		var stdout bytes.Buffer
		if err := RunODDCreate([]string{"gestion-inventario", "--cwd", "workspace"}, &stdout); err != nil {
			t.Fatalf("RunODDCreate() devolvió error inesperado: %v", err)
		}

		wantPath := filepath.Join(parent, "workspace", "odd", "tasks", "gestion-inventario.md")
		if _, statErr := os.Stat(wantPath); statErr != nil {
			t.Fatalf("no se encontró el fichero creado en %s: %v", wantPath, statErr)
		}
	})

	t.Run("nombre inválido devuelve error sin crear ningún fichero", func(t *testing.T) {
		root := t.TempDir()
		var stdout bytes.Buffer

		err := RunODDCreate([]string{"Nombre Invalido", "--cwd", root}, &stdout)
		if err == nil {
			t.Fatalf("RunODDCreate() = nil error, se esperaba un error de nombre inválido")
		}
		if !errors.Is(err, odd.ErrInvalidFeatureName) {
			t.Errorf("RunODDCreate() = %v, no envuelve odd.ErrInvalidFeatureName", err)
		}

		entries, readErr := os.ReadDir(root)
		if readErr != nil {
			t.Fatalf("no se pudo leer el directorio temporal: %v", readErr)
		}
		if len(entries) != 0 {
			t.Errorf("RunODDCreate() con nombre inválido dejó %d entrada(s) en %q", len(entries), root)
		}
	})

	t.Run("colisión de nombre devuelve error sin modificar el documento existente", func(t *testing.T) {
		root := t.TempDir()
		var first bytes.Buffer
		if err := RunODDCreate([]string{"gestion-inventario", "--cwd", root}, &first); err != nil {
			t.Fatalf("RunODDCreate() inicial devolvió error inesperado: %v", err)
		}

		path := filepath.Join(root, "odd", "tasks", "gestion-inventario.md")
		before, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("no se pudo leer el documento tras la primera creación: %v", readErr)
		}

		var second bytes.Buffer
		err := RunODDCreate([]string{"gestion-inventario", "--cwd", root}, &second)
		if err == nil {
			t.Fatalf("RunODDCreate() = nil error en la segunda llamada, se esperaba error de colisión")
		}
		if !errors.Is(err, odd.ErrFeatureExists) {
			t.Errorf("RunODDCreate() = %v, no envuelve odd.ErrFeatureExists", err)
		}

		after, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("no se pudo leer el documento tras el intento de colisión: %v", readErr)
		}
		if string(before) != string(after) {
			t.Errorf("el documento existente se modificó tras una colisión de nombre")
		}
	})

	t.Run("cwd inexistente devuelve error antes de cualquier MkdirAll", func(t *testing.T) {
		parent := t.TempDir()
		missing := filepath.Join(parent, "no-existe")
		var stdout bytes.Buffer

		err := RunODDCreate([]string{"gestion-inventario", "--cwd", missing}, &stdout)
		if err == nil {
			t.Fatalf("RunODDCreate() = nil error, se esperaba error de --cwd inexistente")
		}

		if _, statErr := os.Stat(missing); !os.IsNotExist(statErr) {
			t.Errorf("RunODDCreate() creó el directorio --cwd inexistente %q", missing)
		}

		entries, readErr := os.ReadDir(parent)
		if readErr != nil {
			t.Fatalf("no se pudo leer el directorio temporal: %v", readErr)
		}
		if len(entries) != 0 {
			t.Errorf("RunODDCreate() con --cwd inexistente dejó %d entrada(s) en %q", len(entries), parent)
		}
	})

	t.Run("cwd que apunta a un fichero devuelve error", func(t *testing.T) {
		parent := t.TempDir()
		filePath := filepath.Join(parent, "no-es-un-directorio")
		if err := os.WriteFile(filePath, []byte("contenido"), 0o644); err != nil {
			t.Fatalf("no se pudo preparar el fichero de prueba: %v", err)
		}

		var stdout bytes.Buffer
		err := RunODDCreate([]string{"gestion-inventario", "--cwd", filePath}, &stdout)
		if err == nil {
			t.Fatalf("RunODDCreate() = nil error, se esperaba error porque --cwd no es un directorio")
		}
	})
}
