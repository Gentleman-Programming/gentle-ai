//go:build linux && amd64

package shellinstaller

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUserNativeServicePlanRefusal(t *testing.T) {
	for _, ctx := range []context.Context{nil, context.Background()} {
		if plan, err := newUserNativeServicePlan(ctx, nil, nil); err == nil || plan != nil {
			t.Fatal("unbounded service plan accepted")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if plan, err := newUserNativeServicePlan(ctx, nil, nil); err == nil || plan != nil {
		t.Fatal("missing operation accepted")
	}
	root := t.TempDir()
	operation := &userNativeSlice{name: "gentleshell0123456789abcdef.slice", group: userUnitEvidence{Path: root}}
	before, err := userTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := operation.stop(ctx); err == nil {
		t.Fatal("unowned directory stopped")
	}
	if after, err := userTreeStamp(root); err != nil || before != after {
		t.Fatal("rejected stop changed fixture", err)
	}
}

func TestUserNativeOperationStopPrivate(t *testing.T) {
	if os.Getenv("GENTLE_NATIVE_STOP_TEST") != "parent" {
		t.Skip("requires isolated private user manager")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	operation, err := newUserNativeSlice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := newUserNativeServicePlan(ctx, operation, nil)
	if err != nil || plan.cancel == nil {
		t.Fatal("owner cancellation unavailable", err)
	}
	args, err := plan.arguments("probe", false, "/probe/self", "/fixture", []string{"internal-native-coordinate"}, operation.env)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "\n")
	for _, want := range []string{"--slice=" + operation.name, "--property=CapabilityBoundingSet=", "--property=RuntimeMaxSec=300s"} {
		if !strings.Contains(joined, want+"\n") {
			t.Fatal("missing native coordinator property", want)
		}
	}
	if strings.Count(joined, "GOMAXPROCS=1\n") != 1 {
		t.Fatal("native coordinator lacks fixed Go pool control")
	}
	if strings.Contains(joined, "PrivateUsers=") || strings.Contains(joined, "BindReadOnlyPaths=") {
		t.Fatal("coordinator lost its full-map manager access")
	}
	reader := &userNativeSlice{name: operation.name, group: operation.group}
	readPlan, err := newUserNativeServicePlan(ctx, reader, nil)
	if err != nil || readPlan.cancel != nil {
		t.Fatal("read-only operation acquired cancellation", err)
	}
	commands := make([]*exec.Cmd, 0, 2)
	for _, suffix := range []string{"coordinator", "producer"} {
		command := exec.CommandContext(ctx, "/usr/bin/systemd-run", "--user", "--quiet", "--wait", "--pipe",
			"--unit=gentle-native-stop-"+suffix, "--slice="+operation.name,
			"--property=MemoryMax=3221225472", "--property=MemorySwapMax=0",
			"--property=CPUQuota=100%", "--property=CPUQuotaPeriodSec=100ms", "--property=TasksMax=64",
			"--property=NoNewPrivileges=yes", "--property=CapabilityBoundingSet=",
			"--property=RuntimeMaxSec=300s", "--property=TimeoutStopSec=2s", "/bin/sh", "-c",
			"printf ready > /fixture/ready-"+suffix+"; /usr/bin/sleep 30; printf completed > /fixture/completed-"+suffix)
		command.Env = operation.env
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, command)
	}
	for {
		coordinator, firstErr := os.ReadFile("/fixture/ready-coordinator")
		producer, secondErr := os.ReadFile("/fixture/ready-producer")
		if firstErr == nil && secondErr == nil && string(coordinator) == "ready" && string(producer) == "ready" {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("both siblings never became populated", ctx.Err())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := operation.settle(ctx); err == nil {
		t.Fatal("ordinary settlement accepted active siblings")
	}
	if err := reader.stop(ctx); err == nil {
		t.Fatal("read-only member stopped owner operation")
	}
	if err := plan.cancel(); err != nil {
		t.Fatal("owned cancellation failed", err)
	}
	for _, command := range commands {
		// systemd-run can report success when the service was killed by SIGTERM.
		t.Log("canceled service client result", command.Wait())
	}
	for _, suffix := range []string{"coordinator", "producer"} {
		if _, err := os.Lstat(filepath.Join("/fixture", "completed-"+suffix)); !os.IsNotExist(err) {
			t.Fatal("canceled sibling reached completion", suffix, err)
		}
	}
	if _, err := os.Lstat(operation.group.Path); !os.IsNotExist(err) {
		t.Fatal("canceled aggregate remains", err)
	}
	t.Log("both owned sibling services canceled and physically settled")
}
