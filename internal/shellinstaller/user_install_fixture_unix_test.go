//go:build linux || darwin

package shellinstaller

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeInstallFixture(t *testing.T, target string, manifest userManifest) {
	t.Helper()
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema == "" {
		manifest = userManifest{Schema: userSchema, Destination: target, Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent"), Mode: "separate"}
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "installation.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
