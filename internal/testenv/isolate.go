// Package testenv provides hermeticity helpers for Go test binaries whose
// tests resolve agent config paths through adapters that honor runtime-dir
// override environment variables (for example PI_CODING_AGENT_DIR).
package testenv

import "os"

// overrideEnvVars lists inherited config-path overrides that can bypass
// a test's temporary home during agent-path resolution:
//   - PI_CODING_AGENT_DIR (internal/agents/pi.AgentConfigPath)
//   - OPENCODE_CONFIG_DIR (internal/opencode.ResolveRuntimeConfigForHome)
//   - XDG_CONFIG_HOME (OpenCode config resolution when homeDir is the OS home)
//
// Inherited overrides can redirect tests into external configuration instead
// of their own temp home. Tests that mock HOME can satisfy OpenCode's
// homeDir-equals-os.UserHomeDir() check, so XDG_CONFIG_HOME must also be cleared.
// Tests may explicitly set overrides after isolation to exercise resolution.
var overrideEnvVars = []string{
	"PI_CODING_AGENT_DIR",
	"OPENCODE_CONFIG_DIR",
	"XDG_CONFIG_HOME",
}

// Isolate unsets every known agent runtime-dir override environment
// variable for the current process. Call it from a package's TestMain,
// before m.Run(), so no test in that binary can read an ambient override
// that points outside the test's own temp home.
func Isolate() {
	for _, key := range overrideEnvVars {
		if err := os.Unsetenv(key); err != nil {
			panic("testenv.Isolate: unsetenv " + key + ": " + err.Error())
		}
	}
}
