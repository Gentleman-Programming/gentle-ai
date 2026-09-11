package cli

import (
	"bytes"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestTelemetryRuntimeSendDiscardsFailures(t *testing.T) {
	home := runtimeCLIHome(t)
	before := runtimeCLIDisk(t, home)
	requests := 0
	runtimeCLIServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(500)
		_, _ = io.WriteString(w, "PRIVATE_SERVER_ERROR")
	})
	for _, input := range []string{string(runtimeCLIFixture()), "PRIVATE_INPUT", strings.Repeat(" ", 16385)} {
		var out bytes.Buffer
		if err := runTelemetryRuntimeInput([]string{"send", "--json"}, &out, strings.NewReader(input)); err != nil {
			t.Fatal("send failure escaped", err)
		}
		if !strings.Contains(out.String(), `"discarded"`) || strings.Contains(out.String(), "PRIVATE") {
			t.Fatal("unsafe result")
		}
		if !reflect.DeepEqual(before, runtimeCLIDisk(t, home)) {
			t.Fatal("failure wrote state")
		}
	}
	if requests != 1 {
		t.Fatal("retried or sent invalid input", requests)
	}
}
