//go:build unix

package reviewtransaction

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func secureOpenLocalStoreLock(path string) (*os.File, error) {
	runSecureOpenLocalStoreLockBeforeOpen(path)

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	root, ok := secureLockRoot(absPath)
	if !ok {
		root = string(filepath.Separator)
	}
	parentFD, err := secureOpenLockParent(root, filepath.Dir(absPath))
	if err != nil {
		return nil, err
	}
	defer unix.Close(parentFD)

	fd, err := unix.Openat(parentFD, filepath.Base(absPath), unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, err
	}

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("review store lock %q is not a regular file", path)
	}

	return os.NewFile(uintptr(fd), path), nil
}

// secureOpenLockParent walks from root to the parent directory of path,
// opening every component below root with O_NOFOLLOW to reject symlinked
// traversal. Anchoring at the repository's Git common directory instead of
// the filesystem root narrows the walk to repository-owned directories
// (1781). When path is not under root, this fails safe by falling back to
// today's root-anchored walk verbatim, so no working configuration regresses.
func secureOpenLockParent(root, path string) (int, error) {
	anchor, relative := root, ""
	if rel, err := filepath.Rel(root, path); err == nil && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
		relative = rel
	} else {
		anchor = string(filepath.Separator)
		relative = strings.TrimPrefix(path, string(filepath.Separator))
	}
	fd, err := unix.Open(anchor, secureDirectoryOpenFlags(), 0)
	if err != nil {
		return -1, err
	}
	if relative == "." {
		return fd, nil
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		nextFD, err := unix.Openat(fd, component, secureDirectoryOpenFlags()|unix.O_NOFOLLOW, 0)
		if err != nil {
			_ = unix.Close(fd)
			return -1, err
		}
		_ = unix.Close(fd)
		fd = nextFD
	}
	return fd, nil
}
