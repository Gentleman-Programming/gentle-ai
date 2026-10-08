//go:build linux || darwin

package shellinstaller

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnixCanonicalRequestKeepsUncheckablePathsVerbatim(t *testing.T) {
	parent := t.TempDir()
	canonical, err := userCanonicalPath(filepath.Join(parent, "shell"))
	if err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(parent)
	if err != nil || canonical != filepath.Join(physical, "shell") {
		t.Fatalf("canonical spelling = %q, want below %q (%v)", canonical, physical, err)
	}
	req := UserInstallRequest{Destination: filepath.Join(parent, "shell"), Mode: "separate", SharedAgent: "relative/agent", Confirmation: "token"}
	got, err := userCanonicalRequest(req)
	if err != nil || got.Destination != canonical || got.SharedPrefix != "" || got.SharedAgent != "relative/agent" || got.Mode != "separate" || got.Confirmation != "token" {
		t.Fatalf("canonical request = %+v %v", got, err)
	}
	if again, err := userCanonicalRequest(got); err != nil || again != got {
		t.Fatalf("canonicalization is not idempotent: %+v %v", again, err)
	}
	if _, err := os.Lstat(canonical); !os.IsNotExist(err) {
		t.Fatalf("canonicalization had effects: %v", err)
	}
}
