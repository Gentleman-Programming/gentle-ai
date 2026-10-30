//go:build linux && amd64 && shell_native_runtime

package shellinstaller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// This qualifies held-response replay and system trust, not installation.
func TestUserNativePersistenceMirror(t *testing.T) {
	if testing.Short() {
		t.Skip("requires authentic source/Node/held PNPM in the bounded offline Guest")
	}
	if os.Getuid() != 1000 {
		t.Fatal("native PNPM qualification requires Guest UID1000")
	}
	if err := UserKernelCheck(); err != nil {
		t.Fatal(err)
	}
	setup, stopSetup := context.WithTimeout(context.Background(), 60*time.Second)
	defer stopSetup()
	workspace, err := os.MkdirTemp("/tmp", "native-pnpm-replay-")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := privateDirectory(workspace)
	if err != nil {
		t.Fatal(err)
	}
	// Retain any failed/inconclusive workspace, including observer failures.
	t.Log("owned replay workspace:", workspace)
	root := filepath.Join(workspace, "worker")
	if err := userPrepareInstallStage(setup, root, "separate", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("/native-source.zip")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := userMainDescriptor("73960fcf003b37e612a7b18960d6acf75f33a517", "feed2eb9961c6ab3b3a3d55ea314bfca881a5d901c44f97493231d72e0326453")
	if err != nil {
		t.Fatal(err)
	}
	if err := userStageMainArchive(setup, root, artifact, data); err != nil {
		t.Fatal(err)
	}
	if err := userStageMainSource(setup, root, artifact); err != nil {
		t.Fatal(err)
	}
	w, err := userNativeRuntimeFromSnapshot(setup, root, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(w.node), 0700); err != nil {
		t.Fatal(err)
	}
	cp := exec.CommandContext(setup, "/usr/bin/cp", "--no-dereference", "/opt/node/bin/node", w.node)
	if _, err := userOwnedRun(setup, cp, stopSetup); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(w.node, 0700); err != nil {
		t.Fatal(err)
	}
	tools := filepath.Join(root, "runtime", "native-bootstrap")
	if err := os.Mkdir(tools, 0700); err != nil {
		t.Fatal(err)
	}
	seed := exec.CommandContext(setup, w.node, "--liftoff-only", "--input-type=module", "-e", nativeBootstrapFixture, w.source, tools)
	seed.Dir = filepath.Join(root, "project")
	seed.Env = w.nativeEnvironment()
	// The existing fixture supplies held archive bytes to original bootstrap;
	// it never replaces hashing, the shim, probes, or the installation runner.
	if _, err := userOwnedRun(setup, seed, stopSetup); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "project", "preserved")
	if err := os.WriteFile(marker, []byte("private project marker"), 0600); err != nil {
		t.Fatal(err)
	}
	toolStamp, err := userTreeStamp(tools)
	if err != nil {
		t.Fatal(err)
	}
	projectStamp, err := userTreeStamp(filepath.Join(root, "project"))
	if err != nil {
		t.Fatal(err)
	}
	stopMirror := userPersistenceMirror(t, setup, workspace, w.nativeEnvironment())
	defer stopMirror()
	rowsData, err := os.ReadFile("/persistence-fixture/requests.json")
	if err != nil || len(rowsData) > 256*1024 {
		t.Fatal("fixture index unavailable", err)
	}
	var rows []struct {
		URL  string `json:"url"`
		Size int64  `json:"size"`
		SHA  string `json:"sha256"`
	}
	if err := json.Unmarshal(rowsData, &rows); err != nil || len(rows) != 7 {
		t.Fatal("fixed replay inventory differs", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	const probe = `const {createHash}=await import('node:crypto');
const response=await fetch(process.argv[1]);
const bytes=new Uint8Array(await response.arrayBuffer());
console.log(JSON.stringify({status:response.status,size:bytes.length,sha256:createHash('sha256').update(bytes).digest('hex')}));`
	read := func(url string) (int, int64, string, error) {
		cmd := exec.CommandContext(ctx, w.node, "--liftoff-only", "--input-type=module", "-e", probe, url)
		cmd.Dir, cmd.Env = filepath.Join(root, "project"), w.nativeEnvironment()
		out, err := userOwnedRun(ctx, cmd, cancel)
		var response struct {
			Status int    `json:"status"`
			Size   int64  `json:"size"`
			SHA    string `json:"sha256"`
		}
		if err != nil {
			return 0, 0, "", err
		}
		err = json.Unmarshal([]byte(out), &response)
		return response.Status, response.Size, response.SHA, err
	}
	for _, row := range rows {
		t.Run(row.URL, func(t *testing.T) {
			status, size, sha, err := read(row.URL)
			if err != nil || status != 200 || size != row.Size || sha != row.SHA {
				t.Errorf("authentic replay differs: status=%d size=%d sha=%s err=%v", status, size, sha, err)
			}
		})
	}
	t.Run("unknown-refused", func(t *testing.T) {
		status, _, _, err := read("https://nodejs.org/download/release/not-in-inventory")
		if err != nil || status != 404 {
			t.Errorf("unknown response not refused: %d %v", status, err)
		}
	})
	stopMirror()
	reportData, err := userPersistenceReport(workspace)
	var observed struct {
		Requests  []string `json:"requests"`
		Refused   []string `json:"refused"`
		TLSErrors []string `json:"tlsErrors"`
	}
	if err != nil || len(reportData) > 32*1024 {
		t.Error("bounded mirror report unavailable", err)
	} else if err := json.Unmarshal(reportData, &observed); err != nil || len(observed.Requests) != 7 || len(observed.Refused) != 1 || len(observed.TLSErrors) != 0 {
		t.Error("mirror request/refusal/TLS readback differs", err)
	}
	readback, stopReadback := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopReadback()
	if err := userCompareMainSource(readback, root, artifact); err != nil {
		t.Fatal(err)
	}
	if _, err := privateNativeFile(readback, w.node, 0700, privateNativeNodeSize, privateNativeNodeSHA); err != nil {
		t.Fatal(err)
	}
	if after, err := userTreeStamp(tools); err != nil || after != toolStamp {
		t.Error("copied bootstrap tools changed", err)
	}
	if after, err := userTreeStamp(filepath.Join(root, "project")); err != nil || after != projectStamp {
		t.Error("private project changed", err)
	}
	if t.Failed() {
		return
	}
	if err := privateCleanup(workspace, identity); err != nil {
		t.Fatal(err)
	}
	t.Log("NATIVE_PERSISTENCE_MIRROR_READBACK seven_original_responses=true unknown_refused=true system_trust=true normal_shutdown=true group_empty=true source_compared=true node_reauthenticated=true private_project_unchanged=true cleanup=true installed_acceptance=false")
}

func userPersistenceReport(workspace string) ([]byte, error) {
	file, err := os.Open(filepath.Join(workspace, "mirror-report.json"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 32*1024+1))
	if err == nil && len(data) > 32*1024 {
		err = fmt.Errorf("mirror report exceeds bound")
	}
	return data, err
}

func userPersistenceMirror(t *testing.T, ctx context.Context, workspace string, env []string) func() {
	return userNativeMirror(t, ctx, workspace, env, "shell-native-persistence-mirror.py", "/persistence-fixture")
}

func userNativeMirror(t *testing.T, ctx context.Context, workspace string, env []string, script, fixture string, secondary ...string) func() {
	t.Helper()
	ready, report := filepath.Join(workspace, "mirror-ready"), filepath.Join(workspace, "mirror-report.json")
	input, owner, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	serverCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(serverCtx, "/usr/bin/python3", "-I", filepath.Join("/source/e2e", script), fixture, ready, report)
	cmd.Args = append(cmd.Args, secondary...)
	cmd.Stdin, cmd.Dir = input, workspace
	cmd.Env = append(append([]string(nil), env...), "PYTHONDONTWRITEBYTECODE=1")
	done := make(chan error, 1)
	go func() { _, err := userOwnedRun(serverCtx, cmd, cancel); done <- err }()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		_, writeErr := fmt.Fprintln(owner, "stop")
		owner.Close()
		joined := false
		select {
		case err := <-done:
			joined = true
			if err != nil || writeErr != nil {
				t.Error("mirror normal shutdown failed", err, writeErr)
			}
		case <-time.After(3 * time.Second):
			cancel()
			t.Error("mirror settlement unproved")
			select {
			case <-done:
				joined = true
			case <-time.After(3 * time.Second):
				t.Error("mirror cancellation unproved")
			}
		}
		input.Close()
		cancel()
		if joined && cmd.Process != nil {
			if err := userNativeGroupAbsent(cmd.Process.Pid); err != nil {
				t.Error(err)
			}
		}
		if data, err := userPersistenceReport(workspace); err == nil {
			t.Log("mirror report:", string(data))
		} else {
			t.Error(err)
		}
	}
	t.Cleanup(stop)
	for {
		if data, err := os.ReadFile(ready); err == nil && string(data) == "ready" {
			return stop
		}
		select {
		case err := <-done:
			// Preserve the result so stop still joins the single owned invocation.
			done <- err
			t.Fatal("mirror startup failed:", err)
		case <-ctx.Done():
			t.Fatal("mirror startup observation expired")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
