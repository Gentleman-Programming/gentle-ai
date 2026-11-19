//go:build linux

package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func TestShellLimitsShowSetReset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var out bytes.Buffer
	if err := RunShell([]string{"limits"}, &out); err != nil {
		t.Fatalf("show defaults: %v", err)
	}
	for _, want := range []string{"cpus     1", "memory   3G", "swap     0", "tasks    512", "(defaults)"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("show defaults output missing %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if err := RunShell([]string{"limits", "--cpus", "1", "--memory", "12G", "--swap", "8G", "--tasks", "2048"}, &out); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := shellinstaller.LoadUserUnitLimits(home)
	want := shellinstaller.UserUnitLimits{MemoryMaxBytes: 12 << 30, SwapMaxBytes: 8 << 30, CPUs: 1, TasksMax: 2048}
	if err != nil || got != want {
		t.Fatalf("saved %+v (%v), want %+v", got, err, want)
	}
	for _, want := range []string{"memory   12G", "swap     8G", "tasks    2048", "next Gentle Shell launch"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("set output missing %q:\n%s", want, out.String())
		}
	}

	// Flags left out keep their saved value.
	out.Reset()
	if err := RunShell([]string{"limits", "--tasks", "4096"}, &out); err != nil {
		t.Fatalf("partial set: %v", err)
	}
	got, _ = shellinstaller.LoadUserUnitLimits(home)
	if got.MemoryMaxBytes != 12<<30 || got.TasksMax != 4096 {
		t.Fatalf("partial set lost saved values: %+v", got)
	}

	out.Reset()
	if err := RunShell([]string{"limits", "--reset"}, &out); err != nil {
		t.Fatalf("reset: %v", err)
	}
	got, _ = shellinstaller.LoadUserUnitLimits(home)
	if got != shellinstaller.DefaultUserUnitLimits() || !strings.Contains(out.String(), "(defaults)") {
		t.Fatalf("reset left %+v:\n%s", got, out.String())
	}
}

func TestShellLimitsRefusesInvalidValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, args := range [][]string{
		{"limits", "--cpus", "0"},
		{"limits", "--cpus", "100000"},
		{"limits", "--memory", "512M"},
		{"limits", "--memory", "lots"},
		{"limits", "--tasks", "64"},
		{"limits", "--reset", "--cpus", "2"},
		{"limits", "extra"},
	} {
		if err := RunShell(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("%v accepted", args)
		}
	}
	if got, _ := shellinstaller.LoadUserUnitLimits(home); got != shellinstaller.DefaultUserUnitLimits() {
		t.Fatalf("refused input changed the setting: %+v", got)
	}
	if runtime.NumCPU() < 100000 {
		var out bytes.Buffer
		err := RunShell([]string{"limits", "--cpus", "100000"}, &out)
		if err == nil || !strings.Contains(err.Error(), "this machine has") {
			t.Fatalf("cpus above host count must name the host count, got %v", err)
		}
	}
}

func TestShellHelpMentionsLimits(t *testing.T) {
	var out bytes.Buffer
	if err := RunShell([]string{"--help"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "gentle-ai shell limits") {
		t.Fatalf("shell help does not document limits:\n%s", out.String())
	}
}
