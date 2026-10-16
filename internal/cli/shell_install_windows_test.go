//go:build windows

package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func TestWindowsShellInstallUnconfirmedSelectionHasNoEffects(t *testing.T) {
	if err := shellinstaller.UserKernelCheck(); err != nil {
		t.Skip("requires actual Windows 11 x64 normal-account acceptance; not Server/ARM qualification")
	}
	target := filepath.Join(t.TempDir(), "Gentle Shell")
	if _, err := shellinstaller.InspectUserInstall(shellinstaller.UserInstallRequest{Destination: target, Mode: "separate"}); err != nil {
		t.Fatalf("physical Windows selection fixture refused: %v", err)
	}
	err := RunShell([]string{"install", "--target", target, "--confirm", "not-confirmed"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--inspect") || !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("wrong confirmation did not name the safe continuation: %v", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("unconfirmed request created a target: %v", err)
	}
}

func TestWindowsShellInstallChannels(t *testing.T) {
	for _, channel := range []string{"stable", "main"} {
		req, _, err := parseShellInstall([]string{"--channel", channel}, io.Discard)
		if err != nil || req.Channel != channel || shellEntryValues(req)[6] != channel {
			t.Fatalf("channel did not reach worker: %+v, %v", req, err)
		}
	}
	for _, value := range []string{"nightly", ""} {
		req, _, err := parseShellInstall([]string{"--channel", value}, io.Discard)
		if err == nil || req != (shellinstaller.UserInstallRequest{}) {
			t.Fatal("invalid channel returned actionable request")
		}
	}
	req, _, err := parseShellInstall(nil, io.Discard)
	if err != nil || req.Channel != "stable" {
		t.Fatal("default channel is not stable")
	}
	m := shellInstallModel{field: 2, req: req}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if cmd != nil || next.(shellInstallModel).req.Channel != "main" || !strings.Contains(next.(shellInstallModel).View(), "Main resolves once after confirmation") {
		t.Fatal("TUI channel selection caused effects or hid snapshot resolution timing")
	}
}

func TestWindowsShellInstallFlagsAndNoEffectHelp(t *testing.T) {
	for _, args := range [][]string{{"--unknown"}, {"extra"}, {"--target"}, {"--confirm"}} {
		if _, _, err := parseShellInstall(args, io.Discard); err == nil {
			t.Fatalf("invalid flags admitted: %q", args)
		}
	}
	var output bytes.Buffer
	if err := RunShell([]string{"install", "--help"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"--inspect", "--confirm", "Windows 11 x64", "Separate only", "personal PATH"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing safe help: %s", expected)
		}
	}
}

func TestWindowsShellInstallDestinationPreservesTypedAndPastedSpaces(t *testing.T) {
	for _, pasted := range []bool{false, true} {
		name := "typed"
		if pasted {
			name = "pasted"
		}
		t.Run(name, func(t *testing.T) {
			want := "R:\\Gentle Lab Work\\Owned Shell"
			m := shellInstallModel{req: shellinstaller.UserInstallRequest{Mode: "separate"}}
			keys := []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune(want), Paste: true}}
			if !pasted {
				keys = nil
				for _, char := range want {
					key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}}
					if char == ' ' {
						key = tea.KeyMsg{Type: tea.KeySpace}
					}
					keys = append(keys, key)
				}
			}
			for _, key := range keys {
				next, cmd := m.Update(key)
				m = next.(shellInstallModel)
				if cmd != nil || m.review || m.busy {
					t.Fatal("editing started installation or review effects")
				}
			}
			if m.req.Destination != want {
				t.Fatalf("destination spaces changed: got %q, want %q", m.req.Destination, want)
			}
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
			next, cmd := next.(shellInstallModel).Update(tea.KeyMsg{Type: tea.KeySpace})
			got := next.(shellInstallModel)
			if cmd != nil || got.field != 1 || got.review || got.busy || got.req.Mode != "separate" || got.req.Destination != want {
				t.Fatal("space on the mode field changed the selection or enabled Shared")
			}
		})
	}
}

func TestWindowsShellInstallControlInputPreservesSelectionAndReview(t *testing.T) {
	for _, state := range []string{"editing", "review", "busy"} {
		t.Run(state, func(t *testing.T) {
			m := shellInstallModel{req: shellinstaller.UserInstallRequest{Destination: "R:\\Gentle Lab Work\\Owned Shell", Mode: "separate", Confirmation: "unchanged"}, review: state == "review", busy: state == "busy"}
			before := m
			for _, control := range []rune{0, '\r', '\n', '\t', '\x7f', '\u0085'} {
				next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{control}})
				m = next.(shellInstallModel)
				if cmd != nil || m.req != before.req || m.field != before.field || m.review != before.review || m.busy != before.busy || m.err != before.err {
					t.Fatal("console control event changed the physical selection or review state")
				}
			}
		})
	}
	for _, state := range []string{"editing", "review", "busy"} {
		for _, key := range []tea.KeyMsg{
			{Type: tea.KeyRunes, Runes: []rune("\\Owned\x00 Shell")},
			{Type: tea.KeyRunes, Runes: []rune("\\Owned\x00 Shell"), Paste: true},
			{Type: tea.KeyRunes, Runes: []rune{0}, Paste: true},
		} {
			m := shellInstallModel{req: shellinstaller.UserInstallRequest{Destination: "R:\\Gentle Lab Work", Mode: "separate", Confirmation: "unchanged"}, review: state == "review", busy: state == "busy"}
			before := m
			next, cmd := m.Update(key)
			got := next.(shellInstallModel)
			if cmd != nil || got.req != before.req || got.field != before.field || got.review != before.review || got.busy != before.busy || (got.err != nil) != !before.busy {
				t.Fatal("control text changed selection/review, was admitted, or started effects")
			}
		}
	}
}

func TestWindowsShellInstallReusesEditReviewAndSettledCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := shellInstallModel{ctx: ctx, cancel: cancel, stdout: io.Discard, req: shellinstaller.UserInstallRequest{Mode: "separate"}}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("C:\\owned\\shell")})
	m = next.(shellInstallModel)
	if cmd != nil || m.review || m.busy {
		t.Fatal("typing performed installation effects")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	next, _ = next.(shellInstallModel).Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(shellInstallModel)
	if m.req.Mode != "separate" || !strings.Contains(m.View(), "gentle-shell.cmd") || !strings.Contains(m.View(), "pi.cmd") {
		t.Fatal("Windows offered unsupported Shared or hid owned commands")
	}
	m.busy = true
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil || ctx.Err() == nil || !next.(shellInstallModel).busy {
		t.Fatal("cancel abandoned the installer worker")
	}
	next, cmd = next.(shellInstallModel).Update(shellInstallDone{context.Canceled})
	if cmd == nil || next.(shellInstallModel).busy || next.(shellInstallModel).err != context.Canceled {
		t.Fatal("cancellation did not await actual completion")
	}
}
