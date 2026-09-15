package livingdoc

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	titleRegex       = regexp.MustCompile(`^#\s+(.+)$`)
	requirementRegex = regexp.MustCompile(`^###\s+Requirement:\s*(.+?)(?:\s*\((REQ-[^\)]+)\))?\s*$`)
	scenarioRegex    = regexp.MustCompile(`^####\s+Scenario:\s*(.+)$`)
)

// Indexer analiza especificaciones canónicas y compila el índice maestro.
type Indexer struct{}

// NewIndexer crea una nueva instancia del indexador de documentación viva.
func NewIndexer() *Indexer {
	return &Indexer{}
}

// ScanSpecs recorre el directorio raíz de especificaciones vivas e indexa cada spec.md.
func (idx *Indexer) ScanSpecs(specsRoot string) (*LivingCatalog, []string, error) {
	catalog := &LivingCatalog{
		Specs:    make([]LivingSpecEntry, 0),
		LastSync: time.Now().UTC(),
	}
	var allWarnings []string

	if fi, err := os.Stat(specsRoot); err != nil || !fi.IsDir() {
		return catalog, nil, nil
	}

	err := filepath.WalkDir(specsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		if d.Name() == "spec.md" {
			info, err := d.Info()
			modTime := time.Now()
			if err == nil {
				modTime = info.ModTime()
			}

			domain := filepath.Base(filepath.Dir(path))
			entry, warnings, err := idx.ParseSpecFile(path, domain, modTime)
			if err == nil && entry != nil {
				catalog.Specs = append(catalog.Specs, *entry)
				catalog.TotalRequirements += len(entry.Requirements)
				catalog.TotalScenarios += entry.TotalScenarios
				allWarnings = append(allWarnings, warnings...)
			}
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	sort.Slice(catalog.Specs, func(i, j int) bool {
		return catalog.Specs[i].Domain < catalog.Specs[j].Domain
	})

	return catalog, allWarnings, nil
}

// ParseSpecFile lee y analiza sintácticamente un archivo spec.md.
func (idx *Indexer) ParseSpecFile(filePath, domain string, modTime time.Time) (*LivingSpecEntry, []string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, err
	}

	entry, warnings := idx.ParseSpecContent(string(content), domain, filePath, modTime)
	return entry, warnings, nil
}

// ParseSpecContent analiza el texto Markdown de una especificación extrayendo sus requerimientos y escenarios.
func (idx *Indexer) ParseSpecContent(content, domain, filePath string, modTime time.Time) (*LivingSpecEntry, []string) {
	entry := &LivingSpecEntry{
		Domain:       domain,
		FilePath:     filepath.ToSlash(filePath),
		Requirements: make([]RequirementMeta, 0),
		LastModified: modTime,
	}
	var warnings []string

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	var currentReq *RequirementMeta

	for scanner.Scan() {
		lineNum++
		line := strings.TrimRight(scanner.Text(), "\r\n")
		trimmed := strings.TrimSpace(line)

		// 1. Título principal
		if entry.Title == "" && strings.HasPrefix(trimmed, "# ") {
			if match := titleRegex.FindStringSubmatch(trimmed); len(match) > 1 {
				entry.Title = strings.TrimSpace(match[1])
			}
			continue
		}

		// 2. Requerimiento canónico
		if strings.HasPrefix(trimmed, "### Requirement:") {
			if currentReq != nil {
				if len(currentReq.Scenarios) == 0 {
					warnings = append(warnings, fmt.Sprintf("Dominio '%s': Requerimiento '%s' (%s) no tiene escenarios BDD asociados", domain, currentReq.Title, currentReq.ID))
				}
				entry.Requirements = append(entry.Requirements, *currentReq)
			}

			reqID := ""
			reqTitle := strings.TrimPrefix(trimmed, "### Requirement:")
			reqTitle = strings.TrimSpace(reqTitle)

			if match := requirementRegex.FindStringSubmatch(trimmed); len(match) > 1 {
				reqTitle = strings.TrimSpace(match[1])
				if len(match) > 2 && match[2] != "" {
					reqID = match[2]
				}
			}

			if reqID == "" {
				reqID = fmt.Sprintf("REQ-%s-%d", domain, len(entry.Requirements)+1)
			}

			currentReq = &RequirementMeta{
				ID:         reqID,
				Title:      reqTitle,
				Scenarios:  make([]string, 0),
				LineNumber: lineNum,
			}
			continue
		}

		// 3. Escenario BDD
		if currentReq != nil && strings.HasPrefix(trimmed, "#### Scenario:") {
			if match := scenarioRegex.FindStringSubmatch(trimmed); len(match) > 1 {
				scName := strings.TrimSpace(match[1])
				currentReq.Scenarios = append(currentReq.Scenarios, scName)
				entry.TotalScenarios++
			}
			continue
		}

		// 4. Capturar descripción inicial
		if entry.Description == "" && currentReq == nil && trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "---") && !strings.HasPrefix(trimmed, ">") {
			entry.Description = trimmed
		}
	}

	if currentReq != nil {
		if len(currentReq.Scenarios) == 0 {
			warnings = append(warnings, fmt.Sprintf("Dominio '%s': Requerimiento '%s' (%s) no tiene escenarios BDD asociados", domain, currentReq.Title, currentReq.ID))
		}
		entry.Requirements = append(entry.Requirements, *currentReq)
	}

	if entry.Title == "" {
		entry.Title = domain
	}

	return entry, warnings
}

// RenderIndexMarkdown genera la representación Markdown canónica de openspec/INDEX.md.
func (idx *Indexer) RenderIndexMarkdown(catalog *LivingCatalog) string {
	var sb strings.Builder

	sb.WriteString("# Catálogo Maestro de Especificaciones Vivas — Axiom\n\n")
	sb.WriteString("> **Proyecto:** Axiom (Spec-Driven Development Platform)\n")
	sb.WriteString(fmt.Sprintf("> **Última Sincronización:** %s\n", catalog.LastSync.Format("2006-01-02 15:04:05 MST")))
	sb.WriteString(fmt.Sprintf("> **Total Dominios:** %d | **Total Requerimientos:** %d | **Total Escenarios BDD:** %d\n\n",
		len(catalog.Specs), catalog.TotalRequirements, catalog.TotalScenarios))
	sb.WriteString("---\n\n")

	sb.WriteString("## Resumen de Especificaciones Vivas\n\n")
	sb.WriteString("| Dominio | Título de la Especificación | Reqs | Escenarios | Enlace |\n")
	sb.WriteString("| :--- | :--- | :---: | :---: | :--- |\n")

	for _, spec := range catalog.Specs {
		link := fmt.Sprintf("specs/%s/spec.md", spec.Domain)
		sb.WriteString(fmt.Sprintf("| `%s` | %s | %d | %d | [Ver Spec](%s) |\n",
			spec.Domain, spec.Title, len(spec.Requirements), spec.TotalScenarios, link))
	}

	sb.WriteString("\n---\n\n")
	sb.WriteString("## Detalle de Capacidades y Requerimientos por Dominio\n\n")

	for _, spec := range catalog.Specs {
		sb.WriteString(fmt.Sprintf("### Dominio: `%s` — %s\n\n", spec.Domain, spec.Title))
		if spec.Description != "" {
			sb.WriteString(fmt.Sprintf("%s\n\n", spec.Description))
		}
		sb.WriteString(fmt.Sprintf("**Archivo:** [`specs/%s/spec.md`](specs/%s/spec.md)\n\n", spec.Domain, spec.Domain))

		if len(spec.Requirements) == 0 {
			sb.WriteString("_Sin requerimientos canónicos registrados._\n\n")
		} else {
			for _, req := range spec.Requirements {
				sb.WriteString(fmt.Sprintf("- **[%s]** %s\n", req.ID, req.Title))
				for _, sc := range req.Scenarios {
					sb.WriteString(fmt.Sprintf("  - *Escenario BDD:* %s\n", sc))
				}
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// SyncIndex escanea el directorio de especificaciones vivas y genera o actualiza el archivo INDEX.md.
func (idx *Indexer) SyncIndex(specsRoot, indexFilePath string) (*SyncReport, error) {
	catalog, warnings, err := idx.ScanSpecs(specsRoot)
	if err != nil {
		return nil, fmt.Errorf("error escaneando especificaciones vivas: %w", err)
	}

	mdContent := idx.RenderIndexMarkdown(catalog)

	if err := os.MkdirAll(filepath.Dir(indexFilePath), 0755); err != nil {
		return nil, fmt.Errorf("error asegurando directorio para INDEX.md: %w", err)
	}

	if err := os.WriteFile(indexFilePath, []byte(mdContent), 0644); err != nil {
		return nil, fmt.Errorf("error escribiendo INDEX.md en '%s': %w", indexFilePath, err)
	}

	return &SyncReport{
		SpecsCount:        len(catalog.Specs),
		RequirementsCount: catalog.TotalRequirements,
		ScenariosCount:    catalog.TotalScenarios,
		IndexPath:         indexFilePath,
		SyncedAt:          catalog.LastSync,
		Warnings:          warnings,
	}, nil
}
