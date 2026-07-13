package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/pkg/doctor"
	"gopkg.in/yaml.v3"
)

// ConfigChecker performs configuration-related health checks.
type ConfigChecker struct {
	ExtraPaths []string
}

// NewConfigChecker creates a new config checker.
func NewConfigChecker(extraPaths []string) *ConfigChecker {
	return &ConfigChecker{ExtraPaths: extraPaths}
}

// Name returns the checker identifier.
func (c *ConfigChecker) Name() string { return "config" }

// Category returns the config category.
func (c *ConfigChecker) Category() doctor.Category { return doctor.CategoryConfig }

// Run executes all configuration checks and returns results.
func (c *ConfigChecker) Run(ctx context.Context) []doctor.CheckResult {
	var results []doctor.CheckResult

	homeDir, _ := os.UserHomeDir()

	// Global gentle-ai config
	results = append(results, c.checkFile(
		"cfg/global",
		filepath.Join(homeDir, ".config", "gentle-ai", "config.yaml"),
		c.validateGlobalConfig,
	))

	// State file
	results = append(results, c.checkFile(
		"cfg/state",
		filepath.Join(homeDir, ".config", "gentle-ai", "state.json"),
		c.validateStateFile,
	))

	// Project configs (walk up from cwd)
	results = append(results, c.findProjectConfigs()...)

	// Agent configs
	results = append(results, c.checkAgentConfigs(homeDir)...)

	// SSH
	results = append(results, c.checkSSH(homeDir)...)

	// Git config
	results = append(results, c.checkGitConfig(homeDir))

	// Extra paths from flags
	for _, p := range c.ExtraPaths {
		results = append(results, c.checkFile("cfg/extra", p, c.validateYAML))
	}

	return results
}

// checkFile validates a configuration file using the provided validator function.
func (c *ConfigChecker) checkFile(name, path string, validator func([]byte) (doctor.Status, string)) doctor.CheckResult {
	start := time.Now()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return doctor.CheckResult{
				Name:        "cfg/" + name,
				Category:    doctor.CategoryConfig,
				Status:      doctor.StatusWarn,
				Summary:     fmt.Sprintf("Config not found: %s", path),
				Remediation: &doctor.Remediation{
					Description: "Create config file or run initialization",
					Commands: map[string]string{
						"linux":   "mkdir -p " + filepath.Dir(path) + " && touch " + path,
						"darwin":  "mkdir -p " + filepath.Dir(path) + " && touch " + path,
						"windows": "mkdir " + filepath.Dir(path) + " && type nul > " + path,
					},
				},
				Duration: time.Since(start),
			}
		}
		return doctor.CheckResult{
			Name:        "cfg/" + name,
			Category:    doctor.CategoryConfig,
			Status:      doctor.StatusFail,
			Summary:     fmt.Sprintf("Cannot read config: %s", err),
			Duration:    time.Since(start),
		}
	}

	status, detail := validator(data)
	return doctor.CheckResult{
		Name:        "cfg/" + name,
		Category:    doctor.CategoryConfig,
		Status:      status,
		Summary:     fmt.Sprintf("Config %s: %s", path, status),
		Detail:      detail,
		Duration:    time.Since(start),
	}
}

// validateGlobalConfig checks the global config file for required fields.
func (c *ConfigChecker) validateGlobalConfig(data []byte) (doctor.Status, string) {
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return doctor.StatusFail, fmt.Sprintf("Invalid YAML: %v", err)
	}
	required := []string{"version", "profile"}
	for _, f := range required {
		if _, ok := cfg[f]; !ok {
			return doctor.StatusWarn, fmt.Sprintf("Missing field: %s", f)
		}
	}
	return doctor.StatusPass, "Valid global config"
}

// validateStateFile checks the state file for required fields.
func (c *ConfigChecker) validateStateFile(data []byte) (doctor.Status, string) {
	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		return doctor.StatusFail, fmt.Sprintf("Invalid JSON: %v", err)
	}
	if _, ok := state["installed_agents"]; !ok {
		return doctor.StatusWarn, "Missing installed_agents array"
	}
	return doctor.StatusPass, "Valid state file"
}

// validateYAML checks whether the data is valid YAML.
func (c *ConfigChecker) validateYAML(data []byte) (doctor.Status, string) {
	var v interface{}
	if err := yaml.Unmarshal(data, &v); err != nil {
		return doctor.StatusFail, fmt.Sprintf("Invalid YAML: %v", err)
	}
	return doctor.StatusPass, "Valid YAML"
}

// findProjectConfigs discovers configuration files in the current project.
func (c *ConfigChecker) findProjectConfigs() []doctor.CheckResult {
	var results []doctor.CheckResult
	cwd, _ := os.Getwd()
	for dir := cwd; dir != "/"; dir = filepath.Dir(dir) {
		for _, name := range []string{"gentle-ai.yaml", ".gentle-ai.yaml"} {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				results = append(results, c.checkFile(
					"cfg/project/"+name,
					path,
					c.validateYAML,
				))
			}
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	return results
}

// checkAgentConfigs validates configuration for supported AI agents.
func (c *ConfigChecker) checkAgentConfigs(homeDir string) []doctor.CheckResult {
	agents := map[string]string{
		"claude":    filepath.Join(homeDir, ".claude"),
		"opencode":  filepath.Join(homeDir, ".config", "opencode"),
		"cursor":    filepath.Join(homeDir, ".cursor"),
		"windsurf":  filepath.Join(homeDir, ".windsurf"),
		"codex":     filepath.Join(homeDir, ".codex"),
		"kimi":      filepath.Join(homeDir, ".kimi"),
		"qwen":      filepath.Join(homeDir, ".qwen"),
		"kiro":      filepath.Join(homeDir, ".kiro"),
		"trae":      filepath.Join(homeDir, ".trae"),
	}
	var results []doctor.CheckResult
	for name, path := range agents {
		info, err := os.Stat(path)
		status := doctor.StatusInfo
		detail := "Not configured"
		if err == nil && info.IsDir() {
			status = doctor.StatusPass
			detail = "Directory exists"
		}
		results = append(results, doctor.CheckResult{
			Name:        "cfg/agent/" + name,
			Category:    doctor.CategoryConfig,
			Status:      status,
			Summary:     fmt.Sprintf("Agent config (%s): %s", name, detail),
			Detail:      path,
			Duration:    0,
		})
	}
	return results
}

// checkSSH validates SSH key configuration and permissions.
func (c *ConfigChecker) checkSSH(homeDir string) []doctor.CheckResult {
	start := time.Now()
	sshDir := filepath.Join(homeDir, ".ssh")
	keys, _ := filepath.Glob(filepath.Join(sshDir, "id_*"))
	pubKeys, _ := filepath.Glob(filepath.Join(sshDir, "*.pub"))

	status := doctor.StatusWarn
	detail := "No SSH keys found"
	if len(keys) > 0 || len(pubKeys) > 0 {
		status = doctor.StatusPass
		detail = fmt.Sprintf("Found %d private, %d public keys", len(keys), len(pubKeys))
	}

	return []doctor.CheckResult{{
		Name:        "cfg/ssh/keys",
		Category:    doctor.CategoryConfig,
		Status:      status,
		Summary:     "SSH keys: " + detail,
		Detail:      sshDir,
		Duration:    time.Since(start),
	}}
}

// checkGitConfig validates the global git configuration.
func (c *ConfigChecker) checkGitConfig(homeDir string) doctor.CheckResult {
	start := time.Now()
	gitConfig := filepath.Join(homeDir, ".gitconfig")
	data, err := os.ReadFile(gitConfig)
	if err != nil {
		return doctor.CheckResult{
			Name:        "cfg/git/config",
			Category:    doctor.CategoryConfig,
			Status:      doctor.StatusWarn,
			Summary:     "No global .gitconfig found",
			Duration:    time.Since(start),
		}
	}

	hasName := strings.Contains(string(data), "name =")
	hasEmail := strings.Contains(string(data), "email =")

	status := doctor.StatusPass
	detail := "user.name and user.email configured"
	if !hasName || !hasEmail {
		status = doctor.StatusWarn
		detail = "Missing user.name or user.email"
	}

	return doctor.CheckResult{
		Name:        "cfg/git/config",
		Category:    doctor.CategoryConfig,
		Status:      status,
		Summary:     "Git config: " + detail,
		Detail:      gitConfig,
		Duration:    time.Since(start),
	}
}