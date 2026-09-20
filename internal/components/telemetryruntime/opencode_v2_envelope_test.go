package telemetryruntime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/telemetry"
)

func TestOpenCodeV2Envelope(t *testing.T) {
	for _, tt := range []struct{ name, schema, extra, provider, model, evidence, effort, decision string }{
		{"selected", "v2", `,"selectedEffort":"high"`, "openai", "gpt-5.6-sol", "selected", "high", "stored"},
		{"unknown", "v2", "", "", "", "unknown", "unavailable", "stored"},
		{"unknown effort", "v2", `,"selectedEffort":"private-variant"`, "openai", "gpt-5.6-sol", "selected", "unavailable", "stored"},
		{"v1 field forbidden", "v1", `,"selectedEffort":"high"`, "openai", "gpt-5.6-sol", "", "", "discarded"},
		{"v1 null field forbidden", "v1", `,"selectedEffort":null`, "openai", "gpt-5.6-sol", "", "", "discarded"},
		{"identity forbidden", "v2", `,"sessionID":"secret"`, "openai", "gpt-5.6-sol", "", "", "discarded"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			for _, key := range []string{"DO_NOT_TRACK", "GENTLE_AI_TELEMETRY", "CI", "GITHUB_ACTIONS"} {
				t.Setenv(key, "")
			}
			if err := telemetry.Save(home, telemetry.State{InstallID: "local", Enabled: true, NoticeShown: true}); err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(home, ".config", "opencode", "opencode.json")
			os.MkdirAll(filepath.Dir(p), 0700)
			os.WriteFile(p, []byte(`{"agent":{"sdd-apply":{"model":"anthropic/claude-opus-5","variant":"low"}}}`), 0600)
			var got telemetry.RuntimeEvent
			client := &http.Client{Transport: openCodeTestRoundTrip(func(r *http.Request) (*http.Response, error) {
				body, _ := io.ReadAll(r.Body)
				var err error
				got, err = telemetry.ParseRuntimeEvent(body)
				if err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}`))}, nil
			})}
			input := fmt.Sprintf(`{"schema":"gentle-ai.telemetry-opencode/%s","info":{"role":"assistant","time":{"created":1,"completed":3},"providerID":%q,"modelID":%q,"agent":"sdd-apply"%s}}`, tt.schema, tt.provider, tt.model, tt.extra)
			if decision := SendOpenCode(context.Background(), home, os.Getenv, strings.NewReader(input), client); decision != tt.decision {
				t.Fatalf("decision %s", decision)
			}
			if tt.decision == "stored" && (len(got.Rows) != 1 || got.Rows[0].ModelEvidence != tt.evidence || got.Rows[0].SelectedEffort != tt.effort) {
				t.Fatalf("rows %+v", got.Rows)
			}
		})
	}
}
