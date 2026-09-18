package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/odd"
)

// TestGetODDFeatures cubre la tarea 6.1 (REQ-19.13): GetODDFeatures delega en
// odd.Scan sobre la raíz del workspace del servicio, tanto para un workspace
// sin documentos vivos como para uno con varios.
func TestGetODDFeatures(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T, root string)
		wantFeatures []string
	}{
		{
			name:         "workspace sin odd/tasks/ devuelve lista vacía sin error",
			setup:        func(t *testing.T, root string) {},
			wantFeatures: nil,
		},
		{
			name: "workspace con dos documentos vivos devuelve ambos resúmenes",
			setup: func(t *testing.T, root string) {
				if _, err := odd.Create(root, "gestion-inventario", "2026-09-17"); err != nil {
					t.Fatalf("odd.Create() error = %v", err)
				}
				if _, err := odd.Create(root, "modulo-pagos", "2026-09-17"); err != nil {
					t.Fatalf("odd.Create() error = %v", err)
				}
			},
			wantFeatures: []string{"gestion-inventario", "modulo-pagos"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)

			svc := NewService(root)
			got, err := svc.GetODDFeatures()
			if err != nil {
				t.Fatalf("GetODDFeatures() error = %v", err)
			}

			if len(got) != len(tt.wantFeatures) {
				t.Fatalf("GetODDFeatures() devolvió %d features, esperado %d: %+v", len(got), len(tt.wantFeatures), got)
			}
			for i, want := range tt.wantFeatures {
				if got[i].Feature != want {
					t.Errorf("GetODDFeatures()[%d].Feature = %q, esperado %q", i, got[i].Feature, want)
				}
			}
		})
	}
}

// TestGetODDFeature cubre la tarea 6.1: GetODDFeature con una feature
// existente devuelve el documento ya analizado, y con una feature
// inexistente propaga odd.ErrFeatureNotFound.
func TestGetODDFeature(t *testing.T) {
	root := t.TempDir()
	if _, err := odd.Create(root, "gestion-inventario", "2026-09-17"); err != nil {
		t.Fatalf("odd.Create() error = %v", err)
	}
	svc := NewService(root)

	t.Run("feature existente devuelve el documento analizado", func(t *testing.T) {
		doc, err := svc.GetODDFeature("gestion-inventario")
		if err != nil {
			t.Fatalf("GetODDFeature() error = %v", err)
		}
		if doc.Feature != "gestion-inventario" {
			t.Errorf("GetODDFeature().Feature = %q, esperado %q", doc.Feature, "gestion-inventario")
		}
	})

	t.Run("feature inexistente propaga ErrFeatureNotFound", func(t *testing.T) {
		_, err := svc.GetODDFeature("no-existe")
		if !errors.Is(err, odd.ErrFeatureNotFound) {
			t.Errorf("GetODDFeature() error = %v, esperado errors.Is(..., odd.ErrFeatureNotFound)", err)
		}
	})
}

// TestCreateODDFeature cubre la tarea 6.1: CreateODDFeature recorta y valida
// la presencia del campo antes de delegar en odd.Create, y traduce el error
// centinela del dominio cuando el nombre no es válido, sin envolverlo de
// forma que errors.Is deje de reconocerlo [skill axiom-idiomatic-error-wrapping].
func TestCreateODDFeature(t *testing.T) {
	tests := []struct {
		name      string
		req       ODDCreateRequest
		wantErr   bool
		wantErrIs error // opcional: fija el error centinela exacto cuando aplica
	}{
		{name: "feature válida crea el documento", req: ODDCreateRequest{Feature: "gestion-inventario"}},
		{name: "feature vacía se rechaza antes de llegar al dominio", req: ODDCreateRequest{Feature: "   "}, wantErr: true},
		{name: "nombre inválido se traduce al error centinela del dominio", req: ODDCreateRequest{Feature: "Nombre Invalido"}, wantErr: true, wantErrIs: odd.ErrInvalidFeatureName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			svc := NewService(root)

			doc, err := svc.CreateODDFeature(tt.req)

			if (err != nil) != tt.wantErr {
				t.Fatalf("CreateODDFeature() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Errorf("CreateODDFeature() error = %v, esperado errors.Is(..., %v)", err, tt.wantErrIs)
			}
			if tt.wantErr {
				return
			}

			want := strings.TrimSpace(tt.req.Feature)
			if doc.Feature != want {
				t.Errorf("CreateODDFeature().Feature = %q, esperado %q", doc.Feature, want)
			}
			if _, statErr := os.Stat(filepath.Join(root, "odd", "tasks", want+".md")); statErr != nil {
				t.Errorf("CreateODDFeature() no escribió el fichero esperado: %v", statErr)
			}
		})
	}
}

// TestPromoteODDFeature cubre la tarea 6.1: PromoteODDFeature delega en
// odd.Promote a través del adaptador Scaffolder de este mismo paquete
// (oddScaffolder sobre Service.CreateIncrement). Secuencia intencionadamente
// estatal en un único workspace: dry-run primero (no debe mutar nada),
// promoción real después, y una segunda promoción para confirmar la guarda
// de idempotencia [D-08] — el mismo estilo secuencial que ya usa
// TestProjectsEndpoints en este paquete para un flujo con estado compartido.
func TestPromoteODDFeature(t *testing.T) {
	root := t.TempDir()
	if _, err := odd.Create(root, "gestion-inventario", "2026-09-17"); err != nil {
		t.Fatalf("odd.Create() error = %v", err)
	}
	svc := NewService(root)

	t.Run("dry-run no escribe nada y devuelve DryRun=true", func(t *testing.T) {
		result, err := svc.PromoteODDFeature(ODDPromoteRequest{Feature: "gestion-inventario", DryRun: true})
		if err != nil {
			t.Fatalf("PromoteODDFeature() error = %v", err)
		}
		if !result.DryRun {
			t.Errorf("PromoteODDFeature(dry-run).DryRun = false, esperado true")
		}
		if _, statErr := os.Stat(filepath.Join(root, "openspec", "changes", "gestion-inventario")); !os.IsNotExist(statErr) {
			t.Errorf("PromoteODDFeature(dry-run) no debía crear openspec/changes/gestion-inventario/, statErr = %v", statErr)
		}
	})

	t.Run("promoción real crea el cambio SDD y marca el documento", func(t *testing.T) {
		result, err := svc.PromoteODDFeature(ODDPromoteRequest{Feature: "gestion-inventario"})
		if err != nil {
			t.Fatalf("PromoteODDFeature() error = %v", err)
		}
		if !result.MarkWritten {
			t.Errorf("PromoteODDFeature().MarkWritten = false, esperado true")
		}
		if _, statErr := os.Stat(filepath.Join(root, "openspec", "changes", "gestion-inventario", "proposal.md")); statErr != nil {
			t.Errorf("PromoteODDFeature() no creó proposal.md: %v", statErr)
		}

		doc, err := svc.GetODDFeature("gestion-inventario")
		if err != nil {
			t.Fatalf("GetODDFeature() tras promover error = %v", err)
		}
		if doc.Status != odd.StatusPromoted {
			t.Errorf("GetODDFeature().Status = %q tras promover, esperado %q", doc.Status, odd.StatusPromoted)
		}
	})

	t.Run("segunda promoción de la misma feature se rechaza", func(t *testing.T) {
		_, err := svc.PromoteODDFeature(ODDPromoteRequest{Feature: "gestion-inventario"})
		if !errors.Is(err, odd.ErrAlreadyPromoted) {
			t.Errorf("PromoteODDFeature() error = %v, esperado errors.Is(..., odd.ErrAlreadyPromoted)", err)
		}
	})
}

// TestCheckODDMirror cubre la tarea 6.1: CheckODDMirror delega en
// odd.CheckMirror recibiendo el Exporter como parámetro explícito, la única
// forma de instrumentar los tres estados de espejo en pruebas sin invocar el
// subproceso real "engram export" [mismo patrón que
// internal/cli/odd_status.go: runODDStatus].
func TestCheckODDMirror(t *testing.T) {
	root := t.TempDir()
	if _, err := odd.Create(root, "gestion-inventario", "2026-09-17"); err != nil {
		t.Fatalf("odd.Create() error = %v", err)
	}
	rawBytes, err := os.ReadFile(filepath.Join(root, "odd", "tasks", "gestion-inventario.md"))
	if err != nil {
		t.Fatalf("leyendo el documento vivo para el exportador falso: %v", err)
	}
	rawContent := string(rawBytes)

	svc := NewService(root)

	tests := []struct {
		name      string
		export    odd.Exporter
		wantState odd.MirrorState
	}{
		{
			name: "espejo sincronizado",
			export: func(ctx context.Context, root string) ([]odd.Observation, error) {
				return []odd.Observation{{Topic: odd.MirrorTopic("gestion-inventario"), Content: rawContent}}, nil
			},
			wantState: odd.MirrorSynced,
		},
		{
			name: "espejo divergente",
			export: func(ctx context.Context, root string) ([]odd.Observation, error) {
				return []odd.Observation{{Topic: odd.MirrorTopic("gestion-inventario"), Content: "# ODD: gestion-inventario\ncontenido distinto"}}, nil
			},
			wantState: odd.MirrorDiverged,
		},
		{
			name: "exportador con error se proyecta a no disponible",
			export: func(ctx context.Context, root string) ([]odd.Observation, error) {
				return nil, errors.New("engram no responde")
			},
			wantState: odd.MirrorUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := svc.CheckODDMirror(context.Background(), ODDMirrorRequest{Feature: "gestion-inventario"}, tt.export)
			if err != nil {
				t.Fatalf("CheckODDMirror() error = %v", err)
			}
			if report.State != tt.wantState {
				t.Errorf("CheckODDMirror().State = %q, esperado %q", report.State, tt.wantState)
			}
		})
	}

	t.Run("feature inexistente devuelve error sin invocar el exportador", func(t *testing.T) {
		invoked := false
		fake := func(ctx context.Context, root string) ([]odd.Observation, error) {
			invoked = true
			return nil, nil
		}

		_, err := svc.CheckODDMirror(context.Background(), ODDMirrorRequest{Feature: "no-existe"}, fake)
		if !errors.Is(err, odd.ErrFeatureNotFound) {
			t.Errorf("CheckODDMirror() error = %v, esperado errors.Is(..., odd.ErrFeatureNotFound)", err)
		}
		if invoked {
			t.Error("CheckODDMirror() invocó el Exporter para una feature inexistente; no debía hacerlo")
		}
	})

	t.Run("feature vacía se rechaza antes de tocar el dominio", func(t *testing.T) {
		if _, err := svc.CheckODDMirror(context.Background(), ODDMirrorRequest{Feature: "  "}, odd.DefaultExporter); err == nil {
			t.Error("CheckODDMirror() esperaba error para feature vacía, obtenido nil")
		}
	})
}

// TestODDRequestDTOsUnmarshalFromJSON cubre la tarea 6.3: ODDCreateRequest,
// ODDPromoteRequest y ODDMirrorRequest deserializan correctamente desde el
// JSON de entrada que la capa HTTP (server.go) decodifica en cada ruta
// [D-15].
func TestODDRequestDTOsUnmarshalFromJSON(t *testing.T) {
	t.Run("ODDCreateRequest", func(t *testing.T) {
		var got ODDCreateRequest
		if err := json.Unmarshal([]byte(`{"feature":"gestion-inventario"}`), &got); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if got.Feature != "gestion-inventario" {
			t.Errorf("ODDCreateRequest.Feature = %q, esperado %q", got.Feature, "gestion-inventario")
		}
	})

	t.Run("ODDPromoteRequest", func(t *testing.T) {
		var got ODDPromoteRequest
		if err := json.Unmarshal([]byte(`{"feature":"gestion-inventario","name":"otro-nombre","dry_run":true}`), &got); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		want := ODDPromoteRequest{Feature: "gestion-inventario", Name: "otro-nombre", DryRun: true}
		if got != want {
			t.Errorf("ODDPromoteRequest = %+v, esperado %+v", got, want)
		}
	})

	t.Run("ODDMirrorRequest", func(t *testing.T) {
		var got ODDMirrorRequest
		if err := json.Unmarshal([]byte(`{"feature":"gestion-inventario"}`), &got); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if got.Feature != "gestion-inventario" {
			t.Errorf("ODDMirrorRequest.Feature = %q, esperado %q", got.Feature, "gestion-inventario")
		}
	})
}
