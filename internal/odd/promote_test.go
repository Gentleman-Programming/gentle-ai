package odd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/multirole"
)

// promotedBodyHeadings enumera, en el orden exacto en que RenderProposalBody
// las escribe [diseño §5.8], las cabeceras del cuerpo sembrado que separan
// una sección de la siguiente. Se fija aquí de forma literal, independiente
// de la implementación de producción, para que sectionBody sea un oráculo
// real y no un espejo de RenderProposalBody (mismo principio que
// TestRenderNew en template_test.go).
var promotedBodyHeadings = []string{
	"## Propósito (Intent)",
	"### Problema",
	"### Por qué ahora",
	"### Dentro de Alcance (In Scope)",
	"#### Alcance autorizado heredado",
	"### Fuera de Alcance (Out of Scope)",
	"#### Restricciones heredadas",
	"## Capacidades (Capabilities)",
	"## Enfoque (Approach)",
	"## Criterios de Éxito",
	"### Comprobaciones aplicables",
	"## Estado heredado de ODD",
	"### Evidencia de verificación observada",
	"### Siguiente paso declarado en ODD",
}

// sectionBody devuelve, dentro de body, el texto recortado entre la cabecera
// heading y la cabecera conocida más próxima que aparezca después de ella (o
// el final de body si heading es la última). t.Fatalf si heading no aparece
// en body.
func sectionBody(t *testing.T, body, heading string) string {
	t.Helper()

	start := strings.Index(body, heading)
	if start == -1 {
		t.Fatalf("el cuerpo sembrado no contiene la cabecera %q", heading)
	}
	start += len(heading)

	end := len(body)
	for _, candidate := range promotedBodyHeadings {
		if candidate == heading {
			continue
		}
		idx := strings.Index(body[start:], candidate)
		if idx != -1 && start+idx < end {
			end = start + idx
		}
	}

	return strings.TrimSpace(body[start:end])
}

// sectionDestinations es el mapeo origen → destino de §5.8 del diseño para
// las diez secciones canónicas que RenderProposalBody transporta de forma
// directa a un destino con origen real. Excluye deliberadamente tres
// destinos sin origen declarado en el documento ODD: Capacidades [D-09],
// Enfoque y Estado heredado de ODD (derivan de Tasks, no de Sections), y
// Fuera de Alcance. Este último es el caso relevante: Alcance se transporta
// únicamente a "Dentro de Alcance", nunca también a "Fuera de Alcance",
// porque el documento ODD no declara en ningún sitio qué queda excluido, y
// duplicar el mismo texto en ambos destinos no transportaría esa ausencia
// con fidelidad — afirmaría una contradicción (el mismo contenido entra y
// no entra en el alcance a la vez) que el renderizador habría fabricado.
// Ese comportamiento —"Fuera de Alcance" recibe siempre
// pendienteContenidoTexto— se comprueba aparte, no en esta tabla, en
// "cada sección vacía..." y en el test-guarda dedicado más abajo
// [D-09, frontera T-9: nunca fabricar, nunca sanear].
var sectionDestinations = []struct {
	name    string
	section SectionID
	heading string
}{
	{name: "Objetivo -> Propósito (Intent)", section: SectionObjective, heading: "## Propósito (Intent)"},
	{name: "Problema -> Problema", section: SectionProblem, heading: "### Problema"},
	{name: "Porqué -> Por qué ahora", section: SectionWhy, heading: "### Por qué ahora"},
	{name: "Alcance -> Dentro de Alcance (In Scope)", section: SectionScope, heading: "### Dentro de Alcance (In Scope)"},
	{name: "Alcance autorizado -> Alcance autorizado heredado", section: SectionAuthorizedScope, heading: "#### Alcance autorizado heredado"},
	{name: "Restricciones -> Restricciones heredadas", section: SectionConstraints, heading: "#### Restricciones heredadas"},
	{name: "Criterios de aceptación -> Criterios de Éxito", section: SectionAcceptance, heading: "## Criterios de Éxito"},
	{name: "Comprobaciones aplicables -> Comprobaciones aplicables", section: SectionChecks, heading: "### Comprobaciones aplicables"},
	{name: "Evidencia de verificación -> Evidencia de verificación observada", section: SectionEvidence, heading: "### Evidencia de verificación observada"},
	{name: "Siguiente paso -> Siguiente paso declarado en ODD", section: SectionNextStep, heading: "### Siguiente paso declarado en ODD"},
}

// fullyPopulatedODDDocument construye un documento vivo con contenido
// adversarial deliberado en cada sección: Markdown con negrita, código en
// línea y comillas, para que las aserciones de fidelidad de bytes tengan
// algo real que perder si RenderProposalBody saneara o reescribiera el
// contenido [frontera T-9]. El checklist mezcla tareas completadas y
// pendientes con identificadores estables, y menciona un nombre de
// componente técnico ("SAP-Lite") para confirmar que ese lenguaje técnico no
// se reinterpreta como una capacidad (REQ-19.11).
func fullyPopulatedODDDocument() *Document {
	return &Document{
		Feature: "gestion-inventario",
		Path:    "odd/tasks/gestion-inventario.md",
		// Estado deliberadamente contradictorio: este documento ya afirma
		// estar promovido a otro cambio distinto del que se está generando
		// en la prueba. La cabecera de procedencia del cuerpo sembrado debe
		// ignorar esta contradicción por completo.
		Status:     StatusPromoted,
		PromotedTo: "openspec/changes/otro-cambio-previo/",
		PromotedAt: "2020-01-01",
		Sections: map[SectionID]string{
			SectionObjective:       "Reducir el tiempo de conciliación de inventario de 3 días a 1 hora.",
			SectionProblem:         "El conteo manual con `hoja-de-calculo.xlsx` diverge del almacén real cada cierre de mes.",
			SectionWhy:             "El cierre de mes de octubre reveló una discrepancia de **12.000 €** en existencias.",
			SectionScope:           "Módulo de conciliación automática contra el ERP `SAP-Lite` y su reporte diario.",
			SectionConstraints:     "No se sustituye el ERP existente; sólo se añade un módulo de lectura.",
			SectionAuthorizedScope: "Autorizado explícitamente por Dirección de Operaciones el 2026-09-01.",
			SectionAcceptance:      "El informe de conciliación coincide con el conteo físico en un 99.5% de las referencias.",
			SectionChecks:          "`go test ./internal/inventario/...` y una conciliación manual de control sobre 50 SKU.",
			SectionEvidence:        "Ejecutado 12 veces en staging; 3 divergencias, todas explicadas por mermas registradas.",
			SectionNextStep:        "Desplegar en producción tras la aprobación de Finanzas.",
		},
		Tasks: []Task{
			{ID: "T1", Text: "Diseñar el esquema de conciliación contra `SAP-Lite`.", Done: true},
			{ID: "T2", Text: "Implementar el job nocturno de comparación.", Done: true},
			{ID: "T3", Text: "Instrumentar alertas ante discrepancia > 1%.", Done: false},
		},
		Progress: multirole.RoleTaskProgress{Total: 3, Completed: 2, Pending: 1, Percent: 66.67},
	}
}

// emptyODDDocument construye un documento vivo cuyas doce secciones están
// vacías y sin ninguna tarea de checklist: el caso adversarial contrario a
// fullyPopulatedODDDocument, para confirmar que ninguna sección vacía
// produce texto inventado.
func emptyODDDocument() *Document {
	return &Document{
		Feature:  "modulo-vacio",
		Path:     "odd/tasks/modulo-vacio.md",
		Status:   StatusActive,
		Sections: map[SectionID]string{},
		Tasks:    nil,
		Progress: multirole.RoleTaskProgress{},
	}
}

// TestRenderProposalBody cubre la tarea 5.1: el mapeo determinista de un
// documento vivo ODD al cuerpo sembrado de un proposal.md (REQ-19.9,
// REQ-19.11, diseño §5.8, D-09, frontera T-9).
func TestRenderProposalBody(t *testing.T) {
	t.Run("cada sección no vacía se transporta verbatim a su destino, sin sanear ni reescribir Markdown", func(t *testing.T) {
		doc := fullyPopulatedODDDocument()
		got := RenderProposalBody(doc, "modulo-inventario-v2", "2026-09-18")

		for _, dest := range sectionDestinations {
			want := doc.Sections[dest.section]
			gotBody := sectionBody(t, got, dest.heading)
			if gotBody != want {
				t.Errorf("%s: cuerpo de %q = %q, se esperaba el texto de origen íntegro %q", dest.name, dest.heading, gotBody, want)
			}
		}
	})

	t.Run("cada sección vacía del documento origen produce el marcador de contenido pendiente", func(t *testing.T) {
		doc := emptyODDDocument()
		got := RenderProposalBody(doc, "modulo-vacio", "2026-09-18")

		for _, dest := range sectionDestinations {
			gotBody := sectionBody(t, got, dest.heading)
			if gotBody != pendienteContenidoTexto {
				t.Errorf("%s: cuerpo de %q = %q, se esperaba el marcador %q", dest.name, dest.heading, gotBody, pendienteContenidoTexto)
			}
		}

		if gotEnfoque := sectionBody(t, got, "## Enfoque (Approach)"); gotEnfoque != pendienteContenidoTexto {
			t.Errorf("Enfoque (Approach) = %q, se esperaba el marcador %q al no haber tareas de checklist", gotEnfoque, pendienteContenidoTexto)
		}

		if gotFuera := sectionBody(t, got, "### Fuera de Alcance (Out of Scope)"); gotFuera != pendienteContenidoTexto {
			t.Errorf("Fuera de Alcance (Out of Scope) = %q, se esperaba el marcador %q: el documento ODD no declara qué queda excluido del alcance", gotFuera, pendienteContenidoTexto)
		}
	})

	t.Run("Alcance no vacío no se duplica en Fuera de Alcance: el documento ODD no declara qué queda excluido", func(t *testing.T) {
		// Test-guarda: fija la decisión de que Alcance se transporta
		// EXACTAMENTE UNA VEZ (a "Dentro de Alcance") y nunca también a
		// "Fuera de Alcance". Duplicarlo no transportaría con fidelidad la
		// ausencia de una declaración de exclusión en el documento de
		// origen: afirmaría que el mismo contenido entra y no entra en el
		// alcance a la vez, una contradicción fabricada por el
		// renderizador. Si alguien reintroduce sectionOrPlaceholder(doc,
		// SectionScope) en la rama de "Fuera de Alcance" creyendo que
		// rellena un hueco, este test debe fallar.
		doc := fullyPopulatedODDDocument()
		got := RenderProposalBody(doc, "modulo-inventario-v2", "2026-09-18")

		alcance := doc.Sections[SectionScope]
		if count := strings.Count(got, alcance); count != 1 {
			t.Errorf("el texto de Alcance (%q) aparece %d veces en el cuerpo sembrado, se esperaba exactamente 1", alcance, count)
		}

		gotDentro := sectionBody(t, got, "### Dentro de Alcance (In Scope)")
		if gotDentro != alcance {
			t.Errorf("Dentro de Alcance (In Scope) = %q, se esperaba el texto íntegro de Alcance %q", gotDentro, alcance)
		}

		gotFuera := sectionBody(t, got, "### Fuera de Alcance (Out of Scope)")
		if gotFuera != pendienteContenidoTexto {
			t.Errorf("Fuera de Alcance (Out of Scope) = %q, se esperaba el marcador de pendiente %q en vez del texto de Alcance duplicado", gotFuera, pendienteContenidoTexto)
		}
	})

	t.Run("Capacidades contiene únicamente el marcador de pendiente para sdd-spec, nunca una capacidad inventada", func(t *testing.T) {
		doc := fullyPopulatedODDDocument()
		got := RenderProposalBody(doc, "modulo-inventario-v2", "2026-09-18")

		gotCapacidades := sectionBody(t, got, "## Capacidades (Capabilities)")
		if gotCapacidades != pendienteCapacidadesTexto {
			t.Errorf("Capacidades = %q, se esperaba únicamente el marcador de pendiente %q", gotCapacidades, pendienteCapacidadesTexto)
		}

		for _, invented := range []string{"SAP-Lite", "conciliación", "Diseñar el esquema"} {
			if strings.Contains(gotCapacidades, invented) {
				t.Errorf("Capacidades contiene %q, filtrado del checklist/alcance: la promoción no debe derivar capacidades del documento ODD", invented)
			}
		}
	})

	t.Run("la cabecera de procedencia está siempre presente, incluso cuando el documento origen la contradice", func(t *testing.T) {
		doc := fullyPopulatedODDDocument() // ya afirma Status=promovido a otro cambio distinto
		got := RenderProposalBody(doc, "modulo-inventario-v2", "2026-09-18")

		if !strings.Contains(got, "sembrado") {
			t.Errorf("el cuerpo sembrado no declara en ningún punto que procede de una promoción ODD (falta un aviso de documento sembrado)")
		}
		if !strings.Contains(got, "odd/tasks/gestion-inventario.md") {
			t.Errorf("el cuerpo sembrado no contiene la ruta de procedencia exacta %q", "odd/tasks/gestion-inventario.md")
		}
		if !strings.Contains(got, "2026-09-18") {
			t.Errorf("el cuerpo sembrado no contiene la fecha de promoción %q pasada como parámetro", "2026-09-18")
		}
		// El documento de origen afirma internamente PromotedTo/PromotedAt
		// distintos: la cabecera de procedencia nunca debe reflejarlos.
		if strings.Contains(got, "otro-cambio-previo") {
			t.Errorf("el cuerpo sembrado refleja la referencia de promoción previa y contradictoria del documento de origen (%q); la cabecera de procedencia debe ignorarla", doc.PromotedTo)
		}
		if strings.Contains(got, "2020-01-01") {
			t.Errorf("el cuerpo sembrado refleja la fecha de promoción previa y contradictoria del documento de origen (%q); la cabecera de procedencia debe ignorarla", doc.PromotedAt)
		}
	})

	t.Run(`la tabla "Estado heredado de ODD" refleja el estado real de cada tarea del checklist`, func(t *testing.T) {
		doc := fullyPopulatedODDDocument()
		got := RenderProposalBody(doc, "modulo-inventario-v2", "2026-09-18")

		gotEstado := sectionBody(t, got, "## Estado heredado de ODD")

		tests := []struct {
			name string
			task Task
		}{
			{name: "T1 completada", task: doc.Tasks[0]},
			{name: "T2 completada", task: doc.Tasks[1]},
			{name: "T3 pendiente", task: doc.Tasks[2]},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				wantMark := "- [ ]"
				if tt.task.Done {
					wantMark = "- [x]"
				}
				wantRow := "| [" + tt.task.ID + "] " + tt.task.Text + " | " + wantMark + " |"
				if !strings.Contains(gotEstado, wantRow) {
					t.Errorf("el apéndice de estado heredado no contiene la fila %q para la tarea %s", wantRow, tt.task.ID)
				}
			})
		}
	})

	t.Run("checklist vacío produce una tabla vacía con nota, sin división por cero", func(t *testing.T) {
		doc := emptyODDDocument()
		got := RenderProposalBody(doc, "modulo-vacio", "2026-09-18")

		gotEstado := sectionBody(t, got, "## Estado heredado de ODD")
		if !strings.Contains(gotEstado, "0/0") {
			t.Errorf("el apéndice de estado heredado con checklist vacío = %q, se esperaba una referencia a 0/0 tareas sin división por cero", gotEstado)
		}
		if strings.Contains(gotEstado, "- [x]") || strings.Contains(gotEstado, "- [ ]") {
			t.Errorf("el apéndice de estado heredado con checklist vacío contiene una fila de tarea fabricada: %q", gotEstado)
		}
	})
}

// fakeScaffolder es un Scaffolder de prueba instrumentado: registra cada
// invocación recibida en calls y devuelve result/err de forma fija, sin
// tocar el sistema de ficheros real. Permite comprobar el ciclo de dos
// fases de Promote sin depender de dashboard.Service.CreateIncrement
// [D-01, D-07].
type fakeScaffolder struct {
	result ScaffoldResult
	err    error
	calls  []ScaffoldRequest
}

func (f *fakeScaffolder) Scaffold(req ScaffoldRequest) (ScaffoldResult, error) {
	f.calls = append(f.calls, req)
	return f.result, f.err
}

// TestFormatMarkFailureWarning fija, de forma determinista y sin depender
// de ningún comportamiento real del sistema de ficheros, el mensaje de
// remediación exacto de D-07 que Promote debe producir cuando el cambio SDD
// ya se creó pero la marca de promoción no se pudo escribir.
func TestFormatMarkFailureWarning(t *testing.T) {
	got := formatMarkFailureWarning("modulo-inventario-v2", "gestion-inventario", errors.New("disco lleno"))
	want := "Aviso: el cambio SDD se creó en openspec/changes/modulo-inventario-v2/ pero no se pudo marcar " +
		"odd/tasks/gestion-inventario.md como promovido (disco lleno). Añade manualmente la línea " +
		"«> **Promovido a:** `openspec/changes/modulo-inventario-v2/`» o elimina el directorio del cambio."

	if got != want {
		t.Errorf("formatMarkFailureWarning() = %q, se esperaba %q", got, want)
	}
}

// TestPromote cubre la tarea 5.3: el ciclo de promoción de dos fases —
// validar, renderizar, Scaffold, MarkPromoted, nunca al revés [D-07]— con
// un Scaffolder falso instrumentado que nunca toca openspec/ de verdad.
func TestPromote(t *testing.T) {
	t.Run("documento ya promovido devuelve ErrAlreadyPromoted sin invocar al Scaffolder", func(t *testing.T) {
		root := t.TempDir()
		const feature = "gestion-inventario"

		doc, err := Create(root, feature, "2026-09-17")
		if err != nil {
			t.Fatalf("Create() devolvió error inesperado: %v", err)
		}
		if err := MarkPromoted(root, doc, "modulo-inventario-v1", "2026-09-17"); err != nil {
			t.Fatalf("MarkPromoted() devolvió error inesperado: %v", err)
		}

		sc := &fakeScaffolder{result: ScaffoldResult{Name: "modulo-inventario-v2", Path: "openspec/changes/modulo-inventario-v2/proposal.md"}}

		result, err := Promote(PromoteOptions{Root: root, Feature: feature, ChangeName: "modulo-inventario-v2", Today: "2026-09-18"}, sc)
		if err == nil {
			t.Fatalf("Promote() = nil error, se esperaba ErrAlreadyPromoted")
		}
		if !errors.Is(err, ErrAlreadyPromoted) {
			t.Errorf("Promote() = %v, no envuelve ErrAlreadyPromoted", err)
		}
		if result != nil {
			t.Errorf("Promote() devolvió un PromoteResult no nulo junto al error ErrAlreadyPromoted: %+v", result)
		}
		if len(sc.calls) != 0 {
			t.Errorf("Scaffolder invocado %d veces, se esperaba 0 (documento ya promovido) [D-08]", len(sc.calls))
		}
	})

	t.Run("nombre de cambio inválido rechaza la promoción sin invocar al Scaffolder", func(t *testing.T) {
		root := t.TempDir()
		const feature = "gestion-inventario"

		if _, err := Create(root, feature, "2026-09-17"); err != nil {
			t.Fatalf("Create() devolvió error inesperado: %v", err)
		}

		sc := &fakeScaffolder{result: ScaffoldResult{Name: "irrelevante", Path: "irrelevante"}}

		result, err := Promote(PromoteOptions{Root: root, Feature: feature, ChangeName: "Nombre Invalido", Today: "2026-09-18"}, sc)
		if err == nil {
			t.Fatalf("Promote() = nil error, se esperaba el rechazo de ValidateFeatureName")
		}
		if !errors.Is(err, ErrInvalidFeatureName) {
			t.Errorf("Promote() = %v, no envuelve ErrInvalidFeatureName", err)
		}
		if result != nil {
			t.Errorf("Promote() devolvió un PromoteResult no nulo junto a un nombre de cambio inválido: %+v", result)
		}
		if len(sc.calls) != 0 {
			t.Errorf("Scaffolder invocado %d veces, se esperaba 0 (nombre de cambio inválido) [D-07: validar antes de Scaffold]", len(sc.calls))
		}
	})

	t.Run("--dry-run no invoca al Scaffolder ni escribe nada", func(t *testing.T) {
		root := t.TempDir()
		const feature = "gestion-inventario"

		if _, err := Create(root, feature, "2026-09-17"); err != nil {
			t.Fatalf("Create() devolvió error inesperado: %v", err)
		}
		path, err := DocumentPath(root, feature)
		if err != nil {
			t.Fatalf("DocumentPath() devolvió error inesperado: %v", err)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no se pudo leer el documento antes de Promote(): %v", err)
		}

		sc := &fakeScaffolder{result: ScaffoldResult{Name: "modulo-inventario-v2", Path: "openspec/changes/modulo-inventario-v2/proposal.md"}}

		result, err := Promote(PromoteOptions{Root: root, Feature: feature, ChangeName: "modulo-inventario-v2", DryRun: true, Today: "2026-09-18"}, sc)
		if err != nil {
			t.Fatalf("Promote() con --dry-run devolvió error inesperado: %v", err)
		}
		if !result.DryRun {
			t.Errorf("PromoteResult.DryRun = false, se esperaba true")
		}
		if result.Body == "" {
			t.Errorf("PromoteResult.Body está vacío, se esperaba el cuerpo sembrado previsualizado")
		}
		if len(sc.calls) != 0 {
			t.Errorf("Scaffolder invocado %d veces, se esperaba 0 (--dry-run no escribe nada)", len(sc.calls))
		}

		if _, statErr := os.Stat(filepath.Join(root, "openspec")); !os.IsNotExist(statErr) {
			t.Errorf("--dry-run creó %q (err=%v), se esperaba que no existiera ningún directorio openspec/", filepath.Join(root, "openspec"), statErr)
		}

		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no se pudo leer el documento después de Promote(): %v", err)
		}
		if string(before) != string(after) {
			t.Errorf("--dry-run modificó el documento ODD; se esperaba que permaneciera intacto")
		}
	})

	t.Run("Scaffolder que falla deja el documento ODD intacto, sin marca de promoción", func(t *testing.T) {
		root := t.TempDir()
		const feature = "gestion-inventario"

		if _, err := Create(root, feature, "2026-09-17"); err != nil {
			t.Fatalf("Create() devolvió error inesperado: %v", err)
		}
		path, err := DocumentPath(root, feature)
		if err != nil {
			t.Fatalf("DocumentPath() devolvió error inesperado: %v", err)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no se pudo leer el documento antes de Promote(): %v", err)
		}

		scaffoldErr := errors.New("colisión simulada: ya existe un cambio activo con ese nombre")
		sc := &fakeScaffolder{err: scaffoldErr}

		result, err := Promote(PromoteOptions{Root: root, Feature: feature, ChangeName: "modulo-inventario-v2", Today: "2026-09-18"}, sc)
		if err == nil {
			t.Fatalf("Promote() = nil error, se esperaba el error del Scaffolder propagado")
		}
		if result != nil {
			t.Errorf("Promote() devolvió un PromoteResult no nulo tras un Scaffolder fallido: %+v", result)
		}
		if len(sc.calls) != 1 {
			t.Fatalf("Scaffolder invocado %d veces, se esperaba exactamente 1", len(sc.calls))
		}

		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no se pudo leer el documento después de Promote(): %v", err)
		}
		if string(before) != string(after) {
			t.Errorf("el documento ODD se modificó tras un Scaffolder fallido; se esperaba que permaneciera intacto")
		}
	})

	t.Run("ciclo completo exitoso: valida, renderiza, invoca Scaffold y marca el documento como promovido", func(t *testing.T) {
		root := t.TempDir()
		const feature = "gestion-inventario"
		const changeName = "modulo-inventario-v2"

		if _, err := Create(root, feature, "2026-09-17"); err != nil {
			t.Fatalf("Create() devolvió error inesperado: %v", err)
		}

		sc := &fakeScaffolder{result: ScaffoldResult{Name: changeName, Path: "openspec/changes/" + changeName + "/proposal.md"}}

		result, err := Promote(PromoteOptions{
			Root: root, Feature: feature, ChangeName: changeName,
			Intent: "Reducir el tiempo de conciliación", Type: "feature",
			Today: "2026-09-18",
		}, sc)
		if err != nil {
			t.Fatalf("Promote() devolvió error inesperado: %v", err)
		}
		if len(sc.calls) != 1 {
			t.Fatalf("Scaffolder invocado %d veces, se esperaba exactamente 1", len(sc.calls))
		}

		gotCall := sc.calls[0]
		if gotCall.Name != changeName {
			t.Errorf("ScaffoldRequest.Name = %q, se esperaba %q", gotCall.Name, changeName)
		}
		if gotCall.Intent != "Reducir el tiempo de conciliación" {
			t.Errorf("ScaffoldRequest.Intent = %q, no se reenvió tal cual", gotCall.Intent)
		}
		if gotCall.Type != "feature" {
			t.Errorf("ScaffoldRequest.Type = %q, no se reenvió tal cual", gotCall.Type)
		}
		if gotCall.ProposalBody == "" {
			t.Errorf("ScaffoldRequest.ProposalBody está vacío, se esperaba el cuerpo renderizado")
		}

		if !result.MarkWritten {
			t.Errorf("MarkWritten = false, se esperaba true tras una promoción exitosa")
		}
		if result.Warning != "" {
			t.Errorf("Warning = %q, se esperaba vacío en el camino feliz", result.Warning)
		}
		if result.ProposalPath != sc.result.Path {
			t.Errorf("ProposalPath = %q, se esperaba %q", result.ProposalPath, sc.result.Path)
		}
		if result.MirrorNote == "" {
			t.Errorf("MirrorNote está vacío, se esperaba la nota de espejo pendiente incluso en el camino feliz")
		}

		reloaded, err := Load(root, feature)
		if err != nil {
			t.Fatalf("Load() tras Promote() devolvió error inesperado: %v", err)
		}
		if reloaded.Status != StatusPromoted {
			t.Errorf("Status = %q tras Promote(), se esperaba %q", reloaded.Status, StatusPromoted)
		}
		wantPromotedTo := "openspec/changes/" + changeName + "/"
		if reloaded.PromotedTo != wantPromotedTo {
			t.Errorf("PromotedTo = %q, se esperaba %q", reloaded.PromotedTo, wantPromotedTo)
		}
	})

	t.Run("fallo al escribir la marca tras una creación exitosa produce Warning y MarkWritten en false", func(t *testing.T) {
		root := t.TempDir()
		const feature = "gestion-inventario"
		const changeName = "modulo-inventario-v2"

		if _, err := Create(root, feature, "2026-09-17"); err != nil {
			t.Fatalf("Create() devolvió error inesperado: %v", err)
		}
		path, err := DocumentPath(root, feature)
		if err != nil {
			t.Fatalf("DocumentPath() devolvió error inesperado: %v", err)
		}
		if err := os.Chmod(path, 0o444); err != nil {
			t.Fatalf("no se pudo marcar el documento como solo lectura para simular el fallo de escritura: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

		sc := &fakeScaffolder{result: ScaffoldResult{Name: changeName, Path: "openspec/changes/" + changeName + "/proposal.md"}}

		result, err := Promote(PromoteOptions{Root: root, Feature: feature, ChangeName: changeName, Today: "2026-09-18"}, sc)
		if err != nil {
			t.Fatalf("Promote() devolvió error inesperado: %v; se esperaba un *PromoteResult con Warning poblado en vez de un error", err)
		}
		if len(sc.calls) != 1 {
			t.Fatalf("Scaffolder invocado %d veces, se esperaba exactamente 1 (la creación debe intentarse antes de marcar) [D-07]", len(sc.calls))
		}
		if result.MarkWritten {
			t.Errorf("MarkWritten = true, se esperaba false tras el fallo simulado de escritura de la marca")
		}
		if result.Warning == "" {
			t.Fatalf("Warning está vacío, se esperaba el mensaje de remediación de D-07")
		}

		wantPrefix := fmt.Sprintf("Aviso: el cambio SDD se creó en openspec/changes/%s/ pero no se pudo marcar odd/tasks/%s.md como promovido (", changeName, feature)
		wantSuffix := fmt.Sprintf("). Añade manualmente la línea «> **Promovido a:** `openspec/changes/%s/`» o elimina el directorio del cambio.", changeName)
		if !strings.HasPrefix(result.Warning, wantPrefix) {
			t.Errorf("Warning = %q, no empieza por el prefijo de remediación esperado %q", result.Warning, wantPrefix)
		}
		if !strings.HasSuffix(result.Warning, wantSuffix) {
			t.Errorf("Warning = %q, no termina en el sufijo de remediación esperado %q", result.Warning, wantSuffix)
		}
	})
}
