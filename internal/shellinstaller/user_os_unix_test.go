//go:build linux || darwin

package shellinstaller

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestUnixStatTimesReadKernelTimestamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stamped")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	want := time.Unix(1700000000, 123456789)
	if err := os.Chtimes(path, want, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	mtime, ctime := privateStatTimes(info.Sys().(*syscall.Stat_t))
	if got := time.Unix(mtime.Unix()); !got.Equal(info.ModTime()) || got.Unix() != want.Unix() {
		t.Fatalf("mtime = %v, ModTime = %v, want seconds %d", got, info.ModTime(), want.Unix())
	}
	if ctime.Sec <= 0 || ctime == mtime {
		t.Fatalf("ctime = %v must be a real change time distinct from the forged mtime %v", ctime, mtime)
	}
	if err := os.Chmod(path, 0400); err != nil {
		t.Fatal(err)
	}
	fresh, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if privateStamp(info) == privateStamp(fresh) {
		t.Fatal("stamp must change when metadata changes")
	}
}

func TestUnixTermiosRefusesNonTerminals(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	if _, err := userTermios(int(reader.Fd())); !errors.Is(err, unix.ENOTTY) {
		t.Fatalf("pipe termios error = %v, want ENOTTY", err)
	}
	if interactive, err := userInteractive(reader); interactive || err != nil {
		t.Fatalf("pipe interactive = %v, %v; want false, nil", interactive, err)
	}
	if interactive, err := userInteractive(strings.NewReader("")); interactive || err != nil {
		t.Fatalf("non-file interactive = %v, %v; want false, nil", interactive, err)
	}
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	restore, err := userForeground(cmd, reader)
	if err != nil || restore == nil || restore() != nil {
		t.Fatalf("pipe foreground = %v", err)
	}
	if !cmd.SysProcAttr.Setpgid || cmd.SysProcAttr.Foreground {
		t.Fatalf("pipe launch must own a background process group: %+v", cmd.SysProcAttr)
	}
	closed, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	fd := int(closed.Fd())
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := userTermios(fd); err == nil || errors.Is(err, unix.ENOTTY) {
		t.Fatalf("closed descriptor termios error = %v, want a non-ENOTTY failure", err)
	}
}

func TestUnixTermiosReadsPseudoTerminal(t *testing.T) {
	terminal := openTestTerminal(t)
	if _, err := userTermios(int(terminal.Fd())); err != nil {
		t.Fatalf("pseudo-terminal termios = %v", err)
	}
	if interactive, err := userInteractive(terminal); !interactive || err != nil {
		t.Fatalf("pseudo-terminal interactive = %v, %v; want true, nil", interactive, err)
	}
}

func TestUnixRenameNoReplaceRefusesExistingDestination(t *testing.T) {
	base := t.TempDir()
	source, existing, fresh := filepath.Join(base, "stage"), filepath.Join(base, "existing"), filepath.Join(base, "fresh")
	for _, directory := range []string{source, existing} {
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(existing, "keep"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{existing, file} {
		if err := userRenameNoReplace(source, target); !errors.Is(err, unix.EEXIST) {
			t.Fatalf("rename onto %s error = %v, want EEXIST", filepath.Base(target), err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(existing, "keep")); err != nil || string(data) != "old" {
		t.Fatalf("existing destination changed: %q, %v", data, err)
	}
	if info, err := os.Lstat(source); err != nil || !info.IsDir() {
		t.Fatalf("refused source must remain: %v", err)
	}
	if err := userRenameNoReplace(source, fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(source); !os.IsNotExist(err) {
		t.Fatalf("source after publication = %v, want absent", err)
	}
	if info, err := os.Lstat(fresh); err != nil || !info.IsDir() {
		t.Fatalf("published destination = %v", err)
	}
}

func TestUnixReapGroupKillsOwnedDescendants(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "survived")
	cmd := exec.Command("/bin/sh", "-c", `(sleep 2; : > "$1") & exit 0`, "sh", marker)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitErr := cmd.Wait()
	if waitErr != nil {
		t.Fatal(waitErr)
	}
	if err := syscall.Kill(-cmd.Process.Pid, 0); err != nil {
		t.Fatalf("background descendant must still occupy the group before reaping: %v", err)
	}
	if err := userReapGroup(cmd.Process.Pid, waitErr); err != nil {
		t.Fatalf("reap = %v", err)
	}
	if err := syscall.Kill(-cmd.Process.Pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("group after reap = %v, want ESRCH", err)
	}
	<-time.After(2500 * time.Millisecond)
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant ran after reap: %v", err)
	}
}

func TestUnixReapGroupAcceptsAlreadyEmptyGroup(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 3")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitErr := cmd.Wait()
	var exit *exec.ExitError
	if !errors.As(waitErr, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("wait = %v, want exit 3", waitErr)
	}
	if err := userReapGroup(cmd.Process.Pid, waitErr); err != nil {
		t.Fatalf("empty group reap = %v", err)
	}
}
