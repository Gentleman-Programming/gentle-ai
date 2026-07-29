//go:build windows

package reviewtransaction

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type ntCreateFileFunc func(
	handle *windows.Handle,
	access uint32,
	attributes *windows.OBJECT_ATTRIBUTES,
	status *windows.IO_STATUS_BLOCK,
	allocationSize *int64,
	fileAttributes uint32,
	share uint32,
	disposition uint32,
	options uint32,
	eaBuffer uintptr,
	eaLength uint32,
) error

type queryDosDeviceFunc func(string) ([]string, error)

func secureOpenLocalStoreLock(path string) (*os.File, error) {
	runSecureOpenLocalStoreLockBeforeOpen(path)

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	handle, err := openWindowsStoreLockObject(ntPath(absPath), windows.NtCreateFile, queryWindowsDosDevice)
	if err != nil {
		return nil, err
	}

	fileInfo := new(windows.ByHandleFileInformation)
	if err := windows.GetFileInformationByHandle(handle, fileInfo); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	fileType, err := windows.GetFileType(handle)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	if fileType != windows.FILE_TYPE_DISK || fileInfo.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("review store lock %q is not a regular file", path)
	}

	return os.NewFile(uintptr(handle), path), nil
}

func openWindowsStoreLockObject(objectPath string, createFile ntCreateFileFunc, queryDevice queryDosDeviceFunc) (windows.Handle, error) {
	handle, err := createWindowsStoreLockObject(objectPath, createFile)
	if !errors.Is(err, windows.STATUS_REPARSE_POINT_ENCOUNTERED) {
		return handle, err
	}

	directPath, resolveErr := directLocalDriveObjectPath(objectPath, queryDevice)
	if resolveErr != nil {
		return 0, fmt.Errorf("resolve secure Windows drive after %w: %v", err, resolveErr)
	}
	return createWindowsStoreLockObject(directPath, createFile)
}

func createWindowsStoreLockObject(objectPath string, createFile ntCreateFileFunc) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(objectPath)
	if err != nil {
		return 0, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		Length:     uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		ObjectName: objectName,
		Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = createFile(
		&handle,
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE,
		attributes,
		&status,
		nil,
		windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_OPEN_IF,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT,
		0,
		0,
	)
	return handle, err
}

func directLocalDriveObjectPath(objectPath string, queryDevice queryDosDeviceFunc) (string, error) {
	const dosDevicesPrefix = `\??\`
	if len(objectPath) < len(dosDevicesPrefix)+3 ||
		!strings.HasPrefix(objectPath, dosDevicesPrefix) ||
		!isASCIILetter(objectPath[len(dosDevicesPrefix)]) ||
		objectPath[len(dosDevicesPrefix)+1] != ':' ||
		objectPath[len(dosDevicesPrefix)+2] != '\\' {
		return "", fmt.Errorf("object path %q is not an absolute local drive path", objectPath)
	}

	driveLetter := objectPath[len(dosDevicesPrefix)]
	if driveLetter >= 'a' && driveLetter <= 'z' {
		driveLetter -= 'a' - 'A'
	}
	device := string([]byte{driveLetter, ':'})
	targets, err := queryDevice(device)
	if err != nil {
		return "", fmt.Errorf("QueryDosDevice(%q): %w", device, err)
	}
	if len(targets) != 1 {
		return "", fmt.Errorf("QueryDosDevice(%q) returned %d targets", device, len(targets))
	}

	const devicePrefix = `\Device\`
	target := targets[0]
	if len(target) <= len(devicePrefix) || !strings.EqualFold(target[:len(devicePrefix)], devicePrefix) {
		return "", fmt.Errorf("QueryDosDevice(%q) returned non-device target %q", device, target)
	}
	deviceName := target[len(devicePrefix):]
	if strings.ContainsAny(deviceName, `\/`) || deviceName == "." || deviceName == ".." {
		return "", fmt.Errorf("QueryDosDevice(%q) returned non-direct target %q", device, target)
	}

	return target + objectPath[len(dosDevicesPrefix)+2:], nil
}

func isASCIILetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func queryWindowsDosDevice(device string) ([]string, error) {
	deviceName, err := windows.UTF16PtrFromString(device)
	if err != nil {
		return nil, err
	}
	// QueryDosDevice returns a MULTI_SZ. A full NT path is bounded to 32K UTF-16
	// code units; an oversized or truncated mapping is rejected rather than used.
	buffer := make([]uint16, 32*1024)
	n, err := windows.QueryDosDevice(deviceName, &buffer[0], uint32(len(buffer)))
	if err != nil {
		return nil, err
	}
	if n == 0 || n > uint32(len(buffer)) {
		return nil, fmt.Errorf("QueryDosDevice(%q) returned invalid length %d", device, n)
	}

	var targets []string
	start := 0
	for i, value := range buffer[:n] {
		if value != 0 {
			continue
		}
		if i == start {
			break
		}
		targets = append(targets, windows.UTF16ToString(buffer[start:i]))
		start = i + 1
	}
	if start < int(n) && buffer[n-1] != 0 {
		targets = append(targets, windows.UTF16ToString(buffer[start:n]))
	}
	return targets, nil
}

func ntPath(path string) string {
	if strings.HasPrefix(path, `\\?\UNC\`) {
		return `\??\UNC\` + strings.TrimPrefix(path, `\\?\UNC\`)
	}
	if strings.HasPrefix(path, `\\?\`) {
		return `\??\` + strings.TrimPrefix(path, `\\?\`)
	}
	if strings.HasPrefix(path, `\\`) {
		return `\??\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	return `\??\` + path
}
