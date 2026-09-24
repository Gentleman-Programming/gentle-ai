package semantic

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Detector inspecciona el entorno local para determinar la disponibilidad de Serena MCP y CodeGraph.
type Detector struct {
	homeDir      string
	lookPathFunc func(file string) (string, error)
}

// NewDetector crea una nueva instancia del detector semántico.
func NewDetector() *Detector {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return &Detector{
		homeDir:      home,
		lookPathFunc: exec.LookPath,
	}
}

// NewDetectorWithCustomHome inicializa el detector con rutas y función LookPath personalizadas para tests.
func NewDetectorWithCustomHome(homeDir string, lookPath func(file string) (string, error)) *Detector {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	return &Detector{
		homeDir:      homeDir,
		lookPathFunc: lookPath,
	}
}

// DetectAgents inspecciona tanto los agentes del workspace local como la configuración de usuario global.
func (d *Detector) DetectAgents(projectRoots ...string) []AgentToolStatus {
	var results []AgentToolStatus
	seenPaths := make(map[string]bool)

	// 1. Detección a nivel de Proyecto (Workspace)
	var projectRoot string
	if len(projectRoots) > 0 && strings.TrimSpace(projectRoots[0]) != "" {
		projectRoot, _ = filepath.Abs(projectRoots[0])
	}

	if projectRoot != "" {
		workspaceConfigs := []struct {
			AgentName string
			RelPath   string
		}{
			{AgentName: "Google Antigravity (Workspace)", RelPath: filepath.Join(".gemini", "antigravity", "mcp_config.json")},
			{AgentName: "Google Antigravity (Local)", RelPath: "mcp_config.json"},
			{AgentName: "Claude Code (Settings)", RelPath: filepath.Join(".claude", "settings.json")},
			{AgentName: "Claude Code (Workspace)", RelPath: ".claude.json"},
			{AgentName: "Workspace MCP (.mcp.json)", RelPath: ".mcp.json"},
			{AgentName: "OpenCode (Workspace)", RelPath: "opencode.json"},
			{AgentName: "OpenCode (.opencode)", RelPath: filepath.Join(".opencode", "mcp.json")},
			{AgentName: "VSCode / Cursor (Workspace)", RelPath: filepath.Join(".vscode", "mcp.json")},
			{AgentName: "Cursor IDE (Workspace)", RelPath: filepath.Join(".cursor", "mcp.json")},
		}

		for _, cfg := range workspaceConfigs {
			fullPath := filepath.Join(projectRoot, cfg.RelPath)
			if seenPaths[fullPath] {
				continue
			}
			seenPaths[fullPath] = true

			status := AgentToolStatus{
				AgentName:  cfg.AgentName,
				ConfigPath: fullPath,
				Configured: false,
				Scope:      "workspace",
			}

			content, err := os.ReadFile(fullPath)
			if err != nil {
				// No añadimos archivos de workspace inexistentes para no ensuciar la vista
				continue
			}

			configuredTools := d.findSemanticToolsInJSON(content)
			if len(configuredTools) > 0 {
				status.Configured = true
				status.Details = "Herramientas detectadas: " + strings.Join(configuredTools, ", ")
			} else {
				status.Details = "Presente en workspace pero sin herramientas semánticas registradas"
			}
			results = append(results, status)
		}
	}

	// 2. Detección a nivel de Usuario (Global)
	if d.homeDir != "" {
		agentConfigs := []struct {
			AgentName string
			RelPath   string
		}{
			{AgentName: "Google Antigravity", RelPath: filepath.Join(".gemini", "antigravity", "mcp_config.json")},
			{AgentName: "Google Antigravity Global", RelPath: filepath.Join(".gemini", "config", "mcp_config.json")},
			{AgentName: "Claude Code", RelPath: ".claude.json"},
			{AgentName: "OpenCode Global", RelPath: filepath.Join(".config", "opencode", "opencode.json")},
			{AgentName: "Kiro IDE", RelPath: filepath.Join(".kiro", "settings", "mcp.json")},
			{AgentName: "Cursor IDE", RelPath: filepath.Join(".cursor", "mcp.json")},
		}

		for _, cfg := range agentConfigs {
			fullPath := filepath.Join(d.homeDir, cfg.RelPath)
			if seenPaths[fullPath] {
				continue
			}
			seenPaths[fullPath] = true

			status := AgentToolStatus{
				AgentName:  cfg.AgentName,
				ConfigPath: fullPath,
				Configured: false,
				Scope:      "global",
			}

			content, err := os.ReadFile(fullPath)
			if err != nil {
				status.Details = "Archivo de configuración no encontrado"
				results = append(results, status)
				continue
			}

			configuredTools := d.findSemanticToolsInJSON(content)
			if len(configuredTools) > 0 {
				status.Configured = true
				status.Details = "Herramientas detectadas: " + strings.Join(configuredTools, ", ")
			} else {
				status.Details = "Presente pero sin herramientas semánticas registradas"
			}
			results = append(results, status)
		}
	}

	return results
}

// findSemanticToolsInJSON busca referencias a serena o codegraph en un archivo de configuración JSON.
func (d *Detector) findSemanticToolsInJSON(content []byte) []string {
	var parsed map[string]interface{}
	if err := json.Unmarshal(content, &parsed); err != nil {
		return nil
	}

	var found []string
	searchKeys := []string{"mcpServers", "mcp_servers", "tools", "servers"}

	checkObj := func(m map[string]interface{}) {
		for k := range m {
			lower := strings.ToLower(k)
			if strings.Contains(lower, "serena") {
				found = append(found, "serena-mcp")
			}
			if strings.Contains(lower, "codegraph") {
				found = append(found, "codegraph")
			}
		}
	}

	for _, key := range searchKeys {
		if raw, ok := parsed[key]; ok {
			if m, ok := raw.(map[string]interface{}); ok {
				checkObj(m)
			}
		}
	}

	// Comprobación directa en la raíz
	checkObj(parsed)

	return uniqueStrings(found)
}

// CheckCodeGraphInPath comprueba si el binario de CodeGraph está en el PATH del sistema.
func (d *Detector) CheckCodeGraphInPath() bool {
	_, err := d.lookPathFunc("codegraph")
	return err == nil
}

// CheckSerenaInPath comprueba si el binario o ejecutable de Serena MCP está en el PATH del sistema.
func (d *Detector) CheckSerenaInPath() bool {
	if _, err := d.lookPathFunc("serena"); err == nil {
		return true
	}
	if _, err := d.lookPathFunc("serena-mcp"); err == nil {
		return true
	}
	return false
}

// CheckCodeGraphConfigured comprueba si CodeGraph está configurado en algún agente (workspace o global).
func (d *Detector) CheckCodeGraphConfigured(agents []AgentToolStatus) bool {
	for _, a := range agents {
		if a.Configured && strings.Contains(strings.ToLower(a.Details), "codegraph") {
			return true
		}
	}
	return false
}

// CheckSerenaConfigured comprueba si Serena está configurada en algún agente (workspace o global).
func (d *Detector) CheckSerenaConfigured(agents []AgentToolStatus) bool {
	for _, a := range agents {
		if a.Configured && strings.Contains(strings.ToLower(a.Details), "serena") {
			return true
		}
	}
	return false
}

// CheckSerenaAvailability comprueba si Serena MCP está disponible en algún agente o CLI.
func (d *Detector) CheckSerenaAvailability(agents []AgentToolStatus) bool {
	return d.CheckSerenaInPath() || d.CheckSerenaConfigured(agents)
}

// CheckCodeGraphAvailability comprueba si CodeGraph está disponible en CLI o agentes.
func (d *Detector) CheckCodeGraphAvailability(agents []AgentToolStatus) bool {
	return d.CheckCodeGraphInPath() || d.CheckCodeGraphConfigured(agents)
}

func uniqueStrings(slice []string) []string {
	keys := make(map[string]bool)
	var list []string
	for _, entry := range slice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}
