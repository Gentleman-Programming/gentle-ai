package odd

import (
	"fmt"
	"strings"
)

// pendienteContenidoTexto es el marcador que RenderProposalBody escribe en
// el destino de cualquier sección del documento ODD de origen que esté
// vacía. La promoción nunca fabrica contenido que el documento de origen no
// respalda: un hueco marcado es información, un hueco rellenado con
// plausibilidad es desinformación [D-09, frontera T-9].
const pendienteContenidoTexto = "_(sin contenido en el documento ODD de origen)_"

// pendienteCapacidadesTexto es el contenido íntegro y fijo de la sección
// "## Capacidades" del cuerpo sembrado. La promoción ODD nunca deriva
// capacidades del checklist ni de ninguna otra sección del documento de
// origen: un documento ágil no las declara, y fabricarlas produciría rigor
// aparente que la fase sdd-spec tomaría por bueno [D-09, REQ-19.11].
const pendienteCapacidadesTexto = "> **Pendiente para `sdd-spec`.** La promoción ODD no deriva capacidades: un documento\n" +
	"> ágil no las declara, y fabricarlas produciría rigor aparente. Esta sección debe\n" +
	"> redactarse en la fase de especificación antes de continuar."

// checklistSinTareasTexto sustituye a las filas de la tabla "Estado
// heredado de ODD" cuando el documento de origen no registra ninguna tarea
// de checklist: una tabla vacía con una nota explícita, nunca una fila
// fabricada [diseño §5.8].
const checklistSinTareasTexto = "_(el checklist accionable del documento ODD de origen no registra tareas)_"

// RenderProposalBody traduce doc a los bytes completos de un proposal.md
// sembrado, siguiendo el mapeo determinista de REQ-19.9 (diseño §5.8):
// Objetivo+Problema+Porqué → Propósito; Alcance(+Alcance autorizado) →
// Dentro de Alcance; (sin origen declarado, +Restricciones) → Fuera de
// Alcance; checklist → Enfoque y el apéndice "Estado heredado de ODD";
// Criterios de aceptación y Comprobaciones aplicables → Criterios de
// Éxito; Evidencia y Siguiente paso → sus subsecciones homónimas.
// changeName y today determinan la cabecera de procedencia y el título:
// nunca los campos internos de doc (Status, PromotedTo, PromotedAt), que
// pueden pertenecer a una promoción previa y contradictoria si el propio
// documento ya fue promovido con anterioridad.
//
// La sección "Alcance" del documento ODD es un único bloque de texto libre
// que sólo describe lo que el trabajo cubre: el documento no declara en
// ningún sitio, de forma estructurada, qué queda deliberadamente excluido.
// Transportar ese mismo texto también bajo "Fuera de Alcance" no
// transportaría esa ausencia con fidelidad: afirmaría activamente que el
// mismo contenido entra y no entra en el alcance a la vez, una
// contradicción que fabricaría el propio renderizador, no el documento de
// origen — peor que el hueco que pretende evitar. Por eso "Alcance" se
// transporta verbatim únicamente a "Dentro de Alcance", y "Fuera de
// Alcance" recibe pendienteContenidoTexto: la misma regla, sin excepción,
// que ya se aplica a cualquier sección de destino sin origen declarado (el
// mismo principio que rige "## Capacidades") [D-09, frontera T-9]. Las
// restricciones heredadas, en cambio, sí son contenido real y distinto que
// el documento declara, y se anidan como subsección propia de "Fuera de
// Alcance".
//
// Toda sección de origen vacía produce pendienteContenidoTexto en su
// destino, nunca texto inventado ni la sección omitida. El contenido no
// vacío se transporta verbatim, sin sanear ni reescribir el Markdown del
// usuario [frontera T-9].
func RenderProposalBody(doc *Document, changeName, today string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Propuesta: %s (%s)\n\n", humanizeChangeName(changeName), changeName)
	writeProvenanceHeader(&b, doc.Feature, today)

	fmt.Fprintf(&b, "## Propósito (Intent)\n\n%s\n\n", sectionOrPlaceholder(doc, SectionObjective))
	fmt.Fprintf(&b, "### Problema\n\n%s\n\n", sectionOrPlaceholder(doc, SectionProblem))
	fmt.Fprintf(&b, "### Por qué ahora\n\n%s\n\n", sectionOrPlaceholder(doc, SectionWhy))

	fmt.Fprintf(&b, "### Dentro de Alcance (In Scope)\n\n%s\n\n", sectionOrPlaceholder(doc, SectionScope))
	fmt.Fprintf(&b, "#### Alcance autorizado heredado\n\n%s\n\n", sectionOrPlaceholder(doc, SectionAuthorizedScope))

	fmt.Fprintf(&b, "### Fuera de Alcance (Out of Scope)\n\n%s\n\n", pendienteContenidoTexto)
	fmt.Fprintf(&b, "#### Restricciones heredadas\n\n%s\n\n", sectionOrPlaceholder(doc, SectionConstraints))

	fmt.Fprintf(&b, "## Capacidades (Capabilities)\n\n%s\n\n", pendienteCapacidadesTexto)

	fmt.Fprintf(&b, "## Enfoque (Approach)\n\n%s\n\n", renderApproachList(doc.Tasks))

	fmt.Fprintf(&b, "## Criterios de Éxito\n\n%s\n\n", sectionOrPlaceholder(doc, SectionAcceptance))
	fmt.Fprintf(&b, "### Comprobaciones aplicables\n\n%s\n\n", sectionOrPlaceholder(doc, SectionChecks))

	fmt.Fprintf(&b, "## Estado heredado de ODD\n\n%s\n\n", renderInheritedStateAppendix(doc))

	fmt.Fprintf(&b, "### Evidencia de verificación observada\n\n%s\n\n", sectionOrPlaceholder(doc, SectionEvidence))
	fmt.Fprintf(&b, "### Siguiente paso declarado en ODD\n\n%s\n", sectionOrPlaceholder(doc, SectionNextStep))

	return b.String()
}

// writeProvenanceHeader escribe el bloque de cita de procedencia: declara
// siempre que el documento fue sembrado desde una promoción ODD, la ruta
// exacta de origen y la fecha de promoción, para que ningún revisor ni
// agente lo confunda con prosa redactada a mano [D-09, frontera T-9].
func writeProvenanceHeader(b *strings.Builder, feature, today string) {
	fmt.Fprintf(b, "> **Documento sembrado por promoción ODD.**\n")
	fmt.Fprintf(b, "> **Origen:** `odd/tasks/%s.md`\n", feature)
	fmt.Fprintf(b, "> **Fecha de promoción:** %s\n\n", today)
}

// sectionOrPlaceholder devuelve el texto de la sección id del documento sin
// modificarlo, o pendienteContenidoTexto si esa sección está vacía en el
// documento de origen. Nunca sanea ni reescribe el contenido no vacío
// [frontera T-9].
func sectionOrPlaceholder(doc *Document, id SectionID) string {
	text := doc.Sections[id]
	if strings.TrimSpace(text) == "" {
		return pendienteContenidoTexto
	}
	return text
}

// renderApproachList produce la lista numerada de "## Enfoque (Approach)" a
// partir únicamente del texto de cada tarea del checklist, en el mismo
// orden en que aparecen en el documento de origen. El estado de cada tarea
// no participa aquí: vive en el apéndice "Estado heredado de ODD"
// [diseño §5.8].
func renderApproachList(tasks []Task) string {
	if len(tasks) == 0 {
		return pendienteContenidoTexto
	}

	var b strings.Builder
	for i, task := range tasks {
		fmt.Fprintf(&b, "%d. %s\n", i+1, task.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderInheritedStateAppendix produce el cuerpo del apéndice "Estado
// heredado de ODD": una tabla con el estado real de cada tarea del
// checklist —el mismo marcador "- [x]"/"- [ ]" que ya usa el documento de
// origen, nunca traducido a prosa— seguida de la línea de progreso
// agregado. Un checklist sin tareas produce una tabla vacía con una nota
// explícita en vez de una fila fabricada, y el progreso 0/0 sin división
// por cero [diseño §5.8, D-04].
func renderInheritedStateAppendix(doc *Document) string {
	var b strings.Builder
	b.WriteString("| Tarea | Estado |\n")
	b.WriteString("|---|---|\n")

	if len(doc.Tasks) == 0 {
		fmt.Fprintf(&b, "\n%s\n\n", checklistSinTareasTexto)
	} else {
		for _, task := range doc.Tasks {
			fmt.Fprintf(&b, "| %s | %s |\n", taskLabel(task), checklistMark(task.Done))
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "Progreso: %d/%d tareas completadas (%.0f%%)", doc.Progress.Completed, doc.Progress.Total, doc.Progress.Percent)
	return b.String()
}

// taskLabel formatea la etiqueta de una tarea del checklist para la columna
// "Tarea" del apéndice de estado heredado: "[T1] Texto" cuando la tarea
// declara identificador estable, o sólo el texto cuando no lo declara —el
// mismo caso que Parse ya registra en Warnings (REQ-19.3).
func taskLabel(task Task) string {
	if task.ID == "" {
		return task.Text
	}
	return fmt.Sprintf("[%s] %s", task.ID, task.Text)
}

// checklistMark devuelve el mismo marcador de casilla que usa el checklist
// de origen para el estado done, en vez de traducirlo a prosa: preserva la
// notación del documento [frontera T-9].
func checklistMark(done bool) string {
	if done {
		return "- [x]"
	}
	return "- [ ]"
}

// mirrorPendingNoteTexto es la nota que PromoteResult.MirrorNote transporta
// siempre, incluso en el camino feliz: la CLI nunca da por actualizado el
// espejo de recuperación en Engram, que sólo el agente por MCP mantiene al
// margen del lado Go [principio §4.1.2 del diseño, diseño §5.5].
const mirrorPendingNoteTexto = "El espejo de recuperación en Engram queda pendiente de esta promoción: lo actualiza el agente por MCP en su siguiente sincronización."

// ScaffoldRequest transporta a Scaffolder los datos ya resueltos de una
// promoción para crear el cambio SDD correspondiente [diseño §5.5].
type ScaffoldRequest struct {
	Name         string
	Intent       string
	Type         string
	ProposalBody string
}

// ScaffoldResult es el resultado de un Scaffold exitoso [diseño §5.5].
type ScaffoldResult struct {
	Name string
	Path string
}

// Scaffolder abstrae el andamiador de creación de incrementos SDD que
// Promote consume. internal/odd no puede importar internal/dashboard sin
// cerrar el ciclo cli → odd → dashboard → cli: el adaptador que satisface
// esta interfaz sobre dashboard.Service.CreateIncrement vive en
// internal/cli/odd_promote.go [D-01].
type Scaffolder interface {
	Scaffold(ScaffoldRequest) (ScaffoldResult, error)
}

// PromoteOptions son los parámetros ya resueltos de "axiom odd promote"
// (REQ-19.8, REQ-19.9, REQ-19.10).
//
// Intent y Type amplían de forma aditiva el bloque de código del diseño
// (§5.5), que sólo lista Root/Feature/ChangeName/DryRun/Today: sin estos
// dos campos, las banderas --intent/--type que ParsePromoteArgs ya acepta
// (internal/odd/args.go) no tendrían ningún camino hasta ScaffoldRequest, y
// el propio diagrama de secuencia del diseño (§3.5) muestra
// "Scaffold{Name, Intent, Type, ProposalBody}" saliendo de Promote. Se
// documenta como desviación deliberada, no silenciosa, del bloque de
// código de §5.5.
type PromoteOptions struct {
	Root       string
	Feature    string
	ChangeName string // --name; vacío ⇒ Feature verbatim
	Intent     string
	Type       string
	DryRun     bool
	Today      string // inyectado para determinismo en tests
}

// PromoteResult es el resultado informativo de una promoción: en camino
// feliz, en --dry-run, o con un aviso de marca no escrita tras una
// creación exitosa [D-07, diseño §5.5].
type PromoteResult struct {
	Feature      string `json:"feature"`
	ChangeName   string `json:"change_name"`
	ProposalPath string `json:"proposal_path,omitempty"`
	Body         string `json:"-"`
	DryRun       bool   `json:"dry_run"`
	MarkWritten  bool   `json:"mark_written"`
	MirrorNote   string `json:"mirror_note"`
	Warning      string `json:"warning,omitempty"`
}

// Promote ejecuta el ciclo de promoción ODD → SDD en dos fases: validar y
// renderizar antes de escribir nada, después Scaffold, y sólo si Scaffold
// tiene éxito, MarkPromoted. Nunca al revés [D-07]. Con este orden, el
// único estado inconsistente posible —cambio SDD creado, documento ODD sin
// marcar— es autocorrector: una segunda promoción de la misma feature choca
// con la guarda de colisión del propio Scaffolder, así que nunca se crea un
// duplicado silencioso.
//
// Un documento ya promovido devuelve ErrAlreadyPromoted sin invocar sc en
// ningún momento [D-08]. Un nombre de cambio inválido se rechaza tras
// validar y antes de invocar sc [D-10, REQ-19.10]. Con DryRun, Promote
// devuelve el cuerpo renderizado sin invocar sc y sin escribir nada. Si sc
// falla, el documento ODD permanece intacto: MarkPromoted nunca se invoca.
// Si sc tiene éxito pero MarkPromoted falla, Promote no devuelve error: el
// aviso viaja en PromoteResult.Warning con MarkWritten en false, porque el
// cambio SDD ya existe y la promoción, en ese sentido, no ha fracasado por
// completo.
func Promote(opts PromoteOptions, sc Scaffolder) (*PromoteResult, error) {
	doc, err := Load(opts.Root, opts.Feature)
	if err != nil {
		return nil, err
	}

	if doc.Status == StatusPromoted {
		return nil, fmt.Errorf("la feature %q ya fue promovida a %s: %w", opts.Feature, doc.PromotedTo, ErrAlreadyPromoted)
	}

	changeName := opts.ChangeName
	if changeName == "" {
		changeName = opts.Feature
	}
	if err := ValidateFeatureName(changeName); err != nil {
		return nil, fmt.Errorf("el nombre de cambio %q derivado de la promoción no es válido: %w", changeName, err)
	}

	body := RenderProposalBody(doc, changeName, opts.Today)

	if opts.DryRun {
		return &PromoteResult{
			Feature:    opts.Feature,
			ChangeName: changeName,
			Body:       body,
			DryRun:     true,
			MirrorNote: mirrorPendingNoteTexto,
		}, nil
	}

	scaffolded, err := sc.Scaffold(ScaffoldRequest{
		Name:         changeName,
		Intent:       opts.Intent,
		Type:         opts.Type,
		ProposalBody: body,
	})
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el cambio SDD %q a partir de la feature %q: %w", changeName, opts.Feature, err)
	}

	result := &PromoteResult{
		Feature:      opts.Feature,
		ChangeName:   changeName,
		ProposalPath: scaffolded.Path,
		Body:         body,
		MirrorNote:   mirrorPendingNoteTexto,
	}

	if markErr := MarkPromoted(opts.Root, doc, changeName, opts.Today); markErr != nil {
		result.MarkWritten = false
		result.Warning = formatMarkFailureWarning(changeName, opts.Feature, markErr)
		return result, nil
	}

	result.MarkWritten = true
	return result, nil
}

// formatMarkFailureWarning construye el mensaje de remediación exacto de
// D-07 para el único estado inconsistente posible de Promote: el cambio
// SDD ya se creó, pero la marca de promoción no se pudo escribir en el
// documento ODD de origen.
func formatMarkFailureWarning(changeName, feature string, cause error) string {
	return fmt.Sprintf(
		"Aviso: el cambio SDD se creó en openspec/changes/%s/ pero no se pudo marcar "+
			"odd/tasks/%s.md como promovido (%v). Añade manualmente la línea "+
			"«> **Promovido a:** `openspec/changes/%s/`» o elimina el directorio del cambio.",
		changeName, feature, cause, changeName,
	)
}

// humanizeChangeName deriva un título legible a partir de changeName, con
// el mismo algoritmo que dashboard.humanizeName (no exportada): separar por
// guiones y capitalizar la primera letra de cada palabra. internal/odd no
// puede importar internal/dashboard sin cerrar el ciclo prohibido
// cli → odd → dashboard → cli [D-01], así que esta pequeña duplicación de
// seis líneas es deliberada [diseño §5.8].
func humanizeChangeName(changeName string) string {
	words := strings.Split(changeName, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
