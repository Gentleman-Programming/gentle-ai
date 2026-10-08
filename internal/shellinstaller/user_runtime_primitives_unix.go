//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
	return privatePhysicalEntry(path, true)
}

// privateForeignPhysical checks a regular file the installer reads but does
// not own, such as the operator's gentle-ai binary; on darwin a quarantine
// xattr left by an approved download is tolerated there, never on owned entries.
func privateForeignPhysical(path string) (os.FileInfo, error) {
	return privatePhysicalEntry(path, false)
}

func privatePhysicalEntry(path string, owned bool) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	canonical, canonicalErr := filepath.EvalSymlinks(path)
	if err != nil || canonicalErr != nil || canonical != path || !info.Mode().IsRegular() {
		return nil, privateError("filesystem", errors.Join(err, canonicalErr))
	}
	if err := privateExtendedMetadata(path, info, owned); err != nil {
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
		if err := userKillGroup(cmd.Process.Pid, syscall.SIGKILL); errors.Is(err, syscall.ESRCH) {
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
	killErr := privateReapRun(cmd.Process.Pid, err, kill)
	var uncertain *PrivateRuntimeError
	if errors.As(killErr, &uncertain) {
		return "", killErr // A group member outlived the bounded reap.
	}
	if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
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

func privateColdClient() (*http.Client, error) {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil || len(roots.Subjects()) == 0 {
		return nil, privateError("acquisition", errors.Join(err, errors.New("system TLS roots unavailable")))
	}
	transport := &http.Transport{
		Proxy:                  nil,
		DisableCompression:     true,
		MaxResponseHeaderBytes: 16384,
		DialContext:            (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  30 * time.Second,
		TLSClientConfig:        &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12, ServerName: "nodejs.org"},
	}
	return &http.Client{
		Transport:     transport,
		Timeout:       120 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}

// privateColdReceive owns both resources. Small expected-pin fixtures are DATA
// seams only; the public API always supplies the independent fixed Node pins.
func privateColdReceive(ctx context.Context, response *http.Response, file io.WriteCloser, size int64, digest string) (err error) {
	defer func() {
		var bodyErr error
		if response != nil && response.Body != nil {
			bodyErr = response.Body.Close()
		}
		var fileErr error
		if file != nil {
			fileErr = file.Close()
		}
		var contextErr error
		if ctx != nil {
			contextErr = ctx.Err()
		}
		if joined := errors.Join(err, bodyErr, fileErr, contextErr); joined != nil {
			err = privateError("acquisition", joined)
		}
	}()
	root, _ := url.Parse("https://nodejs.org/")
	if ctx == nil || ctx.Err() != nil || file == nil || response == nil || response.Body == nil {
		return errors.New("missing or canceled acquisition resource")
	}
	if !gentleShellStableResponseMatches(response, root, privateColdPath) || response.StatusCode != http.StatusOK || response.ContentLength != size || response.Uncompressed {
		return errors.New("Node response origin, TLS, status or length differs")
	}
	encoding := response.Header.Values("Content-Encoding")
	if len(encoding) > 1 || (len(encoding) == 1 && encoding[0] != "" && !strings.EqualFold(encoding[0], "identity")) {
		return errors.New("Node response content encoding differs")
	}
	hash := sha256.New()
	count, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, size+1))
	if copyErr != nil || count != size || fmt.Sprintf("%x", hash.Sum(nil)) != digest {
		return errors.Join(copyErr, errors.New("Node DATA length or SHA256 differs"))
	}
	return nil
}

func privateColdFetch(ctx context.Context, client *http.Client, archive string, size int64, digest string) error {
	if ctx == nil || client == nil {
		return privateError("acquisition", errors.New("missing acquisition context/client"))
	}
	if err := ctx.Err(); err != nil {
		return privateError("acquisition", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, privateColdURL, nil)
	if err != nil {
		return privateError("acquisition", err)
	}
	bounded := *client
	bounded.Jar = nil
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := bounded.Do(request)
	if err != nil {
		return privateError("acquisition", err)
	}
	file, openErr := os.OpenFile(archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if openErr != nil {
		var closeErr error
		if response != nil && response.Body != nil {
			closeErr = response.Body.Close()
		}
		return privateError("acquisition", errors.Join(openErr, closeErr))
	}
	return privateColdReceive(ctx, response, file, size, digest)
}
