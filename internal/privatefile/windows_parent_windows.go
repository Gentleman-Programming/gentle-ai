//go:build windows

package privatefile

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// openWindowsPrivateParent opens only existing physical child directories of
// trustedRoot. The caller owns trustedRoot and must close the returned handle.
// It neither opens nor creates a report leaf and makes no ACL claim.
func openWindowsPrivateParent(trustedRoot windows.Handle, components []string) (result windows.Handle, err error) {
	if trustedRoot == 0 || trustedRoot == windows.InvalidHandle || len(components) == 0 {
		return 0, fmt.Errorf("invalid private report parent root or components: %w", ErrUnsafeParent)
	}
	parent := trustedRoot
	var held []windows.Handle
	defer func() {
		for i := len(held) - 1; i >= 0; i-- {
			if closeErr := windows.CloseHandle(held[i]); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
		}
		if err != nil && result != 0 {
			err = errors.Join(err, windows.CloseHandle(result))
			result = 0
		}
	}()

	for _, component := range components {
		if !windowsPrivateComponentSafe(component) {
			return 0, fmt.Errorf("unsafe private report parent component %q: %w", component, ErrUnsafeParent)
		}
		child, openErr := openWindowsPrivateDirectoryAt(parent, component)
		if openErr != nil {
			return 0, fmt.Errorf("open private report parent component %q: %w", component, errors.Join(ErrUnsafeParent, openErr))
		}
		// Retain every ancestor until the entire chain is open. The original
		// trusted root is borrowed and never closed here.
		held = append(held, child)
		parent = child
	}
	result = held[len(held)-1]
	held = held[:len(held)-1]
	return result, nil
}

func windowsPrivateComponentSafe(name string) bool {
	if name == "" || name == "." || name == ".." || strings.TrimRight(name, ". ") != name ||
		strings.ContainsAny(name, `\/:<>"|?*`) {
		return false
	}
	for _, char := range name {
		if char < 32 {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	return len(base) != 4 || (base[:3] != "COM" && base[:3] != "LPT") || base[3] < '1' || base[3] > '9'
}

func openWindowsPrivateDirectoryAt(parent windows.Handle, name string) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: parent,
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var child windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&child, windows.FILE_GENERIC_READ, attributes, &status,
		nil, windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_DIRECTORY_FILE,
		0, 0)
	if err != nil {
		return 0, err
	}
	var info windows.ByHandleFileInformation
	infoErr := windows.GetFileInformationByHandle(child, &info)
	fileType, typeErr := windows.GetFileType(child)
	if infoErr != nil || typeErr != nil || fileType != windows.FILE_TYPE_DISK ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		_ = windows.CloseHandle(child)
		return 0, fmt.Errorf("private report parent is not a physical directory (attributes=%#x, info=%v, type=%v)", info.FileAttributes, infoErr, typeErr)
	}
	return child, nil
}
