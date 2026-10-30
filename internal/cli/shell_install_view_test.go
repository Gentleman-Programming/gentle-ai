//go:build !windows

package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func TestShellInstallReviewFitsAndScrolls(t *testing.T) {
	for _, width := range []int{80, 120, 140} {
		for _, height := range []int{8, 24} {
			t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
				_, cancel := context.WithCancel(context.Background())
				defer cancel()
				req := shellinstaller.UserInstallRequest{Mode: "shared", Confirmation: strings.Repeat("f", 64),
					Destination:  "/owned/" + strings.Repeat("destination", 30),
					SharedPrefix: "/owned/" + strings.Repeat("prefix", 30), SharedAgent: "/owned/agent"}
				m := shellInstallModel{cancel: cancel, req: req, review: true}
				next, cmd := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
				m = next.(shellInstallModel)
				if cmd != nil {
					t.Fatal("resizing performed effects")
				}
				var collected strings.Builder
				for page := 0; page < 100; page++ {
					view := m.View()
					lines := strings.Split(view, "\n")
					if len(lines) > height {
						t.Fatalf("view has %d rows; terminal has %d", len(lines), height)
					}
					for _, line := range lines {
						if ansi.StringWidth(line) > width {
							t.Fatalf("visible line exceeds %d cells: %q", width, line)
						}
					}
					collected.WriteString(strings.Join(lines[:len(lines)-1], ""))
					next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
					m = next.(shellInstallModel)
					if cmd != nil || !m.review || m.confirmed || m.req != req {
						t.Fatal("scrolling edited or confirmed the physical selection")
					}
					if m.View() == view {
						break
					}
					if page == 99 {
						t.Fatal("scrolling failed to reach the end")
					}
				}
				for _, value := range []string{req.Destination, req.SharedPrefix, req.SharedAgent, req.Confirmation} {
					if !strings.Contains(collected.String(), value) {
						t.Fatalf("scrolling lost disclosure bytes: %q", value)
					}
				}
				next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyHome})
				m = next.(shellInstallModel)
				if cmd != nil || !strings.Contains(m.View(), shellInstallTitle) {
					t.Fatal("Home did not return to the beginning")
				}
			})
		}
	}
}

func TestShellInstallReviewResizeAndUnicode(t *testing.T) {
	m := shellInstallModel{review: true, req: shellinstaller.UserInstallRequest{
		Destination: "/owned/" + strings.Repeat("界e\u0301", 100), Confirmation: "TOKEN_AT_END"}}
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 140, Height: 8}, {Width: 80, Height: 24}} {
		next, cmd := m.Update(size)
		m = next.(shellInstallModel)
		if cmd != nil {
			t.Fatal("resize performed effects")
		}
		next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
		m = next.(shellInstallModel)
		if cmd != nil || m.confirmed || !strings.Contains(m.View(), "TOKEN_AT_END") {
			t.Fatal("resized review lost the final confirmation")
		}
		for _, line := range strings.Split(m.View(), "\n") {
			if ansi.StringWidth(line) > size.Width {
				t.Fatal("wide or combining characters overflowed the physical width")
			}
		}
	}
}

func TestShellInstallReviewNavigationPreservesCancelAndConfirm(t *testing.T) {
	for _, action := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC, tea.KeyRunes} {
		ctx, cancel := context.WithCancel(context.Background())
		m := shellInstallModel{cancel: cancel, review: true, req: shellinstaller.UserInstallRequest{
			Destination: "/owned/shell", Mode: "separate", Confirmation: "unchanged"}}
		for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: 80, Height: 8},
			tea.KeyMsg{Type: tea.KeyPgDown}, tea.KeyMsg{Type: tea.KeyPgUp}, tea.KeyMsg{Type: tea.KeyEnd}} {
			next, cmd := m.Update(msg)
			m = next.(shellInstallModel)
			if cmd != nil || m.confirmed {
				t.Fatal("navigation ran or confirmed an installation")
			}
		}
		req := m.req
		next, cmd := m.Update(tea.KeyMsg{Type: action, Runes: []rune("y")})
		m = next.(shellInstallModel)
		if cmd == nil || m.req != req {
			t.Fatal("terminal handoff changed selection or did not quit")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("confirmation/cancellation executed before releasing the TUI")
		}
		if m.confirmed != (action == tea.KeyRunes) || (action != tea.KeyRunes && ctx.Err() == nil) {
			t.Fatal("scrolling changed confirmation or cancellation semantics")
		}
		cancel()
	}
}

func TestShellInstallReviewSharedPreviewReachable(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("physical shared selection is Linux amd64 only")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	req := shellinstaller.UserInstallRequest{Mode: "shared",
		Destination:  filepath.Join(parent, strings.Repeat("target-", 12)),
		SharedPrefix: filepath.Join(parent, strings.Repeat("prefix-", 12)),
		SharedAgent:  filepath.Join(parent, strings.Repeat("agent-", 12))}
	bundle := filepath.Join(req.SharedPrefix, "lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js")
	for _, dir := range []string{filepath.Dir(bundle), req.SharedAgent} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, data := range map[string]string{bundle: "fixture, never executed",
		filepath.Join(req.SharedAgent, "settings.json"): `{"packages":["npm:example-package",{"source":"/owned/example","extensions":[]}],"theme":"preserved"}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := shellInstallModel{cancel: cancel, req: req}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(shellInstallModel)
	if cmd != nil || m.err != nil || !m.review || m.confirmed {
		t.Fatalf("review did not inspect without effects: %+v", m)
	}
	preview, err := shellinstaller.PreviewUserInstall(req, m.req.Confirmation)
	if err != nil || preview == "" || m.preview != preview {
		t.Fatalf("review changed the actual shared preview: %v", err)
	}
	for _, disclosure := range []string{"settings.json", "packages before:", "packages after:", "npmCommand after:",
		"runtime/node/bin/node", "npm-cli.js", "--prefix", "preimages can overwrite later changes", "not uninstall or target deletion"} {
		if !strings.Contains(preview, disclosure) {
			t.Fatalf("fixture lacks disclosure %q", disclosure)
		}
	}
	selection := m.req
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 8}, {Width: 120, Height: 24}, {Width: 20, Height: 1}} {
		t.Run(fmt.Sprintf("%dx%d", size.Width, size.Height), func(t *testing.T) {
			next, _ := m.Update(size)
			page := next.(shellInstallModel)
			var collected strings.Builder
			seen := 0
			for {
				lines := strings.Split(page.View(), "\n")
				if len(lines) > size.Height {
					t.Fatal("preview exceeds terminal height")
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > size.Width {
						t.Fatal("preview exceeds terminal cell width")
					}
				}
				if size.Height > 1 {
					lines = lines[:len(lines)-1] // Exclude only the navigation footer.
				}
				collected.WriteString(strings.Join(lines[max(0, seen-page.scroll):], ""))
				seen = page.scroll + len(lines)
				next, cmd := page.Update(tea.KeyMsg{Type: tea.KeyPgDown})
				advanced := next.(shellInstallModel)
				if cmd != nil || advanced.req != selection || advanced.preview != preview || !advanced.review || advanced.confirmed {
					t.Fatal("paging changed disclosure or confirmation authority")
				}
				if advanced.scroll == page.scroll {
					break
				}
				page = advanced
			}
			if collected.String() != strings.ReplaceAll(m.content(), "\n", "") {
				t.Fatal("paging did not expose every review byte, including the complete shared preview")
			}
		})
	}
	if _, err := os.Lstat(req.Destination); !os.IsNotExist(err) {
		t.Fatalf("review or scrolling created a destination: %v", err)
	}
}

func TestShellInstallReviewNavigationClampsAndEdits(t *testing.T) {
	m := shellInstallModel{review: true, req: shellinstaller.UserInstallRequest{
		Destination: "/owned/" + strings.Repeat("long", 100), Confirmation: "unchanged"}}
	selection := m.req
	for _, step := range []struct {
		msg  tea.Msg
		zero bool
	}{
		{tea.WindowSizeMsg{Width: 80, Height: 8}, true},
		{tea.KeyMsg{Type: tea.KeyPgUp}, true},
		{tea.KeyMsg{Type: tea.KeyEnd}, false},
		{tea.KeyMsg{Type: tea.KeyPgDown}, false},
		{tea.WindowSizeMsg{Width: 140, Height: 24}, true},
		{tea.WindowSizeMsg{Width: 80, Height: 8}, true},
		{tea.KeyMsg{Type: tea.KeyEnd}, false},
		{tea.KeyMsg{Type: tea.KeyPgUp}, false},
		{tea.KeyMsg{Type: tea.KeyHome}, true},
	} {
		next, cmd := m.Update(step.msg)
		m = next.(shellInstallModel)
		if cmd != nil || m.req != selection || !m.review || m.confirmed || m.scroll < 0 || m.scroll > m.lastReviewOffset() {
			t.Fatal("navigation or resize changed selection or escaped the review")
		}
		if step.zero && m.scroll != 0 {
			t.Fatal("navigation or resize did not clamp to the beginning")
		}
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	next, cmd := next.(shellInstallModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(shellInstallModel)
	if cmd != nil || m.req != selection || m.review || m.confirmed || m.scroll != 0 {
		t.Fatal("non-scroll key did not return to editing without changing selection")
	}
}

func TestShellInstallReviewPhysicalBounds(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := shellInstallModel{cancel: cancel, review: true, req: shellinstaller.UserInstallRequest{
		Mode: "shared", Destination: "/owned/" + strings.Repeat("destination", 30), Confirmation: "reviewed"}}
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 8})
	if cmd != nil {
		t.Fatal("resizing performed effects")
	}
	lines := strings.Split(next.(shellInstallModel).View(), "\n")
	if len(lines) > 8 {
		t.Errorf("review has %d rows; terminal has 8", len(lines))
	}
	for _, line := range lines {
		if cells := ansi.StringWidth(line); cells > 80 {
			t.Errorf("review row has %d cells; terminal has 80", cells)
		}
	}
}

// Selection, terminal handoff and cancellation contracts; help contracts live
// in shell_install_help_{darwin,}_test.go.

func TestShellInstallTUIEditAndCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := shellInstallModel{cancel: cancel, req: shellinstaller.UserInstallRequest{Mode: "separate"}}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/owned/shell")})
	m = next.(shellInstallModel)
	if cmd != nil || m.req.Destination != "/owned/shell" || m.review || m.confirmed {
		t.Fatal("typing performed effects or did not edit selection")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(shellInstallModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(shellInstallModel)
	if m.req.Mode != "shared" || !strings.Contains(m.View(), "/bin/pi") {
		t.Fatal("shared selection or explicit command review absent")
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil || ctx.Err() == nil || next.(shellInstallModel).confirmed {
		t.Fatal("idle cancellation did not settle without installation")
	}
}

func TestShellInstallTUIFieldNavigation(t *testing.T) {
	for _, mode := range []string{"separate", "shared"} {
		t.Run(mode, func(t *testing.T) {
			m := shellInstallModel{req: shellinstaller.UserInstallRequest{Mode: mode}}
			fields := 2
			if mode == "shared" {
				fields = 4
			}
			for step := 1; step <= fields*2; step++ {
				next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
				m = next.(shellInstallModel)
				if cmd != nil || m.field != step%fields {
					t.Fatalf("Tab step %d selected field %d; want %d", step, m.field, step%fields)
				}
			}
		})
	}
}

func TestShellInstallTUIPathsWithSpaces(t *testing.T) {
	for _, field := range []int{0, 2, 3} {
		m := shellInstallModel{field: field, req: shellinstaller.UserInstallRequest{Mode: "shared"}}
		for _, text := range []string{"/owned/my", " ", "shell"} {
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
			m = next.(shellInstallModel)
			if cmd != nil {
				t.Fatal("editing performed effects")
			}
		}
		paths := []string{m.req.Destination, "", m.req.SharedPrefix, m.req.SharedAgent}
		if paths[field] != "/owned/my shell" || m.req.Mode != "shared" {
			t.Fatalf("space was consumed instead of editing field %d: %+v", field, m.req)
		}
	}
}

func TestShellInstallTUIConfirmationReleasesTerminal(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := shellInstallModel{cancel: cancel, review: true,
		req: shellinstaller.UserInstallRequest{Destination: "/owned/shell", Mode: "separate", Confirmation: "reviewed"}}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("confirmation did not release the TUI")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("confirmation ran the installer before releasing the TUI")
	}
	if !next.(shellInstallModel).confirmed {
		t.Fatal("confirmation was not retained for the post-TUI handoff")
	}
	if got := next.(shellInstallModel).req; got != m.req {
		t.Fatalf("confirmed selection changed: %+v", got)
	}
}

func TestShellInstallTUIReviewCancelDoesNotInstall(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyCtrlC, tea.KeyEsc} {
		ctx, cancel := context.WithCancel(context.Background())
		m := shellInstallModel{cancel: cancel, review: true}
		next, cmd := m.Update(tea.KeyMsg{Type: key})
		cancel()
		if cmd == nil || ctx.Err() == nil || next.(shellInstallModel).confirmed {
			t.Fatal("review cancellation authorized an installation or failed to quit")
		}
	}
}

func TestShellInstallTUINonConfirmationReturnsToEdit(t *testing.T) {
	m := shellInstallModel{review: true}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(shellInstallModel)
	if cmd != nil || m.review || m.confirmed {
		t.Fatal("declining review did not return to editing without effects")
	}
}
