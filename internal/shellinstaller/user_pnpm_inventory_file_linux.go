//go:build linux && amd64

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// userPNPMInventoryReader anchors one sequential inventory to a physical root.
// A nil reader retains the existing guarded implementation on older kernels.
type userPNPMInventoryReader struct {
	root      string
	directory *os.File
	identity  string
	buffer    [65536]byte
}

func newUserPNPMInventoryReader(root string) (*userPNPMInventoryReader, error) {
	info, err := privateDirectory(root)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, root, &unix.OpenHow{
		Flags:   unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_NO_SYMLINKS,
	})
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	directory := os.NewFile(uintptr(fd), root)
	opened, err := directory.Stat()
	if err != nil || privateStamp(info) != privateStamp(opened) {
		return nil, privateError("source", errors.Join(err, directory.Close()))
	}
	return &userPNPMInventoryReader{root: root, directory: directory, identity: privateStamp(info)}, nil
}

func (r *userPNPMInventoryReader) close() error {
	if r == nil {
		return nil
	}
	return r.directory.Close()
}

func (r *userPNPMInventoryReader) verify() error {
	if r == nil {
		return nil
	}
	info, err := privateDirectory(r.root)
	if err != nil || privateStamp(info) != r.identity {
		return privateError("source", err)
	}
	return nil
}

func (r *userPNPMInventoryReader) open(path string, flags uint64) (*os.File, error) {
	rel, err := filepath.Rel(r.root, path)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Openat2(int(r.directory.Fd()), rel, &unix.OpenHow{
		Flags:   flags | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func (r *userPNPMInventoryReader) file(ctx context.Context, path string) (string, error) {
	if r == nil {
		return privateNativeFile(ctx, path, 0, -1, "")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Nonblocking open prevents a substituted FIFO from hanging the inventory.
	file, err := r.open(path, unix.O_RDONLY|unix.O_NONBLOCK)
	if err != nil {
		return "", err
	}
	info, err := file.Stat()
	if err != nil {
		return "", errors.Join(err, file.Close())
	}
	owner := info.Sys().(*syscall.Stat_t)
	mode := info.Mode()
	if owner.Uid != uint32(os.Getuid()) || owner.Gid != uint32(os.Getgid()) ||
		(mode != 0600 && mode != 0644 && mode != 0700 && mode != 0755) || info.Size() > 268435456 {
		return "", privateError("source", file.Close())
	}
	hash := sha256.New()
	var count int64
	for err == nil {
		var n int
		n, err = file.Read(r.buffer[:])
		count += int64(n)
		_, _ = hash.Write(r.buffer[:n])
		if contextErr := ctx.Err(); contextErr != nil {
			err = contextErr
		}
		if count > 268435456 {
			err = privateError("source", nil)
		}
	}
	if errors.Is(err, io.EOF) {
		err = nil
	}
	held, heldErr := file.Stat()
	err = errors.Join(err, heldErr, file.Close())
	if err != nil || privateStamp(info) != privateStamp(held) || count != info.Size() {
		return "", privateError("source", err)
	}
	fresh, err := r.open(path, unix.O_PATH)
	if err != nil {
		return "", err
	}
	current, statErr := fresh.Stat()
	if err = errors.Join(statErr, fresh.Close()); err != nil || privateStamp(info) != privateStamp(current) {
		return "", privateError("source", err)
	}
	return privateStamp(info) + ":" + fmt.Sprintf("%x", hash.Sum(nil)), nil
}
