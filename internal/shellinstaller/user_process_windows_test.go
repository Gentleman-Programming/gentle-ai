//go:build windows

package shellinstaller

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestUserWindowsJobPhysicalLimits(t *testing.T) {
	job, err := userWindowsJob()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(job) })
	var memory windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&memory)), uint32(unsafe.Sizeof(memory)), nil); err != nil {
		t.Fatal(err)
	}
	if memory.BasicLimitInformation.LimitFlags != userWindowsJobFlags || memory.BasicLimitInformation.ActiveProcessLimit != 64 || memory.JobMemoryLimit != 3221225472 {
		t.Fatalf("physical job limits differ: %#v", memory)
	}
	var cpu userWindowsCPU
	if err := windows.QueryInformationJobObject(job, windows.JobObjectCpuRateControlInformation, uintptr(unsafe.Pointer(&cpu)), uint32(unsafe.Sizeof(cpu)), nil); err != nil {
		t.Fatal(err)
	}
	if cpu.Flags != 5 || cpu.Rate != uint32(10000/runtime.NumCPU()) {
		t.Fatalf("physical CPU hard cap differs: %#v", cpu)
	}
}

func TestUserWindowsProcessFixture(t *testing.T) {
	if os.Getenv("GENTLE_WINDOWS_PROCESS_FIXTURE") != "1" {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(os.Getenv("GENTLE_WINDOWS_PROCESS_MARKER"), []byte(cwd), 0600); err != nil {
		os.Exit(3)
	}
	_, _ = os.Stdout.WriteString("fixture ran\n")
	os.Exit(0)
}

func userWindowsProcessFixture(t *testing.T) (*exec.Cmd, string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	marker := filepath.Join(cwd, "executed.txt")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, self, "-test.run=^TestUserWindowsProcessFixture$")
	command.Dir = cwd
	command.Env = []string{"GENTLE_WINDOWS_PROCESS_FIXTURE=1", "GENTLE_WINDOWS_PROCESS_MARKER=" + marker, "SystemRoot=" + os.Getenv("SystemRoot")}
	return command, marker
}

func TestUserWindowsProcessStartsAfterBindingAndPreservesCWD(t *testing.T) {
	command, marker := userWindowsProcessFixture(t)
	var output bytes.Buffer
	command.Stdout = &output
	release, err := userWindowsStart(command)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := release(); err != nil {
			t.Error(err)
		}
	}()
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != command.Dir || output.String() != "fixture ran\n" {
		t.Fatalf("child CWD/output differ: %q, %q, %v", got, output.String(), err)
	}
}

// Cancellation fixture worker. "cooperative" records that its own context was
// canceled through the parent channel; "quiesce" starts an owned sleeping
// descendant and records whether job quiescence terminated it.
func TestUserWindowsCancelFixture(t *testing.T) {
	mode, marker := os.Getenv("GENTLE_WINDOWS_CANCEL_FIXTURE"), os.Getenv("GENTLE_WINDOWS_PROCESS_MARKER")
	switch mode {
	case "":
		return
	case "sleep":
		time.Sleep(time.Minute)
		os.Exit(5)
	case "cooperative":
		ctx, stop, err := userWindowsCancelWatch(context.Background(), os.Getenv(userWindowsCancelEnv))
		if err != nil {
			os.Exit(2)
		}
		defer stop()
		select {
		case <-ctx.Done():
		case <-time.After(20 * time.Second):
			os.Exit(4)
		}
		_ = os.WriteFile(marker, []byte("cleaned"), 0600)
		os.Exit(0)
	case "quiesce":
		self, _ := os.Executable()
		child := exec.Command(self, "-test.run=^TestUserWindowsCancelFixture$")
		child.Env = []string{"GENTLE_WINDOWS_CANCEL_FIXTURE=sleep", "SystemRoot=" + os.Getenv("SystemRoot")}
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		result := "quiesced"
		if err := userWindowsJobQuiesce(10 * time.Second); err != nil {
			result = err.Error()
		}
		if state, err := child.Process.Wait(); err != nil || state.ExitCode() == 5 {
			result += "; descendant survived"
		}
		_ = os.WriteFile(marker, []byte(result), 0600)
		os.Exit(0)
	}
	os.Exit(3)
}

func userWindowsCancelFixture(t *testing.T, ctx context.Context, mode string) (*exec.Cmd, string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "marker.txt")
	command := exec.CommandContext(ctx, self, "-test.run=^TestUserWindowsCancelFixture$")
	command.Env = []string{"GENTLE_WINDOWS_CANCEL_FIXTURE=" + mode, "GENTLE_WINDOWS_PROCESS_MARKER=" + marker, "SystemRoot=" + os.Getenv("SystemRoot")}
	return command, marker
}

func TestUserWindowsCancellationIsCooperative(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command, marker := userWindowsCancelFixture(t, ctx, "cooperative")
	closeChannel, err := userWindowsCooperative(command, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	release, err := userWindowsStart(command)
	if err != nil {
		t.Fatal(errors.Join(err, closeChannel()))
	}
	started := time.Now()
	cancel()
	waitErr := command.Wait()
	if err := errors.Join(release(), closeChannel()); err != nil {
		t.Error(err)
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "cleaned" || command.ProcessState.ExitCode() != 0 || time.Since(started) >= 30*time.Second {
		t.Fatalf("worker was not allowed to unwind: %q %v %v %v", got, err, waitErr, command.ProcessState)
	}
}

func TestUserWindowsCancelWatchRefusesForeignChannels(t *testing.T) {
	ctx, stop, err := userWindowsCancelWatch(context.Background(), "")
	if err != nil || ctx.Err() != nil {
		t.Fatalf("direct worker without channel: %v %v", err, ctx.Err())
	}
	stop()
	for _, name := range []string{`Local\foreign`, `Global\gentle-shell-windows-cancel-0123456789abcdef0123456789abcdef`, `Local\gentle-shell-windows-cancel-0123456789ABCDEF0123456789ABCDEF`, `Local\gentle-shell-windows-cancel-0123456789abcdef0123456789abcdef`} {
		if _, _, err := userWindowsCancelWatch(context.Background(), name); err == nil {
			t.Fatalf("foreign or absent cancellation channel admitted: %q", name)
		}
	}
}

func TestUserWindowsJobQuiesceStopsOwnedDescendants(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command, marker := userWindowsCancelFixture(t, ctx, "quiesce")
	release, err := userWindowsStart(command)
	if err != nil {
		t.Fatal(err)
	}
	waitErr := command.Wait()
	if err := release(); err != nil {
		t.Error(err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "quiesced" {
		t.Fatalf("owned descendants not quiesced: %q %v %v", got, err, waitErr)
	}
	if err := userWindowsJobQuiesce(time.Second); err == nil {
		t.Fatal("quiescence claimed outside an owned worker job")
	}
}

func TestUserWindowsProcessCannotRunBeforeJobBinding(t *testing.T) {
	previous := assignUserWindowsProcessToJob
	failure := errors.New("forced physical job binding refusal")
	assignUserWindowsProcessToJob = func(windows.Handle, windows.Handle) error { return failure }
	t.Cleanup(func() { assignUserWindowsProcessToJob = previous })
	command, marker := userWindowsProcessFixture(t)
	release, err := userWindowsStart(command)
	if release != nil || !errors.Is(err, failure) {
		t.Fatalf("binding failure = (%v, %v)", release != nil, err)
	}
	if command.ProcessState == nil {
		t.Fatal("refused suspended child was not reaped")
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("child executed before binding: %v", err)
	}
}
