package opencode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ConfigLoadOptions controls effective OpenCode config discovery.
type ConfigLoadOptions struct {
	HomeDir      string
	SettingsPath string
	WorkDir      string
	WorktreeRoot string
	IncludeEnv   bool
}

// LoadEffectiveConfig discovers and merges the OpenCode config sources Gentle AI
// can support locally. Unsupported sources: remote config, managed config files,
// and macOS managed preferences.
func LoadEffectiveConfig(opts ConfigLoadOptions) (EffectiveConfig, error) {
	files, err := DiscoverConfigFiles(opts)
	if err != nil {
		return EffectiveConfig{}, err
	}

	merged := EffectiveConfig{
		Provider: map[string]ConfigProvider{},
		Agent:    map[string]ConfigAgent{},
	}
	var parseErrs []error
	for _, file := range files {
		var raw struct {
			Provider map[string]ConfigProvider `json:"provider"`
			Agent    map[string]ConfigAgent    `json:"agent"`
		}
		if err := json.Unmarshal(file.Data, &raw); err != nil {
			parseErrs = append(parseErrs, fmt.Errorf("parse opencode settings %q: %w", file.Path, err))
			continue
		}
		for id, provider := range raw.Provider {
			merged.Provider[id] = mergeConfigProvider(merged.Provider[id], provider)
		}
		for name, agent := range raw.Agent {
			merged.Agent[name] = mergeConfigAgent(merged.Agent[name], agent)
		}
	}
	return merged, errors.Join(parseErrs...)
}

// DiscoverConfigFiles returns config documents in precedence order, with later
// entries intended to override conflicting keys from earlier entries.
func DiscoverConfigFiles(opts ConfigLoadOptions) ([]ConfigFile, error) {
	seen := map[string]bool{}
	var files []ConfigFile

	addDir := func(dir string) error {
		if dir == "" {
			return nil
		}
		dirFiles, err := ReadConfigFiles(dir)
		if err != nil {
			return err
		}
		for _, file := range dirFiles {
			key := file.Path
			if abs, err := filepath.Abs(file.Path); err == nil {
				key = abs
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			files = append(files, file)
		}
		return nil
	}

	homeDir := opts.HomeDir
	if homeDir == "" {
		homeDir = inferHomeDirFromSettingsPath(opts.SettingsPath)
		opts.HomeDir = homeDir
	}
	if homeDir != "" {
		if err := addDir(filepath.Join(homeDir, ".config", "opencode")); err != nil {
			return nil, err
		}
	}

	if opts.IncludeEnv {
		if customPath := envLookup("OPENCODE_CONFIG"); customPath != "" {
			file, err := readSingleConfigFile(customPath)
			if err != nil {
				if !os.IsNotExist(err) {
					return nil, err
				}
			} else {
				key := file.Path
				if abs, err := filepath.Abs(file.Path); err == nil {
					key = abs
				}
				if !seen[key] {
					seen[key] = true
					files = append(files, file)
				}
			}
		}
	}

	for _, dir := range projectConfigDirs(opts) {
		if err := addDir(dir); err != nil {
			return nil, err
		}
		if err := addDir(filepath.Join(dir, ".opencode")); err != nil {
			return nil, err
		}
	}
	if opts.SettingsPath != "" {
		dir := opts.SettingsPath
		if filepath.Ext(opts.SettingsPath) != "" {
			dir = filepath.Dir(opts.SettingsPath)
		}
		if err := addDir(dir); err != nil {
			return nil, err
		}
	}

	if opts.IncludeEnv {
		if inline := envLookup("OPENCODE_CONFIG_CONTENT"); inline != "" {
			files = append(files, ConfigFile{Path: "OPENCODE_CONFIG_CONTENT", Data: sanitizeJSONC([]byte(inline))})
		}
	}

	return files, nil
}

func readSingleConfigFile(path string) (ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, err
	}
	return ConfigFile{Path: path, Data: sanitizeJSONC(data)}, nil
}

func inferHomeDirFromSettingsPath(settingsPath string) string {
	if settingsPath == "" {
		return ""
	}
	clean := filepath.Clean(settingsPath)
	configDir := filepath.Dir(clean)
	if filepath.Base(configDir) != "opencode" {
		return ""
	}
	dotConfigDir := filepath.Dir(configDir)
	if filepath.Base(dotConfigDir) != ".config" {
		return ""
	}
	return filepath.Dir(dotConfigDir)
}

func projectConfigDirs(opts ConfigLoadOptions) []string {
	workDir := opts.WorkDir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return nil
		}
	}
	workDir, _ = filepath.Abs(workDir)
	root := opts.WorktreeRoot
	if root != "" {
		root, _ = filepath.Abs(root)
	}
	if root == "" {
		root = nearestConfigRoot(workDir, opts.HomeDir)
	}

	var reversed []string
	for dir := workDir; dir != ""; dir = filepath.Dir(dir) {
		reversed = append(reversed, dir)
		if dir == root {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed
}

func nearestConfigRoot(workDir, homeDir string) string {
	if homeDir != "" {
		if absHome, err := filepath.Abs(homeDir); err == nil {
			homeDir = absHome
		}
	}
	for dir := workDir; dir != ""; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if homeDir != "" && dir == homeDir {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
	}
	return workDir
}

func mergeConfigAgent(base ConfigAgent, override ConfigAgent) ConfigAgent {
	merged := base
	if override.ModelSet {
		merged.Model = override.Model
		merged.ModelSet = true
	}
	if override.Variant != "" {
		merged.Variant = override.Variant
	}
	return merged
}
