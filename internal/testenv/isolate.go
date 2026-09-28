// Package testenv provides hermeticity helpers for Go test binaries whose
// tests resolve agent config paths through adapters that honor runtime-dir
// override environment variables (for example PI_CODING_AGENT_DIR).
package testenv

import "os"

// overrideEnvVars lists the runtime-dir override environment variables that
// gentle-ai's own agent-path resolution honors unconditionally when set to
// an absolute value, bypassing whatever homeDir a caller passes in:
//   - PI_CODING_AGENT_DIR (internal/agents/pi.AgentConfigPath)
//   - OPENCODE_CONFIG_DIR (internal/opencode.ResolveRuntimeConfigForHome)
//
// A developer shell that exports either of these (Gentle Shell does, for its
// isolated Pi/OpenCode homes) leaks into any `go test` invocation that
// inherits it, redirecting tests into the real ~/.pi or ~/.config instead of
// their own temp home. XDG_CONFIG_HOME/APPDATA are deliberately excluded:
// every adapter that reads them already guards with an explicit
// homeDir-equals-os.UserHomeDir() check, which an arbitrary t.TempDir() home
// never satisfies.
var overrideEnvVars = []string{
	"PI_CODING_AGENT_DIR",
	"OPENCODE_CONFIG_DIR",
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
