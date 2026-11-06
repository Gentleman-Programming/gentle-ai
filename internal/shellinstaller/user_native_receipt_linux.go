//go:build linux && amd64

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

const userNativeReceiptLimit = 65536

// The receipt transports the original result, not publication authority. The
// coordinator must first observe physical producer settlement and then perform
// the existing physical payload authentication before publishing anything.
func userWriteNativeReceipt(root string, native NativeInstallerResult, location *NativeInstallationLocation) error {
	data, err := json.Marshal(struct {
		Result       json.RawMessage             `json:"result"`
		Installation *NativeInstallationLocation `json:"installation"`
	}{native.Details, location})
	if err != nil || len(data) > userNativeReceiptLimit {
		return privateError("output", err)
	}
	_, decoded, err := decodeNativeInstallation(root, data)
	if err != nil || decoded == nil {
		return privateError("output", err)
	}
	if _, err := privateDirectory(root); err != nil {
		return err
	}
	return userToolWrite(filepath.Join(root, "native-installation.json"), data, 0600)
}

func userReadNativeReceipt(ctx context.Context, root string) (native NativeInstallerResult, location *NativeInstallationLocation, err error) {
	if err = nativeRuntimeError(ctx, nil); err != nil {
		return
	}
	reader, err := newUserPNPMInventoryReader(root)
	if err != nil || reader == nil {
		return native, nil, privateError("output", err)
	}
	defer func() { err = errors.Join(err, reader.close()) }()
	file, err := reader.open(filepath.Join(root, "native-installation.json"), unix.O_RDONLY|unix.O_NONBLOCK)
	if err != nil {
		return
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return
	}
	owner := info.Sys().(*syscall.Stat_t)
	if info.Mode() != 0600 || owner.Uid != uint32(os.Getuid()) || owner.Gid != uint32(os.Getgid()) || info.Size() > userNativeReceiptLimit {
		return native, nil, privateError("output", nil)
	}
	data, err := io.ReadAll(io.LimitReader(file, userNativeReceiptLimit+1))
	held, statErr := file.Stat()
	if err != nil || statErr != nil || int64(len(data)) != info.Size() || privateStamp(info) != privateStamp(held) {
		return native, nil, privateError("output", errors.Join(err, statErr))
	}
	if err = errors.Join(ctx.Err(), reader.verify()); err != nil {
		return
	}
	native, location, err = decodeNativeInstallation(root, data)
	if err == nil && location == nil {
		err = privateError("output", nil)
	}
	return
}
