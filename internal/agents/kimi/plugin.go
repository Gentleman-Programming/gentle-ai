package kimi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
)

// KimiPluginManifest represents the kimi.plugin.json manifest generated
// for Kimi Code plugin-based skill distribution.
//
// Schema reference: https://www.kimi.com/code/docs/en/kimi-code-cli/customization/plugins.html
type KimiPluginManifest struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Description  string            `json:"description"`
	Skills       string            `json:"skills"`
	SessionStart *KimiSessionStart `json:"sessionStart,omitempty"`
}

// KimiSessionStart declares a skill loaded into the main agent at session start.
type KimiSessionStart struct {
	Skill string `json:"skill"`
}

// PluginDir returns the root directory for the Kimi plugin.
//
//	{resolvedConfigDir}/plugins/managed/gentle-ai/
//
// The base directory is resolved through resolveConfigDir so that
// KIMI_CODE_HOME does not split config, plugins, and skills across trees.
func (a *Adapter) PluginDir(homeDir string) string {
	return filepath.Join(a.resolveConfigDir(homeDir), "plugins", "managed", "gentle-ai")
}

// PluginManifestPath returns the full path to kimi.plugin.json.
func (a *Adapter) PluginManifestPath(homeDir string) string {
	return filepath.Join(a.PluginDir(homeDir), "kimi.plugin.json")
}

// InstallPlugin creates the plugin directory and writes the kimi.plugin.json
// manifest with the given version. If the directory already exists it is
// reused; the manifest is always overwritten atomically.
func (a *Adapter) InstallPlugin(homeDir string, version string) error {
	pluginDir := a.PluginDir(homeDir)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}

	skillsDir := filepath.Join(pluginDir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return fmt.Errorf("create plugin skills dir: %w", err)
	}

	manifest := buildManifest(version)

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plugin manifest: %w", err)
	}
	data = append(data, '\n')

	manifestPath := a.PluginManifestPath(homeDir)
	if _, err := filemerge.WriteFileAtomic(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("write plugin manifest: %w", err)
	}

	if err := a.registerPlugin(homeDir); err != nil {
		return fmt.Errorf("register plugin: %w", err)
	}

	return nil
}

// buildManifest constructs the plugin manifest with the given version.
//
// Note: we do NOT declare mcpServers in the plugin manifest. Kimi Code
// namespaces plugin-owned MCP servers as "plugin-<id>:<name>", which would
// expose Engram as "plugin-gentle-ai:engram" instead of the plain "engram"
// name used by every other agent. The Engram component already writes the
// canonical server entry to the resolved Kimi config directory (mcp.json),
// so the plugin only needs to expose skills and the session-start skill.
func buildManifest(version string) KimiPluginManifest {
	return KimiPluginManifest{
		Name:        "gentle-ai",
		Version:     version,
		Description: "Gentle AI — SDD workflow and agent integration for Kimi Code",
		Skills:      "./skills/",
		SessionStart: &KimiSessionStart{
			Skill: "sdd-init",
		},
	}
}

// installedFile matches the Kimi Code CLI plugin registry schema.
// See: https://www.kimi.com/code/docs/en/kimi-code-cli/configuration/data-locations.html
type installedFile struct {
	Version int               `json:"version"`
	Plugins []installedRecord `json:"plugins"`
}

type installedRecord struct {
	ID          string `json:"id"`
	Root        string `json:"root"`
	Source      string `json:"source"`
	Enabled     bool   `json:"enabled"`
	InstalledAt string `json:"installedAt"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

// registerPlugin records the gentle-ai plugin in Kimi Code CLI's installed.json
// so it is loaded on startup. It preserves existing entries and updates the
// gentle-ai record in place.
func (a *Adapter) registerPlugin(homeDir string) error {
	pluginDir, err := filepath.Abs(a.PluginDir(homeDir))
	if err != nil {
		pluginDir = a.PluginDir(homeDir)
	}

	installedPath := filepath.Join(a.resolveConfigDir(homeDir), "plugins", "installed.json")
	var file installedFile
	if data, err := os.ReadFile(installedPath); err == nil {
		_ = json.Unmarshal(data, &file)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read installed.json: %w", err)
	}
	if file.Version == 0 {
		file.Version = 1
	}

	now := time.Now().UTC().Format(time.RFC3339)
	found := false
	for i := range file.Plugins {
		if file.Plugins[i].ID == "gentle-ai" {
			file.Plugins[i].Root = pluginDir
			file.Plugins[i].Enabled = true
			file.Plugins[i].UpdatedAt = now
			if file.Plugins[i].InstalledAt == "" {
				file.Plugins[i].InstalledAt = now
			}
			found = true
			break
		}
	}
	if !found {
		file.Plugins = append(file.Plugins, installedRecord{
			ID:          "gentle-ai",
			Root:        pluginDir,
			Source:      "local-path",
			Enabled:     true,
			InstalledAt: now,
		})
	}

	out, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal installed.json: %w", err)
	}
	out = append(out, '\n')

	if err := os.MkdirAll(filepath.Dir(installedPath), 0o755); err != nil {
		return fmt.Errorf("create plugins dir: %w", err)
	}
	if _, err := filemerge.WriteFileAtomic(installedPath, out, 0o644); err != nil {
		return fmt.Errorf("write installed.json: %w", err)
	}
	return nil
}
