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
	s.mux.HandleFunc("/api/workspace", s.handleWorkspace)
	s.mux.HandleFunc("/api/increments", s.handleIncrements)
	s.mux.HandleFunc("/api/increments/", s.handleIncrementDetail)
	s.mux.HandleFunc("/api/roles", s.handleRoles)
	s.mux.HandleFunc("/api/handoffs", s.handleHandoffs)
	s.mux.HandleFunc("/api/skills", s.handleSkills)

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
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
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
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	list, err := s.service.GetIncrements()
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, list)
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
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
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

func (s *Server) respondJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}
