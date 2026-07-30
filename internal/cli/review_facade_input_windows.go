//go:build windows

package cli

import (
	"context"
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

func openFacadeArtifactManifestInput(ctx context.Context, path string) (*os.File, func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if path != "-" {
		return openFacadeArtifactManifestRegularFile(ctx, path)
	}
	process := windows.CurrentProcess()
	var duplicate windows.Handle
	raw, err := os.Stdin.SyscallConn()
	if err != nil {
		return nil, nil, err
	}
	var duplicateErr error
	if err := raw.Control(func(handle uintptr) {
		duplicateErr = windows.DuplicateHandle(process, windows.Handle(handle), process, &duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS)
	}); err != nil || duplicateErr != nil {
		if duplicate != 0 {
			_ = windows.CloseHandle(duplicate)
		}
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, duplicateErr
	}
	file := os.NewFile(uintptr(duplicate), os.Stdin.Name())
	if file == nil {
		_ = windows.CloseHandle(duplicate)
		return nil, nil, errors.New("duplicate facade stdin")
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, func() error { return nil }, nil
}

func openFacadeArtifactManifestRegularFile(ctx context.Context, path string) (*os.File, func() error, error) {
	if facadeArtifactManifestWindowsDevicePath(path) {
		return nil, nil, errFacadeArtifactManifestInputNotRegular
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, nil, err
	}
	attributes, err := windows.GetFileAttributes(name)
	if err != nil {
		return nil, nil, err
	}
	if attributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_DEVICE) != 0 {
		return nil, nil, errFacadeArtifactManifestInputNotRegular
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OVERLAPPED,
		0,
	)
	if err != nil {
		return nil, nil, err
	}
	closeOnError := func(err error) (*os.File, func() error, error) {
		_ = windows.CloseHandle(handle)
		return nil, nil, err
	}
	fileType, err := windows.GetFileType(handle)
	if err != nil {
		return closeOnError(err)
	}
	if fileType != windows.FILE_TYPE_DISK {
		return closeOnError(errFacadeArtifactManifestInputNotRegular)
	}
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil {
		return closeOnError(errFacadeArtifactManifestInputNotRegular)
	}
	if information.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_DEVICE) != 0 {
		return closeOnError(errFacadeArtifactManifestInputNotRegular)
	}
	if err := ctx.Err(); err != nil {
		return closeOnError(err)
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		return closeOnError(errors.New("open facade artifact manifest input"))
	}
	return file, func() error { return nil }, nil
}

func facadeArtifactManifestWindowsDevicePath(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	if strings.HasPrefix(normalized, `\\.\`) ||
		strings.HasPrefix(normalized, `\\?\pipe\`) ||
		strings.HasPrefix(normalized, `\\?\globalroot\`) {
		return true
	}
	if !strings.HasPrefix(normalized, `\\`) {
		return facadeArtifactManifestWindowsReservedName(normalized)
	}
	parts := strings.Split(strings.TrimPrefix(normalized, `\\`), `\`)
	if len(parts) > 1 && parts[1] == "pipe" {
		return true
	}
	if len(parts) > 3 && parts[0] == "?" && parts[1] == "unc" && parts[3] == "pipe" {
		return true
	}
	return facadeArtifactManifestWindowsReservedName(normalized)
}

func facadeArtifactManifestWindowsReservedName(path string) bool {
	base := path
	if separator := strings.LastIndex(base, `\`); separator >= 0 {
		base = base[separator+1:]
	}
	base = strings.TrimRight(base, " .")
	if separator := strings.IndexAny(base, ".:"); separator >= 0 {
		base = base[:separator]
	}
	base = strings.TrimRight(base, " ")
	switch base {
	case "con", "prn", "aux", "nul", "clock$", "conin$", "conout$",
		"com¹", "com²", "com³", "lpt¹", "lpt²", "lpt³":
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) && base[3] >= '1' && base[3] <= '9'
}

func cancelFacadeArtifactManifestInput(file *os.File) {
	_ = windows.CancelIoEx(windows.Handle(file.Fd()), nil)
}
