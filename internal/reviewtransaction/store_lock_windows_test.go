//go:build windows

package reviewtransaction

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/sys/windows"
)

func TestNTPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "drive path",
			path: `C:\review-store\LOCK`,
			want: `\??\C:\review-store\LOCK`,
		},
		{
			name: "UNC path",
			path: `\\server\share\review-store\LOCK`,
			want: `\??\UNC\server\share\review-store\LOCK`,
		},
		{
			name: "extended drive path",
			path: `\\?\C:\review-store\LOCK`,
			want: `\??\C:\review-store\LOCK`,
		},
		{
			name: "extended UNC path",
			path: `\\?\UNC\server\share\review-store\LOCK`,
			want: `\??\UNC\server\share\review-store\LOCK`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ntPath(tt.path); got != tt.want {
				t.Fatalf("ntPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestWindowsSecureOpenRetriesDirectDriveDeviceWithoutWeakeningFlags(t *testing.T) {
	type openCall struct {
		path             string
		access           uint32
		objectAttributes uint32
		fileAttributes   uint32
		share            uint32
		disposition      uint32
		options          uint32
		eaBuffer         uintptr
		eaLength         uint32
	}

	var calls []openCall
	createFile := func(handle *windows.Handle, access uint32, attributes *windows.OBJECT_ATTRIBUTES, _ *windows.IO_STATUS_BLOCK, _ *int64, fileAttributes, share, disposition, options uint32, eaBuffer uintptr, eaLength uint32) error {
		calls = append(calls, openCall{
			path:             attributes.ObjectName.String(),
			access:           access,
			objectAttributes: attributes.Attributes,
			fileAttributes:   fileAttributes,
			share:            share,
			disposition:      disposition,
			options:          options,
			eaBuffer:         eaBuffer,
			eaLength:         eaLength,
		})
		if len(calls) == 1 {
			return windows.STATUS_REPARSE_POINT_ENCOUNTERED
		}
		*handle = windows.Handle(42)
		return nil
	}
	var queried []string
	queryDevice := func(device string) ([]string, error) {
		queried = append(queried, device)
		return []string{`\Device\HarddiskVolume3`}, nil
	}

	handle, err := openWindowsStoreLockObject(`\??\C:\repo\with spaces\LOCK`, createFile, queryDevice)
	if err != nil {
		t.Fatal(err)
	}
	if handle != windows.Handle(42) {
		t.Fatalf("handle = %v, want 42", handle)
	}
	if want := []string{"C:"}; !reflect.DeepEqual(queried, want) {
		t.Fatalf("QueryDosDevice calls = %#v, want %#v", queried, want)
	}
	if len(calls) != 2 {
		t.Fatalf("NtCreateFile calls = %d, want 2", len(calls))
	}
	if calls[0].path != `\??\C:\repo\with spaces\LOCK` {
		t.Fatalf("initial object path = %q", calls[0].path)
	}
	if calls[1].path != `\Device\HarddiskVolume3\repo\with spaces\LOCK` {
		t.Fatalf("retry object path = %q", calls[1].path)
	}
	for i, call := range calls {
		if call.access != windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE ||
			call.objectAttributes != windows.OBJ_CASE_INSENSITIVE|windows.OBJ_DONT_REPARSE ||
			call.fileAttributes != windows.FILE_ATTRIBUTE_NORMAL ||
			call.share != windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE ||
			call.disposition != windows.FILE_OPEN_IF ||
			call.options != windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT ||
			call.eaBuffer != 0 || call.eaLength != 0 {
			t.Fatalf("NtCreateFile call %d changed secure open contract: %#v", i+1, call)
		}
	}
}

func TestWindowsSecureOpenDriveFallbackFailsClosed(t *testing.T) {
	tests := []struct {
		name        string
		objectPath  string
		targets     []string
		queryErr    error
		wantQuery   bool
		wantCreates int
	}{
		{name: "UNC input", objectPath: `\??\UNC\server\share\LOCK`},
		{name: "ambiguous mapping", objectPath: `\??\C:\repo\LOCK`, targets: []string{`\Device\HarddiskVolume3`, `\Device\HarddiskVolume4`}, wantQuery: true},
		{name: "cyclic mapping", objectPath: `\??\C:\repo\LOCK`, targets: []string{`\??\C:`}, wantQuery: true},
		{name: "substituted mapping", objectPath: `\??\C:\repo\LOCK`, targets: []string{`\??\D:\subdirectory`}, wantQuery: true},
		{name: "redirector mapping", objectPath: `\??\C:\repo\LOCK`, targets: []string{`\Device\Mup\server\share`}, wantQuery: true},
		{name: "query failure", objectPath: `\??\C:\repo\LOCK`, queryErr: errors.New("query failed"), wantQuery: true},
		{name: "second reparse", objectPath: `\??\C:\repo\LOCK`, targets: []string{`\Device\HarddiskVolume3`}, wantQuery: true, wantCreates: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createCalls := 0
			createFile := func(_ *windows.Handle, _ uint32, _ *windows.OBJECT_ATTRIBUTES, _ *windows.IO_STATUS_BLOCK, _ *int64, _, _, _, _ uint32, _ uintptr, _ uint32) error {
				createCalls++
				return windows.STATUS_REPARSE_POINT_ENCOUNTERED
			}
			queryCalls := 0
			queryDevice := func(string) ([]string, error) {
				queryCalls++
				return tt.targets, tt.queryErr
			}

			if _, err := openWindowsStoreLockObject(tt.objectPath, createFile, queryDevice); err == nil {
				t.Fatal("openWindowsStoreLockObject succeeded")
			}
			wantCreates := tt.wantCreates
			if wantCreates == 0 {
				wantCreates = 1
			}
			if createCalls != wantCreates {
				t.Fatalf("NtCreateFile calls = %d, want %d", createCalls, wantCreates)
			}
			wantQueryCalls := 0
			if tt.wantQuery {
				wantQueryCalls = 1
			}
			if queryCalls != wantQueryCalls {
				t.Fatalf("QueryDosDevice calls = %d, want %d", queryCalls, wantQueryCalls)
			}
		})
	}
}

func TestWindowsSecureOpenDoesNotRetryOtherErrors(t *testing.T) {
	createCalls := 0
	createFile := func(_ *windows.Handle, _ uint32, _ *windows.OBJECT_ATTRIBUTES, _ *windows.IO_STATUS_BLOCK, _ *int64, _, _, _, _ uint32, _ uintptr, _ uint32) error {
		createCalls++
		return windows.ERROR_ACCESS_DENIED
	}
	queryDevice := func(string) ([]string, error) {
		t.Fatal("QueryDosDevice called for a non-reparse error")
		return nil, nil
	}

	_, err := openWindowsStoreLockObject(`\??\C:\repo\LOCK`, createFile, queryDevice)
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("error = %v, want ERROR_ACCESS_DENIED", err)
	}
	if createCalls != 1 {
		t.Fatalf("NtCreateFile calls = %d, want 1", createCalls)
	}
}

// Keep the compiler honest about the exact NtCreateFile callback shape used by
// deterministic tests. The production callback is windows.NtCreateFile.
var _ ntCreateFileFunc = windows.NtCreateFile

func TestWindowsSecureStoreLockRejectsReparsePointAndPreservesTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "external-target")
	want := []byte("external data must not be lock metadata\n")
	if err := os.WriteFile(target, want, 0o600); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "review-store", "LOCK")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("creating a file symlink is unavailable: %v", err)
	}

	if _, err := acquireLocalStoreLock(path); err == nil {
		t.Fatal("acquireLocalStoreLock(reparse point) succeeded")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("external target changed: got %q, want %q", got, want)
	}
}

func TestWindowsSecureStoreLockRejectsDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-store", "LOCK")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireLocalStoreLock(path); err == nil {
		t.Fatal("acquireLocalStoreLock(directory) succeeded")
	}
}

func TestWindowsStoreLockUsesExistingInodeAdvisoryTruth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-store", "LOCK")
	held, err := acquireStoreLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireStoreLock(path); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("second Windows advisory acquisition = %v, want ErrConcurrentUpdate", err)
	}
	if evidence, exists := inventoryLock(AuthorityVersionCompact, "", path); !exists || evidence.Status != AuthorityLockOwned || evidence.Owner != nil {
		t.Fatalf("busy Windows lock evidence = %#v, exists=%t", evidence, exists)
	}
	if err := held.release(); err != nil {
		t.Fatal(err)
	}
	if evidence, exists := inventoryLock(AuthorityVersionCompact, "", path); !exists || evidence.Status != AuthorityLockReleased || evidence.Owner != nil {
		t.Fatalf("released Windows lock evidence = %#v, exists=%t", evidence, exists)
	}
}
