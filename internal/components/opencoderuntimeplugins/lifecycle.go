package opencoderuntimeplugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
)

type Result struct {
	Changed bool
	Files   []string
}

func AssetDirectory(agent model.AgentID) (string, error) {
	if agent == model.AgentKilocode {
		return "opencode/plugins/", nil
	}
	major, err := opencode.DetectRuntimeMajor(context.Background())
	if err != nil {
		return "", err
	}
	return major.PluginAssetDirectory()
}

// Preflight the full V2 replacement set before changing any plugin.
func ValidateReplacement(dir, assetDir string) error {
	if assetDir != "opencode/plugins-v2/" {
		return nil
	}
	if info, err := os.Lstat(dir); err == nil && !info.IsDir() {
		return fmt.Errorf("OpenCode plugin directory conflict; user path preserved")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, name := range append(OpenCodePluginLifecycleNames(model.AgentOpenCode), "background-agents.ts") {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("OpenCode plugin %s is not regular; user path preserved", name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		old, oldErr := assets.Read("opencode/plugins/" + name)
		next, nextErr := assets.Read("opencode/plugins-v2/" + name)
		if (oldErr != nil || string(data) != old) && (nextErr != nil || string(data) != next) {
			return fmt.Errorf("OpenCode plugin %s has unverified ownership; custom bytes preserved", name)
		}
	}
	return nil
}

func legacyReview(dir string) (string, bool, error) {
	path := filepath.Join(dir, LegacyOpenCodeReviewPluginName)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return path, false, nil
	}
	if err != nil {
		return path, false, err
	}
	if !info.Mode().IsRegular() {
		return path, false, fmt.Errorf("legacy OpenCode review plugin %s is not a regular file", path)
	}
	return path, true, nil
}

func Install(home string, adapter agents.Adapter) (Result, error) {
	assetDir, err := AssetDirectory(adapter.Agent())
	if err != nil {
		return Result{}, err
	}
	return InstallFromDirectory(home, adapter, assetDir)
}

func InstallFromDirectory(home string, adapter agents.Adapter, assetDir string) (Result, error) {
	dir := filepath.Join(adapter.GlobalConfigDir(home), "plugins")
	if err := ValidateReplacement(dir, assetDir); err != nil {
		return Result{}, err
	}
	legacy, exists, err := legacyReview(dir)
	if err != nil {
		return Result{}, err
	}
	// Refuse every nonregular managed path before writing or removing anything.
	for _, name := range ManagedPluginNames(adapter.Agent()) {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return Result{}, err
		}
		if !info.Mode().IsRegular() {
			return Result{}, fmt.Errorf("OpenCode plugin %s is not regular; user path preserved", name)
		}
	}
	if adapter.Agent() == model.AgentOpenCode {
		path := filepath.Join(dir, "background-agents.ts")
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return Result{}, err
		}
		if err == nil && !info.Mode().IsRegular() {
			return Result{}, fmt.Errorf("legacy OpenCode plugin %s is not regular; user path preserved", path)
		}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Result{}, fmt.Errorf("create plugins dir: %w", err)
	}
	result := Result{}
	if adapter.Agent() == model.AgentOpenCode {
		path := filepath.Join(dir, "background-agents.ts")
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return result, err
		}
		if err == nil {
			if !info.Mode().IsRegular() {
				return result, fmt.Errorf("legacy OpenCode plugin %s is not regular; user path preserved", path)
			}
			if err := os.Remove(path); err != nil {
				return result, err
			}
			result.Changed = true
			result.Files = append(result.Files, path)
		}
	}
	if exists {
		if err := os.Remove(legacy); err != nil {
			return result, err
		}
		result.Changed = true
		result.Files = append(result.Files, legacy)
	}
	for _, name := range ManagedPluginNames(adapter.Agent()) {
		path := filepath.Join(dir, name)
		content := assets.MustRead(assetDir + name)
		wr, err := filemerge.WriteFileAtomic(path, []byte(content), 0644)
		if err != nil {
			return result, fmt.Errorf("write plugin %s: %w", name, err)
		}
		result.Files = append(result.Files, path)
		result.Changed = result.Changed || wr.Changed
	}
	return result, nil
}

// Refresh updates only installed regular plugins, except the review transport
// introduced when migrating an installed legacy review plugin.
func Refresh(home string, adapter agents.Adapter) (Result, error) {
	assetDir, err := AssetDirectory(adapter.Agent())
	if err != nil {
		return Result{}, err
	}
	dir := filepath.Join(adapter.GlobalConfigDir(home), "plugins")
	if err := ValidateReplacement(dir, assetDir); err != nil {
		return Result{}, err
	}
	legacy, migrate, err := legacyReview(dir)
	if err != nil {
		return Result{}, err
	}
	result := Result{}
	if migrate {
		if err := os.Remove(legacy); err != nil {
			return result, err
		}
		result.Changed = true
		result.Files = append(result.Files, legacy)
	}
	for _, name := range ManagedPluginNames(adapter.Agent()) {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			if !(migrate && adapter.Agent() == model.AgentOpenCode && name == "opencode-review-transport.ts") {
				continue
			}
		} else if err != nil {
			return result, err
		} else if !info.Mode().IsRegular() {
			continue
		}
		wr, err := filemerge.WriteFileAtomic(path, []byte(assets.MustRead(assetDir+name)), 0644)
		if err != nil {
			return result, fmt.Errorf("refresh managed OpenCode plugin %s: %w", name, err)
		}
		if wr.Changed {
			result.Changed = true
			result.Files = append(result.Files, path)
		}
	}
	return result, nil
}
