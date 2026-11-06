//go:build linux && shell_codegraph_library

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUserCodeGraphFrozenLibrary(t *testing.T) {
	if testing.Short() {
		t.Skip("authenticated package library integration")
	}
	archives := make([][]byte, 2)
	for i, name := range []string{"/frozen-codegraph-main.tgz", "/frozen-codegraph-linux.tgz"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		archives[i] = data
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// The old single-member helper must NOT be relaxed for this large package.
	pin := fmt.Sprintf("%x", sha256.Sum256(archives[1]))
	if data, err := userToolMember(ctx, archives[1], pin, "package/lib/dist/bin/codegraph.js"); err == nil || data != nil {
		t.Fatal("generic 32MiB archive guard was relaxed")
	}
	root := installStagePath(t)
	for _, profile := range []*UserExperience{nil, {Persona: "unmanaged", BackgroundSubagents: "off"}, {Persona: "unmanaged", BackgroundSubagents: "off", CodeGraph: "off"}} {
		if err := userCodeGraphProvision(nil, root, "separate", profile); err != nil {
			t.Fatal("OFF/absence acquired or created a library", err)
		}
	}
	on := UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", CodeGraph: "on"}
	if err := userCodeGraphProvision(ctx, root, "shared", &on); err == nil {
		t.Fatal("Shared accepted CodeGraph acquisition")
	}
	if err := userPrepareInstallStage(ctx, root, "separate", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, which := range []int{0, 1} {
		t.Run(fmt.Sprintf("tampered-source-%d", which), func(t *testing.T) {
			bad := append([][]byte(nil), archives...)
			bad[which] = bytes.Clone(archives[which])
			bad[which][0] ^= 1
			if err := userStageCodeGraphLibrary(ctx, root, bad); err == nil {
				t.Fatal("unauthenticated source admitted")
			}
			if _, err := os.Lstat(filepath.Join(root, "runtime/codegraph")); !os.IsNotExist(err) {
				t.Fatal("failed authentication materialized a library", err)
			}
		})
	}
	stopped, stop := context.WithCancel(ctx)
	stop()
	if err := userCodeGraphProvision(stopped, root, "separate", &on); !errors.Is(err, context.Canceled) {
		t.Fatal("producer lost the selected context cancellation cause", err)
	}
	if err := userStageCodeGraphLibrary(stopped, root, archives); !errors.Is(err, context.Canceled) {
		t.Fatal("lost cancellation before materialization", err)
	}
	if err := userStageCodeGraphLibrary(ctx, root, archives); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(root, "runtime/codegraph")
	vendored := filepath.Join(library, "node_modules/@colbymchenry/codegraph-linux-x64/node")
	if _, err := os.Lstat(vendored); !os.IsNotExist(err) {
		t.Fatal("vendored Node was materialized", err)
	}
	before, err := userTreeStamp(root)
	if err != nil {
		t.Fatal("materialized files exceed existing custody bounds", err)
	}
	node, err := os.ReadFile("/opt/node/bin/node")
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(node)) != "41a74efb34cbde5c7632cdac0cf8bd1a14d0b8d73dc1e82755014d9a9ce70f5c" {
		t.Fatal("caller Node authentication failed", err)
	}
	cli := filepath.Join(library, "node_modules/@colbymchenry/codegraph-linux-x64/lib/dist/bin/codegraph.js")
	for _, args := range [][]string{{"--version"}, {"status", filepath.Join(root, "project"), "--json"}} {
		cmd := exec.CommandContext(ctx, "/opt/node/bin/node", append([]string{"--liftoff-only", cli}, args...)...)
		cmd.Dir, cmd.Env = filepath.Join(root, "project"), userEnvironment(root, filepath.Join(root, "prefix"), filepath.Join(root, "agent"))
		cmd.Env = append(cmd.Env, "CODEGRAPH_NO_DOWNLOAD=1", "CODEGRAPH_TELEMETRY=0", "DO_NOT_TRACK=1")
		var output, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &output, &stderr
		if err := userLaunchGroup(cmd, nil, func(err error) error { return err }); err != nil || stderr.Len() != 0 {
			t.Fatal("staged CLI/group failed", err, output.String(), stderr.String())
		}
		if args[0] == "--version" && strings.TrimSpace(output.String()) != "1.4.1" {
			t.Fatal("staged library version differs", output.String())
		}
		if args[0] == "status" {
			var status struct {
				Initialized          *bool
				Version, ProjectPath string
				LastIndexed          json.RawMessage
			}
			if json.Unmarshal(output.Bytes(), &status) != nil || status.Initialized == nil || *status.Initialized || status.Version != "1.4.1" || status.ProjectPath != cmd.Dir || string(status.LastIndexed) != "null" {
				t.Fatal("staged status did not remain uninitialized", output.String())
			}
		}
		t.Log("STAGED_CODEGRAPH_CLI_PASS", output.String())
	}
	after, err := userTreeStamp(root)
	if err != nil || before != after {
		t.Fatal("CLI changed the staged tree", err)
	}
	if err := userStageCodeGraphLibrary(ctx, root, archives); err == nil {
		t.Fatal("occupied library was replaced")
	}
	after, err = userTreeStamp(root)
	if err != nil || before != after {
		t.Fatal("occupied-stage refusal changed the tree", err)
	}
	t.Log("CODEGRAPH_NATIVE_LIBRARY_PASS retained_sources_segmented=true per_file_bound=32MiB vendored_node=false no_index=true")
}
