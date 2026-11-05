package engram

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// v1 adapter fingerprints (#4795): the pre-protocol adapter imported the V1
// plugin SDK and exported the plugin as a const entrypoint. Both markers must
// be present so user-authored or v2 Plugin.define files never match.
var (
	v1MarkerImport = "import type { Plugin } from \"@opencode-ai/plugin\""
	v1MarkerExport = "export const Engram: Plugin"
)

// retireStaleOpenCodeV1Adapter removes a v1 engram.ts plugin adapter from the
// OpenCode plugins directory when its bytes match the v1 fingerprint. On
// OpenCode V2 the v1 export shape fails the plugin loader at every boot
// (#4795); the current architecture delivers the protocol through AGENTS.md
// and MCP configuration instead. User-modified or v2 files do not match the
// fingerprint and are preserved. It returns the removed path, or "" when
// there was nothing to retire.
func retireStaleOpenCodeV1Adapter(configDir string) (string, error) {
	path := filepath.Join(configDir, "plugins", "engram.ts")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read stale engram plugin %s: %w", path, err)
	}
	text := string(data)
	if !strings.Contains(text, v1MarkerImport) || !strings.Contains(text, v1MarkerExport) {
		return "", nil
	}
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("remove stale engram plugin %s: %w", path, err)
	}
	return path, nil
}
