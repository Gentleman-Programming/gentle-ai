package dashboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/odd"
)

// Este fichero expone el carril ODD (REQ-19.13) sobre la misma API REST
// local que ya sirve el carril SDD en service.go/server.go. Vive en un
// fichero propio para no inflar service.go [diseño §4.4, tarea 6.2].
//
// Ni esta capa ni la TUI (fase posterior) consumen una proyección
// estructurada del estado ODD: ambas re-derivan lo que muestran a partir de
// odd.Scan/odd.Load, exactamente igual que Service.GetIncrements re-deriva
// el estado SDD escaneando openspec/changes/ en cada llamada [D-11].

// GetODDFeatures escanea odd/tasks/ bajo la raíz del workspace y devuelve el
// resumen de cada documento vivo existente (REQ-19.13). Delegación directa
// en odd.Scan, que ya trata la ausencia del directorio como cero features
// sin error: un workspace sin carril ágil todavía es un estado normal, igual
// que GetIncrements trata la ausencia de openspec/changes/.
func (s *Service) GetODDFeatures() ([]odd.FeatureSummary, error) {
	root := s.getRootPath()
	summaries, err := odd.Scan(root)
	if err != nil {
		return nil, fmt.Errorf("no se pudieron listar los documentos vivos ODD en %q: %w", root, err)
	}
	return summaries, nil
}

// GetODDFeature carga y analiza el documento vivo de feature (REQ-19.13).
// Delegación directa en odd.Load; odd.ErrFeatureNotFound se propaga
// envuelto para que la capa HTTP (server.go) lo traduzca a 404, igual que ya
// hace GetIncrementDetail con FindIncrementPath.
func (s *Service) GetODDFeature(feature string) (*odd.Document, error) {
	root := s.getRootPath()
	doc, err := odd.Load(root, feature)
	if err != nil {
		return nil, fmt.Errorf("no se pudo consultar el documento vivo ODD de la feature %q: %w", feature, err)
	}
	return doc, nil
}

// CreateODDFeature crea un documento vivo ODD nuevo a partir de req
// (REQ-19.5, REQ-19.13). Recorta el nombre recibido y rechaza explícitamente
// una petición sin feature antes de delegar en odd.Create, que aplica el
// resto de la validación de nombre [D-10]. Cualquier error del dominio
// (nombre inválido, reservado o en colisión) se propaga envuelto con
// contexto — nunca con %v — para que errors.Is siga reconociendo el
// centinela original y la capa HTTP lo traduzca a 400, igual que ya hace
// CreateIncrement con validIncrementNameRegex.
func (s *Service) CreateODDFeature(req ODDCreateRequest) (*odd.Document, error) {
	feature := strings.TrimSpace(req.Feature)
	if feature == "" {
		return nil, fmt.Errorf("el campo 'feature' es obligatorio para crear un documento vivo ODD")
	}

	root := s.getRootPath()
	doc, err := odd.Create(root, feature, time.Now().Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el documento vivo ODD de la feature %q: %w", feature, err)
	}
	return doc, nil
}

// PromoteODDFeature promueve el documento vivo de req.Feature a una
// propuesta SDD sembrada, o la previsualiza sin escribir nada cuando
// req.DryRun es true (REQ-19.8, REQ-19.9, REQ-19.13). Delega en odd.Promote
// a través de oddScaffolder, que adapta Service.CreateIncrement al puerto
// odd.Scaffolder [D-01]: a diferencia del adaptador de la CLI
// (internal/cli/odd_promote.go), este vive en el mismo paquete que
// CreateIncrement, así que no hace falta ningún parámetro inyectado ni
// ningún paquete intermedio para evitar un ciclo de importación.
func (s *Service) PromoteODDFeature(req ODDPromoteRequest) (*odd.PromoteResult, error) {
	feature := strings.TrimSpace(req.Feature)
	if feature == "" {
		return nil, fmt.Errorf("el campo 'feature' es obligatorio para promover un documento vivo ODD")
	}

	root := s.getRootPath()
	result, err := odd.Promote(odd.PromoteOptions{
		Root:       root,
		Feature:    feature,
		ChangeName: strings.TrimSpace(req.Name),
		DryRun:     req.DryRun,
		Today:      time.Now().Format("2006-01-02"),
	}, oddScaffolder{service: s})
	if err != nil {
		return nil, fmt.Errorf("no se pudo promover la feature %q a un cambio SDD: %w", feature, err)
	}
	return result, nil
}

// CheckODDMirror compara el documento vivo de req.Feature contra su espejo
// de recuperación en Engram, obtenido a través de export (REQ-19.4,
// REQ-19.7). export se recibe como parámetro explícito, no como un valor por
// defecto oculto en el servicio: server.go decide qué Exporter usar en
// producción (odd.DefaultExporter) y las pruebas inyectan uno falso para
// instrumentar los tres estados de espejo sin invocar el subproceso real
// "engram export" [mismo motivo que documenta
// internal/cli/odd_status.go: RunODDStatus/runODDStatus].
func (s *Service) CheckODDMirror(ctx context.Context, req ODDMirrorRequest, export odd.Exporter) (*odd.MirrorReport, error) {
	feature := strings.TrimSpace(req.Feature)
	if feature == "" {
		return nil, fmt.Errorf("el campo 'feature' es obligatorio para comprobar el espejo ODD")
	}

	root := s.getRootPath()
	doc, err := odd.Load(root, feature)
	if err != nil {
		return nil, fmt.Errorf("no se pudo cargar el documento vivo ODD de la feature %q para comprobar su espejo: %w", feature, err)
	}

	report := odd.CheckMirror(ctx, root, doc, export)
	return &report, nil
}

// oddScaffolder adapta Service.CreateIncrement al puerto odd.Scaffolder que
// consume odd.Promote [D-01]. internal/dashboard ya expone el método que
// necesita: a diferencia del adaptador de la CLI (internal/cli/odd_promote.go),
// no hace falta ningún parámetro inyectado ni ningún paquete intermedio para
// evitar el ciclo de importación que sí afecta a internal/cli — dashboard no
// necesita importarse a sí mismo.
type oddScaffolder struct {
	service *Service
}

// Scaffold traduce ScaffoldRequest a CreateIncrementRequest y delega en
// Service.CreateIncrement, reutilizando sus dos guardas de colisión y su
// validación de nombre intactas [D-01, D-02].
func (a oddScaffolder) Scaffold(req odd.ScaffoldRequest) (odd.ScaffoldResult, error) {
	res, err := a.service.CreateIncrement(CreateIncrementRequest{
		Name:         req.Name,
		Intent:       req.Intent,
		Type:         req.Type,
		ProposalBody: req.ProposalBody,
	})
	if err != nil {
		return odd.ScaffoldResult{}, err
	}
	return odd.ScaffoldResult{Name: res.Name, Path: res.Path}, nil
}
