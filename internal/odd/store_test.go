package odd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreate cubre la creación del documento vivo y su guarda de colisión.
func TestCreate(t *testing.T) {
	t.Run("crea el documento vivo con las doce secciones", func(t *testing.T) {
		root := t.TempDir()

		doc, err := Create(root, "gestion-inventario", "2026-09-17")
		if err != nil {
			t.Fatalf("Create() devolvió error inesperado: %v", err)
		}
		if doc.Feature != "gestion-inventario" {
			t.Errorf("Feature = %q, se esperaba %q", doc.Feature, "gestion-inventario")
		}
		if doc.Status != StatusActive {
			t.Errorf("Status = %q, se esperaba %q", doc.Status, StatusActive)
		}
		if len(doc.Sections) != len(CanonicalSections) {
			t.Errorf("Sections tiene %d entradas, se esperaban %d", len(doc.Sections), len(CanonicalSections))
		}

		wantPath := filepath.Join(root, "odd", "tasks", "gestion-inventario.md")
		if _, statErr := os.Stat(wantPath); statErr != nil {
			t.Fatalf("no se encontró el fichero creado en %s: %v", wantPath, statErr)
		}
	})

	t.Run("feature ya existente devuelve ErrFeatureExists sin sobrescribir", func(t *testing.T) {
		root := t.TempDir()

		if _, err := Create(root, "gestion-inventario", "2026-09-17"); err != nil {
			t.Fatalf("Create() inicial devolvió error inesperado: %v", err)
		}

		path := filepath.Join(root, "odd", "tasks", "gestion-inventario.md")
		before, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("no se pudo leer el documento tras la primera creación: %v", readErr)
		}

		_, err := Create(root, "gestion-inventario", "2026-09-18")
		if err == nil {
			t.Fatalf("Create() = nil error en la segunda llamada, se esperaba ErrFeatureExists")
		}
		if !errors.Is(err, ErrFeatureExists) {
			t.Errorf("Create() = %v, no envuelve ErrFeatureExists", err)
		}

		after, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("no se pudo leer el documento tras el intento de colisión: %v", readErr)
		}
		if string(before) != string(after) {
			t.Errorf("el documento existente se modificó tras un Create() en colisión")
		}
	})
}

// TestLoad cubre la carga de un documento vivo existente y el error
// centinela cuando la feature solicitada no tiene documento.
func TestLoad(t *testing.T) {
	t.Run("feature inexistente devuelve ErrFeatureNotFound", func(t *testing.T) {
		root := t.TempDir()

		_, err := Load(root, "no-existe")
		if err == nil {
			t.Fatalf("Load() = nil error, se esperaba ErrFeatureNotFound")
		}
		if !errors.Is(err, ErrFeatureNotFound) {
			t.Errorf("Load() = %v, no envuelve ErrFeatureNotFound", err)
		}
	})

	t.Run("feature existente se analiza correctamente", func(t *testing.T) {
		root := t.TempDir()

		created, err := Create(root, "gestion-inventario", "2026-09-17")
		if err != nil {
			t.Fatalf("Create() devolvió error inesperado: %v", err)
		}

		loaded, err := Load(root, "gestion-inventario")
		if err != nil {
			t.Fatalf("Load() devolvió error inesperado: %v", err)
		}
		if loaded.Feature != created.Feature {
			t.Errorf("Feature = %q, se esperaba %q", loaded.Feature, created.Feature)
		}
		if loaded.Path == "" {
			t.Errorf("Path está vacío, se esperaba la ruta relativa del documento")
		}
	})
}

// TestScan cubre el listado de documentos vivos, incluido el caso de
// directorio ausente (workspace sin carril ágil todavía).
func TestScan(t *testing.T) {
	t.Run("directorio odd/tasks ausente devuelve lista vacía sin error", func(t *testing.T) {
		root := t.TempDir()

		summaries, err := Scan(root)
		if err != nil {
			t.Fatalf("Scan() devolvió error inesperado: %v", err)
		}
		if len(summaries) != 0 {
			t.Errorf("Scan() = %d resúmenes, se esperaban 0", len(summaries))
		}
	})

	t.Run("directorio con documentos vivos devuelve un resumen por feature", func(t *testing.T) {
		root := t.TempDir()

		if _, err := Create(root, "gestion-inventario", "2026-09-17"); err != nil {
			t.Fatalf("Create() devolvió error inesperado: %v", err)
		}
		if _, err := Create(root, "modulo-facturacion", "2026-09-17"); err != nil {
			t.Fatalf("Create() devolvió error inesperado: %v", err)
		}

		summaries, err := Scan(root)
		if err != nil {
			t.Fatalf("Scan() devolvió error inesperado: %v", err)
		}
		if len(summaries) != 2 {
			t.Fatalf("Scan() = %d resúmenes, se esperaban 2", len(summaries))
		}
	})
}

// TestMarkPromoted cubre la reescritura de la cabecera tras una promoción:
// estado, referencia al cambio y fecha, tanto en memoria como en disco.
func TestMarkPromoted(t *testing.T) {
	root := t.TempDir()

	doc, err := Create(root, "gestion-inventario", "2026-09-17")
	if err != nil {
		t.Fatalf("Create() devolvió error inesperado: %v", err)
	}

	if err := MarkPromoted(root, doc, "modulo-inventario-v2", "2026-09-18"); err != nil {
		t.Fatalf("MarkPromoted() devolvió error inesperado: %v", err)
	}

	if doc.Status != StatusPromoted {
		t.Errorf("Status = %q tras MarkPromoted, se esperaba %q", doc.Status, StatusPromoted)
	}
	wantPromotedTo := "openspec/changes/modulo-inventario-v2/"
	if doc.PromotedTo != wantPromotedTo {
		t.Errorf("PromotedTo = %q, se esperaba %q", doc.PromotedTo, wantPromotedTo)
	}
	if doc.PromotedAt != "2026-09-18" {
		t.Errorf("PromotedAt = %q, se esperaba %q", doc.PromotedAt, "2026-09-18")
	}

	reloaded, err := Load(root, "gestion-inventario")
	if err != nil {
		t.Fatalf("Load() tras MarkPromoted devolvió error inesperado: %v", err)
	}
	if reloaded.Status != StatusPromoted {
		t.Errorf("el fichero en disco no refleja el estado promovido tras recargarlo: %q", reloaded.Status)
	}
	if reloaded.PromotedTo != wantPromotedTo {
		t.Errorf("el fichero en disco no refleja la referencia de promoción tras recargarlo: %q", reloaded.PromotedTo)
	}
	if !strings.Contains(reloaded.Raw, "**Promovido el:** 2026-09-18") {
		t.Errorf("el documento reescrito no contiene la fecha de promoción esperada")
	}
}
