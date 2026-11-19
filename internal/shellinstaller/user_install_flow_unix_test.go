//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestUnixCanonicalRequestKeepsUncheckablePathsVerbatim(t *testing.T) {
	parent := t.TempDir()
	canonical, err := userCanonicalPath(filepath.Join(parent, "shell"))
	if err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(parent)
	if err != nil || canonical != filepath.Join(physical, "shell") {
		t.Fatalf("canonical spelling = %q, want below %q (%v)", canonical, physical, err)
	}
	req := UserInstallRequest{Destination: filepath.Join(parent, "shell"), Mode: "separate", SharedAgent: "relative/agent", Confirmation: "token"}
	got, err := userCanonicalRequest(req)
	if err != nil || got.Destination != canonical || got.SharedPrefix != "" || got.SharedAgent != "relative/agent" || got.Mode != "separate" || got.Confirmation != "token" {
		t.Fatalf("canonical request = %+v %v", got, err)
	}
	if again, err := userCanonicalRequest(got); err != nil || again != got {
		t.Fatalf("canonicalization is not idempotent: %+v %v", again, err)
	}
	if _, err := os.Lstat(canonical); !os.IsNotExist(err) {
		t.Fatalf("canonicalization had effects: %v", err)
	}
}

// The publish tail runs the ephemeral cleanup only after a successful rename,
// readback and launcher publication; an uncertain publish must not prune the
// renamed evidence tree.
func TestUserPublishManifestSkipsEphemeralCleanupOnUncertainPublish(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	stage, root := filepath.Join(parent, "stage"), filepath.Join(parent, "final")
	for _, dir := range []string{stage, stage + "/prefix", stage + "/agent", stage + "/bin"} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	manifest := userManifest{Schema: userSchema, Destination: root, Prefix: stage + "/prefix", Agent: stage + "/agent", Mode: "separate"}
	if manifest.PrefixIdentity, err = userIdentity(stage + "/prefix"); err != nil {
		t.Fatal(err)
	}
	if manifest.AgentIdentity, err = userIdentity(stage + "/agent"); err != nil {
		t.Fatal(err)
	}
	original := userPublishEphemeralCleanup
	called := false
	userPublishEphemeralCleanup = func(string) error {
		called = true
		return nil
	}
	t.Cleanup(func() { userPublishEphemeralCleanup = original })
	if _, publishErr := userPublishManifest(context.Background(), stage, stage+"/prefix", stage+"/agent", manifest); publishErr == nil {
		t.Fatal("incomplete staged tree published successfully")
	} else {
		var failure *PrivateRuntimeError
		if !errors.As(publishErr, &failure) || failure.Kind != "uncertain" {
			t.Fatalf("publish failure is not an uncertain rename outcome: %v", publishErr)
		}
	}
	if called {
		t.Fatal("ephemeral cleanup ran on an uncertain publish")
	}
	if _, err := os.Lstat(filepath.Join(root, "installation.json")); err != nil {
		t.Fatalf("uncertain publish lost the renamed evidence tree: %v", err)
	}
}

// The staged Unix install runs the network-bound provision at least twice
// (global install, then the verify readback) and the main channel adds
// further npm work, so the external install deadline must stay strictly above
// the npm budget of every provision invocation it contains. A bound at or
// below that product reproduces issue #5714: the deadline expires after the
// publish step and the installation is reported unqualified.
func TestUnixInstallBoundCoversNpmNetworkBudget(t *testing.T) {
	npm := userScriptsNpmNetworkTimeout(t)
	if userUnixNpmNetworkTimeout != npm {
		t.Fatalf("userUnixNpmNetworkTimeout %s differs from the scripts' NPM_NETWORK_TIMEOUT %s", userUnixNpmNetworkTimeout, npm)
	}
	if userUnixProvisionBound != userUnixNpmNetworkTimeout {
		t.Fatalf("userUnixProvisionBound %s differs from the npm budget %s", userUnixProvisionBound, userUnixNpmNetworkTimeout)
	}
	if userUnixInstallBound <= 3*userUnixNpmNetworkTimeout {
		t.Fatalf("userUnixInstallBound %s does not exceed three network-bound provision invocations at the npm budget %s each", userUnixInstallBound, userUnixNpmNetworkTimeout)
	}
}

func TestUserToolRedirectPolicyAcceptsGitHubAssetHosts(t *testing.T) {
	via := []*http.Request{{}}
	for raw, want := range map[string]bool{
		"https://release-assets.githubusercontent.com/x": true,
		"https://objects.githubusercontent.com/x":        true,
		"https://objects.githubusercontent.com:8443/x":   false,
		"http://objects.githubusercontent.com/x":         false,
		"https://user@objects.githubusercontent.com/x":   false,
		"https://objects.githubusercontent.com/x#frag":   false,
		"https://githubusercontent.com.evil.example/x":   false,
		"https://nodejs.org/x":                           false,
	} {
		target, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := userToolRedirect(&http.Request{URL: target}, via) == nil; got != want {
			t.Errorf("%s: accepted=%v, want %v", raw, got, want)
		}
	}
	ok, _ := url.Parse("https://objects.githubusercontent.com/x")
	if userToolRedirect(&http.Request{URL: ok}, make([]*http.Request, 5)) == nil {
		t.Error("redirect hop limit not enforced")
	}
}

func TestUserToolsClientHasNoWholeBodyTimeout(t *testing.T) {
	client, err := userToolsClient()
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	if client.Timeout != 0 {
		t.Fatalf("tool client keeps a whole-body timeout %s", client.Timeout)
	}
	idle, ok := client.Transport.(*userDownloadIdleTransport)
	if !ok || idle.idle != userDownloadIdleTimeout {
		t.Fatalf("tool client lacks the inactivity timeout: %T", client.Transport)
	}
	base := idle.base.(*http.Transport)
	if base.Proxy != nil || base.ResponseHeaderTimeout != 30*time.Second || base.TLSHandshakeTimeout != 10*time.Second || base.TLSClientConfig.ServerName != "" {
		t.Fatal("tool client transport bounds changed")
	}
}

func TestUserToolFetchRetriesTransientFailuresOnly(t *testing.T) {
	userDownloadFastRetry(t)
	data := []byte("pinned tool archive fixture")
	pin := fmt.Sprintf("%x", sha256.Sum256(data))
	for _, tc := range []struct {
		name     string
		first    func(http.ResponseWriter, *http.Request)
		requests int
		ok       bool
	}{
		{"502 then success", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }, 2, true},
		{"stalled body then success", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", fmt.Sprint(len(data)))
			_, _ = w.Write(data[:4])
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}, 2, true},
		{"404 is final", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }, 1, false},
		{"digest mismatch is final", func(w http.ResponseWriter, _ *http.Request) {
			corrupt := append([]byte(nil), data...)
			corrupt[0] ^= 0xff
			_, _ = w.Write(corrupt)
		}, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if requests == 1 || !tc.ok {
					tc.first(w, r)
					return
				}
				_, _ = w.Write(data)
			}))
			defer server.Close()
			client := server.Client()
			client.Transport = userDownloadIdle(client.Transport, userDownloadIdleTimeout)
			got, err := userToolFetch(context.Background(), client, server.URL+"/fd.tar.gz", userToolSource{size: int64(len(data)), pin: pin})
			if requests != tc.requests || (err == nil) != tc.ok || (tc.ok && string(got) != string(data)) {
				t.Fatalf("requests=%d err=%v, want requests=%d ok=%v", requests, err, tc.requests, tc.ok)
			}
		})
	}
}

type userBootstrapRoundTrip func(*http.Request) (*http.Response, error)

func (f userBootstrapRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUserBootstrapFetchRetriesTransientAndKeepsPreexistingArchive(t *testing.T) {
	userDownloadFastRetry(t)
	for _, preexisting := range []bool{false, true} {
		archive := filepath.Join(t.TempDir(), "node.tgz")
		if preexisting {
			if err := os.WriteFile(archive, []byte("sentinel"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		calls := 0
		client := &http.Client{Transport: userBootstrapRoundTrip(func(*http.Request) (*http.Response, error) {
			calls++
			return nil, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
		})}
		err := userBootstrapFetch(context.Background(), client, archive)
		var failure *PrivateRuntimeError
		if !errors.As(err, &failure) || failure.Kind != "acquisition" || calls != userDownloadAttempts {
			t.Fatalf("preexisting=%v: calls=%d err=%v, want %d transient attempts", preexisting, calls, err, userDownloadAttempts)
		}
		data, readErr := os.ReadFile(archive)
		if preexisting && (readErr != nil || string(data) != "sentinel") {
			t.Fatalf("pre-existing archive was touched: %q %v", data, readErr)
		}
		if !preexisting && !errors.Is(readErr, os.ErrNotExist) {
			t.Fatalf("failed transient attempts left an archive: %v", readErr)
		}
	}
}
