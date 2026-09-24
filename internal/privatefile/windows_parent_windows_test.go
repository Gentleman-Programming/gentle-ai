//go:build windows

package privatefile

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func privateWindowsRoot(t *testing.T, path string) windows.Handle {
	t.Helper()
	root, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return windows.Handle(root.Fd())
}

func privateWindowsHandleCount(t *testing.T) uint32 {
	t.Helper()
	var count uint32
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessHandleCount")
	ok, _, err := proc.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&count)))
	if ok == 0 {
		t.Fatalf("GetProcessHandleCount: %v", err)
	}
	return count
}

func TestWindowsPrivateParentOpensPhysicalDirectoriesWithoutTouchingLeaf(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "one", "two")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	leaf := filepath.Join(parent, "report.json")
	if err := os.WriteFile(leaf, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := privateWindowsRoot(t, base)
	handle, err := openWindowsPrivateParent(root, []string{"one", "two"})
	if err != nil {
		t.Fatalf("open physical parent: %v", err)
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		t.Fatal(err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		t.Fatalf("opened nonphysical directory: %#x", info.FileAttributes)
	}
	if err := windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(leaf)
	if err != nil || string(data) != "existing" {
		t.Fatalf("existing leaf changed: %q, %v", data, err)
	}
	if err := WriteNewReport(filepath.Join(parent, "absent.json"), []byte("secret")); err != ErrUnsupported {
		t.Fatalf("Windows report writer = %v, want ErrUnsupported", err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "absent.json")); !os.IsNotExist(err) {
		t.Fatalf("unsupported writer created leaf: %v", err)
	}
}

func TestWindowsPrivateParentRefusesReparsesAndNonDirectories(t *testing.T) {
	for _, kind := range []string{"junction", "symlink", "file"} {
		t.Run(kind, func(t *testing.T) {
			base, outside := t.TempDir(), t.TempDir()
			if err := os.Mkdir(filepath.Join(base, "safe"), 0o700); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(base, "safe", "suspect")
			switch kind {
			case "junction":
				output, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput()
				if err != nil {
					t.Fatalf("mklink /J: %v: %s", err, output)
				}
			case "symlink":
				if err := os.Symlink(outside, link); err != nil {
					t.Fatalf("create directory symlink (native proof requires symlink capability): %v", err)
				}
			case "file":
				if err := os.WriteFile(link, []byte("not a directory"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			sentinel := filepath.Join(outside, "sentinel")
			if err := os.WriteFile(sentinel, []byte("outside"), 0o600); err != nil {
				t.Fatal(err)
			}
			root := privateWindowsRoot(t, base)
			for _, parts := range [][]string{
				{"safe", "suspect"},
				{"safe", "suspect", ".."}, // adversarial traversal after reparse
			} {
				handle, err := openWindowsPrivateParent(root, parts)
				if err == nil {
					_ = windows.CloseHandle(handle)
					t.Fatalf("unsafe parent %q accepted", parts)
				}
			}
			data, readErr := os.ReadFile(sentinel)
			if readErr != nil || string(data) != "outside" {
				t.Fatalf("outside sentinel changed: %q, %v", data, readErr)
			}
		})
	}
}

func TestWindowsPrivateParentRefusesUnsafeComponentsAndClosesOnFailure(t *testing.T) {
	base := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "safe"), 0o700); err != nil {
		t.Fatal(err)
	}
	root := privateWindowsRoot(t, base)
	for _, component := range []string{"", ".", "..", `..\outside`, "safe/../outside", "C:outside", "name:stream", "name.", "name ", "CON", "nul", "bad\x00name"} {
		t.Run(strings.ReplaceAll(component, "\x00", "NUL"), func(t *testing.T) {
			handle, err := openWindowsPrivateParent(root, []string{"safe", component})
			if err == nil {
				_ = windows.CloseHandle(handle)
				t.Fatal("unsafe component accepted")
			}
		})
	}
	// A missing second child exercises release of a successfully opened first
	// child. Compare process handle counts after repeated attempts, not just
	// whether the caller remembered to close a returned handle.
	before := privateWindowsHandleCount(t)
	for range 64 {
		if handle, err := openWindowsPrivateParent(root, []string{"safe", "missing"}); err == nil {
			_ = windows.CloseHandle(handle)
			t.Fatal("missing child accepted")
		}
	}
	if after := privateWindowsHandleCount(t); after != before {
		t.Fatalf("failure leaked handles: before=%d after=%d", before, after)
	}
}
