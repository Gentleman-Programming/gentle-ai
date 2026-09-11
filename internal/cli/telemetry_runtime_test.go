package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

type runtimeHeldInput struct {
	release  <-chan struct{}
	finished chan<- struct{}
}

func (r runtimeHeldInput) Read([]byte) (int, error) {
	<-r.release
	close(r.finished)
	return 0, io.EOF
}

func TestTelemetryRuntimeStdinDeadline(t *testing.T) {
	old := runtimeStdinTimeout
	runtimeStdinTimeout = 20 * time.Millisecond
	t.Cleanup(func() { runtimeStdinTimeout = old })
	for _, route := range []string{"send", "opencode"} {
		t.Run(route, func(t *testing.T) {
			home := runtimeCLIHome(t)
			before := runtimeCLIDisk(t, home)
			runtimeCLIServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("timed-out stdin made HTTP request") })
			release, finished := make(chan struct{}), make(chan struct{})
			done := make(chan struct{})
			var out bytes.Buffer
			var runErr error
			start := time.Now()
			go func() {
				defer close(done)
				runErr = runTelemetryRuntimeInput([]string{route, "--json"}, &out, runtimeHeldInput{release, finished})
			}()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("runtime command did not bound open stdin")
			}
			if time.Since(start) > 250*time.Millisecond {
				t.Error("stdin deadline seam was not honored")
			}
			close(release)
			<-finished
			<-done
			if runErr != nil || !strings.Contains(out.String(), `"discarded"`) {
				t.Fatal("unexpected timeout decision", out.String(), runErr)
			}
			if !reflect.DeepEqual(before, runtimeCLIDisk(t, home)) {
				t.Fatal("timed-out stdin wrote artifacts")
			}
		})
	}
}

type runtimeSignalInput struct {
	io.Reader
	entered chan struct{}
}

func (r runtimeSignalInput) Read(p []byte) (int, error) {
	select {
	case <-r.entered:
	default:
		close(r.entered)
	}
	return r.Reader.Read(p)
}

func TestTelemetryRuntimePolicyRevokedDuringStdin(t *testing.T) {
	for _, route := range []string{"send", "opencode"} {
		t.Run(route, func(t *testing.T) {
			home := runtimeCLIHome(t)
			runtimeCLIServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("revoked input sent HTTP") })
			r, w := io.Pipe()
			defer r.Close()
			defer w.Close()
			entered, done := make(chan struct{}), make(chan struct{})
			var out bytes.Buffer
			var runErr error
			go func() {
				defer close(done)
				runErr = runTelemetryRuntimeInput([]string{route, "--json"}, &out, runtimeSignalInput{r, entered})
			}()
			<-entered
			if err := telemetry.Save(home, telemetry.State{InstallID: "PRIVATE_INSTALL", Enabled: false, NoticeShown: true}); err != nil {
				t.Fatal(err)
			}
			revoked := runtimeCLIDisk(t, home)
			body := string(runtimeCLIFixture())
			if route == "opencode" {
				body = completedOpenCodeEnvelope
			}
			if _, err := io.WriteString(w, body); err != nil {
				t.Fatal(err)
			}
			_ = w.Close()
			<-done
			if runErr != nil || !strings.Contains(out.String(), `"disabled"`) {
				t.Fatal("policy revocation ignored", out.String(), runErr)
			}
			if !reflect.DeepEqual(revoked, runtimeCLIDisk(t, home)) {
				t.Fatal("revoked command wrote artifacts")
			}
		})
	}
}

func TestTelemetryRuntimeDirectSendDisabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("DO_NOT_TRACK", "1")
	var out bytes.Buffer
	if err := runTelemetryRuntimeInput([]string{"send", "--json"}, &out, noOpenCodeRead{t}); err != nil {
		t.Fatalf("direct send command unavailable: %v", err)
	}
	if !strings.Contains(out.String(), `"disabled"`) {
		t.Fatal(out.String())
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("disabled command wrote artifacts: %v %v", entries, err)
	}
}

func runtimeCLIFixture() []byte {
	token := json.RawMessage(`{"reported":1,"unavailable":2,"unsupported":3,"sum":4}`)
	batch := telemetry.RuntimeBatch{Schema: telemetry.RuntimeSchema, Registry: json.RawMessage("1"), Host: "pi", Rows: []telemetry.RuntimeRow{{
		Model: telemetry.RuntimeModel{Provider: "unknown", ID: "unknown"}, ModelEvidence: "unknown", AgentKind: "built_in", AgentClass: "sdd-apply", SelectedEffort: "off", EffectiveEffort: "unsupported", Launches: json.RawMessage("1"), Responses: json.RawMessage("1"), Input: token, Output: token, CacheRead: token, CacheCreation: token, ReasoningTokens: token, TotalTokens: token, ErrorCategory: "none", Duration: telemetry.RuntimeDuration{Kind: "unavailable", MeasuredCount: json.RawMessage("0"), SumMS: json.RawMessage("null")},
	}}}
	body, _ := json.Marshal(batch)
	return body
}

func runtimeCLIHome(t *testing.T) string {
	t.Helper()
	home := telemetryTestHome(t)
	enableTelemetryForTest(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	if err := telemetry.Save(home, telemetry.State{InstallID: "PRIVATE_INSTALL", Enabled: true, NoticeShown: true}); err != nil {
		t.Fatal(err)
	}
	return home
}

func runtimeCLIDisk(t *testing.T, home string) map[string]string {
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
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result[relative] += string(body)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func runtimeCLIServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	old := runtimeHTTPClient
	runtimeHTTPClient = server.Client
	t.Cleanup(func() { runtimeHTTPClient = old })
	t.Setenv(telemetry.EndpointEnvVar, server.URL)
	return server
}

func TestTelemetryRuntimeStdin(t *testing.T) {
	home := runtimeCLIHome(t)
	before := runtimeCLIDisk(t, home)
	requests := 0
	runtimeCLIServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, _ := io.ReadAll(r.Body)
		event, err := telemetry.ParseRuntimeEvent(body)
		if err != nil || event.Host != "pi" || event.Rows[0].AgentClass != "sdd-apply" || bytes.Contains(body, []byte("PRIVATE")) || bytes.Contains(body, []byte("batch_id")) {
			t.Error("unsafe remote event", err)
		}
		_, _ = io.WriteString(w, `{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`)
	})
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = original; _ = r.Close() })
	if _, err = w.Write(runtimeCLIFixture()); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	var out bytes.Buffer
	if err := RunTelemetry([]string{"runtime", "send", "--json"}, &out); err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if json.Unmarshal(out.Bytes(), &result) != nil || len(result) != 2 || result["schema"] != "gentle-ai.telemetry-runtime-send/v1" || result["decision"] != "stored" || requests != 1 {
		t.Fatal(out.String(), requests)
	}
	if !reflect.DeepEqual(before, runtimeCLIDisk(t, home)) {
		t.Fatal("direct send wrote disk")
	}
}

func TestTelemetryRuntimeRemovedRoutes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DO_NOT_TRACK", "1")
	for _, args := range [][]string{{"ingest", "--json"}, {"flush", "--json"}, {"capabilities", "--json"}, {"send"}, {"send", "--json", "PRIVATE_PATH"}, {"PRIVATE_COMMAND", "--json"}} {
		var out bytes.Buffer
		err := runTelemetryRuntimeInput(args, &out, noOpenCodeRead{t})
		if err == nil || strings.Contains(err.Error(), "PRIVATE") || out.Len() != 0 {
			t.Fatal("invalid route exposed or admitted")
		}
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatal("removed route wrote artifacts")
	}
}
