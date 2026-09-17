package telemetry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

type runtimeNoRead struct{ t *testing.T }

func (r runtimeNoRead) Read([]byte) (int, error) {
	r.t.Fatal("disabled sender read input")
	return 0, io.EOF
}

func TestRuntimeSendPolicyNoDisk(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer server.Close()
	for _, scenario := range []string{"missing", "disabled", "unenrolled", "corrupt", "DO_NOT_TRACK", "GENTLE_AI_TELEMETRY", "CI", "GITHUB_ACTIONS"} {
		t.Run(scenario, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", home)
			t.Setenv("XDG_STATE_HOME", home)
			t.Setenv("XDG_CACHE_HOME", home)
			if scenario != "missing" {
				if err := Save(home, State{InstallID: "PRIVATE", Enabled: scenario != "disabled", NoticeShown: scenario != "unenrolled"}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "corrupt" {
				if err := os.WriteFile(Path(home), []byte("PRIVATE_CORRUPT"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			env := func(key string) string {
				if key == EndpointEnvVar {
					return server.URL
				}
				if key == scenario {
					if key == "GENTLE_AI_TELEMETRY" {
						return "0"
					}
					return "1"
				}
				return ""
			}
			before := runtimeDisk(t, home)
			if got := SendRuntime(context.Background(), home, env, runtimeNoRead{t}, server.Client()); got != "disabled" {
				t.Fatal(got)
			}
			if !reflect.DeepEqual(before, runtimeDisk(t, home)) {
				t.Fatal("disabled send wrote state")
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatal("disabled send made HTTP request")
	}
}

func TestRuntimeSendInvalidNoDisk(t *testing.T) {
	home := runtimeHome(t)
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer server.Close()
	for _, body := range []string{"PRIVATE_INPUT", runtimeFixture + `{}`, strings.Repeat(" ", RuntimeMaxBytes+1), strings.Replace(runtimeFixture, "claude-opus-5", "PRIVATE_MODEL", 1), strings.Replace(runtimeFixture, `"registry":1`, `"registry":1,"session_id":"PRIVATE"`, 1)} {
		before := runtimeDisk(t, home)
		if got := SendRuntime(context.Background(), home, runtimeEndpoint(server.URL), strings.NewReader(body), server.Client()); got != "discarded" {
			t.Fatal(got)
		}
		if !reflect.DeepEqual(before, runtimeDisk(t, home)) {
			t.Fatal("invalid input wrote metrics")
		}
	}
	if requests.Load() != 0 {
		t.Fatal("invalid input sent")
	}
}

type runtimeReadHook struct {
	io.Reader
	before func()
}

func (r runtimeReadHook) Read(p []byte) (int, error) { r.before(); return r.Reader.Read(p) }

func TestRuntimeSendPolicyRevokedDuringInput(t *testing.T) {
	home := runtimeHome(t)
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer server.Close()
	var revoked bool
	input := runtimeReadHook{strings.NewReader(runtimeFixture), func() { revoked = true }}
	env := func(key string) string {
		if key == "DO_NOT_TRACK" && revoked {
			return "1"
		}
		return runtimeEndpoint(server.URL)(key)
	}
	before := runtimeDisk(t, home)
	if got := SendRuntime(context.Background(), home, env, input, server.Client()); got != "disabled" {
		t.Fatal(got)
	}
	if requests.Load() != 0 || !reflect.DeepEqual(before, runtimeDisk(t, home)) {
		t.Fatal("revoked send had side effects")
	}
}

func TestRuntimeSendPolicyFileRechecked(t *testing.T) {
	home := runtimeHome(t)
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer server.Close()
	var revoked map[string]string
	input := runtimeReadHook{strings.NewReader(runtimeFixture), func() {
		// A separate actor changes the existing opt-out file while stdin is read.
		if revoked != nil {
			return
		}
		if err := Save(home, State{InstallID: "PRIVATE_INSTALL", Enabled: false, NoticeShown: true}); err != nil {
			t.Fatal(err)
		}
		revoked = runtimeDisk(t, home)
	}}
	if got := SendRuntime(context.Background(), home, runtimeEndpoint(server.URL), input, server.Client()); got != "disabled" {
		t.Fatal(got)
	}
	if requests.Load() != 0 || !reflect.DeepEqual(revoked, runtimeDisk(t, home)) {
		t.Fatal("policy file revocation ignored or modified")
	}
}

func TestRuntimeSendPolicyRecheckedBeforeHTTP(t *testing.T) {
	home := runtimeHome(t)
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer server.Close()
	revoked := false
	env := func(key string) string {
		if key == EndpointEnvVar {
			revoked = true
			return server.URL
		}
		if key == "DO_NOT_TRACK" && revoked {
			return "1"
		}
		return ""
	}
	before := runtimeDisk(t, home)
	if got := SendRuntime(context.Background(), home, env, strings.NewReader(runtimeFixture), server.Client()); got != "disabled" {
		t.Fatal(got)
	}
	if requests.Load() != 0 || !reflect.DeepEqual(before, runtimeDisk(t, home)) {
		t.Fatal("final policy check missing")
	}
}
