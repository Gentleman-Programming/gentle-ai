package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/telemetryruntime"
	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

// Tests inject transport and the stdin deadline. Native execution never spawns
// another sender. The 500 ms input budget precedes the 3-second HTTP budget.
var runtimeHTTPClient = func() *http.Client { return nil }
var runtimeStdinTimeout = 500 * time.Millisecond

// readRuntimeStdin is CLI/process scoped, not a library reader API. We do not own
// arbitrary caller readers (including shared os.Stdin), so never close them. On
// timeout at most this one reader goroutine can remain until process exit. Its
// buffered result cannot block a late completion, and its byte limit bounds data.
func readRuntimeStdin(ctx context.Context, input io.Reader) ([]byte, bool) {
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(input, telemetry.RuntimeMaxBytes+1))
		done <- result{data, err}
	}()
	select {
	case <-ctx.Done():
		return nil, false
	case got := <-done:
		return got.data, ctx.Err() == nil && got.err == nil && len(got.data) <= telemetry.RuntimeMaxBytes
	}
}

func runTelemetryRuntime(args []string, stdout io.Writer) error {
	return runTelemetryRuntimeInput(args, stdout, os.Stdin)
}

func runTelemetryRuntimeInput(args []string, stdout io.Writer, input io.Reader) error {
	if len(args) != 2 || (args[0] != "send" && args[0] != "opencode") || args[1] != "--json" {
		return errors.New("usage: gentle-ai telemetry runtime <send|opencode> --json (bounded aggregate on stdin)")
	}
	decision := "disabled"
	if telemetry.Decide(os.Getenv, telemetry.State{Enabled: true}).Enabled {
		home, err := osUserHomeDir()
		if err != nil {
			decision = "discarded"
		} else {
			// Preserve enrolled-policy gating before any raw input is consumed. The
			// library rechecks fresh policy after this read and immediately before HTTP.
			policy, err := telemetry.LoadPolicyState(home)
			if err == nil && policy.NoticeShown && telemetry.Decide(os.Getenv, policy).Enabled {
				ctx, cancel := context.WithTimeout(context.Background(), runtimeStdinTimeout)
				data, ok := readRuntimeStdin(ctx, input)
				cancel()
				decision = "discarded"
				if ok {
					if args[0] == "opencode" {
						decision = telemetryruntime.SendOpenCode(context.Background(), home, os.Getenv, bytes.NewReader(data), runtimeHTTPClient())
					} else {
						decision = telemetry.SendRuntime(context.Background(), home, os.Getenv, bytes.NewReader(data), runtimeHTTPClient())
					}
				}
			}
		}
	}
	return encodeReviewJSON(stdout, struct {
		Schema   string `json:"schema"`
		Decision string `json:"decision"`
	}{"gentle-ai.telemetry-runtime-send/v1", decision})
}
