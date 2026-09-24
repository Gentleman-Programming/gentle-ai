//go:build windows

package privatefile

import (
	"fmt"
	"strings"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

// openWindowsManagedRootCandidate consumes a KnownFolderPath string, not a
// caller-selected destination. Its returned handle and mapping are UNTRUSTED
// candidate provenance, never locality, ownership, ACL or writer authority.
// The caller owns the handle and must close it. No children are opened here.
func openWindowsManagedRootCandidate(path string, query func(string) ([]string, error), open func(string) (windows.Handle, error)) (windows.Handle, string, error) {
	refuse := func(err error) (windows.Handle, string, error) { return 0, "", err }
	if len(path) < 4 || !utf8.ValidString(path) ||
		!((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) ||
		path[1:3] != `:\` || strings.Contains(path, "/") {
		return refuse(fmt.Errorf("invalid known-folder drive path: %w", ErrInvalidDestination))
	}
	for _, part := range strings.Split(path[3:], `\`) {
		if !windowsPrivateComponentSafe(part) || !windowsPrivateLiteralNameSafe(part) {
			return refuse(fmt.Errorf("ambiguous known-folder component: %w", ErrInvalidDestination))
		}
	}
	drive := strings.ToUpper(path[:2])
	targets, err := query(drive)
	if err != nil || len(targets) != 1 || !windowsManagedDirectVolume(targets[0]) {
		return refuse(fmt.Errorf("QueryDosDevice(%q) has no single direct volume: %v", drive, err))
	}
	h, err := open(targets[0])
	if err != nil {
		if h != 0 && h != windows.InvalidHandle {
			_ = windows.CloseHandle(h)
		}
		return refuse(fmt.Errorf("open direct volume root: %w", err))
	}
	if h == 0 || h == windows.InvalidHandle {
		return refuse(fmt.Errorf("invalid direct volume handle"))
	}
	var info windows.ByHandleFileInformation
	infoErr := windows.GetFileInformationByHandle(h, &info)
	kind, typeErr := windows.GetFileType(h)
	if infoErr != nil || typeErr != nil || kind != windows.FILE_TYPE_DISK ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(h)
		return refuse(fmt.Errorf("direct volume root is not a non-reparse disk directory (attributes=%#x, info=%v, type=%v)", info.FileAttributes, infoErr, typeErr))
	}
	return h, targets[0], nil
}

func windowsManagedDirectVolume(target string) bool {
	const prefix = `\Device\HarddiskVolume`
	if !strings.HasPrefix(target, prefix) {
		return false
	}
	number := target[len(prefix):]
	if len(number) == 0 || number[0] < '1' || number[0] > '9' {
		return false
	}
	for i := 1; i < len(number); i++ {
		if number[i] < '0' || number[i] > '9' {
			return false
		}
	}
	return true
}

// QueryDosDevice returns a MULTI_SZ, including the final empty terminator.
// Reject malformed/truncated output rather than silently discarding targets.
func queryWindowsManagedDevice(drive string) ([]string, error) {
	name, err := windows.UTF16PtrFromString(drive)
	if err != nil {
		return nil, err
	}
	buffer := make([]uint16, 32*1024)
	n, err := windows.QueryDosDevice(name, &buffer[0], uint32(len(buffer)))
	if err != nil {
		return nil, err
	}
	if n < 3 || n > uint32(len(buffer)) || buffer[n-1] != 0 || buffer[n-2] != 0 {
		return nil, fmt.Errorf("malformed QueryDosDevice output")
	}
	var targets []string
	start := 0
	for i := 0; i < int(n)-1; i++ {
		if buffer[i] != 0 {
			continue
		}
		if i == start {
			return nil, fmt.Errorf("empty QueryDosDevice target")
		}
		targets = append(targets, windows.UTF16ToString(buffer[start:i]))
		start = i + 1
	}
	if start != int(n)-1 {
		return nil, fmt.Errorf("malformed QueryDosDevice terminator")
	}
	return targets, nil
}

func openWindowsManagedVolume(target string) (windows.Handle, error) {
	name, err := windows.NewNTUnicodeString(target + `\`)
	if err != nil {
		return 0, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		Length:     uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		ObjectName: name,
		Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ, attributes, &status,
		nil, windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		windows.FILE_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT,
		0, 0)
	return handle, err
}
