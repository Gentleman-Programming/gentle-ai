package odd

import (
	"fmt"
	"strings"
)

// placeholderContent es el marcador de contenido pendiente que RenderNew
// coloca bajo cada una de las doce secciones canónicas de un documento vivo
// recién creado.
const placeholderContent = "_(pendiente de completar)_"

// sectionTitles asocia cada SectionID con el título exacto de su encabezado
// Markdown, en el orden canónico definido por REQ-19.1. La plantilla y —en
// fases posteriores de este mismo paquete— el analizador y el renderizador
// de promoción comparten esta única fuente de verdad para el texto del
// encabezado.
var sectionTitles = map[SectionID]string{
	SectionObjective:       "Objetivo",
	SectionProblem:         "Problema",
	SectionWhy:             "Porqué",
	SectionScope:           "Alcance",
	SectionConstraints:     "Restricciones",
	SectionAuthorizedScope: "Alcance autorizado",
	SectionChecklist:       "Checklist accionable",
	SectionAcceptance:      "Criterios de aceptación",
	SectionChecks:          "Comprobaciones aplicables",
	SectionProgress:        "Progreso",
	SectionEvidence:        "Evidencia de verificación",
	SectionNextStep:        "Siguiente paso",
}

// RenderNew genera el contenido determinista de un documento vivo ODD nuevo
// para feature, con las doce secciones canónicas en su orden y la cabecera
// de estado activo (REQ-19.1, REQ-19.2). today se usa únicamente como nota
// informativa de creación, fuera del bloque de cita de estado que analizará
// la fase de análisis de este paquete [D-06]. El resultado siempre termina
// en un único salto de línea.
func RenderNew(feature, today string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# ODD: %s\n\n", feature)
	fmt.Fprintf(&b, "> **Estado:** %s\n\n", StatusActive)
	fmt.Fprintf(&b, "_Documento creado el %s._\n", today)

	for _, id := range CanonicalSections {
		fmt.Fprintf(&b, "\n## %s\n\n", sectionTitles[id])
		b.WriteString(placeholderContent)
		b.WriteString("\n")
	}

	return b.String()
}
