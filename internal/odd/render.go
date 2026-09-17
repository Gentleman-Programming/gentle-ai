package odd

import (
	"fmt"
	"strings"
)

// sinDocumentosVivosTexto es la salida exacta de RenderStatusText cuando el
// StatusReport no contiene ninguna feature. Se declara aparte para que el
// mensaje de estado vacío tenga una única fuente de verdad.
const sinDocumentosVivosTexto = "No se encontraron documentos vivos ODD en odd/tasks/.\n"

// RenderStatusText produce la salida en texto legible de "axiom odd status"
// (REQ-19.6) a partir de un StatusReport ya resuelto por Scan. Sin
// documentos vivos, informa un estado vacío explícito en vez de imprimir un
// listado en blanco, sin tratarlo como error. Cuando FeatureSummary.Mirror
// está poblado —únicamente cuando la CLI invocó --check-mirror (REQ-19.7)—
// añade una línea adicional con el estado del espejo de recuperación en
// Engram para esa feature. El resultado siempre termina en un único salto
// de línea.
func RenderStatusText(report StatusReport) string {
	if len(report.Features) == 0 {
		return sinDocumentosVivosTexto
	}

	var b strings.Builder
	for i, feature := range report.Features {
		if i > 0 {
			b.WriteString("\n")
		}
		writeFeatureStatusBlock(&b, feature)
	}

	return b.String()
}

// writeFeatureStatusBlock escribe en b el bloque de texto de una sola
// feature: la línea principal de progreso, la línea de referencia de
// promoción cuando el documento ya fue promovido (REQ-19.12), y la línea de
// estado del espejo de recuperación en Engram cuando Mirror está poblado
// (REQ-19.7).
func writeFeatureStatusBlock(b *strings.Builder, feature FeatureSummary) {
	fmt.Fprintf(b, "%s [%s] — %d/%d tareas completadas (%.0f%%)\n",
		feature.Feature, feature.Status, feature.Progress.Completed, feature.Progress.Total, feature.Progress.Percent)

	if feature.Status == StatusPromoted && feature.PromotedTo != "" {
		fmt.Fprintf(b, "  Promovido a: %s\n", feature.PromotedTo)
	}

	if feature.Mirror != nil {
		if feature.Mirror.Reason != "" {
			fmt.Fprintf(b, "  Espejo: %s (%s)\n", feature.Mirror.State, feature.Mirror.Reason)
		} else {
			fmt.Fprintf(b, "  Espejo: %s\n", feature.Mirror.State)
		}
	}
}
