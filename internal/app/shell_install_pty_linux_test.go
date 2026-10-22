//go:build linux

package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/gentleman-programming/gentle-ai/v4/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"golang.org/x/sys/unix"
)

func TestShellPTYFailureChildIsStoppedAndReaped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestShellInstallParentPhysicalPTY$", "-test.v")
	child.Env = append(os.Environ(), "GENTLE_TEST_SHELL_PTY_SCENARIO=stalled-cleanup")
	output, err := child.CombinedOutput()
	if !strings.Contains(string(output), "STALLED_PTY_CHILD_READY") || err == nil || ctx.Err() != context.DeadlineExceeded || child.ProcessState == nil {
		t.Fatalf("controlled stalled child was not bounded/reaped: err=%v ctx=%v output=%s", err, ctx.Err(), output)
	}
	if err := unix.Kill(child.Process.Pid, 0); err != unix.ESRCH {
		t.Fatalf("stalled PTY child still exists after wait: %v", err)
	}
}

// Isolate terminal interaction from unrelated startup network commands.
// Update/View and the application's physical selection remain production code.
type shellPTYModel struct{ tea.Model }

func (shellPTYModel) Init() tea.Cmd { return nil }

func shellPTYExperience(scenario string) shellinstaller.UserExperience {
	profile := shellinstaller.DefaultUserExperience()
	profile.StrictTDD, profile.GlobalRDD, profile.CodeGraph, profile.Context7, profile.GGA = "on", "on", "on", "on", "off"
	if strings.HasPrefix(scenario, "confirm-main") {
		profile.SDKSource = "main-preview"
	}
	return profile
}

func TestShellInstallParentPhysicalPTY(t *testing.T) {
	scenarios := []string{"confirm", "confirm-edit", "cancel-target", "cancel-review", "ctrl-c-target", "ctrl-c-review", "install-failure"}
	if runtime.GOARCH == "amd64" {
		scenarios = append(scenarios, "confirm-main", "confirm-main-edit")
	}
	const childScenario = "GENTLE_TEST_SHELL_PTY_SCENARIO"
	if selected := os.Getenv(childScenario); selected != "" {
		if selected == "stalled-cleanup" {
			fmt.Println("STALLED_PTY_CHILD_READY")
			time.Sleep(time.Minute) // Controlled negative: parent must stop/reap this child.
			return
		}
		scenarios = []string{selected}
	}
	for _, scenario := range scenarios {
		t.Run(scenario, func(t *testing.T) {
			if os.Getenv(childScenario) == "" {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestShellInstallParentPhysicalPTY$", "-test.v")
				child.Env = append(os.Environ(), childScenario+"="+scenario)
				output, err := child.CombinedOutput() // Always waits/reaps, including cancellation.
				t.Logf("%s", output)
				if err != nil || ctx.Err() != nil {
					t.Fatalf("isolated PTY scenario failed: %v, deadline=%v", err, ctx.Err())
				}
				return
			}
			master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = master.Close() })
			if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
				t.Fatal(err)
			}
			number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
			if err != nil {
				t.Fatal(err)
			}
			slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|unix.O_NOCTTY, 0)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = slave.Close() })
			if err := unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: 100, Row: 35}); err != nil {
				t.Fatal(err)
			}
			before, err := term.GetState(slave.Fd())
			if err != nil {
				t.Fatal(err)
			}
			home := t.TempDir()
			if err := os.Chmod(home, 0700); err != nil {
				t.Fatal(err)
			}
			setupMockHome(t, home)
			t.Setenv("GENTLE_AI_NO_ANIMATION", "1")
			target := filepath.Join(home, ".local", "share", "gentle-shell", "install")
			var originalConfirmation string
			readerDone, parentDone := make(chan struct{}), make(chan struct{})
			oldIn, oldOut := os.Stdin, os.Stdout
			oldOS, oldDetect, oldTUI, oldRun, oldInline := ensureCurrentOSSupported, detectSystem, runTUI, runShellInstallSelection, runInlineShellInstall
			os.Stdin, os.Stdout = slave, slave // Real TTY boundary, not an isatty override.
			t.Cleanup(func() {
				select {
				case <-readerDone:
				default:
					t.Error("PTY reader is still running before global restoration")
				}
				select {
				case <-parentDone:
				default:
					t.Error("terminal owner is still running before global restoration")
				}
				os.Stdin, os.Stdout = oldIn, oldOut
				ensureCurrentOSSupported, detectSystem, runTUI, runShellInstallSelection, runInlineShellInstall = oldOS, oldDetect, oldTUI, oldRun, oldInline
			})
			ensureCurrentOSSupported = func() error { return nil }
			detectSystem = func(context.Context) (system.DetectionResult, error) {
				return system.DetectionResult{System: system.SystemInfo{Supported: true}}, nil
			}
			programs, returned := 0, false
			created := make(chan *tea.Program, 1)
			runTUI = func(m tea.Model, opts ...tea.ProgramOption) (tea.Model, error) {
				programs++
				p := tea.NewProgram(shellPTYModel{m}, append(opts, tea.WithInput(slave), tea.WithOutput(slave))...)
				created <- p
				defer func() { returned = true }()
				return p.Run()
			}
			inlineCalls, postWorkers := 0, 0
			runInlineShellInstall = func(ctx context.Context, request shellinstaller.UserInstallRequest, report func(shellinstaller.UserInstallProgress)) error {
				inlineCalls++
				if returned {
					return fmt.Errorf("in-process worker ran after the TUI returned")
				}
				wantProfile := shellPTYExperience(scenario)
				if strings.HasSuffix(scenario, "edit") {
					wantProfile.Persona = "neutral"
				}
				if request.Destination != target || request.Confirmation == "" || request.Experience == nil || *request.Experience != wantProfile {
					return fmt.Errorf("in-process worker lost physical selection: got %+v want %+v", request.Experience, wantProfile)
				}
				if strings.HasSuffix(scenario, "edit") && (originalConfirmation == "" || request.Confirmation == originalConfirmation) {
					return fmt.Errorf("edited profile reused physical confirmation: %+v", request)
				}
				for index, label := range []string{"Validate the selection and target", "Prepare the private stage and preferences", "Install the runtime and pinned suppliers", "Apply optional runtimes and the SDK channel", "Publish, verify and register the installation"} {
					report(shellinstaller.UserInstallProgress{Index: index + 1, Total: 5, Label: label, State: "running"})
					report(shellinstaller.UserInstallProgress{Index: index + 1, Total: 5, Label: label, State: "done"})
					time.Sleep(30 * time.Millisecond)
				}
				if scenario == "install-failure" {
					return fmt.Errorf("private installation: uncertain; Preserve evidence: workspace=%q destination/unit=%q", filepath.Dir(target)+"/.gentle-pty", target)
				}
				return nil
			}
			runShellInstallSelection = func(selection cli.ShellInstallSelection, stdout io.Writer) error {
				postWorkers++
				return fmt.Errorf("post-TUI worker ran for an in-process selection")
			}
			var output bytes.Buffer
			var mu sync.Mutex
			go func() {
				defer close(readerDone)
				buffer := make([]byte, 4096)
				for {
					n, err := master.Read(buffer)
					mu.Lock()
					if output.Len()+n < 256*1024 {
						_, _ = output.Write(buffer[:n])
					}
					mu.Unlock()
					if err != nil {
						return
					}
				}
			}()
			waitFor := func(text string) {
				t.Helper()
				deadline := time.Now().Add(8 * time.Second)
				for time.Now().Before(deadline) {
					mu.Lock()
					view := ansi.Strip(output.String())
					mu.Unlock()
					if strings.Contains(view, text) {
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
				mu.Lock()
				view := ansi.Strip(output.String())
				mu.Unlock()
				t.Fatalf("PTY did not render %q; output=%q", text, view)
			}
			send := func(text string) {
				t.Helper()
				if _, err := master.Write([]byte(text)); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan error, 1)
			go func() {
				err := RunArgs(nil, slave)
				close(parentDone)
				done <- err
			}()
			t.Cleanup(func() {
				select {
				case p := <-created:
					p.Kill()
				default:
				}
				select {
				case <-parentDone:
				case <-time.After(2 * time.Second):
					t.Error("terminal owner did not join during cleanup")
				}
				_ = master.Close() // Wake the reader before any process-global restoration.
				select {
				case <-readerDone:
				case <-time.After(2 * time.Second):
					t.Error("PTY reader did not join during cleanup")
				}
			})
			waitFor("Install Gentle-Shell, our own agent")
			send("\r")
			waitFor("Configure Gentle-Shell experience")
			if strings.HasPrefix(scenario, "confirm-main") {
				// Main keeps the concrete fresh preset; ON/OFF toggles no longer
				// expose an inherited third state in the editor.
				send("\x1b[H" + strings.Repeat("\x1b[B", 9) + "\r")
				waitFor("SDK Main + stock provider")
			}
			send("\x1b[F\r")
			waitFor("Installation plan")
			waitFor("5. Publish, verify and register the installation")
			if strings.HasSuffix(scenario, "edit") {
				profile := shellPTYExperience(scenario)
				var err error
				originalConfirmation, err = shellinstaller.InspectUserInstall(shellinstaller.UserInstallRequest{
					Destination: target, Mode: "separate", Experience: &profile,
				})
				if err != nil {
					t.Fatal(err)
				}
				mu.Lock()
				output.Reset()
				mu.Unlock()
				send("\x1b") // The plan goes back to configuration without consent.
				waitFor("Configure Gentle-Shell experience")
				send("\x1b[H\x1b[B\r\x1b[F\r")
				waitFor("Installation plan")
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatal("editing published before physical confirmation", err)
				}
				select {
				case err := <-done:
					t.Fatal("editing ended the terminal owner before confirmation", err)
				default:
				}
			}
			confirms := strings.HasPrefix(scenario, "confirm") || scenario == "install-failure"
			if confirms {
				send("\r")
				waitFor("Installing Gentle-Shell")
				if scenario == "install-failure" {
					waitFor("Installation failed")
					waitFor("workspace=")
					send("\r")
					waitFor("Install Gentle-Shell, our own agent")
					send("q")
				} else {
					waitFor("Installation complete")
					send("\r")
				}
			} else {
				mu.Lock()
				output.Reset()
				mu.Unlock()
				if strings.HasPrefix(scenario, "ctrl-c") {
					send("\x03")
					waitFor("Install Gentle-Shell, our own agent")
				} else {
					send("\x1b")
					waitFor("Configure Gentle-Shell experience")
					send("\x1b")
					waitFor("Install Gentle-Shell, our own agent")
				}
				send("q")
			}
			select {
			case err := <-done:
				wantInline := 0
				if strings.HasPrefix(scenario, "confirm") || scenario == "install-failure" {
					wantInline = 1
				}
				if err != nil || programs != 1 || inlineCalls != wantInline || postWorkers != 0 {
					t.Fatalf("programs=%d inline=%d post=%d error=%v", programs, inlineCalls, postWorkers, err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("terminal owner did not finish")
			}
			after, err := term.GetState(slave.Fd())
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("termios not restored", err)
			}
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatal("PTY fixture installed a target", err)
			}
		})
	}
}
