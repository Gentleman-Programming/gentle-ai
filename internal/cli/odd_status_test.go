package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/odd"
)

// fakeODDExporter construye un odd.Exporter de prueba que devuelve
// observations y err de forma fija, sin invocar ningún subproceso real. Es
// el equivalente, en este paquete, de odd.fakeExporter
// (internal/odd/mirror_test.go): no se puede reutilizar directamente porque
// es un identificador no exportado de otro paquete.
func fakeODDExporter(observations []odd.Observation, err error) odd.Exporter {
	return func(context.Context, string) ([]odd.Observation, error) {
		return observations, err
	}
}

// panicIfCalledODDExporter construye un odd.Exporter que hace fallar la
// prueba si llega a invocarse. Demuestra que, sin --check-mirror, "axiom odd
// status" no realiza ninguna comparación contra Engram (REQ-19.7).
func panicIfCalledODDExporter(t *testing.T) odd.Exporter {
	t.Helper()
	return func(context.Context, string) ([]odd.Observation, error) {
		t.Fatal("el exportador del espejo se invocó sin --check-mirror")
		return nil, nil
	}
}

// createODDFixture crea, para las pruebas de este fichero, un documento vivo
// de prueba mediante RunODDCreate (ya cubierto por su propia suite en
// odd_create_test.go), evitando reconstruir aquí el contenido canónico de la
// plantilla.
func createODDFixture(t *testing.T, root, feature string) {
	t.Helper()
	var stdout bytes.Buffer
	if err := RunODDCreate([]string{feature, "--cwd", root}, &stdout); err != nil {
		t.Fatalf("no se pudo preparar el documento vivo de prueba %q: %v", feature, err)
	}
}

// TestRunODDStatus_TextoYJSON cubre la salida en texto legible por defecto
// frente a la salida estructurada con --json (REQ-19.6).
func TestRunODDStatus_TextoYJSON(t *testing.T) {
	root := t.TempDir()
	createODDFixture(t, root, "gestion-inventario")

	t.Run("salida en texto por defecto", func(t *testing.T) {
		var stdout bytes.Buffer
		if err := runODDStatus([]string{"--cwd", root}, &stdout, panicIfCalledODDExporter(t)); err != nil {
			t.Fatalf("runODDStatus() devolvió error inesperado: %v", err)
		}
		out := stdout.String()
		if !strings.Contains(out, "gestion-inventario") || !strings.Contains(out, "activo") {
			t.Errorf("salida en texto inesperada: %q", out)
		}
	})

	t.Run("salida en JSON con --json", func(t *testing.T) {
		var stdout bytes.Buffer
		if err := runODDStatus([]string{"--cwd", root, "--json"}, &stdout, panicIfCalledODDExporter(t)); err != nil {
			t.Fatalf("runODDStatus() devolvió error inesperado: %v", err)
		}

		var report odd.StatusReport
		if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
			t.Fatalf("la salida --json no es JSON válido: %v\nsalida: %s", err, stdout.String())
		}
		if len(report.Features) != 1 || report.Features[0].Feature != "gestion-inventario" {
			t.Errorf("StatusReport inesperado: %+v", report)
		}
	})
}

// TestRunODDStatus_SinCheckMirrorNoInvocaElEspejo cubre el primer escenario
// de REQ-19.7: sin --check-mirror, el comando nunca invoca ninguna
// comparación contra Engram.
func TestRunODDStatus_SinCheckMirrorNoInvocaElEspejo(t *testing.T) {
	root := t.TempDir()
	createODDFixture(t, root, "gestion-inventario")

	var stdout bytes.Buffer
	if err := runODDStatus([]string{"--cwd", root}, &stdout, panicIfCalledODDExporter(t)); err != nil {
		t.Fatalf("runODDStatus() devolvió error inesperado: %v", err)
	}
	if strings.Contains(stdout.String(), "Espejo:") {
		t.Errorf("la salida sin --check-mirror no debe mencionar el estado del espejo: %q", stdout.String())
	}
}

// TestRunODDStatus_CheckMirrorTresEstados cubre REQ-19.7 y la decisión O-1
// confirmada por el orquestador: --check-mirror informa sincronizado,
// divergente y no disponible, y en los tres casos runODDStatus devuelve
// error nil (equivalente a exit 0 desde runODD en cmd/axiom/main.go).
func TestRunODDStatus_CheckMirrorTresEstados(t *testing.T) {
	root := t.TempDir()
	createODDFixture(t, root, "gestion-inventario")

	path := filepath.Join(root, "odd", "tasks", "gestion-inventario.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no se pudo leer el documento de prueba: %v", err)
	}
	topic := odd.MirrorTopic("gestion-inventario")

	tests := []struct {
		name      string
		export    odd.Exporter
		wantState odd.MirrorState
	}{
		{
			name:      "sincronizado cuando el espejo coincide con el fichero",
			export:    fakeODDExporter([]odd.Observation{{Topic: topic, Content: string(raw)}}, nil),
			wantState: odd.MirrorSynced,
		},
		{
			name:      "divergente cuando el espejo difiere del fichero",
			export:    fakeODDExporter([]odd.Observation{{Topic: topic, Content: "# ODD: gestion-inventario\ncontenido distinto\n"}}, nil),
			wantState: odd.MirrorDiverged,
		},
		{
			name:      "no disponible cuando el exportador falla",
			export:    fakeODDExporter(nil, errors.New("fallo simulado del transporte")),
			wantState: odd.MirrorUnavailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := runODDStatus([]string{"--cwd", root, "--check-mirror"}, &stdout, tc.export)
			if err != nil {
				t.Fatalf("runODDStatus() con --check-mirror devolvió error inesperado para el estado %q (los tres estados de espejo deben ser exit 0, decisión O-1): %v", tc.wantState, err)
			}
			if !strings.Contains(stdout.String(), string(tc.wantState)) {
				t.Errorf("la salida no informa el estado de espejo %q: %q", tc.wantState, stdout.String())
			}
		})
	}
}

// TestRunODDStatus_FeatureNombradaInexistente cubre el contrato de CLI
// (diseño §5.7): una feature nombrada que no existe devuelve un error que
// envuelve odd.ErrFeatureNotFound (exit distinto de 0 desde runODD).
func TestRunODDStatus_FeatureNombradaInexistente(t *testing.T) {
	root := t.TempDir()

	var stdout bytes.Buffer
	err := RunODDStatus([]string{"feature-inexistente", "--cwd", root}, &stdout)
	if err == nil {
		t.Fatalf("RunODDStatus() = nil error, se esperaba error de feature no encontrada")
	}
	if !errors.Is(err, odd.ErrFeatureNotFound) {
		t.Errorf("RunODDStatus() = %v, no envuelve odd.ErrFeatureNotFound", err)
	}
}

// TestRunODDStatus_FeatureNombradaExistente confirma que pedir el estado de
// una única feature por nombre (en vez del listado completo) funciona sin
// --check-mirror.
func TestRunODDStatus_FeatureNombradaExistente(t *testing.T) {
	root := t.TempDir()
	createODDFixture(t, root, "gestion-inventario")

	var stdout bytes.Buffer
	if err := RunODDStatus([]string{"gestion-inventario", "--cwd", root}, &stdout); err != nil {
		t.Fatalf("RunODDStatus() devolvió error inesperado: %v", err)
	}
	if !strings.Contains(stdout.String(), "gestion-inventario") {
		t.Errorf("la salida no menciona la feature solicitada: %q", stdout.String())
	}
}

// TestRunODDStatus_CWDInexistenteDevuelveError cubre la frontera T-2 de la
// matriz de amenazas del diseño para "axiom odd status": un --cwd inexistente
// devuelve error, igual que ya exige TestRunODDCreate para "create".
func TestRunODDStatus_CWDInexistenteDevuelveError(t *testing.T) {
	parent := t.TempDir()
	missing := filepath.Join(parent, "no-existe")

	var stdout bytes.Buffer
	err := RunODDStatus([]string{"--cwd", missing}, &stdout)
	if err == nil {
		t.Fatalf("RunODDStatus() = nil error, se esperaba error de --cwd inexistente")
	}
}

// TestRunODDStatus_SinDocumentosVivos cubre el estado vacío explícito
// (REQ-19.6): un workspace sin odd/tasks/ informa "sin documentos", nunca un
// error.
func TestRunODDStatus_SinDocumentosVivos(t *testing.T) {
	root := t.TempDir()

	var stdout bytes.Buffer
	if err := RunODDStatus([]string{"--cwd", root}, &stdout); err != nil {
		t.Fatalf("RunODDStatus() devolvió error inesperado con directorio vacío: %v", err)
	}
	if !strings.Contains(stdout.String(), "No se encontraron documentos vivos") {
		t.Errorf("salida inesperada sin documentos vivos: %q", stdout.String())
	}
}
