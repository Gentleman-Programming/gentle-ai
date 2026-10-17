//go:build linux && shell_gga_canonical

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserGGARetainedIntegrity(t *testing.T) {
	archive, err := os.ReadFile("/frozen-gga-source.tgz")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"unchanged", "CLI bytes", "providers bytes", "cache bytes", "pr bytes", "archive bytes", "missing archive", "CLI symlink", "CLI hardlink", "CLI mode", "directory mode", "extra source", "extra library"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := filepath.Join(t.TempDir(), "stage")
			profile := UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", GGA: "on", GGAProvider: "codex"}
			if err := userPrepareInstallStage(ctx, root, "separate", &profile); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := userStageGGAExecutable(ctx, root, archive); err != nil {
				t.Fatal(err)
			}
			base := filepath.Join(root, "runtime/gga")
			cli := filepath.Join(base, "bin/gga")
			var changeErr error
			switch name {
			case "CLI bytes":
				changeErr = os.WriteFile(cli, []byte("tampered DATA"), 0700)
			case "providers bytes":
				changeErr = os.WriteFile(filepath.Join(base, "lib/providers.sh"), []byte("tampered DATA"), 0700)
			case "cache bytes":
				changeErr = os.WriteFile(filepath.Join(base, "lib/cache.sh"), []byte("tampered DATA"), 0700)
			case "pr bytes":
				changeErr = os.WriteFile(filepath.Join(base, "lib/pr_mode.sh"), []byte("tampered DATA"), 0700)
			case "archive bytes":
				changeErr = os.WriteFile(filepath.Join(base, "source.tgz"), []byte("tampered DATA"), 0600)
			case "missing archive":
				changeErr = os.Remove(filepath.Join(base, "source.tgz"))
			case "CLI symlink":
				changeErr = os.Remove(cli)
				if changeErr == nil {
					changeErr = os.Symlink("../lib/providers.sh", cli)
				}
			case "CLI hardlink":
				changeErr = os.Link(cli, filepath.Join(root, "alias"))
			case "CLI mode":
				changeErr = os.Chmod(cli, 0755)
			case "directory mode":
				changeErr = os.Chmod(base, 0755)
			case "extra source":
				changeErr = os.WriteFile(filepath.Join(base, "extra"), []byte("unexpected DATA"), 0600)
			case "extra library":
				changeErr = os.WriteFile(filepath.Join(base, "lib/extra.sh"), []byte("unexpected DATA"), 0600)
			}
			if changeErr != nil {
				t.Fatal(changeErr)
			}
			before, err := userTreeStamp(root)
			if err != nil {
				t.Fatal(err)
			}
			err = userGGAReadback(ctx, root)
			if (err == nil) != (name == "unchanged") {
				t.Fatalf("retained integrity result: %v", err)
			}
			after, stampErr := userTreeStamp(root)
			if stampErr != nil || before != after {
				t.Fatal("integrity check mutated stage", stampErr)
			}
		})
	}
}

func TestUserGGAManifestSelection(t *testing.T) {
	// Manifest DATA fixtures stop before supervisor/Node qualification. Never
	// fabricate an installed runtime or native supplier to complete that gate.
	ctx := context.Background()
	for _, mode := range []string{"separate", "shared"} {
		t.Run(mode, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "stage")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			manifest := userManifest{Schema: userSchema, Destination: root, Mode: mode, GGA: true}
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if err := userToolWrite(filepath.Join(root, "installation.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			before, err := userTreeStamp(root)
			if err != nil {
				t.Fatal(err)
			}
			_, err = userReadManifest(ctx, root)
			if err == nil || !strings.Contains(err.Error(), "GGA") {
				t.Fatal("reader ignored selected GGA", err)
			}
			after, stampErr := userTreeStamp(root)
			if stampErr != nil || before != after {
				t.Fatal("manifest refusal mutated stage", stampErr)
			}
		})
	}
	data, err := json.Marshal(userManifest{})
	if err != nil {
		t.Fatal(err)
	}
	const historical = `{"Schema":"","Destination":"","Prefix":"","Agent":"","Mode":"","SupervisorSHA":"","NodeSHA":"","PrefixIdentity":"","AgentIdentity":""}`
	if !bytes.Equal(data, []byte(historical)) {
		t.Fatal("absent GGA changed historical manifest", string(data))
	}
}
