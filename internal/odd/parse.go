package odd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/multirole"
)

// Prefijos de línea reconocidos al analizar la cabecera de estado del
// documento vivo. El análisis es puramente léxico, por prefijo de línea —
// el mismo estilo que multirole.CountTasks — sin ningún parser de Markdown
// de propósito general. [D-06]
const (
	headerPrefix     = "# ODD: "
	stateLinePrefix  = "> **Estado:**"
	promotedToPrefix = "> **Promovido a:**"
	promotedAtPrefix = "> **Promovido el:**"
	sectionMark      = "## "
)

// checklistLineRegex reconoce una línea de checklist con un identificador
// estable opcional entre corchetes justo tras la casilla, por ejemplo
// "- [ ] [T1] Redactar el borrador". Si la línea no declara identificador,
// la tarea se conserva con ID vacío y Parse la registra en Warnings: el
// sistema nunca reasigna un identificador a partir de la posición
// (REQ-19.3).
var checklistLineRegex = regexp.MustCompile(`^-\s*\[([ xX])\]\s*(?:\[(T\d+)\]\s*)?(.*)$`)

// Parse analiza el contenido crudo de un documento vivo ODD. Sólo devuelve
// error cuando el documento carece de la cabecera mínima "# ODD:
// <feature>"; cualquier otra irregularidad —sección canónica ausente,
// tarea de checklist sin identificador— se degrada a Warnings en vez de a
// error, para que un documento editado a mano por un humano se siga
// analizando en vez de bloquear el carril ágil.
func Parse(raw string) (*Document, error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	feature, headerIdx, err := parseFeatureHeader(lines)
	if err != nil {
		return nil, err
	}

	doc := &Document{
		Feature:  feature,
		Status:   StatusActive,
		Sections: make(map[SectionID]string),
		Raw:      normalized,
	}

	cursor := parseStatusHeader(doc, lines, headerIdx+1)
	sectionsByTitle := splitSections(lines[cursor:])

	var warnings []string
	for _, id := range CanonicalSections {
		title := sectionTitles[id]
		text, ok := sectionsByTitle[title]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("sección canónica ausente en el documento: %q", title))
			continue
		}
		doc.Sections[id] = text
	}

	tasks, taskWarnings := parseChecklist(doc.Sections[SectionChecklist])
	doc.Tasks = tasks
	warnings = append(warnings, taskWarnings...)
	doc.Warnings = warnings

	// El progreso se calcula únicamente sobre la rebanada de texto de
	// "Checklist accionable", nunca sobre el documento completo: la
	// plantilla canónica usa casillas también en "Criterios de aceptación"
	// y "Comprobaciones aplicables", y multirole.CountTasks no distingue
	// secciones. [D-04]
	doc.Progress = multirole.CountTasks(doc.Sections[SectionChecklist])

	return doc, nil
}

// parseFeatureHeader localiza la primera línea no vacía del documento y
// exige que sea la cabecera "# ODD: <feature>". Es el único punto de
// análisis que puede producir ErrMalformedDocument: un documento sin esta
// cabecera no es un documento ODD.
func parseFeatureHeader(lines []string) (string, int, error) {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, headerPrefix) {
			return "", 0, fmt.Errorf("el documento no comienza con la cabecera %q: %w", headerPrefix, ErrMalformedDocument)
		}
		return strings.TrimSpace(strings.TrimPrefix(trimmed, headerPrefix)), i, nil
	}
	return "", 0, fmt.Errorf("el documento está vacío: falta la cabecera %q: %w", headerPrefix, ErrMalformedDocument)
}

// parseStatusHeader analiza el bloque de cita que sigue a la cabecera de
// feature: el estado y —cuando el documento ya fue promovido— la
// referencia al cambio SDD y la fecha [D-06]. Devuelve el índice de la
// primera línea posterior al bloque de cita, desde el que continúa el
// troceado por secciones.
func parseStatusHeader(doc *Document, lines []string, start int) int {
	cursor := start
	for cursor < len(lines) {
		trimmed := strings.TrimSpace(lines[cursor])
		if trimmed == "" {
			cursor++
			continue
		}
		if !strings.HasPrefix(trimmed, ">") {
			break
		}

		switch {
		case strings.HasPrefix(trimmed, stateLinePrefix):
			if strings.TrimSpace(strings.TrimPrefix(trimmed, stateLinePrefix)) == string(StatusPromoted) {
				doc.Status = StatusPromoted
			}
		case strings.HasPrefix(trimmed, promotedToPrefix):
			doc.PromotedTo = extractBacktickValue(strings.TrimPrefix(trimmed, promotedToPrefix))
		case strings.HasPrefix(trimmed, promotedAtPrefix):
			doc.PromotedAt = strings.TrimSpace(strings.TrimPrefix(trimmed, promotedAtPrefix))
		}
		cursor++
	}
	return cursor
}

// extractBacktickValue devuelve el contenido entre el primer par de
// comillas invertidas de s, o s recortado si no hay comillas invertidas.
func extractBacktickValue(s string) string {
	s = strings.TrimSpace(s)
	start := strings.Index(s, "`")
	if start == -1 {
		return s
	}
	end := strings.Index(s[start+1:], "`")
	if end == -1 {
		return s
	}
	return s[start+1 : start+1+end]
}

// splitSections trocea las líneas posteriores al bloque de cita en un mapa
// título de sección → contenido, usando los encabezados "## <título>" como
// separador. El contenido de cada sección se devuelve recortado de espacio
// en blanco perimetral.
func splitSections(lines []string) map[string]string {
	sections := make(map[string]string)

	var title string
	var buf strings.Builder
	open := false

	flush := func() {
		if open {
			sections[title] = strings.TrimSpace(buf.String())
		}
		buf.Reset()
	}

	for _, line := range lines {
		if strings.HasPrefix(line, sectionMark) {
			flush()
			title = strings.TrimSpace(strings.TrimPrefix(line, sectionMark))
			open = true
			continue
		}
		if open {
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}
	flush()

	return sections
}

// parseChecklist analiza el texto de la sección "Checklist accionable" y
// devuelve sus tareas junto con las advertencias de identificador ausente
// (REQ-19.3). Una sección vacía o sin líneas de checklist reconocibles
// devuelve cero tareas sin error.
func parseChecklist(section string) ([]Task, []string) {
	var tasks []Task
	var warnings []string

	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		matches := checklistLineRegex.FindStringSubmatch(trimmed)
		if matches == nil {
			continue
		}

		mark, id, text := matches[1], matches[2], strings.TrimSpace(matches[3])
		tasks = append(tasks, Task{ID: id, Text: text, Done: mark == "x" || mark == "X"})
		if id == "" {
			warnings = append(warnings, fmt.Sprintf("tarea de checklist sin identificador estable: %q", text))
		}
	}

	return tasks, warnings
}
