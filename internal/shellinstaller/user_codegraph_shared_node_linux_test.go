//go:build linux && shell_codegraph_shared_node

package shellinstaller

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestUserCodeGraphFrozenSharedNode(t *testing.T) {
	if testing.Short() {
		t.Skip("authenticated archive CLI integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/python3", "../../e2e/shell-codegraph-shared-node.py")
	cmd.Env = []string{"HOME=" + t.TempDir(), "TMPDIR=/tmp", "PATH=/usr/bin:/bin", "PYTHONDONTWRITEBYTECODE=1"}
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := userLaunchGroup(cmd, nil, func(err error) error { return err }); err != nil {
		t.Fatalf("authenticated CLI/group probe failed: %v\n%s", err, output.String())
	}
	for _, marker := range []string{"FROZEN_CALLER_NODE_SHA_VERSION_PASS 24.18.0", "VENDORED_NODE_EXCLUDED bytes=123438592", "FROZEN_CODEGRAPH_SHELL_NODE_PASS no_index=true tree_unchanged=true vendored_node=false materialized_file_bound=32MiB"} {
		if !strings.Contains(output.String(), marker) {
			t.Fatal("missing CLI proof marker", marker, output.String())
		}
	}
	t.Log(output.String())
}
