//go:build windows

package privatefile

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsManagedRootCandidate(t *testing.T) {
	base, outside := t.TempDir(), t.TempDir()
	sentinel := filepath.Join(outside, "sentinel")
	if err := os.WriteFile(sentinel, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		data, err := os.ReadFile(sentinel)
		if err != nil || string(data) != "untouched" {
			t.Errorf("outside sentinel changed: %q, %v", data, err)
		}
	})
	// This is an injected KnownFolderPath string, never the runner's profile.
	knownFolder := filepath.Join(base, "LocalAppData")
	if err := os.Mkdir(knownFolder, 0o700); err != nil {
		t.Fatal(err)
	}
	drive := strings.ToUpper(knownFolder[:2])
	neverOpen := func(string) (windows.Handle, error) { t.Fatal("opened rejected mapping"); return 0, nil }
	for _, tc := range []struct {
		name     string
		path     string
		targets  []string
		queryErr error
	}{
		{"empty mapping", knownFolder, nil, nil},
		{"multiple mappings", knownFolder, []string{`\Device\HarddiskVolume1`, `\Device\HarddiskVolume2`}, nil},
		{"empty target", knownFolder, []string{""}, nil},
		{"query failure", knownFolder, nil, errors.New("unavailable")},
		{"subst drive", knownFolder, []string{`\??\C:\other`}, nil},
		{"mapped UNC", knownFolder, []string{`\Device\Mup\server\share`}, nil},
		{"alias", knownFolder, []string{`\Device\HarddiskVolume1\folder`}, nil},
		{"chain", knownFolder, []string{`\Device\HarddiskVolume1\..\other`}, nil},
		{"other device", knownFolder, []string{`\Device\LanmanRedirector`}, nil},
		{"zero", knownFolder, []string{`\Device\HarddiskVolume0`}, nil},
		{"leading zero", knownFolder, []string{`\Device\HarddiskVolume01`}, nil},
		{"suffix", knownFolder, []string{`\Device\HarddiskVolume1x`}, nil},
		{"case alias", knownFolder, []string{`\device\HarddiskVolume1`}, nil},
		{"UNC", `\\server\share\folder`, nil, nil},
		{"extended", `\\?\C:\folder`, nil, nil},
		{"device", `\\.\C:\folder`, nil, nil},
		{"NT path", `\??\C:\folder`, nil, nil},
		{"relative", `C:folder`, nil, nil},
		{"ADS", drive + `\folder:stream`, nil, nil},
		{"dot", drive + `\foo\.\bar`, nil, nil},
		{"dotdot", drive + `\foo\..\bar`, nil, nil},
		{"empty component", drive + `\foo\\bar`, nil, nil},
		{"short alias", drive + `\PROGRA~1`, nil, nil},
		{"trailing dot", drive + `\foo.`, nil, nil},
		{"slash", drive + `/foo`, nil, nil},
		{"root only", drive + `\`, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			queried := false
			query := func(device string) ([]string, error) {
				queried = true
				if tc.targets == nil && tc.queryErr == nil && tc.path != knownFolder {
					t.Fatal("queried invalid path")
				}
				if device != drive {
					t.Fatalf("queried %q, want %q", device, drive)
				}
				return tc.targets, tc.queryErr
			}
			h, _, err := openWindowsManagedRootCandidate(tc.path, query, neverOpen)
			if err == nil {
				_ = windows.CloseHandle(h)
				t.Fatal("accepted unsafe candidate")
			}
			if tc.path == knownFolder && !queried {
				t.Fatal("did not query mapping")
			}
		})
	}

	t.Run("open error with valid handle closes it", func(t *testing.T) {
		name, err := windows.UTF16PtrFromString(knownFolder)
		if err != nil {
			t.Fatal(err)
		}
		opened, err := windows.CreateFile(name, windows.GENERIC_READ,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if err != nil {
			t.Fatal(err)
		}
		h, mapping, err := openWindowsManagedRootCandidate(knownFolder,
			func(string) ([]string, error) { return []string{`\Device\HarddiskVolume1`}, nil },
			func(string) (windows.Handle, error) { return opened, errors.New("injected open failure") })
		if err == nil || h != 0 || mapping != "" {
			_ = windows.CloseHandle(opened)
			t.Fatalf("open failure returned a candidate: handle=%v, mapping=%q, err=%v", h != 0, mapping, err)
		}
		var info windows.ByHandleFileInformation
		if err := windows.GetFileInformationByHandle(opened, &info); !errors.Is(err, windows.ERROR_INVALID_HANDLE) {
			_ = windows.CloseHandle(opened)
			t.Fatalf("error path left handle open: %v", err)
		}
	})

	t.Run("non-directory and reparse handles", func(t *testing.T) {
		file := filepath.Join(base, "file")
		if err := os.WriteFile(file, []byte("file"), 0o600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(base, "junction")
		if output, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
			t.Fatalf("create junction: %v: %s", err, output)
		}
		for _, path := range []string{file, link} {
			open := func(target string) (windows.Handle, error) {
				if target != `\Device\HarddiskVolume1` {
					t.Fatalf("opened %q", target)
				}
				name, err := windows.UTF16PtrFromString(path)
				if err != nil {
					return 0, err
				}
				return windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
					nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
			}
			h, _, err := openWindowsManagedRootCandidate(knownFolder,
				func(string) ([]string, error) { return []string{`\Device\HarddiskVolume1`}, nil }, open)
			if err == nil {
				_ = windows.CloseHandle(h)
				t.Fatalf("accepted %q", path)
			}
		}
	})

	t.Run("held direct volume root is only a candidate", func(t *testing.T) {
		h, mapping, err := openWindowsManagedRootCandidate(knownFolder, queryWindowsManagedDevice, openWindowsManagedVolume)
		if err != nil {
			t.Fatalf("fixture drive has no direct candidate: %v", err)
		}
		defer windows.CloseHandle(h)
		if !strings.HasPrefix(mapping, `\Device\HarddiskVolume`) {
			t.Fatalf("mapping %q", mapping)
		}
		var info windows.ByHandleFileInformation
		if err := windows.GetFileInformationByHandle(h, &info); err != nil {
			t.Fatal(err)
		}
		if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			t.Fatalf("candidate attributes: %#x", info.FileAttributes)
		}
	})
}
