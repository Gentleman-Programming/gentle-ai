package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// isolateAgentDiscoveryEnv pins HOME/USERPROFILE and, on Windows, APPDATA/LOCALAPPDATA
// under home so agent discovery (e.g. Kiro’s GlobalConfigDir on Windows) does not see
// the developer’s real profile while tests pass a temp directory as home.
func isolateAgentDiscoveryEnv(t *testing.T, home string) {
	t.Helper()
	keys := []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME"}
	orig := make(map[string]string, len(keys))
	for _, k := range keys {
		orig[k] = os.Getenv(k)
	}
	t.Cleanup(func() {
		for _, k := range keys {
			if orig[k] == "" {
				_ = os.Unsetenv(k)
			} else {
				os.Setenv(k, orig[k])
			}
		}
	})
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	if runtime.GOOS == "windows" {
		os.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
		os.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	}
}

// ensureOpenCodeSDDNodeStub creates node_modules/unique-names-generator under the
// OpenCode config dir so SDD post-install checks pass without a real bun/npm run in CI.
func ensureOpenCodeSDDNodeStub(t *testing.T, home string) {
	t.Helper()
	p := filepath.Join(home, ".config", "opencode", "node_modules", "unique-names-generator")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", p, err)
	}
}
