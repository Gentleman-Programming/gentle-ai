//go:build windows

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserWindowsMainDescriptorReadback(t *testing.T) {
	commit := strings.Repeat("a", 40)
	for _, mutation := range []string{"canonical", "wrong URL", "unknown field"} {
		t.Run(mutation, func(t *testing.T) {
			root := t.TempDir()
			artifact, _ := userWindowsMainArtifact(commit, strings.Repeat("ab", 32))
			if mutation == "wrong URL" {
				artifact.URL = "https://foreign.invalid/source.zip"
			}
			data, _ := json.Marshal(artifact)
			if mutation == "unknown field" {
				data = []byte(strings.TrimSuffix(string(data), "}") + `,"Injected":true}`)
			}
			if err := userWindowsWrite(filepath.Join(root, "main.json"), data); err != nil {
				t.Fatal(err)
			}
			_, err := userWindowsMainRead(root, userWindowsManifest{MainCommit: commit, MainArchiveSHA: artifact.SHA})
			if (err != nil) != (mutation != "canonical") {
				t.Fatalf("Main descriptor readback %s: %v", mutation, err)
			}
		})
	}
}

func TestUserWindowsMainArchiveAuthority(t *testing.T) {
	commit := strings.Repeat("a", 40)
	prefix := "gentle-shell-" + commit
	for _, tt := range []struct{ name, member string }{
		{"canonical", prefix + "/bin/entry.mjs"},
		{"wrong prefix", "foreign/entry.mjs"},
		{"traversal", prefix + "/../escape"},
		{"native replacement", prefix + "/.gentle-ai/entry"},
		{"dependency replacement", prefix + "/NODE_MODULES/entry"},
		{"duplicate", prefix + "/package.json"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := userWindowsZipFixture(t, prefix+"/package.json", tt.member)
			artifact, err := userWindowsMainArtifact(commit, userWindowsSHA(data))
			if err != nil {
				t.Fatal(err)
			}
			_, err = userWindowsMainFiles(context.Background(), data, artifact)
			if (err != nil) != (tt.name != "canonical") {
				t.Fatalf("Main archive %s: %v", tt.name, err)
			}
			artifact.SHA = strings.Repeat("0", 64)
			if _, err := userWindowsMainFiles(context.Background(), data, artifact); err == nil {
				t.Fatal("changed retained archive digest accepted")
			}
		})
	}
}

func TestUserWindowsVerifyGoZipRequiredFollowsMarker(t *testing.T) {
	root := t.TempDir()
	required, err := userWindowsVerifyGoZipRequired(root)
	if !required || err != nil {
		t.Fatalf("absent marker must keep the pinned go.zip contract: required=%v err=%v", required, err)
	}
	goExe := userWindowsSystemGoFixture(t, "")
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsSystemGoStage(root, userWindowsSystemGo{Go: goExe, Version: "go1.27.1"}); err != nil {
		t.Fatal(err)
	}
	required, err = userWindowsVerifyGoZipRequired(root)
	if required || err != nil {
		t.Fatalf("valid marker must accept the absent go.zip artifact: required=%v err=%v", required, err)
	}
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(filepath.Join(root, "runtime", "system-go.json"), []byte("{")); err != nil {
		t.Fatal(err)
	}
	_, err = userWindowsVerifyGoZipRequired(root)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("malformed marker was not refused: %v", err)
	}
}

// The complete verification must refuse a present but invalid reuse marker
// outright, before any pinned-archive noise can mask it.
func TestUserWindowsVerifyRefusesMalformedSystemGoMarker(t *testing.T) {
	root := userWindowsLaunchFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(filepath.Join(root, "runtime", "system-go.json"), []byte("{")); err != nil {
		t.Fatal(err)
	}
	err := userWindowsVerify(context.Background(), root, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "system Go") {
		t.Fatalf("complete verification did not refuse the malformed system Go marker: %v", err)
	}
}

// The owned provision/launch PATH entry for the Go toolchain follows the
// reuse marker: a valid marker puts the system Go directory on PATH and
// removes the pinned runtime entry; absence or any doubt keeps the pinned
// entry, so launch never depends on the marker.
func TestUserWindowsSystemGoBinFollowsMarker(t *testing.T) {
	goExe := userWindowsSystemGoFixture(t, "")
	root := t.TempDir()
	if got := userWindowsSystemGoBin(root); got != filepath.Join(root, "runtime", "go", "bin") {
		t.Fatalf("absent marker changed the pinned Go PATH entry: %q", got)
	}
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(filepath.Join(root, "runtime", "system-go.json"), []byte("{")); err != nil {
		t.Fatal(err)
	}
	if got := userWindowsSystemGoBin(root); got != filepath.Join(root, "runtime", "go", "bin") {
		t.Fatalf("malformed marker changed the pinned Go PATH entry: %q", got)
	}
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsSystemGoStage(root, userWindowsSystemGo{Go: goExe, Version: "go1.27.1"}); err != nil {
		t.Fatal(err)
	}
	if got := userWindowsSystemGoBin(root); got != filepath.Dir(goExe) {
		t.Fatalf("valid marker did not select the system Go directory: %q", got)
	}
}

func TestUserWindowsEnvironmentCarriesSystemGoPath(t *testing.T) {
	goExe := userWindowsSystemGoFixture(t, "")
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsSystemGoStage(root, userWindowsSystemGo{Go: goExe, Version: "go1.27.1"}); err != nil {
		t.Fatal(err)
	}
	env, err := userWindowsEnvironment(root)
	if err != nil {
		t.Fatalf("owned environment refused: %v", err)
	}
	var path string
	for _, entry := range env {
		if name, value, found := strings.Cut(entry, "="); found && name == "PATH" {
			path = value
		}
	}
	if !strings.Contains(path, filepath.Dir(goExe)) {
		t.Fatalf("provision PATH lacks the system Go directory: %q", path)
	}
	if strings.Contains(path, filepath.Join(root, "runtime", "go", "bin")) {
		t.Fatalf("provision PATH retains the pinned Go entry under reuse: %q", path)
	}
}

// A canceled native build leaves the stock installer's sealed Go module cache
// (bang-escaped names, read-only files) inside the worker's own stage.
func userWindowsCanceledStage(t *testing.T) (string, string) {
	t.Helper()
	parent := filepath.Join(t.TempDir(), "parent")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsPrivate(parent); err != nil {
		t.Fatal(err)
	}
	stage, err := os.MkdirTemp(parent, ".gentle-shell-windows-stage-")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := userWindowsIdentity(stage, true)
	if err != nil {
		t.Fatal(err)
	}
	module := filepath.Join(stage, filepath.FromSlash("prefix/node_modules/gentle-pi/.gentle-ai/.v4.0.0.staging-1/.build/gomodcache/github.com/!burnt!sushi/toml@v1.4.0/decode.go"))
	if err := os.MkdirAll(filepath.Dir(module), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(module, []byte("fixture-only, never executed"), 0400); err != nil {
		t.Fatal(err)
	}
	return stage, identity
}

func TestUserWindowsStageRemovalAfterCancellation(t *testing.T) {
	stage, identity := userWindowsCanceledStage(t)
	if err := userWindowsStageRemove(stage, identity); err != nil {
		t.Fatalf("owned canceled stage preserved: %v", err)
	}
	if _, err := os.Lstat(stage); !os.IsNotExist(err) {
		t.Fatalf("owned stage remains: %v", err)
	}
}

func TestUserWindowsStageRemovalRefusesUncertainCustody(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(t *testing.T, stage string) (string, string)
	}{
		{"replaced identity", func(t *testing.T, stage string) (string, string) {
			return "", "00000000:0000000000000000"
		}},
		{"foreign junction", func(t *testing.T, stage string) (string, string) {
			foreign := t.TempDir()
			marker := filepath.Join(foreign, "foreign.txt")
			if err := os.WriteFile(marker, []byte("foreign"), 0600); err != nil {
				t.Fatal(err)
			}
			userWindowsJunction(t, filepath.Join(stage, "home"), foreign)
			return marker, ""
		}},
		{"hard-linked foreign file", func(t *testing.T, stage string) (string, string) {
			marker := filepath.Join(t.TempDir(), "foreign.txt")
			if err := os.WriteFile(marker, []byte("foreign"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(marker, filepath.Join(stage, "linked.txt")); err != nil {
				t.Skipf("hard link fixture unavailable: %v", err)
			}
			return marker, ""
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stage, identity := userWindowsCanceledStage(t)
			marker, replaced := tt.mutate(t, stage)
			if replaced != "" {
				identity = replaced
			}
			if err := userWindowsStageRemove(stage, identity); err == nil {
				t.Fatal("uncertain stage custody removed")
			}
			if _, err := os.Lstat(stage); err != nil {
				t.Fatalf("uncertain stage was not preserved: %v", err)
			}
			if marker != "" {
				if got, err := os.ReadFile(marker); err != nil || string(got) != "foreign" {
					t.Fatalf("foreign data changed: %q %v", got, err)
				}
			}
		})
	}
}

func TestUserWindowsMainExactSourceSet(t *testing.T) {
	ctx := context.Background()
	commit := strings.Repeat("a", 40)
	prefix := "gentle-shell-" + commit
	data, _ := userWindowsZipFixture(t, prefix+"/package.json", prefix+"/bin/entry.mjs")
	artifact, _ := userWindowsMainArtifact(commit, userWindowsSHA(data))
	for _, mutation := range []string{"unchanged", "extra file", "extra directory", "changed bytes", "missing source", "alias"} {
		t.Run(mutation, func(t *testing.T) {
			root := t.TempDir()
			if mutation != "missing source" && mutation != "alias" {
				if _, err := userWindowsZIP(ctx, data, artifact, root, ""); err != nil {
					t.Fatal(err)
				}
			}
			var err error
			switch mutation {
			case "extra file":
				err = userWindowsWrite(filepath.Join(root, "extra.mjs"), nil)
			case "extra directory":
				err = os.Mkdir(filepath.Join(root, "extra"), 0700)
			case "changed bytes":
				err = os.WriteFile(filepath.Join(root, "package.json"), []byte("changed fixture"), 0600)
			case "alias":
				real := filepath.Join(t.TempDir(), "real.json")
				if err := os.WriteFile(real, []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(real, filepath.Join(root, "package.json")); err != nil {
					t.Skipf("unelevated symlink unavailable: %v", err)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			err = userWindowsCompareMain(ctx, data, artifact, root)
			if (err != nil) != (mutation != "unchanged") {
				t.Fatalf("Main exact-set %s: %v", mutation, err)
			}
		})
	}
}
