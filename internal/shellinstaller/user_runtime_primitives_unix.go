//go:build linux || darwin

package shellinstaller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
)

func privateError(kind string, cause error) *PrivateRuntimeError {
	return &PrivateRuntimeError{Kind: kind, Cause: cause}
}

func privateHierarchyPath(path string) bool {
	return utf8.ValidString(path) && filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsRune(path, '\\') && strings.IndexFunc(path, unicode.IsControl) == -1
}

func privateDirectory(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	canonical, canonicalErr := filepath.EvalSymlinks(path)
	if err != nil || canonicalErr != nil || canonical != path || !info.IsDir() || info.Mode() != os.ModeDir|0700 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return nil, privateError("filesystem", errors.Join(err, canonicalErr))
	}
	if err := privateExtendedMetadata(path, info, false); err != nil {
		return nil, privateError("filesystem", err)
	}
	return info, nil
}

func privateDestination(dest string) error {
	if !filepath.IsAbs(dest) || filepath.Clean(dest) != dest || !regexp.MustCompile(`^[A-Za-z0-9_.-]+$`).MatchString(filepath.Base(dest)) {
		return privateError("refused", nil)
	}
	if _, err := privateDirectory(filepath.Dir(dest)); err != nil {
		return privateError("refused", err)
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		return privateError("refused", err)
	}
	return nil
}

func privatePhysical(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	canonical, canonicalErr := filepath.EvalSymlinks(path)
	if err != nil || canonicalErr != nil || canonical != path || !info.Mode().IsRegular() {
		return nil, privateError("filesystem", errors.Join(err, canonicalErr))
	}
	if err := privateExtendedMetadata(path, info, true); err != nil {
		return nil, privateError("filesystem", err)
	}
	return info, nil
}

func privateStamp(info os.FileInfo) string {
	st := info.Sys().(*syscall.Stat_t)
	mtime, ctime := privateStatTimes(st)
	return fmt.Sprintf("%d:%d:%d:%d:%v:%d:%v:%v", st.Dev, st.Ino, st.Uid, st.Gid, info.Mode(), info.Size(), mtime, ctime)
}

func privateCleanup(workspace string, identity os.FileInfo) error {
	current, err := privateDirectory(workspace)
	if err != nil || identity == nil || !os.SameFile(identity, current) {
		return privateError("uncertain", err)
	}
	return os.RemoveAll(workspace)
}

func privateFailure(dest string, cause error) *PrivateRuntimeError {
	if _, err := os.Lstat(dest); cause == nil || !os.IsNotExist(err) {
		return privateError("uncertain", cause)
	}
	var failure *PrivateRuntimeError
	if errors.As(cause, &failure) {
		return privateError(failure.Kind, cause)
	}
	return privateError("filesystem", cause)
}

type privateOutput struct {
	sync.Mutex
	data     []byte
	overflow bool
	cancel   context.CancelFunc
}

func (w *privateOutput) Write(data []byte) (int, error) {
	w.Lock()
	defer w.Unlock()
	if !w.overflow && len(w.data)+len(data) > 4096 {
		w.overflow, w.data = true, nil
		w.cancel()
	}
	if !w.overflow {
		w.data = append(w.data, data...)
	}
	return len(data), nil
}

func privateRun(ctx context.Context, cmd *exec.Cmd, cancel context.CancelFunc) (string, error) {
	output := &privateOutput{cancel: cancel}
	cmd.Stdout, cmd.Stderr = output, output
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 750 * time.Millisecond
	kill := func() error {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		} else {
			return err
		}
	}
	cmd.Cancel = kill
	if err := cmd.Start(); err != nil {
		return "", privateError("start", err)
	}
	err := cmd.Wait() // WaitDelay bounds inherited pipes and joins stdlib copiers.
	if killErr := kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
		err = errors.Join(err, killErr)
	}
	if output.overflow || !utf8.Valid(output.data) {
		return "", privateError("output", err)
	}
	kind := "nonzero"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		kind = "deadline"
	} else if ctx.Err() != nil {
		kind = "canceled"
	} else if errors.Is(err, exec.ErrWaitDelay) {
		kind = "pipes"
	} else if err == nil {
		return string(output.data), nil
	}
	return string(output.data), privateError(kind, err)
}
