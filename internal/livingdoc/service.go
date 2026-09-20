package livingdoc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
)

// Service provee la interfaz unificada para operaciones de documentación viva.
type Service struct {
	workspaceRoot string
	specsRoot     string
	indexPath     string
	indexer       *Indexer
	synthesizer   *Synthesizer
}

// NewService inicializa una nueva instancia del servicio de documentación viva.
func NewService(workspaceRoot string, indexer *Indexer, synthesizer *Synthesizer) *Service {
	if workspaceRoot == "" {
		workspaceRoot = "."
	}
	if indexer == nil {
		indexer = NewIndexer()
	}
	if synthesizer == nil {
		synthesizer = NewSynthesizer(indexer)
	}

	specsRoot := filepath.Join(workspaceRoot, "openspec", "specs")
	indexPath := filepath.Join(workspaceRoot, "openspec", "INDEX.md")

	// Inspeccionar axiom.yaml si existe para ajustar specs_repository
	cfgPath := filepath.Join(workspaceRoot, "axiom.yaml")
	if cfg, err := workspace.LoadConfig(cfgPath); err == nil && cfg != nil {
		if cfg.Workspace.SpecsRepository != "" && cfg.Workspace.SpecsRepository != "." {
			specsRoot = filepath.Join(workspaceRoot, cfg.Workspace.SpecsRepository, "specs")
			indexPath = filepath.Join(workspaceRoot, cfg.Workspace.SpecsRepository, "INDEX.md")
		}
	}

	return &Service{
		workspaceRoot: workspaceRoot,
		specsRoot:     specsRoot,
		indexPath:     indexPath,
		indexer:       indexer,
		synthesizer:   synthesizer,
	}
}

// GetCatalog devuelve el catálogo completo de especificaciones vivas consolidadas.
func (s *Service) GetCatalog(ctx context.Context) (*LivingCatalog, error) {
	catalog, _, err := s.indexer.ScanSpecs(s.specsRoot)
	return catalog, err
}

// GetSpecDetail obtiene los metadatos y el contenido Markdown completo de una especificación viva.
func (s *Service) GetSpecDetail(domain string) (*LivingSpecEntry, string, error) {
	specPath := filepath.Join(s.specsRoot, domain, "spec.md")
	contentBytes, err := os.ReadFile(specPath)
	if err != nil {
		return nil, "", fmt.Errorf("especificación viva para dominio '%s' no encontrada: %w", domain, err)
	}

	fi, _ := os.Stat(specPath)
	modTime := fi.ModTime()

	entry, _ := s.indexer.ParseSpecContent(string(contentBytes), domain, specPath, modTime)
	return entry, string(contentBytes), nil
}

// Sync ejecuta el re-escaneo y regenera el archivo openspec/INDEX.md.
func (s *Service) Sync(ctx context.Context) (*SyncReport, error) {
	return s.indexer.SyncIndex(s.specsRoot, s.indexPath)
}

// ColdStart ejecuta la adopción orgánica para un cambio y sincroniza el catálogo.
func (s *Service) ColdStart(changeName, domain string) (*LivingSpecEntry, error) {
	entry, err := s.synthesizer.ColdStart(s.workspaceRoot, changeName, domain)
	if err != nil {
		return nil, err
	}

	// Sincronizar catálogo maestro
	_, _ = s.Sync(context.Background())

	return entry, nil
}

// GetSpecsRoot retorna el directorio donde residen las especificaciones vivas.
func (s *Service) GetSpecsRoot() string {
	return s.specsRoot
}

// GetIndexPath retorna la ruta al archivo INDEX.md.
func (s *Service) GetIndexPath() string {
	return s.indexPath
}
