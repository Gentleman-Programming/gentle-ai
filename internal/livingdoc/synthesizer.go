package livingdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Synthesizer gestiona la síntesis orgánica de especificaciones vivas para proyectos con Cold Start.
type Synthesizer struct {
	indexer *Indexer
}

// NewSynthesizer inicializa una nueva instancia del sintetizador de documentación viva.
func NewSynthesizer(indexer *Indexer) *Synthesizer {
	if indexer == nil {
		indexer = NewIndexer()
	}
	return &Synthesizer{
		indexer: indexer,
	}
}

// ColdStart adopta orgánicamente un cambio, promoviendo su spec.md a especificación viva si no existía.
func (s *Synthesizer) ColdStart(workspaceRoot, changeName, targetDomain string) (*LivingSpecEntry, error) {
	if targetDomain == "" {
		// Limpiar prefijos de fecha o de incremento (ej. 2026-09-14-inc-01-workspace -> workspace)
		parts := strings.Split(changeName, "-")
		if len(parts) >= 3 && parts[1] == "inc" {
			targetDomain = strings.Join(parts[3:], "-")
		} else {
			targetDomain = changeName
		}
	}

	specsDir := filepath.Join(workspaceRoot, "openspec", "specs")
	domainDir := filepath.Join(specsDir, targetDomain)
	targetSpecPath := filepath.Join(domainDir, "spec.md")

	// 1. Localizar el spec.md del cambio (en openspec/changes/ o en openspec/changes/archive/)
	var sourceSpecPath string
	candidates := []string{
		filepath.Join(workspaceRoot, "openspec", "changes", changeName, "spec.md"),
		filepath.Join(workspaceRoot, "openspec", "changes", "archive", changeName, "spec.md"),
	}

	// También buscar por coincidencia parcial en archive
	archiveDir := filepath.Join(workspaceRoot, "openspec", "changes", "archive")
	if entries, err := os.ReadDir(archiveDir); err == nil {
		for _, e := range entries {
			if strings.Contains(e.Name(), changeName) {
				candidates = append(candidates, filepath.Join(archiveDir, e.Name(), "spec.md"))
			}
		}
	}

	for _, cand := range candidates {
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			sourceSpecPath = cand
			break
		}
	}

	if sourceSpecPath == "" {
		return nil, fmt.Errorf("no se encontró el archivo spec.md para el cambio '%s'", changeName)
	}

	contentBytes, err := os.ReadFile(sourceSpecPath)
	if err != nil {
		return nil, fmt.Errorf("error leyendo spec de origen '%s': %w", sourceSpecPath, err)
	}

	// 2. Si la especificación viva no existe, inicializarla
	if err := os.MkdirAll(domainDir, 0755); err != nil {
		return nil, fmt.Errorf("error creando directorio de dominio '%s': %w", domainDir, err)
	}

	headerNote := fmt.Sprintf("<!-- Especificación Viva generada orgánicamente (Zero-Doc Cold Start) a partir de '%s' -->\n\n", changeName)
	newContent := headerNote + string(contentBytes)

	if err := os.WriteFile(targetSpecPath, []byte(newContent), 0644); err != nil {
		return nil, fmt.Errorf("error escribiendo especificación viva en '%s': %w", targetSpecPath, err)
	}

	// 3. Indexar la nueva especificación
	entry, _, err := s.indexer.ParseSpecFile(targetSpecPath, targetDomain, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("error indexando la especificación sintetizada: %w", err)
	}

	return entry, nil
}
