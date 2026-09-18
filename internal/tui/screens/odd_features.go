package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

// oddStatusPromovido es el valor literal que odd.StatusPromoted escribe en
// el documento vivo y expone en odd.FeatureSummary.Status [D-14]. Este
// paquete no importa internal/odd (ver GoDoc de ODDFeatureInfo): el literal
// se declara aquí, en el único punto donde este fichero necesita
// distinguir el estado de promoción para decidir el prefijo, el rótulo y el
// salto al cambio SDD de una fila.
const oddStatusPromovido = "promovido"

// oddFeatureTrailingOptions enumera, en este orden fijo, las etiquetas de
// las acciones globales que ODDFeaturesOptions añade siempre después de la
// lista de features. El orden aquí y el orden de las ramas de
// ODDFeaturesActionAt son la misma fuente de verdad: si se reordena esta
// lista, hay que reordenar ODDFeaturesActionAt exactamente igual [D-12].
var oddFeatureTrailingOptions = []string{
	"+ Crear nuevo documento ODD",
	"Promover a SDD",
	"↻ Comprobar espejo Engram",
	"→ Ir al carril formal (SDD)",
	"Volver a gobernanza",
}

// ODDFeatureInfo modela un documento vivo ODD para su listado en la TUI, con
// la misma información de progreso y estado de promoción que expone el
// Dashboard Web para la misma capacidad (REQ-19.13, REQ-19.14).
//
// Es un tipo propio de este paquete, no un alias de odd.FeatureSummary: el
// resto de pantallas de internal/tui/screens (por ejemplo SDDIncrementInfo)
// sigue la misma convención de desacoplar la presentación del dominio, y
// mantiene este paquete libre de la importación de internal/odd —esa arista
// nueva la introduce quien construya []ODDFeatureInfo a partir de
// odd.Scan(), fuera de este fichero.
type ODDFeatureInfo struct {
	// Feature es el nombre de la feature en kebab-case (REQ-19.2).
	Feature string
	// Path es la ruta del documento relativa a la raíz del workspace,
	// típicamente "odd/tasks/<feature>.md".
	Path string
	// Status reutiliza tal cual el valor literal de odd.Status ("activo" o
	// "promovido"): ya está en castellano y mostrarlo sin traducir evita
	// una segunda tabla de traducción que pueda divergir de la primera
	// [D-14].
	Status string
	// PromotedTo es la referencia al cambio SDD ya creado; vacío cuando
	// Status no es "promovido".
	PromotedTo string
	// TasksTotal y TasksCompleted reflejan el progreso derivado del
	// checklist accionable del documento (REQ-19.3).
	TasksTotal     int
	TasksCompleted int
	// ProgressPct es el porcentaje de progreso, ya redondeado para
	// presentación.
	ProgressPct int
}

// ODDAction identifica, de forma tipada, cada acción alcanzable desde la
// pantalla ODD. Sustituye la aritmética de índices que ya falló una vez en
// este mismo paquete (ScreenSDDIncrements, ver el análisis de la
// alternativa descartada en design.md D-12): ningún código fuera de
// ODDFeaturesActionAt debe calcular a qué acción corresponde una posición
// de cursor.
type ODDAction int

const (
	// ODDActionNone indica que la posición de cursor consultada no
	// corresponde a ninguna acción resoluble (por ejemplo, un cursor fuera
	// de rango).
	ODDActionNone ODDAction = iota
	// ODDActionSelectFeature indica que el cursor está sobre la fila de una
	// feature concreta; el índice acompañante identifica cuál dentro del
	// slice recibido.
	ODDActionSelectFeature
	// ODDActionCreate corresponde a la acción global "Crear nuevo documento
	// ODD" (REQ-19.5).
	ODDActionCreate
	// ODDActionPromote corresponde a la acción global "Promover a SDD"
	// (REQ-19.9-REQ-19.12).
	ODDActionPromote
	// ODDActionCheckMirror corresponde a la acción global "Comprobar
	// espejo Engram" (REQ-19.4, REQ-19.7).
	ODDActionCheckMirror
	// ODDActionGoToSDDLane corresponde a la acción global de conmutación
	// explícita hacia el carril formal SDD (REQ-19.15).
	ODDActionGoToSDDLane
	// ODDActionBack corresponde a "Volver a gobernanza".
	ODDActionBack
)

// ODDFeaturesOptions construye la lista de opciones de la pantalla ODD: una
// fila por cada feature, en el mismo orden en que se recibe, seguida
// siempre —con independencia de cuántas features existan, incluido
// cero— por las cinco acciones globales de oddFeatureTrailingOptions, en
// ese orden fijo.
func ODDFeaturesOptions(features []ODDFeatureInfo) []string {
	options := make([]string, 0, len(features)+len(oddFeatureTrailingOptions))
	for _, f := range features {
		options = append(options, formatODDFeatureOption(f))
	}
	options = append(options, oddFeatureTrailingOptions...)
	return options
}

// formatODDFeatureOption da forma a la fila de una sola feature: prefijo
// según su estado (al estilo de SDDIncrementsOptions), nombre, estado entre
// corchetes, progreso, ruta del documento y, cuando ya fue promovida, el
// salto visible al cambio SDD de destino (REQ-19.15).
func formatODDFeatureOption(f ODDFeatureInfo) string {
	prefix := "▶"
	if f.Status == oddStatusPromovido {
		prefix = "✓"
	}

	label := fmt.Sprintf("%s %s [%s] (%d/%d tareas, %d%%) — %s",
		prefix, f.Feature, strings.ToUpper(f.Status), f.TasksCompleted, f.TasksTotal, f.ProgressPct, f.Path)

	if f.Status == oddStatusPromovido && f.PromotedTo != "" {
		label += fmt.Sprintf(" → Ir al cambio SDD: %s", f.PromotedTo)
	}

	return label
}

// ODDFeaturesActionAt resuelve la acción correspondiente a cursor sobre la
// lista devuelta por ODDFeaturesOptions(features). Es la única fuente de
// verdad del mapeo cursor → acción de esta pantalla [D-12]: ningún otro
// punto del paquete debe repetir esta aritmética.
//
// El segundo valor devuelto es el índice de features al que se refiere la
// acción. Sólo ODDActionSelectFeature señala una fila de feature concreta;
// el resto de acciones son globales a la pantalla y devuelven -1.
func ODDFeaturesActionAt(features []ODDFeatureInfo, cursor int) (ODDAction, int) {
	if cursor < 0 {
		return ODDActionNone, -1
	}
	if cursor < len(features) {
		return ODDActionSelectFeature, cursor
	}

	switch cursor - len(features) {
	case 0:
		return ODDActionCreate, -1
	case 1:
		return ODDActionPromote, -1
	case 2:
		return ODDActionCheckMirror, -1
	case 3:
		return ODDActionGoToSDDLane, -1
	case 4:
		return ODDActionBack, -1
	default:
		return ODDActionNone, -1
	}
}

// RenderODDFeatures renderiza la pantalla del carril ágil ODD en la TUI
// (REQ-19.14): la lista de documentos vivos con su progreso y su estado de
// promoción, más las acciones para crear, promover, comprobar el espejo de
// recuperación en Engram y conmutar de forma visible al carril formal SDD
// (REQ-19.15). Sin documentos vivos, informa un estado vacío explícito en
// vez de un listado en blanco, sin tratarlo como error.
func RenderODDFeatures(features []ODDFeatureInfo, cursor int, message string) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("🌱 Carril Ágil ODD"))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("Documentos vivos en odd/tasks/, con su progreso y su estado de promoción:"))
	b.WriteString("\n\n")

	if message != "" {
		b.WriteString(styles.WarningStyle.Render(message))
		b.WriteString("\n\n")
	}

	if len(features) == 0 {
		b.WriteString(styles.SubtextStyle.Render("No hay documentos vivos ODD todavía. Usa «+ Crear nuevo documento ODD» para empezar."))
		b.WriteString("\n\n")
	}

	options := ODDFeaturesOptions(features)
	b.WriteString(renderOptions(options, cursor))

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navegar • enter: seleccionar/ejecutar • esc: volver"))

	return styles.FrameStyle.Render(b.String())
}
