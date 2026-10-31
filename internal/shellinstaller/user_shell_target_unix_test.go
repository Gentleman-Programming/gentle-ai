//go:build linux || darwin

package shellinstaller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func detectPiFixture(t *testing.T) string {
	t.Helper()
	// DetectUserPi guards candidate prefixes with privateDirectory, which
	// requires the canonical spelling; on darwin t.TempDir is the /var alias.
	home := userUnixTempRoot(t)
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0700); err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Join(home, ".npm-global")
	bundle := filepath.Join(prefix, "lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle")
	if err := os.MkdirAll(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "cli.js"), []byte("stock cli"), 0600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestDetectUserPiSingleCandidate(t *testing.T) {
	t.Setenv("NPM_CONFIG_PREFIX", "")
	home := detectPiFixture(t)
	got := DetectUserPi(home)
	if !got.Found || got.Prefix != filepath.Join(home, ".npm-global") || got.Agent != filepath.Join(home, ".pi", "agent") || got.Problem != "" {
		t.Fatalf("single Pi not detected: %+v", got)
	}
	// A second qualifying prefix makes the choice ambiguous instead of guessing.
	if err := os.MkdirAll(filepath.Join(home, ".local/lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".local/lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js"), []byte("stock cli"), 0600); err != nil {
		t.Fatal(err)
	}
	if ambiguous := DetectUserPi(home); ambiguous.Found || ambiguous.Problem != "ambiguous" {
		t.Fatalf("ambiguous Pi was selected: %+v", ambiguous)
	}
}

func TestDetectUserPiMissing(t *testing.T) {
	t.Setenv("NPM_CONFIG_PREFIX", "")
	home := t.TempDir()
	if got := DetectUserPi(home); got.Found || got.Problem != "missing" {
		t.Fatalf("empty home detected Pi: %+v", got)
	}
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0700); err != nil {
		t.Fatal(err)
	}
	if got := DetectUserPi(home); got.Found || got.Problem != "missing" {
		t.Fatalf("Pi agent without a bundle was detected: %+v", got)
	}
}

func TestDetectUserPiRestrictedPrefix(t *testing.T) {
	t.Setenv("NPM_CONFIG_PREFIX", "")
	home := detectPiFixture(t)
	if err := os.Chmod(filepath.Join(home, ".npm-global"), 0755); err != nil {
		t.Fatal(err)
	}
	got := DetectUserPi(home)
	if got.Found || got.Problem != "restricted" || !strings.Contains(got.Detail, "private owned 0700") {
		t.Fatalf("restricted Pi was not explained: %+v", got)
	}
}
