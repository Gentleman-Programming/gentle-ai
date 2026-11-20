//go:build windows

package shellinstaller

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A file under the installation can be held for a moment by an antivirus scan
// or the search indexer right after it is written, and for as long as it runs
// by a Gentle Shell process (node.exe, supervisor.exe, engram.exe) started from
// the installation. Renames and removals of owned paths therefore retry a
// sharing, lock or access violation with a short bounded backoff (about 2.6 s
// in total) and, when the path stays busy, name the processes running from
// the installation root instead of returning the raw Windows error.
var (
	userWindowsBusyDelays = []time.Duration{100 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond, 750 * time.Millisecond, 1000 * time.Millisecond}
	userWindowsBusySleep  = time.Sleep
)

// userWindowsBusyCause reports whether err is a sharing, lock or access
// violation that a closing handle can clear.
func userWindowsBusyCause(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

// userWindowsRetryBusy runs one idempotent-on-failure filesystem operation
// (a rename or a removal) and retries it while it fails as busy. A different
// error returns at once, unchanged. A path still busy after the last attempt
// returns an actionable error that keeps the original cause for errors.Is.
func userWindowsRetryBusy(root string, operation func() error) error {
	err := operation()
	for _, delay := range userWindowsBusyDelays {
		if err == nil || !userWindowsBusyCause(err) {
			return err
		}
		userWindowsBusySleep(delay)
		err = operation()
	}
	if err == nil || !userWindowsBusyCause(err) {
		return err
	}
	if busy := userWindowsRefuseRunning(root); busy != nil {
		return fmt.Errorf("%w; %w", err, busy)
	}
	return fmt.Errorf("%w; a file under %s is still in use by another program (for example an antivirus scan or an open editor or terminal); close it and retry", err, root)
}

// userWindowsRefuseRunning refuses a mutation of root while a process started
// from it still runs. Windows lets a directory holding a running image be
// renamed, so the rename alone would move a live installation aside and leave
// its files undeletable; the refusal comes before the first byte moves.
func userWindowsRefuseRunning(root string) error {
	running := userWindowsProcessesUnder(root)
	if len(running) == 0 {
		return nil
	}
	return fmt.Errorf("the Gentle Shell installation at %s is still in use: close these Gentle Shell processes and retry: %s", root, strings.Join(running, ", "))
}

func userWindowsRenameBusy(root, from, to string) error {
	return userWindowsRetryBusy(root, func() error { return os.Rename(from, to) })
}

// userWindowsMoveNoReplaceBusy renames from to to like userWindowsRenameBusy
// but without MOVEFILE_REPLACE_EXISTING: an existing destination is never
// overwritten (ERROR_ALREADY_EXISTS is not a busy cause and returns at once).
func userWindowsMoveNoReplaceBusy(root, from, to string) error {
	source, sourceErr := windows.UTF16PtrFromString(from)
	destination, destinationErr := windows.UTF16PtrFromString(to)
	if sourceErr != nil || destinationErr != nil {
		return errors.Join(sourceErr, destinationErr)
	}
	return userWindowsRetryBusy(root, func() error {
		return windows.MoveFileEx(source, destination, windows.MOVEFILE_WRITE_THROUGH)
	})
}

func userWindowsRemoveBusy(root, path string) error {
	return userWindowsRetryBusy(root, func() error { return os.Remove(path) })
}

func userWindowsRemoveAllBusy(root, path string) error {
	return userWindowsRetryBusy(root, func() error { return os.RemoveAll(path) })
}

// userWindowsLongPath expands an 8.3 short path so it compares with the long
// image paths Windows reports for running processes.
func userWindowsLongPath(path string) string {
	path = filepath.Clean(path)
	short, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return path
	}
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetLongPathName(short, &buffer[0], uint32(len(buffer)))
	if err != nil || n == 0 || int(n) > len(buffer) {
		return path
	}
	return windows.UTF16ToString(buffer[:n])
}

// userWindowsProcessesUnder lists running processes whose image lives below
// root as "name (PID n)", sorted. Processes this user cannot query are
// skipped; enumeration failures yield an empty list, never an error, because
// the list only improves the message of an operation that already failed.
func userWindowsProcessesUnder(root string) []string {
	prefix := strings.ToLower(strings.TrimRight(userWindowsLongPath(root), `\`) + `\`)
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snapshot)
	var running []string
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		process, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if openErr != nil {
			continue
		}
		buffer := make([]uint16, windows.MAX_LONG_PATH)
		size := uint32(len(buffer))
		queryErr := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size)
		windows.CloseHandle(process)
		if queryErr != nil {
			continue
		}
		image := windows.UTF16ToString(buffer[:size])
		if strings.HasPrefix(strings.ToLower(userWindowsLongPath(image)), prefix) {
			running = append(running, fmt.Sprintf("%s (PID %d)", filepath.Base(image), entry.ProcessID))
		}
	}
	sort.Strings(running)
	return running
}
