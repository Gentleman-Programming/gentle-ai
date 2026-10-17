//go:build linux && shell_gga_canonical

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Actual staged source CLI and existing owned group, not installed acceptance,
// a model-issued tool call, a fabricated Node/supervisor or provider execution.
func TestUserGGALaunchEnvironment(t *testing.T) {
	if testing.Short() {
		t.Skip("real GGA process-group integration")
	}
	archive, err := os.ReadFile("/frozen-gga-source.tgz")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"version", "config", "absent selection", "Shared marker", "invalid command", "mutated payload"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
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
			if err := userGGAReadback(ctx, root); err != nil {
				t.Fatal(err)
			}
			manifest := userManifest{Mode: "separate", GGA: true, Agent: filepath.Join(root, "agent")}
			if name == "absent selection" {
				manifest.GGA = false
			}
			if name == "Shared marker" {
				manifest.Mode = "shared"
			}
			// Consume the actual ordinary launch builder's environment. Its Node
			// executable is not substituted, created or executed by this fixture.
			planned := userLaunchCommand(ctx, root, manifest, "/not-executed/cli.js", nil)
			private := filepath.Join(root, "runtime/gga/bin")
			allowed := manifest.GGA && manifest.Mode == "separate"
			for _, entry := range planned.Env {
				if strings.HasPrefix(entry, "PATH=") && strings.Contains(entry, private) != allowed {
					t.Fatal("private GGA selection differs", entry)
				}
			}
			script := `command -v gga && exec gga "$@"`
			argument := "--version"
			if name == "config" {
				argument = "config"
			}
			if name == "invalid command" {
				argument = "not-a-command"
			}
			if !allowed {
				script = `resolved=$(command -v gga || true); test "$resolved" != "$1"`
			}
			if name == "mutated payload" {
				script = `gga --version && printf 'tampered DATA' > "$1"`
			}
			if !allowed || name == "mutated payload" {
				argument = filepath.Join(private, "gga")
			}
			cmd := exec.CommandContext(ctx, "/usr/bin/bash", "-c", script, "probe", argument)
			cmd.Dir, cmd.Env = filepath.Join(root, "project"), planned.Env
			var output bytes.Buffer
			cmd.Stdout, cmd.Stderr = &output, &output
			before, err := userTreeStamp(root)
			if err != nil {
				t.Fatal(err)
			}
			readbacks := 0
			err = userLaunchGroup(cmd, nil, func(waitErr error) error {
				readbacks++
				if probe := syscall.Kill(-cmd.Process.Pid, 0); !errors.Is(probe, syscall.ESRCH) {
					t.Error("readback preceded group reap", probe)
				}
				return userGGAExitReadback(context.Background(), root, manifest.GGA, waitErr)
			})
			if readbacks != 1 {
				t.Fatal("owned group did not complete readback", err)
			}
			text := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(output.String(), "")
			if name == "invalid command" {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 1 {
					t.Fatal("CLI failure was hidden", err)
				}
			} else if name == "mutated payload" {
				var failure *PrivateRuntimeError
				if !errors.As(err, &failure) || failure.Kind != "uncertain" || failure.Workspace != root {
					t.Fatal("payload drift did not retain uncertain root", err)
				}
			} else if err != nil {
				t.Fatalf("actual CLI/group failed: %v\n%s", err, text)
			}
			if name == "version" && !strings.Contains(text, "gga v2.10.1") {
				t.Fatal("actual recipe version absent", text)
			}
			if name == "config" && (!strings.Contains(text, "codex") || !strings.Contains(text, filepath.Join(root, "config/gga/config"))) {
				t.Fatal("actual CLI did not consume private config", text)
			}
			after, stampErr := userTreeStamp(root)
			if stampErr != nil {
				t.Fatal(stampErr)
			}
			if name != "mutated payload" && before != after {
				t.Fatal("CLI/group changed private stage")
			}
			if name == "mutated payload" && before == after {
				t.Fatal("controlled mutation did not happen")
			}
		})
	}
}

func TestUserGGALaunchRefusesAmbiguousPATH(t *testing.T) {
	root := filepath.Join(t.TempDir(), "parent:alias", "stage")
	manifest := userManifest{Mode: "separate", GGA: true}
	cmd := userLaunchCommand(context.Background(), root, manifest, "/not-executed/cli.js", nil)
	if cmd.Err == nil {
		t.Fatal("ambiguous PATH root admitted")
	}
	if err := cmd.Start(); !errors.Is(err, cmd.Err) || cmd.Process != nil {
		t.Fatal("refused launch attempted a process", err)
	}
}

// Manifest DATA: the real launch consumer must reject this combination before
// any supervisor, Node, Bash or payload execution; no full runtime is invented.
func TestUserGGASharedManifestRefusesLaunch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "stage")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := userManifest{Schema: userSchema, Destination: root, Mode: "shared", GGA: true}
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
	var output bytes.Buffer
	err = userLaunch(context.Background(), root, nil, strings.NewReader(""), &output, &output)
	if err == nil || !strings.Contains(err.Error(), "GGA requires a Separate manifest") {
		t.Fatal("real launch consumer did not reject Shared GGA at manifest validation", err)
	}
	after, stampErr := userTreeStamp(root)
	if stampErr != nil || before != after || output.Len() != 0 {
		t.Fatal("manifest refusal changed stage or issued CLI presentation", stampErr, output.String())
	}
}
