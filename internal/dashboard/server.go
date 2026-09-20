package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/semantic"
)

// Server representa el servidor HTTP local para el dashboard web de Axiom.
type Server struct {
	service    *Service
	mux        *http.ServeMux
	httpServer *http.Server
	listener   net.Listener
	port       int
}

// NewServer inicializa el servidor HTTP y configura las rutas de la API y assets estáticos.
func NewServer(svc *Service) *Server {
	s := &Server{
		service: svc,
		mux:     http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Router retorna el ServeMux para pruebas unitarias con httptest.
func (s *Server) Router() http.Handler {
	return s.mux
}

func (s *Server) registerRoutes() {
	// 1. Endpoints de la API REST
	s.mux.HandleFunc("/api/projects", s.handleProjects)
	s.mux.HandleFunc("/api/projects/switch", s.handleProjectsSwitch)
	s.mux.HandleFunc("/api/projects/add", s.handleProjectsAdd)
	s.mux.HandleFunc("/api/projects/init", s.handleProjectsInit)
	s.mux.HandleFunc("/api/workspace", s.handleWorkspace)
	s.mux.HandleFunc("/api/increments", s.handleIncrements)
	s.mux.HandleFunc("/api/increments/continue", s.handleIncrementContinue)
	s.mux.HandleFunc("/api/increments/verify", s.handleIncrementVerify)
	s.mux.HandleFunc("/api/increments/migrate-cumulative", s.handleMigrateCumulative)
	s.mux.HandleFunc("/api/increments/", s.handleIncrementDetail)
	s.mux.HandleFunc("/api/roles", s.handleRoles)
	s.mux.HandleFunc("/api/handoffs", s.handleHandoffs)
	s.mux.HandleFunc("/api/skills", s.handleSkills)
	s.mux.HandleFunc("/api/skills/inbox", s.handleSkillsInbox)
	s.mux.HandleFunc("/api/skills/scan", s.handleSkillsScan)
	s.mux.HandleFunc("/api/skills/approve", s.handleSkillsApprove)
	s.mux.HandleFunc("/api/skills/reject", s.handleSkillsReject)
	s.mux.HandleFunc("/api/semantic/status", s.handleSemanticStatus)
	s.mux.HandleFunc("/api/semantic/symbols", s.handleSemanticSymbols)
	s.mux.HandleFunc("/api/semantic/dependencies", s.handleSemanticDependencies)
	s.mux.HandleFunc("/api/archive/specs", s.handleArchiveSpecs)
	s.mux.HandleFunc("/api/archive/specs/", s.handleArchiveSpecDetail)
	s.mux.HandleFunc("/api/archive/sync", s.handleArchiveSync)
	s.mux.HandleFunc("/api/ecosystem/doctor", s.handleEcosystemDoctor)
	s.mux.HandleFunc("/api/ecosystem/sync", s.handleEcosystemSync)
	s.mux.HandleFunc("/api/ecosystem/upgrade", s.handleEcosystemUpgrade)
	s.mux.HandleFunc("/api/ecosystem/backups", s.handleEcosystemBackups)
	s.mux.HandleFunc("/api/ecosystem/backups/create", s.handleEcosystemBackupCreate)
	s.mux.HandleFunc("/api/ecosystem/backups/restore", s.handleEcosystemBackupRestore)
	s.mux.HandleFunc("/api/ecosystem/models", s.handleEcosystemModels)

	// 2. Servidor de Archivos Estáticos Embebidos
	subFS, err := fs.Sub(AssetsFS, "assets")
	if err == nil {
		fileServer := http.FileServer(http.FS(subFS))
		s.mux.Handle("/", fileServer)
	} else {
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Assets embebidos no disponibles", http.StatusInternalServerError)
		})
	}
}

// ListenAndServe inicia el servidor buscando un puerto libre a partir del puerto sugerido.
func (s *Server) ListenAndServe(initialPort int) (int, error) {
	listener, port, err := s.findAvailablePort(initialPort)
	if err != nil {
		return 0, err
	}
	s.listener = listener
	s.port = port

	s.httpServer = &http.Server{
		Handler:      s.withHeaders(s.mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		_ = s.httpServer.Serve(listener)
	}()

	return port, nil
}

// Shutdown detiene el servidor de forma ordenada.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// Port retorna el puerto donde está escuchando el servidor.
func (s *Server) Port() int {
	return s.port
}

func (s *Server) findAvailablePort(startPort int) (net.Listener, int, error) {
	for p := startPort; p <= startPort+10; p++ {
		addr := fmt.Sprintf("127.0.0.1:%d", p)
		l, err := net.Listen("tcp", addr)
		if err == nil {
			return l, p, nil
		}
	}
	return nil, 0, fmt.Errorf("no se encontró ningún puerto disponible entre %d y %d", startPort, startPort+10)
}

func (s *Server) withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	projects, err := s.service.GetProjects()
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, projects)
}

func (s *Server) handleProjectsSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	var req ProjectSwitchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
		return
	}

	target := req.Path
	if target == "" {
		target = req.ID
	}
	if target == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Debes especificar 'id' o 'path'"})
		return
	}

	// Si target no es un directorio físico directo, buscarlo en el Hub por ID o nombre
	if !dirExists(target) && s.service.GetHubManager() != nil {
		rec, err := s.service.GetHubManager().FindWorkspace(target)
		if err == nil && rec != nil {
			target = rec.Path
		}
	}

	ws, err := s.service.SwitchWorkspace(target)
	if err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.respondJSON(w, http.StatusOK, ws)
}

func (s *Server) handleProjectsAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	var req ProjectAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
		return
	}

	rec, err := s.service.AddProject(req)
	if err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.respondJSON(w, http.StatusOK, rec)
}

func (s *Server) handleProjectsInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	var req ProjectInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
		return
	}

	res, err := s.service.InitProject(req)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.respondJSON(w, http.StatusOK, res)
}

func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	dto, err := s.service.GetWorkspace()
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, dto)
}

func (s *Server) handleIncrements(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.service.GetIncrements()
		if err != nil {
			s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.respondJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var req CreateIncrementRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido: " + err.Error()})
			return
		}
		res, err := s.service.CreateIncrement(req)
		if err != nil {
			s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.respondJSON(w, http.StatusCreated, res)
	default:
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleIncrementContinue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	var req IncrementActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}
	res, err := s.service.ContinueIncrement(req.Name)
	if err != nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, res)
}

func (s *Server) handleIncrementVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	var req IncrementActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}
	res, err := s.service.VerifyIncrement(req.Name)
	if err != nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, res)
}

func (s *Server) handleIncrementDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/increments/")
	name = strings.TrimSpace(name)
	if name == "" {
		http.Error(w, "Nombre de incremento requerido", http.StatusBadRequest)
		return
	}

	dto, err := s.service.GetIncrementDetail(name)
	if err != nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, dto)
}

func (s *Server) handleMigrateCumulative(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	var req MigrateCumulativeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
		return
	}
	if strings.TrimSpace(req.Change) == "" || strings.TrimSpace(req.Role) == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Campos 'change' y 'role' son requeridos"})
		return
	}

	count, err := s.service.MigrateIncompleteTasksToCumulative(req.Change, req.Role)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"migrated": count,
		"change":   req.Change,
		"role":     req.Role,
	})
}

func (s *Server) handleRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	change := r.URL.Query().Get("change")
	if change == "" {
		http.Error(w, "Parámetro 'change' es requerido", http.StatusBadRequest)
		return
	}

	barrier, err := s.service.GetRoleStatus(change)
	if err != nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, barrier)
}

func (s *Server) handleHandoffs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		change := r.URL.Query().Get("change")
		if change == "" {
			http.Error(w, "Parámetro 'change' es requerido", http.StatusBadRequest)
			return
		}

		ho, err := s.service.GetHandoff(change)
		if err != nil {
			s.respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		s.respondJSON(w, http.StatusOK, ho)
	case http.MethodPost:
		var req CreateHandoffRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido: " + err.Error()})
			return
		}
		res, err := s.service.CreateHandoff(req)
		if err != nil {
			s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.respondJSON(w, http.StatusCreated, res)
	default:
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	skills, err := s.service.GetSkills()
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, skills)
}

func (s *Server) handleSkillsInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	proposals, err := s.service.GetSkillsInbox()
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, proposals)
}

func (s *Server) handleSkillsScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	var req SkillActionDTO
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	report, err := s.service.ScanSkills(r.Context(), req.Role, req.Offline)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, report)
}

func (s *Server) handleSkillsApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	var req SkillActionDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "Campo 'name' es requerido en el cuerpo JSON", http.StatusBadRequest)
		return
	}

	if err := s.service.ApproveSkill(req.Name); err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":  "approved",
		"message": fmt.Sprintf("Skill '%s' aprobada e instalada con éxito", req.Name),
	})
}

func (s *Server) handleSkillsReject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	var req SkillActionDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "Campo 'name' es requerido en el cuerpo JSON", http.StatusBadRequest)
		return
	}

	if err := s.service.RejectSkill(req.Name); err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":  "rejected",
		"message": fmt.Sprintf("Propuesta '%s' descartada del buzón", req.Name),
	})
}

func (s *Server) handleSemanticStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	status, err := s.service.GetSemanticStatus(r.Context())
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, status)
}

func (s *Server) handleSemanticSymbols(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	q := semantic.SemanticQuery{
		Query: r.URL.Query().Get("query"),
		Kind:  semantic.SymbolKind(r.URL.Query().Get("kind")),
		Role:  r.URL.Query().Get("role"),
	}
	symbols, err := s.service.FindSemanticSymbols(q)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if symbols == nil {
		symbols = make([]semantic.SymbolItem, 0)
	}
	s.respondJSON(w, http.StatusOK, symbols)
}

func (s *Server) handleSemanticDependencies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	role := r.URL.Query().Get("role")
	deps, err := s.service.InspectSemanticDependencies(role)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if deps == nil {
		deps = make([]semantic.DependencyRelation, 0)
	}
	s.respondJSON(w, http.StatusOK, deps)
}

func (s *Server) handleArchiveSpecs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	catalog, err := s.service.GetLivingSpecs(r.Context())
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, catalog)
}

func (s *Server) handleArchiveSpecDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	domain := strings.TrimPrefix(r.URL.Path, "/api/archive/specs/")
	domain = strings.TrimSpace(domain)
	if domain == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Dominio de especificación requerido"})
		return
	}
	entry, content, err := s.service.GetLivingSpecDetail(domain)
	if err != nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"entry":   entry,
		"content": content,
	})
}

func (s *Server) handleArchiveSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	report, err := s.service.SyncLivingDocs(r.Context())
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, report)
}

func (s *Server) handleEcosystemDoctor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	report, err := s.service.GetDoctorDiagnostics()
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, report)
}

func (s *Server) handleEcosystemSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	resp, err := s.service.RunSync()
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, resp)
		return
	}
	s.respondJSON(w, http.StatusOK, resp)
}

func (s *Server) handleEcosystemUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	resp, err := s.service.RunUpgrade()
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, resp)
		return
	}
	s.respondJSON(w, http.StatusOK, resp)
}

func (s *Server) handleEcosystemBackups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	backups, err := s.service.GetBackups()
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, backups)
}

func (s *Server) handleEcosystemBackupCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	var req BackupActionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	item, err := s.service.CreateBackup(req.Description)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusCreated, item)
}

func (s *Server) handleEcosystemBackupRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	var req BackupActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Identificador de respaldo (name) requerido"})
		return
	}
	resp, err := s.service.RestoreBackup(req.Name)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, resp)
		return
	}
	s.respondJSON(w, http.StatusOK, resp)
}

func (s *Server) handleEcosystemModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	models, err := s.service.GetModelAssignments()
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, models)
}

func (s *Server) respondJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}
