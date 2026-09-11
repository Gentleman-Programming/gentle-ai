package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const runtimeAck = `{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`

func runtimeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	if err := Save(home, State{InstallID: "PRIVATE_INSTALL", Enabled: true, NoticeShown: true}); err != nil {
		t.Fatal(err)
	}
	return home
}

// Snapshot every file, directory and mode under the test HOME, including XDG.
// Policy and deliberately stale files are test setup, never sender output.
func runtimeDisk(t *testing.T, home string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(home, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(home, path)
		result[relative] = info.Mode().String()
		if !entry.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result[relative] += string(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func runtimeEndpoint(endpoint string) func(string) string {
	return func(key string) string {
		if key == EndpointEnvVar {
			return endpoint
		}
		return ""
	}
}

func TestRuntimeSendOneAttemptNoDisk(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		ack, want string
	}{
		{"stored", 200, runtimeAck, "stored"},
		{"duplicate", 200, strings.Replace(runtimeAck, "stored", "duplicate", 1), "duplicate"},
		{"server", 500, "PRIVATE_ERROR", "discarded"}, {"rate limited", 429, "", "discarded"},
		{"invalid", 400, "", "discarded"}, {"conflict", 409, "", "discarded"},
		{"bad ack", 200, `{"schema":"wrong","decision":"stored"}`, "discarded"},
		{"extra ack", 200, strings.Replace(runtimeAck, `"stored"`, `"stored","private":"canary"`, 1), "discarded"},
		{"duplicate ack key", 200, strings.Replace(runtimeAck, `"stored"`, `"stored","decision":"stored"`, 1), "discarded"},
		{"trailing ack", 200, runtimeAck + `{}`, "discarded"},
		{"bounded ack", 200, runtimeAck + strings.Repeat(" ", 1025), "discarded"},
		{"wrong status", 202, runtimeAck, "discarded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := runtimeHome(t)
			for _, name := range []string{"telemetry-runtime-pending-v1.json", "telemetry-runtime-outbox-v1.json", "telemetry-runtime-kick-v1.json", "telemetry-runtime.lock"} {
				if err := os.WriteFile(filepath.Join(home, stateDir, name), []byte("STALE_PRIVATE_METRICS"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := runtimeDisk(t, home)
			var requests atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				body, _ := io.ReadAll(r.Body)
				ev, err := ParseRuntimeEvent(body)
				if err != nil || ev.Host != "claude-code" || r.Method != "POST" || r.URL.Path != "/v1/runtime-events" || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("invalid remote event: %v", err)
				}
				for _, forbidden := range []string{"batch_id", "session", "task_id", "install_id", "PRIVATE", "STALE"} {
					if bytes.Contains(body, []byte(forbidden)) {
						t.Errorf("private identifier sent: %s", forbidden)
					}
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.ack)
			}))
			defer server.Close()
			got := SendRuntime(context.Background(), home, runtimeEndpoint(server.URL), strings.NewReader(runtimeFixture), server.Client())
			if got != tc.want || requests.Load() != 1 {
				t.Fatalf("decision=%s requests=%d", got, requests.Load())
			}
			if !reflect.DeepEqual(before, runtimeDisk(t, home)) {
				t.Fatal("runtime send changed disk")
			}
		})
	}
}

func TestRuntimeSendInputByteLimit(t *testing.T) {
	home := runtimeHome(t)
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, runtimeAck)
	}))
	defer server.Close()
	before := runtimeDisk(t, home)
	exact := runtimeFixture + strings.Repeat(" ", RuntimeMaxBytes-len(runtimeFixture))
	if got := SendRuntime(context.Background(), home, runtimeEndpoint(server.URL), strings.NewReader(exact), server.Client()); got != "stored" {
		t.Fatal("exact limit refused", got)
	}
	oversized := strings.NewReader(exact + strings.Repeat(" ", 100))
	if got := SendRuntime(context.Background(), home, runtimeEndpoint(server.URL), oversized, server.Client()); got != "discarded" || oversized.Len() != 99 {
		t.Fatal("input read exceeded bounded sentinel", got, oversized.Len())
	}
	if requests.Load() != 1 || !reflect.DeepEqual(before, runtimeDisk(t, home)) {
		t.Fatal("oversized send or disk mutation")
	}
}

func TestRuntimeSendFreshIdentity(t *testing.T) {
	home := runtimeHome(t)
	var ids []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ev RuntimeEvent
		_ = json.NewDecoder(r.Body).Decode(&ev)
		ids = append(ids, ev.DeliveryID)
		_, _ = io.WriteString(w, runtimeAck)
	}))
	defer server.Close()
	for i := 0; i < 2; i++ {
		if got := SendRuntime(context.Background(), home, runtimeEndpoint(server.URL), strings.NewReader(runtimeFixture), server.Client()); got != "stored" {
			t.Fatal(got)
		}
	}
	if len(ids) != 2 || ids[0] == ids[1] || !runtimeID.MatchString(ids[0]) || !runtimeID.MatchString(ids[1]) {
		t.Fatal("delivery identities not independent")
	}
}

func TestRuntimeSendTimeout(t *testing.T) {
	for _, phase := range []string{"headers", "body"} {
		t.Run(phase, func(t *testing.T) {
			home := runtimeHome(t)
			var requests atomic.Int32
			release := make(chan struct{})
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if phase == "body" {
					_, _ = io.WriteString(w, `{`)
					w.(http.Flusher).Flush()
				}
				<-release
			}))
			defer server.Close()
			defer close(release)
			before := runtimeDisk(t, home)
			start := time.Now()
			got := SendRuntime(context.Background(), home, runtimeEndpoint(server.URL), strings.NewReader(runtimeFixture), server.Client())
			elapsed := time.Since(start)
			if got != "discarded" || requests.Load() != 1 || elapsed > 3500*time.Millisecond || elapsed < 2500*time.Millisecond {
				t.Fatalf("decision=%s requests=%d duration=%s", got, requests.Load(), elapsed)
			}
			if !reflect.DeepEqual(before, runtimeDisk(t, home)) {
				t.Fatal("timeout wrote metrics")
			}
		})
	}
}

func TestRuntimeSendRefusesRedirectAndUserinfo(t *testing.T) {
	home := runtimeHome(t)
	var requests, forwarded atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1) }))
	defer target.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	for _, endpoint := range []string{redirect.URL, strings.Replace(redirect.URL, "https://", "https://PRIVATE:SECRET@", 1), strings.Replace(redirect.URL, "https://", "https://PRIVATE@", 1), strings.Replace(redirect.URL, "https://", "http://", 1)} {
		before := runtimeDisk(t, home)
		if got := SendRuntime(context.Background(), home, runtimeEndpoint(endpoint), strings.NewReader(runtimeFixture), redirect.Client()); got != "discarded" {
			t.Fatal(got)
		}
		if !reflect.DeepEqual(before, runtimeDisk(t, home)) {
			t.Fatal("refused endpoint wrote metrics")
		}
	}
	if requests.Load() != 1 || forwarded.Load() != 0 {
		t.Fatal("followed redirect or accepted userinfo/http")
	}
}
