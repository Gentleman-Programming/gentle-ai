package engram

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRetireStaleOpenCodeV1Adapter proves the engram component retires a v1
// engram.ts plugin adapter left by older installs (#4795): on OpenCode V2 its
// export shape fails the plugin loader at every boot. Files that do not match
// the v1 fingerprint (user-modified or v2 Plugin.define files) are preserved.
func TestRetireStaleOpenCodeV1Adapter(t *testing.T) {
	configDir := t.TempDir()
	pluginsDir := filepath.Join(configDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	v1 := "import type { Plugin } from \"@opencode-ai/plugin\"\n\nexport const Engram: Plugin = async (ctx) => {\n  return {}\n}\n"
	path := filepath.Join(pluginsDir, "engram.ts")
	if err := os.WriteFile(path, []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, err := retireStaleOpenCodeV1Adapter(configDir)
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if removed != path {
		t.Fatalf("removed = %q, want %q", removed, path)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stale v1 adapter still exists")
	}

	// Absent file: nothing to do, no error.
	removed, err = retireStaleOpenCodeV1Adapter(configDir)
	if err != nil || removed != "" {
		t.Fatalf("absent file: removed=%q err=%v", removed, err)
	}

	// A v2 Plugin.define file does not match the v1 fingerprint.
	v2 := "import { Plugin } from \"@opencode/plugin\"\n\nexport default Plugin.define({ id: \"engram\", setup: async () => {} })\n"
	if err := os.WriteFile(path, []byte(v2), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err = retireStaleOpenCodeV1Adapter(configDir)
	if err != nil {
		t.Fatalf("retire v2 file: %v", err)
	}
	if removed != "" {
		t.Fatalf("v2 file was retired")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("v2 file removed")
	}

	// A user-modified file without the v1 markers is preserved too.
	user := "import { Plugin } from \"@opencode-ai/plugin\"\n\n// custom local adapter\nexport default {}\n"
	if err := os.WriteFile(path, []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err = retireStaleOpenCodeV1Adapter(configDir)
	if err != nil {
		t.Fatalf("retire user file: %v", err)
	}
	if removed != "" {
		t.Fatalf("user-authored file was retired")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("user file removed")
	}
}
