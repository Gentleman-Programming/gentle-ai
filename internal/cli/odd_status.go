package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gentleman-programming/gentle-ai/v2/internal/odd"
)

// RunODDStatus es el punto de entrada de la CLI para "axiom odd status
// [feature]" (REQ-19.6, REQ-19.7). Es un adaptador fino: interpreta los
// argumentos, resuelve la raíz de trabajo, delega en el dominio y traduce el
// resultado a texto legible o a JSON estructurado. Nunca escribe en Engram;
// la comparación contra el espejo de recuperación sólo se ejecuta cuando el
// usuario pasa --check-mirror explícitamente (REQ-19.4, REQ-19.7).
func RunODDStatus(args []string, stdout io.Writer) error {
	return runODDStatus(args, stdout, odd.DefaultExporter)
}

// runODDStatus implementa RunODDStatus recibiendo el Exporter del espejo
// como parámetro. Esta variante no exportada es la única forma de
// instrumentar los tres estados de espejo (sincronizado, divergente, no
// disponible) en pruebas sin invocar el subproceso real "engram export":
// RunODDStatus, la única firma que exige el diseño (§5.7), no admite un
// Exporter inyectado.
func runODDStatus(args []string, stdout io.Writer, export odd.Exporter) error {
	parsed, err := odd.ParseStatusArgs(args)
	if err != nil {
		return err
	}

	if err := requireODDWorkDir(parsed.CWD); err != nil {
		return err
	}

	summaries, err := loadRequestedODDFeatureSummaries(parsed.CWD, parsed.Feature)
	if err != nil {
		return err
	}

	if parsed.CheckMirror {
		attachODDMirrorReports(parsed.CWD, summaries, export)
	}

	report := odd.StatusReport{Features: summaries}

	if parsed.JSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	_, err = fmt.Fprint(stdout, odd.RenderStatusText(report))
	return err
}

// loadRequestedODDFeatureSummaries resuelve los resúmenes de feature que
// debe mostrar "axiom odd status": todos los documentos vivos existentes
// bajo odd/tasks/ cuando el usuario no especifica ninguna feature
// (REQ-19.6), o únicamente el resumen de la feature nombrada cuando sí la
// indica. El camino con nombre usa odd.Load (no odd.Scan + filtrado) para
// heredar exactamente la misma validación defensiva de nombre que ya aplica
// odd.DocumentPath [D-10, frontera T-7]: ningún argumento del usuario llega
// a construir una ruta de fichero sin pasar antes por esa validación.
func loadRequestedODDFeatureSummaries(cwd, feature string) ([]odd.FeatureSummary, error) {
	if feature == "" {
		summaries, err := odd.Scan(cwd)
		if err != nil {
			return nil, fmt.Errorf("no se pudo listar los documentos vivos ODD en %q: %w", cwd, err)
		}
		return summaries, nil
	}

	doc, err := odd.Load(cwd, feature)
	if err != nil {
		return nil, fmt.Errorf("no se pudo consultar el estado de la feature %q: %w", feature, err)
	}
	return []odd.FeatureSummary{oddFeatureSummaryFromDocument(doc)}, nil
}

// oddFeatureSummaryFromDocument proyecta un Document ya analizado a su
// FeatureSummary correspondiente. odd.Scan aplica el mismo mapeo
// internamente mediante una función no exportada (toFeatureSummary,
// internal/odd/store.go): esta copia mínima existe porque el camino de una
// sola feature con nombre explícito pasa por odd.Load, no por odd.Scan, y
// ensanchar la superficie exportada del dominio para un único llamador no se
// justifica frente a repetir estas siete líneas de asignación.
func oddFeatureSummaryFromDocument(doc *odd.Document) odd.FeatureSummary {
	return odd.FeatureSummary{
		Feature:    doc.Feature,
		Path:       doc.Path,
		Status:     doc.Status,
		PromotedTo: doc.PromotedTo,
		PromotedAt: doc.PromotedAt,
		Progress:   doc.Progress,
		Warnings:   doc.Warnings,
	}
}

// attachODDMirrorReports puebla, para cada resumen recibido, el campo Mirror
// con el resultado de comparar su documento contra el espejo de
// recuperación en Engram (REQ-19.7). Sólo se invoca desde runODDStatus
// cuando el usuario pasó --check-mirror explícitamente: nunca en el camino
// por defecto. Vuelve a cargar cada documento con odd.Load para obtener su
// contenido íntegro (Document.Raw): odd.Scan no lo conserva, porque
// descartar el documento tras proyectarlo a su resumen es correcto para el
// camino por defecto, que nunca compara contra el espejo.
func attachODDMirrorReports(cwd string, summaries []odd.FeatureSummary, export odd.Exporter) {
	ctx := context.Background()
	for i := range summaries {
		doc, err := odd.Load(cwd, summaries[i].Feature)
		if err != nil {
			summaries[i].Mirror = &odd.MirrorReport{
				State:  odd.MirrorUnavailable,
				Topic:  odd.MirrorTopic(summaries[i].Feature),
				Reason: fmt.Sprintf("no se pudo volver a cargar el documento para comparar el espejo: %v", err),
			}
			continue
		}
		report := odd.CheckMirror(ctx, cwd, doc, export)
		summaries[i].Mirror = &report
	}
}
