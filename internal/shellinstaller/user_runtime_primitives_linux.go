//go:build linux

package shellinstaller

import (
	"bytes"
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
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func privateMountPath(field string) (string, bool) {
	// Only the defined space escape is supported. Refuse all other escapes,
	// including encoded separators and control characters, rather than guess.
	path := strings.ReplaceAll(field, `\040`, " ")
	return path, privateHierarchyPath(path)
}

// privateCgroupPath resolves DATA only; production supplies /proc/self bytes.
// Membership is in hierarchy coordinates, not relative to the mountpoint.
func privateCgroupPath(membership, mounts string) (string, error) {
	member := strings.TrimSuffix(membership, "\n")
	if !strings.HasPrefix(member, "0::") || !privateHierarchyPath(strings.TrimPrefix(member, "0::")) {
		return "", privateError("unavailable", nil)
	}
	member = strings.TrimPrefix(member, "0::")
	const trusted = "/sys/fs/cgroup"
	root := ""
	for _, line := range strings.Split(strings.TrimSuffix(mounts, "\n"), "\n") {
		fields := strings.Fields(line)
		separator := -1
		for i, field := range fields {
			if field == "-" {
				if separator != -1 {
					return "", privateError("unavailable", nil)
				}
				separator = i
			}
		}
		if separator < 6 || len(fields) != separator+4 {
			return "", privateError("unavailable", nil)
		}
		mountRoot, rootOK := privateMountPath(fields[3])
		mountpoint, pointOK := privateMountPath(fields[4])
		if !rootOK || !pointOK || strings.HasPrefix(mountpoint, trusted+"/") {
			return "", privateError("unavailable", nil)
		}
		if mountpoint == trusted {
			if root != "" || fields[separator+1] != "cgroup2" {
				return "", privateError("unavailable", nil)
			}
			root = mountRoot
		}
	}
	if root == "" || (root != "/" && member != root && !strings.HasPrefix(member, root+"/")) {
		return "", privateError("unavailable", nil)
	}
	relative := strings.TrimPrefix(member, root)
	return filepath.Join(trusted, relative), nil
}

func privateKernelData(membership, mounts string, limits []string) error {
	if _, err := privateCgroupPath(membership, mounts); err != nil {
		return err
	}
	if len(limits) != 4 || strings.Join(limits, "\n") != "3221225472\n0\n100000 100000\n64" {
		return privateError("unavailable", nil)
	}
	return nil
}

func privateCgroupPhysical(path string, directory bool) error {
	info, err := os.Lstat(path)
	canonical, canonicalErr := filepath.EvalSymlinks(path)
	var fs unix.Statfs_t
	if err != nil || canonicalErr != nil || canonical != path || (directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) || unix.Statfs(path, &fs) != nil || fs.Type != unix.CGROUP2_SUPER_MAGIC {
		return privateError("unavailable", errors.Join(err, canonicalErr))
	}
	return nil
}

func privateKernel() error {
	membership, e1 := os.ReadFile("/proc/self/cgroup")
	mounts, e2 := os.ReadFile("/proc/self/mountinfo")
	if e1 != nil || e2 != nil {
		return privateError("unavailable", errors.Join(e1, e2))
	}
	current, err := privateCgroupPath(string(membership), string(mounts))
	if err != nil {
		return err
	}
	for _, path := range []string{"/sys/fs/cgroup", current} {
		if err := privateCgroupPhysical(path, true); err != nil {
			return err
		}
	}
	// Exact leaf limits enforce upper bounds even with permissive ancestors.
	// Stricter ancestors can reduce availability; this does not promise capacity.
	var limits []string
	for _, name := range []string{"memory.max", "memory.swap.max", "cpu.max", "pids.max"} {
		path := filepath.Join(current, name)
		if err := privateCgroupPhysical(path, false); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) > 64 {
			return privateError("unavailable", err)
		}
		if err := privateCgroupPhysical(path, false); err != nil {
			return err
		}
		limits = append(limits, strings.TrimSpace(string(data)))
	}
	freshMembership, e1 := os.ReadFile("/proc/self/cgroup")
	freshMounts, e2 := os.ReadFile("/proc/self/mountinfo")
	if e1 != nil || e2 != nil || !bytes.Equal(membership, freshMembership) || !bytes.Equal(mounts, freshMounts) {
		return privateError("unavailable", errors.Join(e1, e2))
	}
	return privateKernelData(string(membership), string(mounts), limits)
}

const privateColdURL = "https://nodejs.org/dist/v24.18.0/node-v24.18.0-linux-x64.tar.gz"
const privateColdSize int64 = 57224421
const privateColdSHA = "783130984963db7ba9cbd01089eaf2c2efb055c7c1693c943174b967b3050cb8"

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
	if !gentleShellStableResponseMatches(response, root, "/dist/v24.18.0/node-v24.18.0-linux-x64.tar.gz") || response.StatusCode != http.StatusOK || response.ContentLength != size || response.Uncompressed {
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
