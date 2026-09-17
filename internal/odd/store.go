package odd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Create genera un documento vivo nuevo para feature y lo escribe en
// odd/tasks/<feature>.md bajo root. Devuelve ErrFeatureExists si ya existe
// un documento con ese nombre (REQ-19.2): esta función nunca sobrescribe un
// documento existente.
func Create(root, feature, today string) (*Document, error) {
	path, err := DocumentPath(root, feature)
	if err != nil {
		return nil, err
	}

	if _, statErr := os.Stat(path); statErr == nil {
		return nil, fmt.Errorf("ya existe un documento vivo para la feature %q en %s: %w", feature, path, ErrFeatureExists)
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("no se pudo comprobar si el documento vivo de %q ya existe: %w", feature, statErr)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("no se pudo crear el directorio del documento vivo de %q: %w", feature, err)
	}

	raw := RenderNew(feature, today)
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		return nil, fmt.Errorf("no se pudo escribir el documento vivo de %q en %s: %w", feature, path, err)
	}

	doc, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("el documento recién creado para la feature %q no se pudo analizar: %w", feature, err)
	}
	doc.Path = relativeDocumentPath(feature)

	return doc, nil
}

// Load lee y analiza el documento vivo de feature. Devuelve
// ErrFeatureNotFound si no existe ningún documento con ese nombre.
func Load(root, feature string) (*Document, error) {
	path, err := DocumentPath(root, feature)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no existe ningún documento vivo para la feature %q: %w", feature, ErrFeatureNotFound)
		}
		return nil, fmt.Errorf("no se pudo leer el documento vivo de %q en %s: %w", feature, path, err)
	}

	doc, err := Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("el documento vivo de %q en %s está malformado: %w", feature, path, err)
	}
	doc.Path = relativeDocumentPath(feature)

	return doc, nil
}

// Scan lista los documentos vivos existentes bajo odd/tasks/ y devuelve su
// resumen. La ausencia del directorio no es un fallo: un workspace sin
// carril ágil todavía es un estado normal (REQ-19.6), igual que el
// escaneo de incrementos SDD trata la ausencia de su propio directorio.
// Un documento individual ilegible o malformado se omite del listado en
// vez de abortar el escaneo completo.
func Scan(root string) ([]FeatureSummary, error) {
	tasksDir := filepath.Join(root, "odd", "tasks")

	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("no se pudo listar el directorio de documentos vivos %s: %w", tasksDir, err)
	}

	var summaries []FeatureSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		feature := strings.TrimSuffix(entry.Name(), ".md")
		doc, err := Load(root, feature)
		if err != nil {
			continue
		}
		summaries = append(summaries, toFeatureSummary(doc))
	}

	return summaries, nil
}

// MarkPromoted reescribe la cabecera de estado de doc para reflejar una
// promoción exitosa a openspec/changes/<changeName>/, persiste el cambio en
// disco y actualiza doc en memoria (Status, PromotedTo, PromotedAt, Raw).
// [D-06] Es la única función de este paquete que muta un Document tras su
// análisis inicial.
func MarkPromoted(root string, doc *Document, changeName, today string) error {
	path, err := DocumentPath(root, doc.Feature)
	if err != nil {
		return err
	}

	reference := promotionReference(changeName)

	updatedRaw, err := replaceStatusHeader(doc.Raw, reference, today)
	if err != nil {
		return fmt.Errorf("no se pudo construir la cabecera de promoción del documento vivo de %q: %w", doc.Feature, err)
	}

	if err := os.WriteFile(path, []byte(updatedRaw), 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir la marca de promoción del documento vivo de %q en %s: %w", doc.Feature, path, err)
	}

	doc.Status = StatusPromoted
	doc.PromotedTo = reference
	doc.PromotedAt = today
	doc.Raw = updatedRaw

	return nil
}

// promotionReference construye la referencia canónica al cambio SDD que
// recibe la promoción, tal como se escribe en la cabecera del documento
// vivo y en Document.PromotedTo. [D-06]
func promotionReference(changeName string) string {
	return fmt.Sprintf("openspec/changes/%s/", changeName)
}

// replaceStatusHeader sustituye la línea "> **Estado:**" (y cualquier línea
// de promoción previa que la siga inmediatamente) por el bloque de tres
// líneas del estado promovido, conservando intacto el resto del documento.
func replaceStatusHeader(raw, reference, today string) (string, error) {
	lines := strings.Split(raw, "\n")

	stateIdx := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), stateLinePrefix) {
			stateIdx = i
			break
		}
	}
	if stateIdx == -1 {
		return "", fmt.Errorf("no se encontró la línea de estado %q en el documento: %w", stateLinePrefix, ErrMalformedDocument)
	}

	end := stateIdx + 1
	for end < len(lines) {
		trimmed := strings.TrimSpace(lines[end])
		if !strings.HasPrefix(trimmed, promotedToPrefix) && !strings.HasPrefix(trimmed, promotedAtPrefix) {
			break
		}
		end++
	}

	replacement := []string{
		fmt.Sprintf("> **Estado:** %s", StatusPromoted),
		fmt.Sprintf("> **Promovido a:** `%s`", reference),
		fmt.Sprintf("> **Promovido el:** %s", today),
	}

	result := make([]string, 0, len(lines)-(end-stateIdx)+len(replacement))
	result = append(result, lines[:stateIdx]...)
	result = append(result, replacement...)
	result = append(result, lines[end:]...)

	return strings.Join(result, "\n"), nil
}

// relativeDocumentPath devuelve la ruta del documento relativa a la raíz
// del workspace, con separadores '/' con independencia del sistema
// operativo (Document.Path se expone en JSON y se muestra al usuario).
func relativeDocumentPath(feature string) string {
	return "odd/tasks/" + feature + ".md"
}

// toFeatureSummary proyecta un Document ya analizado a su resumen para
// listados: axiom odd status, GET /api/odd y la pantalla ODD de la TUI.
func toFeatureSummary(doc *Document) FeatureSummary {
	return FeatureSummary{
		Feature:    doc.Feature,
		Path:       doc.Path,
		Status:     doc.Status,
		PromotedTo: doc.PromotedTo,
		PromotedAt: doc.PromotedAt,
		Progress:   doc.Progress,
		Warnings:   doc.Warnings,
	}
}
