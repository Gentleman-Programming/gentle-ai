//go:build linux && shell_gga_canonical

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// Offline response DATA only: simulated TLS metadata is not a TLS handshake,
// publisher signature, live-network acceptance or permission to install.
type userGGAResponseTransport func(*http.Request) (*http.Response, error)

func (f userGGAResponseTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type userGGAObservedBody struct {
	reader       io.Reader
	read, closed int
	closeErr     error
	cancel       context.CancelFunc
	cancelClose  context.CancelFunc
}

func (b *userGGAObservedBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	b.read += n
	if b.cancel != nil {
		b.cancel()
	}
	return n, err
}

func (b *userGGAObservedBody) Close() error {
	b.closed++
	if b.cancelClose != nil {
		b.cancelClose()
	}
	return b.closeErr
}

func TestUserGGAAcquisitionOfflineResponses(t *testing.T) {
	archive, err := os.ReadFile("/frozen-gga-source.tgz")
	if err != nil {
		t.Fatal(err)
	}
	const endpoint = "https://codeload.github.com/Gentleman-Programming/gentleman-guardian-angel/tar.gz/1124c3672f082c56b033c4e23a30e95d0e8cd593"
	for _, name := range []string{"success", "chunked", "redirect", "status", "origin", "no TLS", "unverified", "empty verified chain", "wrong hostname", "wrong certificate", "encoding", "duplicate encoding", "decompressed", "truncated", "oversized", "digest", "read failure", "close failure", "canceled before request", "canceled during read", "canceled on refused close"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			root := filepath.Join(t.TempDir(), "stage")
			profile := UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", GGA: "on", GGAProvider: "codex"}
			if err := userPrepareInstallStage(ctx, root, "separate", &profile); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
				t.Fatal(err)
			}
			before, err := userTreeStamp(root)
			if err != nil {
				t.Fatal(err)
			}
			body := &userGGAObservedBody{reader: bytes.NewReader(archive)}
			leaf := &x509.Certificate{DNSNames: []string{"codeload.github.com"}, Raw: []byte("offline certificate DATA")}
			response := &http.Response{StatusCode: 200, ContentLength: int64(len(archive)), Header: make(http.Header), Body: body, TLS: &tls.ConnectionState{Version: tls.VersionTLS12, ServerName: "codeload.github.com", PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}}
			switch name {
			case "chunked":
				response.ContentLength = -1
			case "redirect":
				response.StatusCode = 302
				response.Header.Set("Location", endpoint+"/unapproved")
			case "status":
				response.StatusCode = 503
			case "no TLS":
				response.TLS = nil
			case "unverified":
				response.TLS.VerifiedChains = nil
			case "empty verified chain":
				response.TLS.VerifiedChains[0] = nil
			case "wrong hostname":
				response.TLS.ServerName = "other.invalid"
			case "wrong certificate":
				leaf.DNSNames = []string{"other.invalid"}
			case "encoding":
				response.Header.Set("Content-Encoding", "gzip")
			case "duplicate encoding":
				response.Header["Content-Encoding"] = []string{"identity", "identity"}
			case "decompressed":
				response.Uncompressed = true
			case "truncated":
				body.reader = bytes.NewReader(archive[:len(archive)-1])
			case "oversized":
				body.reader = bytes.NewReader(append(append([]byte(nil), archive...), make([]byte, 32768)...))
			case "digest":
				data := append([]byte(nil), archive...)
				data[0] ^= 1
				body.reader = bytes.NewReader(data)
			case "read failure":
				body.reader = io.MultiReader(bytes.NewReader(archive[:17]), userGGAErrorReader{})
			case "close failure":
				body.closeErr = io.ErrClosedPipe
			case "canceled before request":
				cancel()
			case "canceled during read":
				body.cancel = cancel
			case "canceled on refused close":
				response.StatusCode = 503
				body.cancelClose = cancel
			}
			requests, followed := 0, false
			jar, _ := cookiejar.New(nil)
			u, _ := url.Parse(endpoint)
			jar.SetCookies(u, []*http.Cookie{{Name: "fixture", Value: "test-only"}})
			client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { followed = true; return nil }, Transport: userGGAResponseTransport(func(req *http.Request) (*http.Response, error) {
				requests++
				if requests > 1 || req.Method != http.MethodGet || req.URL.String() != endpoint || req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" {
					t.Fatal("request departed from fixed credentialless acquisition")
				}
				response.Request = req
				if name == "origin" {
					copy := req.Clone(ctx)
					copy.URL, _ = url.Parse("https://other.invalid/source")
					response.Request = copy
				}
				return response, nil
			})}
			err = userGGAAcquire(ctx, root, client)
			wantOK := name == "success" || name == "chunked"
			if (err == nil) != wantOK {
				t.Fatalf("result = %v, success = %v", err, wantOK)
			}
			wantRequests := 1
			if name == "canceled before request" {
				wantRequests = 0
			}
			if requests != wantRequests || body.closed != wantRequests || followed || body.read > len(archive)+1 {
				t.Fatalf("ownership requests=%d closed=%d followed=%v bytes=%d", requests, body.closed, followed, body.read)
			}
			if name == "oversized" && body.read != len(archive)+1 {
				t.Fatal("oversized response was not stopped at the exact read bound", body.read)
			}
			if name == "close failure" && !errors.Is(err, io.ErrClosedPipe) {
				t.Fatal("lost close failure", err)
			}
			if !wantOK {
				after, stampErr := userTreeStamp(root)
				if stampErr != nil || before != after {
					t.Fatal("refused response changed stage", stampErr)
				}
				if (name == "canceled during read" || name == "canceled on refused close") && !errors.Is(err, context.Canceled) {
					t.Fatal("lost cancellation cause", err)
				}
				return
			}
			if _, err := privateNativeFile(ctx, filepath.Join(root, "runtime/gga/source.tgz"), 0600, 81841, userGGASourceSHA); err != nil {
				t.Fatal("missing retained authenticated archive", err)
			}
			for _, member := range []string{"bin/gga", "lib/providers.sh", "lib/cache.sh", "lib/pr_mode.sh"} {
				if _, err := privateNativeFile(ctx, filepath.Join(root, "runtime/gga", member), 0700, -1, ""); err != nil {
					t.Fatal("missing private member", err)
				}
			}
		})
	}
}

type userGGAErrorReader struct{}

func (userGGAErrorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
