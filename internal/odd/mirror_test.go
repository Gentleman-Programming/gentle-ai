package odd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mirrorSubprocessGuardBound acota, en los tests de esta frontera, cuánto
// puede tardar una llamada que se espera falle de inmediato (binario
// ausente o contexto ya cancelado). Es deliberadamente mucho menor que el
// timeout real de 10 s de DefaultExporter: si la llamada tardara tanto como
// ese timeout, no estaría fallando de inmediato, estaría bloqueando.
const mirrorSubprocessGuardBound = 5 * time.Second

// fakeExporter construye un Exporter de prueba que devuelve observations y
// err de forma fija, sin inspeccionar ctx ni root. Permite instrumentar cada
// desenlace de CheckMirror sin invocar un subproceso real.
func fakeExporter(observations []Observation, err error) Exporter {
	return func(context.Context, string) ([]Observation, error) {
		return observations, err
	}
}

// TestCheckMirror cubre los seis desenlaces de comparar un documento vivo
// contra su espejo de recuperación en Engram: sincronizado, divergente, y
// los cuatro caminos que proyectan "no disponible" con una causa distinta y
// descriptiva cada uno [D-05].
func TestCheckMirror(t *testing.T) {
	const feature = "gestion-inventario"
	raw := "# ODD: gestion-inventario\n\n> **Estado:** activo\n\n## Objetivo\n\nTexto de ejemplo.\n"
	diverged := "# ODD: gestion-inventario\n\n> **Estado:** activo\n\n## Objetivo\n\nTexto distinto.\n"
	doc := &Document{Feature: feature, Raw: raw}
	topic := MirrorTopic(feature)

	// noisyBody reproduce exactamente el mismo contenido que raw, pero con
	// saltos de línea CRLF y espacios finales en cada línea, precedido de un
	// preámbulo ajeno al documento (el que Engram añade a su propia
	// observación) y seguido de líneas en blanco terminales adicionales. Tras
	// normalizar ambos lados, debe seguir comparando igual que raw.
	noisyBody := strings.ReplaceAll(raw, "\n", "  \r\n")
	noisyContent := "Preámbulo simulado del agente que guarda en Engram.\n\n" + noisyBody + "\r\n\r\n   \r\n"

	exportErr := errors.New("fallo simulado del transporte")

	tests := []struct {
		name       string
		export     Exporter
		wantState  MirrorState
		wantReason string // sólo aplica cuando wantState == MirrorUnavailable
	}{
		{
			name:      "sincronizado tras normalizar CRLF, espacios finales y preámbulo ajeno",
			export:    fakeExporter([]Observation{{Topic: topic, Content: noisyContent}}, nil),
			wantState: MirrorSynced,
		},
		{
			name:      "divergente cuando el contenido normalizado difiere",
			export:    fakeExporter([]Observation{{Topic: topic, Content: diverged}}, nil),
			wantState: MirrorDiverged,
		},
		{
			name:       "no disponible cuando el exportador devuelve error",
			export:     fakeExporter(nil, exportErr),
			wantState:  MirrorUnavailable,
			wantReason: "no se pudo exportar el espejo Engram: fallo simulado del transporte",
		},
		{
			name:       "no disponible cuando el exportador devuelve una lista vacía",
			export:     fakeExporter([]Observation{}, nil),
			wantState:  MirrorUnavailable,
			wantReason: "el espejo no devolvió ninguna observación",
		},
		{
			name:       "no disponible cuando ninguna observación coincide con el topic",
			export:     fakeExporter([]Observation{{Topic: "odd/otra-feature/tasks", Content: raw}}, nil),
			wantState:  MirrorUnavailable,
			wantReason: `no se encontró ninguna observación del espejo con el topic "odd/gestion-inventario/tasks"`,
		},
		{
			name:       "no disponible cuando el topic coincide pero falta el centinela",
			export:     fakeExporter([]Observation{{Topic: topic, Content: "contenido sin cabecera reconocible"}}, nil),
			wantState:  MirrorUnavailable,
			wantReason: "el espejo no contiene un cuerpo de documento reconocible",
		},
	}

	seenReasons := make(map[string]bool)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := CheckMirror(context.Background(), t.TempDir(), doc, tt.export)

			if report.State != tt.wantState {
				t.Fatalf("State = %q, se esperaba %q", report.State, tt.wantState)
			}
			if report.Topic != topic {
				t.Errorf("Topic = %q, se esperaba %q", report.Topic, topic)
			}

			if tt.wantState != MirrorUnavailable {
				if report.Reason != "" {
					t.Errorf("Reason = %q, se esperaba vacío para el estado %q", report.Reason, tt.wantState)
				}
				return
			}

			if report.Reason != tt.wantReason {
				t.Errorf("Reason = %q, se esperaba exactamente %q", report.Reason, tt.wantReason)
			}
			if seenReasons[report.Reason] {
				t.Errorf("Reason %q ya se usó en otro caso 'no disponible'; los cuatro deben ser distintos", report.Reason)
			}
			seenReasons[report.Reason] = true
		})
	}

	if len(seenReasons) != 4 {
		t.Fatalf("se registraron %d motivos distintos para 'no disponible', se esperaban 4", len(seenReasons))
	}
}

// TestMirrorTopic confirma el formato canónico del topic de Engram bajo el
// que se busca el espejo de recuperación de una feature (REQ-19.4).
func TestMirrorTopic(t *testing.T) {
	got := MirrorTopic("gestion-inventario")
	want := "odd/gestion-inventario/tasks"
	if got != want {
		t.Errorf("MirrorTopic(%q) = %q, se esperaba %q", "gestion-inventario", got, want)
	}
}

// TestDefaultExporter cubre la mitad de la frontera T-6 de la matriz de
// amenazas que corresponde al puerto real: con el binario "engram" ausente
// del PATH, DefaultExporter debe devolver un error de inmediato —nunca
// bloquear— y no debe dejar ningún fichero temporal de exportación tras
// retornar.
func TestDefaultExporter(t *testing.T) {
	t.Run("binario ausente del PATH devuelve error sin bloquear ni dejar temporales", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		root := t.TempDir()

		start := time.Now()
		_, err := DefaultExporter(context.Background(), root)
		elapsed := time.Since(start)

		if err == nil {
			t.Fatalf("DefaultExporter() = nil error, se esperaba un fallo por binario ausente del PATH")
		}
		if elapsed >= mirrorSubprocessGuardBound {
			t.Errorf("DefaultExporter() tardó %s, se esperaba un fallo inmediato por PATH vacío, no el timeout de exportación", elapsed)
		}
		assertNoLeftoverMirrorTempFiles(t)
	})
}

// TestCheckMirror_FronteraDeSubproceso cubre, a través de CheckMirror, la
// frontera T-6 completa sobre el puerto real DefaultExporter: binario
// ausente del PATH y contexto ya cancelado antes de invocar deben
// proyectarse ambos a "no disponible" sin bloquear, y en ningún caso debe
// sobrevivir el fichero temporal de exportación tras el retorno.
func TestCheckMirror_FronteraDeSubproceso(t *testing.T) {
	doc := &Document{Feature: "gestion-inventario", Raw: "# ODD: gestion-inventario\n"}

	t.Run("PATH vacío se proyecta a no disponible sin bloquear", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		root := t.TempDir()

		start := time.Now()
		report := CheckMirror(context.Background(), root, doc, DefaultExporter)
		elapsed := time.Since(start)

		if report.State != MirrorUnavailable {
			t.Fatalf("State = %q, se esperaba %q", report.State, MirrorUnavailable)
		}
		if report.Reason == "" {
			t.Errorf("Reason está vacía, se esperaba la causa del fallo de subproceso")
		}
		if elapsed >= mirrorSubprocessGuardBound {
			t.Errorf("CheckMirror tardó %s, se esperaba un fallo inmediato por PATH vacío, no el timeout de exportación", elapsed)
		}
		assertNoLeftoverMirrorTempFiles(t)
	})

	t.Run("contexto ya cancelado se proyecta a no disponible sin bloquear", func(t *testing.T) {
		root := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		start := time.Now()
		report := CheckMirror(ctx, root, doc, DefaultExporter)
		elapsed := time.Since(start)

		if report.State != MirrorUnavailable {
			t.Fatalf("State = %q, se esperaba %q", report.State, MirrorUnavailable)
		}
		if report.Reason == "" {
			t.Errorf("Reason está vacía, se esperaba la causa del fallo de subproceso")
		}
		if elapsed >= mirrorSubprocessGuardBound {
			t.Errorf("CheckMirror tardó %s con un contexto ya cancelado, se esperaba un retorno inmediato sin bloqueo", elapsed)
		}
		assertNoLeftoverMirrorTempFiles(t)
	})
}

// TestDecodeEngramExportPayload demuestra que el decodificador interpreta
// el esquema JSON real que "engram export" escribe en su fichero de
// salida: un envoltorio con la clave "observations", cuatro campos por
// observación (title/content/project/scope) y ningún campo "topic" — nunca
// el array plano de {topic, content} que se había asumido inicialmente.
// Corrección verificada contra internal/sddstatus.exportEngramObservations
// (status.go:1077-1104), la implementación ya probada en producción para
// el mismo subproceso: mismo envoltorio, misma forma de observación. El
// topic se puebla desde Title (recortado de espacio en blanco), y las
// observaciones de ámbito "personal" se descartan.
func TestDecodeEngramExportPayload(t *testing.T) {
	const fixture = `{
		"observations": [
			{"title": "sdd/otro-cambio/proposal", "content": "irrelevante para este topic", "project": "axiom", "scope": "project"},
			{"title": "  odd/gestion-inventario/tasks  ", "content": "# ODD: gestion-inventario\n\ncontenido de ámbito project", "project": "axiom", "scope": "project"},
			{"title": "odd/gestion-inventario/tasks", "content": "cuerpo de ámbito personal, debe descartarse", "project": "axiom", "scope": "personal"},
			{"title": "notas sueltas sin forma de topic reconocible", "content": "ruido", "project": "axiom", "scope": "project"}
		]
	}`

	observations, err := decodeEngramExportPayload([]byte(fixture))
	if err != nil {
		t.Fatalf("decodeEngramExportPayload() devolvió error inesperado: %v", err)
	}

	// El envoltorio real trae 4 observaciones; la de ámbito "personal" se
	// descarta, quedan 3.
	if len(observations) != 3 {
		t.Fatalf("decodeEngramExportPayload() devolvió %d observaciones, se esperaban 3 (una de ámbito personal descartada)", len(observations))
	}

	wantTopic := "odd/gestion-inventario/tasks"
	var matched *Observation
	matches := 0
	for i := range observations {
		if observations[i].Topic == wantTopic {
			matched = &observations[i]
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("se encontraron %d observaciones con Topic %q, se esperaba exactamente 1 tras descartar el ámbito personal", matches, wantTopic)
	}
	wantContent := "# ODD: gestion-inventario\n\ncontenido de ámbito project"
	if matched.Content != wantContent {
		t.Errorf("Content = %q, se esperaba %q (la observación de ámbito project, no la personal)", matched.Content, wantContent)
	}
}

// TestCheckMirror_LimitacionConocida_ColisionEntreProyectos documenta, con
// una prueba ejecutable que falla si el comportamiento cambia en silencio,
// la limitación conocida y aceptada de esta rebanada: como internal/odd no
// importa internal/sddstatus (prohibido en este incremento) y duplicar
// aquí la inferencia de proyecto (inferEngramProject, status.go:1106 y
// siguientes) sería exceso de alcance para esta rebanada, CheckMirror no
// discrimina por Project. Sólo descarta observaciones de ámbito "personal"
// (Scope, ya cubierto por decodeEngramExportPayload) y confía en la
// especificidad del topic más el centinela de cabecera. Si dos proyectos
// distintos comparten el mismo nombre de feature, sus observaciones
// colisionan bajo el mismo topic "odd/<feature>/tasks", y CheckMirror
// compara contra la que aparezca primero en la respuesta del exportador,
// sin ningún criterio de desempate adicional. A confirmar o endurecer en
// la rebanada de la CLI (Fase 3) si la colisión resulta observable en un
// workspace real.
func TestCheckMirror_LimitacionConocida_ColisionEntreProyectos(t *testing.T) {
	const feature = "gestion-inventario"
	raw := "# ODD: gestion-inventario\n\n> **Estado:** activo\n\n## Objetivo\n\nTexto de este proyecto.\n"
	doc := &Document{Feature: feature, Raw: raw}
	topic := MirrorTopic(feature)

	// ajenoContent simula la observación de un proyecto sin relación que,
	// por coincidencia de nombre de feature, comparte el mismo topic y
	// aparece primero en la respuesta del exportador.
	ajenoContent := "# ODD: gestion-inventario\n\n> **Estado:** activo\n\n## Objetivo\n\nTexto de un proyecto totalmente distinto.\n"

	export := fakeExporter([]Observation{
		{Topic: topic, Content: ajenoContent},
		{Topic: topic, Content: raw},
	}, nil)

	report := CheckMirror(context.Background(), t.TempDir(), doc, export)

	if report.State != MirrorDiverged {
		t.Fatalf("State = %q, se esperaba %q — limitación conocida: CheckMirror compara contra la primera observación que coincide por topic, sin discriminar por proyecto; si esta prueba pasa a esperar %q sin más cambios, la colisión entre proyectos dejó de estar cubierta", report.State, MirrorDiverged, MirrorSynced)
	}
}

// assertNoLeftoverMirrorTempFiles confirma que ningún fichero temporal de
// exportación del espejo (creado por DefaultExporter) sobrevive tras el
// retorno de la llamada que se acaba de ejecutar.
func assertNoLeftoverMirrorTempFiles(t *testing.T) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "odd-mirror-export-*.json"))
	if err != nil {
		t.Fatalf("no se pudo listar el directorio temporal del sistema: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("quedaron %d fichero(s) temporal(es) de exportación sin eliminar: %v", len(matches), matches)
	}
}
