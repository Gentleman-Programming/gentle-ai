package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultSchemaVersion define la versión actual del archivo de registro global.
const DefaultSchemaVersion = "1.0"

// Manager administra la persistencia y operaciones del catálogo global de proyectos.
type Manager struct {
	configPath string
	mu         sync.RWMutex
}

// NewManager crea un nuevo gestor. Si configPath es vacío, usa ~/.axiom/workspaces.json.
func NewManager(configPath string) (*Manager, error) {
	if configPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("no se pudo determinar el directorio home del usuario: %w", err)
		}
		configPath = filepath.Join(homeDir, ".axiom", "workspaces.json")
	}

	return &Manager{
		configPath: configPath,
	}, nil
}

// GetConfigPath retorna la ruta absoluta del archivo de configuración administrado.
func (m *Manager) GetConfigPath() string {
	return m.configPath
}

// Load lee el archivo workspaces.json. Si no existe, retorna una configuración vacía válida.
func (m *Manager) Load() (*HubConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.loadUnlocked()
}

func (m *Manager) loadUnlocked() (*HubConfig, error) {
	if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
		return &HubConfig{
			Version:         DefaultSchemaVersion,
			ActiveWorkspace: "",
			Workspaces:      make([]WorkspaceRecord, 0),
		}, nil
	}

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("error leyendo %s: %w", m.configPath, err)
	}

	var cfg HubConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("error analizando JSON de %s: %w", m.configPath, err)
	}

	if cfg.Workspaces == nil {
		cfg.Workspaces = make([]WorkspaceRecord, 0)
	}
	if cfg.Version == "" {
		cfg.Version = DefaultSchemaVersion
	}

	return &cfg, nil
}

// Save persiste la configuración de forma atómica en el sistema de archivos.
func (m *Manager) Save(cfg *HubConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.saveUnlocked(cfg)
}

func (m *Manager) saveUnlocked(cfg *HubConfig) error {
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creando directorio %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("error serializando configuración del hub: %w", err)
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", m.configPath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("error escribiendo archivo temporal: %w", err)
	}

	if err := os.Rename(tmpFile, m.configPath); err != nil {
		// Fallback para Windows en caso de colisión de rename sobre archivo existente
		_ = os.Remove(m.configPath)
		if err2 := os.Rename(tmpFile, m.configPath); err2 != nil {
			_ = os.Remove(tmpFile)
			return fmt.Errorf("error reemplazando %s: %w", m.configPath, err2)
		}
	}

	return nil
}

// Register incorpora o actualiza un workspace en el catálogo global.
func (m *Manager) Register(path, name, topology string) (*WorkspaceRecord, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("ruta de workspace inválida: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, err := m.loadUnlocked()
	if err != nil {
		return nil, err
	}

	if name == "" {
		name = filepath.Base(absPath)
	}
	if topology == "" {
		topology = "monorepo-embedded"
	}

	now := time.Now()
	cleanPath := filepath.Clean(absPath)
	isCfg := fileExists(filepath.Join(cleanPath, "axiom.yaml"))

	// Verificar si ya existe por ruta normalizada
	for i, w := range cfg.Workspaces {
		if strings.EqualFold(filepath.Clean(w.Path), cleanPath) {
			cfg.Workspaces[i].Name = name
			cfg.Workspaces[i].Topology = topology
			cfg.Workspaces[i].LastAccessed = now
			cfg.Workspaces[i].IsConfigured = isCfg

			if cfg.ActiveWorkspace == "" {
				cfg.ActiveWorkspace = cfg.Workspaces[i].ID
			}

			if err := m.saveUnlocked(cfg); err != nil {
				return nil, err
			}
			return &cfg.Workspaces[i], nil
		}
	}

	// Generar ID único
	baseID := slugify(name)
	if baseID == "" {
		baseID = "project"
	}
	uniqueID := baseID
	counter := 1
	for {
		collision := false
		for _, w := range cfg.Workspaces {
			if w.ID == uniqueID {
				collision = true
				break
			}
		}
		if !collision {
			break
		}
		counter++
		uniqueID = fmt.Sprintf("%s-%d", baseID, counter)
	}

	rec := WorkspaceRecord{
		ID:           uniqueID,
		Name:         name,
		Path:         cleanPath,
		Topology:     topology,
		RegisteredAt: now,
		LastAccessed: now,
		IsConfigured: isCfg,
	}

	cfg.Workspaces = append(cfg.Workspaces, rec)
	if cfg.ActiveWorkspace == "" {
		cfg.ActiveWorkspace = rec.ID
	}

	if err := m.saveUnlocked(cfg); err != nil {
		return nil, err
	}

	return &rec, nil
}

// Unregister elimina un proyecto del catálogo sin alterar sus archivos físicos.
func (m *Manager) Unregister(idOrPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, err := m.loadUnlocked()
	if err != nil {
		return err
	}

	targetIndex := -1
	cleanTarget := filepath.Clean(idOrPath)

	for i, w := range cfg.Workspaces {
		if w.ID == idOrPath || strings.EqualFold(filepath.Clean(w.Path), cleanTarget) {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("no se encontró ningún workspace coincidente con '%s'", idOrPath)
	}

	removedID := cfg.Workspaces[targetIndex].ID
	cfg.Workspaces = append(cfg.Workspaces[:targetIndex], cfg.Workspaces[targetIndex+1:]...)

	if cfg.ActiveWorkspace == removedID {
		if len(cfg.Workspaces) > 0 {
			cfg.ActiveWorkspace = cfg.Workspaces[0].ID
		} else {
			cfg.ActiveWorkspace = ""
		}
	}

	return m.saveUnlocked(cfg)
}

// List retorna todos los proyectos registrados con su estado de configuración actualizado.
func (m *Manager) List() ([]WorkspaceRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg, err := m.loadUnlocked()
	if err != nil {
		return nil, err
	}

	result := make([]WorkspaceRecord, len(cfg.Workspaces))
	for i, w := range cfg.Workspaces {
		result[i] = w
		result[i].IsConfigured = fileExists(filepath.Join(w.Path, "axiom.yaml"))
	}

	return result, nil
}

// SetActive establece cuál es el workspace activo del desarrollador.
func (m *Manager) SetActive(idOrPath string) (*WorkspaceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, err := m.loadUnlocked()
	if err != nil {
		return nil, err
	}

	cleanTarget := filepath.Clean(idOrPath)
	for i, w := range cfg.Workspaces {
		if w.ID == idOrPath || strings.EqualFold(filepath.Clean(w.Path), cleanTarget) {
			cfg.ActiveWorkspace = w.ID
			cfg.Workspaces[i].LastAccessed = time.Now()
			cfg.Workspaces[i].IsConfigured = fileExists(filepath.Join(w.Path, "axiom.yaml"))

			if err := m.saveUnlocked(cfg); err != nil {
				return nil, err
			}
			return &cfg.Workspaces[i], nil
		}
	}

	return nil, fmt.Errorf("workspace '%s' no encontrado en el catálogo", idOrPath)
}

// GetActive resuelve el workspace activo actual con fallback inteligente.
func (m *Manager) GetActive() (*WorkspaceRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg, err := m.loadUnlocked()
	if err != nil {
		return nil, err
	}

	// 1. Probar active_workspace configurado
	if cfg.ActiveWorkspace != "" {
		for _, w := range cfg.Workspaces {
			if w.ID == cfg.ActiveWorkspace {
				w.IsConfigured = fileExists(filepath.Join(w.Path, "axiom.yaml"))
				return &w, nil
			}
		}
	}

	// 2. Si no hay activo configurado pero hay workspaces, retornar el primero
	if len(cfg.Workspaces) > 0 {
		w := cfg.Workspaces[0]
		w.IsConfigured = fileExists(filepath.Join(w.Path, "axiom.yaml"))
		return &w, nil
	}

	return nil, errors.New("no hay ningún workspace registrado en el hub")
}

// FindWorkspace busca un workspace por ID, nombre o ruta.
func (m *Manager) FindWorkspace(idOrPath string) (*WorkspaceRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg, err := m.loadUnlocked()
	if err != nil {
		return nil, err
	}

	cleanTarget := filepath.Clean(idOrPath)
	for _, w := range cfg.Workspaces {
		if w.ID == idOrPath || strings.EqualFold(w.Name, idOrPath) || strings.EqualFold(filepath.Clean(w.Path), cleanTarget) {
			w.IsConfigured = fileExists(filepath.Join(w.Path, "axiom.yaml"))
			return &w, nil
		}
	}

	return nil, fmt.Errorf("workspace '%s' no encontrado", idOrPath)
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else if r == ' ' || r == '/' || r == '\\' || r == '.' {
			sb.WriteRune('-')
		}
	}
	res := sb.String()
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	return strings.Trim(res, "-")
}
