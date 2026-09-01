//go:build aix || android || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cli

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"golang.org/x/sys/unix"
)

const maxCompatibilityAtomicFileSize = 16 << 20

type unixCompatibilityDirectoryWriter struct {
	root   string
	rootFD int
}

func newCompatibilityDirectoryWriter(homeDir, skillDir string) (compatibilityDirectoryWriter, error) {
	homeFD, err := unix.Open(homeDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open compatibility home directory %q: %w", homeDir, err)
	}
	defer unix.Close(homeFD)

	agentsFD, err := openPhysicalCompatibilityDirectoryAt(homeFD, ".agents")
	if err != nil {
		return nil, err
	}
	defer unix.Close(agentsFD)

	skillsFD, err := openPhysicalCompatibilityDirectoryAt(agentsFD, "skills")
	if err != nil {
		return nil, err
	}
	return &unixCompatibilityDirectoryWriter{root: skillDir, rootFD: skillsFD}, nil
}

func openPhysicalCompatibilityDirectoryAt(parentFD int, name string) (int, error) {
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("open physical compatibility directory %q: %w", name, err)
	}
	return fd, nil
}

func (w *unixCompatibilityDirectoryWriter) Close() error {
	return unix.Close(w.rootFD)
}

func (w *unixCompatibilityDirectoryWriter) Write(path string, content []byte, perm fs.FileMode) (filemerge.WriteResult, error) {
	parts, err := compatibilityPathParts(w.root, path)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	dirFD, err := w.openParent(parts[:len(parts)-1])
	if err != nil {
		return filemerge.WriteResult{}, err
	}
	if dirFD != w.rootFD {
		defer unix.Close(dirFD)
	}

	name := parts[len(parts)-1]
	existing, exists, err := readPhysicalCompatibilityFile(dirFD, name)
	if err != nil {
		return filemerge.WriteResult{}, err
	}
	if exists && bytes.Equal(existing, content) {
		return filemerge.WriteResult{}, nil
	}

	tmpName, tmp, err := createCompatibilityTempFile(dirFD, perm)
	if err != nil {
		return filemerge.WriteResult{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = unix.Unlinkat(dirFD, tmpName, 0)
		}
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return filemerge.WriteResult{}, fmt.Errorf("write compatibility temp file %q: %w", path, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return filemerge.WriteResult{}, fmt.Errorf("set compatibility temp file permissions for %q: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return filemerge.WriteResult{}, fmt.Errorf("sync compatibility temp file %q: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return filemerge.WriteResult{}, fmt.Errorf("close compatibility temp file %q: %w", path, err)
	}
	if err := unix.Renameat(dirFD, tmpName, dirFD, name); err != nil {
		return filemerge.WriteResult{}, fmt.Errorf("replace compatibility destination %q: %w", path, err)
	}
	cleanup = false

	result := filemerge.WriteResult{Changed: true, Created: !exists}
	readBack, readBackExists, err := readPhysicalCompatibilityFile(dirFD, name)
	if err != nil {
		return result, err
	}
	if !readBackExists || !bytes.Equal(readBack, content) {
		// refusal:by-design world-action: publication did not produce the staged bytes at the anchored destination.
		return result, fmt.Errorf("replace compatibility destination %q: the replacement did not land", path)
	}
	if err := unix.Fsync(dirFD); err != nil {
		return result, fmt.Errorf("sync compatibility parent directory for %q: %w", path, err)
	}
	return result, nil
}

func (w *unixCompatibilityDirectoryWriter) Remove(path string) (bool, error) {
	parts, err := compatibilityPathParts(w.root, path)
	if err != nil {
		return false, err
	}

	dirFD, err := w.openParent(parts[:len(parts)-1])
	if err != nil {
		return false, err
	}
	if dirFD != w.rootFD {
		defer unix.Close(dirFD)
	}

	name := parts[len(parts)-1]
	exists, err := physicalCompatibilityFileExists(dirFD, name)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	if err := unix.Unlinkat(dirFD, name, 0); err != nil {
		return false, fmt.Errorf("remove compatibility destination %q: %w", path, err)
	}
	if err := unix.Fsync(dirFD); err != nil {
		return true, fmt.Errorf("sync compatibility parent directory for %q: %w", path, err)
	}
	return true, nil
}

func compatibilityPathParts(root, path string) ([]string, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		// refusal:by-design operator-knowledge: embedded compatibility destinations must stay below the anchored skills root.
		return nil, fmt.Errorf("compatibility destination %q escapes %q", path, root)
	}
	return strings.Split(relative, string(filepath.Separator)), nil
}

func (w *unixCompatibilityDirectoryWriter) openParent(parts []string) (int, error) {
	currentFD := w.rootFD
	for _, part := range parts {
		nextFD, err := openPhysicalCompatibilityDirectoryAt(currentFD, part)
		if err != nil && !errors.Is(err, unix.ENOENT) {
			if currentFD != w.rootFD {
				_ = unix.Close(currentFD)
			}
			return -1, err
		}
		if errors.Is(err, unix.ENOENT) {
			if err := unix.Mkdirat(currentFD, part, 0o700); err != nil && err != unix.EEXIST {
				if currentFD != w.rootFD {
					_ = unix.Close(currentFD)
				}
				return -1, fmt.Errorf("create physical compatibility directory %q: %w", part, err)
			}
			nextFD, err = openPhysicalCompatibilityDirectoryAt(currentFD, part)
			if err != nil {
				if currentFD != w.rootFD {
					_ = unix.Close(currentFD)
				}
				return -1, err
			}
		}
		if currentFD != w.rootFD {
			_ = unix.Close(currentFD)
		}
		currentFD = nextFD
	}
	return currentFD, nil
}

func physicalCompatibilityFileExists(dirFD int, name string) (bool, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(dirFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if err == unix.ENOENT {
			return false, nil
		}
		return false, fmt.Errorf("stat physical compatibility destination %q: %w", name, err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		// refusal:by-design world-action: a non-regular destination must be replaced before a safe atomic refresh can continue.
		return false, fmt.Errorf("compatibility destination %q must be a regular file", name)
	}
	return true, nil
}

func readPhysicalCompatibilityFile(dirFD int, name string) ([]byte, bool, error) {
	fd, err := unix.Openat(dirFD, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err == unix.ENOENT {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("open physical compatibility destination %q: %w", name, err)
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("stat compatibility destination %q: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		// refusal:by-design world-action: a non-regular destination must be replaced before a safe atomic refresh can continue.
		return nil, false, fmt.Errorf("compatibility destination %q must be a regular file", name)
	}
	if info.Size() > maxCompatibilityAtomicFileSize {
		// refusal:by-design world-action: an oversized destination cannot be safely compared before replacement.
		return nil, false, fmt.Errorf("compatibility destination %q exceeds max atomic compare size %d bytes", name, maxCompatibilityAtomicFileSize)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxCompatibilityAtomicFileSize+1))
	if err != nil {
		return nil, false, fmt.Errorf("read compatibility destination %q: %w", name, err)
	}
	if len(data) > maxCompatibilityAtomicFileSize {
		// refusal:by-design world-action: an oversized destination cannot be safely compared before replacement.
		return nil, false, fmt.Errorf("compatibility destination %q exceeds max atomic compare size %d bytes", name, maxCompatibilityAtomicFileSize)
	}
	return data, true, nil
}

func createCompatibilityTempFile(dirFD int, perm fs.FileMode) (string, *os.File, error) {
	for range 10 {
		var token [12]byte
		if _, err := rand.Read(token[:]); err != nil {
			return "", nil, fmt.Errorf("generate compatibility temp file name: %w", err)
		}
		name := ".gentle-ai-" + hex.EncodeToString(token[:]) + ".tmp"
		fd, err := unix.Openat(dirFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, uint32(perm))
		if err == unix.EEXIST {
			continue
		}
		if err != nil {
			return "", nil, fmt.Errorf("create compatibility temp file: %w", err)
		}
		return name, os.NewFile(uintptr(fd), name), nil
	}
	// refusal:by-design world-action: repeated random-name collisions prevent a safe temporary file publication.
	return "", nil, fmt.Errorf("create compatibility temp file: exhausted unique names")
}
