package cli

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

// Published V1 AssistantMessage subset: no batch/session/message identities.
const completedOpenCodeEnvelope = `{"schema":"gentle-ai.telemetry-opencode/v1","info":{"role":"assistant","time":{"created":1,"completed":3},"providerID":"PRIVATE_PROVIDER","modelID":"PRIVATE_MODEL"}}`

type noOpenCodeRead struct{ t *testing.T }

func (r noOpenCodeRead) Read([]byte) (int, error) {
	r.t.Fatal("read event without permission")
	return 0, io.EOF
}

func TestTelemetryRuntimeOpenCodeDirectSend(t *testing.T) {
	home := runtimeCLIHome(t)
	before := runtimeCLIDisk(t, home)
	requests := 0
	wantProvider := "custom"
	runtimeCLIServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, _ := io.ReadAll(r.Body)
		event, err := telemetry.ParseRuntimeEvent(body)
		if err != nil {
			t.Error(err)
		} else if event.Host != "opencode" || event.Rows[0].Model.Provider != wantProvider || event.Rows[0].Model.ID != "custom" || event.Rows[0].Duration.Kind != "message" || string(event.Rows[0].Duration.SumMS) != "2" {
			t.Error("incorrect normalized event")
		}
		for _, private := range []string{"PRIVATE", "batch_id", "created", "completed", "session", "task_id"} {
			if bytes.Contains(body, []byte(private)) {
				t.Error("private wire field", private)
			}
		}
		_, _ = io.WriteString(w, `{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`)
	})
	for i, provider := range []string{"PRIVATE_PROVIDER", "opencode", "OpenCode", "openai"} {
		wantProvider = "custom"
		if provider == "opencode" {
			wantProvider = "opencode"
		}
		input := strings.Replace(completedOpenCodeEnvelope, "PRIVATE_PROVIDER", provider, 1)
		var out bytes.Buffer
		if err := runTelemetryRuntimeInput([]string{"opencode", "--json"}, &out, strings.NewReader(input)); err != nil || !strings.Contains(out.String(), `"stored"`) {
			t.Fatal(out.String(), err)
		}
		if requests != i+1 || !reflect.DeepEqual(before, runtimeCLIDisk(t, home)) {
			t.Fatal("extra attempt or disk mutation")
		}
	}
}

func TestTelemetryRuntimeOpenCodePolicyBeforeRead(t *testing.T) {
	for _, scenario := range []string{"missing", "disabled", "unenrolled", "env"} {
		t.Run(scenario, func(t *testing.T) {
			home := telemetryTestHome(t)
			enableTelemetryForTest(t)
			if scenario != "missing" {
				if err := telemetry.Save(home, telemetry.State{InstallID: "local", Enabled: scenario != "disabled", NoticeShown: scenario != "unenrolled"}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "env" {
				t.Setenv("DO_NOT_TRACK", "1")
			}
			before := runtimeCLIDisk(t, home)
			var out bytes.Buffer
			if err := runTelemetryRuntimeInput([]string{"opencode", "--json"}, &out, noOpenCodeRead{t}); err != nil || !strings.Contains(out.String(), `"disabled"`) {
				t.Fatal(out.String(), err)
			}
			if !reflect.DeepEqual(before, runtimeCLIDisk(t, home)) {
				t.Fatal("disabled adapter wrote state")
			}
		})
	}
}

func TestTelemetryRuntimeOpenCodeIgnoresPartial(t *testing.T) {
	home := runtimeCLIHome(t)
	runtimeCLIServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("ignored event made HTTP request") })
	before := runtimeCLIDisk(t, home)
	for _, input := range []string{
		strings.Replace(completedOpenCodeEnvelope, `,"completed":3`, "", 1),
		strings.Replace(completedOpenCodeEnvelope, `"role":"assistant"`, `"role":"user"`, 1),
		strings.Replace(completedOpenCodeEnvelope, `"role":"assistant"`, `"role":"assistant","summary":true`, 1),
	} {
		var out bytes.Buffer
		if err := runTelemetryRuntimeInput([]string{"opencode", "--json"}, &out, strings.NewReader(input)); err != nil || !strings.Contains(out.String(), `"ignored"`) {
			t.Fatal(out.String(), err)
		}
	}
	if !reflect.DeepEqual(before, runtimeCLIDisk(t, home)) {
		t.Fatal("ignored adapter wrote state")
	}
}

func TestTelemetryRuntimeOpenCodeRejectsUnsafeEnvelope(t *testing.T) {
	home := runtimeCLIHome(t)
	before := runtimeCLIDisk(t, home)
	// Any accidental send is confined to a local server, never an external endpoint.
	runtimeCLIServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid envelope made HTTP request") })
	for _, input := range []string{
		strings.Replace(completedOpenCodeEnvelope, `"role":"assistant"`, `"role":"assistant","parts":["PRIVATE_PROMPT"]`, 1),
		strings.Replace(completedOpenCodeEnvelope, `"role":"assistant"`, `"role":"assistant","role":"user"`, 1),
		strings.Replace(completedOpenCodeEnvelope, `"created":1,`, "", 1),
		strings.Replace(completedOpenCodeEnvelope, `"role":"assistant"`, `"role":"assistant","tokens":{"input":"2"}`, 1),
		strings.Replace(completedOpenCodeEnvelope, `"role":"assistant"`, `"role":"assistant","id":"PRIVATE_SOURCE_ID"`, 1),
		strings.Replace(completedOpenCodeEnvelope, `"info":`, `"batch_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","info":`, 1),
		completedOpenCodeEnvelope + `{}`, strings.Repeat("x", 16385),
	} {
		var out bytes.Buffer
		if err := runTelemetryRuntimeInput([]string{"opencode", "--json"}, &out, strings.NewReader(input)); err != nil || !strings.Contains(out.String(), `"discarded"`) || strings.Contains(out.String(), "PRIVATE") {
			t.Fatal("unsafe input accepted or exposed", err)
		}
	}
	if !reflect.DeepEqual(before, runtimeCLIDisk(t, home)) {
		t.Fatal("invalid adapter wrote state")
	}
	// Existing policy remains byte-for-byte intact, including private install ID.
	if body, err := os.ReadFile(telemetry.Path(home)); err != nil || !bytes.Contains(body, []byte("PRIVATE_INSTALL")) {
		t.Fatal("policy replaced")
	}
}
