//go:build linux

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

var privateNonRootCgroup = flag.Bool("private-cgroup-non-root", false, "require live non-root unified cgroup membership")

func privateMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func privateMount(data, target, required string) bool {
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 || fields[4] != target {
			continue
		}
		options := "," + fields[5] + ","
		for _, option := range strings.Split(required, ",") {
			if !strings.Contains(options, ","+option+",") {
				return false
			}
		}
		return target != "/tmp" || !strings.Contains(options, ",noexec,")
	}
	return false
}

func privateGuest(t *testing.T) {
	t.Helper()
	if testing.Short() || os.Getenv("GENTLE_PRIVATE_GUEST") != "approved" {
		t.Skip("requires separately approved bounded Linux Guest")
	}
	privateMust(t, privateKernel())
	membership, err := os.ReadFile("/proc/self/cgroup")
	privateMust(t, err)
	if *privateNonRootCgroup && strings.TrimSuffix(string(membership), "\n") == "0::/" {
		t.Fatal("host-like qualification requires actual non-root membership")
	}
	status, err := os.ReadFile("/proc/self/status")
	privateMust(t, err)
	if os.Getuid() != 65532 || !strings.Contains(string(status), "CapEff:\t0000000000000000") || !strings.Contains(string(status), "NoNewPrivs:\t1") {
		t.Fatal("Guest privilege guard failed")
	}
	mounts, err := os.ReadFile("/proc/self/mountinfo")
	privateMust(t, err)
	if !privateMount(string(mounts), "/", "ro") || !privateMount(string(mounts), "/tmp", "rw,nosuid,nodev") {
		t.Fatal("Guest mount guard failed")
	}
	allowed := map[string]string{"PATH": "/usr/bin:/bin", "HOME": "/tmp", "TMPDIR": "/tmp", "GENTLE_PRIVATE_GUEST": "approved"}
	if len(os.Environ()) != len(allowed) {
		t.Fatal("unexpected Guest environment")
	}
	for name, value := range allowed {
		if os.Getenv(name) != value {
			t.Fatal("Guest environment mismatch", name)
		}
	}
}

func TestPrivateKernel(t *testing.T) {
	privateGuest(t)
	mount := "1 0 0:1 / /sys/fs/cgroup ro - cgroup2 cgroup rw\n"
	limits := []string{"3221225472", "0", "100000 100000", "64"}
	cases := []struct {
		name, membership, mount string
		change                  int
	}{
		{"accepted", "0::/\n", mount, -1},
		{"unmapped membership", "0::/host\n", strings.Replace(mount, "0:1 / ", "0:1 /other ", 1), -1},
		{"multiple memberships", "0::/\n1:cpu:/\n", mount, -1},
		{"absent mount", "0::/\n", "", -1},
		{"wrong filesystem", "0::/\n", strings.ReplaceAll(mount, "cgroup2", "tmpfs"), -1},
		{"memory", "0::/\n", mount, 0},
		{"swap", "0::/\n", mount, 1},
		{"cpu", "0::/\n", mount, 2},
		{"pids", "0::/\n", mount, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values := append([]string(nil), limits...)
			if tc.change >= 0 {
				values[tc.change] = "max"
			}
			if (privateKernelData(tc.membership, tc.mount, values) == nil) != (tc.name == "accepted") {
				t.Fatal("kernel qualification mismatch")
			}
		})
	}
}

func TestPrivateCgroupMapping(t *testing.T) {
	privateGuest(t)
	mount := "1 0 0:1 / /sys/fs/cgroup ro - cgroup2 cgroup rw\n"
	submount := strings.Replace(mount, "0:1 / ", "0:1 /tenant ", 1)
	cases := []struct {
		name, membership, mounts, want string
	}{
		{"root", "0::/\n", mount, "/sys/fs/cgroup"},
		{"nested", "0::/system.slice/docker-abc.scope\n", mount, "/sys/fs/cgroup/system.slice/docker-abc.scope"},
		{"subtree", "0::/tenant/child\n", submount, "/sys/fs/cgroup/child"},
		{"subtree root", "0::/tenant\n", submount, "/sys/fs/cgroup"},
		{"decoded root", "0::/tenant space/child\n", strings.Replace(mount, "0:1 / ", `0:1 /tenant\040space `, 1), "/sys/fs/cgroup/child"},
		{"optional fields", "0::/child\n", strings.Replace(mount, " ro - ", " ro shared:2 master:1 - ", 1), "/sys/fs/cgroup/child"},
		{"literal space", "0::/child space\n", mount, "/sys/fs/cgroup/child space"},
		{"invalid UTF-8", "0::/child\xff\n", mount, ""},
		{"missing membership", "", mount, ""},
		{"duplicate membership", "0::/\n0::/\n", mount, ""},
		{"hybrid", "0::/\n1:cpu:/\n", mount, ""},
		{"unrooted", "0::child\n", mount, ""},
		{"traversal", "0::/tenant/../other\n", mount, ""},
		{"dot", "0::/./child\n", mount, ""},
		{"lateral", "0::/../../docker.scope\n", mount, ""},
		{"repeated slash", "0:://child\n", mount, ""},
		{"trailing slash", "0::/child/\n", mount, ""},
		{"whitespace", " 0::/\n", mount, ""},
		{"nul", "0::/child\x00\n", mount, ""},
		{"control character", "0::/child\v\n", mount, ""},
		{"unsupported member escape", `0::/child\040name`, mount, ""},
		{"prefix boundary", "0::/tenant-other\n", submount, ""},
		{"outside subtree", "0::/other\n", submount, ""},
		{"missing mount", "0::/\n", "", ""},
		{"duplicate mount", "0::/\n", mount + mount, ""},
		{"wrong filesystem", "0::/\n", strings.Replace(mount, "cgroup2", "tmpfs", 1), ""},
		{"shadow", "0::/child\n", mount + "2 1 0:2 / /sys/fs/cgroup/child ro - tmpfs tmpfs rw\n", ""},
		{"noncanonical root", "0::/tenant\n", strings.Replace(mount, "0:1 / ", "0:1 /tenant/../ ", 1), ""},
		{"unsupported escape", "0::/\n", strings.Replace(mount, "/sys/fs/cgroup", `/sys/fs/cgroup\057`, 1), ""},
		{"decoded traversal", "0::/\n", strings.Replace(mount, "0:1 / ", `0:1 /tenant\040/../ `, 1), ""},
		{"missing separator", "0::/\n", strings.Replace(mount, " - ", " ", 1), ""},
	}
	// All DATA cases execute in both Guest modes. Avoid per-case success logs so
	// the complete verbose Guest output remains subject to the 4096-byte cap.
	for _, tc := range cases {
		got, err := privateCgroupPath(tc.membership, tc.mounts)
		var failure *PrivateRuntimeError
		if got != tc.want || (tc.want != "" && err != nil) || (tc.want == "" && (!errors.As(err, &failure) || failure.Kind != "unavailable")) {
			t.Errorf("%s: mapping=%q error=%v", tc.name, got, err)
		}
	}
	// Assert each C0, DEL and C1 code point independently without success logs.
	// Numeric failure labels cannot themselves emit terminal control characters.
	for _, bounds := range [][2]rune{{0, 0x1f}, {0x7f, 0x9f}} {
		for control := bounds[0]; control <= bounds[1]; control++ {
			membership := "0::/child" + string(control) + "name\n"
			got, err := privateCgroupPath(membership, mount)
			var failure *PrivateRuntimeError
			if got != "" || !errors.As(err, &failure) || failure.Kind != "unavailable" {
				t.Errorf("control U+%04X: expected empty mapping and typed unavailable", control)
			}
		}
	}
	// Missing limit DATA is refusal, not a simulation of live kernel file reads.
	if err := privateKernelData("0::/\n", mount, nil); err == nil {
		t.Fatal("missing limits accepted")
	}
	// Real filesystem refusal checks stay inside the qualified Guest; no fake
	// reader or alternate kernel source can authorize RunPrivateInstall.
	parent := t.TempDir()
	link := filepath.Join(parent, "cgroup-link")
	privateMust(t, os.Symlink("/sys/fs/cgroup", link))
	for _, path := range []string{filepath.Join(parent, "missing"), parent, link} {
		var failure *PrivateRuntimeError
		if err := privateCgroupPhysical(path, true); !errors.As(err, &failure) || failure.Kind != "unavailable" {
			t.Fatal("unreadable, wrong-filesystem or symlink kernel path accepted")
		}
	}
}

func privateFixture(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	privateMust(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		var data []byte
		if info.Mode().IsRegular() {
			data, err = os.ReadFile(path)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			var target string
			target, err = os.Readlink(path)
			data = []byte(target)
		}
		st := info.Sys().(*syscall.Stat_t)
		result[path] = fmt.Sprintf("%v:%d:%d:%d:%d:%x", info.Mode(), st.Uid, st.Gid, st.Dev, st.Ino, sha256.Sum256(data))
		return err
	}))
	return result
}

func TestPrivateRunner(t *testing.T) {
	privateGuest(t)
	cases := []struct{ name, path, body, kind string }{
		{"start", "/nonexistent-gentle-private", "", "start"},
		{"nonzero", "/bin/sh", "exit 7", "nonzero"},
		{"combined overflow", "/bin/sh", "printf '%3000s' x; printf '%3000s' y >&2; sleep 30", "output"},
		{"cancel", "/bin/sh", "sleep 30 & echo $!; wait", "canceled"},
		{"deadline", "/bin/sh", "sleep 30 & echo $!; wait", "deadline"},
		{"held pipes", "/bin/sh", "sleep 30 & echo $!; exit 0", "pipes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			budget := 5 * time.Second
			if tc.kind == "deadline" {
				budget = 200 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), budget)
			defer cancel()
			if tc.kind == "canceled" {
				timer := time.AfterFunc(200*time.Millisecond, cancel)
				defer timer.Stop()
			}
			cmd := exec.CommandContext(ctx, tc.path, "-c", tc.body)
			cmd.Env = []string{"PATH=/usr/bin:/bin"}
			start := time.Now()
			output, err := privateRun(ctx, cmd, cancel)
			var failure *PrivateRuntimeError
			if !errors.As(err, &failure) || failure.Kind != tc.kind || time.Since(start) > 3*time.Second || len(output) > 4096 {
				t.Fatal("runner outcome", err, len(output))
			}
			if tc.kind != "start" && cmd.ProcessState == nil {
				t.Fatal("direct child not reaped")
			}
			if tc.kind == "output" && output != "" {
				t.Fatal("overflow output disclosed")
			}
			if tc.kind == "canceled" || tc.kind == "deadline" || tc.kind == "pipes" {
				pid, err := strconv.Atoi(strings.TrimSpace(output))
				privateMust(t, err)
				fd, err := unix.PidfdOpen(pid, 0)
				if err == nil {
					defer unix.Close(fd)
					fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
					n, err := unix.Poll(fds, 1000)
					if err != nil || n != 1 || fds[0].Revents&unix.POLLIN == 0 {
						t.Fatal("descendant still live", err)
					}
				} else if !errors.Is(err, unix.ESRCH) {
					t.Fatal(err)
				}
			}
		})
	}
}

// These seams exercise DATA and resource accounting, not production TLS authority.
type privateColdTransport func(*http.Request) (*http.Response, error)

func (f privateColdTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type privateColdBody struct {
	io.Reader
	closes   int
	closeErr error
}

func (b *privateColdBody) Close() error {
	b.closes++
	return b.closeErr
}

type privateColdSink struct {
	bytes.Buffer
	closes             int
	writeErr, closeErr error
}

func (s *privateColdSink) Write(p []byte) (int, error) {
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	return s.Buffer.Write(p)
}
func (s *privateColdSink) Close() error {
	s.closes++
	return s.closeErr
}

type privateColdReader struct{ err error }

func (r privateColdReader) Read([]byte) (int, error) { return 0, r.err }

type privateColdReadFunc func([]byte) (int, error)

func (f privateColdReadFunc) Read(p []byte) (int, error) { return f(p) }

func privateColdCertificate(t *testing.T, hostname string) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	privateMust(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: hostname},
		DNSNames:              []string{hostname},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	privateMust(t, err)
	cert, err := x509.ParseCertificate(der)
	privateMust(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, cert
}

func privateColdResponse(t *testing.T, body *privateColdBody, length int64) *http.Response {
	t.Helper()
	_, cert := privateColdCertificate(t, "nodejs.org")
	request, err := http.NewRequest(http.MethodGet, "https://nodejs.org/dist/v24.18.0/node-v24.18.0-linux-x64.tar.gz", nil)
	privateMust(t, err)
	return &http.Response{
		StatusCode:    200,
		Header:        make(http.Header),
		Body:          body,
		ContentLength: length,
		Request:       request,
		TLS:           &tls.ConnectionState{Version: tls.VersionTLS12, ServerName: "nodejs.org", PeerCertificates: []*x509.Certificate{cert}, VerifiedChains: [][]*x509.Certificate{{cert}}},
	}
}

func TestPrivateColdData(t *testing.T) {
	privateGuest(t)
	// Synthetic Guest-only ambient proxy settings cannot select a transport.
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	// Independent literals: changing production pins cannot silently change the controls.
	if privateColdURL != "https://nodejs.org/dist/v24.18.0/node-v24.18.0-linux-x64.tar.gz" || privateColdSize != 57224421 || privateColdSHA != "783130984963db7ba9cbd01089eaf2c2efb055c7c1693c943174b967b3050cb8" {
		t.Fatal("fixed Node DATA pins changed")
	}
	client, err := privateColdClient()
	privateMust(t, err)
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || !transport.DisableCompression || transport.MaxResponseHeaderBytes != 16384 || transport.TLSHandshakeTimeout != 10*time.Second || transport.ResponseHeaderTimeout != 30*time.Second || client.Timeout != 120*time.Second || client.Jar != nil {
		t.Fatal("production transport bounds or ambient effects")
	}
	defer transport.CloseIdleConnections()
	config := transport.TLSClientConfig
	if config == nil || config.InsecureSkipVerify || config.MinVersion != tls.VersionTLS12 || config.ServerName != "nodejs.org" || config.RootCAs == nil || len(config.RootCAs.Subjects()) == 0 || config.VerifyConnection != nil || config.VerifyPeerCertificate != nil {
		t.Fatal("production TLS verification configuration")
	}
	if client.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("redirects enabled")
	}
	fixture := []byte("small helper-only pinned DATA; never production archive authority")
	digest := fmt.Sprintf("%x", sha256.Sum256(fixture))
	fault := errors.New("resource fault DATA")
	cases := []struct {
		label  string
		mutate func(*http.Response, *privateColdBody, *privateColdSink, context.CancelFunc)
		wantOK bool
	}{
		{"valid", func(*http.Response, *privateColdBody, *privateColdSink, context.CancelFunc) {}, true},
		{"status", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.StatusCode = 302
		}, false},
		{"encoding", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.Header.Set("Content-Encoding", "gzip")
		}, false},
		{"duplicate encoding", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.Header["Content-Encoding"] = []string{"identity", "gzip"}
		}, false},
		{"decoded", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.Uncompressed = true
		}, false},
		{"declared length", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.ContentLength++
		}, false},
		{"unknown length", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.ContentLength = -1
		}, false},
		{"oversize", func(_ *http.Response, b *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			b.Reader = bytes.NewReader(append(append([]byte(nil), fixture...), 'x'))
		}, false},
		{"truncated", func(_ *http.Response, b *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			b.Reader = bytes.NewReader(fixture[:len(fixture)-1])
		}, false},
		{"hash", func(_ *http.Response, b *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			b.Reader = bytes.NewReader(bytes.Repeat([]byte{'x'}, len(fixture)))
		}, false},
		{"read", func(_ *http.Response, b *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			b.Reader = privateColdReader{fault}
		}, false},
		{"write", func(_ *http.Response, _ *privateColdBody, s *privateColdSink, _ context.CancelFunc) {
			s.writeErr = fault
		}, false},
		{"body close", func(_ *http.Response, b *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			b.closeErr = fault
		}, false},
		{"file close", func(_ *http.Response, _ *privateColdBody, s *privateColdSink, _ context.CancelFunc) {
			s.closeErr = fault
		}, false},
		{"cancel", func(_ *http.Response, _ *privateColdBody, _ *privateColdSink, cancel context.CancelFunc) { cancel() }, false},
		{"cancel during read", func(_ *http.Response, b *privateColdBody, _ *privateColdSink, cancel context.CancelFunc) {
			reader := bytes.NewReader(fixture)
			b.Reader = privateColdReadFunc(func(p []byte) (int, error) {
				cancel()
				return reader.Read(p)
			})
		}, false},
		{"method", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.Request.Method = "POST"
		}, false},
		{"origin", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.Request.URL.Host = "example.com"
		}, false},
		{"path", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.Request.URL.Path += "/"
		}, false},
		{"query", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.Request.URL.RawQuery = "x=1"
		}, false},
		{"request host", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.Request.Host = "example.com"
		}, false},
		{"scheme", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.Request.URL.Scheme = "http"
		}, false},
		{"raw path", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.Request.URL.RawPath = r.Request.URL.Path
		}, false},
		{"userinfo", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.Request.URL.User = url.User("synthetic")
		}, false},
		{"fragment", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.Request.URL.Fragment = "x"
		}, false},
		{"SNI", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.TLS.ServerName = "example.com"
		}, false},
		{"unverified", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.TLS.VerifiedChains = nil
		}, false},
		{"chain leaf mismatch", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			_, different := privateColdCertificate(t, "nodejs.org")
			r.TLS.VerifiedChains = [][]*x509.Certificate{{different}}
		}, false},
		{"old TLS", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) {
			r.TLS.Version = tls.VersionTLS11
		}, false},
		{"no TLS", func(r *http.Response, _ *privateColdBody, _ *privateColdSink, _ context.CancelFunc) { r.TLS = nil }, false},
	}
	// No per-success logs: every assertion executes under the unchanged raw 4KiB bound.
	for index, tc := range cases {
		body := &privateColdBody{Reader: bytes.NewReader(fixture)}
		sink := &privateColdSink{}
		response := privateColdResponse(t, body, int64(len(fixture)))
		ctx, cancel := context.WithCancel(context.Background())
		tc.mutate(response, body, sink, cancel)
		err := privateColdReceive(ctx, response, sink, int64(len(fixture)), digest)
		cancel()
		var failure *PrivateRuntimeError
		if (err == nil) != tc.wantOK || (!tc.wantOK && (!errors.As(err, &failure) || failure.Kind != "acquisition")) || body.closes != 1 || sink.closes != 1 || sink.Len() > len(fixture)+1 {
			t.Fatalf("DATA control %d (%s): result/resources %v", index, tc.label, err)
		}
		wantBytes := 0
		switch tc.label {
		case "valid", "hash", "body close", "file close", "cancel during read":
			wantBytes = len(fixture)
		case "oversize":
			wantBytes = len(fixture) + 1
		case "truncated":
			wantBytes = len(fixture) - 1
		}
		if sink.Len() != wantBytes || (tc.wantOK && !bytes.Equal(sink.Bytes(), fixture)) {
			t.Fatalf("DATA control %d wrote unexpected bytes", index)
		}
		if (tc.label == "read" || tc.label == "write" || tc.label == "body close" || tc.label == "file close") && !errors.Is(err, fault) {
			t.Fatalf("DATA control %d lost resource cause", index)
		}
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	body := &privateColdBody{Reader: bytes.NewReader(fixture)}
	sink := &privateColdSink{}
	err = privateColdReceive(ctx, privateColdResponse(t, body, int64(len(fixture))), sink, int64(len(fixture)), digest)
	if !errors.Is(err, context.DeadlineExceeded) || body.closes != 1 || sink.closes != 1 || sink.Len() != 0 {
		t.Fatal("expired DATA deadline did not close without writes")
	}
}

func TestPrivateColdTLS(t *testing.T) {
	privateGuest(t)
	fixture := []byte("local TLS helper DATA, not a production Node archive")
	digest := fmt.Sprintf("%x", sha256.Sum256(fixture))
	for index, hostname := range []string{"nodejs.org", "example.com", "nodejs.org", "nodejs.org", "nodejs.org"} {
		certificate, cert := privateColdCertificate(t, hostname)
		var requests atomic.Int32
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			if r.Method != "GET" || r.Host != "nodejs.org" || r.URL.RequestURI() != "/dist/v24.18.0/node-v24.18.0-linux-x64.tar.gz" || r.Header.Get("Cookie") != "" || r.Header.Get("Accept-Encoding") != "" {
				t.Error("fixed TLS request or ambient headers changed")
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(fixture)))
			if index == 3 {
				w.Header().Set("X-Oversize", strings.Repeat("x", 20000))
			}
			if index == 4 {
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				return
			}
			_, _ = w.Write(fixture)
		}))
		server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
		// Expected handshake rejection is not an unbounded server diagnostic stream.
		server.Config.ErrorLog = log.New(io.Discard, "", 0)
		server.StartTLS()
		roots := x509.NewCertPool()
		if index != 2 {
			roots.AddCert(cert)
		}
		transport := &http.Transport{
			Proxy:                  nil,
			DisableCompression:     true,
			MaxResponseHeaderBytes: 16384,
			TLSClientConfig:        &tls.Config{RootCAs: roots, ServerName: "nodejs.org", MinVersion: tls.VersionTLS12},
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				if address != "nodejs.org:443" {
					return nil, errors.New("unexpected TLS address")
				}
				return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, server.Listener.Addr().String())
			},
		}
		jar, err := cookiejar.New(nil)
		privateMust(t, err)
		origin, err := url.Parse("https://nodejs.org/")
		privateMust(t, err)
		jar.SetCookies(origin, []*http.Cookie{{Name: "synthetic-canary", Value: "not-a-credential"}})
		client := &http.Client{Transport: transport, Timeout: 3 * time.Second, Jar: jar}
		if index == 4 {
			client.Timeout = time.Second
		}
		parent := t.TempDir()
		archive := filepath.Join(parent, "fixture")
		err = privateColdFetch(context.Background(), client, archive, int64(len(fixture)), digest)
		transport.CloseIdleConnections()
		server.Close()
		var failure *PrivateRuntimeError
		if index == 0 {
			data, readErr := os.ReadFile(archive)
			info, statErr := os.Lstat(archive)
			if err != nil || readErr != nil || statErr != nil || !bytes.Equal(data, fixture) || info.Mode() != 0600 || requests.Load() != 1 {
				t.Fatal("real local TLS verified Node SAN DATA transfer", err)
			}
		} else {
			wantRequests := int32(0)
			if index >= 3 {
				wantRequests = 1
			}
			if !errors.As(err, &failure) || failure.Kind != "acquisition" || requests.Load() != wantRequests {
				t.Fatalf("actual TLS/HTTP rejection %d effects", index)
			}
			if index == 4 {
				info, statErr := os.Lstat(archive)
				if !errors.Is(err, context.DeadlineExceeded) || statErr != nil || info.Size() != 0 || info.Mode() != 0600 {
					t.Fatal("actual body deadline/resource effects", err)
				}
			} else if _, statErr := os.Lstat(archive); !os.IsNotExist(statErr) {
				t.Fatal("TLS/header rejection wrote archive")
			}
		}
	}
}

func TestPrivateColdRequest(t *testing.T) {
	privateGuest(t)
	parent := t.TempDir()
	fixture := []byte("bounded request DATA")
	digest := fmt.Sprintf("%x", sha256.Sum256(fixture))
	for index, kind := range []string{"success", "redirect", "transport", "nil body", "file collision", "cancel"} {
		archive := filepath.Join(parent, strconv.Itoa(index))
		body := &privateColdBody{Reader: bytes.NewReader(fixture)}
		calls := 0
		client := &http.Client{Transport: privateColdTransport(func(request *http.Request) (*http.Response, error) {
			calls++
			deadline, ok := request.Context().Deadline()
			if request.Method != "GET" || request.URL.String() != "https://nodejs.org/dist/v24.18.0/node-v24.18.0-linux-x64.tar.gz" || request.Header.Get("Cookie") != "" || !ok || time.Until(deadline) > 120*time.Second {
				t.Fatal("fixed request/deadline changed")
			}
			if kind == "transport" {
				return nil, errors.New("transport DATA fault")
			}
			response := privateColdResponse(t, body, int64(len(fixture)))
			if kind == "redirect" {
				response.StatusCode = 302
				response.Header.Set("Location", "https://example.com/")
			}
			if kind == "nil body" {
				response.Body = nil
			}
			return response, nil
		})}
		if kind == "file collision" {
			privateMust(t, os.WriteFile(archive, []byte("sentinel"), 0600))
		}
		before := privateFixture(t, parent)
		ctx, cancel := context.WithCancel(context.Background())
		if kind == "cancel" {
			cancel()
		}
		err := privateColdFetch(ctx, client, archive, int64(len(fixture)), digest)
		cancel()
		var failure *PrivateRuntimeError
		if kind == "success" {
			data, readErr := os.ReadFile(archive)
			if err != nil || readErr != nil || !bytes.Equal(data, fixture) || body.closes != 1 || calls != 1 {
				t.Fatal("request success resources", err)
			}
		} else {
			if !errors.As(err, &failure) || failure.Kind != "acquisition" || calls > 1 {
				t.Fatalf("request DATA control %d", index)
			}
			if kind != "nil body" && kind != "transport" && kind != "cancel" && body.closes != 1 {
				t.Fatal("rejected response body leaked")
			}
			if (kind == "file collision" || kind == "cancel" || kind == "transport") && !reflect.DeepEqual(before, privateFixture(t, parent)) {
				t.Fatal("request rejection changed preimage")
			}
			if kind == "cancel" && (calls != 0 || !errors.Is(err, context.Canceled)) {
				t.Fatal("cancelled request reached transport or lost cause")
			}
		}
	}
}

func privateNoStage(t *testing.T, parent string) {
	t.Helper()
	entries, err := os.ReadDir(parent)
	privateMust(t, err)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".gentle-go-") || strings.HasPrefix(entry.Name(), ".gentle-shell-stage.") {
			t.Fatal("owned workspace leaked", entry.Name())
		}
	}
}

func TestPrivateDirectoryAndCleanup(t *testing.T) {
	privateGuest(t)
	parent := t.TempDir()
	privateMust(t, os.Chmod(parent, 0700))
	// Owned inert DATA replaces staging the retired installer assets.
	workspace, err := os.MkdirTemp(parent, ".gentle-go-")
	privateMust(t, err)
	identity, err := privateDirectory(workspace)
	privateMust(t, err)
	if identity.Mode() != os.ModeDir|0700 || identity.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		t.Fatal("workspace ownership")
	}
	privateMust(t, os.WriteFile(filepath.Join(workspace, "data"), []byte("not executable fixture"), 0444))
	info, err := os.Lstat(parent)
	privateMust(t, err)
	privateMust(t, os.Chmod(parent, info.Mode()|os.ModeSticky))
	_, directoryErr := privateDirectory(parent)
	if directoryErr == nil {
		t.Fatal("special mode bit accepted", parent)
	}
	privateMust(t, os.Chmod(parent, info.Mode()))
	blocked := filepath.Join(workspace, "blocked")
	privateMust(t, os.Mkdir(blocked, 0700))
	privateMust(t, os.WriteFile(filepath.Join(blocked, "child"), nil, 0600))
	privateMust(t, os.Chmod(blocked, 0000))
	cleanupErr := privateCleanup(workspace, identity)
	privateMust(t, os.Chmod(blocked, 0700))
	if cleanupErr == nil {
		t.Fatal("cleanup fault hidden")
	}
	old := workspace + ".old"
	privateMust(t, os.Rename(workspace, old))
	privateMust(t, os.Mkdir(workspace, 0700))
	if privateCleanup(workspace, identity) == nil {
		t.Fatal("replacement deleted")
	}
	_, err = os.Lstat(workspace)
	privateMust(t, err)
}

func TestPrivateDestination(t *testing.T) {
	privateGuest(t)
	parent := t.TempDir()
	privateMust(t, os.Chmod(parent, 0700))
	pi := filepath.Join(parent, "pi")
	privateMust(t, os.Mkdir(pi, 0700))
	privateMust(t, os.WriteFile(filepath.Join(pi, "settings.json"), []byte("private Pi fixture"), 0600))
	privateMust(t, os.Symlink("settings.json", filepath.Join(pi, "link")))
	before := privateFixture(t, pi)
	for _, kind := range []string{"file", "directory", "symlink"} {
		dest := filepath.Join(parent, kind)
		switch kind {
		case "file":
			privateMust(t, os.WriteFile(dest, []byte("Pi sentinel"), 0600))
		case "directory":
			privateMust(t, os.Mkdir(dest, 0700))
		case "symlink":
			privateMust(t, os.Symlink(pi, dest))
		}
		preimage := privateFixture(t, parent)
		err := privateDestination(dest)
		var failure *PrivateRuntimeError
		if !errors.As(err, &failure) || failure.Kind != "refused" || !reflect.DeepEqual(preimage, privateFixture(t, parent)) {
			t.Fatal("real API collision changed preimage", kind, err)
		}
		privateNoStage(t, parent)
		if !reflect.DeepEqual(before, privateFixture(t, pi)) {
			t.Fatal("Pi preimage changed")
		}
	}
}

func TestPrivateColdInvalidArchive(t *testing.T) {
	privateGuest(t)
	parent := t.TempDir()
	privateMust(t, os.Chmod(parent, 0700))
	pi := filepath.Join(parent, "pi")
	privateMust(t, os.Mkdir(pi, 0700))
	privateMust(t, os.WriteFile(filepath.Join(pi, "sentinel"), []byte("unrelated Pi"), 0600))
	before := privateFixture(t, pi)
	for index, kind := range []string{"wrong hash", "truncated", "oversize"} {
		workspace, err := os.MkdirTemp(parent, ".gentle-go-cold-")
		privateMust(t, err)
		identity, err := privateDirectory(workspace)
		privateMust(t, err)
		length := int64(57224421)
		if kind == "truncated" {
			length--
		}
		if kind == "oversize" {
			length++
		}
		body := &privateColdBody{Reader: io.LimitReader(privateColdReadFunc(func(p []byte) (int, error) {
			clear(p)
			return len(p), nil
		}), length)}
		calls := 0
		client := &http.Client{Transport: privateColdTransport(func(*http.Request) (*http.Response, error) {
			calls++
			return privateColdResponse(t, body, 57224421), nil
		})}
		err = privateColdFetch(context.Background(), client, filepath.Join(workspace, "node.tgz"), 57224421, "783130984963db7ba9cbd01089eaf2c2efb055c7c1693c943174b967b3050cb8")
		var failure *PrivateRuntimeError
		entries, readErr := os.ReadDir(workspace)
		if !errors.As(err, &failure) || failure.Kind != "acquisition" || body.closes != 1 || calls != 1 || readErr != nil || len(entries) != 1 || entries[0].Name() != "node.tgz" || !reflect.DeepEqual(before, privateFixture(t, pi)) {
			t.Fatalf("invalid full-size DATA control %d reached bootstrap or changed Pi", index)
		}
		// Direct owned cleanup; the retired cold finalizer/result is not recreated.
		privateMust(t, privateCleanup(workspace, identity))
		dest := filepath.Join(parent, "rejected")
		if _, statErr := os.Lstat(dest); !os.IsNotExist(statErr) {
			t.Fatal("invalid DATA published a destination")
		}
	}
	privateNoStage(t, parent)
}

func TestPrivateColdOwnership(t *testing.T) {
	privateGuest(t)
	rootOwned, err := os.Lstat("/cold-root-owned")
	privateMust(t, err)
	_, ownerErr := privateDirectory("/cold-root-owned")
	if rootOwned.Mode() != os.ModeDir|0700 || rootOwned.Sys().(*syscall.Stat_t).Uid != 0 || ownerErr == nil {
		t.Fatal("actual foreign owner with otherwise private mode accepted")
	}
	for index, kind := range []string{"valid", "parent mode", "stage mode", "parent replacement", "stage replacement", "parent symlink", "stage symlink", "nil identity", "destination appeared"} {
		base := t.TempDir()
		parent := filepath.Join(base, "parent")
		privateMust(t, os.Mkdir(parent, 0700))
		parentIdentity, err := privateDirectory(parent)
		privateMust(t, err)
		workspace, err := os.MkdirTemp(parent, ".gentle-go-cold-")
		privateMust(t, err)
		identity, err := privateDirectory(workspace)
		privateMust(t, err)
		dest := filepath.Join(parent, "published")
		switch kind {
		case "parent mode":
			privateMust(t, os.Chmod(parent, 0755))
		case "stage mode":
			privateMust(t, os.Chmod(workspace, 0755))
		case "parent replacement":
			privateMust(t, os.Rename(parent, parent+".old"))
			privateMust(t, os.Mkdir(parent, 0700))
			privateMust(t, os.Mkdir(workspace, 0700))
		case "parent symlink":
			privateMust(t, os.Rename(parent, parent+".old"))
			privateMust(t, os.Symlink(parent+".old", parent))
		case "stage replacement", "stage symlink":
			privateMust(t, os.Rename(workspace, workspace+".old"))
			if kind == "stage symlink" {
				privateMust(t, os.Symlink(workspace+".old", workspace))
			} else {
				privateMust(t, os.Mkdir(workspace, 0700))
			}
		case "nil identity":
			identity = nil
		case "destination appeared":
			privateMust(t, os.Mkdir(dest, 0700))
		}
		before := privateFixture(t, base)
		// Assert physical reinspection and held identities directly, not through
		// the retired composite workspace validator's preimage classification.
		freshParent, parentErr := privateDirectory(parent)
		freshStage, stageErr := privateDirectory(workspace)
		destErr := privateDestination(dest)
		parentMatches := parentErr == nil && os.SameFile(parentIdentity, freshParent)
		stageMatches := stageErr == nil && identity != nil && os.SameFile(identity, freshStage)
		wantParent := kind != "parent mode" && kind != "parent replacement" && kind != "parent symlink"
		wantStage := kind != "stage mode" && kind != "stage replacement" && kind != "stage symlink" && kind != "parent replacement" && kind != "parent symlink" && kind != "nil identity"
		wantDest := kind != "parent mode" && kind != "parent symlink" && kind != "destination appeared"
		if parentMatches != wantParent || stageMatches != wantStage || (destErr == nil) != wantDest || !reflect.DeepEqual(before, privateFixture(t, base)) {
			t.Fatalf("ownership control %d changed preimage/classification", index)
		}
		for _, err := range []error{parentErr, stageErr} {
			var failure *PrivateRuntimeError
			if err != nil && (!errors.As(err, &failure) || failure.Kind != "filesystem") {
				t.Fatal("physical directory refusal classification", err)
			}
		}
		var failure *PrivateRuntimeError
		if destErr != nil && (!errors.As(destErr, &failure) || failure.Kind != "refused") {
			t.Fatal("destination refusal classification", destErr)
		}
	}
	// Actual filesystem cleanup denial and replacement preservation. The
	// published sentinel is DATA, not a simulated full pipeline.
	for index, kind := range []string{"permission", "replacement", "nil identity", "clean published"} {
		parent := t.TempDir()
		workspace, err := os.MkdirTemp(parent, ".gentle-go-cold-")
		privateMust(t, err)
		identity, err := privateDirectory(workspace)
		privateMust(t, err)
		dest := filepath.Join(parent, "published")
		privateMust(t, os.Mkdir(dest, 0700))
		privateMust(t, os.WriteFile(filepath.Join(dest, "sentinel"), []byte("published preserve"), 0600))
		published := privateFixture(t, dest)
		blocked := filepath.Join(workspace, "blocked")
		privateMust(t, os.Mkdir(blocked, 0700))
		privateMust(t, os.WriteFile(filepath.Join(blocked, "child"), []byte("owned"), 0600))
		if kind == "permission" {
			privateMust(t, os.Chmod(blocked, 0000))
		}
		if kind == "replacement" {
			privateMust(t, os.Rename(workspace, workspace+".old"))
			privateMust(t, os.Mkdir(workspace, 0700))
			privateMust(t, os.WriteFile(filepath.Join(workspace, "foreign"), []byte("preserve"), 0600))
		}
		if kind == "nil identity" {
			identity = nil
		}
		var stageBefore map[string]string
		if kind == "replacement" || kind == "nil identity" {
			stageBefore = privateFixture(t, workspace)
		}
		cleanupErr := privateCleanup(workspace, identity)
		if kind == "permission" {
			privateMust(t, os.Chmod(blocked, 0700))
		}
		var failure *PrivateRuntimeError
		permissionFailure := kind == "permission" && os.IsPermission(cleanupErr)
		identityFailure := (kind == "replacement" || kind == "nil identity") && errors.As(cleanupErr, &failure) && failure.Kind == "uncertain"
		if (cleanupErr == nil) != (kind == "clean published") || (cleanupErr != nil && !permissionFailure && !identityFailure) || !reflect.DeepEqual(published, privateFixture(t, dest)) {
			t.Fatalf("cleanup control %d lost publication/ambiguity", index)
		}
		if (kind == "replacement" || kind == "nil identity") && !reflect.DeepEqual(stageBefore, privateFixture(t, workspace)) {
			t.Fatal("refused cleanup changed foreign or unowned preimage")
		}
		if kind == "replacement" {
			data, err := os.ReadFile(filepath.Join(workspace, "foreign"))
			if err != nil || string(data) != "preserve" {
				t.Fatal("foreign replacement deleted")
			}
		}
		if kind == "clean published" {
			privateNoStage(t, parent)
		}
	}
}

func TestPrivateLiveCgroupMarker(t *testing.T) {
	privateGuest(t)
	membership, err := os.ReadFile("/proc/self/cgroup")
	privateMust(t, err)
	mounts, err := os.ReadFile("/proc/self/mountinfo")
	privateMust(t, err)
	mapped, err := privateCgroupPath(string(membership), string(mounts))
	privateMust(t, err)
	marker := fmt.Sprintf("live-cgroup-member=%s mapped=%s\n", strings.TrimSuffix(string(membership), "\n"), mapped)
	if len(marker) > 1024 {
		t.Fatal("live membership diagnostic withheld: bound exceeded")
	}
	fmt.Print(marker)
}
