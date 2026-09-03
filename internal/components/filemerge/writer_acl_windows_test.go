//go:build windows

package filemerge

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWriteFileAtomicFailsClosedWhenParentACLDeniesFileCreation(t *testing.T) {
	dir := createWindowsTestDirectory(t, t.TempDir())
	target := filepath.Join(dir, "config.txt")
	originalContent := []byte("original\n")
	if err := os.WriteFile(target, originalContent, 0o644); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}

	dirHandle := openWindowsTestPath(t, dir, windows.READ_CONTROL|windows.WRITE_DAC, true)
	t.Cleanup(func() {
		if err := windows.CloseHandle(dirHandle); err != nil {
			t.Errorf("CloseHandle(parent) error = %v", err)
		}
	})
	originalParentDACL := readWindowsTestDACL(t, dir)
	originalTargetDACL := readWindowsTestDACL(t, target)
	t.Cleanup(func() {
		restoreWindowsTestDACL(t, dirHandle, originalParentDACL)
		assertWindowsTestDACL(t, dir, originalParentDACL)
	})

	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("GetTokenUser() error = %v", err)
	}
	trustee := windows.TRUSTEE{
		TrusteeForm:  windows.TRUSTEE_IS_SID,
		TrusteeType:  windows.TRUSTEE_IS_USER,
		TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
	}
	deniedACL, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{
		{
			// FILE_WRITE_DATA is FILE_ADD_FILE when the secured object is a directory.
			AccessPermissions: windows.FILE_WRITE_DATA,
			AccessMode:        windows.DENY_ACCESS,
			Trustee:           trustee,
		},
		{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.GRANT_ACCESS,
			Trustee:           trustee,
		},
	}, nil)
	if err != nil {
		t.Fatalf("ACLFromEntries(deny FILE_ADD_FILE) error = %v", err)
	}
	if err := windows.SetSecurityInfo(
		dirHandle,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		deniedACL,
		nil,
	); err != nil {
		t.Fatalf("SetSecurityInfo(deny FILE_ADD_FILE) error = %v", err)
	}
	assertWindowsTestDACL(t, target, originalTargetDACL)

	entriesBefore := readWindowsTestDirectory(t, dir)
	probe := filepath.Join(dir, "namespace-probe")
	probeFile, probeErr := os.OpenFile(probe, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if probeErr == nil {
		_ = probeFile.Close()
		_ = os.Remove(probe)
		t.Fatal("OpenFile(namespace probe) error = nil, want parent ACL to deny file creation")
	}
	if !errors.Is(probeErr, os.ErrPermission) {
		t.Fatalf("OpenFile(namespace probe) error = %v, want os.ErrPermission", probeErr)
	}
	if entriesAfterProbe := readWindowsTestDirectory(t, dir); !slices.Equal(entriesAfterProbe, entriesBefore) {
		t.Fatalf("directory entries after denied namespace probe = %v, want %v", entriesAfterProbe, entriesBefore)
	}

	parentDACLBefore := readWindowsTestDACL(t, dir)
	contentBefore, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("ReadFile(target before write) error = %v", readErr)
	}
	metadataBefore := readWindowsTestTargetMetadata(t, target)

	result, err := WriteFileAtomic(target, []byte("replacement\n"), 0o600)
	if err == nil {
		t.Fatal("WriteFileAtomic() error = nil, want parent ACL failure")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("WriteFileAtomic() error = %v, want os.ErrPermission", err)
	}
	if result != (WriteResult{}) {
		t.Fatalf("WriteFileAtomic() result = %+v, want no change", result)
	}

	metadataAfter := readWindowsTestTargetMetadata(t, target)
	contentAfter, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("ReadFile(target after write) error = %v", readErr)
	}
	if !bytes.Equal(contentAfter, contentBefore) || !bytes.Equal(contentAfter, originalContent) {
		t.Fatalf("target content after failure = %q, want unchanged %q", contentAfter, originalContent)
	}
	if metadataAfter != metadataBefore {
		t.Fatalf("target metadata changed after failure:\n got  %+v\n want %+v", metadataAfter, metadataBefore)
	}
	assertWindowsTestDACL(t, target, originalTargetDACL)
	assertWindowsTestDACL(t, dir, parentDACLBefore)

	entriesAfter := readWindowsTestDirectory(t, dir)
	if !slices.Equal(entriesAfter, entriesBefore) {
		t.Fatalf("directory entries after failure = %v, want unchanged %v", entriesAfter, entriesBefore)
	}
	for _, name := range entriesAfter {
		if strings.HasPrefix(name, ".gentle-ai-") {
			t.Fatalf("temporary artifact %q remains after failure", name)
		}
	}
}

func createWindowsTestDirectory(t *testing.T, root string) string {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("GetTokenUser() error = %v", err)
	}
	dacl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
		},
	}}, nil)
	if err != nil {
		t.Fatalf("ACLFromEntries(fixture) error = %v", err)
	}
	descriptor, err := windows.NewSecurityDescriptor()
	if err != nil {
		t.Fatalf("NewSecurityDescriptor() error = %v", err)
	}
	if err := descriptor.SetDACL(dacl, true, false); err != nil {
		t.Fatalf("SECURITY_DESCRIPTOR.SetDACL() error = %v", err)
	}
	if err := descriptor.SetControl(windows.SE_DACL_PROTECTED, windows.SE_DACL_PROTECTED); err != nil {
		t.Fatalf("SECURITY_DESCRIPTOR.SetControl() error = %v", err)
	}

	dir := filepath.Join(root, "acl-denied")
	dirPtr, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		t.Fatalf("UTF16PtrFromString(%q) error = %v", dir, err)
	}
	attributes := windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: descriptor,
	}
	if err := windows.CreateDirectory(dirPtr, &attributes); err != nil {
		t.Fatalf("CreateDirectory(fixture) error = %v", err)
	}
	return dir
}

type windowsTestDACLState struct {
	control windows.SECURITY_DESCRIPTOR_CONTROL
	bytes   []byte
}

type windowsTestACLHeader struct {
	revision uint8
	reserved uint8
	size     uint16
	aceCount uint16
	padding  uint16
}

func readWindowsTestDACL(t *testing.T, path string) windowsTestDACLState {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo(%q) error = %v", path, err)
	}
	control, _, err := descriptor.Control()
	if err != nil {
		t.Fatalf("SECURITY_DESCRIPTOR.Control(%q) error = %v", path, err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatalf("SECURITY_DESCRIPTOR.DACL(%q) error = %v", path, err)
	}
	if dacl == nil {
		t.Fatalf("SECURITY_DESCRIPTOR.DACL(%q) = nil, want a concrete DACL", path)
	}
	header := (*windowsTestACLHeader)(unsafe.Pointer(dacl))
	if header.size < uint16(unsafe.Sizeof(*header)) {
		t.Fatalf("DACL(%q) size = %d, want at least %d", path, header.size, unsafe.Sizeof(*header))
	}
	daclBytes := bytes.Clone(unsafe.Slice((*byte)(unsafe.Pointer(dacl)), int(header.size)))
	return windowsTestDACLState{control: control, bytes: daclBytes}
}

func windowsTestACL(t *testing.T, state windowsTestDACLState) *windows.ACL {
	t.Helper()
	if len(state.bytes) < int(unsafe.Sizeof(windowsTestACLHeader{})) {
		t.Fatalf("saved DACL size = %d, want a complete ACL header", len(state.bytes))
	}
	return (*windows.ACL)(unsafe.Pointer(&state.bytes[0]))
}

func windowsTestDACLInformation(control windows.SECURITY_DESCRIPTOR_CONTROL) windows.SECURITY_INFORMATION {
	information := windows.SECURITY_INFORMATION(
		windows.DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION,
	)
	if control&windows.SE_DACL_PROTECTED != 0 {
		information = windows.SECURITY_INFORMATION(
			windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION,
		)
	}
	return information
}

func restoreWindowsTestDACL(t *testing.T, handle windows.Handle, state windowsTestDACLState) {
	t.Helper()
	if err := windows.SetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windowsTestDACLInformation(state.control),
		nil,
		nil,
		windowsTestACL(t, state),
		nil,
	); err != nil {
		t.Fatalf("restore original parent DACL error = %v", err)
	}
}

func assertWindowsTestDACL(t *testing.T, path string, want windowsTestDACLState) {
	t.Helper()
	got := readWindowsTestDACL(t, path)
	if got.control != want.control || !bytes.Equal(got.bytes, want.bytes) {
		t.Fatalf("DACL changed for %q:\n got  control=%#x bytes=%x\n want control=%#x bytes=%x", path, got.control, got.bytes, want.control, want.bytes)
	}
}

func openWindowsTestPath(t *testing.T, path string, access uint32, directory bool) windows.Handle {
	t.Helper()
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("UTF16PtrFromString(%q) error = %v", path, err)
	}
	attributes := uint32(windows.FILE_ATTRIBUTE_NORMAL)
	if directory {
		attributes = windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	handle, err := windows.CreateFile(
		pathPtr,
		access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		attributes,
		0,
	)
	if err != nil {
		t.Fatalf("CreateFile(%q) error = %v", path, err)
	}
	return handle
}

type windowsTestTargetMetadata struct {
	attributes         uint32
	creationTime       windows.Filetime
	lastWriteTime      windows.Filetime
	volumeSerialNumber uint32
	fileSizeHigh       uint32
	fileSizeLow        uint32
	numberOfLinks      uint32
	fileIndexHigh      uint32
	fileIndexLow       uint32
}

func readWindowsTestTargetMetadata(t *testing.T, path string) windowsTestTargetMetadata {
	t.Helper()
	handle := openWindowsTestPath(t, path, windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL, false)
	defer windows.CloseHandle(handle)
	var metadata windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &metadata); err != nil {
		t.Fatalf("GetFileInformationByHandle(%q) error = %v", path, err)
	}
	return windowsTestTargetMetadata{
		attributes:         metadata.FileAttributes,
		creationTime:       metadata.CreationTime,
		lastWriteTime:      metadata.LastWriteTime,
		volumeSerialNumber: metadata.VolumeSerialNumber,
		fileSizeHigh:       metadata.FileSizeHigh,
		fileSizeLow:        metadata.FileSizeLow,
		numberOfLinks:      metadata.NumberOfLinks,
		fileIndexHigh:      metadata.FileIndexHigh,
		fileIndexLow:       metadata.FileIndexLow,
	}
}

func readWindowsTestDirectory(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", path, err)
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}
