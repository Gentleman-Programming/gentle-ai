package dashboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/autoskill"
	"github.com/gentleman-programming/gentle-ai/v2/internal/handoff"
	"github.com/gentleman-programming/gentle-ai/v2/internal/livingdoc"
	"github.com/gentleman-programming/gentle-ai/v2/internal/multirole"
	"github.com/gentleman-programming/gentle-ai/v2/internal/semantic"
	"github.com/gentleman-programming/gentle-ai/v2/internal/workspace"
)

// Service provee la lógica de lectura y agregación del estado de Axiom.
type Service struct {
	rootPath         string
	autoskillManager *autoskill.Manager
	semanticService  *semantic.Service
	livingdocService *livingdoc.Service
}

// NewService crea una nueva instancia del servicio para el workspace dado.
func NewService(rootPath string) *Service {
	if rootPath == "" {
		rootPath = "."
	}
	return &Service{
		rootPath:         rootPath,
		autoskillManager: autoskill.NewManager(rootPath, nil, nil, nil),
		semanticService:  semantic.NewService(rootPath, nil, nil),
		livingdocService: livingdoc.NewService(rootPath, nil, nil),
	}
}

// GetWorkspace obtiene la información del espacio de trabajo y su estado de cumplimiento.
func (s *Service) GetWorkspace() (*WorkspaceDTO, error) {
	configPath := filepath.Join(s.rootPath, "axiom.yaml")
	cfg, err := workspace.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("error cargando axiom.yaml: %w", err)
	}

	valReport, valErr := workspace.Validate(workspace.DefaultFS(), cfg, s.rootPath)
	compliant := (valErr == nil && valReport != nil && valReport.Valid)
	message := "Workspace conforme con la topología declarada"
	if !compliant {
		if valErr != nil {
			message = valErr.Error()
		} else if valReport != nil && len(valReport.Errors) > 0 {
			message = strings.Join(valReport.Errors, "; ")
		}
	}

	rolesMap := make(map[string]RoleMeta)
	for id, r := range cfg.Roles {
		var repoPaths []string
		for _, repo := range r.Repositories {
			repoPaths = append(repoPaths, repo.Path)
		}

		rolesMap[id] = RoleMeta{
			Name:         r.Name,
			GatePolicy:   "blocking",
			Repositories: repoPaths,
			Tech:         r.Tech,
		}
	}

	return &WorkspaceDTO{
		Name:            cfg.Workspace.Name,
		Topology:        string(cfg.Workspace.Topology),
		SpecsRepository: cfg.Workspace.SpecsRepository,
		Root:            s.rootPath,
		Roles:           rolesMap,
		Compliant:       compliant,
		Message:         message,
	}, nil
}

// GetIncrements escanea y lista los incrementos activos y archivados.
func (s *Service) GetIncrements() ([]IncrementSummaryDTO, error) {
	var list []IncrementSummaryDTO

	// 1. Escanear cambios activos en openspec/changes/
	activeDir := filepath.Join(s.rootPath, "openspec", "changes")
	if entries, err := os.ReadDir(activeDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() || e.Name() == "archive" || e.Name() == "e2e-cumulative" {
				continue
			}
			incPath := filepath.Join(activeDir, e.Name())
			summary := s.inspectIncrement(incPath, e.Name(), "active")
			list = append(list, summary)
		}
	}

	// 2. Escanear cambios archivados en openspec/changes/archive/
	archiveDir := filepath.Join(s.rootPath, "openspec", "changes", "archive")
	if entries, err := os.ReadDir(archiveDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			incPath := filepath.Join(archiveDir, e.Name())
			summary := s.inspectIncrement(incPath, e.Name(), "archived")
			list = append(list, summary)
		}
	}

	// Ordenar: activos primero, luego archivados en orden inverso
	sort.Slice(list, func(i, j int) bool {
		if list[i].Type != list[j].Type {
			return list[i].Type == "active"
		}
		return list[i].Name > list[j].Name
	})

	return list, nil
}

func (s *Service) inspectIncrement(path, name, kind string) IncrementSummaryDTO {
	phase := "explore"
	date := ""

	// Extraer fecha de archivo si sigue el patrón YYYY-MM-DD-...
	parts := strings.SplitN(name, "-", 4)
	if len(parts) >= 4 && len(parts[0]) == 4 && len(parts[1]) == 2 && len(parts[2]) == 2 {
		date = fmt.Sprintf("%s-%s-%s", parts[0], parts[1], parts[2])
	}

	hasProposal := fileExists(filepath.Join(path, "proposal.md"))
	hasSpec := fileExists(filepath.Join(path, "spec.md"))
	hasDesign := fileExists(filepath.Join(path, "design.md"))
	hasTasks := fileExists(filepath.Join(path, "tasks.md"))
	hasVerify := fileExists(filepath.Join(path, "verify-report.md"))
	hasArchive := fileExists(filepath.Join(path, "archive-report.md"))

	if kind == "archived" || hasArchive {
		phase = "archive"
	} else if hasVerify {
		phase = "verify"
	} else if hasTasks {
		phase = "apply"
	} else if hasDesign {
		phase = "tasks"
	} else if hasSpec {
		phase = "design"
	} else if hasProposal {
		phase = "spec"
	}

	tasksTotal, tasksCompleted, pct := 0, 0, 0
	if hasTasks {
		if data, err := os.ReadFile(filepath.Join(path, "tasks.md")); err == nil {
			prog := multirole.CountTasks(string(data))
			tasksTotal = prog.Total
			tasksCompleted = prog.Completed
			pct = int(prog.Percent)
		}
	}

	return IncrementSummaryDTO{
		Name:           name,
		Type:           kind,
		Phase:          phase,
		TasksTotal:     tasksTotal,
		TasksCompleted: tasksCompleted,
		ProgressPct:    pct,
		Date:           date,
	}
}

// FindIncrementPath busca la ruta de un incremento por nombre o prefijo/sufijo.
func (s *Service) FindIncrementPath(name string) (string, string, error) {
	// 1. Probar en activos
	activePath := filepath.Join(s.rootPath, "openspec", "changes", name)
	if dirExists(activePath) {
		return activePath, "active", nil
	}

	// 2. Probar en archivados exacto
	archivePath := filepath.Join(s.rootPath, "openspec", "changes", "archive", name)
	if dirExists(archivePath) {
		return archivePath, "archived", nil
	}

	// 3. Probar en archivados buscando por sufijo (ej. 'inc-01' en '2026-09-14-inc-01-...')
	archiveDir := filepath.Join(s.rootPath, "openspec", "changes", "archive")
	if entries, err := os.ReadDir(archiveDir); err == nil {
		for _, e := range entries {
			if e.IsDir() && (strings.Contains(e.Name(), name) || strings.HasSuffix(e.Name(), name)) {
				return filepath.Join(archiveDir, e.Name()), "archived", nil
			}
		}
	}

	return "", "", fmt.Errorf("incremento '%s' no encontrado", name)
}

// GetIncrementDetail retorna el detalle completo de un cambio y el contenido de sus artefactos.
func (s *Service) GetIncrementDetail(name string) (*IncrementDetailDTO, error) {
	path, kind, err := s.FindIncrementPath(name)
	if err != nil {
		return nil, err
	}

	summary := s.inspectIncrement(path, filepath.Base(path), kind)

	dto := &IncrementDetailDTO{
		Summary:       summary,
		HasProposal:   fileExists(filepath.Join(path, "proposal.md")),
		HasSpec:       fileExists(filepath.Join(path, "spec.md")),
		HasDesign:     fileExists(filepath.Join(path, "design.md")),
		HasTasks:      fileExists(filepath.Join(path, "tasks.md")),
		HasVerify:     fileExists(filepath.Join(path, "verify-report.md")),
		HasArchive:    fileExists(filepath.Join(path, "archive-report.md")),
	}

	if dto.HasProposal {
		dto.Proposal, _ = readFileString(filepath.Join(path, "proposal.md"))
	}
	if dto.HasSpec {
		dto.Spec, _ = readFileString(filepath.Join(path, "spec.md"))
	}
	if dto.HasDesign {
		dto.Design, _ = readFileString(filepath.Join(path, "design.md"))
	}
	if dto.HasTasks {
		dto.TasksContent, _ = readFileString(filepath.Join(path, "tasks.md"))
	}
	if dto.HasVerify {
		dto.VerifyReport, _ = readFileString(filepath.Join(path, "verify-report.md"))
	}
	if dto.HasArchive {
		dto.ArchiveReport, _ = readFileString(filepath.Join(path, "archive-report.md"))
	}

	// Cargar barrera multi-rol si es posible
	cfg, errCfg := workspace.LoadConfig(filepath.Join(s.rootPath, "axiom.yaml"))
	if errCfg == nil && dto.HasDesign {
		roles, errRoles := multirole.DetectRoles(filepath.Join(path, "design.md"), cfg)
		if errRoles == nil {
			barrier, errBarrier := multirole.EvaluateBarrier(path, name, roles)
			if errBarrier == nil {
				dto.BarrierReport = barrier
			}
		}
	}

	return dto, nil
}

// GetRoleStatus obtiene el estado de los roles y la barrera de sincronización de un cambio.
func (s *Service) GetRoleStatus(changeName string) (*multirole.BarrierReport, error) {
	path, _, err := s.FindIncrementPath(changeName)
	if err != nil {
		return nil, err
	}

	cfg, err := workspace.LoadConfig(filepath.Join(s.rootPath, "axiom.yaml"))
	if err != nil {
		return nil, fmt.Errorf("error cargando configuración de roles: %w", err)
	}

	designFile := filepath.Join(path, "design.md")
	roles, err := multirole.DetectRoles(designFile, cfg)
	if err != nil {
		return nil, fmt.Errorf("no se pudieron detectar los roles del cambio: %w", err)
	}

	return multirole.EvaluateBarrier(path, changeName, roles)
}

// GetHandoff obtiene el relevo estructurado handoff.md de un cambio.
func (s *Service) GetHandoff(changeName string) (*handoff.Handoff, error) {
	path, _, err := s.FindIncrementPath(changeName)
	if err != nil {
		return nil, err
	}

	handoffPath := filepath.Join(path, "handoff.md")
	if !fileExists(handoffPath) {
		return nil, errors.New("no existe handoff.md en el cambio especificado")
	}

	data, err := os.ReadFile(handoffPath)
	if err != nil {
		return nil, fmt.Errorf("error leyendo handoff.md: %w", err)
	}

	return handoff.Parse(bytes.NewReader(data))
}

// GetSkills escanea y cataloga las skills locales del proyecto.
func (s *Service) GetSkills() ([]SkillDTO, error) {
	var skills []SkillDTO

	scanDir := func(base string) {
		fullBase := filepath.Join(s.rootPath, base)
		entries, err := os.ReadDir(fullBase)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			skillFile := filepath.Join(fullBase, e.Name(), "SKILL.md")
			if fileExists(skillFile) {
				desc, trigger := extractSkillMeta(skillFile)
				relPath, _ := filepath.Rel(s.rootPath, skillFile)
				skills = append(skills, SkillDTO{
					Name:        e.Name(),
					Path:        filepath.ToSlash(relPath),
					Description: desc,
					Trigger:     trigger,
				})
			}
		}
	}

	scanDir("skills")
	scanDir(filepath.Join("internal", "assets", "skills"))

	return skills, nil
}

func extractSkillMeta(path string) (string, string) {
	content, err := readFileString(path)
	if err != nil {
		return "Skill de desarrollo", ""
	}

	desc := ""
	trigger := ""
	lines := strings.Split(content, "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "description:") {
			desc = strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
			desc = strings.Trim(desc, `"'`)
		}
		if strings.HasPrefix(trimmed, "trigger:") {
			trigger = strings.TrimSpace(strings.TrimPrefix(trimmed, "trigger:"))
			trigger = strings.Trim(trigger, `"'`)
		}
	}

	if desc == "" && len(lines) > 2 {
		desc = "Skill para automatización de flujos y tareas de Axiom"
	}
	return desc, trigger
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func readFileString(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// GetSkillsInbox retorna las propuestas pendientes de revisión en el buzón transitorio.
func (s *Service) GetSkillsInbox() ([]SkillProposalDTO, error) {
	proposals, err := s.autoskillManager.ListInbox()
	if err != nil {
		return nil, err
	}

	var dtos []SkillProposalDTO
	for _, p := range proposals {
		dtos = append(dtos, SkillProposalDTO{
			Name:          p.Metadata.Name,
			Origin:        string(p.Metadata.Origin),
			Source:        p.Metadata.Source,
			Verified:      p.Metadata.Verified,
			Role:          p.Metadata.Role,
			DetectedBy:    p.Metadata.DetectedBy,
			Justification: p.Metadata.Justification,
			CreatedAt:     p.Metadata.CreatedAt.Format(time.RFC3339),
			SkillMD:       p.SkillMD,
			SHA256:        p.Metadata.SHA256,
		})
	}
	if dtos == nil {
		dtos = make([]SkillProposalDTO, 0)
	}
	return dtos, nil
}

// ScanSkills ejecuta el escaneo de tecnologías y minería heurística depositando candidatos en el buzón.
func (s *Service) ScanSkills(ctx context.Context, role string, offline bool) (*autoskill.ScanReport, error) {
	return s.autoskillManager.Scan(ctx, role, offline)
}

// ApproveSkill aprueba y promociona una skill del buzón a skills/.
func (s *Service) ApproveSkill(name string) error {
	return s.autoskillManager.Approve(name)
}

// RejectSkill descarta y purga una propuesta del buzón.
func (s *Service) RejectSkill(name string) error {
	return s.autoskillManager.Reject(name)
}

// GetSemanticStatus obtiene el diagnóstico del entorno semántico y métricas del workspace.
func (s *Service) GetSemanticStatus(ctx context.Context) (*semantic.SemanticStatus, error) {
	return s.semanticService.GetStatus(ctx)
}

// FindSemanticSymbols consulta y filtra los símbolos en el workspace.
func (s *Service) FindSemanticSymbols(query semantic.SemanticQuery) ([]semantic.SymbolItem, error) {
	return s.semanticService.FindSymbols(query)
}

// InspectSemanticDependencies obtiene las relaciones de dependencia entre paquetes.
func (s *Service) InspectSemanticDependencies(role string) ([]semantic.DependencyRelation, error) {
	return s.semanticService.InspectDependencies(role)
}

// GetLivingSpecs obtiene el catálogo maestro de especificaciones vivas.
func (s *Service) GetLivingSpecs(ctx context.Context) (*livingdoc.LivingCatalog, error) {
	return s.livingdocService.GetCatalog(ctx)
}

// GetLivingSpecDetail obtiene el detalle y contenido de una especificación viva por dominio.
func (s *Service) GetLivingSpecDetail(domain string) (*livingdoc.LivingSpecEntry, string, error) {
	return s.livingdocService.GetSpecDetail(domain)
}

// SyncLivingDocs sincroniza y regenera el índice de especificaciones vivas.
func (s *Service) SyncLivingDocs(ctx context.Context) (*livingdoc.SyncReport, error) {
	return s.livingdocService.Sync(ctx)
}

