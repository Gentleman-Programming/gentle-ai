//go:build linux || darwin

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func userLaunchTestPID(t *testing.T, path string) int {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if data, err := os.ReadFile(path); err == nil && strings.HasSuffix(string(data), "\n") {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil || pid <= 1 {
				t.Fatalf("invalid pid in %s: %q", path, data)
			}
			return pid
		}
	}
	t.Fatalf("launched group never wrote %s", path)
	return 0
}

func TestUnixLaunchGroupReapsBeforeReadback(t *testing.T) {
	dir := t.TempDir()
	// userLaunchGroup sets Cancel, which requires a context-bound command.
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", `(sleep 1; : > "$1/survived") & exit 7`, "sh", dir)
	stdin, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	defer writer.Close()
	cmd.Stdin = stdin
	sentinel := errors.New("readback refused")
	readbackRan := false
	err = userLaunchGroup(cmd, stdin, func(waitErr error) error {
		readbackRan = true
		var exit *exec.ExitError
		if !errors.As(waitErr, &exit) || exit.ExitCode() != 7 {
			t.Errorf("readback wait error = %v, want exit 7", waitErr)
		}
		if probe := syscall.Kill(-cmd.Process.Pid, 0); !errors.Is(probe, syscall.ESRCH) {
			t.Errorf("readback ran before the group was reaped: %v", probe)
		}
		return errors.Join(waitErr, sentinel)
	})
	if !readbackRan || !errors.Is(err, sentinel) {
		t.Fatalf("launch = %v, readback ran %v; want the readback result", err, readbackRan)
	}
	<-time.After(1500 * time.Millisecond)
	if _, err := os.Lstat(filepath.Join(dir, "survived")); !os.IsNotExist(err) {
		t.Fatalf("backgrounded descendant ran after launch returned: %v", err)
	}
}

func TestUnixLaunchGroupCancelKillsGroup(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `(sleep 1; : > "$1/survived") & echo $$ > "$1/pid"; sleep 30`, "sh", dir)
	done := make(chan error, 1)
	go func() { done <- userLaunchGroup(cmd, nil, func(err error) error { return err }) }()
	pid := userLaunchTestPID(t, filepath.Join(dir, "pid"))
	cancel()
	select {
	case err := <-done:
		// The kill is the expected outcome; only a failed reap is uncertain.
		var private *PrivateRuntimeError
		var exit *exec.ExitError
		if errors.As(err, &private) || !errors.As(err, &exit) {
			t.Fatalf("canceled launch = %v, want the killed child status after a clean reap", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("launch ignored cancellation")
	}
	if err := syscall.Kill(-pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("group after cancel = %v, want ESRCH", err)
	}
	<-time.After(1500 * time.Millisecond)
	if _, err := os.Lstat(filepath.Join(dir, "survived")); !os.IsNotExist(err) {
		t.Fatalf("backgrounded descendant ran after cancel: %v", err)
	}
}

// TestUnixLaunchGroupTerminalFixture runs as a session leader whose
// controlling terminal is a fresh pty, then launches through userLaunchGroup.
func TestUnixLaunchGroupTerminalFixture(t *testing.T) {
	dir := os.Getenv("GENTLE_SHELL_LAUNCH_FIXTURE")
	if dir == "" {
		return
	}
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", `ps -o tpgid= -o pgid= -p $$ > "$1/child"`, "sh", dir)
	cmd.Stdin = os.Stdin
	err := userLaunchGroup(cmd, os.Stdin, func(err error) error { return err })
	group, groupErr := unix.IoctlGetInt(0, unix.TIOCGPGRP)
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	result := fmt.Sprintf("launch=%v restored=%v group=%d self=%d pid=%d\n", err, groupErr == nil && group == syscall.Getpgrp(), group, syscall.Getpgrp(), pid)
	if writeErr := os.WriteFile(filepath.Join(dir, "result"), []byte(result), 0600); writeErr != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestUnixLaunchGroupHandsOffAndRestoresForeground(t *testing.T) {
	terminal := openTestTerminal(t)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	fixture := exec.Command(self, "-test.run=^TestUnixLaunchGroupTerminalFixture$")
	fixture.Env = []string{"GENTLE_SHELL_LAUNCH_FIXTURE=" + dir, "PATH=/usr/bin:/bin"}
	fixture.Stdin = terminal
	fixture.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	output, err := fixture.CombinedOutput()
	if err != nil {
		t.Fatalf("terminal fixture = %v: %s", err, output)
	}
	result, err := os.ReadFile(filepath.Join(dir, "result"))
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(result))
	if len(fields) != 5 || fields[0] != "launch=<nil>" || fields[1] != "restored=true" {
		t.Fatalf("fixture result = %q", result)
	}
	child, err := os.ReadFile(filepath.Join(dir, "child"))
	if err != nil {
		t.Fatal(err)
	}
	ids := strings.Fields(string(child))
	pid := strings.TrimPrefix(fields[4], "pid=")
	// The launched command led its own group and owned the terminal foreground.
	if len(ids) != 2 || ids[0] != pid || ids[1] != pid || "self="+pid == fields[3] {
		t.Fatalf("child tpgid/pgid = %q, launched pid %s; want both equal to the launched pid", child, pid)
	}
}

// userLaunchRepairFixture builds a published Separate installation with the
// residue a Pi self-update leaves: the installed pi-coding-agent version
// differs from the recorded graph seal and an orphan global package is planted
// (issue #5812).
func userLaunchRepairFixture(t *testing.T, recorded string) (string, userManifest) {
	t.Helper()
	root := t.TempDir()
	manifest := userManifest{Schema: userSchema, Destination: root, Prefix: filepath.Join(root, "prefix"), Agent: filepath.Join(root, "agent"), Mode: "separate"}
	writeInstallFixture(t, root, manifest)
	write := func(rel, body string, mode os.FileMode) {
		t.Helper()
		name := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("prefix/lib/node_modules/@earendil-works/pi-coding-agent/package.json",
		`{"name":"@earendil-works/pi-coding-agent","version":"9.9.9"}`, 0600)
	write("prefix/lib/node_modules/orphan-global/package.json", `{"name":"orphan-global","version":"1.0.0"}`, 0600)
	write("state/global-graph.json", recorded, 0600)
	return root, manifest
}

// userLaunchRepairSeams substitutes the launch verifier and the canonical
// re-provision so the repair sequence can be observed without Node or npm.
func userLaunchRepairSeams(t *testing.T, reprovision, verify func(context.Context, string, userManifest) error) {
	t.Helper()
	previousReprovision, previousVerify := userLaunchReprovision, userLaunchVerify
	userLaunchReprovision, userLaunchVerify = reprovision, verify
	t.Cleanup(func() { userLaunchReprovision, userLaunchVerify = previousReprovision, previousVerify })
}

func TestUserLaunchPrepareAcceptsAndReauthenticatesDriftedGraph(t *testing.T) {
	const installed = "9.9.9"
	const recorded = "4.3.0"
	root, manifest := userLaunchRepairFixture(t,
		`[{"directory":"lib/node_modules/@earendil-works/pi-coding-agent","name":"@earendil-works/pi-coding-agent","version":"`+recorded+`","integrity":"sha512-old"}]`)
	reprovisionCalls, verifyCalls := 0, 0
	userLaunchRepairSeams(t,
		func(ctx context.Context, root string, manifest userManifest) error {
			reprovisionCalls++
			// The provisioner's generated-route re-acquisition re-resolves npm
			// latest and re-issues the seal for the newly resolved graph; the
			// clean npm ci closure drops the orphaned global package.
			seal := `[{"directory":"lib/node_modules/@earendil-works/pi-coding-agent","name":"@earendil-works/pi-coding-agent","version":"` + installed + `","integrity":"sha512-reacquired"}]`
			if err := os.WriteFile(filepath.Join(root, "state/global-graph.json"), []byte(seal), 0600); err != nil {
				return err
			}
			return os.RemoveAll(filepath.Join(manifest.Prefix, "lib/node_modules/orphan-global"))
		},
		func(ctx context.Context, root string, manifest userManifest) error {
			verifyCalls++
			data, err := os.ReadFile(filepath.Join(root, "state/global-graph.json"))
			if err != nil {
				return err
			}
			var observed []struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}
			if err := json.Unmarshal(data, &observed); err != nil {
				return err
			}
			for _, entry := range observed {
				if entry.Name == "@earendil-works/pi-coding-agent" && entry.Version != installed {
					return fmt.Errorf("seal not re-authenticated to %s: %s", installed, data)
				}
			}
			if _, err := os.Lstat(filepath.Join(manifest.Prefix, "lib/node_modules/orphan-global")); !os.IsNotExist(err) {
				return fmt.Errorf("orphan survived re-provision: %v", err)
			}
			return nil
		})

	if err := userLaunchPrepare(context.Background(), root, manifest); err != nil {
		t.Fatalf("launch prepare = %v", err)
	}
	if reprovisionCalls != 1 || verifyCalls != 1 {
		t.Fatalf("repair sequence = reprovision %d verify %d, want 1/1", reprovisionCalls, verifyCalls)
	}
	// The seal is re-authenticated to the newly resolved graph: it carries the
	// accepted self-updated version and never reverts to the recorded one.
	data, err := os.ReadFile(filepath.Join(root, "state/global-graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), installed) || !strings.Contains(string(data), "sha512-reacquired") {
		t.Fatalf("seal was not re-authenticated to the new graph: %s", data)
	}
	if strings.Contains(string(data), recorded) || strings.Contains(string(data), "sha512-old") {
		t.Fatalf("seal reverted to the recorded graph instead of accepting the self-update: %s", data)
	}
	if _, err := os.Lstat(filepath.Join(manifest.Prefix, "lib/node_modules/orphan-global")); !os.IsNotExist(err) {
		t.Fatalf("orphan survived re-provision: %v", err)
	}
}

func TestUserLaunchPrepareNoDriftKeepsVerifyOffline(t *testing.T) {
	root, manifest := userLaunchRepairFixture(t,
		`[{"directory":"lib/node_modules/@earendil-works/pi-coding-agent","name":"@earendil-works/pi-coding-agent","version":"9.9.9","integrity":"sha512-aaa"}]`)
	reprovisionCalls, verifyCalls := 0, 0
	userLaunchRepairSeams(t,
		func(ctx context.Context, root string, manifest userManifest) error {
			reprovisionCalls++
			t.Fatal("no-drift launch re-provisioned the graph")
			return nil
		},
		func(ctx context.Context, root string, manifest userManifest) error {
			verifyCalls++
			return nil
		})

	if err := userLaunchPrepare(context.Background(), root, manifest); err != nil {
		t.Fatalf("launch prepare = %v", err)
	}
	if reprovisionCalls != 0 || verifyCalls != 1 {
		t.Fatalf("no-drift sequence = reprovision %d verify %d, want 0/1", reprovisionCalls, verifyCalls)
	}
}

func TestUserLaunchPrepareReprovisionFailureRefuses(t *testing.T) {
	root, manifest := userLaunchRepairFixture(t,
		`[{"directory":"lib/node_modules/@earendil-works/pi-coding-agent","name":"@earendil-works/pi-coding-agent","version":"4.3.0","integrity":"sha512-aaa"}]`)
	verifyCalls := 0
	cause := errors.New("re-acquisition refused")
	userLaunchRepairSeams(t,
		func(ctx context.Context, root string, manifest userManifest) error { return cause },
		func(ctx context.Context, root string, manifest userManifest) error {
			verifyCalls++
			return nil
		})

	err := userLaunchPrepare(context.Background(), root, manifest)
	if err == nil || !errors.Is(err, cause) {
		t.Fatalf("launch prepare = %v, want the re-acquisition cause", err)
	}
	if verifyCalls != 0 {
		t.Fatalf("verify ran after a failed re-provision: %d", verifyCalls)
	}
	if !strings.Contains(err.Error(), "gentle-ai shell install --help") {
		t.Fatalf("repair refusal lost the manual continuation: %v", err)
	}
}

func TestUserLaunchGlobalDriftDetection(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		installed               string
		seal                    string
		withInstalled, withSeal bool
		want                    bool
	}{
		{"drifted self-update", "9.9.9", `[{"directory":"lib/node_modules/@earendil-works/pi-coding-agent","name":"@earendil-works/pi-coding-agent","version":"4.3.0"}]`, true, true, true},
		{"matching graph", "4.3.0", `[{"directory":"lib/node_modules/@earendil-works/pi-coding-agent","name":"@earendil-works/pi-coding-agent","version":"4.3.0"}]`, true, true, false},
		{"missing installed package", "", "", false, true, false},
		{"missing graph seal", "9.9.9", "", true, false, false},
		{"seal without pi entry", "9.9.9", `[{"directory":"lib/node_modules/gentle-pi","name":"gentle-pi","version":"4.3.0"}]`, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			write := func(rel, body string, mode os.FileMode) {
				t.Helper()
				name := filepath.Join(root, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(name, []byte(body), mode); err != nil {
					t.Fatal(err)
				}
			}
			if tc.withInstalled {
				write("prefix/lib/node_modules/@earendil-works/pi-coding-agent/package.json",
					`{"name":"@earendil-works/pi-coding-agent","version":"`+tc.installed+`"}`, 0600)
			}
			if tc.withSeal {
				write("state/global-graph.json", tc.seal, 0600)
			}
			if got := userLaunchGlobalDrift(context.Background(), root, filepath.Join(root, "prefix")); got != tc.want {
				t.Fatalf("drift = %v, want %v", got, tc.want)
			}
		})
	}
}
