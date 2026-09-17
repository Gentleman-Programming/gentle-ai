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

// fakeCLIScaffolder es el equivalente, en este paquete, de
// odd.fakeScaffolder (internal/odd/promote_test.go): un Scaffolder falso
// instrumentado que registra cada invocación recibida y nunca llega a
// invocar dashboard.Service.CreateIncrement de verdad. No se puede
// reutilizar directamente porque es un identificador no exportado de otro
// paquete.
type fakeCLIScaffolder struct {
	result odd.ScaffoldResult
	err    error
	calls  []odd.ScaffoldRequest
}

func (f *fakeCLIScaffolder) Scaffold(req odd.ScaffoldRequest) (odd.ScaffoldResult, error) {
	f.calls = append(f.calls, req)
	return f.result, f.err
}

// fixedScaffolderFactory adapta un odd.Scaffolder falso ya construido a la
// firma func(root string) odd.Scaffolder que runODDPromote exige: la única
// forma de instrumentar el ciclo de promoción en pruebas sin invocar
// dashboardScaffolder -- y, por tanto, dashboard.Service.CreateIncrement --
// de verdad (mismo patrón que runODDStatus en internal/cli/odd_status.go
// con el Exporter inyectado). root llega ignorado a propósito: el falso no
// lo necesita.
func fixedScaffolderFactory(sc odd.Scaffolder) func(string) odd.Scaffolder {
	return func(string) odd.Scaffolder { return sc }
}

// TestRunODDPromote cubre la tarea 5.7 (REQ-19.8, REQ-19.9, REQ-19.10,
// REQ-19.12, diseño §5.7 y §3.5): resumen y exit 0 en éxito; --dry-run sin
// ninguna escritura; feature ya promovida; colisión de nombre; fallo de
// marca tras una creación exitosa con la línea de remediación exacta de
// D-07.
func TestRunODDPromote(t *testing.T) {
	t.Run("ciclo completo exitoso: resumen en stdout, exit 0, Scaffolder invocado una vez", func(t *testing.T) {
		root := t.TempDir()
		const feature = "gestion-inventario"
		if _, err := odd.Create(root, feature, "2026-09-17"); err != nil {
			t.Fatalf("no se pudo preparar el documento vivo de prueba: %v", err)
		}

		sc := &fakeCLIScaffolder{result: odd.ScaffoldResult{Name: "modulo-inventario-v2", Path: "openspec/changes/modulo-inventario-v2/proposal.md"}}
		var stdout bytes.Buffer

		err := RunODDPromote([]string{feature, "--cwd", root, "--name", "modulo-inventario-v2", "--intent", "Reducir el tiempo de conciliación", "--type", "feature"}, &stdout, fixedScaffolderFactory(sc))
		if err != nil {
			t.Fatalf("RunODDPromote() devolvió error inesperado: %v", err)
		}

		if len(sc.calls) != 1 {
			t.Fatalf("Scaffolder invocado %d veces, se esperaba exactamente 1", len(sc.calls))
		}
		gotCall := sc.calls[0]
		if gotCall.Name != "modulo-inventario-v2" {
			t.Errorf("ScaffoldRequest.Name = %q, se esperaba %q (--name)", gotCall.Name, "modulo-inventario-v2")
		}
		if gotCall.Intent != "Reducir el tiempo de conciliación" {
			t.Errorf("ScaffoldRequest.Intent = %q, no se reenvió --intent tal cual", gotCall.Intent)
		}
		if gotCall.Type != "feature" {
			t.Errorf("ScaffoldRequest.Type = %q, no se reenvió --type tal cual", gotCall.Type)
		}
		if gotCall.ProposalBody == "" {
			t.Errorf("ScaffoldRequest.ProposalBody está vacío, se esperaba el cuerpo sembrado")
		}

		out := stdout.String()
		if !strings.Contains(out, feature) || !strings.Contains(out, "modulo-inventario-v2") {
			t.Errorf("el resumen no menciona la feature y el cambio SDD creado: %q", out)
		}
		if !strings.Contains(out, "espejo") {
			t.Errorf("el resumen no incluye el recordatorio de espejo pendiente [diseño §3.5, §5.5]: %q", out)
		}
	})

	t.Run("--dry-run imprime el cuerpo por stdout, exit 0, cero escrituras", func(t *testing.T) {
		root := t.TempDir()
		const feature = "gestion-inventario"
		if _, err := odd.Create(root, feature, "2026-09-17"); err != nil {
			t.Fatalf("no se pudo preparar el documento vivo de prueba: %v", err)
		}
		path := filepath.Join(root, "odd", "tasks", feature+".md")
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no se pudo leer el documento antes de la promoción: %v", err)
		}

		sc := &fakeCLIScaffolder{result: odd.ScaffoldResult{Name: "modulo-inventario-v2", Path: "irrelevante"}}
		var stdout bytes.Buffer

		err = RunODDPromote([]string{feature, "--cwd", root, "--dry-run"}, &stdout, fixedScaffolderFactory(sc))
		if err != nil {
			t.Fatalf("RunODDPromote() con --dry-run devolvió error inesperado: %v", err)
		}

		if len(sc.calls) != 0 {
			t.Errorf("Scaffolder invocado %d veces, se esperaba 0 (--dry-run no escribe nada)", len(sc.calls))
		}
		if !strings.Contains(stdout.String(), "## Propósito (Intent)") {
			t.Errorf("--dry-run no imprimió el cuerpo sembrado por stdout: %q", stdout.String())
		}
		if _, statErr := os.Stat(filepath.Join(root, "openspec")); !os.IsNotExist(statErr) {
			t.Errorf("--dry-run creó %q, se esperaba que no existiera ningún directorio openspec/", filepath.Join(root, "openspec"))
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no se pudo leer el documento después de la promoción: %v", err)
		}
		if string(before) != string(after) {
			t.Errorf("--dry-run modificó el documento ODD; se esperaba que permaneciera intacto")
		}
	})

	t.Run("feature ya promovida devuelve error de idempotencia sin invocar al Scaffolder", func(t *testing.T) {
		root := t.TempDir()
		const feature = "gestion-inventario"
		doc, err := odd.Create(root, feature, "2026-09-17")
		if err != nil {
			t.Fatalf("no se pudo preparar el documento vivo de prueba: %v", err)
		}
		if err := odd.MarkPromoted(root, doc, "modulo-inventario-v1", "2026-09-17"); err != nil {
			t.Fatalf("no se pudo marcar el documento como ya promovido: %v", err)
		}

		sc := &fakeCLIScaffolder{result: odd.ScaffoldResult{Name: "irrelevante", Path: "irrelevante"}}
		var stdout bytes.Buffer

		err = RunODDPromote([]string{feature, "--cwd", root}, &stdout, fixedScaffolderFactory(sc))
		if err == nil {
			t.Fatalf("RunODDPromote() = nil error, se esperaba error de idempotencia")
		}
		if !errors.Is(err, odd.ErrAlreadyPromoted) {
			t.Errorf("RunODDPromote() = %v, no envuelve odd.ErrAlreadyPromoted", err)
		}
		if !strings.Contains(err.Error(), "ya fue promovida") {
			t.Errorf("RunODDPromote() = %v, se esperaba un mensaje de idempotencia explícito", err)
		}
		if len(sc.calls) != 0 {
			t.Errorf("Scaffolder invocado %d veces, se esperaba 0 (feature ya promovida) [D-08]", len(sc.calls))
		}
	})

	t.Run("colisión de nombre devuelve error explícito sin modificar el documento ODD", func(t *testing.T) {
		root := t.TempDir()
		const feature = "gestion-inventario"
		if _, err := odd.Create(root, feature, "2026-09-17"); err != nil {
			t.Fatalf("no se pudo preparar el documento vivo de prueba: %v", err)
		}
		path := filepath.Join(root, "odd", "tasks", feature+".md")
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no se pudo leer el documento antes de la promoción: %v", err)
		}

		collisionErr := errors.New(`el incremento "modulo-inventario-v2" ya existe como cambio activo`)
		sc := &fakeCLIScaffolder{err: collisionErr}
		var stdout bytes.Buffer

		err = RunODDPromote([]string{feature, "--cwd", root, "--name", "modulo-inventario-v2"}, &stdout, fixedScaffolderFactory(sc))
		if err == nil {
			t.Fatalf("RunODDPromote() = nil error, se esperaba el error de colisión propagado")
		}
		if !strings.Contains(err.Error(), "ya existe como cambio activo") {
			t.Errorf("RunODDPromote() = %v, se esperaba que propagara el mensaje de colisión", err)
		}

		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no se pudo leer el documento después de la promoción: %v", err)
		}
		if string(before) != string(after) {
			t.Errorf("el documento ODD se modificó tras una colisión de nombre; se esperaba que permaneciera intacto")
		}
	})

	t.Run("fallo al escribir la marca tras una creación exitosa devuelve la línea de remediación exacta de D-07", func(t *testing.T) {
		root := t.TempDir()
		const feature = "gestion-inventario"
		if _, err := odd.Create(root, feature, "2026-09-17"); err != nil {
			t.Fatalf("no se pudo preparar el documento vivo de prueba: %v", err)
		}
		path := filepath.Join(root, "odd", "tasks", feature+".md")
		if err := os.Chmod(path, 0o444); err != nil {
			t.Fatalf("no se pudo marcar el documento como solo lectura para simular el fallo de escritura: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

		const changeName = "modulo-inventario-v2"
		sc := &fakeCLIScaffolder{result: odd.ScaffoldResult{Name: changeName, Path: "openspec/changes/" + changeName + "/proposal.md"}}
		var stdout bytes.Buffer

		err := RunODDPromote([]string{feature, "--cwd", root, "--name", changeName}, &stdout, fixedScaffolderFactory(sc))
		if err == nil {
			t.Fatalf("RunODDPromote() = nil error, se esperaba la línea de remediación de D-07")
		}

		wantPrefix := "Aviso: el cambio SDD se creó en openspec/changes/" + changeName + "/ pero no se pudo marcar odd/tasks/" + feature + ".md como promovido ("
		wantSuffix := "). Añade manualmente la línea «> **Promovido a:** `openspec/changes/" + changeName + "/`» o elimina el directorio del cambio."
		if !strings.HasPrefix(err.Error(), wantPrefix) {
			t.Errorf("RunODDPromote() = %q, no empieza por el prefijo de remediación esperado %q", err.Error(), wantPrefix)
		}
		if !strings.HasSuffix(err.Error(), wantSuffix) {
			t.Errorf("RunODDPromote() = %q, no termina en el sufijo de remediación esperado %q", err.Error(), wantSuffix)
		}
	})
}
