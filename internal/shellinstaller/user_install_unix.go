//go:build linux || darwin

package shellinstaller

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func userSelectionPath(path string) bool {
	return privateHierarchyPath(path) && strings.IndexFunc(path, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/_.-", r))
	}) == -1
}

func userIdentity(path string) (string, error) {
	info, err := privateDirectory(path)
	if err != nil {
		return "", err
	}
	st := info.Sys().(*syscall.Stat_t)
	return fmt.Sprintf("%d:%d:%d:%v", st.Dev, st.Ino, st.Uid, info.Mode()), nil
}

// This is a consent preimage, not a replacement for recoverable snapshots.
// Inventory both trees twice at inspection; never follow an escaping symlink.
func userTreeStamp(root string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	hash := sha256.New()
	var entries, total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		entries++
		if entries > 250000 {
			return errors.New("selection inventory exceeds entry bound")
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		before := info.Sys().(*syscall.Stat_t)
		if int(before.Uid) != os.Getuid() {
			return errors.New("selection contains foreign owner")
		}
		content := ""
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			target, err := filepath.EvalSymlinks(path)
			if err != nil || !strings.HasPrefix(target, root+"/") {
				return errors.New("selection symlink escapes physical tree")
			}
			content = link
		case info.IsDir():
			if info.Mode().Perm()&0022 != 0 {
				return errors.New("selection directory is writable by others")
			}
		case info.Mode().IsRegular():
			total += info.Size()
			if info.Mode().Perm()&0022 != 0 || info.Size() > 32<<20 || total > 1024<<20 {
				return errors.New("selection file permissions or byte bound")
			}
			fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
			if err != nil {
				return err
			}
			file := os.NewFile(uintptr(fd), path)
			opened, statErr := file.Stat()
			if statErr != nil || !os.SameFile(info, opened) {
				return errors.Join(errors.New("selection opened preimage differs"), statErr, file.Close())
			}
			bytesHash := sha256.New()
			n, readErr := io.Copy(bytesHash, io.LimitReader(file, (32<<20)+1))
			if closeErr := file.Close(); readErr != nil || closeErr != nil || n != info.Size() {
				return errors.Join(errors.New("selection file read differs"), readErr, closeErr)
			}
			content = fmt.Sprintf("%x", bytesHash.Sum(nil))
		default:
			return errors.New("selection contains nonregular object")
		}
		after, err := os.Lstat(path)
		if err != nil {
			return err
		}
		fresh := after.Sys().(*syscall.Stat_t)
		beforeMtime, beforeCtime := privateStatTimes(before)
		freshMtime, freshCtime := privateStatTimes(fresh)
		if !os.SameFile(info, after) || before.Mode != fresh.Mode || before.Uid != fresh.Uid || before.Size != fresh.Size || beforeMtime != freshMtime || beforeCtime != freshCtime {
			return errors.New("selection changed during inspection")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%q:%d:%d:%d:%d:%d:%v:%v:%q\n", relative, before.Dev, before.Ino, before.Uid, before.Mode, before.Size, beforeMtime, beforeCtime, content)
		return nil
	})
	return fmt.Sprintf("%x", hash.Sum(nil)), err
}

func userInteractive(stdin io.Reader) (bool, error) {
	file, ok := stdin.(*os.File)
	if !ok {
		return false, nil
	}
	_, err := userTermios(int(file.Fd()))
	if errors.Is(err, unix.ENOTTY) {
		return false, nil
	}
	return err == nil, err
}

func userFinish(ctx context.Context, result UserInstallResult, cause error, workspace string, identity os.FileInfo, dest string) (UserInstallResult, error) {
	cleanupErr := privateCleanup(workspace, identity)
	cause = errors.Join(cause, cleanupErr, ctx.Err())
	if cause == nil {
		return result, nil
	}
	failure := privateFailure(dest, cause)
	if cleanupErr != nil {
		failure = privateError("uncertain", cause)
	}
	failure.Workspace, failure.Destination = workspace, dest
	return UserInstallResult{}, failure
}

func userInstallFinish(ctx context.Context, result UserInstallResult, cause error, workspace string, identity os.FileInfo, req UserInstallRequest, sharedProvisioningStarted bool) (UserInstallResult, error) {
	if sharedProvisioningStarted && cause != nil {
		failure := privateError("uncertain", cause)
		failure.Workspace, failure.Destination = workspace, req.Destination
		return UserInstallResult{}, failure
	}
	return userFinish(ctx, result, cause, workspace, identity, req.Destination)
}

func userRecoveryID(path string) (string, error) {
	info, err := privateDirectory(path)
	if err != nil {
		return "", err
	}
	stat := info.Sys().(*syscall.Stat_t)
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}

type userSyncedFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

func userToolWrite(path string, data []byte, mode os.FileMode) error {
	return userWriteWithSync(path, data, mode, func(path string, flags int, mode os.FileMode) (userSyncedFile, error) {
		return os.OpenFile(path, flags, mode)
	})
}

func userWriteWithSync(path string, data []byte, mode os.FileMode, open func(string, int, os.FileMode) (userSyncedFile, error)) error {
	file, err := open(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	n, writeErr := file.Write(data)
	if n != len(data) {
		writeErr = errors.Join(writeErr, io.ErrShortWrite)
	}
	err = errors.Join(writeErr, userSync(file), file.Close())
	if err != nil {
		return privateError("source", err)
	}
	directory, err := open(filepath.Dir(path), os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	return errors.Join(userSync(directory), directory.Close())
}

func userDirectorySync(path string) error {
	directory, err := os.OpenFile(path, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	return errors.Join(userSync(directory), directory.Close())
}

func userSourceFile(ctx context.Context, path string, size int64, pin string) (string, error) {
	info, err := privatePhysical(path)
	if err != nil {
		return "", err
	}
	mode := info.Mode()
	if mode != 0600 && mode != 0644 && mode != 0700 && mode != 0755 {
		return "", privateError("source", fmt.Errorf("supplier mode refused: name=%q mode=%#o", filepath.Base(path), mode.Perm()))
	}
	stamp, err := privateNativeFile(ctx, path, mode, size, pin)
	if err != nil {
		return "", fmt.Errorf("supplier file %q independent readback: %w", filepath.Base(path), err)
	}
	return stamp, nil
}

// Distribution provenance belongs to the separately trusted source build, not
// this mutable checksum. It witnesses cooperative preimages, not loaded bytes.
func userSupervisorSHA(ctx context.Context, source string) (string, error) {
	info, err := privateForeignPhysical(source)
	if err != nil || ctx.Err() != nil || info.Size() > 268435456 || info.Mode().Perm()&0022 != 0 || info.Mode().Perm()&0111 == 0 {
		return "", privateError("source", errors.Join(err, ctx.Err()))
	}
	for current := source; ; current = filepath.Dir(current) {
		st, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		owner := st.Sys().(*syscall.Stat_t).Uid
		trustedTmp := current == "/tmp" && owner == 0 && st.Mode()&os.ModeSticky != 0
		if (owner != 0 && owner != uint32(os.Getuid())) || (st.Mode().Perm()&0022 != 0 && !trustedTmp) {
			return "", privateError("source", fmt.Errorf("supervisor ancestor refused: path=%q owner=%d mode=%#o", current, owner, st.Mode().Perm()))
		}
		if current == "/" {
			break
		}
	}
	data, err := os.ReadFile(source)
	fresh, freshErr := privateForeignPhysical(source)
	if err != nil || freshErr != nil || privateStamp(info) != privateStamp(fresh) || ctx.Err() != nil {
		return "", privateError("preimage", errors.Join(err, freshErr, ctx.Err()))
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

func userCopySupervisor(ctx context.Context, source, dest string) (string, error) {
	before, err := userSupervisorSHA(ctx, source)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(source)
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != before {
		return "", privateError("preimage", err)
	}
	return before, userToolWrite(dest, data, 0700)
}

// Foreground uses the parent's controlling-tty descriptor. A pipe is not a tty;
// descriptor errors and background callers must fail before executing Node.
func userForeground(cmd *exec.Cmd, stdin io.Reader) (func() error, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	unchanged := func() error { return nil }
	file, ok := stdin.(*os.File)
	if !ok {
		return unchanged, nil
	}
	fd := int(file.Fd())
	if _, err := userTermios(fd); err != nil {
		if errors.Is(err, unix.ENOTTY) {
			return unchanged, nil
		}
		return nil, err
	}
	group, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if err != nil {
		return nil, err
	}
	if group != syscall.Getpgrp() {
		return nil, errors.New("caller does not own the foreground terminal")
	}
	cmd.SysProcAttr.Foreground, cmd.SysProcAttr.Ctty = true, fd
	return func() error {
		ignored := signal.Ignored(syscall.SIGTTOU)
		signal.Ignore(syscall.SIGTTOU)
		if !ignored {
			defer signal.Reset(syscall.SIGTTOU)
		}
		return unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, group)
	}, nil
}

// userReapGroup kills the owned process group after Wait and returns nil only
// once the kernel reports no member remains; every other outcome is uncertain.
func userReapGroup(pid int, waitErr error) error {
	killErr := syscall.Kill(-pid, syscall.SIGKILL)
	if killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
		return privateError("uncertain", errors.Join(waitErr, killErr))
	}
	for attempts := 0; attempts < 20; attempts++ {
		if probeErr := syscall.Kill(-pid, 0); errors.Is(probeErr, syscall.ESRCH) {
			return nil
		} else if probeErr != nil {
			return privateError("uncertain", errors.Join(waitErr, probeErr))
		}
		<-time.After(25 * time.Millisecond)
	}
	return privateError("uncertain", errors.Join(waitErr, errors.New("owned process group remains after kill/wait")))
}

// Fixed acquisition pins, not mutable latest or independently published checksums.
// Only this exact regular member is copied; no other archive path is materialized.
func userToolMember(ctx context.Context, data []byte, pin, member string) ([]byte, error) {
	if ctx.Err() != nil || len(data) > 32<<20 || fmt.Sprintf("%x", sha256.Sum256(data)) != pin || filepath.IsAbs(member) || filepath.Clean(member) != member || strings.HasPrefix(member, "../") {
		return nil, privateError("source", ctx.Err())
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	decoded, err := io.ReadAll(io.LimitReader(gz, (32<<20)+1))
	if err = errors.Join(err, gz.Close(), ctx.Err()); err != nil || len(decoded) > 32<<20 {
		return nil, privateError("source", err)
	}
	reader := tar.NewReader(bytes.NewReader(decoded))
	var selected []byte
	for i := 0; ; i++ {
		h, err := reader.Next()
		if err == io.EOF && len(selected) > 0 && ctx.Err() == nil {
			return selected, nil
		}
		if err != nil || i >= 4096 || ctx.Err() != nil {
			return nil, privateError("source", errors.Join(err, ctx.Err()))
		}
		if h.Name != member {
			continue
		}
		if selected != nil || (h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA) || h.Size <= 0 || h.Size > 32<<20 {
			return nil, privateError("source", nil)
		}
		selected, err = io.ReadAll(reader)
		if err != nil || int64(len(selected)) != h.Size {
			return nil, privateError("source", err)
		}
	}
}

type userToolSource struct {
	name, repo, tag, stem, pin string
	size                       int64
}
