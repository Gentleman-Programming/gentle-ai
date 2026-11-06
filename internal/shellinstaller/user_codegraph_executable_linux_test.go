//go:build linux && shell_codegraph_library

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUserCodeGraphPrivateCommandAndPackagedSDK(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	archives := make([][]byte, 2)
	for i, name := range []string{"/frozen-codegraph-main.tgz", "/frozen-codegraph-linux.tgz"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		archives[i] = data
	}
	root := installStagePath(t)
	if err := userPrepareInstallStage(ctx, root, "separate", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userStageCodeGraphLibrary(ctx, root, archives); err != nil {
		t.Fatal(err)
	}
	if err := userStageCodeGraphExecutable(ctx, root, root); err != nil {
		t.Fatal(err)
	}
	if err := userCodeGraphReadback(ctx, root); err != nil {
		t.Fatal(err)
	}
	// Authentic fixed Shell Node, not personal/CodeGraph bundled Node or a
	// stock-helper substitute. Native bootstrap custody is its existing ABI;
	// all CodeGraph source/library/command files keep their 32MiB bounds.
	node, err := os.ReadFile("/opt/node/bin/node")
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(node)) != "41a74efb34cbde5c7632cdac0cf8bd1a14d0b8d73dc1e82755014d9a9ce70f5c" {
		t.Fatal("caller Shell Node authentication differs", err)
	}
	binary := filepath.Join(root, "runtime/node/bin/node")
	if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, node, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := privateNativeFile(ctx, binary, 0700, int64(len(node)), fmt.Sprintf("%x", sha256.Sum256(node))); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(root, "runtime/codegraph")
	before, err := userTreeStamp(library)
	if err != nil {
		t.Fatal(err)
	}
	manifest := userManifest{Mode: "separate", CodeGraph: true, Prefix: filepath.Join(root, "prefix"), Agent: filepath.Join(root, "agent")}
	planned := userLaunchCommand(ctx, root, manifest, "/not-executed/cli.js", nil)
	cmd := exec.CommandContext(ctx, binary, "--liftoff-only", "/source/e2e/shell-codegraph-sdk-discovery.mjs", root)
	cmd.Dir, cmd.Env = filepath.Join(root, "project"), planned.Env
	var output, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &stderr
	if err := userLaunchGroup(cmd, nil, func(err error) error { return err }); err != nil {
		t.Fatal("actual private command/SDK fixture failed", err, output.String(), stderr.String())
	}
	if !strings.Contains(output.String(), "CODEGRAPH_PACKAGED_SDK_DISCOVERY_PASS") {
		t.Fatal("missing actual SDK readback", output.String(), stderr.String())
	}
	after, err := userTreeStamp(library)
	if err != nil || before != after {
		t.Fatal("discovery changed library", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "project/.codegraph")); !os.IsNotExist(err) {
		t.Fatal("discovery indexed a project", err)
	}
	if err := userCodeGraphReadback(ctx, root); err != nil {
		t.Fatal("settled retained integrity differs", err)
	}
	t.Log(output.String())
	if stderr.Len() != 0 {
		t.Log("Guest stderr (not live TLS qualification):", stderr.String())
	}
}
