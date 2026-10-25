//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestUserNativeSliceCommands(t *testing.T) {
	name := "gentleshell0123456789abcdef.slice"
	args := userNativeSliceCreateArgs(name)
	want := []string{"--user", "call", "org.freedesktop.systemd1", "/org/freedesktop/systemd1", "org.freedesktop.systemd1.Manager", "StartTransientUnit", "ssa(sv)a(sa(sv))", name, "fail", "5", "MemoryMax", "t", "3221225472", "MemorySwapMax", "t", "0", "CPUQuotaPerSecUSec", "t", "1000000", "CPUQuotaPeriodUSec", "t", "100000", "TasksMax", "t", "64", "0"}
	if !reflect.DeepEqual(args, want) {
		t.Fatal("aggregate creation changed", args)
	}
	for _, invalid := range []string{"app.slice", "gentleshell-0123456789abcdef.slice", name + "/child", "gentleshell0123456789abcdeg.slice"} {
		if userNativeSliceName(invalid) {
			t.Fatal("unsafe slice name accepted", invalid)
		}
	}
	for _, ctx := range []context.Context{nil, func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }()} {
		if slice, err := newUserNativeSlice(ctx); err == nil || slice != nil {
			t.Fatal("invalid context reached manager")
		}
	}
}

func TestUserNativeSlicePhysicalRefusal(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"memory.max", "memory.swap.max", "cpu.max", "pids.max"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("3221225472"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	slice := &userNativeSlice{name: "gentleshell0123456789abcdef.slice", group: userUnitEvidence{Path: root}}
	before, err := userTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := slice.read(); err == nil {
		t.Fatal("ordinary directory impersonated cgroup2")
	}
	if arg, err := slice.serviceArg(context.Background()); err == nil || arg != "" {
		t.Fatal("unbound service argument")
	}
	if err := slice.settle(context.Background()); err == nil {
		t.Fatal("unbound slice stopped")
	}
	if after, err := userTreeStamp(root); err != nil || before != after {
		t.Fatal("refusal changed fixture", err)
	}
}

// The private manager harness sets this switch inside its isolated root only.
// It never selects or discovers a personal manager.
func TestUserNativeSlicePrivate(t *testing.T) {
	mode := os.Getenv("GENTLE_PRIVATE_SLICE_TEST")
	if mode == "" {
		t.Skip("requires isolated private user manager")
	}
	ctx := context.Background()
	if mode == "member" {
		data, err := os.ReadFile("/fixture/slice-evidence.json")
		if err != nil {
			t.Fatal(err)
		}
		var group userUnitEvidence
		if err := json.Unmarshal(data, &group); err != nil {
			t.Fatal(err)
		}
		slice := &userNativeSlice{name: filepath.Base(group.Path), group: group}
		if err := slice.member(ctx); err != nil {
			t.Fatal(err)
		}
		time.Sleep(200 * time.Millisecond)
		return
	}
	slice, err := newUserNativeSlice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	arg, err := slice.serviceArg(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := slice.member(ctx); err == nil {
		t.Fatal("outside sibling accepted")
	}
	data, err := json.Marshal(slice.group)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/fixture/slice-evidence.json", data, 0600); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := []string{"--user", "--quiet", "--wait", "--pipe", arg, "--unit=gentle-private-slice-member", "--property=MemoryMax=3221225472", "--property=MemorySwapMax=0", "--property=CPUQuota=100%", "--property=CPUQuotaPeriodSec=100ms", "--property=TasksMax=64", "--property=NoNewPrivileges=yes", "--property=CapabilityBoundingSet=", "--property=RuntimeMaxSec=300s", "--property=TimeoutStopSec=2s", "/usr/bin/env", "GENTLE_PRIVATE_SLICE_TEST=member", self, "-test.run=^TestUserNativeSlicePrivate$", "-test.v"}
	env, err := userBusEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/systemd-run", command...)
	cmd.Env = env
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 100; attempt++ {
		pids, _ := os.ReadFile(filepath.Join(slice.group.Path, "pids.current"))
		if strings.TrimSpace(string(pids)) != "0" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	pids, err := os.ReadFile(filepath.Join(slice.group.Path, "pids.current"))
	if err != nil || strings.TrimSpace(string(pids)) == "0" {
		t.Fatal("child was not observed in aggregate", err)
	}
	if err := slice.settle(ctx); err == nil {
		t.Fatal("populated aggregate stopped")
	}
	if err := cmd.Wait(); err != nil || !strings.Contains(output.String(), "PASS") {
		t.Fatal("descendant failed", err, output.String())
	}
	if err := slice.settle(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(slice.group.Path); !os.IsNotExist(err) {
		t.Fatal("settled slice remains", err)
	}
	t.Log("created bounded slice, verified child, settled exact identity", slice.name)
}
