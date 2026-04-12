package sdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallSddPlugin_IdempotentAndUpdatesConfig(t *testing.T) {
	home := t.TempDir()
	opencodeDir := filepath.Join(home, ".config", "opencode")
	pluginDir := filepath.Join(opencodeDir, "plugins", pluginSddDirName)

	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(pluginDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "stale.txt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale.txt): %v", err)
	}

	initialTUI := map[string]any{
		"$schema": tuiJSONSchema,
		"keybinds": map[string]any{
			"tool_details": "ctrl+alt+t",
		},
		"plugin": []any{
			[]any{
				"/home/test/opencode-mask/",
				map[string]any{
					"enabled":      true,
					"theme":        "tokyo-night-dev",
					"set_theme":    true,
					"show_sidebar": true,
				},
			},
		},
	}
	initialTUIBytes, _ := json.MarshalIndent(initialTUI, "", "  ")
	initialTUIBytes = append(initialTUIBytes, '\n')
	if err := os.WriteFile(filepath.Join(opencodeDir, "tui.json"), initialTUIBytes, 0o644); err != nil {
		t.Fatalf("WriteFile(tui.json): %v", err)
	}

	initialPackage := map[string]any{
		"name": "opencode-config",
		"dependencies": map[string]string{
			"@opencode-ai/plugin": "9.9.9",
		},
		"scripts": map[string]string{
			"dev": "bun run dev",
		},
	}
	initialPackageBytes, _ := json.MarshalIndent(initialPackage, "", "  ")
	initialPackageBytes = append(initialPackageBytes, '\n')
	if err := os.WriteFile(filepath.Join(opencodeDir, "package.json"), initialPackageBytes, 0o644); err != nil {
		t.Fatalf("WriteFile(package.json): %v", err)
	}

	result, err := InstallSddPlugin(home)
	if err != nil {
		t.Fatalf("InstallSddPlugin(): %v", err)
	}
	if result.FilesChanged == 0 {
		t.Fatal("InstallSddPlugin() FilesChanged = 0, want > 0")
	}

	if _, err := os.Stat(filepath.Join(pluginDir, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale plugin file still exists after reinstall")
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "index.tsx")); err != nil {
		t.Fatalf("plugin entry file missing after install: %v", err)
	}

	tuiBytes, err := os.ReadFile(filepath.Join(opencodeDir, "tui.json"))
	if err != nil {
		t.Fatalf("ReadFile(tui.json): %v", err)
	}
	var tuiCfg map[string]any
	if err := json.Unmarshal(tuiBytes, &tuiCfg); err != nil {
		t.Fatalf("unmarshal tui.json: %v", err)
	}
	if got := tuiCfg["$schema"]; got != tuiJSONSchema {
		t.Fatalf("tui.json schema = %v, want %q", got, tuiJSONSchema)
	}
	if _, ok := tuiCfg["keybinds"].(map[string]any); !ok {
		t.Fatal("tui.json keybinds field was lost during rewrite")
	}
	pluginList, ok := tuiCfg["plugin"].([]any)
	if !ok {
		t.Fatal("tui.json plugin field is not an array")
	}
	count := 0
	hasTuplePlugin := false
	for _, p := range pluginList {
		switch v := p.(type) {
		case string:
			if v == pluginSddRegistryKey {
				count++
			}
		case []any:
			if len(v) > 0 {
				if path, ok := v[0].(string); ok && path == "/home/test/opencode-mask/" {
					hasTuplePlugin = true
				}
			}
		}
	}
	if !hasTuplePlugin {
		t.Fatal("existing tuple-style plugin entry was lost from tui.json")
	}
	if count != 1 {
		t.Fatalf("plugin registry count = %d, want 1; plugin list = %v", count, pluginList)
	}

	result2, err := InstallSddPlugin(home)
	if err != nil {
		t.Fatalf("second InstallSddPlugin(): %v", err)
	}
	if result2.FilesChanged != 0 {
		t.Fatalf("second install FilesChanged = %d, want 0", result2.FilesChanged)
	}
	if !result2.AlreadyInstalled {
		t.Fatal("second install AlreadyInstalled = false, want true")
	}
	if result2.PackageWarning != "" {
		t.Fatalf("second install PackageWarning = %q, want empty", result2.PackageWarning)
	}

	tuiBytes2, err := os.ReadFile(filepath.Join(opencodeDir, "tui.json"))
	if err != nil {
		t.Fatalf("ReadFile(tui.json) after second install: %v", err)
	}
	if err := json.Unmarshal(tuiBytes2, &tuiCfg); err != nil {
		t.Fatalf("unmarshal tui.json after second install: %v", err)
	}
	pluginList, ok = tuiCfg["plugin"].([]any)
	if !ok {
		t.Fatal("tui.json plugin field is not an array after second install")
	}
	count = 0
	for _, p := range pluginList {
		if s, ok := p.(string); ok && s == pluginSddRegistryKey {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("after second install plugin registry count = %d, want 1; plugin list = %v", count, pluginList)
	}

	pkgBytes, err := os.ReadFile(filepath.Join(opencodeDir, "package.json"))
	if err != nil {
		t.Fatalf("ReadFile(package.json): %v", err)
	}
	var pkg map[string]any
	if err := json.Unmarshal(pkgBytes, &pkg); err != nil {
		t.Fatalf("unmarshal package.json: %v", err)
	}
	deps, ok := pkg["dependencies"].(map[string]any)
	if !ok {
		t.Fatal("package.json missing dependencies object")
	}
	if got := deps["@opencode-ai/plugin"]; got != "9.9.9" {
		t.Fatalf("@opencode-ai/plugin version = %v, want preserved existing version 9.9.9", got)
	}
	if _, exists := deps["unique-names-generator"]; exists {
		t.Fatal("dependencies should not include unique-names-generator")
	}
	if _, ok := pkg["scripts"]; !ok {
		t.Fatal("package.json scripts field was lost during rewrite")
	}
	if result.PackageWarning != "" {
		t.Fatalf("PackageWarning = %q, want empty in normal successful flow", result.PackageWarning)
	}

}

func TestEnsureOpencodePackageJSON_CreatesFileWhenMissing(t *testing.T) {
	home := t.TempDir()
	opencodeDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(opencodeDir): %v", err)
	}

	warning := ensureOpencodePackageJSON(opencodeDir)
	if warning != "" {
		t.Fatalf("ensureOpencodePackageJSON() warning = %q, want empty in successful flow", warning)
	}

	pkgBytes, err := os.ReadFile(filepath.Join(opencodeDir, "package.json"))
	if err != nil {
		t.Fatalf("ReadFile(package.json): %v", err)
	}
	var pkg opencodePackageJSON
	if err := json.Unmarshal(pkgBytes, &pkg); err != nil {
		t.Fatalf("unmarshal package.json: %v", err)
	}
	if got := pkg.Dependencies["@opencode-ai/plugin"]; got != "1.4.2" {
		t.Fatalf("@opencode-ai/plugin = %q, want %q", got, "1.4.2")
	}
	if _, exists := pkg.Dependencies["unique-names-generator"]; exists {
		t.Fatal("dependencies should not include unique-names-generator")
	}
}

func TestEnsureOpencodePackageJSON_UpdatesPluginToMinimumWhenTooOld(t *testing.T) {
	home := t.TempDir()
	opencodeDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(opencodeDir): %v", err)
	}

	initialPackage := map[string]any{
		"dependencies": map[string]string{
			"@opencode-ai/plugin": "1.2.0",
		},
	}
	initialPackageBytes, _ := json.MarshalIndent(initialPackage, "", "  ")
	initialPackageBytes = append(initialPackageBytes, '\n')
	if err := os.WriteFile(filepath.Join(opencodeDir, "package.json"), initialPackageBytes, 0o644); err != nil {
		t.Fatalf("WriteFile(package.json): %v", err)
	}

	warning := ensureOpencodePackageJSON(opencodeDir)
	if warning != "" {
		t.Fatalf("ensureOpencodePackageJSON() warning = %q, want empty in successful flow", warning)
	}

	pkgBytes, err := os.ReadFile(filepath.Join(opencodeDir, "package.json"))
	if err != nil {
		t.Fatalf("ReadFile(package.json): %v", err)
	}
	var pkg opencodePackageJSON
	if err := json.Unmarshal(pkgBytes, &pkg); err != nil {
		t.Fatalf("unmarshal package.json: %v", err)
	}
	if got := pkg.Dependencies["@opencode-ai/plugin"]; got != opencodePluginMinVersion {
		t.Fatalf("@opencode-ai/plugin = %q, want %q", got, opencodePluginMinVersion)
	}
}

func TestEnsureOpencodePackageJSON_PreservesNewerPluginVersion(t *testing.T) {
	home := t.TempDir()
	opencodeDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(opencodeDir): %v", err)
	}

	initialPackage := map[string]any{
		"dependencies": map[string]string{
			"@opencode-ai/plugin": "1.9.0",
		},
	}
	initialPackageBytes, _ := json.MarshalIndent(initialPackage, "", "  ")
	initialPackageBytes = append(initialPackageBytes, '\n')
	if err := os.WriteFile(filepath.Join(opencodeDir, "package.json"), initialPackageBytes, 0o644); err != nil {
		t.Fatalf("WriteFile(package.json): %v", err)
	}

	warning := ensureOpencodePackageJSON(opencodeDir)
	if warning != "" {
		t.Fatalf("ensureOpencodePackageJSON() warning = %q, want empty", warning)
	}

	pkgBytes, err := os.ReadFile(filepath.Join(opencodeDir, "package.json"))
	if err != nil {
		t.Fatalf("ReadFile(package.json): %v", err)
	}
	var pkg opencodePackageJSON
	if err := json.Unmarshal(pkgBytes, &pkg); err != nil {
		t.Fatalf("unmarshal package.json: %v", err)
	}
	if got := pkg.Dependencies["@opencode-ai/plugin"]; got != "1.9.0" {
		t.Fatalf("@opencode-ai/plugin = %q, want preserved newer version %q", got, "1.9.0")
	}
}

func TestEnsureOpencodePackageJSON_PreservesUnrelatedFields(t *testing.T) {
	home := t.TempDir()
	opencodeDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(opencodeDir): %v", err)
	}

	initialPackage := map[string]any{
		"name":    "my-opencode-config",
		"private": true,
		"scripts": map[string]string{
			"dev":   "bun run dev",
			"build": "bun run build",
		},
		"dependencies": map[string]string{
			"@opencode-ai/plugin": "1.4.2",
		},
		"devDependencies": map[string]string{
			"@types/node": "^22.0.0",
			"bun-types":   "latest",
		},
	}
	initialPackageBytes, _ := json.MarshalIndent(initialPackage, "", "  ")
	initialPackageBytes = append(initialPackageBytes, '\n')
	if err := os.WriteFile(filepath.Join(opencodeDir, "package.json"), initialPackageBytes, 0o644); err != nil {
		t.Fatalf("WriteFile(package.json): %v", err)
	}

	warning := ensureOpencodePackageJSON(opencodeDir)
	if warning != "" {
		t.Fatalf("ensureOpencodePackageJSON() warning = %q, want empty in successful flow", warning)
	}

	pkgBytes, err := os.ReadFile(filepath.Join(opencodeDir, "package.json"))
	if err != nil {
		t.Fatalf("ReadFile(package.json): %v", err)
	}
	var pkg map[string]any
	if err := json.Unmarshal(pkgBytes, &pkg); err != nil {
		t.Fatalf("unmarshal package.json: %v", err)
	}

	if got := pkg["name"]; got != "my-opencode-config" {
		t.Fatalf("name = %v, want %q", got, "my-opencode-config")
	}
	if got := pkg["private"]; got != true {
		t.Fatalf("private = %v, want true", got)
	}
	if _, ok := pkg["scripts"].(map[string]any); !ok {
		t.Fatal("scripts field was lost or changed type")
	}
	devDeps, ok := pkg["devDependencies"].(map[string]any)
	if !ok {
		t.Fatal("devDependencies field was lost or changed type")
	}
	if got := devDeps["@types/node"]; got != "^22.0.0" {
		t.Fatalf("devDependencies[@types/node] = %v, want %q", got, "^22.0.0")
	}
	if got := devDeps["bun-types"]; got != "latest" {
		t.Fatalf("devDependencies[bun-types] = %v, want %q", got, "latest")
	}

	deps, ok := pkg["dependencies"].(map[string]any)
	if !ok {
		t.Fatal("dependencies field was lost or changed type")
	}
	if got := deps["@opencode-ai/plugin"]; got != "1.4.2" {
		t.Fatalf("dependencies[@opencode-ai/plugin] = %v, want %q", got, "1.4.2")
	}
	if _, exists := deps["unique-names-generator"]; exists {
		t.Fatal("dependencies should not include unique-names-generator")
	}
}

func TestIsVersionBelowMinimum(t *testing.T) {
	tests := []struct {
		name    string
		current string
		minimum string
		want    bool
	}{
		{name: "older exact version", current: "1.2.0", minimum: "1.4.2", want: true},
		{name: "equal version", current: "1.4.2", minimum: "1.4.2", want: false},
		{name: "newer version", current: "1.9.0", minimum: "1.4.2", want: false},
		{name: "caret version older", current: "^1.3.0", minimum: "1.4.2", want: true},
		{name: "caret version newer", current: "^1.5.0", minimum: "1.4.2", want: false},
		{name: "unknown tag preserved", current: "latest", minimum: "1.4.2", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isVersionBelowMinimum(tt.current, tt.minimum)
			if got != tt.want {
				t.Fatalf("isVersionBelowMinimum(%q, %q) = %v, want %v", tt.current, tt.minimum, got, tt.want)
			}
		})
	}
}
