package opencoderuntimeplugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

//go:generate bash ../../../scripts/gen-opencode-plugin-digests.sh released_digests.go

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

// UserOwnedPathError reports a plugin path whose bytes or type Gentle AI cannot
// prove it wrote. The path is preserved; callers may scope the refusal to the
// plugin convergence instead of aborting unrelated work (issue #5393).
type UserOwnedPathError struct {
	Path    string
	message string
}

func (e *UserOwnedPathError) Error() string { return e.message }

func userOwnedPath(path, format string) error {
	return &UserOwnedPathError{Path: path, message: fmt.Sprintf(format, path)}
}

// ValidateReplacement refuses, before any plugin changes, every path Install
// would replace or remove unless it is a regular file holding bytes a Gentle AI
// release shipped for that name. Any other bytes are user-owned.
func ValidateReplacement(dir string, agent model.AgentID) error {
	if info, err := os.Lstat(dir); err == nil && !info.IsDir() {
		return userOwnedPath(dir, "OpenCode plugin directory %s is not a directory; user path preserved; move or delete it to let Gentle AI install its managed plugins")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, name := range OpenCodePluginLifecycleNames(agent) {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return userOwnedPath(path, "OpenCode plugin %s is not a regular file; user path preserved; move or delete it to let Gentle AI install its managed plugins")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !ReleasedPlugin(name, data) {
			return userOwnedPath(path, "OpenCode plugin %s does not match any Gentle AI release; custom bytes preserved; move or delete it to let Gentle AI install its managed plugins")
		}
	}
	return nil
}

// ReleasedPlugin is the ownership proof for managed and retired plugins: data
// is Gentle AI-owned only when some release shipped exactly these bytes under
// name. Every other byte sequence belongs to the user.
func ReleasedPlugin(name string, data []byte) bool {
	sum := sha256.Sum256(data)
	return slices.Contains(releasedPluginDigests[name], hex.EncodeToString(sum[:]))
}

func Install(home string, adapter agents.Adapter) (Result, error) {
	assetDir, err := AssetDirectory(adapter.Agent())
	if err != nil {
		return Result{}, err
	}
	return InstallFromDirectory(home, adapter, assetDir)
}

// PluginPaths lists every path Install can write or remove for the adapter.
func PluginPaths(home string, adapter agents.Adapter) []string {
	dir := filepath.Join(adapter.GlobalConfigDir(home), "plugins")
	paths := make([]string, 0)
	for _, name := range OpenCodePluginLifecycleNames(adapter.Agent()) {
		paths = append(paths, filepath.Join(dir, name))
	}
	return paths
}

func InstallFromDirectory(home string, adapter agents.Adapter, assetDir string) (Result, error) {
	dir := filepath.Join(adapter.GlobalConfigDir(home), "plugins")
	if err := ValidateReplacement(dir, adapter.Agent()); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Result{}, fmt.Errorf("create plugins dir: %w", err)
	}
	result := Result{}
	for _, name := range retiredPluginNames(adapter.Agent()) {
		path := filepath.Join(dir, name)
		if err := os.Remove(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return result, err
		}
		result.Changed = true
		result.Files = append(result.Files, path)
	}
	for _, name := range ManagedPluginNames(adapter.Agent()) {
		path := filepath.Join(dir, name)
		content := assets.MustRead(assetDir + name)
		wr, err := filemerge.WriteFileAtomic(path, []byte(content), 0644)
		if err != nil {
			return result, fmt.Errorf("write plugin %s: %w", name, err)
		}
		if wr.Changed {
			result.Changed = true
			result.Files = append(result.Files, path)
		}
	}
	return result, nil
}
