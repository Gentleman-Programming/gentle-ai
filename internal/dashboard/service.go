package dashboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/app"
	"github.com/gentleman-programming/gentle-ai/v3/internal/autoskill"
	"github.com/gentleman-programming/gentle-ai/v3/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v3/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/v3/internal/handoff"
	"github.com/gentleman-programming/gentle-ai/v3/internal/hub"
	"github.com/gentleman-programming/gentle-ai/v3/internal/livingdoc"
	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole"
	"github.com/gentleman-programming/gentle-ai/v3/internal/semantic"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
)

// Service provee la lógica de lectura y agregación del estado de Axiom.
type Service struct {
	mu               sync.RWMutex
	rootPath         string
	hubManager       *hub.Manager
	hubDetector      *hub.Detector
	autoskillManager *autoskill.Manager
	semanticService  *semantic.Service
	livingdocService *livingdoc.Service
}

// NewService crea una nueva instancia del servicio para el workspace dado.
func NewService(rootPath string) *Service {
	if rootPath == "" {
		rootPath = "."
	}
	absRoot, err := filepath.Abs(rootPath)
	if err == nil {
		rootPath = absRoot
	}

	hubMgr, _ := hub.NewManager("")
	hubDet := hub.NewDetector()

	s := &Service{
		rootPath:         rootPath,
		hubManager:       hubMgr,
		hubDetector:      hubDet,
		autoskillManager: newAutoskillManager(rootPath),
		semanticService:  semantic.NewService(rootPath, nil, nil),
		livingdocService: livingdoc.NewService(rootPath, nil, nil),
	}

	// Si el directorio tiene axiom.yaml y tenemos hubManager, auto-registrarlo
	if hubMgr != nil && fileExists(filepath.Join(rootPath, "axiom.yaml")) {
		_, _ = hubMgr.Register(rootPath, filepath.Base(rootPath), "monorepo-embedded")
	}

	return s
}

// NewServiceWithHub instancia el servicio inyectando explícitamente el gestor de Hub.
func NewServiceWithHub(rootPath string, hubMgr *hub.Manager) *Service {
	if rootPath == "" {
		rootPath = "."
	}
	absRoot, err := filepath.Abs(rootPath)
	if err == nil {
		rootPath = absRoot
	}

	hubDet := hub.NewDetector()
	return &Service{
		rootPath:         rootPath,
		hubManager:       hubMgr,
		hubDetector:      hubDet,
		autoskillManager: newAutoskillManager(rootPath),
		semanticService:  semantic.NewService(rootPath, nil, nil),
		livingdocService: livingdoc.NewService(rootPath, nil, nil),
	}
}

func (s *Service) getRootPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rootPath
}

// GetWorkspace obtiene la información del espacio de trabajo y su estado de cumplimiento.
func (s *Service) GetWorkspace() (*WorkspaceDTO, error) {
	curPath := s.getRootPath()
	configPath := filepath.Join(curPath, "axiom.yaml")

	cfg, err := workspace.LoadConfig(configPath)
	if err != nil {
		// Proyecto sin axiom.yaml: proveer información descriptiva para el Zero-Config Welcome
		detected, _ := s.hubDetector.Detect(curPath)
		return &WorkspaceDTO{
			Name:         filepath.Base(curPath),
			Topology:     "unconfigured",
			Root:         curPath,
			Roles:        make(map[string]RoleMeta),
			Compliant:    false,
			Message:      "Este proyecto aún no cuenta con un archivo axiom.yaml configurado.",
			IsConfigured: false,
			DetectedTech: detected,
		}, nil
	}

	valReport, valErr := workspace.Validate(workspace.DefaultFS(), cfg, curPath)
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
		Root:            curPath,
		Roles:           rolesMap,
		Compliant:       compliant,
		Message:         message,
		IsConfigured:    true,
	}, nil
}

// GetIncrements escanea y lista los incrementos activos y archivados.
func (s *Service) GetIncrements() ([]IncrementSummaryDTO, error) {
	var list []IncrementSummaryDTO
	root := s.getRootPath()

	// 1. Escanear cambios activos en openspec/changes/
	activeDir := filepath.Join(root, "openspec", "changes")
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
	archiveDir := filepath.Join(root, "openspec", "changes", "archive")
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
	root := s.getRootPath()

	// 1. Probar en activos
	activePath := filepath.Join(root, "openspec", "changes", name)
	if dirExists(activePath) {
		return activePath, "active", nil
	}

	// 2. Probar en archivados exacto
	archivePath := filepath.Join(root, "openspec", "changes", "archive", name)
	if dirExists(archivePath) {
		return archivePath, "archived", nil
	}

	// 3. Probar en archivados buscando por sufijo (ej. 'inc-01' en '2026-09-14-inc-01-...')
	archiveDir := filepath.Join(root, "openspec", "changes", "archive")
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

	root := s.getRootPath()
	summary := s.inspectIncrement(path, filepath.Base(path), kind)

	dto := &IncrementDetailDTO{
		Summary:     summary,
		HasProposal: fileExists(filepath.Join(path, "proposal.md")),
		HasSpec:     fileExists(filepath.Join(path, "spec.md")),
		HasDesign:   fileExists(filepath.Join(path, "design.md")),
		HasTasks:    fileExists(filepath.Join(path, "tasks.md")),
		HasVerify:   fileExists(filepath.Join(path, "verify-report.md")),
		HasArchive:  fileExists(filepath.Join(path, "archive-report.md")),
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
	cfg, errCfg := workspace.LoadConfig(filepath.Join(root, "axiom.yaml"))
	if errCfg == nil && dto.HasDesign {
		roles, errRoles := multirole.DetectRoles(filepath.Join(path, "design.md"), cfg)
		if errRoles == nil {
			report, errReport := multirole.EvaluateBarrier(path, filepath.Base(path), roles)
			if errReport == nil {
				dto.BarrierReport = report
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

	root := s.getRootPath()
	cfg, err := workspace.LoadConfig(filepath.Join(root, "axiom.yaml"))
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
	root := s.getRootPath()

	scanDir := func(base string) {
		fullBase := filepath.Join(root, base)
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
				relPath, _ := filepath.Rel(root, skillFile)
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
	scanDir(filepath.Join(".agents", "skills"))

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

// newAutoskillManager builds the autoskill manager with the production index
// regenerator wired in (REQ-22.13): Manager.Approve refreshes the unified
// skills index so a promoted skill is available immediately.
func newAutoskillManager(rootPath string) *autoskill.Manager {
	manager := autoskill.NewManager(rootPath, nil, nil, nil)
	manager.RegenerateIndex = app.RegenerateSkillsIndex
	return manager
}

// ScanSkills ejecuta el escaneo de tecnologías y minería heurística depositando candidatos en el buzón.
func (s *Service) ScanSkills(ctx context.Context, role string, offline bool) (*autoskill.ScanReport, error) {
	return s.autoskillManager.Scan(ctx, role, offline)
}

// ApproveSkill aprueba y promociona una skill del buzón a skills/. La
// regeneración del índice vive en Manager.Approve (REQ-22.13); su fallo se
// devuelve como aviso y nunca deshace la promoción (D-12).
func (s *Service) ApproveSkill(name string) (warning string, err error) {
	outcome, err := s.autoskillManager.Approve(name)
	if err != nil {
		return "", err
	}
	if outcome.RegenerateError != nil {
		return outcome.RegenerateError.Error(), nil
	}
	return "", nil
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

// SwitchWorkspace conmuta en tiempo de ejecución el proyecto gestionado por el servicio.
func (s *Service) SwitchWorkspace(targetPath string) (*WorkspaceDTO, error) {
	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		return nil, fmt.Errorf("ruta de workspace inválida: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("el directorio '%s' no existe en el sistema", absPath)
	}

	s.mu.Lock()
	s.rootPath = absPath
	s.autoskillManager = newAutoskillManager(absPath)
	s.semanticService = semantic.NewService(absPath, nil, nil)
	s.livingdocService = livingdoc.NewService(absPath, nil, nil)
	s.mu.Unlock()

	if s.hubManager != nil {
		_, _ = s.hubManager.SetActive(absPath)
	}

	return s.GetWorkspace()
}

// GetProjects obtiene la lista de proyectos registrados en el Hub global y el activo.
func (s *Service) GetProjects() (*ProjectListDTO, error) {
	if s.hubManager == nil {
		mgr, err := hub.NewManager("")
		if err != nil {
			return nil, err
		}
		s.hubManager = mgr
	}

	projects, err := s.hubManager.List()
	if err != nil {
		return nil, err
	}

	activeID := ""
	activeRec, err := s.hubManager.GetActive()
	if err == nil && activeRec != nil {
		activeID = activeRec.ID
	}

	projInterfaces := make([]interface{}, len(projects))
	for i, p := range projects {
		projInterfaces[i] = p
	}

	return &ProjectListDTO{
		ActiveWorkspace: activeID,
		Projects:        projInterfaces,
	}, nil
}

// AddProject registra un proyecto existente en el Hub.
func (s *Service) AddProject(req ProjectAddRequest) (*hub.WorkspaceRecord, error) {
	if s.hubManager == nil {
		mgr, err := hub.NewManager("")
		if err != nil {
			return nil, err
		}
		s.hubManager = mgr
	}

	if req.Path == "" {
		return nil, errors.New("debes especificar la ruta del proyecto ('path')")
	}

	return s.hubManager.Register(req.Path, req.Name, req.Topology)
}

// InitProject inicializa un proyecto con axiom.yaml y lo registra.
func (s *Service) InitProject(req ProjectInitRequest) (*hub.InitResult, error) {
	targetPath := req.Path
	if targetPath == "" {
		targetPath = s.getRootPath()
	}

	ini := hub.NewInitializer(s.hubManager, s.hubDetector)
	res, err := ini.Init(hub.InitOptions{
		Path:     targetPath,
		Name:     req.Name,
		Topology: req.Topology,
		Roles:    req.Roles,
	})
	if err != nil {
		return nil, err
	}

	// Conmutar automáticamente el workspace activo al inicializado
	_, _ = s.SwitchWorkspace(targetPath)

	return res, nil
}

// MigrateIncompleteTasksToCumulative transfiere tareas pendientes de un rol no bloqueante a su incremento acumulativo correspondiente.
func (s *Service) MigrateIncompleteTasksToCumulative(changeName, role string) (int, error) {
	path, _, err := s.FindIncrementPath(changeName)
	if err != nil {
		return 0, err
	}

	root := s.getRootPath()
	var tasksFiles []string

	// Buscar tasks.<role>.md o tasks.md
	roleTasksFile := filepath.Join(path, fmt.Sprintf("tasks.%s.md", role))
	if fileExists(roleTasksFile) {
		tasksFiles = append(tasksFiles, roleTasksFile)
	}
	generalTasksFile := filepath.Join(path, "tasks.md")
	if fileExists(generalTasksFile) && len(tasksFiles) == 0 {
		tasksFiles = append(tasksFiles, generalTasksFile)
	}

	if len(tasksFiles) == 0 {
		return 0, nil
	}

	var incompleteTasks []string
	for _, tf := range tasksFiles {
		data, err := os.ReadFile(tf)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		var modifiedLines []string
		fileChanged := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- [ ]") {
				incompleteTasks = append(incompleteTasks, trimmed)
				taskText := strings.TrimPrefix(trimmed, "- [ ]")
				taskText = strings.TrimSpace(taskText)
				indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				modifiedLines = append(modifiedLines, fmt.Sprintf("%s- [x] ~~(migrada a acumulativo %s)~~ %s", indent, role, taskText))
				fileChanged = true
			} else {
				modifiedLines = append(modifiedLines, line)
			}
		}
		if fileChanged {
			_ = os.WriteFile(tf, []byte(strings.Join(modifiedLines, "\n")), 0644)
		}
	}

	if len(incompleteTasks) == 0 {
		return 0, nil
	}

	// Directorio acumulativo para el rol
	specsRepo := "openspec"
	if cfg, err := workspace.LoadConfig(filepath.Join(root, "axiom.yaml")); err == nil && cfg.Workspace.SpecsRepository != "" {
		specsRepo = cfg.Workspace.SpecsRepository
	}

	cumulativeDir := filepath.Join(root, specsRepo, "changes", fmt.Sprintf("cumulative-%s", role))
	if err := os.MkdirAll(cumulativeDir, 0755); err != nil {
		return 0, fmt.Errorf("error creando directorio de incremento acumulativo: %w", err)
	}

	cumulativeTasksPath := filepath.Join(cumulativeDir, "tasks.md")
	var existingContent string
	if fileExists(cumulativeTasksPath) {
		if b, err := os.ReadFile(cumulativeTasksPath); err == nil {
			existingContent = string(b)
		}
	} else {
		existingContent = fmt.Sprintf("# Incremento Acumulativo de Deuda - Rol: %s\n\n> Tareas pendientes transferidas automáticamente desde incrementos archivados.\n\n", strings.ToUpper(role))
	}

	timestamp := time.Now().Format("2006-01-02")
	var sb strings.Builder
	sb.WriteString(existingContent)
	if !strings.HasSuffix(existingContent, "\n\n") {
		sb.WriteString("\n\n")
	}
	sb.WriteString(fmt.Sprintf("### Transferidas desde %s (%s)\n", changeName, timestamp))
	for _, t := range incompleteTasks {
		sb.WriteString(fmt.Sprintf("%s\n", t))
	}

	if err := os.WriteFile(cumulativeTasksPath, []byte(sb.String()), 0644); err != nil {
		return 0, fmt.Errorf("error guardando tareas acumulativas: %w", err)
	}

	return len(incompleteTasks), nil
}

// GetHubManager retorna el gestor de Hub asociado al servicio.
func (s *Service) GetHubManager() *hub.Manager {
	return s.hubManager
}

var validIncrementNameRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// CreateIncrement crea un nuevo cambio SDD con su plantilla inicial de proposal.md en español.
func (s *Service) CreateIncrement(req CreateIncrementRequest) (*CreateIncrementResponse, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if name == "" {
		return nil, fmt.Errorf("el nombre del incremento no puede estar vacío")
	}
	if !validIncrementNameRegex.MatchString(name) {
		return nil, fmt.Errorf("el nombre %q no es válido: debe estar en minúsculas kebab-case (ej. mi-cambio-funcional)", name)
	}

	root := s.getRootPath()
	activePath := filepath.Join(root, "openspec", "changes", name)
	if _, err := os.Stat(activePath); err == nil {
		return nil, fmt.Errorf("el incremento %q ya existe como cambio activo", name)
	}

	// Comprobar colisión con archivados
	archiveDir := filepath.Join(root, "openspec", "changes", "archive")
	if entries, err := os.ReadDir(archiveDir); err == nil {
		for _, e := range entries {
			if e.IsDir() && (e.Name() == name || strings.HasSuffix(e.Name(), "-"+name)) {
				return nil, fmt.Errorf("el incremento %q ya existe archivado (%s)", name, e.Name())
			}
		}
	}

	if err := os.MkdirAll(activePath, 0755); err != nil {
		return nil, fmt.Errorf("error al crear el directorio del incremento: %w", err)
	}

	intent := strings.TrimSpace(req.Intent)
	if intent == "" {
		intent = fmt.Sprintf("Implementación e integración de la funcionalidad %s bajo la metodología SDD.", name)
	}

	title := humanizeName(name)
	changeType := strings.TrimSpace(req.Type)
	if changeType == "" {
		changeType = "feature"
	}

	proposalContent := fmt.Sprintf(`# Propuesta: %s (%s)

## Propósito (Intent)

%s

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)
- Diseño, implementación y verificación de las capacidades de %s.
- Cobertura de pruebas unitarias y validación formal de requerimientos.

### Fuera de Alcance (Out of Scope)
- Cambios no relacionados directamente con los objetivos de esta iteración.

---

## Capacidades (Capabilities)

### Nuevas Capacidades
- `+"`%s`"+`: Funcionalidad principal introducida por el cambio.

---

## Enfoque de Implementación (Approach)
1. Exploración y definición de especificaciones con escenarios BDD en spec.md.
2. Diseño técnico detallado y arquitectura en design.md.
3. Desglose y seguimiento de tareas en tasks.md.
4. Verificación formal y consolidación de documentación viva en archive.
`, title, name, intent, name, name)

	if strings.TrimSpace(req.ProposalBody) != "" {
		proposalContent = req.ProposalBody // INC-19: bytes exactos del renderizador ODD, sin normalizar [D-02]
	}

	proposalPath := filepath.Join(activePath, "proposal.md")
	if err := os.WriteFile(proposalPath, []byte(proposalContent), 0644); err != nil {
		return nil, fmt.Errorf("error al escribir proposal.md: %w", err)
	}

	return &CreateIncrementResponse{
		Success: true,
		Name:    name,
		Path:    proposalPath,
		Message: fmt.Sprintf("Incremento %s inicializado correctamente con proposal.md", name),
	}, nil
}

// ContinueIncrement ejecuta la transición del despachador SDD sobre un cambio activo.
func (s *Service) ContinueIncrement(name string) (*IncrementActionResponse, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("el nombre del incremento es obligatorio")
	}
	root := s.getRootPath()
	_, kind, err := s.FindIncrementPath(name)
	if err != nil || kind != "active" {
		return nil, fmt.Errorf("el incremento %q no existe como cambio activo", name)
	}

	var stdout bytes.Buffer
	runErr := cli.RunSDDContinue([]string{name, "--cwd", root}, &stdout)
	outStr := stdout.String()
	if runErr != nil && outStr == "" {
		outStr = runErr.Error()
	}

	errMsg := ""
	if runErr != nil {
		errMsg = runErr.Error()
	}

	return &IncrementActionResponse{
		Success:    runErr == nil,
		ChangeName: name,
		Action:     "sdd-continue",
		Output:     outStr,
		Error:      errMsg,
	}, nil
}

// VerifyIncrement valida formalmente el reporte de verificación contra las especificaciones.
func (s *Service) VerifyIncrement(name string) (*IncrementActionResponse, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("el nombre del incremento es obligatorio")
	}
	targetPath, kind, err := s.FindIncrementPath(name)
	if err != nil || kind != "active" {
		return nil, fmt.Errorf("el incremento %q no existe como cambio activo", name)
	}

	verifyFile := filepath.Join(targetPath, "verify-report.md")
	if !fileExists(verifyFile) {
		return &IncrementActionResponse{
			Success:    false,
			ChangeName: name,
			Action:     "sdd-verify-validate",
			Output:     "No se encontró el archivo verify-report.md en el cambio. Se requiere completar la fase verify.",
			Error:      "verify-report.md ausente",
		}, nil
	}

	// La absorción de upstream (INC-20 F4, commit 62ce74b7) retira el
	// subcomando "sdd verify-validate": la verificación pasa a ser opcional
	// e informativa y deja de ser una compuerta independiente de archive.
	return &IncrementActionResponse{
		Success:    true,
		ChangeName: name,
		Action:     "sdd-verify-validate",
		Output:     "verify-report.md existe. La validación formal contra especificaciones ya no es una compuerta independiente de archive tras la absorción de upstream (verificación opcional, sin atestación).",
		Error:      "",
	}, nil
}

// CreateHandoff serializa y guarda un relevo formal estructurado en el cambio indicado.
func (s *Service) CreateHandoff(req CreateHandoffRequest) (*CreateHandoffResponse, error) {
	change := strings.TrimSpace(req.Change)
	if change == "" {
		return nil, fmt.Errorf("el nombre del cambio es obligatorio")
	}

	targetPath, kind, err := s.FindIncrementPath(change)
	if err != nil || kind != "active" {
		return nil, fmt.Errorf("el incremento %q no existe como cambio activo", change)
	}

	fromPhase := handoff.Phase(strings.ToLower(strings.TrimSpace(req.FromPhase)))
	toPhase := handoff.Phase(strings.ToLower(strings.TrimSpace(req.ToPhase)))
	if fromPhase == "" {
		fromPhase = handoff.PhaseDesign
	}
	if toPhase == "" {
		toPhase = handoff.PhaseApply
	}

	status := handoff.Status(strings.ToLower(strings.TrimSpace(req.Status)))
	if status == "" {
		status = handoff.StatusReady
	}

	fromRole := strings.TrimSpace(req.FromRole)
	if fromRole == "" {
		fromRole = "architect"
	}
	toRole := strings.TrimSpace(req.ToRole)
	if toRole == "" {
		toRole = "developer"
	}

	execSummary := strings.TrimSpace(req.ExecutiveSummary)
	if execSummary == "" {
		return nil, fmt.Errorf("el resumen ejecutivo es obligatorio")
	}
	artifacts := strings.TrimSpace(req.Artifacts)
	if artifacts == "" {
		artifacts = fmt.Sprintf("openspec/changes/%s", change)
	}
	decisions := strings.TrimSpace(req.Decisions)
	if decisions == "" {
		decisions = "Ninguna decisión crítica adicional documentada."
	}
	risks := strings.TrimSpace(req.Risks)
	if risks == "" {
		risks = "Sin riesgos identificados al momento del relevo."
	}
	instructions := strings.TrimSpace(req.Instructions)
	if instructions == "" {
		instructions = "Continuar con el avance de la siguiente fase según spec y tasks."
	}

	ho := &handoff.Handoff{
		Metadata: handoff.Metadata{
			Change:    change,
			FromPhase: fromPhase,
			ToPhase:   toPhase,
			FromRole:  fromRole,
			ToRole:    toRole,
			Timestamp: time.Now().UTC(),
			Status:    status,
		},
		Sections: handoff.Sections{
			ExecutiveSummary:   execSummary,
			Artifacts:          artifacts,
			Decisions:          decisions,
			RisksAndBlockers:   risks,
			DirectInstructions: instructions,
		},
	}

	if err := handoff.Validate(ho, nil); err != nil {
		return nil, fmt.Errorf("handoff inválido: %w", err)
	}

	filePath := filepath.Join(targetPath, "handoff.md")
	if err := handoff.WriteFile(filePath, ho); err != nil {
		return nil, fmt.Errorf("error al escribir handoff.md: %w", err)
	}

	return &CreateHandoffResponse{
		Success:  true,
		Change:   change,
		FilePath: filePath,
		Message:  fmt.Sprintf("Handoff para %s creado correctamente", change),
	}, nil
}

func humanizeName(name string) string {
	words := strings.Split(name, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// GetDoctorDiagnostics recopila diagnósticos de salud del ecosistema Axiom.
func (s *Service) GetDoctorDiagnostics() (*DoctorReport, error) {
	report := &DoctorReport{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Healthy:   true,
		Checks:    make([]DoctorCheck, 0),
	}

	// 1. Ejecutar cli.RunDoctor en un buffer para comprobar la verificación global
	var buf bytes.Buffer
	docErr := cli.RunDoctor(context.Background(), &buf)

	// 2. Detección de herramientas y agentes del ecosistema
	home, _ := os.UserHomeDir()
	det, _ := system.Detect(context.Background())
	for name, t := range det.Tools {
		status := "warning"
		details := "No detectado en el sistema"
		if t.Installed {
			status = "ok"
			details = fmt.Sprintf("Instalado en %s", t.Path)
		}
		cat := "agent"
		if name == "git" || name == "node" || name == "npm" || name == "brew" || name == "go" {
			cat = "tool"
		}
		report.Checks = append(report.Checks, DoctorCheck{
			Name:     name,
			Category: cat,
			Status:   status,
			Details:  details,
		})
	}

	// 4. Directorio base de usuario Axiom
	axiomDir := filepath.Join(home, ".axiom")
	if info, err := os.Stat(axiomDir); err == nil && info.IsDir() {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:     "directorio-axiom",
			Category: "environment",
			Status:   "ok",
			Details:  fmt.Sprintf("Directorio base activo: %s", axiomDir),
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:           "directorio-axiom",
			Category:       "environment",
			Status:         "warning",
			Details:        "Directorio ~/.axiom no existe aún",
			Recommendation: "Se creará automáticamente al ejecutar acciones de usuario",
		})
	}

	// 5. Workspace actual
	ws, err := s.GetWorkspace()
	if err == nil && ws.Compliant {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:     "workspace-axiom.yaml",
			Category: "environment",
			Status:   "ok",
			Details:  fmt.Sprintf("Topología: %s, Repositorio specs: %s", ws.Topology, ws.SpecsRepository),
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:           "workspace-axiom.yaml",
			Category:       "environment",
			Status:         "warning",
			Details:        "axiom.yaml no configurado o no conforme",
			Recommendation: "Usa 'axiom init' o el botón de inicialización",
		})
	}

	if docErr != nil {
		report.Healthy = false
	}
	return report, nil
}

// RunSync ejecuta la sincronización de configuraciones y reglas de agentes.
func (s *Service) RunSync() (*EcosystemActionResponse, error) {
	var buf bytes.Buffer
	err := app.RunArgs([]string{"sync"}, &buf)
	rawOut := strings.TrimSpace(buf.String())
	var lines []string
	if rawOut != "" {
		lines = strings.Split(rawOut, "\n")
	}
	if err != nil {
		return &EcosystemActionResponse{
			Success: false,
			Action:  "sync",
			Message: "Error durante la sincronización",
			Error:   err.Error(),
			Output:  lines,
		}, nil
	}
	return &EcosystemActionResponse{
		Success: true,
		Action:  "sync",
		Message: "Sincronización completada exitosamente",
		Output:  lines,
	}, nil
}

// upgradeSequenceReportFn y upgradeSequenceSyncFn son las dos primitivas que
// compone la cadena upgrade->sync (D-06). Son seams a nivel de paquete para que
// los tests fijen la regla de salto sin sustituir binarios ni invocar un sync
// real.
var (
	upgradeSequenceReportFn = app.RunUpgradeReport
	upgradeSequenceSyncFn   = func(s *Service) (*EcosystemActionResponse, error) { return s.RunSync() }
)

// upgradeSequenceLiteral es el identificador de la cadena que este endpoint
// ejecuta (spec §2.2).
const upgradeSequenceLiteral = "upgrade->sync"

// RunUpgradeSequence ejecuta la cadena upgrade -> sync (REQ-22.4) y devuelve el
// reporte consolidado de ambas fases.
//
// La fase upgrade es solo-binario: corresponde exactamente al comportamiento de
// `axiom upgrade`, que NO invoca install ni sync (REQ-22.5). La fase sync se
// ejecuta solo cuando la regla de salto compartida lo permite; en caso contrario
// queda `executed: false` con su motivo explícito (REQ-22.6, D-08).
//
// Devuelve error solo cuando el servicio no logra producir reporte alguno
// (spec §2.4): un fallo de la fase upgrade es un reporte válido, no un 500.
func (s *Service) RunUpgradeSequence() (*EcosystemActionResponse, error) {
	var upBuf bytes.Buffer
	upReport, upErr := upgradeSequenceReportFn(context.Background(), &upBuf)
	upLines := outputLines(upBuf.String())

	if upErr != nil && upReport.Status == "" {
		// Sin reporte alguno: ni siquiera el resultado de la primera fase.
		return &EcosystemActionResponse{
			Success: false,
			Action:  "upgrade",
			Message: "Error durante la actualización de herramientas",
			Error:   upErr.Error(),
			Output:  upLines,
		}, upErr
	}

	upgradePhase := UpgradePhaseReport{
		Success:         upReport.Status != app.UpgradeStatusFailed,
		Status:          upReport.Status,
		RestartRequired: upReport.RestartRequired,
		ManualHint:      upReport.ManualHint,
		Output:          upLines,
	}
	if upErr != nil {
		upgradePhase.Success = false
		upgradePhase.Error = upErr.Error()
		if upgradePhase.Status == "" {
			upgradePhase.Status = app.UpgradeStatusFailed
		}
	}

	skip, reason := app.ResolveSyncSkip(upReport)
	syncPhase := SyncPhaseReport{Success: false, SkippedReason: reason}
	if !skip {
		syncPhase.Executed = true
		syncPhase.SkippedReason = ""
		syncResp, syncErr := upgradeSequenceSyncFn(s)
		if syncErr != nil {
			syncPhase.Error = syncErr.Error()
		} else if syncResp != nil {
			syncPhase.Success = syncResp.Success
			syncPhase.Output = syncResp.Output
			syncPhase.Error = syncResp.Error
		}
	}

	// Regla §2.3.5: el éxito superior es true cuando toda fase ejecutada tuvo
	// éxito, incluso si sync quedó omitido por restart-required; false si alguna
	// fase ejecutada falló o si sync quedó omitido por upgrade-failed.
	overallSuccess := upgradePhase.Success && (syncPhase.Success || syncPhase.SkippedReason == "restart-required")

	resp := &EcosystemActionResponse{
		Success:  overallSuccess,
		Action:   "upgrade",
		Message:  upgradeSequenceMessage(upgradePhase, syncPhase),
		Output:   append(append([]string{}, upLines...), syncPhase.Output...),
		Sequence: upgradeSequenceLiteral,
		Phases: &EcosystemPhases{
			Upgrade: upgradePhase,
			Sync:    syncPhase,
		},
	}
	if !upgradePhase.Success && upgradePhase.Error != "" {
		resp.Error = upgradePhase.Error
	}
	if syncPhase.Executed && !syncPhase.Success && syncPhase.Error != "" {
		if resp.Error != "" {
			resp.Error += "; "
		}
		resp.Error += syncPhase.Error
	}
	return resp, nil
}

// upgradeSequenceMessage resume la cadena para el consumidor del DTO.
func upgradeSequenceMessage(upgrade UpgradePhaseReport, sync SyncPhaseReport) string {
	switch {
	case !upgrade.Success:
		return "Error durante la actualización de herramientas"
	case sync.SkippedReason == "restart-required":
		return "Upgrade completado; sync omitido — reinicia axiom antes de sincronizar"
	case sync.SkippedReason == "upgrade-failed":
		return "Upgrade falló; sync no se ejecutó"
	case sync.Executed && !sync.Success:
		return "Upgrade completado; sync con errores"
	case sync.Executed:
		return "Upgrade y sync completados"
	default:
		return "Actualización procesada exitosamente"
	}
}

// outputLines parte la salida ya renderizada en líneas no vacías, igual que
// hacen los endpoints de ecosistema existentes.
func outputLines(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

// GetBackups retorna los respaldos existentes registrados en el sistema.
func (s *Service) GetBackups() ([]BackupItem, error) {
	manifests := app.ListBackups()
	items := make([]BackupItem, 0, len(manifests))
	for _, m := range manifests {
		files := make([]string, 0, len(m.Entries))
		for _, e := range m.Entries {
			if e.SnapshotPath != "" {
				files = append(files, e.OriginalPath)
			}
		}
		items = append(items, BackupItem{
			Name:        m.ID,
			Created:     m.CreatedAt.Format(time.RFC3339),
			Description: m.Description,
			Pinned:      m.Pinned,
			Files:       files,
		})
	}
	return items, nil
}

// CreateBackup genera un snapshot de respaldo bajo demanda.
func (s *Service) CreateBackup(description string) (*BackupItem, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("obtener directorio de usuario: %w", err)
	}
	backupRoot := backup.BackupRootFor(home)
	_ = os.MkdirAll(backupRoot, 0o755)

	snapshotID := time.Now().UTC().Format("20060102150405.000000000")
	snapshotDir := filepath.Join(backupRoot, snapshotID)

	targets := []string{}
	axiomDir := filepath.Join(home, ".axiom")
	if fileExists(axiomDir) || dirExists(axiomDir) {
		targets = append(targets, axiomDir)
	}
	opencodeDir := filepath.Join(home, ".config", "opencode")
	if fileExists(opencodeDir) || dirExists(opencodeDir) {
		targets = append(targets, opencodeDir)
	}
	curWs := s.getRootPath()
	wsCfg := filepath.Join(curWs, "axiom.yaml")
	if fileExists(wsCfg) {
		targets = append(targets, wsCfg)
	}

	snap := backup.NewSnapshotter()
	manifest, err := snap.Create(snapshotDir, targets)
	if err != nil {
		return nil, fmt.Errorf("crear snapshot: %w", err)
	}
	if description != "" {
		manifest.Description = description
		_ = backup.WriteManifest(filepath.Join(snapshotDir, backup.ManifestFilename), manifest)
	}

	return &BackupItem{
		Name:        manifest.ID,
		Created:     manifest.CreatedAt.Format(time.RFC3339),
		Description: manifest.Description,
		Pinned:      manifest.Pinned,
	}, nil
}

// RestoreBackup restaura un respaldo previo por su ID.
func (s *Service) RestoreBackup(name string) (*EcosystemActionResponse, error) {
	var buf bytes.Buffer
	err := cli.RunRestore([]string{name, "--yes"}, &buf)
	rawOut := strings.TrimSpace(buf.String())
	var lines []string
	if rawOut != "" {
		lines = strings.Split(rawOut, "\n")
	}
	if err != nil {
		return &EcosystemActionResponse{
			Success: false,
			Action:  "restore",
			Message: fmt.Sprintf("Error restaurando respaldo %s", name),
			Error:   err.Error(),
			Output:  lines,
		}, nil
	}
	return &EcosystemActionResponse{
		Success: true,
		Action:  "restore",
		Message: fmt.Sprintf("Respaldo %s restaurado correctamente", name),
		Output:  lines,
	}, nil
}

// GetModelAssignments consulta las configuraciones de modelos de IA activas.
func (s *Service) GetModelAssignments() (*ModelAssignmentsDTO, error) {
	home, _ := os.UserHomeDir()
	curWs := s.getRootPath()
	settingsCandidates := []string{
		filepath.Join(curWs, "opencode.json"),
		filepath.Join(home, ".config", "opencode", "opencode.json"),
	}

	assignments := make([]ModelConfigItem, 0)
	for _, p := range settingsCandidates {
		if fileExists(p) {
			mMap, err := sdd.ReadCurrentModelAssignments(p)
			if err == nil {
				for role, a := range mMap {
					assignments = append(assignments, ModelConfigItem{
						Agent:     "opencode",
						Role:      role,
						Model:     a.FullID(),
						Reasoning: a.Effort,
					})
				}
				break
			}
		}
	}

	return &ModelAssignmentsDTO{
		ActivePersona: "axiom",
		Assignments:   assignments,
	}, nil
}
