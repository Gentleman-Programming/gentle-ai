package sdd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
)

func openCodePluginAssetDirectory(agent model.AgentID) (string, error) {
	if agent == model.AgentKilocode {
		return "opencode/plugins/", nil
	}
	major, err := opencode.DetectRuntimeMajor(context.Background())
	if err != nil {
		return "", err
	}
	return major.PluginAssetDirectory()
}

// V2 replacement is a code upgrade only, never adoption of similarly named user
// plugins. Preflight the complete set before any write/removal.
func validateOpenCodePluginReplacement(dir, assetDir string) error {
	if assetDir != "opencode/plugins-v2/" {
		return nil
	}
	if info, err := os.Lstat(dir); err == nil && !info.IsDir() {
		return fmt.Errorf("OpenCode plugin directory conflict; user path preserved")
	}
	for _, name := range append(ManagedOpenCodePluginNames(), LegacyOpenCodeReviewPluginName, "background-agents.ts") {
		p := filepath.Join(dir, name)
		info, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("OpenCode plugin %s is not regular; user path preserved", name)
		}
		data, err := os.ReadFile(p)
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
