package kimi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
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

// installedRecord holds the fields gentle-ai manages in a Kimi Code CLI plugin
// registry entry. The registry is otherwise treated as opaque JSON so fields
// added by Kimi Code (or other tools) are preserved on rewrite.
// See: https://www.kimi.com/code/docs/en/kimi-code-cli/configuration/data-locations.html
type installedRecord map[string]json.RawMessage

func (r installedRecord) stringField(key string) string {
	var value string
	if raw, ok := r[key]; ok {
		_ = json.Unmarshal(raw, &value)
	}
	return value
}

func (r installedRecord) setField(key string, value any) {
	// Marshal of string/bool literals cannot fail.
	raw, _ := json.Marshal(value)
	r[key] = raw
}

// registerPlugin records the gentle-ai plugin in Kimi Code CLI's installed.json
// so it is loaded on startup. It preserves existing entries — including fields
// this version of gentle-ai does not know about — and updates the gentle-ai
// record in place. A corrupt registry is a hard error: silently rewriting it
// would wipe every other plugin's registration.
func (a *Adapter) registerPlugin(homeDir string) error {
	pluginDir, err := filepath.Abs(a.PluginDir(homeDir))
	if err != nil {
		pluginDir = a.PluginDir(homeDir)
	}

	installedPath := filepath.Join(a.resolveConfigDir(homeDir), "plugins", "installed.json")
	root := map[string]json.RawMessage{}
	var plugins []installedRecord
	if data, err := os.ReadFile(installedPath); err == nil {
		if err := json.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parse installed.json (refusing to overwrite existing registry): %w", err)
		}
		if raw, ok := root["plugins"]; ok {
			if err := json.Unmarshal(raw, &plugins); err != nil {
				return fmt.Errorf("parse installed.json plugins (refusing to overwrite existing registry): %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read installed.json: %w", err)
	}
	if _, ok := root["version"]; !ok {
		root["version"] = json.RawMessage("1")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	found := false
	for _, record := range plugins {
		if record.stringField("id") == "gentle-ai" {
			record.setField("root", pluginDir)
			record.setField("enabled", true)
			record.setField("updatedAt", now)
			if record.stringField("installedAt") == "" {
				record.setField("installedAt", now)
			}
			found = true
			break
		}
	}
	if !found {
		record := installedRecord{}
		record.setField("id", "gentle-ai")
		record.setField("root", pluginDir)
		record.setField("source", "local-path")
		record.setField("enabled", true)
		record.setField("installedAt", now)
		plugins = append(plugins, record)
	}

	pluginsRaw, err := json.Marshal(plugins)
	if err != nil {
		return fmt.Errorf("marshal installed.json plugins: %w", err)
	}
	root["plugins"] = pluginsRaw

	out, err := json.MarshalIndent(root, "", "  ")
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
