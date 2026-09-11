package cli

import (
	"bytes"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetrycollector"
)

func TestTelemetryRuntimeCollectorCompatibility(t *testing.T) {
	home := runtimeCLIHome(t)
	before := runtimeCLIDisk(t, home)
	// Collector persistence belongs to the server, outside the client HOME/XDG.
	storage, err := telemetrycollector.OpenStorage(filepath.Join(t.TempDir(), "collector.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	mux := (&telemetrycollector.Server{Storage: storage, Limiter: telemetrycollector.NewRateLimiter(100)}).NewMux()
	requests := 0
	runtimeCLIServer(t, func(w http.ResponseWriter, r *http.Request) { requests++; mux.ServeHTTP(w, r) })
	for _, tc := range []struct{ route, input string }{{"send", string(runtimeCLIFixture())}, {"opencode", completedOpenCodeEnvelope}} {
		var out bytes.Buffer
		if err := runTelemetryRuntimeInput([]string{tc.route, "--json"}, &out, strings.NewReader(tc.input)); err != nil || !strings.Contains(out.String(), `"stored"`) {
			t.Fatal("collector did not commit simplified event", out.String(), err)
		}
	}
	if requests != 2 || !reflect.DeepEqual(before, runtimeCLIDisk(t, home)) {
		t.Fatal("wrong request count or client disk mutation")
	}
}
